package sam

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/openrouter"
)

// ownerLoader only knows user-1's session-1 in project-1.
type ownerLoader struct{ sc SessionContext }

func (l ownerLoader) Load(_ context.Context, projectID, userID, sessionID string) (SessionContext, error) {
	if projectID != "project-1" || userID != "user-1" || sessionID != "session-1" {
		return SessionContext{}, ErrSessionNotFound
	}
	return l.sc, nil
}

// blockingLLM emits one delta, then holds the turn open until released or canceled.
type blockingLLM struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingLLM) Stream(ctx context.Context, _ openrouter.Request, h openrouter.Handler) (openrouter.Result, error) {
	result := openrouter.Result{Text: "working"}
	if err := h.OnText("working"); err != nil {
		return result, err
	}
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return result, ctx.Err()
	case <-b.release:
		return result, nil
	}
}

type chatServer struct {
	*httptest.Server
	h    *harness
	chat *Chat
}

func newChatServer(t *testing.T, h *harness, authed bool) *chatServer {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	chat := NewChat(h.agent, ownerLoader{sc: h.sc}, h.store)
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authed {
				r = r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user-1"}))
			}
			next.ServeHTTP(w, r)
		})
	}
	passthrough := func(next http.Handler) http.Handler { return next }
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: logger, WithSession: withSession, WithProjectAccess: passthrough, Chat: chat})
	// The real server wraps every handler in the logging middleware, which hides
	// http.Flusher; streaming must still work through it.
	server := httptest.NewServer(httpx.RequestLogging(logger)(mux))
	t.Cleanup(server.Close)
	return &chatServer{Server: server, h: h, chat: chat}
}

func (s *chatServer) post(t *testing.T, ctx context.Context, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL+"/api/v1/projects/project-1/sam/sessions/"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func chatBody(sessionID, id, text string) string {
	body, _ := json.Marshal(map[string]any{"sessionId": sessionID, "message": map[string]string{"id": id, "text": text}})
	return string(body)
}

// readEvents returns the data payloads of an event stream until it ends.
func readEvents(t *testing.T, r io.Reader) []string {
	t.Helper()
	var events []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			events = append(events, data)
		}
	}
	return events
}

func TestChatStreamsServerSentEventsInTheAISDKFormat(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"Hello ", "world"}})
	server := newChatServer(t, h, true)
	resp := server.post(t, context.Background(), "chat", chatBody("session-1", "m1", "  hi there  "))

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q", got)
	}
	if got := resp.Header.Get("x-vercel-ai-ui-message-stream"); got != "v1" {
		t.Fatalf("stream header = %q: useChat will not parse the stream without it", got)
	}
	events := readEvents(t, resp.Body)
	if len(events) < 3 || events[len(events)-1] != "[DONE]" {
		t.Fatalf("events = %v", events)
	}
	var first, last map[string]any
	_ = json.Unmarshal([]byte(events[0]), &first)
	_ = json.Unmarshal([]byte(events[len(events)-2]), &last)
	if first["type"] != "start" || last["type"] != "finish" {
		t.Fatalf("first = %v last = %v", first, last)
	}

	messages, _ := h.store.List(context.Background(), "session-1")
	if len(messages) != 2 || messages[0].Role != "user" || messages[0].Parts[0].Text != "hi there" || messages[1].Role != "assistant" {
		t.Fatalf("stored = %+v", messages)
	}
}

