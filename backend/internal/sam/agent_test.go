package sam

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/toufiqqureshi/uplumo/backend/internal/mcp"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/openrouter"
)

func partTypes(m Message) []string {
	out := make([]string, len(m.Parts))
	for i, p := range m.Parts {
		out[i] = p.Type
	}
	return out
}

func TestTextTurnStreamsAndStores(t *testing.T) {
	h := newHarness(t, scriptStep{reasoning: []string{"hm"}, text: []string{"Hello ", "there"}})
	sink := &recordingSink{}
	h.run(context.Background(), t, "What should I do next?", sink)

	want := []string{"start", "start-step", "reasoning-start", "reasoning-delta", "reasoning-end",
		"text-start", "text-delta", "text-delta", "text-end", "finish-step", "finish"}
	if got := sink.types(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("chunks = %v\nwant     %v", got, want)
	}
	assistant := h.lastAssistant(t)
	if got := partTypes(assistant); strings.Join(got, ",") != "step-start,reasoning,text" {
		t.Fatalf("stored parts = %v", got)
	}
	if assistant.Parts[2].Text != "Hello there" || assistant.Parts[2].State != stateDone {
		t.Fatalf("stored text = %+v", assistant.Parts[2])
	}
	if got := sink.chunks[0]["messageId"]; got != assistant.ID {
		t.Fatalf("start messageId = %v, stored id = %s", got, assistant.ID)
	}
	if strings.Join(h.titles.titles, "|") != "What should I do next?" {
		t.Fatalf("titles = %v", h.titles.titles)
	}
}

func TestSecondTurnTouchesInsteadOfRetitling(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"ok"}})
	h.sc.Title = "Already named"
	h.run(context.Background(), t, "hello", &recordingSink{})
	if len(h.titles.titles) != 0 || h.titles.touched != 1 {
		t.Fatalf("titles = %v touched = %d", h.titles.titles, h.titles.touched)
	}
}

func TestSystemPromptCarriesMemoryAndProject(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"ok"}})
	h.run(context.Background(), t, "hello", &recordingSink{})
	system := h.llm.requests[0].Messages[0]
	if system.Role != "system" {
		t.Fatalf("first message role = %s", system.Role)
	}
	for _, want := range []string{"You are SAM", `"Acme"`, "projectId: project-1", "Acme sells anvils.", "## skills", "keyword-research"} {
		if !strings.Contains(system.Content, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestToolTurnBindsProjectAndFeedsResultBack(t *testing.T) {
	h := newHarness(t,
		scriptStep{toolCalls: []openrouter.ToolCall{toolCall("call-1", "get_domain_overview", `{"domain":"acme.test"}`)}},
		scriptStep{text: []string{"Here you go."}},
	)
	h.runner.results["get_domain_overview"] = []mcp.ToolResult{{Text: "Domain overview", Structured: map[string]any{"rows": []any{"a", "b"}}}}
	sink := &recordingSink{}
	h.run(context.Background(), t, "overview please", sink)

	if len(h.runner.calls) != 1 {
		t.Fatalf("tool calls = %d", len(h.runner.calls))
	}
	call := h.runner.calls[0]
	var args map[string]any
	_ = json.Unmarshal(call.args, &args)
	if args["projectId"] != "project-1" || args["domain"] != "acme.test" {
		t.Fatalf("tool arguments = %s", call.args)
	}
	if call.auth.UserID != "user-1" || call.auth.OrganizationID != "org-1" || call.auth.OrgScope != "pinned" || call.auth.BaseURL != "https://app.example.test" {
		t.Fatalf("tool auth = %+v", call.auth)
	}

	assistant := h.lastAssistant(t)
	if got := partTypes(assistant); strings.Join(got, ",") != "step-start,tool-get_domain_overview,step-start,text" {
		t.Fatalf("stored parts = %v", got)
	}
	tool := assistant.Parts[1]
	if tool.State != stateOutputAvailable || tool.ToolCallID != "call-1" || !strings.Contains(string(tool.Output), `"summary":"Domain overview"`) {
		t.Fatalf("tool part = %+v output=%s", tool, tool.Output)
	}

	// The second model call must carry the assistant tool call and its result.
	second := h.llm.requests[1].Messages
	roles := make([]string, len(second))
	for i, m := range second {
		roles[i] = m.Role
	}
	if strings.Join(roles, ",") != "system,user,assistant,tool" {
		t.Fatalf("second request roles = %v", roles)
	}
	if second[3].ToolCallID != "call-1" || !strings.Contains(second[3].Content, "Domain overview") {
		t.Fatalf("tool message = %+v", second[3])
	}
	types := sink.types()
	for _, want := range []string{"tool-input-available", "tool-output-available"} {
		if !contains(types, want) {
			t.Errorf("missing chunk %s in %v", want, types)
		}
	}
}

func TestModelCannotTargetAnotherProject(t *testing.T) {
	h := newHarness(t,
		scriptStep{toolCalls: []openrouter.ToolCall{toolCall("c", "get_domain_overview", `{"projectId":"someone-elses","domain":"x.test"}`)}},
		scriptStep{text: []string{"done"}},
	)
	h.run(context.Background(), t, "go", &recordingSink{})
	var args map[string]any
	_ = json.Unmarshal(h.runner.calls[0].args, &args)
	if args["projectId"] != "project-1" {
		t.Fatalf("a model-supplied projectId must be overwritten, got %v", args["projectId"])
	}
}

func TestToolSchemaHidesProjectID(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"ok"}})
	h.run(context.Background(), t, "hi", &recordingSink{})
	for _, tool := range h.llm.requests[0].Tools {
		if tool.Name == "get_domain_overview" {
			if strings.Contains(string(tool.Parameters), "projectId") {
				t.Fatalf("model-facing schema still has projectId: %s", tool.Parameters)
			}
			return
		}
	}
	t.Fatal("get_domain_overview not offered to the model")
}

