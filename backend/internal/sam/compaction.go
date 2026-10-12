package sam

import (
	"context"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/uplumo/backend/internal/platform/openrouter"
)

const (
	summaryMaxTokens = 2_000
	// transcriptToolChars bounds how much of one tool call survives into the
	// text handed to the summarizer.
	transcriptToolChars = 400
)

const summarizeInstruction = "You summarize an SEO chat so it can continue with less history. Write a compact brief the assistant can rely on: what the user wants, decisions made, facts learned about the business, conclusions from research (with the numbers that matter), and anything still open. Plain Markdown, no preamble."

// compact folds the oldest messages into a rolling summary once the live
// transcript outgrows the budget. It runs between turns, never mid-turn, so a
// summary can never drop a tool result the current turn still needs.
func (a *Agent) compact(ctx context.Context, sc SessionContext) error {
	messages, err := a.Store.List(ctx, sc.SessionID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}
	prior, hasPrior, err := a.Store.Summary(ctx, sc.SessionID)
	if err != nil {
		return err
	}
	live := messages
	if hasPrior {
		live = messagesAfter(messages, prior.ThroughSeq)
	}
	if estimateTokens(live) <= compactAfterTokens || len(live) <= keepRecentMessages+1 {
		return nil
	}
	older := live[:len(live)-keepRecentMessages]

	var prompt strings.Builder
	if hasPrior {
		prompt.WriteString("Summary so far:\n" + prior.Text + "\n\n")
	}
	prompt.WriteString("Conversation to fold in:\n\n")
	prompt.WriteString(renderTranscript(older))

	result, err := a.LLM.Stream(ctx, openrouter.Request{
		Model: a.Model,
		Messages: []openrouter.Message{
			{Role: "system", Content: summarizeInstruction},
			{Role: "user", Content: prompt.String()},
		},
		MaxTokens:       summaryMaxTokens,
		ReasoningEffort: "low",
	}, openrouter.Handler{})
	// The call is billed even when its text turns out unusable.
	if a.Meter != nil && result.Usage.CostUSD > 0 {
		key := fmt.Sprintf("sam:compact:%s:%d", sc.SessionID, older[len(older)-1].Seq)
		if chargeErr := a.Meter.Charge(ctx, sc.OrganizationID, result.Usage.CostUSD, key); chargeErr != nil {
			a.logger().ErrorContext(ctx, "SAM compaction metering failed", "err", chargeErr)
		}
	}
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
	}
	text := strings.TrimSpace(result.Text)
	if text == "" {
		return fmt.Errorf("summarize: the model returned no text")
	}
	return a.Store.SaveSummary(ctx, sc.SessionID, Summary{Text: text, ThroughSeq: older[len(older)-1].Seq})
}

// renderTranscript flattens messages to plain text for the summarizer. Tool
// calls keep their name and a clipped input; outputs are left out because the
// assistant's replies already carry what mattered.
func renderTranscript(messages []Message) string {
	var b strings.Builder
	for _, m := range messages {
		label := "User"
		if m.Role == "assistant" {
			label = "Assistant"
		}
		for _, part := range m.Parts {
			switch {
			case part.Type == "text" && part.Text != "":
				fmt.Fprintf(&b, "%s: %s\n", label, part.Text)
			case strings.HasPrefix(part.Type, "tool-"):
				input := []rune(string(part.Input))
				if len(input) > transcriptToolChars {
					input = append(input[:transcriptToolChars], '…')
				}
				fmt.Fprintf(&b, "Assistant called %s %s\n", strings.TrimPrefix(part.Type, "tool-"), string(input))
			}
		}
	}
	return b.String()
}
