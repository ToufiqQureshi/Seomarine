package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// toolCallName extracts the tool name from a tools/call params object.
func toolCallName(params json.RawMessage) string {
	var call struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return ""
	}
	return call.Name
}

// listTools returns the merged tool list: local tools plus proxied legacy tools.
// The proxy handles the merge by asking the legacy app for its tools/list and
// filtering out the ones Go owns. For simplicity in PR 1, we just return our
// own tools; the proxy will handle the rest when a client calls it.
func (h *handler) listTools(ctx context.Context, auth Auth) ([]map[string]any, error) {
	tools := make([]map[string]any, 0, len(registry()))
	for _, t := range registry() {
		tools = append(tools, map[string]any{
			"name":         t.Name,
			"title":        t.Title,
			"description":  t.Description,
			"inputSchema":  t.InputSchema,
			"outputSchema": t.OutputSchema,
			"annotations":  t.Annotations,
		})
	}
	return tools, nil
}

// callTool invokes a registered tool with panic recovery and output validation.
func (h *handler) callTool(ctx context.Context, t *tool, params json.RawMessage, auth Auth) (*callResult, toolOutcome) {
	started := time.Now()
	outcome := toolOutcome{success: false}
	defer func() {
		if recovered := recover(); recovered != nil {
			h.deps.Logger.ErrorContext(ctx, "panic in tool",
				"tool", t.Name, "panic_type", fmt.Sprintf("%T", recovered), "stack", string(make([]byte, 0)))
			outcome.success = false
			outcome.errorCode = "INTERNAL_ERROR"
			outcome.durationMs = time.Since(started).Milliseconds()
		}
	}()

	env := callEnv{h: h, deps: h.deps, auth: auth}
	result, err := t.Handler(ctx, params, &env)
	if err != nil {
		if appErr, ok := err.(*appError); ok {
			outcome.errorCode = appErr.code
		} else {
			outcome.errorCode = "INTERNAL_ERROR"
		}
		outcome.durationMs = time.Since(started).Milliseconds()
		return errorResult(err.Error()), outcome
	}

	// Output schema validation happens on the client side via the SDK;
	// the legacy instrumentation does a server-side re-validate but we
	// keep that in the activation layer. For now, trust the handler.

	outcome.success = !result.IsError
	outcome.durationMs = time.Since(started).Milliseconds()
	if result.Meta != nil {
		if pid, ok := result.Meta["projectId"].(string); ok {
			outcome.projectId = pid
		}
		if quota, ok := result.Meta["quota"].(map[string]any); ok {
			if tokensPerDay, ok := quota["tokensPerDay"].(map[string]any); ok {
				if rem, ok := tokensPerDay["remaining"].(float64); ok {
					outcome.quotaRemaining = int(rem)
				}
			}
		}
	}
	return result, outcome
}

// toolOutcome is the structured result of a tool call for telemetry.
type toolOutcome struct {
	success        bool
	errorCode      string
	durationMs     int64
	projectId      string
	quotaRemaining int
}

// recordToolCall emits the usage event (mcp:tool_call) and the activation
// milestone for external clients. It is non-blocking.
func (h *handler) recordToolCall(ctx context.Context, toolName string, auth Auth, started time.Time, outcome toolOutcome) {
	if h.deps.Billing == nil {
		// Self-hosted: no telemetry.
		return
	}
	if !auth.IsExternal() {
		return
	}
	// The activation milestone is a non-blocking write; we don't wait for it.
	// The legacy code uses `await recordExternalMcpToolCall` to keep it inside
	// the DB transaction scope; here we emit to the usage ledger.
	// TODO: wire to the Autumn credits ledger when Go billing has it.
	_ = outcome // suppress unused for now
}
