package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serve(t *testing.T, status int, body string, inspect func(*http.Request, []byte)) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		if inspect != nil {
			inspect(r, payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return &Client{APIKey: "test-key", BaseURL: server.URL}
}

func TestStreamAssemblesTextReasoningAndUsage(t *testing.T) {
	body := strings.Join([]string{
		": OPENROUTER PROCESSING",
		`data: {"choices":[{"delta":{"reasoning":"think "}}]}`,
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo ✓"},"finish_reason":"stop"}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"cost":0.0123}}`,
		"data: [DONE]",
		"",
	}, "\n")
	var seenAuth, seenBody string
	client := serve(t, 200, body, func(r *http.Request, payload []byte) {
		seenAuth = r.Header.Get("Authorization")
		seenBody = string(payload)
	})

	var text, reasoning []string
	result, err := client.Stream(context.Background(), Request{
		Model:           "m/x",
		Messages:        []Message{{Role: "user", Content: "hi"}},
		MaxTokens:       100,
		ReasoningEffort: "low",
	}, Handler{
		OnText:      func(d string) error { text = append(text, d); return nil },
		OnReasoning: func(d string) error { reasoning = append(reasoning, d); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Hello ✓" || strings.Join(text, "|") != "Hel|lo ✓" {
		t.Fatalf("text = %q deltas = %v", result.Text, text)
	}
	if strings.Join(reasoning, "") != "think " {
		t.Fatalf("reasoning = %v", reasoning)
	}
	if result.FinishReason != "stop" || result.Usage.CostUSD != 0.0123 || result.Usage.PromptTokens != 10 {
		t.Fatalf("result = %+v", result)
	}
	if seenAuth != "Bearer test-key" {
		t.Fatalf("auth header = %q", seenAuth)
	}
	for _, want := range []string{`"stream":true`, `"usage":{"include":true}`, `"reasoning":{"effort":"low"}`, `"max_tokens":100`} {
		if !strings.Contains(seenBody, want) {
			t.Errorf("request body missing %s: %s", want, seenBody)
		}
	}
}

func TestStreamAssemblesToolCallsAcrossChunks(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"second","arguments":"{\"q\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"first","arguments":"{}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"x\"}"}}]},"finish_reason":"tool_calls"}]}`,
		"data: [DONE]",
		"",
	}, "\n")
	client := serve(t, 200, body, nil)
	result, err := client.Stream(context.Background(), Request{Model: "m"}, Handler{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v", result.ToolCalls)
	}
	first, second := result.ToolCalls[0], result.ToolCalls[1]
	if first.ID != "a" || first.Function.Name != "first" || first.Type != "function" {
		t.Errorf("first = %+v", first)
	}
	if second.ID != "b" || second.Function.Arguments != `{"q":"x"}` {
		t.Errorf("second = %+v (arguments must join in order)", second)
	}
	if !json.Valid([]byte(second.Function.Arguments)) {
		t.Errorf("arguments are not valid JSON: %s", second.Function.Arguments)
	}
}

func TestStreamSendsToolsWithDefaultSchema(t *testing.T) {
	var seen string
	client := serve(t, 200, "data: [DONE]\n", func(_ *http.Request, payload []byte) { seen = string(payload) })
	_, err := client.Stream(context.Background(), Request{Model: "m", Tools: []Tool{{Name: "ping", Description: "d"}}}, Handler{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seen, `"name":"ping"`) || !strings.Contains(seen, `"parameters":{"type":"object","properties":{}}`) {
		t.Fatalf("request = %s", seen)
	}
}

func TestStreamReturnsStatusErrorWithBoundedBody(t *testing.T) {
	client := serve(t, 429, strings.Repeat("x", maxErrorBody*3), nil)
	_, err := client.Stream(context.Background(), Request{Model: "m"}, Handler{})
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != 429 {
		t.Fatalf("err = %v", err)
	}
	if len(status.Body) != maxErrorBody {
		t.Fatalf("body length = %d, want %d", len(status.Body), maxErrorBody)
	}
}

func TestStreamMidStreamErrorKeepsPartialResult(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"par"}}]}` + "\n" +
		`data: {"error":{"message":"upstream went away"}}` + "\n"
	client := serve(t, 200, body, nil)
	result, err := client.Stream(context.Background(), Request{Model: "m"}, Handler{})
	if !errors.Is(err, ErrStreamError) || !strings.Contains(err.Error(), "upstream went away") {
		t.Fatalf("err = %v", err)
	}
	if result.Text != "par" {
		t.Fatalf("partial text = %q", result.Text)
	}
}

func TestStreamRejectsMalformedChunk(t *testing.T) {
	client := serve(t, 200, "data: {not json}\n", nil)
	if _, err := client.Stream(context.Background(), Request{Model: "m"}, Handler{}); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestStreamStopsWhenHandlerFails(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"a"}}]}` + "\n" + `data: {"choices":[{"delta":{"content":"b"}}]}` + "\n"
	client := serve(t, 200, body, nil)
	boom := errors.New("client went away")
	calls := 0
	result, err := client.Stream(context.Background(), Request{Model: "m"}, Handler{OnText: func(string) error {
		calls++
		return boom
	}})
	if !errors.Is(err, boom) || calls != 1 {
		t.Fatalf("err = %v calls = %d", err, calls)
	}
	if result.Text != "a" {
		t.Fatalf("partial text = %q", result.Text)
	}
}

func TestStreamHonorsContextCancellation(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-release
	}))
	t.Cleanup(func() { close(release); server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	client := &Client{APIKey: "k", BaseURL: server.URL}
	if _, err := client.Stream(ctx, Request{Model: "m"}, Handler{}); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestStreamRejectsOversizedLine(t *testing.T) {
	client := serve(t, 200, "data: "+strings.Repeat("x", maxLine+10)+"\n", nil)
	if _, err := client.Stream(context.Background(), Request{Model: "m"}, Handler{}); err == nil {
		t.Fatal("expected scanner error for oversized line")
	}
}
