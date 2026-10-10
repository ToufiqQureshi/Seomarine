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

func TestGA4ToolsAreRegisteredWithStrictReadOnlyContracts(t *testing.T) {
	want := map[string]bool{
		"get_google_analytics_organic_landing_pages": true,
		"get_google_analytics_page_performance":      true,
		"get_google_analytics_key_events":            true,
		"get_google_analytics_traffic_acquisition":   true,
		"get_google_analytics_ecommerce_performance": true,
		"get_google_analytics_site_search":           true,
		"get_google_analytics_audience_breakdown":    true,
		"get_google_analytics_organic_overview":      true,
		"get_search_opportunities":                   true,
		"get_google_analytics_measurement_health":    true,
	}
	for _, candidate := range registry() {
		if _, ok := want[candidate.Name]; !ok {
			continue
		}
		if candidate.Annotations["readOnlyHint"] != true || candidate.Annotations["destructiveHint"] != false {
			t.Errorf("%s annotations = %#v", candidate.Name, candidate.Annotations)
		}
		var input, output map[string]any
		if err := json.Unmarshal(candidate.InputSchema, &input); err != nil {
			t.Errorf("%s input schema: %v", candidate.Name, err)
		} else if input["additionalProperties"] != false {
			t.Errorf("%s input schema accepts unknown properties", candidate.Name)
		}
		if err := json.Unmarshal(candidate.OutputSchema, &output); err != nil {
			t.Errorf("%s output schema: %v", candidate.Name, err)
		} else if output["type"] != "object" {
			t.Errorf("%s output schema is not an object", candidate.Name)
		}
		delete(want, candidate.Name)
	}
	for name := range want {
		t.Errorf("%s is not registered", name)
	}
}

func TestGA4ReportToolRejectsUnknownFields(t *testing.T) {
	_, err := handleGA4Report(context.Background(), json.RawMessage(`{"projectId":"p","notInSchema":true}`), &callEnv{}, ga4ReportToolSpec{Name: "test", Kind: "landing_pages"})
	appErr, ok := err.(*appError)
	if !ok || appErr.code != "VALIDATION_ERROR" {
		t.Fatalf("error = %#v, want VALIDATION_ERROR", err)
	}
}

func TestSearchConsolePerformanceToolIsRegisteredReadOnly(t *testing.T) {
	for _, candidate := range registry() {
		if candidate.Name != "get_search_console_performance" {
			continue
		}
		if candidate.Annotations["readOnlyHint"] != true || candidate.Annotations["destructiveHint"] != false {
			t.Fatalf("tool annotations = %#v", candidate.Annotations)
		}
		var schema map[string]any
		if err := json.Unmarshal(candidate.InputSchema, &schema); err != nil {
			t.Fatalf("decode input schema: %v", err)
		}
		if schema["additionalProperties"] != false {
			t.Fatal("input schema must reject unknown fields")
		}
		return
	}
	t.Fatal("get_search_console_performance is not registered")
}

func TestSearchConsoleToolRejectsMalformedArguments(t *testing.T) {
	_, err := handleSearchConsolePerformance(context.Background(), json.RawMessage(`{`), &callEnv{})
	appErr, ok := err.(*appError)
	if !ok || appErr.code != "VALIDATION_ERROR" {
		t.Fatalf("error = %#v, want VALIDATION_ERROR", err)
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
