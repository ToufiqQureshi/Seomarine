package sam

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/mcp"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/openrouter"
)

//go:embed product_fact_sheet.md
var productFactSheet string

const (
	// maxScrapePages is enough for SAM to work out what a business does, sells
	// and positions against on its own.
	maxScrapePages  = 10
	maxMappedURLs   = 60
	summaryHeadChar = 300

	// An audit runs for minutes and a chat model cannot sleep: given an instant
	// status tool it spin-polls, and every call is persisted into the history.
	// get_audit_status therefore waits server-side while the audit runs and
	// returns as soon as the progress line changes, or when the budget runs out.
	auditPollInterval = 2 * time.Second
	auditWaitBudget   = 50 * time.Second
)

// samMCPTools is the allowlist of MCP tools SAM exposes. Deliberately absent:
// list_projects and create_project (SAM is bound to one project),
// get_project_context (already a context block), and report, audit-history and
// audit-delete tools (no chat surface for them). When the MCP server gains a
// tool the in-app agent should have, add it here; a test fails if a name below
// stops existing.
var samMCPTools = []string{
	"whoami",
	"update_project_context",
	"list_saved_keywords",
	"remove_saved_keywords",
	"research_keywords",
	"save_keywords",
	"get_domain_overview",
	"get_domain_keyword_suggestions",
	"get_backlinks_overview",
	"get_backlinks_profile",
	"get_serp_results",
	"search_serp_locations",
	"create_rank_tracker",
	"get_rank_tracker",
	"add_rank_tracking_keywords",
	"remove_rank_tracking_keywords",
	"estimate_rank_tracker_cost",
	"run_rank_tracker",
	"get_ranked_keywords",
	"find_serp_competitors",
	"search_local_businesses",
	"get_local_serp_results",
	"get_google_business_questions",
	"get_business_profile",
	"get_business_reviews",
	"get_business_updates",
	"list_business_categories",
	"get_local_rank_grid",
	"get_keyword_metrics",
	"get_search_console_performance",
	"inspect_urls",
	"get_google_analytics_organic_landing_pages",
	"get_google_analytics_page_performance",
	"get_google_analytics_key_events",
	"get_search_opportunities",
	"get_google_analytics_organic_overview",
	"get_google_analytics_traffic_acquisition",
	"get_google_analytics_measurement_health",
	"get_google_analytics_ecommerce_performance",
	"get_google_analytics_site_search",
	"get_google_analytics_audience_breakdown",
	"run_site_audit",
	"get_audit_status",
	"get_audit_issues",
	"get_audit_pages",
}

// ToolRunner runs the MCP tool handlers in process. *mcp.Runner implements it.
type ToolRunner interface {
	Tools() []mcp.ToolInfo
	Call(ctx context.Context, auth mcp.Auth, name string, arguments json.RawMessage) (mcp.ToolResult, bool)
}

type localTool struct {
	def openrouter.Tool
	run func(ctx context.Context, args json.RawMessage) (any, error)
}

// toolset is one turn's tool surface: allowlisted MCP tools bound to the
// session's project, plus the local tools.
type toolset struct {
	runner  ToolRunner
	auth    mcp.Auth
	project projectInfo
	// binds records which MCP tools had projectId stripped from their schema.
	binds map[string]bool
	defs  []openrouter.Tool
	local map[string]localTool
	// sleep and now drive the audit wait; tests replace them.
	sleep func(context.Context, time.Duration) error
	now   func() time.Time
}

// newToolset builds the tool surface for one turn.
func newToolset(runner ToolRunner, sc scraper, skills []skill, auth mcp.Auth, project projectInfo) (*toolset, error) {
	ts := &toolset{
		runner:  runner,
		auth:    auth,
		project: project,
		binds:   map[string]bool{},
		local:   map[string]localTool{},
		sleep:   sleepContext,
		now:     time.Now,
	}

	available := map[string]mcp.ToolInfo{}
	for _, info := range runner.Tools() {
		available[info.Name] = info
	}
	for _, name := range samMCPTools {
		info, ok := available[name]
		if !ok {
			return nil, fmt.Errorf("SAM allowlists unknown MCP tool %q", name)
		}
		schema, binds, err := stripProjectID(info.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("prepare schema of %s: %w", name, err)
		}
		description := info.Description
		if name == "get_audit_status" {
			description += " While the audit is running this call waits up to ~1 minute server-side and returns as soon as progress changes, so never call it in a tight loop: check a few times, narrating progress to the user in between, and if it is still running after that, say so and let the user come back for the results."
		}
		ts.binds[name] = binds
		ts.defs = append(ts.defs, openrouter.Tool{Name: name, Description: description, Parameters: schema})
	}

	for _, lt := range ts.localTools(sc, skills) {
		ts.local[lt.def.Name] = lt
		ts.defs = append(ts.defs, lt.def)
	}
	return ts, nil
}

