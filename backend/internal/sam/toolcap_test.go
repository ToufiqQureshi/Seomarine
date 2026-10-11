package sam

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func rows(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = map[string]any{"keyword": fmt.Sprintf("keyword number %d", i), "volume": float64(i)}
	}
	return out
}

func TestCapToolOutputLeavesSmallValuesUntouched(t *testing.T) {
	value := map[string]any{"rows": rows(3)}
	got := capToolOutput(value, maxToolOutputChars)
	if jsonSize(got) != jsonSize(value) || strings.Contains(fmt.Sprint(got), "truncated") {
		t.Fatalf("small value changed: %v", got)
	}
}

func TestCapToolOutputHalvesLargestArrayAndExplainsIt(t *testing.T) {
	value := map[string]any{"total": float64(900), "rows": rows(900)}
	got, ok := capToolOutput(value, 8_000).(map[string]any)
	if !ok {
		t.Fatalf("expected an object, got %T", got)
	}
	if size := jsonSize(got); size > 8_000 {
		t.Fatalf("trimmed output is %d chars, over the cap", size)
	}
	kept, _ := got["rows"].([]any)
	if len(kept) == 0 || len(kept) >= 900 {
		t.Fatalf("rows kept = %d", len(kept))
	}
	first, _ := kept[0].(map[string]any)
	if first["keyword"] != "keyword number 0" {
		t.Fatalf("the head must be kept, first row = %v", first)
	}
	note, _ := got["truncated"].(string)
	if !strings.Contains(note, fmt.Sprintf("`rows` shows %d of 900 rows", len(kept))) {
		t.Fatalf("note does not describe the cut: %q", note)
	}
	if got["total"] != float64(900) {
		t.Fatalf("untouched fields must survive, total = %v", got["total"])
	}
}

func TestCapToolOutputCapsLongStringsFirst(t *testing.T) {
	value := map[string]any{"pages": []any{map[string]any{"text": strings.Repeat("é", 30_000)}}}
	got := capToolOutput(value, maxToolOutputChars).(map[string]any)
	pages := got["pages"].([]any)
	text := pages[0].(map[string]any)["text"].(string)
	if n := len([]rune(text)); n != maxStringChars {
		t.Fatalf("string kept %d runes, want %d", n, maxStringChars)
	}
	if !strings.Contains(got["truncated"].(string), "`pages[*].text` shortened to 8000 chars") {
		t.Fatalf("note = %q", got["truncated"])
	}
	if !json.Valid([]byte(mustJSON(t, got))) {
		t.Fatal("trimmed output is not valid JSON (split a multi-byte rune?)")
	}
}

func TestCapToolOutputShortensStringsWhenNoArrayRemains(t *testing.T) {
	value := map[string]any{}
	for i := range 10 {
		value[fmt.Sprintf("k%d", i)] = strings.Repeat("x", 7_000)
	}
	got := capToolOutput(value, 20_000).(map[string]any)
	if size := jsonSize(got); size > 20_000 {
		t.Fatalf("output is %d chars, over the cap", size)
	}
}

func TestCapToolOutputWrapsNonObjects(t *testing.T) {
	got, ok := capToolOutput(rows(2_000), 5_000).(map[string]any)
	if !ok || got["truncated"] == nil || got["data"] == nil {
		t.Fatalf("array results must be wrapped: %T %v", got, got)
	}
}

func TestCapToolOutputDoesNotMutateInput(t *testing.T) {
	value := map[string]any{"rows": rows(900)}
	_ = capToolOutput(value, 8_000)
	if n := len(value["rows"].([]any)); n != 900 {
		t.Fatalf("input rows mutated to %d", n)
	}
}

func TestPresentIgnoresPathsThatNoLongerExist(t *testing.T) {
	value := map[string]any{"rows": []any{}}
	if present(value, "rows[*].body") {
		t.Fatal("an empty rows array has no body")
	}
	if !present(map[string]any{"rows": []any{map[string]any{"body": "x"}}}, "rows[*].body") {
		t.Fatal("body exists")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
