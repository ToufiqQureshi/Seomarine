package sam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/toufiqqureshi/uplumo/backend/internal/mcp"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/ids"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/openrouter"
)

const (
	// maxSteps is generous because SAM runs complex multi-step work in one turn
	// (site-read intake plus a full research chain, multi-competitor sweeps);
	// cost is bounded by per-chunk metering and the model stopping on its own.
	maxSteps = 40
	// maxOutputTokens is shared by max-effort reasoning and the visible reply,
	// so it stays well above measured reasoning use (~3k tokens). A tight cap
	// lets reasoning eat the reply.
	maxOutputTokens = 16_000

	// billingChunkUSD batches metering: spend accumulates step by step and is
	// charged once this much is unbilled, with the remainder flushed when the
	// turn ends. The user pays the turn's actual provider cost either way.
	billingChunkUSD = 0.05
	// maxTurnUSD stops one turn once its provider cost reaches it, so a model
	// stuck calling tools cannot spend a workspace's month in one message.
	maxTurnUSD = 1.0

	// compactAfterTokens is the estimated transcript size that triggers a
	// summary of the oldest messages between turns.
	compactAfterTokens = 120_000
	// keepRecentMessages stay verbatim when older ones are summarized.
	keepRecentMessages = 6

	titleMaxChars = 60
	defaultTitle  = "New chat"

	persistTimeout = 10 * time.Second
)

var errStreamClosed = errors.New("stream closed")

// LLM streams one model completion. *openrouter.Client implements it.
type LLM interface {
	Stream(ctx context.Context, req openrouter.Request, h openrouter.Handler) (openrouter.Result, error)
}

// Meter gates turns on credits and charges provider spend. Uplumo is a SaaS
// product and the model key is the platform's, so production always sets one;
// nil only exists for tests and local development.
type Meter interface {
	// HasCredits reports whether the organization may start a turn.
	HasCredits(ctx context.Context, organizationID string) (bool, error)
	// Charge bills costUSD of provider spend. key makes a retry idempotent.
	Charge(ctx context.Context, organizationID string, costUSD float64, key string) error
}

// SessionContext is everything a turn needs to know about who is chatting.
type SessionContext struct {
	SessionID string
	Title     string
	Project   projectInfo
	UserID    string
	UserEmail string
	// OrganizationID is the project's organization.
	OrganizationID string
	Role           string
}

// SessionTitler names and touches the registry row.
type SessionTitler interface {
	SetTitle(ctx context.Context, sessionID, title string) error
	Touch(ctx context.Context, sessionID string) error
}

// Memory renders the project's shared memory for the prompt.
type Memory interface {
	// Render returns the memory as Markdown, and whether the project has no
	// business overview yet (which switches SAM into its intake flow).
	Render(ctx context.Context, projectID string) (markdown string, intake bool, err error)
}

// Agent runs SAM chat turns.
type Agent struct {
	LLM        LLM
	Model      string
	Store      MessageStore
	Titles     SessionTitler
	Memory     Memory
	Runner     ToolRunner
	Guard      URLGuard
	Meter      Meter
	Logger     *slog.Logger
	BaseURL    string
	skillsOnce sync.Once
	skillList  []skill
}

// skills returns the embedded skill catalog, loaded once. A catalog that fails
// to parse is a build defect (a test covers it); the agent runs without skills
// rather than refusing to chat.
func (a *Agent) skills() []skill {
	a.skillsOnce.Do(func() {
		list, err := loadSkills()
		if err != nil {
			a.logger().Error("load SAM skills", "err", err)
			return
		}
		a.skillList = list
	})
	return a.skillList
}

// Refusal texts. They stream as ordinary assistant messages, so a refusal is
// free (no provider call) and renders and persists like any other reply.
const (
	failureText          = "SAM could not finish this reply. Please try again."
	refuseNoCredits      = "You're out of credits. Top up to keep using SAM."
	refuseRanOutMidTurn  = "You ran out of credits, so I stopped here. Your credits refill on your next billing date, or upgrade your plan for more."
	refuseTurnLimit      = "This request was getting expensive, so I stopped here to protect your credits. Ask me to continue if you need more."
	refuseBillingUnknown = "I can't check your credits right now, so I didn't start. Please try again in a minute."
	refuseNoAccess       = "You no longer have access to this organization, so I can't continue this chat."
)