func TestToolFailuresGoBackToTheModel(t *testing.T) {
	h := newHarness(t,
		scriptStep{toolCalls: []openrouter.ToolCall{
			toolCall("a", "get_domain_overview", `{}`),
			toolCall("b", "no_such_tool", `{}`),
			toolCall("c", "get_serp_results", `not json`),
		}},
		scriptStep{text: []string{"Sorry."}},
	)
	h.runner.results["get_domain_overview"] = []mcp.ToolResult{{Text: "out of credits", IsError: true}}
	sink := &recordingSink{}
	h.run(context.Background(), t, "go", sink)

	assistant := h.lastAssistant(t)
	states := map[string]string{}
	for _, p := range assistant.Parts {
		if strings.HasPrefix(p.Type, "tool-") {
			states[p.ToolCallID] = p.State
		}
	}
	for id, state := range states {
		if state != stateOutputError {
			t.Errorf("tool %s state = %s, want output-error", id, state)
		}
	}
	if len(states) != 3 {
		t.Fatalf("tool parts = %v", states)
	}
	if h.llm.callCount() != 2 {
		t.Fatalf("a failing tool must not end the turn: model calls = %d", h.llm.callCount())
	}
	tools := h.llm.requests[1].Messages
	var errorResults int
	for _, m := range tools {
		if m.Role == "tool" && strings.HasPrefix(m.Content, "Error: ") {
			errorResults++
		}
	}
	if errorResults != 3 {
		t.Fatalf("error results sent to the model = %d, want 3", errorResults)
	}
	if !contains(sink.types(), "tool-output-error") {
		t.Fatalf("chunks = %v", sink.types())
	}
	// Invalid arguments from the model must still produce a valid input chunk.
	for _, c := range sink.chunks {
		if c["type"] == "tool-input-available" && c["toolCallId"] == "c" {
			if input, _ := json.Marshal(c["input"]); string(input) != "{}" {
				t.Errorf("input for invalid args = %s", input)
			}
		}
	}
}

func TestOutOfCreditsRefusesWithoutCallingTheModel(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"should not run"}})
	h.agent.Meter = h.meter
	h.meter.hasCred = false
	sink := &recordingSink{}
	h.run(context.Background(), t, "hi", sink)

	if h.llm.callCount() != 0 {
		t.Fatalf("model called %d times for a refused turn", h.llm.callCount())
	}
	assistant := h.lastAssistant(t)
	if assistant.Parts[len(assistant.Parts)-1].Text != refuseNoCredits {
		t.Fatalf("stored reply = %+v", assistant.Parts)
	}
	if len(h.meter.charges) != 0 {
		t.Fatalf("a refusal must be free, charges = %v", h.meter.charges)
	}
}

