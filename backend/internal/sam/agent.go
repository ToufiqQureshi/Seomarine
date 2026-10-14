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
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
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

// Meter reserves credits before every model call and charges its real cost
// after. Uplumo is a SaaS product and the model key is the platform's, so
// production always sets one; nil only exists for tests and local development.
type Meter interface {
	// Hold reserves the most a call can cost (spend.ErrNoCredits,
	// ErrWorkspaceDailyLimit or ErrPaused when it cannot). The caller
	// settles the hold exactly once with the real cost.
	Hold(ctx context.Context, organizationID string, ceilingUSD float64) (spend.Hold, error)
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
	refuseNoCredits      = "You don't have enough credits left for an answer. Your credits refill on your next billing date, or upgrade your plan for more."
	refuseDailyLimit     = "You've used today's share of your credits, so I stopped here. It resets at midnight UTC."
	refusePaused         = "AI answers are paused on our side for a moment. Please try again later."
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

	if refusal := a.gate(sc); refusal != "" {
		turn.refuse(refusal)
		return nil
	}
	return turn.run(ctx)
}

// gate refuses a turn its user no longer has access to. Credits are checked
// by the hold before every model call.
func (a *Agent) gate(sc SessionContext) string {
	if a.Meter != nil && sc.Role == "" {
		return refuseNoAccess
	}
	return ""
}

// holdStep reserves credits for one model call. midTurn picks the wording
// when credits run out after the turn has started.
func (t *turnState) holdStep(ctx context.Context, req openrouter.Request, midTurn bool) (spend.Hold, string) {
	a := t.agent
	if a.Meter == nil {
		return nil, ""
	}
	ceiling, err := openrouter.MaxCostUSD(req)
	if err != nil {
		a.logger().ErrorContext(ctx, "SAM step has no cost ceiling", "err", err)
		return nil, refuseBillingUnknown
	}
	hold, err := a.Meter.Hold(ctx, t.sc.OrganizationID, ceiling)
	switch {
	case err == nil:
		return hold, ""
	case errors.Is(err, spend.ErrNoCredits) && midTurn:
		return nil, refuseRanOutMidTurn
	case errors.Is(err, spend.ErrNoCredits):
		return nil, refuseNoCredits
	case errors.Is(err, spend.ErrWorkspaceDailyLimit):
		return nil, refuseDailyLimit
	case errors.Is(err, spend.ErrPaused):
		return nil, refusePaused
	default:
		// Fail closed: a call that cannot be paid for must not run on
		// Uplumo's money.
		a.logger().ErrorContext(ctx, "SAM credit hold failed", "err", err)
		return nil, refuseBillingUnknown
	}
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
	// spent is the turn's provider cost so far.
	spent float64
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

// finish stores the assistant message. It runs on every exit path, on a
// context that survives the client leaving, so a turn that was stopped or
// failed keeps its partial reply. Each step settled its own spend.
func (t *turnState) finish(ctx context.Context) {
	if t.finished {
		return
	}
	t.finished = true
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
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
		if step > 0 && t.spent >= maxTurnUSD {
			t.refuse(refuseTurnLimit)
			return nil
		}
		req := openrouter.Request{
			Model:           a.Model,
			Messages:        conversation,
			Tools:           toolset.Definitions(),
			MaxTokens:       maxOutputTokens,
			ReasoningEffort: "max",
		}
		// Every step costs money, so each holds its ceiling first: credits
		// can run out, or the daily limit be reached, mid-turn.
		hold, refusal := t.holdStep(ctx, req, step > 0)
		if refusal != "" {
			t.refuse(refusal)
			return nil
		}
		result, ok := t.step(ctx, req, hold)
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

// step runs one model call, streams it and settles its hold with the real
// cost. ok is false when the turn must end.
func (t *turnState) step(ctx context.Context, req openrouter.Request, hold spend.Hold) (openrouter.Result, bool) {
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
	result, err := a.LLM.Stream(ctx, req, handler)

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
	t.settle(ctx, hold, cost)

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

// settle charges a step's real cost against its hold, on a context that
// survives the client leaving: the provider has already been paid.
func (t *turnState) settle(ctx context.Context, hold spend.Hold, costUSD float64) {
	t.spent += costUSD
	if hold == nil {
		return
	}
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	if err := hold.Settle(settleCtx, costUSD); err != nil {
		t.agent.logger().ErrorContext(ctx, "SAM credit settle failed", "session", t.sc.SessionID, "cost_usd", costUSD, "err", err)
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
