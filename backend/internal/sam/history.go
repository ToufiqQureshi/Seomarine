package sam

import (
	"encoding/json"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/openrouter"
)

const interruptedToolResult = "Error: this tool call was interrupted before it returned a result."

// toLLMMessages rebuilds the provider conversation from stored messages. A
// stored assistant message holds every step of one turn, separated by
// step-start parts; each step becomes one assistant message followed by the
// tool results it asked for, which is the order providers require.
func toLLMMessages(messages []Message) []openrouter.Message {
	var out []openrouter.Message
	for _, message := range messages {
		switch message.Role {
		case "user":
			if text := joinText(message.Parts); text != "" {
				out = append(out, openrouter.Message{Role: "user", Content: text})
			}
		case "assistant":
			out = append(out, assistantSteps(message.Parts)...)
		}
	}
	return out
}

func joinText(parts []Part) string {
	var texts []string
	for _, part := range parts {
		if part.Type == "text" && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func assistantSteps(parts []Part) []openrouter.Message {
	var (
		out     []openrouter.Message
		text    strings.Builder
		calls   []openrouter.ToolCall
		results []openrouter.Message
	)
	flush := func() {
		if text.Len() > 0 || len(calls) > 0 {
			out = append(out, openrouter.Message{Role: "assistant", Content: text.String(), ToolCalls: calls})
			out = append(out, results...)
		}
		text.Reset()
		calls, results = nil, nil
	}
	for _, part := range parts {
		switch {
		case part.Type == "step-start":
			flush()
		case part.Type == "text":
			text.WriteString(part.Text)
		case strings.HasPrefix(part.Type, "tool-"):
			arguments := string(part.Input)
			if arguments == "" {
				arguments = "{}"
			}
			calls = append(calls, openrouter.ToolCall{
				ID:   part.ToolCallID,
				Type: "function",
				Function: openrouter.FunctionCall{
					Name:      strings.TrimPrefix(part.Type, "tool-"),
					Arguments: arguments,
				},
			})
			results = append(results, openrouter.Message{
				Role:       "tool",
				ToolCallID: part.ToolCallID,
				Content:    toolResultContent(part),
			})
		}
	}
	flush()
	return out
}

func toolResultContent(part Part) string {
	switch part.State {
	case stateOutputAvailable:
		if len(part.Output) == 0 {
			return "null"
		}
		return string(part.Output)
	case stateOutputError:
		return "Error: " + part.ErrorText
	default:
		return interruptedToolResult
	}
}

// estimateTokens approximates the transcript size at four characters a token.
// It runs high next to a real tokenizer, which errs on the safe side for a
// budget check.
func estimateTokens(messages []Message) int {
	chars := 0
	for _, message := range messages {
		encoded, err := json.Marshal(message.Parts)
		if err != nil {
			continue
		}
		chars += len(encoded)
	}
	return chars / 4
}
