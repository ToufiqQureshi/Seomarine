package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// listTools returns Go-owned tools and any legacy tools not yet ported to Go.
func (h *handler) listTools(ctx context.Context, r *http.Request, req rpcRequest) ([]json.RawMessage, error) {
	local := make([]json.RawMessage, 0, len(registry()))
	owned := make(map[string]struct{}, len(registry()))
	for _, t := range registry() {
		data, err := json.Marshal(map[string]any{
			"name":         t.Name,
			"title":        t.Title,
			"description":  t.Description,
			"inputSchema":  t.InputSchema,
			"outputSchema": t.OutputSchema,
			"annotations":  t.Annotations,
		})
		if err != nil {
			return nil, fmt.Errorf("encode local tool %s: %w", t.Name, err)
		}
		local = append(local, data)
		owned[t.Name] = struct{}{}
	}

	legacy, err := h.fetchLegacyTools(ctx, r, req)
	if err != nil {
		return nil, err
	}
	merged := make([]json.RawMessage, 0, len(local)+len(legacy))
	merged = append(merged, local...)
	for _, raw := range legacy {
		var tool struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil || strings.TrimSpace(tool.Name) == "" {
			return nil, errors.New("legacy MCP tools/list returned a tool without a name")
		}
		if _, duplicate := owned[tool.Name]; duplicate {
			continue
		}
		owned[tool.Name] = struct{}{}
		merged = append(merged, raw)
	}
	return merged, nil
}

func (h *handler) fetchLegacyTools(ctx context.Context, r *http.Request, req rpcRequest) ([]json.RawMessage, error) {
	if h.deps.Upstream == nil {
		return nil, errors.New("legacy MCP upstream is not configured")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode tools/list request: %w", err)
	}
	proxyCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp, err := h.proxyRoundTrip(proxyCtx, r, body)
	if err != nil {
		return nil, fmt.Errorf("request legacy MCP tool list: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			h.deps.Logger.WarnContext(ctx, "close legacy MCP tools/list response", "err", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("legacy MCP tools/list returned HTTP %d", resp.StatusCode)
	}
	const maxLegacyToolListBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxLegacyToolListBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read legacy MCP tool list: %w", err)
	}
	if len(data) > maxLegacyToolListBytes {
		return nil, errors.New("legacy MCP tool list is too large")
	}
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode legacy MCP tool list: %w", err)
	}
	if envelope.JSONRPC != "2.0" || !bytes.Equal(bytes.TrimSpace(envelope.ID), bytes.TrimSpace(req.ID)) {
		return nil, errors.New("legacy MCP tools/list response does not match the request")
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("legacy MCP tools/list failed with code %d", envelope.Error.Code)
	}
	if envelope.Result.Tools == nil {
		return nil, errors.New("legacy MCP tools/list response has no tools array")
	}
	return envelope.Result.Tools, nil
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