// Run streams the reply to userMessage, which the caller has already stored,
// and stores the reply. It returns an error only for failures the caller should
// log; model and tool failures reach the client as an error chunk instead.
func (a *Agent) Run(ctx context.Context, sc SessionContext, userMessage Message, sink Sink) error {
	assistantID, err := ids.New()
	if err != nil {
		return err
	}
	turn := &turnState{agent: a, sc: sc, sink: sink, assistant: Message{ID: assistantID, Role: "assistant"}}
	if err := sink.Send(Chunk{"type": "start", "messageId": assistantID}); err != nil {
		return nil // the client is already gone; nothing was generated
	}
	defer func() {
		turn.finish(ctx)
		a.afterTurn(ctx, sc, userMessage)
	}()

	if refusal := a.gate(ctx, sc); refusal != "" {
		turn.refuse(refusal)
		return nil
	}
	return turn.run(ctx)
}

// gate refuses a turn the organization cannot pay for. Depleted credits are
// checked here, per turn, because a long-lived chat outlives plan changes.
func (a *Agent) gate(ctx context.Context, sc SessionContext) string {
	if a.Meter == nil {
		return ""
	}
	if sc.Role == "" {
		return refuseNoAccess
	}
	ok, err := a.Meter.HasCredits(ctx, sc.OrganizationID)
	if err != nil {
		// Fail closed: a turn that cannot be paid for must not run on
		// Uplumo's money.
		a.logger().ErrorContext(ctx, "SAM credit check failed", "err", err)
		return refuseBillingUnknown
	}
	if !ok {
		return refuseNoCredits
	}
	return ""
}

func (a *Agent) logger() *slog.Logger {
	if a.Logger != nil {
		return a.Logger
	}
	return slog.Default()
}

// afterTurn names the session from its first message and compacts long chats.
// It is best effort: the reply is already stored.
func (a *Agent) afterTurn(ctx context.Context, sc SessionContext, userMessage Message) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*persistTimeout)
	defer cancel()
	if sc.Title == defaultTitle {
		if title := deriveTitle(joinText(userMessage.Parts)); title != defaultTitle {
			if err := a.Titles.SetTitle(ctx, sc.SessionID, title); err != nil {
				a.logger().WarnContext(ctx, "SAM title update failed", "err", err)
			}
		}
	} else if err := a.Titles.Touch(ctx, sc.SessionID); err != nil {
		a.logger().WarnContext(ctx, "SAM session touch failed", "err", err)
	}
	if err := a.compact(ctx, sc); err != nil {
		a.logger().WarnContext(ctx, "SAM compaction failed", "err", err)
	}
}

// deriveTitle shortens the first user message into a session title.
func deriveTitle(text string) string {
	trimmed := strings.Join(strings.Fields(text), " ")
	if trimmed == "" {
		return defaultTitle
	}
	if utf8.RuneCountInString(trimmed) > titleMaxChars {
		return string([]rune(trimmed)[:titleMaxChars-3]) + "…"
	}
	return trimmed
}

// turnState is one turn's mutable state.
type turnState struct {
	agent     *Agent
	sc        SessionContext
	sink      Sink
	assistant Message
	unbilled  float64
	// spent is the turn's provider cost so far, billed or not.
	spent   float64
	charges int
	// stepChars counts the characters the current step streamed, to price a
	// call whose usage never arrived.
	stepChars int
	finished  bool
	// open streaming parts of the current step, as indexes into assistant.Parts
	// (-1 when none).
	textPart, reasoningPart int
	partCounter             int
}

func (t *turnState) newPartID(kind string) string {
	t.partCounter++
	return fmt.Sprintf("%s-%s-%d", kind, t.assistant.ID, t.partCounter)
}

