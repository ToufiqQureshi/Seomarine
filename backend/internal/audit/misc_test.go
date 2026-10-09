package audit

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDeterministicAuditRowID(t *testing.T) {
	first := DeterministicAuditRowID("audit-1", "https://example.com/", "missing-title")
	second := DeterministicAuditRowID("audit-1", "https://example.com/", "missing-title")
	if first != second {
		t.Fatal("ids must be deterministic")
	}
	if len(first) != 36 {
		t.Fatalf("id length = %d, want 36", len(first))
	}
	if third := DeterministicAuditRowID("audit-1", "https://example.com/", "broken-page"); third == first {
		t.Fatal("a different input must produce a different id")
	}
	if SHA256Hex("abc") != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatal("unexpected sha256")
	}
}

func TestClassifyAuditError(t *testing.T) {
	cases := []struct {
		message string
		want    ErrorCode
	}{
		{"Worker exceeded memory limit", ErrorOOM},
		{"exceeded CPU time limit", ErrorCPULimit},
		{"WorkflowTimeoutError: boom", ErrorStepTimeout},
		{"Execution timed out", ErrorStepTimeout},
		{"step output is too large", ErrorStepOutputTooLarge},
		{"WorkflowInternalError", ErrorWorkflowInternal},
		{"Failed query: select 1", ErrorDB},
		{"D1_ERROR: no such table", ErrorDB},
		{"Postgres database accessed outside a request scope", ErrorDB},
		{"something else entirely", ErrorUnknown},
	}
	for _, tc := range cases {
		if got := ClassifyAuditError(errors.New(tc.message)).ErrorCode; got != tc.want {
			t.Errorf("ClassifyAuditError(%q) = %q, want %q", tc.message, got, tc.want)
		}
	}
	if ClassifyAuditError(nil).ErrorCode != ErrorUnknown {
		t.Fatal("a nil error classifies as unknown")
	}
}

func TestClassifyAuditErrorTruncatesDetail(t *testing.T) {
	long := strings.Repeat("x", 1000)
	if got := ClassifyAuditError(errors.New(long)); len(got.ErrorDetail) != errorDetailMaxChars {
		t.Fatalf("detail length = %d, want %d", len(got.ErrorDetail), errorDetailMaxChars)
	}
}

func TestParseAuditConfig(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantOK    bool
		wantMax   int
		wantStrat LighthouseStrategy
		wantSite  string
	}{
		{"valid", `{"maxPages":50,"lighthouseStrategy":"auto"}`, true, 50, LighthouseAuto, ""},
		{"retired all maps to auto", `{"maxPages":50,"lighthouseStrategy":"all"}`, true, 50, LighthouseAuto, ""},
		{"retired manual maps to none", `{"maxPages":50,"lighthouseStrategy":"manual"}`, true, 50, LighthouseNone, ""},
		{"unknown strategy falls back to auto", `{"maxPages":50,"lighthouseStrategy":"weird"}`, true, 50, LighthouseAuto, ""},
		{"missing strategy defaults to auto", `{"maxPages":50}`, true, 50, LighthouseAuto, ""},
		{"shopify platform", `{"maxPages":50,"lighthouseStrategy":"auto","sitePlatform":"shopify"}`, true, 50, LighthouseAuto, "shopify"},
		{"unknown platform ignored", `{"maxPages":50,"sitePlatform":"magento"}`, true, 50, LighthouseAuto, ""},
		{"empty is invalid", ``, false, 0, "", ""},
		{"missing maxPages is invalid", `{"lighthouseStrategy":"auto"}`, false, 0, "", ""},
		{"maxPages too small", `{"maxPages":5}`, false, 0, "", ""},
		{"maxPages too large", `{"maxPages":99999}`, false, 0, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config, ok := ParseAuditConfig(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if config.MaxPages != tc.wantMax {
				t.Errorf("maxPages = %d, want %d", config.MaxPages, tc.wantMax)
			}
			if config.LighthouseStrategy != tc.wantStrat {
				t.Errorf("strategy = %q, want %q", config.LighthouseStrategy, tc.wantStrat)
			}
			if config.SitePlatform != tc.wantSite {
				t.Errorf("sitePlatform = %q, want %q", config.SitePlatform, tc.wantSite)
			}
		})
	}
}

