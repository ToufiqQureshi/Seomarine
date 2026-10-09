package audit

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
)

// errTestNoSuchHost is returned by fakeResolver for unknown hostnames.
var errTestNoSuchHost = errors.New("no such host")

// fakeResolver resolves fixed hostnames for the guard tests.
type fakeResolver struct {
	addrs map[string][]string
	err   error
}

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if f.err != nil {
		return nil, f.err
	}
	raw, ok := f.addrs[host]
	if !ok {
		return nil, errTestNoSuchHost
	}
	addrs := make([]netip.Addr, 0, len(raw))
	for _, value := range raw {
		addrs = append(addrs, netip.MustParseAddr(value))
	}
	return addrs, nil
}

// dialAllTo returns a client that dials server for every host, without
// following redirects. Test-only.
func dialAllTo(server *httptest.Server) *http.Client {
	target, _ := url.Parse(server.URL)
	return &http.Client{
		Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, target.Host)
		}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func TestIsCrawlableURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"public https", "https://example.com/page", true},
		{"public http", "http://example.com/", true},
		{"loopback", "http://127.0.0.1/", false},
		{"loopback high", "http://127.9.9.9/", false},
		{"metadata", "http://169.254.169.254/latest/meta-data/", false},
		{"ipv6 loopback", "http://[::1]/", false},
		{"ipv6 unique local", "http://[fd00::1]/", false},
		{"ipv6 link-local", "http://[fe80::1]/", false},
		{"ipv4 mapped private", "http://[::ffff:10.0.0.1]/", false},
		{"private 10", "http://10.1.2.3/", false},
		{"private 192.168", "http://192.168.1.1/", false},
		{"carrier grade nat", "http://100.64.0.1/", false},
		{"benchmarking", "http://198.18.0.1/", false},
		{"multicast", "http://224.0.0.1/", false},
		{"localhost name", "http://localhost/", false},
		{"localhost subdomain", "http://app.localhost/", false},
		{"internal suffix", "http://db.internal/", false},
		{"local suffix", "http://printer.local/", false},
		{"metadata host", "http://metadata.google.internal/", false},
		{"ftp scheme", "ftp://example.com/", false},
		{"javascript scheme", "javascript:alert(1)", false},
		{"empty", "", false},
		{"public ip", "http://93.184.216.34/", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCrawlableURL(tc.url); got != tc.want {
				t.Fatalf("isCrawlableURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestNormalizeAndValidateStartURL(t *testing.T) {
	guard := &Guard{Resolver: fakeResolver{addrs: map[string][]string{
		"example.com": {"93.184.216.34"},
		"evil.test":   {"10.0.0.5"},
		"rebind.test": {"93.184.216.34", "127.0.0.1"},
	}}}
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "bare host gets https", input: "example.com", want: "https://example.com/"},
		{name: "query order preserved", input: "https://example.com/a/b?x=2&a=1", want: "https://example.com/a/b?x=2&a=1"},
		{name: "fragment stripped", input: "https://example.com/page#section", want: "https://example.com/page"},
		{name: "empty is invalid", input: "   ", wantErr: ErrStartURLInvalid},
		{name: "ftp is invalid", input: "ftp://example.com/", wantErr: ErrStartURLInvalid},
		{name: "loopback blocked", input: "http://127.0.0.1/", wantErr: ErrCrawlTargetBlocked},
		{name: "metadata blocked", input: "http://169.254.169.254/", wantErr: ErrCrawlTargetBlocked},
		{name: "ipv6 loopback blocked", input: "http://[::1]/", wantErr: ErrCrawlTargetBlocked},
		{name: "internal host blocked", input: "http://thing.internal/", wantErr: ErrCrawlTargetBlocked},
		{name: "resolves private blocked", input: "http://evil.test/", wantErr: ErrCrawlTargetBlocked},
		{name: "any private answer blocked", input: "http://rebind.test/", wantErr: ErrCrawlTargetBlocked},
		{name: "unresolvable fails", input: "http://missing.test/", wantErr: errTestNoSuchHost},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := guard.NormalizeAndValidateStartURL(context.Background(), tc.input)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveStartURLRedirectsAnchorsToFinalURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.Header().Set("Powered-By", "Shopify")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Location", "http://final.test/page")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()

	guard := &Guard{Resolver: fakeResolver{addrs: map[string][]string{
		"start.test": {"93.184.216.34"},
		"final.test": {"93.184.216.34"},
	}}}
	probe, err := validator{guard: guard, client: dialAllTo(server)}.resolveRedirects(context.Background(), "http://start.test/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if probe.URL != "http://final.test/page" {
		t.Fatalf("URL = %q, want the redirect target", probe.URL)
	}
	if probe.PoweredBy != "Shopify" {
		t.Fatalf("PoweredBy = %q, want Shopify", probe.PoweredBy)
	}
}

func TestResolveStartURLRedirectsRejectsInternalTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://169.254.169.254/latest/meta-data/")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	guard := &Guard{Resolver: fakeResolver{addrs: map[string][]string{"start.test": {"93.184.216.34"}}}}
	_, err := validator{guard: guard, client: dialAllTo(server)}.resolveRedirects(context.Background(), "http://start.test/")
	if !errors.Is(err, ErrCrawlTargetBlocked) {
		t.Fatalf("error = %v, want ErrCrawlTargetBlocked", err)
	}
}

