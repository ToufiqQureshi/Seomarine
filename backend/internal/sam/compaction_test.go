package sam

import (
	"context"
	"strings"
	"testing"
)

func bigMessage(id, role string, chars int) Message {
	return Message{ID: id, Role: role, Parts: []Part{{Type: "text", Text: strings.Repeat("w", chars)}}}
}

func seed(t *testing.T, h *harness, messages ...Message) {
	t.Helper()
	for _, m := range messages {
		if err := h.store.Append(context.Background(), h.sc.SessionID, m); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCompactionLeavesShortChatsAlone(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"summary"}})
	seed(t, h, bigMessage("a", "user", 100), bigMessage("b", "assistant", 100))
	if err := h.agent.compact(context.Background(), h.sc); err != nil {
		t.Fatal(err)
	}
	if h.llm.callCount() != 0 || h.store.summary != nil {
		t.Fatal("a short chat must not be summarized")
	}
}

func TestCompactionSummarizesOlderMessagesAndKeepsRecentOnes(t *testing.T) {
	h := newHarness(t, scriptStep{text: []string{"They sell anvils; goal is sales."}, cost: 0.02})
	h.agent.Meter = h.meter
	var messages []Message
	for i := range 12 {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, bigMessage(string(rune('a'+i)), role, 60_000))
	}
	seed(t, h, messages...)
	if err := h.agent.compact(context.Background(), h.sc); err != nil {
		t.Fatal(err)
	}
	if h.store.summary == nil {
		t.Fatal("no summary saved")
	}
	// 12 messages, 6 kept verbatim: the first 6 (seq 1..6) are summarized.
	if h.store.summary.ThroughSeq != 6 || h.store.summary.Text != "They sell anvils; goal is sales." {
		t.Fatalf("summary = %+v", *h.store.summary)
	}
	if len(h.meter.charges) != 1 {
		t.Fatalf("the summary call is billed: charges = %v", h.meter.charges)
	}
	req := h.llm.requests[0]
	if req.ReasoningEffort != "low" || len(req.Tools) != 0 {
		t.Fatalf("summaries run at low effort without tools: %+v", req)
	}
	if !strings.Contains(req.Messages[1].Content, "Conversation to fold in") {
		t.Fatalf("prompt = %.200s", req.Messages[1].Content)
	}

	// A second pass folds the prior summary in and only sees newer messages.
	for i := range 8 {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		seed(t, h, bigMessage("n"+string(rune('a'+i)), role, 60_000))
	}
	h.llm.steps = append(h.llm.steps, scriptStep{text: []string{"second summary"}})
	if err := h.agent.compact(context.Background(), h.sc); err != nil {
		t.Fatal(err)
	}
	prompt := h.llm.requests[1].Messages[1].Content
	if !strings.Contains(prompt, "Summary so far:\nThey sell anvils") {
		t.Fatalf("the prior summary must be carried forward: %.200s", prompt)
	}
	if h.store.summary.Text != "second summary" || h.store.summary.ThroughSeq <= 6 {
		t.Fatalf("summary = %+v", *h.store.summary)
	}
}

func TestCompactionKeepsThePreviousSummaryWhenTheModelFails(t *testing.T) {
	h := newHarness(t, scriptStep{text: nil})
	var messages []Message
	for i := range 12 {
		messages = append(messages, bigMessage(string(rune('a'+i)), "user", 60_000))
	}
	seed(t, h, messages...)
	if err := h.agent.compact(context.Background(), h.sc); err == nil {
		t.Fatal("an empty summary must be an error, not saved")
	}
	if h.store.summary != nil {
		t.Fatal("an empty summary was saved")
	}
}

func TestRenderTranscriptClipsToolInputsAndDropsOutputs(t *testing.T) {
	out := renderTranscript([]Message{
		{Role: "user", Parts: []Part{{Type: "text", Text: "check my rankings"}}},
		{Role: "assistant", Parts: []Part{
			{Type: "step-start"},
			{Type: "tool-get_rank_tracker", Input: []byte(`{"q":"` + strings.Repeat("z", 1000) + `"}`), Output: []byte(`"SECRET-OUTPUT"`)},
			{Type: "text", Text: "You rank #3."},
		}},
	})
	if strings.Contains(out, "SECRET-OUTPUT") {
		t.Fatal("tool outputs must not be sent to the summarizer")
	}
	if !strings.Contains(out, "User: check my rankings") || !strings.Contains(out, "Assistant: You rank #3.") || !strings.Contains(out, "get_rank_tracker") {
		t.Fatalf("transcript = %q", out)
	}
	if strings.Count(out, "z") > transcriptToolChars {
		t.Fatal("tool input was not clipped")
	}
}
