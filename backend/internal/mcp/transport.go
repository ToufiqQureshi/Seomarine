package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"time"
)

// serverInfo mirrors the legacy McpServer constructor arguments.
var (
	serverName         = "Seomarine MCP"
	serverTitle        = "Seomarine"
	serverVersion      = "0.0.12"
	serverDescription  = "SEO research tools for AI agents: keyword research and metrics, SERP and local SERP results, domain and backlink analysis, rank tracking, and Google Search Console performance."
	serverInstructions = "Seomarine research tools use credits. Proceed with normal focused research, but ask the user for confirmation before planned batches over 2,000 credits. Seomarine cannot purchase credits or charge cards. Explain unsupported purchase requests without promoting subscriptions, upgrades or credit purchases, or directing users to checkout."
)

// supportedProtocolVersions are the versions the Go server negotiates. The
// client's requested version is honored when known; the default stays on the
// stable entry.
var supportedProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const defaultProtocolVersion = "2025-06-18"

// serveRPC answers JSON-RPC requests locally on the API-key lane.
func (h *handler) serveRPC(ctx context.Context, w http.ResponseWriter, r *http.Request, auth Auth) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		h.writeJSON(w, http.StatusMethodNotAllowed, errorResponse(json.RawMessage("null"), codeServerError, "Method not allowed."))
		return
	}
	// A lone GET opens an SSE stream for server notifications in the stateful
	// protocol; the Go server is stateless and never publishes, so it answers
	// like the legacy lane: no body, connection can close.
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodDelete {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		h.writeJSON(w, http.StatusBadRequest, errorResponse(json.RawMessage("null"), codeParseError, "Unreadable request body."))
		return
	}
	if int64(len(body)) > maxBodyBytes {
		h.writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse(json.RawMessage("null"), codeInvalidRequest, "Request body too large."))
		return
	}

	trimmed := bytes.TrimSpace(body)
	requests, isBatch, ok := decodeRequests(trimmed)
	if !ok {
		h.writeJSON(w, http.StatusBadRequest, errorResponse(json.RawMessage("null"), codeParseError, "Invalid JSON-RPC request."))
		return
	}
	if len(requests) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}


	responses := make([]rpcResponse, 0, len(requests))
	for _, req := range requests {
		if req.isNotification() {
			h.executeNotification(ctx, req, auth)
			continue
		}
		responses = append(responses, h.execute(ctx, r, req, auth))
	}
	if len(responses) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if !isBatch {
		h.writeJSON(w, http.StatusOK, responses[0])
		return
	}
	h.writeJSON(w, http.StatusOK, responses)
}

// decodeRequests parses a single request or a batch. ok is false only when
// the body is not JSON-RPC at all.
func decodeRequests(body []byte) (requests []rpcRequest, isBatch bool, ok bool) {
	if len(body) == 0 {
		return nil, false, false
	}
	if body[0] == '[' {
		var batch []rpcRequest
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, true, false
		}
		for _, req := range batch {
			if req.JSONRPC == "" {
				req.JSONRPC = "2.0"
			}
			requests = append(requests, req)
		}
		return requests, true, true
	}
	var single rpcRequest
	if err := json.Unmarshal(body, &single); err != nil {
		return nil, false, false
	}
	if single.JSONRPC == "" {
		single.JSONRPC = "2.0"
	}
	return []rpcRequest{single}, false, true
}

// executeNotification runs a notification's side effects. The Go server has
// none today beyond accepting the handshake.
func (h *handler) executeNotification(_ context.Context, _ rpcRequest, _ Auth) {}

// execute runs one JSON-RPC request locally with panic recovery.
func (h *handler) execute(ctx context.Context, r *http.Request, req rpcRequest, auth Auth) (resp rpcResponse) {
	defer func() {
		if recovered := recover(); recovered != nil {
			h.deps.Logger.ErrorContext(ctx, "panic in mcp handler",
				"method", req.Method, "panic_type", fmt.Sprintf("%T", recovered), "stack", string(debug.Stack()))
			resp = errorResponse(req.ID, codeInternalError, "Internal error.")
		}
	}()

	switch req.Method {
	case "initialize":
		return resultResponse(req.ID, h.initializeResult(req.Params))
	case "ping":
		return resultResponse(req.ID, map[string]any{})
	case "notifications/initialized", "notifications/cancelled", "notifications/progress":
		return resultResponse(req.ID, map[string]any{})
	case "tools/list":
		tools, err := h.listTools()
		if err != nil {
			h.deps.Logger.ErrorContext(ctx, "merge tools list failed", "err", err)
			return errorResponse(req.ID, codeServerError, "Could not load the Seomarine tool list.")
		}
		return resultResponse(req.ID, map[string]any{"tools": tools})
	case "tools/call":
		name := toolCallName(req.Params)
		t, ok := h.registryIndex()[name]
		if !ok {
			return errorResponse(req.ID, codeMethodNotFound, "Method not found.")
		}
		started := time.Now()
		result, outcome := h.callTool(ctx, t, req.Params, auth)
		h.recordToolCall(ctx, name, auth, started, outcome)
		return resultResponse(req.ID, result)
	default:
		return errorResponse(req.ID, codeMethodNotFound, "Method not found.")
	}
}

// initializeResult answers the MCP handshake with the same server identity and
// instructions as legacy.
func (h *handler) initializeResult(params json.RawMessage) map[string]any {
	version := defaultProtocolVersion
	var parsed struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &parsed); err == nil && parsed.ProtocolVersion != "" {
		for _, known := range supportedProtocolVersions {
			if parsed.ProtocolVersion == known {
				version = known
				break
			}
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo": map[string]any{
			"name":        serverName,
			"title":       serverTitle,
			"version":     serverVersion,
			"description": serverDescription,
		},
		"instructions": serverInstructions,
	}
}
