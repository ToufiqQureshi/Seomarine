package sam

import (
	"encoding/json"
	"testing"
)

func TestToLLMMessagesRebuildsStepsInProviderOrder(t *testing.T) {
	messages := []Message{
		{Role: "user", Parts: []Part{{Type: "text", Text: "audit my site"}}},
		{Role: "assistant", Parts: []Part{
			{Type: "step-start"},
			{Type: "reasoning", Text: "private thoughts"},
			{Type: "text", Text: "Starting."},
			{Type: "tool-run_site_audit", ToolCallID: "c1", State: stateOutputAvailable,
				Input: json.RawMessage(`{"url":"x"}`), Output: json.RawMessage(`{"ok":true}`)},
			{Type: "tool-get_audit_status", ToolCallID: "c2", State: stateOutputError, ErrorText: "boom"},
			{Type: "step-start"},
			{Type: "text", Text: "Done."},
		}},
	}
	got := toLLMMessages(messages)
	wantRoles := []string{"user", "assistant", "tool", "tool", "assistant"}
	if len(got) != len(wantRoles) {
		t.Fatalf("got %d messages: %+v", len(got), got)
	}
	for i, role := range wantRoles {
		if got[i].Role != role {
			t.Errorf("message %d role = %s, want %s", i, got[i].Role, role)
		}
	}
	if got[1].Content != "Starting." || len(got[1].ToolCalls) != 2 {
		t.Errorf("step 1 = %+v", got[1])
	}
	if got[1].ToolCalls[0].Function.Name != "run_site_audit" || got[1].ToolCalls[0].Function.Arguments != `{"url":"x"}` {
		t.Errorf("tool call = %+v", got[1].ToolCalls[0])
	}
	if got[2].ToolCallID != "c1" || got[2].Content != `{"ok":true}` {
		t.Errorf("first result = %+v", got[2])
	}
	if got[3].Content != "Error: boom" {
		t.Errorf("error result = %+v", got[3])
	}
	for _, m := range got {
		if m.Content == "private thoughts" {
			t.Fatal("reasoning must never be sent back to the provider")
		}
	}
}

func TestToLLMMessagesAnswersInterruptedToolCalls(t *testing.T) {
	got := toLLMMessages([]Message{{Role: "assistant", Parts: []Part{
		{Type: "tool-research_keywords", ToolCallID: "c1", State: "input-available"},
	}}})
	if len(got) != 2 || got[1].Role != "tool" || got[1].Content != interruptedToolResult {
		t.Fatalf("an unanswered tool call must still get a result: %+v", got)
	}
	if got[0].ToolCalls[0].Function.Arguments != "{}" {
		t.Fatalf("missing input should default to an empty object, got %q", got[0].ToolCalls[0].Function.Arguments)
	}
}

func TestToLLMMessagesSkipsEmptyMessages(t *testing.T) {
	got := toLLMMessages([]Message{
		{Role: "user", Parts: nil},
		{Role: "assistant", Parts: []Part{{Type: "step-start"}, {Type: "reasoning", Text: "x"}}},
		{Role: "user", Parts: []Part{{Type: "text", Text: "hi"}, {Type: "text", Text: "there"}}},
	})
	if len(got) != 1 || got[0].Content != "hi\nthere" {
		t.Fatalf("got %+v", got)
	}
}

func TestEstimateTokensGrowsWithContent(t *testing.T) {
	small := estimateTokens([]Message{{Parts: []Part{{Type: "text", Text: "hi"}}}})
	large := estimateTokens([]Message{{Parts: []Part{{Type: "text", Text: string(make([]byte, 4_000))}}}})
	if small >= large || large < 1_000 {
		t.Fatalf("small = %d large = %d", small, large)
	}
}

func TestStripNULEscapes(t *testing.T) {
	tests := []struct{ in, want string }{
		{`"a\u0000b"`, `"ab"`},
		{`"\u0000\u0000"`, `""`},
		{`"keep \\u0000 literal"`, `"keep \\u0000 literal"`}, // escaped backslash + text
		{`"\\\u0000"`, `"\\"`},                               // escaped backslash, then a real NUL
		{`"tab\tquote\""`, `"tab\tquote\""`},
		{`"é"`, `"é"`},
		{`"trailing\`, `"trailing\`},
	}
	for _, tt := range tests {
		if got := string(stripNULEscapes([]byte(tt.in))); got != tt.want {
			t.Errorf("stripNULEscapes(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
