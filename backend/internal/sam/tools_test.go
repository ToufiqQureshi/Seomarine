package sam

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/mcp"
)

func testToolset(t *testing.T, runner ToolRunner) *toolset {
	t.Helper()
	skills, err := loadSkills()
	if err != nil {
		t.Fatal(err)
	}
	ts, err := newToolset(runner, scraper{guard: testGuard{allowedHost: "none.invalid"}}, skills,
		mcp.Auth{UserID: "u"}, projectInfo{ID: "project-1", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestAllowlistMatchesTheRealMCPRegistry(t *testing.T) {
	runner := mcp.NewRunner(mcp.Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ts := testToolset(t, runner)

	offered := map[string]bool{}
	for _, def := range ts.Definitions() {
		offered[def.Name] = true
		if !json.Valid(def.Parameters) {
			t.Errorf("%s has an invalid schema: %s", def.Name, def.Parameters)
		}
		if strings.Contains(string(def.Parameters), `"projectId"`) {
			t.Errorf("%s exposes projectId to the model", def.Name)
		}
	}
	for _, name := range []string{"get_domain_overview", "run_site_audit", "get_audit_status", "update_project_context", "get_product_info", "map_links", "read_pages", "activate_skill"} {
		if !offered[name] {
			t.Errorf("%s is not offered", name)
		}
	}
	// Project-bound chat must not reach these.
	for _, name := range []string{"list_projects", "create_project", "get_project_context", "save_report", "delete_report", "list_site_audits", "delete_site_audit"} {
		if offered[name] {
			t.Errorf("%s must not be offered to SAM", name)
		}
	}
}

func TestNewToolsetRejectsAnAllowlistedToolThatNoLongerExists(t *testing.T) {
	runner := newFakeRunner()
	runner.infos = runner.infos[1:]
	if _, err := newToolset(runner, scraper{}, nil, mcp.Auth{}, projectInfo{}); err == nil {
		t.Fatal("a stale allowlist entry must fail loudly, not silently drop a tool")
	}
}

func TestStripProjectID(t *testing.T) {
	tests := []struct {
		name      string
		schema    string
		wantBinds bool
		want      string
	}{
		{"required and optional", `{"type":"object","properties":{"projectId":{"type":"string"},"q":{"type":"string"}},"required":["projectId","q"]}`, true,
			`{"properties":{"q":{"type":"string"}},"required":["q"],"type":"object"}`},
		{"only required", `{"type":"object","properties":{"projectId":{"type":"string"}},"required":["projectId"]}`, true,
			`{"properties":{},"type":"object"}`},
		{"no project", `{"type":"object","properties":{"q":{"type":"string"}}}`, false,
			`{"type":"object","properties":{"q":{"type":"string"}}}`},
		{"no properties", `{"type":"object"}`, false, `{"type":"object"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, binds, err := stripProjectID(json.RawMessage(tt.schema))
			if err != nil || binds != tt.wantBinds {
				t.Fatalf("binds = %v err = %v", binds, err)
			}
			var a, b any
			_ = json.Unmarshal(got, &a)
			_ = json.Unmarshal([]byte(tt.want), &b)
			ga, _ := json.Marshal(a)
			gb, _ := json.Marshal(b)
			if string(ga) != string(gb) {
				t.Fatalf("schema = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestBindProjectRules(t *testing.T) {
	ts := testToolset(t, newFakeRunner())
	bound, err := ts.bindProject("get_domain_overview", json.RawMessage(`{"domain":"x.test"}`))
	if err != nil || !strings.Contains(string(bound), `"projectId":"project-1"`) {
		t.Fatalf("bound = %s err = %v", bound, err)
	}
	// A tool without a projectId input must never receive one from the model.
	unbound, err := ts.bindProject("whoami", json.RawMessage(`{"projectId":"other"}`))
	if err != nil || strings.Contains(string(unbound), "projectId") {
		t.Fatalf("whoami arguments = %s err = %v", unbound, err)
	}
	for _, bad := range []string{`[1]`, `"x"`, `null`, `{`} {
		if _, err := ts.bindProject("get_domain_overview", json.RawMessage(bad)); err == nil {
			t.Errorf("arguments %s must be rejected", bad)
		}
	}
	if empty, err := ts.bindProject("get_domain_overview", nil); err != nil || !strings.Contains(string(empty), "project-1") {
		t.Fatalf("empty arguments = %s err = %v", empty, err)
	}
}

func TestRunRejectsUnknownTools(t *testing.T) {
	ts := testToolset(t, newFakeRunner())
	if _, _, err := ts.Run(context.Background(), "drop_database", nil); err == nil {
		t.Fatal("unknown tool must error")
	}
}

func TestToModelOutput(t *testing.T) {
	long := strings.Repeat("x", 500)
	rows := toModelOutput(mcp.ToolResult{Text: long, Structured: map[string]any{"rows": []any{1}, "meta": map[string]any{"url": "u"}}})
	summary, _ := rows["summary"].(string)
	if !strings.HasSuffix(summary, "… (full rows in data)") || len([]rune(summary)) > summaryHeadChar+30 {
		t.Fatalf("summary with rows = %q", summary)
	}
	if rows["data"] == nil {
		t.Fatal("rows must travel in data")
	}

	metaOnly := toModelOutput(mcp.ToolResult{Text: long, Structured: map[string]any{"meta": map[string]any{"url": "u"}}})
	if metaOnly["summary"] != long {
		t.Fatal("a result whose only structure is meta keeps its full text")
	}

	textOnly := toModelOutput(mcp.ToolResult{Text: "hi"})
	if textOnly["summary"] != "hi" || textOnly["data"] != nil {
		t.Fatalf("text-only = %v", textOnly)
	}

	typed := toModelOutput(mcp.ToolResult{Text: "t", Structured: struct {
		Count int `json:"count"`
	}{Count: 2}})
	if data, _ := typed["data"].(map[string]any); data["count"] != float64(2) {
		t.Fatalf("typed structured content must be normalized: %v", typed)
	}
}

func TestToolOutputIsCapped(t *testing.T) {
	runner := newFakeRunner()
	rows := make([]any, 5_000)
	for i := range rows {
		rows[i] = map[string]any{"keyword": strings.Repeat("k", 50)}
	}
	runner.results["get_domain_overview"] = []mcp.ToolResult{{Text: "big", Structured: map[string]any{"rows": rows}}}
	ts := testToolset(t, runner)
	out, _, err := ts.Run(context.Background(), "get_domain_overview", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if size := jsonSize(out); size > maxToolOutputChars {
		t.Fatalf("model-facing output is %d chars", size)
	}
}

// auditSequence returns scripted audit statuses, one per call.
func auditSequence(texts ...string) []mcp.ToolResult {
	out := make([]mcp.ToolResult, len(texts))
	for i, text := range texts {
		status := "running"
		if strings.HasPrefix(text, "done") {
			status = "complete"
		}
		out[i] = mcp.ToolResult{Text: text, Structured: map[string]any{"status": map[string]any{"status": status}}}
	}
	return out
}

func clockedToolset(t *testing.T, runner *fakeRunner) (*toolset, *int) {
	t.Helper()
	ts := testToolset(t, runner)
	now := time.Unix(0, 0)
	sleeps := 0
	ts.now = func() time.Time { return now }
	ts.sleep = func(_ context.Context, d time.Duration) error {
		sleeps++
		now = now.Add(d)
		return nil
	}
	return ts, &sleeps
}

func TestAuditStatusWaitsUntilProgressChanges(t *testing.T) {
	runner := newFakeRunner()
	runner.results["get_audit_status"] = auditSequence("crawling 1/10", "crawling 1/10", "crawling 4/10", "crawling 9/10")
	ts, sleeps := clockedToolset(t, runner)
	out, _, err := ts.Run(context.Background(), "get_audit_status", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if *sleeps != 2 || len(runner.calls) != 3 {
		t.Fatalf("sleeps = %d calls = %d, want 2 and 3", *sleeps, len(runner.calls))
	}
	if !strings.Contains(out.(map[string]any)["summary"].(string), "4/10") {
		t.Fatalf("the changed progress line must be returned: %v", out)
	}
}

func TestAuditStatusStopsWaitingWhenFinished(t *testing.T) {
	runner := newFakeRunner()
	runner.results["get_audit_status"] = auditSequence("crawling", "done 10/10")
	ts, sleeps := clockedToolset(t, runner)
	_, _, _ = ts.Run(context.Background(), "get_audit_status", json.RawMessage(`{}`))
	if *sleeps != 1 || len(runner.calls) != 2 {
		t.Fatalf("sleeps = %d calls = %d", *sleeps, len(runner.calls))
	}

	runner = newFakeRunner()
	runner.results["get_audit_status"] = auditSequence("done already")
	ts, sleeps = clockedToolset(t, runner)
	_, _, _ = ts.Run(context.Background(), "get_audit_status", json.RawMessage(`{}`))
	if *sleeps != 0 {
		t.Fatalf("a finished audit must not be waited on, sleeps = %d", *sleeps)
	}
}

func TestAuditStatusWaitIsBounded(t *testing.T) {
	runner := newFakeRunner()
	runner.results["get_audit_status"] = auditSequence("crawling 1/10")
	ts, sleeps := clockedToolset(t, runner)
	_, _, _ = ts.Run(context.Background(), "get_audit_status", json.RawMessage(`{}`))
	want := int(auditWaitBudget / auditPollInterval)
	if *sleeps != want {
		t.Fatalf("sleeps = %d, want %d (the budget must end the wait)", *sleeps, want)
	}
}

func TestAuditStatusWaitStopsWhenTheTurnIsCanceled(t *testing.T) {
	runner := newFakeRunner()
	runner.results["get_audit_status"] = auditSequence("crawling")
	ts := testToolset(t, runner)
	ts.sleep = func(ctx context.Context, _ time.Duration) error { return context.Canceled }
	if _, _, err := ts.Run(context.Background(), "get_audit_status", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d", len(runner.calls))
	}
}

func TestAuditToolDescriptionWarnsAgainstPolling(t *testing.T) {
	ts := testToolset(t, newFakeRunner())
	for _, def := range ts.Definitions() {
		if def.Name == "get_audit_status" && !strings.Contains(def.Description, "never call it in a tight loop") {
			t.Fatalf("description = %q", def.Description)
		}
	}
}

func TestLocalTools(t *testing.T) {
	ts := testToolset(t, newFakeRunner())
	ctx := context.Background()

	out, isErr, err := ts.Run(ctx, "get_product_info", nil)
	if err != nil || isErr || !strings.Contains(out.(map[string]any)["factSheet"].(string), "Uplumo") {
		t.Fatalf("fact sheet = %v err = %v", out, err)
	}

	out, _, err = ts.Run(ctx, "map_links", json.RawMessage(`{}`))
	if err != nil || !strings.Contains(out.(map[string]any)["error"].(string), "no website") {
		t.Fatalf("map_links without a domain = %v err = %v", out, err)
	}
	out, _, _ = ts.Run(ctx, "read_pages", json.RawMessage(`{}`))
	if !strings.Contains(out.(map[string]any)["error"].(string), "no website") {
		t.Fatalf("read_pages without a site = %v", out)
	}
	if _, _, err := ts.Run(ctx, "read_pages", json.RawMessage(`{"urls":["a","b","c","d","e","f","g","h","i","j","k"]}`)); err == nil {
		t.Fatal("more than the page limit must be refused")
	}
	if _, _, err := ts.Run(ctx, "map_links", json.RawMessage(`{"surprise":1}`)); err == nil {
		t.Fatal("unknown arguments must be refused")
	}

	out, _, err = ts.Run(ctx, "activate_skill", json.RawMessage(`{"name":"keyword-research"}`))
	if err != nil || !strings.Contains(out.(map[string]any)["instructions"].(string), "Surface note") {
		t.Fatalf("activate_skill = %v err = %v", out, err)
	}
	if _, _, err := ts.Run(ctx, "activate_skill", json.RawMessage(`{"name":"../../etc/passwd"}`)); err == nil {
		t.Fatal("unknown skill must error")
	}
}

func TestMapLinksBlockedHostIsReportedNotFatal(t *testing.T) {
	ts := testToolset(t, newFakeRunner())
	out, _, err := ts.Run(context.Background(), "map_links", json.RawMessage(`{"domain":"169.254.169.254"}`))
	if err != nil || out.(map[string]any)["blocked"] != true {
		t.Fatalf("out = %v err = %v", out, err)
	}
}
