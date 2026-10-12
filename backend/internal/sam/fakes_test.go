package sam

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/toufiqqureshi/uplumo/backend/internal/mcp"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/openrouter"
)

// scriptStep is one scripted model response.
type scriptStep struct {
	reasoning []string
	text      []string
	toolCalls []openrouter.ToolCall
	cost      float64
	err       error
}

type scriptedLLM struct {
	mu       sync.Mutex
	steps    []scriptStep
	requests []openrouter.Request
	// repeat serves the last step forever instead of failing when steps run out.
	repeat bool
}

func (l *scriptedLLM) Stream(_ context.Context, req openrouter.Request, h openrouter.Handler) (openrouter.Result, error) {
	l.mu.Lock()
	l.requests = append(l.requests, req)
	var step scriptStep
	switch {
	case len(l.requests) <= len(l.steps):
		step = l.steps[len(l.requests)-1]
	case l.repeat && len(l.steps) > 0:
		step = l.steps[len(l.steps)-1]
	default:
		l.mu.Unlock()
		return openrouter.Result{}, errors.New("scriptedLLM: no more steps")
	}
	l.mu.Unlock()

	var result openrouter.Result
	for _, d := range step.reasoning {
		if h.OnReasoning != nil {
			if err := h.OnReasoning(d); err != nil {
				return result, err
			}
		}
	}
	for _, d := range step.text {
		result.Text += d
		if h.OnText != nil {
			if err := h.OnText(d); err != nil {
				return result, err
			}
		}
	}
	result.ToolCalls = step.toolCalls
	result.Usage.CostUSD = step.cost
	if len(step.toolCalls) > 0 {
		result.FinishReason = "tool_calls"
	} else {
		result.FinishReason = "stop"
	}
	return result, step.err
}

func (l *scriptedLLM) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.requests)
}

type memoryStore struct {
	mu       sync.Mutex
	messages []Message
	next     int64
	summary  *Summary
}

func (s *memoryStore) List(_ context.Context, _ string) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.messages), nil
}

func (s *memoryStore) Append(_ context.Context, _ string, m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.messages {
		if existing.ID == m.ID {
			return &pgconn.PgError{Code: "23505"}
		}
	}
	s.next++
	m.Seq = s.next
	s.messages = append(s.messages, m)
	return nil
}

func (s *memoryStore) DeleteFrom(_ context.Context, _, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.messages {
		if m.ID == id {
			s.messages = s.messages[:i]
			if s.summary != nil && s.summary.ThroughSeq > int64(len(s.messages)) {
				s.summary = nil
			}
			return true, nil
		}
	}
	return false, nil
}

func (s *memoryStore) Summary(_ context.Context, _ string) (Summary, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.summary == nil {
		return Summary{}, false, nil
	}
	return *s.summary, true, nil
}

func (s *memoryStore) SaveSummary(_ context.Context, _ string, sum Summary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.summary = &sum
	return nil
}

type recordingSink struct {
	mu     sync.Mutex
	chunks []Chunk
	// failAfter makes Send fail once that many chunks were accepted (0 = never).
	failAfter int
}

func (r *recordingSink) Send(c Chunk) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failAfter > 0 && len(r.chunks) >= r.failAfter {
		return errStreamClosed
	}
	r.chunks = append(r.chunks, c)
	return nil
}

func (r *recordingSink) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.chunks))
	for i, c := range r.chunks {
		out[i], _ = c["type"].(string)
	}
	return out
}

type fakeRunner struct {
	mu      sync.Mutex
	calls   []fakeCall
	results map[string][]mcp.ToolResult
	infos   []mcp.ToolInfo
}

type fakeCall struct {
	name string
	args json.RawMessage
	auth mcp.Auth
}

func newFakeRunner() *fakeRunner {
	r := &fakeRunner{results: map[string][]mcp.ToolResult{}}
	for _, name := range samMCPTools {
		schema := `{"type":"object","properties":{"projectId":{"type":"string"},"domain":{"type":"string"}},"required":["projectId"],"additionalProperties":false}`
		if name == "whoami" {
			schema = `{"type":"object","properties":{}}`
		}
		r.infos = append(r.infos, mcp.ToolInfo{Name: name, Description: name + " description", InputSchema: json.RawMessage(schema)})
	}
	return r
}

