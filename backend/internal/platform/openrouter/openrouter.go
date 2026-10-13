// Package openrouter streams chat completions from OpenRouter.
//
// It speaks the OpenAI-compatible chat completions API and nothing else: no
// retries (the caller decides, because a retry re-bills), no tool execution.
package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the OpenRouter API root.
	DefaultBaseURL = "https://openrouter.ai/api/v1"
	// DefaultModel is the model used when none is configured.
	DefaultModel = "openai/gpt-5.6-luna"

	// maxLine bounds one SSE line so a hostile or broken upstream cannot grow
	// memory without limit.
	maxLine = 1 << 20
	// maxErrorBody bounds how much of an error response is read.
	maxErrorBody = 4 << 10
)

// Message is one chat message. Assistant messages may carry ToolCalls; tool
// messages carry the ToolCallID they answer.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a model request to run one tool. Arguments is a JSON document.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall names the tool and carries its JSON arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool describes one callable function to the model.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// Request is one streaming completion.
type Request struct {
	Model     string
	Messages  []Message
	Tools     []Tool
	MaxTokens int
	// ReasoningEffort is passed through as reasoning.effort when set.
	ReasoningEffort string
}

// Usage is the token and cost accounting OpenRouter reports for a response.
type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CostUSD          float64 `json:"cost"`
}

// Prices used when OpenRouter reports no cost, from the expensive end of the
// models Uplumo uses, so a missing field never makes an answer free.
const (
	fallbackInputUSDPerToken  = 5.0 / 1_000_000
	fallbackOutputUSDPerToken = 25.0 / 1_000_000
	// charsPerToken is a low guess at characters per token, for output that
	// streamed before the stream broke and no usage arrived.
	charsPerToken = 3
)

// BillableUSD is what to charge for a call: the cost OpenRouter reported, or,
// when it reported none, an estimate from the token counts or, failing those,
// from generatedChars, the characters that streamed. estimated says which.
func (u Usage) BillableUSD(generatedChars int) (usd float64, estimated bool) {
	if u.CostUSD > 0 && !math.IsInf(u.CostUSD, 0) {
		return u.CostUSD, false
	}
	outputTokens := u.CompletionTokens
	if outputTokens <= 0 && generatedChars > 0 {
		outputTokens = (generatedChars + charsPerToken - 1) / charsPerToken
	}
	inputTokens := max(u.PromptTokens, 0)
	if outputTokens <= 0 && inputTokens == 0 {
		return 0, false
	}
	return float64(inputTokens)*fallbackInputUSDPerToken + float64(max(outputTokens, 0))*fallbackOutputUSDPerToken, true
}

// Result is the assembled response once the stream ends.
type Result struct {
	Text         string
	ToolCalls    []ToolCall
	FinishReason string
	Usage        Usage
}

// Handler receives deltas as they arrive. Either callback may be nil.
type Handler struct {
	OnText      func(delta string) error
	OnReasoning func(delta string) error
}

// Client calls the OpenRouter API.
type Client struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

// StatusError is a non-2xx response from the API.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("openrouter returned %d: %s", e.StatusCode, e.Body)
}

// ErrStreamError means the upstream sent an error object mid-stream.
var ErrStreamError = errors.New("openrouter stream error")

type requestBody struct {
	Model     string          `json:"model"`
	Messages  []Message       `json:"messages"`
	Tools     []toolEnvelope  `json:"tools,omitempty"`
	Stream    bool            `json:"stream"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Usage     map[string]bool `json:"usage"`
	Reasoning map[string]any  `json:"reasoning,omitempty"`
}

type toolEnvelope struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type streamChunk struct {
	Error   *streamError `json:"error"`
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

type streamError struct {
	Message string `json:"message"`
}

// Stream sends req and feeds deltas to h until the response ends. The returned
// Result is complete even when the context is canceled mid-stream only if the
// error is nil; on error the partial Result is still returned so the caller can
// bill and persist what was produced.
func (c *Client) Stream(ctx context.Context, req Request, h Handler) (Result, error) {
	body := requestBody{
		Model:     req.Model,
		Messages:  req.Messages,
		Stream:    true,
		MaxTokens: req.MaxTokens,
		Usage:     map[string]bool{"include": true},
	}
	if req.ReasoningEffort != "" {
		body.Reasoning = map[string]any{"effort": req.ReasoningEffort}
	}
	for _, t := range req.Tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		body.Tools = append(body.Tools, toolEnvelope{Type: "function", Function: toolFunction{
			Name: t.Name, Description: t.Description, Parameters: params,
		}})
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, fmt.Errorf("encode openrouter request: %w", err)
	}

	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Result{}, fmt.Errorf("build openrouter request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	client := c.HTTP
	if client == nil {
		// No overall timeout: a turn can legitimately stream for minutes. The
		// header timeout bounds how long the provider may take to start.
		client = &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 60 * time.Second}}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return Result{}, fmt.Errorf("call openrouter: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		closeErr := resp.Body.Close()
		statusErr := &StatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(snippet))}
		if readErr != nil || closeErr != nil {
			return Result{}, errors.Join(statusErr, readErr, closeErr)
		}
		return Result{}, statusErr
	}
	result, streamErr := readStream(resp.Body, h)
	if closeErr := resp.Body.Close(); closeErr != nil {
		streamErr = errors.Join(streamErr, fmt.Errorf("close OpenRouter response: %w", closeErr))
	}
	return result, streamErr
}

// readStream folds the SSE body into a Result.
func readStream(body io.Reader, h Handler) (Result, error) {
	var (
		result Result
		text   strings.Builder
		calls  = map[int]*ToolCall{}
	)
	finish := func() Result {
		result.Text = text.String()
		result.ToolCalls = nil
		for _, index := range slices.Sorted(maps.Keys(calls)) {
			call := calls[index]
			call.Type = "function"
			result.ToolCalls = append(result.ToolCalls, *call)
		}
		return result
	}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLine)
	for scanner.Scan() {
		line := scanner.Text()
		// Comment lines (": OPENROUTER PROCESSING") keep the connection warm.
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return finish(), fmt.Errorf("decode openrouter chunk: %w", err)
		}
		if chunk.Error != nil {
			return finish(), fmt.Errorf("%w: %s", ErrStreamError, chunk.Error.Message)
		}
		if chunk.Usage != nil {
			result.Usage = *chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Reasoning != "" && h.OnReasoning != nil {
				if err := h.OnReasoning(choice.Delta.Reasoning); err != nil {
					return finish(), err
				}
			}
			if choice.Delta.Content != "" {
				text.WriteString(choice.Delta.Content)
				if h.OnText != nil {
					if err := h.OnText(choice.Delta.Content); err != nil {
						return finish(), err
					}
				}
			}
			for _, delta := range choice.Delta.ToolCalls {
				call := calls[delta.Index]
				if call == nil {
					call = &ToolCall{}
					calls[delta.Index] = call
				}
				if delta.ID != "" {
					call.ID = delta.ID
				}
				if delta.Function.Name != "" {
					call.Function.Name = delta.Function.Name
				}
				call.Function.Arguments += delta.Function.Arguments
			}
			if choice.FinishReason != nil {
				result.FinishReason = *choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return finish(), fmt.Errorf("read openrouter stream: %w", err)
	}
	return finish(), nil
}
