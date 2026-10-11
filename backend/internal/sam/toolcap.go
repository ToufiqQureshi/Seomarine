package sam

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Every tool result is stored in the transcript and re-sent on each later step
// of its turn, so one oversized payload (hundreds of keyword rows, ten scraped
// pages) is paid for dozens of times. Results over the budget are trimmed
// before the model sees them, with a note saying what was dropped so it narrows
// the request instead of guessing. MCP clients still get the full payload.
const (
	maxToolOutputChars = 32_000
	// A single long string never gets more than this share.
	maxStringChars = 8_000
	minStringChars = 500
	// Reserved for the truncation note, so the note cannot push a trimmed
	// result back over the cap.
	noteBudgetChars = 600
)

type trim struct {
	path string
	kept int
	// total is the size before trimming, in rows or characters.
	total int
	rows  bool
}

// slot is one array or string inside the value, addressable for trimming.
type slot struct {
	get  func() any
	set  func(any)
	path string
}

func jsonSize(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(encoded)
}

// slots lists every array and string below value, depth first, with its path.
func slots(value any, path string) []slot {
	var out []slot
	add := func(item any, itemPath string, get func() any, set func(any)) {
		switch item.(type) {
		case []any, string:
			out = append(out, slot{get: get, set: set, path: itemPath})
		}
		out = append(out, slots(item, itemPath)...)
	}
	switch v := value.(type) {
	case []any:
		for i := range v {
			add(v[i], fmt.Sprintf("%s[%d]", path, i), func() any { return v[i] }, func(n any) { v[i] = n })
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			itemPath := key
			if path != "" {
				itemPath = path + "." + key
			}
			add(v[key], itemPath, func() any { return v[key] }, func(n any) { v[key] = n })
		}
	}
	return out
}

var indexSegment = regexp.MustCompile(`\[\d+\]`)

// shape collapses indexes so 500 shortened rows[i].body strings are one note
// line, rows[*].body.
func shape(path string) string { return indexSegment.ReplaceAllString(path, "[*]") }

var pathSegment = regexp.MustCompile(`\[\*\]|[^.\[\]]+`)

// present reports whether a shape path still points at something in value, so
// the note never describes rows the halving loop has since dropped.
func present(value any, path string) bool {
	node := value
	for _, segment := range pathSegment.FindAllString(path, -1) {
		if segment == "[*]" {
			list, ok := node.([]any)
			if !ok || len(list) == 0 {
				return false
			}
			node = list[0]
			continue
		}
		object, ok := node.(map[string]any)
		if !ok {
			return false
		}
		next, ok := object[segment]
		if !ok {
			return false
		}
		node = next
	}
	return true
}

func trimNote(trims []trim) string {
	parts := make([]string, 0, len(trims))
	for _, t := range trims {
		if t.rows {
			parts = append(parts, fmt.Sprintf("`%s` shows %d of %d rows", t.path, t.kept, t.total))
		} else {
			parts = append(parts, fmt.Sprintf("`%s` shortened to %d chars", t.path, t.kept))
		}
	}
	text := "Output trimmed to fit the chat context: " + strings.Join(parts, "; ") +
		". Narrow the request (filters, a smaller limit, fewer pages) to see the rest."
	if runes := []rune(text); len(runes) > noteBudgetChars {
		return string(runes[:noteBudgetChars-1]) + "…"
	}
	return text
}

// capToolOutput returns value unchanged when it serializes under maxChars.
// Otherwise it returns a trimmed deep copy: long strings are capped first, then
// the largest array is halved (keeping the head, since results are ordered by
// relevance) until it fits, then the longest string, with a "truncated" note
// listing what was cut. The note counts against the cap.
func capToolOutput(value any, maxChars int) any {
	if jsonSize(value) <= maxChars {
		return value
	}
	budget := maxChars - noteBudgetChars

	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{"truncated": "Output could not be encoded and was dropped."}
	}
	var work any
	if err := json.Unmarshal(encoded, &work); err != nil {
		return map[string]any{"truncated": "Output could not be decoded and was dropped."}
	}

	trims := map[string]*trim{}
	var order []string
	record := func(s slot, kept, total int, rows bool) {
		key := shape(s.path)
		existing, ok := trims[key]
		if !ok {
			trims[key] = &trim{path: key, kept: kept, total: total, rows: rows}
			order = append(order, key)
			return
		}
		existing.kept = min(existing.kept, kept)
		existing.total = max(existing.total, total)
	}

	for _, s := range slots(work, "") {
		if text, ok := s.get().(string); ok {
			if runes := []rune(text); len(runes) > maxStringChars {
				s.set(string(runes[:maxStringChars]))
				record(s, maxStringChars, len(runes), false)
			}
		}
	}

	for jsonSize(work) > budget {
		var (
			best     slot
			bestSize = -1
		)
		for _, s := range slots(work, "") {
			if list, ok := s.get().([]any); ok && len(list) > 1 {
				if size := jsonSize(list); size > bestSize {
					best, bestSize = s, size
				}
			}
		}
		if bestSize < 0 {
			break
		}
		list, _ := best.get().([]any)
		kept := (len(list) + 1) / 2
		best.set(list[:kept])
		record(best, kept, len(list), true)
	}

	for jsonSize(work) > budget {
		var (
			best    slot
			bestLen = minStringChars
		)
		for _, s := range slots(work, "") {
			if text, ok := s.get().(string); ok {
				if n := len([]rune(text)); n > bestLen {
					best, bestLen = s, n
				}
			}
		}
		if best.get == nil {
			break
		}
		runes := []rune(best.get().(string))
		kept := max(minStringChars, (len(runes)+1)/2)
		best.set(string(runes[:kept]))
		record(best, kept, len(runes), false)
	}

	var surviving []trim
	for _, key := range order {
		if present(work, trims[key].path) {
			surviving = append(surviving, *trims[key])
		}
	}
	note := trimNote(surviving)
	if object, ok := work.(map[string]any); ok {
		object["truncated"] = note
		return object
	}
	return map[string]any{"data": work, "truncated": note}
}
