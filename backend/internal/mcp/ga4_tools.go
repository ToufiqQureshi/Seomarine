package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/ga4"
)

const ga4ReportOutput = `{"type":"object","properties":{"status":{"type":"string","enum":["ok","error"]},"source":{"type":"object","additionalProperties":true},"request":{"type":"object","additionalProperties":true},"rowCount":{"type":"integer"},"totalRowCount":{"type":"integer"},"rows":{"type":"array","items":{"type":"object","additionalProperties":true}},"pageInfo":{"type":"object","additionalProperties":true},"reportMetadata":{"type":"object","additionalProperties":true},"warnings":{"type":"array","items":{"type":"string"}},"quota":{"type":["object","null"],"additionalProperties":true},"comparison":{"type":"object","additionalProperties":true},"diagnostics":{"type":"array","items":{"type":"object","additionalProperties":true}},"error":{"type":"object","additionalProperties":true}},"required":["status"],"additionalProperties":true}`
const ga4OverviewOutput = `{"type":"object","properties":{"status":{"type":"string","enum":["ok","error"]},"source":{"type":"object","additionalProperties":true},"request":{"type":"object","additionalProperties":true},"current":{"type":["object","null"],"additionalProperties":true},"previous":{"type":["object","null"],"additionalProperties":true},"comparison":{"type":"object","additionalProperties":true},"trend":{"type":"array","items":{"type":"object","additionalProperties":true}},"diagnostics":{"type":"array","items":{"type":"object","additionalProperties":true}},"reportMetadata":{"type":"object","additionalProperties":true},"warnings":{"type":"array","items":{"type":"string"}},"quota":{"type":["object","null"],"additionalProperties":true},"error":{"type":"object","additionalProperties":true}},"required":["status"],"additionalProperties":true}`
const ga4HealthOutput = `{"type":"object","properties":{"status":{"type":"string","enum":["ok","error"]},"source":{"type":"object","additionalProperties":true},"summary":{"type":"object","additionalProperties":true},"issues":{"type":"array","items":{"type":"string"}},"webStreams":{"type":"array","items":{"type":"object","additionalProperties":true}},"otherStreams":{"type":"array","items":{"type":"object","additionalProperties":true}},"keyEvents":{"type":"array","items":{"type":"object","additionalProperties":true}},"customDefinitions":{"type":"object","additionalProperties":true},"error":{"type":"object","additionalProperties":true}},"required":["status"],"additionalProperties":true}`
const ga4OpportunityOutput = `{"type":"object","properties":{"status":{"type":"string","enum":["ok","error"]},"source":{"type":"object","additionalProperties":true},"request":{"type":"object","additionalProperties":true},"rowCount":{"type":"integer"},"totalCandidateRows":{"type":"integer"},"rows":{"type":"array","items":{"type":"object","additionalProperties":true}},"scoring":{"type":"object","additionalProperties":true},"coverage":{"type":"object","additionalProperties":true},"truncated":{"type":"object","additionalProperties":true},"warnings":{"type":"array","items":{"type":"string"}},"reportMetadata":{"type":"object","additionalProperties":true},"quota":{"type":["object","null"],"additionalProperties":true},"error":{"type":"object","additionalProperties":true}},"required":["status"],"additionalProperties":true}`

func ga4OverviewInputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"startDate":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},"endDate":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},"trend":{"type":"string","enum":["daily","weekly"],"default":"daily"}},"required":["projectId"],"additionalProperties":false}`)
}

func ga4OpportunityInputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"startDate":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},"endDate":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},"limit":{"type":"integer","minimum":1,"maximum":100,"default":50}},"required":["projectId"],"additionalProperties":false}`)
}