// refuse streams a canned reply.
func (t *turnState) refuse(text string) {
	id := t.newPartID("text")
	t.assistant.Parts = append(t.assistant.Parts, Part{Type: "step-start"}, Part{Type: "text", Text: text, State: stateDone})
	_ = t.sink.Send(Chunk{"type": "start-step"})
	_ = t.sink.Send(Chunk{"type": "text-start", "id": id})
	_ = t.sink.Send(Chunk{"type": "text-delta", "id": id, "delta": text})
	_ = t.sink.Send(Chunk{"type": "text-end", "id": id})
	_ = t.sink.Send(Chunk{"type": "finish-step"})
}

// finish stores the assistant message and flushes unbilled spend. It runs on
// every exit path, on a context that survives the client leaving, so a turn
// that was stopped or failed still bills what it spent and keeps its partial
// reply.
func (t *turnState) finish(ctx context.Context) {
	if t.finished {
		return
	}
	t.finished = true
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	t.flushSpend(ctx)
	// A tool that never returned must not stay "running" in the transcript.
	for i := range t.assistant.Parts {
		part := &t.assistant.Parts[i]
		if strings.HasPrefix(part.Type, "tool-") && part.State != stateOutputAvailable && part.State != stateOutputError {
			part.State = stateOutputError
			part.ErrorText = "Stopped before this tool returned."
		}
	}
	if len(t.assistant.Parts) > 0 {
		if err := t.agent.Store.Append(ctx, t.sc.SessionID, t.assistant); err != nil {
			t.agent.logger().ErrorContext(ctx, "store SAM reply failed", "session", t.sc.SessionID, "err", err)
		}
	}
	_ = t.sink.Send(Chunk{"type": "finish"})
}