func TestRemovedMemberIsRefused(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"should not run"}})
	h.agent.Meter = h.meter
	h.sc.Role = ""
	h.run(context.Background(), t, "hi", &recordingSink{})
	if h.llm.callCount() != 0 || h.lastAssistant(t).Parts[1].Text != refuseNoAccess {
		t.Fatalf("calls = %d reply = %+v", h.llm.callCount(), h.lastAssistant(t).Parts)
	}
}

// A turn that cannot be paid for must not run on Uplumo's money: when the
// credit check fails, SAM refuses instead of answering for free.
func TestCreditCheckFailureFailsClosed(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"ok"}})
	h.agent.Meter = h.meter
	h.meter.credErr = errors.New("ledger down")
	h.run(context.Background(), t, "hi", &recordingSink{})
	if h.llm.callCount() != 0 {
		t.Fatalf("model calls = %d during a billing outage, want 0", h.llm.callCount())
	}
	if got := h.lastAssistant(t).Parts[1].Text; got != refuseBillingUnknown {
		t.Fatalf("reply = %q", got)
	}
	if len(h.meter.charges) != 0 {
		t.Fatalf("charges = %v", h.meter.charges)
	}
}

func TestTurnStopsWhenCreditsRunOutMidTurn(t *testing.T) {
	h := newHarness(t,
		scriptStep{toolCalls: []openrouter.ToolCall{toolCall("1", "whoami", `{}`)}, cost: 0.06},
		scriptStep{text: []string{"should not run"}, cost: 0.5},
	)
	h.agent.Meter = h.meter
	h.meter.credFn = func(n int) bool { return n == 1 }
	h.run(context.Background(), t, "go", &recordingSink{})
	if h.llm.callCount() != 1 {
		t.Fatalf("model calls = %d, want 1: the second step must not start without credits", h.llm.callCount())
	}
	parts := h.lastAssistant(t).Parts
	if got := parts[len(parts)-1].Text; got != refuseRanOutMidTurn {
		t.Fatalf("last part = %q, want the ran-out message", got)
	}
	var charged float64
	for _, c := range h.meter.charges {
		charged += c
	}
	if charged < 0.0599 || charged > 0.0601 {
		t.Fatalf("charged %v, want the first step's 0.06", charged)
	}
}

func TestTurnStopsAtTheCostLimit(t *testing.T) {
	steps := make([]scriptStep, 0, 5)
	for i := range 5 {
		steps = append(steps, scriptStep{toolCalls: []openrouter.ToolCall{toolCall(strconv.Itoa(i), "whoami", `{}`)}, cost: 0.6})
	}
	h := newHarness(t, steps...)
	h.agent.Meter = h.meter
	h.meter.hasCred = true
	h.run(context.Background(), t, "go", &recordingSink{})
	if h.llm.callCount() != 2 {
		t.Fatalf("model calls = %d, want 2: the turn passed %v after two steps", h.llm.callCount(), maxTurnUSD)
	}
	parts := h.lastAssistant(t).Parts
	if got := parts[len(parts)-1].Text; got != refuseTurnLimit {
		t.Fatalf("last part = %q, want the turn-limit message", got)
	}
}

func TestAnAnswerWithoutAReportedCostIsStillCharged(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"a reply that the provider forgot to price"}})
	h.agent.Meter = h.meter
	h.meter.hasCred = true
	h.run(context.Background(), t, "hi", &recordingSink{})
	if len(h.meter.charges) != 1 || h.meter.charges[0] <= 0 {
		t.Fatalf("charges = %v, want one estimated charge", h.meter.charges)
	}
}