func TestGuardDialContextRefusesPrivateAddress(t *testing.T) {
	guard := &Guard{Resolver: fakeResolver{addrs: map[string][]string{"evil.test": {"10.0.0.5"}}}}
	_, err := guard.DialContext(context.Background(), "tcp", "evil.test:80")
	if !errors.Is(err, ErrCrawlTargetBlocked) {
		t.Fatalf("error = %v, want ErrCrawlTargetBlocked", err)
	}
}

func TestGuardDialContextRefusesBlockedHost(t *testing.T) {
	guard := &Guard{}
	_, err := guard.DialContext(context.Background(), "tcp", "localhost:80")
	if !errors.Is(err, ErrCrawlTargetBlocked) {
		t.Fatalf("error = %v, want ErrCrawlTargetBlocked", err)
	}
}

func TestGuardDialContextRefusesMetadataLiteral(t *testing.T) {
	guard := &Guard{}
	_, err := guard.DialContext(context.Background(), "tcp", "169.254.169.254:80")
	if !errors.Is(err, ErrCrawlTargetBlocked) {
		t.Fatalf("error = %v, want ErrCrawlTargetBlocked", err)
	}
}

func TestGuardClientNeverAutoFollowsRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://169.254.169.254/")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	// The validator's client must never follow a redirect to an internal
	// address on its own; the hop is revalidated here.
	guard := &Guard{Resolver: fakeResolver{addrs: map[string][]string{"start.test": {"93.184.216.34"}}}}
	prober := validator{guard: guard, client: dialAllTo(server)}
	if _, err := prober.resolveRedirects(context.Background(), "http://start.test/"); !errors.Is(err, ErrCrawlTargetBlocked) {
		t.Fatalf("error = %v, want ErrCrawlTargetBlocked", err)
	}
}

func TestIsPrivateIPv6MappedLiteral(t *testing.T) {
	if !isPrivateIPv6("::ffff:c0a8:1") {
		t.Fatal("expected ::ffff:c0a8:1 to be private")
	}
	if isPrivateIPv6("2001:4860:4860::8888") {
		t.Fatal("expected a public IPv6 to be public")
	}
}

func TestNormalizeHostStripsDecorations(t *testing.T) {
	cases := map[string]string{
		"[::1]":        "::1",
		"Example.COM":  "example.com",
		"example.com.": "example.com",
		"fe80::1%eth0": "fe80::1",
	}
	for input, want := range cases {
		if got := normalizeHost(input); got != want {
			t.Errorf("normalizeHost(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseMappedIPv4FromIPv6(t *testing.T) {
	if got, ok := parseMappedIPv4FromIPv6("::ffff:192.168.1.1"); !ok || got != "192.168.1.1" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	if _, ok := parseMappedIPv4FromIPv6("2001:db8::1"); ok {
		t.Fatal("expected no mapped IPv4")
	}
}

func TestGuardPropagatesResolverFailure(t *testing.T) {
	guard := &Guard{Resolver: fakeResolver{err: context.DeadlineExceeded}}
	if _, err := guard.resolveAllowed(context.Background(), "slow.test"); err == nil {
		t.Fatal("expected an error when the resolver fails")
	}
}
