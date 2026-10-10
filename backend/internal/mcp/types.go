package mcp

import (
	"context"
	"encoding/json"
)

// Auth is the caller identity behind one MCP request. It mirrors the legacy
// seomarineAuth props object.
type Auth struct {
	UserID         string
	UserEmail      string
	OrganizationID string
	// Role is the caller's role in OrganizationID.
	Role string
	// OrgScope is "pinned" (one bound org) or "user" (the hosted credentials
	// the API-key lane stamps, where project membership is the boundary).
	OrgScope string
	// BaseURL is the public origin dashboard links are built from.
	BaseURL string
	// ClientID is "api_key" on the API-key lane; empty is first-party.
	ClientID string
	// ClientLabel is a display-only hint ("API key", "Claude Code", ...).
	ClientLabel string
	Scopes      []string
}

// IsExternal reports whether the call came from an external MCP client. It
// drives the dashboard activation milestone.
func (a Auth) IsExternal() bool { return a.ClientID != "" }

// callEnv is the per-call environment a tool handler needs.
type callEnv struct {
	h    *handler
	deps Deps
	auth Auth
}

// contentBlock is one MCP content block. Only text is produced today.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// callResult is an MCP tools/call result. Its JSON shape is identical to the
// legacy CallToolResult.
type callResult struct {
	Content           []contentBlock `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
}

// tool is one registered MCP tool. InputSchema and OutputSchema are JSON Schema
// documents carried to the client verbatim.
type tool struct {
	Name         string
	Title        string
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	Annotations  map[string]any
	Handler      func(ctx context.Context, args json.RawMessage, env *callEnv) (*callResult, error)
}

// registry returns every tool the Go server owns, in the legacy registration
// order. A tool not in this list is proxied to the legacy app.
func registry() []*tool {
	return []*tool{
		whoamiTool(),
		listProjectsTool(),
		createProjectTool(),
		getProjectContextTool(),
	}
}

// registryIndex maps a tool name to its handler for the dispatcher.
func (h *handler) registryIndex() map[string]*tool {
	index := make(map[string]*tool)
	for _, t := range registry() {
		index[t.Name] = t
	}
	return index
}

// textResult builds a plain text tool result.
func textResult(text string) *callResult {
	return &callResult{Content: []contentBlock{{Type: "text", Text: text}}}
}

// errorResult builds a tool-error result the way the legacy SDK does: an
// in-band result with isError set, not a JSON-RPC error.
func errorResult(message string) *callResult {
	result := textResult(message)
	result.IsError = true
	return result
}

// metaFields are the optional response-meta fields carried on every tool. They
// are emitted only when set, matching the legacy mcpResponse helper.
type metaFields struct {
	URL              string
	ProjectID        string
	RunID            string
	CreditsCharged   *int
	CreditsRemaining *int
}

// mcpResponse mirrors the legacy mcpResponse: text plus structuredContent, with
// the non-empty meta fields merged into structuredContent.meta and set as
// _meta. structured is any JSON-marshalable value (a map or a typed struct).
func mcpResponse(text string, structured any, meta metaFields) *callResult {
	metaMap := map[string]any{}
	if meta.URL != "" {
		metaMap["url"] = meta.URL
	}
	if meta.ProjectID != "" {
		metaMap["projectId"] = meta.ProjectID
	}
	if meta.RunID != "" {
		metaMap["runId"] = meta.RunID
	}
	if meta.CreditsCharged != nil {
		metaMap["creditsCharged"] = *meta.CreditsCharged
	}
	if meta.CreditsRemaining != nil {
		metaMap["creditsRemaining"] = *meta.CreditsRemaining
	}

	result := textResult(text)
	if structured != nil {
		if asMap, ok := toMap(structured); ok {
			if len(metaMap) > 0 {
				asMap["meta"] = metaMap
			}
			result.StructuredContent = asMap
		} else {
			result.StructuredContent = structured
		}
	} else if len(metaMap) > 0 {
		result.StructuredContent = map[string]any{"meta": metaMap}
	}
	if len(metaMap) > 0 {
		result.Meta = metaMap
	}
	return result
}

// toMap normalizes a structured value to a map so meta can be merged into it.
func toMap(value any) (map[string]any, bool) {
	if m, ok := value.(map[string]any); ok {
		return m, true
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false
	}
	return m, true
}
