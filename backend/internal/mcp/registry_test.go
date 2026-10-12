package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListToolsServesOnlyTheGoRegistry(t *testing.T) {
	h := newHandler(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://uplumo.test/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":17,"method":"tools/list"}`))
	rec := httptest.NewRecorder()
	h.serveRPC(context.Background(), rec, r, Auth{})
	if rec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tools/list response: %v", err)
	}
	if got, want := len(response.Result.Tools), len(registry()); got != want {
		t.Fatalf("tool count = %d, want exactly the %d Go tools", got, want)
	}
	seen := make(map[string]bool, len(response.Result.Tools))
	for _, tool := range response.Result.Tools {
		if seen[tool.Name] {
			t.Errorf("tool %q listed twice", tool.Name)
		}
		seen[tool.Name] = true
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

func TestDispatcherDoesNotProxyUnknownTools(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unknown tool call reached the legacy app at %s", r.URL.Path)
	}))
	defer upstream.Close()
	h := newHandler(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	body := `{"jsonrpc":"2.0","id":42,"method":"tools/call","params":{"name":"legacy_tool","arguments":{"projectId":"project-a"}}}`
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, upstream.URL+"/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.serveRPC(context.Background(), rec, r, Auth{})
	var response struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	if response.Error == nil || response.Error.Code != codeMethodNotFound {
		t.Fatalf("response = %s, want method-not-found for an unknown tool", rec.Body.String())
	}
}

func TestToolCallArgumentsUnwrapsArguments(t *testing.T) {
	tests := []struct {
		name   string
		params string
		want   string
	}{
		{"arguments object", `{"name":"x","arguments":{"projectId":"p"}}`, `{"projectId":"p"}`},
		{"missing arguments", `{"name":"x"}`, `{}`},
		{"null arguments", `{"name":"x","arguments":null}`, `{}`},
		{"malformed params", `{`, `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(toolCallArguments(json.RawMessage(tt.params))); got != tt.want {
				t.Fatalf("toolCallArguments(%s) = %s, want %s", tt.params, got, tt.want)
			}
		})
	}
}

func TestDispatcherPassesArgumentsToHandler(t *testing.T) {
	h := newHandler(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_search_console_performance","arguments":{"projectId":"p"}}}`
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.serveRPC(context.Background(), rec, r, Auth{})
	if strings.Contains(rec.Body.String(), "projectId is required") {
		t.Fatalf("handler did not receive the call arguments: %s", rec.Body.String())
	}
}
