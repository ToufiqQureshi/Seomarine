package mcp

import (
	"context"
	"encoding/json"
)

// ToolInfo describes one tool to an in-process caller such as the SAM agent.
type ToolInfo struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ToolResult is a tool call outcome flattened for in-process callers.
type ToolResult struct {
	Text       string
	Structured any
	IsError    bool
}

// Runner calls the same tool handlers the MCP endpoint serves, without the
// JSON-RPC transport. SAM uses it so the in-app agent and external MCP clients
// can never drift in what a tool does.
type Runner struct {
	h     *handler
	index map[string]*tool
}

// NewRunner returns a Runner over the Go-owned tool registry.
func NewRunner(d Deps) *Runner {
	h := newHandler(d)
	return &Runner{h: h, index: h.registryIndex()}
}

// Tools lists every registered tool in registration order.
func (r *Runner) Tools() []ToolInfo {
	all := registry()
	infos := make([]ToolInfo, 0, len(all))
	for _, t := range all {
		infos = append(infos, ToolInfo{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return infos
}

// Call runs one tool with the given arguments. ok is false when the name is not
// registered. Tool failures come back as an IsError result, like the endpoint.
func (r *Runner) Call(ctx context.Context, auth Auth, name string, arguments json.RawMessage) (result ToolResult, ok bool) {
	t, ok := r.index[name]
	if !ok {
		return ToolResult{}, false
	}
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	res, _ := r.h.callTool(ctx, t, arguments, auth)
	out := ToolResult{Structured: res.StructuredContent, IsError: res.IsError}
	for _, block := range res.Content {
		if block.Type != "text" {
			continue
		}
		if out.Text != "" {
			out.Text += "\n"
		}
		out.Text += block.Text
	}
	return out, true
}