func TestChatRejectsBadRequests(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"x"}})
	server := newChatServer(t, h, true)
	tooLong := strings.Repeat("é", maxMessageChars+1)
	tests := []struct {
		name string
		body string
	}{
		{"empty text", chatBody("session-1", "m", "")},
		{"whitespace text", chatBody("session-1", "m", " \n\t")},
		{"too long", chatBody("session-1", "m", tooLong)},
		{"missing session", chatBody("", "m", "hi")},
		{"missing message id", chatBody("session-1", "", "hi")},
		{"id too long", chatBody("session-1", strings.Repeat("a", maxIDChars+1), "hi")},
		{"unknown field", `{"sessionId":"session-1","message":{"id":"m","text":"hi"},"extra":1}`},
		{"not json", `{`},
		{"trailing data", chatBody("session-1", "m", "hi") + `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := server.post(t, context.Background(), "chat", tt.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
	if h.llm.callCount() != 0 {
		t.Fatal("a rejected request must never reach the model")
	}
}

func TestChatHidesOtherUsersAndProjectsSessions(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"x"}})
	server := newChatServer(t, h, true)
	resp := server.post(t, context.Background(), "chat", chatBody("someone-elses-session", "m", "hi"))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if msgs, _ := h.store.List(context.Background(), "someone-elses-session"); len(msgs) != 0 || h.llm.callCount() != 0 {
		t.Fatal("nothing may be stored or generated for a session the caller does not own")
	}
	for _, path := range []string{"messages", "rewind"} {
		body := `{"sessionId":"someone-elses-session","messageId":"m"}`
		if path == "messages" {
			body = `{"sessionId":"someone-elses-session"}`
		}
		if got := server.post(t, context.Background(), path, body).StatusCode; got != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, got)
		}
	}
}

func TestChatRequiresASession(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"x"}})
	server := newChatServer(t, h, false)
	for _, path := range []string{"chat", "messages", "rewind"} {
		if got := server.post(t, context.Background(), path, chatBody("session-1", "m", "hi")).StatusCode; got != http.StatusBadRequest && got != http.StatusUnauthorized {
			t.Errorf("%s status = %d", path, got)
		}
	}
	resp := server.post(t, context.Background(), "chat", chatBody("session-1", "m", "hi"))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestChatIsUnavailableWithoutAnAgent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	withSession := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: "user-1"})))
		})
	}
	mux := http.NewServeMux()
	Mount(mux, Deps{Logger: logger, WithSession: withSession, WithProjectAccess: func(n http.Handler) http.Handler { return n }})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/project-1/sam/sessions/chat", strings.NewReader(chatBody("session-1", "m", "hi")))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestOnlyOneTurnRunsPerSession(t *testing.T) {
	h := newHarness(t)
	blocker := &blockingLLM{started: make(chan struct{}, 1), release: make(chan struct{})}
	h.agent.LLM = blocker
	server := newChatServer(t, h, true)

	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan *http.Response, 1)
	go func() { first <- server.post(t, ctx, "chat", chatBody("session-1", "m1", "first")) }()
	<-blocker.started

	second := server.post(t, context.Background(), "chat", chatBody("session-1", "m2", "second"))
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 while a turn is running", second.StatusCode)
	}
	msgs, _ := h.store.List(context.Background(), "session-1")
	for _, m := range msgs {
		if m.ID == "m2" {
			t.Fatal("a refused message must not be stored")
		}
	}
	close(blocker.release)
	resp := <-first
	_, _ = io.Copy(io.Discard, resp.Body)
	cancel()

	// The session is free again once the turn ends.
	h.agent.LLM = &scriptedLLM{steps: []scriptStep{{text: []string{"ok"}}}}
	if got := server.post(t, context.Background(), "chat", chatBody("session-1", "m3", "third")).StatusCode; got != http.StatusOK {
		t.Fatalf("status after the turn ended = %d", got)
	}
}

func TestDuplicateMessageIDIsRefused(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"ok"}}, scriptStep{text: []string{"ok"}})
	server := newChatServer(t, h, true)
	first := server.post(t, context.Background(), "chat", chatBody("session-1", "same", "hi"))
	_, _ = io.Copy(io.Discard, first.Body)
	if got := server.post(t, context.Background(), "chat", chatBody("session-1", "same", "hi")).StatusCode; got != http.StatusConflict {
		t.Fatalf("status = %d, want 409", got)
	}
	if h.llm.callCount() != 1 {
		t.Fatalf("model calls = %d: a duplicate must not be answered twice", h.llm.callCount())
	}
}

func TestClientDisconnectKeepsThePartialReply(t *testing.T) {
	h := newHarness(t)
	blocker := &blockingLLM{started: make(chan struct{}, 1), release: make(chan struct{})}
	h.agent.LLM = blocker
	server := newChatServer(t, h, true)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp := server.post(t, ctx, "chat", chatBody("session-1", "m1", "hello"))
		_, _ = io.Copy(io.Discard, resp.Body)
	}()
	<-blocker.started
	cancel()
	<-done

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		msgs, _ := h.store.List(context.Background(), "session-1")
		if len(msgs) == 2 {
			if got := msgs[1].Parts[len(msgs[1].Parts)-1].Text; got != "working" {
				t.Fatalf("partial reply = %q", got)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the partial reply was never stored after the client left")
}

func TestMessagesReturnsTheTranscript(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"hi"}})
	server := newChatServer(t, h, true)
	resp := server.post(t, context.Background(), "chat", chatBody("session-1", "m1", "hello"))
	_, _ = io.Copy(io.Discard, resp.Body)

	list := server.post(t, context.Background(), "messages", `{"sessionId":"session-1"}`)
	var messages []struct {
		ID    string `json:"id"`
		Role  string `json:"role"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(list.Body).Decode(&messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].ID != "m1" || messages[1].Role != "assistant" {
		t.Fatalf("messages = %+v", messages)
	}

	empty := newChatServer(t, newHarness(t), true)
	body, _ := io.ReadAll(empty.post(t, context.Background(), "messages", `{"sessionId":"session-1"}`).Body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("an empty transcript must be [], got %s", body)
	}
}

func TestRewindDeletesFromTheMessageOn(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"one"}}, scriptStep{text: []string{"two"}})
	server := newChatServer(t, h, true)
	for _, id := range []string{"m1", "m2"} {
		resp := server.post(t, context.Background(), "chat", chatBody("session-1", id, "question "+id))
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	resp := server.post(t, context.Background(), "rewind", `{"sessionId":"session-1","messageId":"m2"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	msgs, _ := h.store.List(context.Background(), "session-1")
	if len(msgs) != 2 || msgs[1].Role != "assistant" {
		t.Fatalf("after rewind to m2: %+v", msgs)
	}
	if got := server.post(t, context.Background(), "rewind", `{"sessionId":"session-1","messageId":"nope"}`).StatusCode; got != http.StatusNotFound {
		t.Fatalf("unknown message status = %d", got)
	}
}

func TestRewindStopsARunningTurnFirst(t *testing.T) {
	h := newHarness(t)
	blocker := &blockingLLM{started: make(chan struct{}, 1), release: make(chan struct{})}
	h.agent.LLM = blocker
	server := newChatServer(t, h, true)

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp := server.post(t, context.Background(), "chat", chatBody("session-1", "m1", "hello"))
		_, _ = io.Copy(io.Discard, resp.Body)
	}()
	<-blocker.started

	resp := server.post(t, context.Background(), "rewind", `{"sessionId":"session-1","messageId":"m1"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	<-done
	if msgs, _ := h.store.List(context.Background(), "session-1"); len(msgs) != 0 {
		t.Fatalf("a reply to a rewound message was stored: %+v", msgs)
	}
}

func TestSSESinkFramesEventsAndTerminates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sink, ok := newSSESink(w)
		if !ok {
			t.Error("a real server response supports streaming")
			return
		}
		if err := sink.Send(Chunk{"type": "start", "messageId": "a"}); err != nil {
			t.Error(err)
		}
		sink.Close()
		sink.Close() // closing twice must be safe
		if err := sink.Send(Chunk{"type": "late"}); err == nil {
			t.Error("a closed stream must reject writes")
		}
	}))
	t.Cleanup(server.Close)
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	want := "data: {\"messageId\":\"a\",\"type\":\"start\"}\n\ndata: [DONE]\n\n"
	if !bytes.Equal(body, []byte(want)) {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

type noFlushWriter struct{ header http.Header }

func (w noFlushWriter) Header() http.Header         { return w.header }
func (w noFlushWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w noFlushWriter) WriteHeader(int)             {}

func TestSSESinkRefusesWritersThatCannotStream(t *testing.T) {
	if _, ok := newSSESink(noFlushWriter{header: http.Header{}}); ok {
		t.Fatal("a writer that cannot flush must be refused before anything is written")
	}
}