func TestSpendIsMeteredInChunksAndFlushedAtTheEnd(t *testing.T) {
	h := newHarness(t,
		scriptStep{toolCalls: []openrouter.ToolCall{toolCall("1", "whoami", `{}`)}, cost: 0.03},
		scriptStep{toolCalls: []openrouter.ToolCall{toolCall("2", "whoami", `{}`)}, cost: 0.04},
		scriptStep{text: []string{"done"}, cost: 0.01},
	)
	h.agent.Meter = h.meter
	h.run(context.Background(), t, "go", &recordingSink{})

	// 0.03 stays unbilled; 0.03+0.04 crosses the chunk size; 0.01 flushes at the end.
	if len(h.meter.charges) != 2 {
		t.Fatalf("charges = %v", h.meter.charges)
	}
	if got := h.meter.charges[0]; got < 0.0699 || got > 0.0701 {
		t.Errorf("first charge = %v, want 0.07", got)
	}
	if got := h.meter.charges[1]; got < 0.0099 || got > 0.0101 {
		t.Errorf("second charge = %v, want 0.01", got)
	}
	if h.meter.keys[0] == h.meter.keys[1] {
		t.Errorf("charge keys must differ so neither is deduplicated: %v", h.meter.keys)
	}
}

func TestMeteringFailureDoesNotFailTheTurn(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"ok"}, cost: 0.2})
	h.agent.Meter = h.meter
	h.meter.chargeFn = func() error { return errors.New("ledger down") }
	sink := &recordingSink{}
	h.run(context.Background(), t, "hi", sink)
	if contains(sink.types(), "error") || len(h.lastAssistant(t).Parts) == 0 {
		t.Fatalf("chunks = %v", sink.types())
	}
}

func TestStoppedTurnKeepsPartialReplyAndStillBills(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := newHarness(t, scriptStep{text: []string{"partial "}, cost: 0.02, err: context.Canceled})
	h.agent.Meter = h.meter
	h.llm.steps[0].err = nil
	h.agent.LLM = cancelingLLM{inner: h.llm, cancel: cancel}
	sink := &recordingSink{}
	h.run(ctx, t, "hi", sink)

	assistant := h.lastAssistant(t)
	text := assistant.Parts[len(assistant.Parts)-1]
	if text.Type != "text" || text.Text != "partial " {
		t.Fatalf("partial reply lost: %+v", assistant.Parts)
	}
	if len(h.meter.charges) != 1 {
		t.Fatalf("a stopped turn still consumed what it consumed: charges = %v", h.meter.charges)
	}
}

// cancelingLLM cancels the turn after the first delta, like a client stopping.
type cancelingLLM struct {
	inner  *scriptedLLM
	cancel context.CancelFunc
}

func (c cancelingLLM) Stream(ctx context.Context, req openrouter.Request, h openrouter.Handler) (openrouter.Result, error) {
	onText := h.OnText
	h.OnText = func(d string) error {
		err := onText(d)
		c.cancel()
		return err
	}
	result, err := c.inner.Stream(ctx, req, h)
	if err == nil {
		err = ctx.Err()
	}
	return result, err
}

func TestInterruptedToolIsNotLeftRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	h := newHarness(t, scriptStep{toolCalls: []openrouter.ToolCall{toolCall("1", "get_audit_status", `{}`)}})
	h.runner.results["get_audit_status"] = []mcp.ToolResult{{Text: "running", Structured: map[string]any{"status": map[string]any{"status": "running"}}}}
	// Cancel as soon as the tool is invoked.
	h.agent.Runner = cancelOnCall{fakeRunner: h.runner, cancel: cancel}
	user := Message{ID: "user-msg-go", Role: "user", Parts: []Part{{Type: "text", Text: "go"}}}
	if err := h.store.Append(ctx, h.sc.SessionID, user); err != nil {
		t.Fatal(err)
	}
	if err := h.agent.Run(ctx, h.sc, user, &recordingSink{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", err)
	}

	for _, p := range h.lastAssistant(t).Parts {
		if strings.HasPrefix(p.Type, "tool-") && p.State != stateOutputError && p.State != stateOutputAvailable {
			t.Fatalf("tool part left in state %q", p.State)
		}
	}
}

type cancelOnCall struct {
	*fakeRunner
	cancel context.CancelFunc
}

func (c cancelOnCall) Call(ctx context.Context, a mcp.Auth, name string, args json.RawMessage) (mcp.ToolResult, bool) {
	c.cancel()
	return c.fakeRunner.Call(ctx, a, name, args)
}