// Definitions returns the tools in a stable order for the provider.
func (ts *toolset) Definitions() []openrouter.Tool { return ts.defs }

// Run executes one tool call. The returned value is what the model sees;
// isError marks results the UI should show as failed. A Go error means the
// call itself could not run (unknown tool, bad arguments).
func (ts *toolset) Run(ctx context.Context, name string, arguments json.RawMessage) (output any, isError bool, err error) {
	if local, ok := ts.local[name]; ok {
		out, err := local.run(ctx, arguments)
		if err != nil {
			return nil, false, err
		}
		return capToolOutput(out, maxToolOutputChars), false, nil
	}
	if _, ok := ts.binds[name]; !ok {
		return nil, false, fmt.Errorf("unknown tool %q", name)
	}
	bound, err := ts.bindProject(name, arguments)
	if err != nil {
		return nil, false, err
	}
	result, ok := ts.runner.Call(ctx, ts.auth, name, bound)
	if !ok {
		return nil, false, fmt.Errorf("unknown tool %q", name)
	}
	if name == "get_audit_status" {
		result = ts.waitForAuditProgress(ctx, bound, result)
	}
	if result.IsError {
		return nil, true, errors.New(result.Text)
	}
	return capToolOutput(toModelOutput(result), maxToolOutputChars), false, nil
}

// bindProject injects the session's project id, so the model never has to know
// or pass it, cannot target another project, and cannot hallucinate a wrong one.
func (ts *toolset) bindProject(name string, arguments json.RawMessage) (json.RawMessage, error) {
	args := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(arguments))) > 0 {
		if err := json.Unmarshal(arguments, &args); err != nil || args == nil {
			return nil, errors.New("tool arguments must be a JSON object")
		}
	}
	if ts.binds[name] {
		id, err := json.Marshal(ts.project.ID)
		if err != nil {
			return nil, fmt.Errorf("encode project id: %w", err)
		}
		args["projectId"] = id
	} else {
		// A tool without a projectId input must not receive one.
		delete(args, "projectId")
	}
	bound, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encode tool arguments: %w", err)
	}
	return bound, nil
}