func TestClampMaxPagesAndCapacity(t *testing.T) {
	if got := clampMaxPages(0); got != DefaultAuditPages {
		t.Fatalf("got %d", got)
	}
	if got := clampMaxPages(1); got != MinAuditPages {
		t.Fatalf("got %d", got)
	}
	if got := clampMaxPages(999999); got != PaidMaxAuditPages {
		t.Fatalf("got %d", got)
	}
	capacity := getEstimatedCapacity(50, LighthouseAuto)
	if capacity.PagesTotal != 50 || capacity.LighthouseTotal != 20 || capacity.Total != 70 {
		t.Fatalf("capacity = %+v", capacity)
	}
	silent := getEstimatedCapacity(50, LighthouseNone)
	if silent.LighthouseTotal != 0 {
		t.Fatalf("capacity = %+v", silent)
	}
}

func TestNormalizeCrawlerHost(t *testing.T) {
	cases := map[string]string{
		"https://Store.example.com/path": "store.example.com",
		"store.example.com":              "store.example.com",
		"STORE.EXAMPLE.COM.":             "store.example.com",
	}
	for input, want := range cases {
		got, ok := NormalizeCrawlerHost(input)
		if !ok || got != want {
			t.Errorf("NormalizeCrawlerHost(%q) = %q ok=%v, want %q", input, got, ok, want)
		}
	}
	for _, bad := range []string{"", "not a host", "no-dot"} {
		if _, ok := NormalizeCrawlerHost(bad); ok {
			t.Errorf("NormalizeCrawlerHost(%q) should fail", bad)
		}
	}
}

func TestCrawlerHeadersFor(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	access := &CrawlerAccess{Host: "store.example.com", Headers: ShopifyCrawlerHeaders("sig-input", "sig"), ExpiresAt: &expiry}
	if headers := CrawlerHeadersFor("https://store.example.com/page", access); headers == nil {
		t.Fatal("expected headers for the bound host")
	}
	if headers := CrawlerHeadersFor("https://other.example.com/page", access); headers != nil {
		t.Fatal("headers must not leak to another host")
	}
	if headers := CrawlerHeadersFor("http://store.example.com/page", access); headers != nil {
		t.Fatal("headers must not be replayed over plain http")
	}
	expired := &CrawlerAccess{Host: "store.example.com", Headers: ShopifyCrawlerHeaders("s", "s"), ExpiresAt: &expiry}
	past := time.Now().Add(-time.Hour)
	expired.ExpiresAt = &past
	if headers := CrawlerHeadersFor("https://store.example.com/page", expired); headers != nil {
		t.Fatal("an expired signature must not be replayed")
	}
}

func TestParseSignatureExpiry(t *testing.T) {
	expiry := ParseSignatureExpiry(`sig1=("@target-uri");expires=1700000000`)
	if expiry == nil || expiry.Unix() != 1700000000 {
		t.Fatalf("expiry = %v", expiry)
	}
	if ParseSignatureExpiry("no expiry here") != nil {
		t.Fatal("expected no expiry")
	}
}

func TestReadTextUpToAndTruncate(t *testing.T) {
	response := &http.Response{Body: http.NoBody}
	text, err := ReadTextUpTo(response, 10)
	if err != nil || text != "" {
		t.Fatalf("text = %q err = %v", text, err)
	}
	if got := TruncateToBytes("hello", 10); got != "hello" {
		t.Fatalf("got %q", got)
	}
	if got := TruncateToBytes("hello world", 5); got != "hello" {
		t.Fatalf("got %q", got)
	}
	// A multibyte rune split by the cap must be dropped, not corrupted.
	if got := TruncateToBytes("héllo", 2); got != "h" {
		t.Fatalf("got %q", got)
	}
}

func TestComputeIsIndexable(t *testing.T) {
	if !computeIsIndexable("", "") {
		t.Fatal("no directive means indexable")
	}
	if computeIsIndexable("noindex, follow", "") {
		t.Fatal("noindex must win")
	}
	if computeIsIndexable("", "none") {
		t.Fatal("x-robots-tag none must win")
	}
	if !computeIsIndexable("index, follow", "index") {
		t.Fatal("explicit index stays indexable")
	}
}

func TestParseHeaderCanonical(t *testing.T) {
	headers := http.Header{}
	headers.Add("Link", `<https://example.com/canonical>; rel="canonical"`)
	if got := parseHeaderCanonical(headers); got != "https://example.com/canonical" {
		t.Fatalf("got %q", got)
	}
	if got := parseHeaderCanonical(http.Header{}); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestContentHash(t *testing.T) {
	if contentHash("   ") != "" {
		t.Fatal("whitespace-only content has no hash")
	}
	if contentHash("a") == contentHash("b") {
		t.Fatal("different content must hash differently")
	}
	first, second := contentHash("a"), contentHash("a")
	if first != second {
		t.Fatal("the hash must be stable")
	}
}