func (r *fakeRunner) Tools() []mcp.ToolInfo { return r.infos }

func (r *fakeRunner) Call(_ context.Context, auth mcp.Auth, name string, args json.RawMessage) (mcp.ToolResult, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, fakeCall{name: name, args: args, auth: auth})
	queue := r.results[name]
	if len(queue) == 0 {
		return mcp.ToolResult{Text: "ok", Structured: map[string]any{"rows": []any{1}}}, true
	}
	result := queue[0]
	if len(queue) > 1 {
		r.results[name] = queue[1:]
	}
	return result, true
}

type fakeTitles struct {
	mu      sync.Mutex
	titles  []string
	touched int
}

func (f *fakeTitles) SetTitle(_ context.Context, _, title string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.titles = append(f.titles, title)
	return nil
}

func (f *fakeTitles) Touch(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched++
	return nil
}

type fakeMemory struct {
	markdown string
	intake   bool
	err      error
}

func (f fakeMemory) Render(context.Context, string) (string, bool, error) {
	return f.markdown, f.intake, f.err
}

type fakeMeter struct {
	mu       sync.Mutex
	hasCred  bool
	credErr  error
	charges  []float64
	keys     []string
	chargeFn func() error
}

func (m *fakeMeter) HasCredits(context.Context, string) (bool, error) { return m.hasCred, m.credErr }

func (m *fakeMeter) Charge(_ context.Context, _ string, cost float64, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.charges = append(m.charges, cost)
	m.keys = append(m.keys, key)
	if m.chargeFn != nil {
		return m.chargeFn()
	}
	return nil
}

// testGuardForAgent blocks every URL; agent tests do not scrape.
type blockAllGuard struct{ testGuard }

type harness struct {
	agent  *Agent
	llm    *scriptedLLM
	store  *memoryStore
	runner *fakeRunner
	titles *fakeTitles
	meter  *fakeMeter
	sc     SessionContext
}

func newHarness(t *testing.T, steps ...scriptStep) *harness {
	t.Helper()
	h := &harness{
		llm:    &scriptedLLM{steps: steps},
		store:  &memoryStore{},
		runner: newFakeRunner(),
		titles: &fakeTitles{},
		meter:  &fakeMeter{hasCred: true},
		sc: SessionContext{
			SessionID: "session-1", Title: defaultTitle, UserID: "user-1", UserEmail: "owner@example.test",
			OrganizationID: "org-1", Role: "owner",
			Project: projectInfo{ID: "project-1", Name: "Acme", Domain: "acme.test", LocationCode: 2840, LanguageCode: "en"},
		},
	}
	h.agent = &Agent{
		LLM: h.llm, Model: "test/model", Store: h.store, Titles: h.titles,
		Memory: fakeMemory{markdown: "## Business\nAcme sells anvils."}, Runner: h.runner,
		Guard: blockAllGuard{testGuard{allowedHost: "none.invalid"}}, BaseURL: "https://app.example.test",
	}
	return h
}

// run stores the user message like the HTTP handler does, then runs the turn.
func (h *harness) run(ctx context.Context, t *testing.T, text string, sink Sink) {
	t.Helper()
	user := Message{ID: "user-msg-" + text, Role: "user", Parts: []Part{{Type: "text", Text: text}}}
	if err := h.store.Append(ctx, h.sc.SessionID, user); err != nil {
		t.Fatal(err)
	}
	if err := h.agent.Run(ctx, h.sc, user, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func (h *harness) lastAssistant(t *testing.T) Message {
	t.Helper()
	messages, _ := h.store.List(context.Background(), h.sc.SessionID)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i]
		}
	}
	t.Fatal("no assistant message stored")
	return Message{}
}

func toolCall(id, name, args string) openrouter.ToolCall {
	return openrouter.ToolCall{ID: id, Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: args}}
}
