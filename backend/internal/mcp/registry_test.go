package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestListToolsMergesLegacyToolsAndDropsGoDuplicates(t *testing.T) {
	seenRequest := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		seenRequest <- r.Header.Get("Authorization") + "\n" + r.Header.Get("X-Forwarded-Host") + "\n" + string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":17,"result":{"tools":[{"name":"whoami","description":"legacy duplicate"},{"name":"legacy_tool","inputSchema":{"type":"object"}}]}}`)
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Upstream: parsed})
	requestBody := `{"jsonrpc":"2.0","id":17,"method":"tools/list"}`
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://seomarine.test/mcp", strings.NewReader(requestBody))
	r.Header.Set("Authorization", "Bearer oseo_example")
	rec := httptest.NewRecorder()
	h.serveRPC(context.Background(), rec, r, Auth{})
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	got := response.Result.Tools
	if len(got) != len(registry())+1 {
		t.Fatalf("merged tool count = %d, want %d Go tools plus one legacy tool", len(got), len(registry())+1)
	}
	if request := <-seenRequest; !strings.Contains(request, "Bearer oseo_example") || !strings.Contains(request, "seomarine.test") || !strings.Contains(request, `"method":"tools/list"`) {
		t.Errorf("legacy request did not retain auth and method: %q", request)
	}
	names := make(map[string]int, len(got))
	for _, raw := range got {
		var item struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatal(err)
		}
		names[item.Name]++
	}
	if names["whoami"] != 1 || names["legacy_tool"] != 1 {
		t.Errorf("merged tools have unexpected duplicate/missing names: %+v", names)
	}
}

func TestListToolsFailsWhenLegacyListIsUnavailable(t *testing.T) {
	parsed, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Upstream: parsed})
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://seomarine.test/mcp", strings.NewReader("{}"))
	if _, err := h.listTools(context.Background(), r, rpcRequest{ID: []byte("1"), Method: "tools/list"}); err == nil {
		t.Fatal("listTools succeeded without the legacy tool list")
	}
}

func TestDispatcherForwardsUnownedToolCallsWithCallerCredentials(t *testing.T) {
	requestBody := `{"jsonrpc":"2.0","id":42,"method":"tools/call","params":{"name":"legacy_tool","arguments":{"projectId":"project-a"}}}`
	seenRequest := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		seenRequest <- r.Header.Get("Authorization") + "\n" + r.Header.Get("X-Forwarded-Host") + "\n" + string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":42,"result":{"content":[{"type":"text","text":"legacy result"}]}}`)
	}))
	defer upstream.Close()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Upstream: parsed})
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://seomarine.test/mcp", strings.NewReader(requestBody))
	r.Header.Set("Authorization", "Bearer oseo_example")
	rec := httptest.NewRecorder()
	h.serveRPC(context.Background(), rec, r, Auth{})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "legacy result") {
		t.Fatalf("proxy response = %d %s", rec.Code, rec.Body.String())
	}
	if request := <-seenRequest; !strings.Contains(request, "Bearer oseo_example") || !strings.Contains(request, "seomarine.test") || !strings.Contains(request, `"name":"legacy_tool"`) {
		t.Errorf("legacy tool call lost caller credential or body: %q", request)
	}
}