func TestModelFailureReachesTheClientAsAnErrorChunk(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"half a sent"}, err: errors.New("upstream 502 secret detail")})
	sink := &recordingSink{}
	h.run(context.Background(), t, "hi", sink)

	var errorText string
	for _, c := range sink.chunks {
		if c["type"] == "error" {
			errorText, _ = c["errorText"].(string)
		}
	}
	if errorText == "" || strings.Contains(errorText, "secret detail") {
		t.Fatalf("error chunk = %q (provider details must not leak)", errorText)
	}
	if h.lastAssistant(t).Parts[len(h.lastAssistant(t).Parts)-1].Text != "half a sent" {
		t.Fatalf("partial reply lost: %+v", h.lastAssistant(t).Parts)
	}
	if got := sink.types(); got[len(got)-1] != "finish" {
		t.Fatalf("stream must still end with finish: %v", got)
	}
}

func TestStepLimitEndsRunawayLoops(t *testing.T) {
	h := newHarness(t, scriptStep{toolCalls: []openrouter.ToolCall{toolCall("x", "whoami", `{}`)}})
	h.llm.repeat = true
	h.run(context.Background(), t, "loop", &recordingSink{})
	if h.llm.callCount() != maxSteps {
		t.Fatalf("model calls = %d, want %d", h.llm.callCount(), maxSteps)
	}
	last := h.lastAssistant(t).Parts
	if !strings.Contains(last[len(last)-1].Text, "step limit") {
		t.Fatalf("the user needs to be told why the reply stopped: %+v", last[len(last)-1])
	}
}

func TestClientLeavingStopsTheTurnQuietly(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"a", "b", "c"}})
	sink := &recordingSink{failAfter: 4}
	h.run(context.Background(), t, "hi", sink)
	if h.llm.callCount() != 1 {
		t.Fatalf("calls = %d", h.llm.callCount())
	}
}

func TestSummaryReplacesOlderHistory(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"ok"}})
	ctx := context.Background()
	for i, text := range []string{"old question", "new question"} {
		_ = h.store.Append(ctx, h.sc.SessionID, Message{ID: "u" + string(rune('a'+i)), Role: "user", Parts: []Part{{Type: "text", Text: text}}})
	}
	_ = h.store.SaveSummary(ctx, h.sc.SessionID, Summary{Text: "They sell anvils.", ThroughSeq: 1})
	h.run(ctx, t, "latest", &recordingSink{})

	req := h.llm.requests[0].Messages
	if !strings.Contains(req[0].Content, "They sell anvils.") {
		t.Fatal("the summary must be in the system prompt")
	}
	for _, m := range req {
		if strings.Contains(m.Content, "old question") {
			t.Fatal("messages covered by the summary must not be re-sent")
		}
	}
}

func TestDeriveTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", defaultTitle},
		{"   \n\t ", defaultTitle},
		{"  find   quick\nwins ", "find quick wins"},
		{strings.Repeat("a", 60), strings.Repeat("a", 60)},
		{strings.Repeat("é", 70), strings.Repeat("é", 57) + "…"},
	}
	for _, tt := range tests {
		if got := deriveTitle(tt.in); got != tt.want {
			t.Errorf("deriveTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestSetupFailuresAreGenericToTheClientAndReturnedToTheCaller(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"never reached"}})
	h.agent.Memory = fakeMemory{err: errors.New("pq: password authentication failed for user uplumo")}
	sink := &recordingSink{}
	user := Message{ID: "u", Role: "user", Parts: []Part{{Type: "text", Text: "hi"}}}
	_ = h.store.Append(context.Background(), h.sc.SessionID, user)
	err := h.agent.Run(context.Background(), h.sc, user, sink)
	if err == nil || !strings.Contains(err.Error(), "password authentication") {
		t.Fatalf("the caller needs the cause to log it: %v", err)
	}
	var errorText string
	for _, c := range sink.chunks {
		if c["type"] == "error" {
			errorText, _ = c["errorText"].(string)
		}
	}
	if errorText != failureText {
		t.Fatalf("client error = %q, want the generic text", errorText)
	}
	if h.llm.callCount() != 0 {
		t.Fatal("the model must not be called when the prompt cannot be built")
	}
}