func (t *turnState) run(ctx context.Context) error {
	a := t.agent
	system, err := a.systemPrompt(ctx, t.sc)
	if err != nil {
		return t.fail(ctx, fmt.Errorf("build system prompt: %w", err))
	}
	history, err := a.history(ctx, t.sc.SessionID)
	if err != nil {
		return t.fail(ctx, fmt.Errorf("load history: %w", err))
	}
	toolset, err := newToolset(a.Runner, scraper{guard: a.Guard}, a.skills(), t.auth(), t.sc.Project)
	if err != nil {
		return t.fail(ctx, fmt.Errorf("build toolset: %w", err))
	}

	conversation := append([]openrouter.Message{{Role: "system", Content: system}}, history...)
	for step := 0; step < maxSteps; step++ {
		if step > 0 {
			// Every step costs money, so check again before each one: credits
			// can run out, or the daily limit be reached, mid-turn.
			if t.spent >= maxTurnUSD {
				t.refuse(refuseTurnLimit)
				return nil
			}
			switch refusal := a.gate(ctx, t.sc); refusal {
			case "":
			case refuseNoCredits:
				t.refuse(refuseRanOutMidTurn)
				return nil
			default:
				t.refuse(refusal)
				return nil
			}
		}
		result, ok := t.step(ctx, conversation, toolset)
		if !ok {
			return nil
		}
		if len(result.ToolCalls) == 0 {
			return nil
		}
		conversation = append(conversation, openrouter.Message{Role: "assistant", Content: result.Text, ToolCalls: result.ToolCalls})
		for _, call := range result.ToolCalls {
			conversation = append(conversation, t.runTool(ctx, toolset, call))
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	t.refuseStepLimit()
	return nil
}

func (t *turnState) auth() mcp.Auth {
	return mcp.Auth{
		UserID:         t.sc.UserID,
		UserEmail:      t.sc.UserEmail,
		OrganizationID: t.sc.OrganizationID,
		Role:           t.sc.Role,
		// A SAM session belongs to one project's workspace; org context is
		// fixed for the session, like an OAuth token's.
		OrgScope: "pinned",
		BaseURL:  t.agent.BaseURL,
		Scopes:   []string{"mcp"},
	}
}

// step runs one model call and streams it. ok is false when the turn must end.
func (t *turnState) step(ctx context.Context, conversation []openrouter.Message, toolset *toolset) (openrouter.Result, bool) {
	a := t.agent
	t.textPart, t.reasoningPart = -1, -1
	t.stepChars = 0
	t.assistant.Parts = append(t.assistant.Parts, Part{Type: "step-start"})
	_ = t.sink.Send(Chunk{"type": "start-step"})

	var textID, reasoningID string
	handler := openrouter.Handler{
		OnReasoning: func(delta string) error {
			if t.reasoningPart < 0 {
				reasoningID = t.newPartID("reasoning")
				t.assistant.Parts = append(t.assistant.Parts, Part{Type: "reasoning", State: "streaming"})
				t.reasoningPart = len(t.assistant.Parts) - 1
				if err := t.sink.Send(Chunk{"type": "reasoning-start", "id": reasoningID}); err != nil {
					return err
				}
			}
			t.assistant.Parts[t.reasoningPart].Text += delta
			t.stepChars += len(delta)
			return t.sink.Send(Chunk{"type": "reasoning-delta", "id": reasoningID, "delta": delta})
		},
		OnText: func(delta string) error {
			if t.reasoningPart >= 0 {
				t.assistant.Parts[t.reasoningPart].State = stateDone
				t.reasoningPart = -1
				if err := t.sink.Send(Chunk{"type": "reasoning-end", "id": reasoningID}); err != nil {
					return err
				}
			}
			if t.textPart < 0 {
				textID = t.newPartID("text")
				t.assistant.Parts = append(t.assistant.Parts, Part{Type: "text", State: "streaming"})
				t.textPart = len(t.assistant.Parts) - 1
				if err := t.sink.Send(Chunk{"type": "text-start", "id": textID}); err != nil {
					return err
				}
			}
			t.assistant.Parts[t.textPart].Text += delta
			t.stepChars += len(delta)
			return t.sink.Send(Chunk{"type": "text-delta", "id": textID, "delta": delta})
		},
	}
	result, err := a.LLM.Stream(ctx, openrouter.Request{
		Model:           a.Model,
		Messages:        conversation,
		Tools:           toolset.Definitions(),
		MaxTokens:       maxOutputTokens,
		ReasoningEffort: "max",
	}, handler)

	// Close whatever was streaming, even when the call failed, so the stored
	// message and the client agree on what exists.
	if t.reasoningPart >= 0 {
		t.assistant.Parts[t.reasoningPart].State = stateDone
		_ = t.sink.Send(Chunk{"type": "reasoning-end", "id": reasoningID})
	}
	if t.textPart >= 0 {
		t.assistant.Parts[t.textPart].State = stateDone
		_ = t.sink.Send(Chunk{"type": "text-end", "id": textID})
	}
	cost, estimated := result.Usage.BillableUSD(t.stepChars)
	if estimated {
		a.logger().WarnContext(ctx, "SAM model call reported no cost; charging an estimate", "session", t.sc.SessionID, "cost_usd", cost)
	}
	t.recordSpend(ctx, cost)

	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, errStreamClosed) {
			_ = t.fail(ctx, fmt.Errorf("model call: %w", err))
		}
		return result, false
	}
	_ = t.sink.Send(Chunk{"type": "finish-step"})
	return result, true
}

// runTool executes one call and returns the message that answers it.
func (t *turnState) runTool(ctx context.Context, toolset *toolset, call openrouter.ToolCall) openrouter.Message {
	if call.ID == "" {
		if id, err := ids.New(); err == nil {
			call.ID = id
		}
	}
	input := json.RawMessage(call.Function.Arguments)
	if !json.Valid(input) {
		input = json.RawMessage(`{}`)
	}
	part := Part{Type: "tool-" + call.Function.Name, ToolCallID: call.ID, State: "input-available", Input: input}
	t.assistant.Parts = append(t.assistant.Parts, part)
	index := len(t.assistant.Parts) - 1
	_ = t.sink.Send(Chunk{"type": "tool-input-available", "toolCallId": call.ID, "toolName": call.Function.Name, "input": input})

	output, isError, err := toolset.Run(ctx, call.Function.Name, json.RawMessage(call.Function.Arguments))
	switch {
	case err != nil && ctx.Err() != nil:
		// The turn was stopped mid-tool; finish() marks the part interrupted.
		return openrouter.Message{Role: "tool", ToolCallID: call.ID, Content: interruptedToolResult}
	case err != nil || isError:
		// Failures go back to the model so it can recover or report them,
		// instead of aborting the whole turn on one bad call.
		text := "tool failed"
		if err != nil {
			text = err.Error()
		}
		t.assistant.Parts[index].State = stateOutputError
		t.assistant.Parts[index].ErrorText = text
		_ = t.sink.Send(Chunk{"type": "tool-output-error", "toolCallId": call.ID, "errorText": text})
		return openrouter.Message{Role: "tool", ToolCallID: call.ID, Content: "Error: " + text}
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		encoded = []byte(`null`)
	}
	t.assistant.Parts[index].State = stateOutputAvailable
	t.assistant.Parts[index].Output = encoded
	_ = t.sink.Send(Chunk{"type": "tool-output-available", "toolCallId": call.ID, "output": json.RawMessage(encoded)})
	return openrouter.Message{Role: "tool", ToolCallID: call.ID, Content: string(encoded)}
}

