package domain

import "testing"

func TestParseResearchTarget(t *testing.T) {
	t.Run("normalizes host and strips www", func(t *testing.T) {
		got, err := parseResearchTarget("https://www.example.com/blog", ScopeSubfolder)
		if err != nil {
			t.Fatalf("parseResearchTarget returned err: %v", err)
		}
		if got.Hostname != "example.com" {
			t.Fatalf("Hostname = %q, want %q", got.Hostname, "example.com")
		}
		if got.Path != "/blog" {
			t.Fatalf("Path = %q, want %q", got.Path, "/blog")
		}
	})

	t.Run("rejects invalid domain", func(t *testing.T) {
		_, err := parseResearchTarget("not a domain", ScopeDomain)
		if err == nil {
			t.Fatal("parseResearchTarget() error = nil, want validation error")
		}
	})
}
