package aisearch

import (
	"errors"
	"strings"
	"testing"
)

func TestDetectTarget(t *testing.T) {
	tests := []struct {
		input string
		want  Target
	}{
		{"example.com", Target{TargetDomain, "example.com"}},
		{"  https://www.Example.com/blog?x=1  ", Target{TargetDomain, "example.com"}},
		{"Example Brand", Target{TargetKeyword, "Example Brand"}},
		{"Nike", Target{TargetKeyword, "Nike"}},
		{"node.js tutorial", Target{TargetKeyword, "node.js tutorial"}}, // whitespace means keyword
		{"v1.2", Target{TargetDomain, "v1.2"}},                          // looks like a host, as the legacy heuristic does
		{"bücher.de", Target{TargetDomain, "xn--bcher-kva.de"}},
		{"", Target{TargetKeyword, ""}},
		{"https://", Target{TargetKeyword, "https://"}},
		{"exa mple.com", Target{TargetKeyword, "exa mple.com"}},
		// Like the legacy parser, detection only normalizes; research scoping rejects.
		{"a..b.com", Target{TargetDomain, "a..b.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := detectTarget(tt.input); got != tt.want {
				t.Errorf("detectTarget(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseResearchTarget(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		requested Scope
		want      ResearchTarget
		wantErr   string
	}{
		{name: "root domain defaults to subdomains", input: "example.com", want: ResearchTarget{ScopeSubdomains, "example.com", "", "example.com"}},
		{name: "path defaults to subfolder", input: "https://www.example.com/blog/", want: ResearchTarget{ScopeSubfolder, "example.com", "/blog", "example.com/blog"}},
		{name: "query and fragment are dropped", input: "example.com/a/b?q=1#top", requested: ScopeExactURL, want: ResearchTarget{ScopeExactURL, "example.com", "/a/b", "example.com/a/b"}},
		{name: "domain scope hides the path", input: "example.com/blog", requested: ScopeDomain, want: ResearchTarget{ScopeDomain, "example.com", "/blog", "example.com"}},
		{name: "private suffix is a real host", input: "mysite.github.io", want: ResearchTarget{ScopeSubdomains, "mysite.github.io", "", "mysite.github.io"}},
		{name: "subfolder needs a path", input: "example.com", requested: ScopeSubfolder, wantErr: "Add a path to use Subfolder (e.g. example.com/blog)"},
		{name: "empty", input: "   ", wantErr: "Enter a domain or URL"},
		{name: "fake tld", input: "example.por", wantErr: "Enter a valid domain like example.com"},
		{name: "no dot", input: "localhost", wantErr: "Enter a valid domain like example.com"},
		{name: "ip address", input: "http://192.168.1.1/admin", wantErr: "Enter a valid domain like example.com"},
		{name: "underscore host", input: "my_site.com", wantErr: "Enter a valid domain like example.com"},
		{name: "credentials", input: "https://user:pass@example.com", wantErr: "URLs with embedded credentials are not supported"},
		{name: "garbage", input: "http://exa mple.com", wantErr: "Enter a valid domain like example.com"},
		{name: "empty label", input: "a..b.com", wantErr: "Enter a valid domain like example.com"},
		{name: "label starting with a hyphen", input: "-a.com", wantErr: "Enter a valid domain like example.com"},
		{name: "label ending with a hyphen", input: "a-.com", wantErr: "Enter a valid domain like example.com"},
		{name: "label over 63 characters", input: strings.Repeat("a", 64) + ".com", wantErr: "Enter a valid domain like example.com"},
		{name: "trailing dot", input: "example.com.", wantErr: "Enter a valid domain like example.com"},
		{name: "longest label", input: strings.Repeat("a", 63) + ".com", want: ResearchTarget{ScopeSubdomains, strings.Repeat("a", 63) + ".com", "", strings.Repeat("a", 63) + ".com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResearchTarget(tt.input, tt.requested)
			if tt.wantErr != "" {
				var inputErr inputError
				if !errors.As(err, &inputErr) || err.Error() != tt.wantErr {
					t.Fatalf("parseResearchTarget(%q) error = %v, want %q", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("parseResearchTarget(%q) = %+v, %v, want %+v", tt.input, got, err, tt.want)
			}
		})
	}
}

func TestResearchTargetMatches(t *testing.T) {
	target := func(scope Scope, path string) ResearchTarget {
		return ResearchTarget{Scope: scope, Hostname: "example.com", Path: path}
	}
	tests := []struct {
		name   string
		target ResearchTarget
		url    string
		want   bool
	}{
		{"subdomains takes the root", target(ScopeSubdomains, ""), "https://example.com/x", true},
		{"subdomains takes www", target(ScopeSubdomains, ""), "https://www.example.com/x", true},
		{"subdomains takes a subdomain", target(ScopeSubdomains, ""), "https://blog.example.com/x", true},
		{"subdomains rejects a look-alike host", target(ScopeSubdomains, ""), "https://notexample.com/x", false},
		{"domain rejects a subdomain", target(ScopeDomain, ""), "https://blog.example.com/x", false},
		{"domain takes any path", target(ScopeDomain, ""), "https://example.com/a/b", true},
		{"subfolder takes the folder itself", target(ScopeSubfolder, "/blog"), "https://example.com/blog", true},
		{"subfolder takes a trailing slash", target(ScopeSubfolder, "/blog"), "https://example.com/blog/", true},
		{"subfolder takes children", target(ScopeSubfolder, "/blog"), "https://example.com/blog/post", true},
		{"subfolder rejects a sibling with the same prefix", target(ScopeSubfolder, "/blog"), "https://example.com/blogging", false},
		{"subfolder rejects another host", target(ScopeSubfolder, "/blog"), "https://other.example/blog", false},
		{"exact takes the page", target(ScopeExactURL, "/a"), "https://example.com/a?x=1#y", true},
		{"exact rejects a child", target(ScopeExactURL, "/a"), "https://example.com/a/b", false},
		{"exact root", target(ScopeExactURL, ""), "https://example.com/", true},
		{"unparseable URL", target(ScopeDomain, ""), "http://exa mple.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.target.matches(tt.url); got != tt.want {
				t.Errorf("matches(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestSafeHTTPURL(t *testing.T) {
	for _, tt := range []struct {
		in string
		ok bool
	}{
		{"https://example.com/a", true},
		{"http://example.com", true},
		{"javascript:alert(1)", false},
		{"data:text/html,x", false},
		{"//example.com/a", false},
		{"/relative", false},
		{"https://user:pw@example.com", false},
		{"https://", false},
		{"", false},
		{"not a url", false},
	} {
		if got, ok := safeHTTPURL(tt.in); ok != tt.ok || (ok && got != tt.in) {
			t.Errorf("safeHTTPURL(%q) = %q, %v, want ok=%v", tt.in, got, ok, tt.ok)
		}
	}
	if got := safeHostname("https://WWW.Example.com/x"); got == nil || *got != "example.com" {
		t.Errorf("safeHostname = %v, want example.com", got)
	}
	if got := safeHostname("ftp://example.com"); got != nil {
		t.Errorf("safeHostname(ftp) = %v, want nil", *got)
	}
}