func (t *turnState) refuseStepLimit() {
	const text = "I hit my step limit for one reply. Everything above is saved; tell me where to pick up."
	t.assistant.Parts = append(t.assistant.Parts, Part{Type: "text", Text: text, State: stateDone})
	id := t.newPartID("text")
	_ = t.sink.Send(Chunk{"type": "text-start", "id": id})
	_ = t.sink.Send(Chunk{"type": "text-delta", "id": id, "delta": text})
	_ = t.sink.Send(Chunk{"type": "text-end", "id": id})
}

// fail logs a turn-level failure and tells the client something went wrong. The
// cause stays in the log: provider and database text is not for the user.
func (t *turnState) fail(ctx context.Context, cause error) error {
	t.agent.logger().ErrorContext(ctx, "SAM turn failed", "session", t.sc.SessionID, "err", cause)
	_ = t.sink.Send(Chunk{"type": "error", "errorText": failureText})
	return cause
}

// recordSpend adds provider cost to the turn and charges once a chunk of it has
// accumulated.
func (t *turnState) recordSpend(ctx context.Context, costUSD float64) {
	if costUSD > 0 {
		t.unbilled += costUSD
		t.spent += costUSD
	}
	if t.unbilled >= billingChunkUSD {
		t.flushSpend(ctx)
	}
}

func (t *turnState) flushSpend(ctx context.Context) {
	a := t.agent
	if a.Meter == nil || t.unbilled <= 0 {
		return
	}
	cost := t.unbilled
	t.unbilled = 0
	t.charges++
	key := fmt.Sprintf("sam:%s:%d", t.assistant.ID, t.charges)
	chargeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	if err := a.Meter.Charge(chargeCtx, t.sc.OrganizationID, cost, key); err != nil {
		a.logger().ErrorContext(ctx, "SAM credit metering failed", "session", t.sc.SessionID, "cost_usd", cost, "err", err)
	}
}

// systemPrompt renders the identity, project memory and skill blocks fresh each
// turn, so context written by another session, the settings UI or an MCP client
// is current.
func (a *Agent) systemPrompt(ctx context.Context, sc SessionContext) (string, error) {
	memory, intake, err := a.Memory.Render(ctx, sc.Project.ID)
	if err != nil {
		return "", err
	}
	prompt := buildSystemPrompt(sc.Project, intake, memory, a.skills())
	summary, ok, err := a.Store.Summary(ctx, sc.SessionID)
	if err != nil {
		return "", err
	}
	if ok {
		prompt += "\n\n## earlier_conversation\nThe oldest part of this chat was summarized to save space:\n\n" + summary.Text
	}
	return prompt, nil
}

// history loads the provider conversation: messages after the summary.
func (a *Agent) history(ctx context.Context, sessionID string) ([]openrouter.Message, error) {
	messages, err := a.Store.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if summary, ok, err := a.Store.Summary(ctx, sessionID); err != nil {
		return nil, err
	} else if ok {
		messages = messagesAfter(messages, summary.ThroughSeq)
	}
	return toLLMMessages(messages), nil
}

func messagesAfter(messages []Message, seq int64) []Message {
	for i, m := range messages {
		if m.Seq > seq {
			return messages[i:]
		}
	}
	return nil
}