// stripProjectID removes projectId from a tool's input schema and reports
// whether it was there.
func stripProjectID(schema json.RawMessage) (json.RawMessage, bool, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(schema, &doc); err != nil {
		return nil, false, err
	}
	var properties map[string]json.RawMessage
	if raw, ok := doc["properties"]; ok {
		if err := json.Unmarshal(raw, &properties); err != nil {
			return nil, false, err
		}
	}
	if _, has := properties["projectId"]; !has {
		return schema, false, nil
	}
	delete(properties, "projectId")
	rewritten, err := json.Marshal(properties)
	if err != nil {
		return nil, false, err
	}
	doc["properties"] = rewritten
	if raw, ok := doc["required"]; ok {
		var required []string
		if err := json.Unmarshal(raw, &required); err != nil {
			return nil, false, err
		}
		required = slices.DeleteFunc(required, func(s string) bool { return s == "projectId" })
		if len(required) == 0 {
			delete(doc, "required")
		} else if doc["required"], err = json.Marshal(required); err != nil {
			return nil, false, err
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// toModelOutput flattens a tool result for the model. The text block renders
// the same rows the structured data carries, so when both exist only the head
// of the text survives. A result is re-sent on every step of its turn and
// persisted, so the duplicate would cost twice everywhere.
func toModelOutput(result mcp.ToolResult) map[string]any {
	structured := normalizeStructured(result.Structured)
	hasRows := false
	for key := range structured {
		if key != "meta" {
			hasRows = true
			break
		}
	}
	if !hasRows {
		out := map[string]any{"summary": result.Text}
		if structured != nil {
			out["data"] = structured
		}
		return out
	}
	summary := result.Text
	if runes := []rune(summary); len(runes) > summaryHeadChar {
		summary = string(runes[:summaryHeadChar]) + "… (full rows in data)"
	}
	return map[string]any{"summary": summary, "data": structured}
}

func normalizeStructured(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(encoded, &m) != nil {
		return nil
	}
	return m
}

// waitForAuditProgress re-reads the audit status every few seconds while it is
// running and its progress line has not changed.
func (ts *toolset) waitForAuditProgress(ctx context.Context, arguments json.RawMessage, result mcp.ToolResult) mcp.ToolResult {
	initial := result.Text
	deadline := ts.now().Add(auditWaitBudget)
	for auditRunning(result) && result.Text == initial && ts.now().Before(deadline) {
		if err := ts.sleep(ctx, auditPollInterval); err != nil {
			return result
		}
		next, ok := ts.runner.Call(ctx, ts.auth, "get_audit_status", arguments)
		if !ok {
			return result
		}
		result = next
	}
	return result
}

func auditRunning(result mcp.ToolResult) bool {
	if result.IsError {
		return false
	}
	data := normalizeStructured(result.Structured)
	status, _ := data["status"].(map[string]any)
	return status["status"] == "running"
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (ts *toolset) localTools(sc scraper, skills []skill) []localTool {
	objectSchema := func(properties string) json.RawMessage {
		return json.RawMessage(`{"type":"object","properties":{` + properties + `},"additionalProperties":false}`)
	}
	tools := []localTool{
		{
			def: openrouter.Tool{
				Name:        "get_product_info",
				Description: "The Seomarine fact sheet: what the product does, plans/pricing, credit costs, integrations, MCP setup. Call before answering questions about Seomarine itself. Uses no credits.",
				Parameters:  objectSchema(""),
			},
			run: func(context.Context, json.RawMessage) (any, error) {
				return map[string]any{"factSheet": productFactSheet}, nil
			},
		},
		{
			def: openrouter.Tool{
				Name:        "map_links",
				Description: "List a site's page URLs (homepage plus its sitemap) so you can choose which pages to read with read_pages. Defaults to the project's own site; pass `domain` to map another site (e.g. a competitor). Uses no credits.",
				Parameters:  objectSchema(`"domain":{"type":"string","description":"Domain or URL to map. Omit for the project's own site."}`),
			},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var args struct {
					Domain string `json:"domain"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				target := args.Domain
				if target == "" {
					target = ts.project.Domain
				}
				if target == "" {
					return map[string]any{"error": "This project has no website set — ask the user for their site first."}, nil
				}
				urls, blocked := sc.mapLinks(ctx, target, maxMappedURLs)
				if blocked {
					return map[string]any{"blocked": true, "urls": []string{}, "note": "Could not reach the site."}, nil
				}
				return map[string]any{"blocked": false, "urls": urls}, nil
			},
		},
		{
			def: openrouter.Tool{
				Name:        "read_pages",
				Description: fmt.Sprintf("Read up to %d web pages as plain text — the project's own pages or anyone else's (competitors, references). Pass specific `urls` (usually picked from map_links); omit to read a representative sample of the project's own site. Uses no credits.", maxScrapePages),
				Parameters:  objectSchema(fmt.Sprintf(`"urls":{"type":"array","maxItems":%d,"items":{"type":"string","format":"uri"},"description":"Specific page URLs to read (max %d). Omit to read the project's own site."}`, maxScrapePages, maxScrapePages)),
			},
			run: func(ctx context.Context, raw json.RawMessage) (any, error) {
				var args struct {
					URLs []string `json:"urls"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				var (
					pages   []scrapedPage
					blocked bool
				)
				switch {
				case len(args.URLs) > maxScrapePages:
					return nil, fmt.Errorf("pass at most %d urls", maxScrapePages)
				case len(args.URLs) > 0:
					pages, blocked = sc.readPages(ctx, args.URLs, maxScrapePages)
				case ts.project.Domain != "":
					pages, blocked = sc.readSite(ctx, ts.project.Domain, maxScrapePages)
				default:
					return map[string]any{"error": "This project has no website set — ask the user for their site, or pass explicit urls."}, nil
				}
				if blocked {
					return map[string]any{"blocked": true, "pages": []scrapedPage{}, "note": "Could not read the requested page(s). Ask the user to describe the site instead, and say you couldn't read it."}, nil
				}
				return map[string]any{"blocked": false, "pages": pages}, nil
			},
		},
	}
	if len(skills) > 0 {
		tools = append(tools, localTool{
			def: openrouter.Tool{
				Name:        "activate_skill",
				Description: "Load the full step-by-step instructions of one skill from the skills list in your system prompt. Uses no credits.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`),
			},
			run: func(_ context.Context, raw json.RawMessage) (any, error) {
				var args struct {
					Name string `json:"name"`
				}
				if err := decodeArgs(raw, &args); err != nil {
					return nil, err
				}
				for _, s := range skills {
					if s.Name == args.Name {
						return map[string]any{"name": s.Name, "instructions": s.Body}, nil
					}
				}
				return nil, fmt.Errorf("no skill named %q", args.Name)
			},
		})
	}
	return tools
}

// decodeArgs parses a tool's JSON arguments, treating empty input as {}.
func decodeArgs(raw json.RawMessage, dst any) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}
