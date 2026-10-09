package ranktracking

import (
	"slices"
	"strings"
	"testing"
)

func TestEstimateCheck(t *testing.T) {
	many := make([]string, 101)
	for i := range many {
		many[i] = "kw"
	}
	tests := []struct {
		name     string
		keywords []string
		devices  string
		depth    int
		method   Method
		credits  int
	}{
		{"no keywords cost nothing", nil, "both", 10, Live, 0},
		{"live bills each pair on its own call", []string{"a"}, "both", 10, Live, 6},
		{"queued posts both pairs in one call", []string{"a"}, "both", 10, Queued, 2},
		{"operator keyword costs five times", []string{"site:example.com"}, "desktop", 10, Live, 13},
		{"deeper SERP adds pages", []string{"a"}, "desktop", 20, Live, 5},
		{"queued splits at 100 pairs per call", many, "desktop", 10, Queued, 77 + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EstimateCheck(tt.keywords, tt.devices, tt.depth, tt.method).CostCredits; got != tt.credits {
				t.Fatalf("credits = %d, want %d", got, tt.credits)
			}
		})
	}
}

func TestEstimateScheduledMonthly(t *testing.T) {
	for interval, checks := range map[Interval]int{Daily: 30, Weekly: 4, Monthly: 1} {
		got := EstimateScheduled([]string{"a"}, "desktop", 10, interval)
		if got.ChecksPerMonth != checks || got.MonthlyCostCredits != got.CostCredits*checks {
			t.Fatalf("%s: %+v", interval, got)
		}
	}
}

func TestPrepareKeywords(t *testing.T) {
	t.Run("lowercases, trims, dedupes and skips existing", func(t *testing.T) {
		added, _ := PrepareKeywords([]string{" SEO Tool ", "seo tool", "", "  ", "New"}, []string{"new"}, false)
		if !slices.Equal(added, []string{"seo tool"}) {
			t.Fatalf("added = %q", added)
		}
	})
	t.Run("match case keeps twins apart", func(t *testing.T) {
		added, _ := PrepareKeywords([]string{"Nodex", "nodex"}, []string{"nodex"}, true)
		if !slices.Equal(added, []string{"Nodex"}) {
			t.Fatalf("added = %q", added)
		}
	})
	t.Run("caps at the per-config limit", func(t *testing.T) {
		existing := make([]string, MaxKeywordsPerConfig-1)
		added, _ := PrepareKeywords([]string{"one", "two"}, existing, false)
		if !slices.Equal(added, []string{"one"}) {
			t.Fatalf("added = %q", added)
		}
	})
	t.Run("rejects over-long keywords by UTF-16 units", func(t *testing.T) {
		emoji := ""
		for range 101 { // each emoji is 2 UTF-16 units
			emoji += "😀"
		}
		added, tooLong := PrepareKeywords([]string{emoji, "ok"}, nil, false)
		if !slices.Equal(added, []string{"ok"}) || len(tooLong) != 1 {
			t.Fatalf("added = %q tooLong = %d", added, len(tooLong))
		}
	})
}

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		in, want string
		err      bool
	}{
		{"https://WWW.Example.com/path?q=1#x", "example.com", false},
		{"  example.com/  ", "example.com", false},
		{"http://sub.example.com", "sub.example.com", false},
		{"", "", true},
		{"https://", "", true},
		{"/path", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeDomain(tt.in)
		if (err != nil) != tt.err || got != tt.want {
			t.Errorf("NormalizeDomain(%q) = %q, %v; want %q, err %v", tt.in, got, err, tt.want, tt.err)
		}
	}
}

func TestParseDomain(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"Sub-Domain.Example.co.in", "sub-domain.example.co.in", true},
		{"https://www.example.com/a?b#c", "example.com", true},
		{"localhost", "", false},
		{"example.com:8080", "", false},
		{"-bad.example.com", "", false},
		{"bad-.example.com", "", false},
		{"a..com", "", false},
		{"exa_mple.com", "", false},
		{"münchen.de", "", false},
		{strings.Repeat("a", 64) + ".com", "", false},
		{strings.Repeat("a.", 130) + "com", "", false},
	}
	for _, tt := range tests {
		got, err := ParseDomain(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParseDomain(%q) = %q, %v; want %q, ok %v", tt.in, got, err, tt.want, tt.ok)
		}
	}
}