func ga4HealthInputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1}},"required":["projectId"],"additionalProperties":false}`)
}

type ga4ReportToolSpec struct {
	Name, Title, Description string
	Kind                     ga4.ReportKind
	Schema                   json.RawMessage
	Channel                  string
	BreakdownField           string
	DefaultBreakdown         string
	IncludeDate              bool
	Compare                  bool
	OnlyTransactions         bool
}

func ga4Tools() []*tool {
	common := `"projectId":{"type":"string","minLength":1},"startDate":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},"endDate":{"type":"string","pattern":"^\\d{4}-\\d{2}-\\d{2}$"},"limit":{"type":"integer","minimum":1,"maximum":1000,"default":100},"offset":{"type":"integer","minimum":0,"default":0}`
	commonObject := func(extra string) json.RawMessage {
		return json.RawMessage(`{"type":"object","properties":{` + common + extra + `},"required":["projectId"],"additionalProperties":false}`)
	}
	base := []ga4ReportToolSpec{
		{Name: "get_google_analytics_organic_landing_pages", Title: "Get Google Analytics organic landing pages", Description: "Read organic-search landing page sessions, engagement, key events, transactions, and revenue from the connected GA4 property. Read-only; uses no Seomarine credits.", Kind: ga4.LandingPages, Schema: commonObject(""), Channel: "organic_search"},
		{Name: "get_google_analytics_page_performance", Title: "Get Google Analytics page performance", Description: "Read page views, users, engagement duration, and key events from the connected property. Organic Search is the default; channel all includes every channel. Read-only; uses no credits.", Kind: ga4.PagePerformance, Schema: commonObject(`,"includeDate":{"type":"boolean","default":false},"channel":{"type":"string","enum":["organic_search","all"],"default":"organic_search"}`), Channel: "organic_search", IncludeDate: true},
		{Name: "get_google_analytics_key_events", Title: "Get Google Analytics key events", Description: "Read active GA4 key events by event or organic landing page, optionally compared with the previous period. Read-only; uses no credits.", Kind: ga4.KeyEvents, Schema: commonObject(`,"breakdown":{"type":"string","enum":["event","event_and_landing_page"],"default":"event"},"channel":{"type":"string","enum":["organic_search","all"],"default":"organic_search"},"comparePreviousPeriod":{"type":"boolean","default":false}`), Channel: "organic_search", BreakdownField: "breakdown", DefaultBreakdown: "event", Compare: true},
		{Name: "get_google_analytics_traffic_acquisition", Title: "Get Google Analytics traffic acquisition", Description: "Compare session acquisition by channel group, source/medium, or campaign. Read-only; uses no credits.", Kind: ga4.TrafficAcquisition, Schema: commonObject(`,"breakdown":{"type":"string","enum":["channel_group","source_medium","campaign"],"default":"channel_group"},"comparePreviousPeriod":{"type":"boolean","default":false}`), Channel: "all", BreakdownField: "acquisitionBreakdown", DefaultBreakdown: "channel_group", Compare: true},
		{Name: "get_google_analytics_ecommerce_performance", Title: "Get Google Analytics ecommerce performance", Description: "Read item or landing-page ecommerce outcomes; Organic Search is the default. Read-only; uses no credits.", Kind: ga4.EcommercePerformance, Schema: commonObject(`,"breakdown":{"type":"string","enum":["item","landing_page"],"default":"item"},"onlyWithTransactions":{"type":"boolean","default":false},"channel":{"type":"string","enum":["organic_search","all"],"default":"organic_search"}`), Channel: "organic_search", BreakdownField: "ecommerceBreakdown", DefaultBreakdown: "item", OnlyTransactions: true},
		{Name: "get_google_analytics_site_search", Title: "Get Google Analytics site search", Description: "Read measured internal search terms and related engagement. Requires GA4 site-search measurement. Read-only; uses no credits.", Kind: ga4.SiteSearch, Schema: commonObject(""), Channel: "all"},
		{Name: "get_google_analytics_audience_breakdown", Title: "Get Google Analytics audience breakdown", Description: "Read device, country, or new-versus-returning audience outcomes. Read-only; uses no credits.", Kind: ga4.AudienceBreakdown, Schema: commonObject(`,"breakdown":{"type":"string","enum":["device","country","new_vs_returning"],"default":"device"},"channel":{"type":"string","enum":["organic_search","all"],"default":"organic_search"},"comparePreviousPeriod":{"type":"boolean","default":false}`), Channel: "organic_search", BreakdownField: "audienceBreakdown", DefaultBreakdown: "device", Compare: true},
	}
	tools := make([]*tool, 0, len(base)+3)
	for _, spec := range base {
		copy := spec
		tools = append(tools, &tool{Name: spec.Name, Title: spec.Title, Description: spec.Description, InputSchema: spec.Schema, OutputSchema: json.RawMessage(ga4ReportOutput), Annotations: readOnlyAnnotations(), Handler: func(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
			return handleGA4Report(ctx, raw, env, copy)
		}})
	}
	tools = append(tools, organicOverviewTool(ga4OverviewInputSchema()), searchOpportunityTool(ga4OpportunityInputSchema()), measurementHealthTool(ga4HealthInputSchema()))
	return tools
}

type ga4ToolArgs struct {
	ProjectID                     string `json:"projectId"`
	StartDate                     string `json:"startDate"`
	EndDate                       string `json:"endDate"`
	Limit                         *int   `json:"limit"`
	Offset                        int    `json:"offset"`
	Channel                       string `json:"channel"`
	IncludeDate                   bool   `json:"includeDate"`
	Breakdown                     string `json:"breakdown"`
	AcquisitionBreakdown          string `json:"acquisitionBreakdown"`
	EcommerceBreakdown            string `json:"ecommerceBreakdown"`
	EcommerceOnlyWithTransactions bool   `json:"onlyWithTransactions"`
	AudienceBreakdown             string `json:"audienceBreakdown"`
	ComparePreviousPeriod         bool   `json:"comparePreviousPeriod"`
	Trend                         string `json:"trend"`
}

func handleGA4Report(ctx context.Context, raw json.RawMessage, env *callEnv, spec ga4ReportToolSpec) (*callResult, error) {
	var args ga4ToolArgs
	if err := decodeSchemaToolArgs(raw, &args, spec.Schema); err != nil || strings.TrimSpace(args.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "The GA4 report arguments are invalid.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.GA4 == nil {
		return ga4ErrorResult(env, access, "ga4_unavailable", "Google Analytics reporting is not set up on this server.", nil), nil
	}
	channel := args.Channel
	if channel == "" {
		channel = spec.Channel
	}
	input := ga4.ReportInput{Kind: spec.Kind, StartDate: args.StartDate, EndDate: args.EndDate, Limit: args.Limit, Offset: args.Offset, Channel: channel, IncludeDate: args.IncludeDate, ComparePreviousPeriod: spec.Compare && args.ComparePreviousPeriod, EcommerceOnlyWithTransactions: spec.OnlyTransactions && args.EcommerceOnlyWithTransactions}
	switch spec.BreakdownField {
	case "breakdown":
		input.Breakdown = args.Breakdown
		if input.Breakdown == "" {
			input.Breakdown = spec.DefaultBreakdown
		}
	case "acquisitionBreakdown":
		input.AcquisitionBreakdown = args.Breakdown
		if input.AcquisitionBreakdown == "" {
			input.AcquisitionBreakdown = spec.DefaultBreakdown
		}
	case "ecommerceBreakdown":
		input.EcommerceBreakdown = args.Breakdown
		if input.EcommerceBreakdown == "" {
			input.EcommerceBreakdown = spec.DefaultBreakdown
		}
	case "audienceBreakdown":
		input.AudienceBreakdown = args.Breakdown
		if input.AudienceBreakdown == "" {
			input.AudienceBreakdown = spec.DefaultBreakdown
		}
	}
	result, err := env.deps.GA4.RunReport(ctx, access.Project.ID, input)
	if err != nil {
		return ga4ErrorFromError(ctx, env, access, err), nil
	}
	if rows, ok := result["rowCount"].(int); ok {
		return mcpResponse(fmt.Sprintf("%s: %d rows returned.", spec.Title, rows), result, ga4Meta(env, access)), nil
	}
	return mcpResponse(spec.Title+" complete.", result, ga4Meta(env, access)), nil
}

func decodeSchemaToolArgs(raw json.RawMessage, dst any, schema json.RawMessage) error {
	var envelope struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &envelope); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for key := range fields {
		if _, ok := envelope.Properties[key]; !ok {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func ga4Meta(env *callEnv, access projectAccess) metaFields {
	return metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/reports", nil)}
}

func ga4ErrorFromError(ctx context.Context, env *callEnv, access projectAccess, err error) *callResult {
	if code, message, retry, ok := ga4.PublicError(err); ok {
		return ga4ErrorResult(env, access, code, message, retry)
	}
	env.deps.Logger.ErrorContext(ctx, "MCP Google Analytics report", "project_id", access.Project.ID, "err", err)
	return ga4ErrorResult(env, access, "internal", "Google Analytics reporting failed. Please try again.", nil)
}

func ga4ErrorResult(env *callEnv, access projectAccess, code, message string, retry *int) *callResult {
	var action string
	switch code {
	case "ga4_not_connected", "ga4_reconnect_required", "ga4_property_inaccessible":
		action = buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/settings/integrations", nil)
	}
	if strings.HasPrefix(code, "gsc_") {
		action = buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/search-performance", nil)
	}
	errorShape := map[string]any{"code": code, "message": message}
	if retry != nil {
		errorShape["retryAfterSeconds"] = *retry
	}
	if action != "" {
		errorShape["actionUrl"] = action
	}
	text := message
	if action != "" {
		text += " Continue here: " + action
	}
	return mcpResponse(text, map[string]any{"status": "error", "error": errorShape}, ga4Meta(env, access))
}

func organicOverviewTool(schema json.RawMessage) *tool {
	return &tool{Name: "get_google_analytics_organic_overview", Title: "Get Google Analytics organic overview", Description: "Compare organic sessions, users, engagement, key events, transactions, revenue, and daily or weekly trends. Read-only; uses no credits.", InputSchema: schema, OutputSchema: json.RawMessage(ga4OverviewOutput), Annotations: readOnlyAnnotations(), Handler: handleGA4Overview}
}

func handleGA4Overview(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args ga4ToolArgs
	if err := decodeSchemaToolArgs(raw, &args, ga4OverviewInputSchema()); err != nil || args.ProjectID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "The GA4 overview arguments are invalid.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.GA4 == nil {
		return ga4ErrorResult(env, access, "ga4_unavailable", "Google Analytics reporting is not set up on this server.", nil), nil
	}
	result, err := env.deps.GA4.GetOrganicOverview(ctx, access.Project.ID, ga4.OrganicOverviewInput{StartDate: args.StartDate, EndDate: args.EndDate, Trend: args.Trend})
	if err != nil {
		return ga4ErrorFromError(ctx, env, access, err), nil
	}
	return mcpResponse("Organic Analytics overview complete.", result, ga4Meta(env, access)), nil
}

func searchOpportunityTool(schema json.RawMessage) *tool {
	return &tool{Name: "get_search_opportunities", Title: "Get search opportunities", Description: "Join Search Console pages ranking 4–20 with GA4 organic landing-page outcomes and score matched opportunities. Read-only; uses no credits.", InputSchema: schema, OutputSchema: json.RawMessage(ga4OpportunityOutput), Annotations: readOnlyAnnotations(), Handler: handleSearchOpportunities}
}

func handleSearchOpportunities(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args ga4ToolArgs
	if err := decodeSchemaToolArgs(raw, &args, ga4OpportunityInputSchema()); err != nil || args.ProjectID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "The search opportunity arguments are invalid.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.GA4 == nil {
		return ga4ErrorResult(env, access, "ga4_unavailable", "Google Analytics reporting is not set up on this server.", nil), nil
	}
	limit := args.Limit
	if limit == nil {
		defaultLimit := 50
		limit = &defaultLimit
	}
	result, err := env.deps.GA4.GetSearchOpportunities(ctx, env.deps.GSC, access.Project.OrganizationID, access.Project.ID, ga4.SearchOpportunityInput{StartDate: args.StartDate, EndDate: args.EndDate, Limit: limit})
	if err != nil {
		return ga4ErrorFromError(ctx, env, access, err), nil
	}
	return mcpResponse("Search opportunities complete.", result, ga4Meta(env, access)), nil
}

func measurementHealthTool(schema json.RawMessage) *tool {
	return &tool{Name: "get_google_analytics_measurement_health", Title: "Get Google Analytics measurement health", Description: "Inspect GA4 streams, web measurement IDs, enhanced measurement, key events, and custom definitions. Read-only; uses no credits.", InputSchema: schema, OutputSchema: json.RawMessage(ga4HealthOutput), Annotations: readOnlyAnnotations(), Handler: handleMeasurementHealth}
}

func handleMeasurementHealth(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args ga4ToolArgs
	if err := decodeSchemaToolArgs(raw, &args, ga4HealthInputSchema()); err != nil || args.ProjectID == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.GA4 == nil {
		return ga4ErrorResult(env, access, "ga4_unavailable", "Google Analytics reporting is not set up on this server.", nil), nil
	}
	result, err := env.deps.GA4.GetMeasurementHealth(ctx, access.Project.ID)
	if err != nil {
		return ga4ErrorFromError(ctx, env, access, err), nil
	}
	return mcpResponse("Google Analytics measurement health complete.", result, ga4Meta(env, access)), nil
}
