package ranktracking

import (
	"errors"
	"strings"
	"unicode/utf16"
)

// ErrInvalidDomain means the input has no usable host after normalising.
var ErrInvalidDomain = errors.New("invalid domain")

// NormalizeDomain lowercases a domain and strips the protocol, www, path, query,
// fragment and trailing slashes, so one site is tracked under one key.
func NormalizeDomain(input string) (string, error) {
	d := strings.ToLower(strings.TrimSpace(input))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	d = strings.TrimPrefix(d, "www.")
	if d == "" {
		return "", ErrInvalidDomain
	}
	return d, nil
}

// PrepareKeywords normalises, dedupes and caps new keywords for a config.
// existing holds keywords already stored; room is how many more may be added.
// Keywords are lowercased unless matchCase is set, in which case "Nodex" and
// "nodex" are tracked (and billed) separately. Keywords longer than
// MaxKeywordUnits UTF-16 units are rejected, not truncated.
func PrepareKeywords(raw, existing []string, matchCase bool) (added []string, tooLong []string) {
	have := make(map[string]struct{}, len(existing)+len(raw))
	for _, kw := range existing {
		have[kw] = struct{}{}
	}
	room := MaxKeywordsPerConfig - len(existing)
	for _, r := range raw {
		if len(added) >= room {
			break
		}
		kw := strings.TrimSpace(r)
		if !matchCase {
			kw = strings.ToLower(kw)
		}
		if kw == "" {
			continue
		}
		if len(utf16.Encode([]rune(kw))) > MaxKeywordUnits {
			tooLong = append(tooLong, kw)
			continue
		}
		if _, dup := have[kw]; dup {
			continue
		}
		have[kw] = struct{}{}
		added = append(added, kw)
	}
	return added, tooLong
}
