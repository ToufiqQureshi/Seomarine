package audit

import (
	"testing"
	"time"
)

func TestNullableHelpers(t *testing.T) {
	if nullableInt(0) != nil {
		t.Fatal("a zero status code must stay NULL")
	}
	if got := nullableInt(404); got == nil || *got != 404 {
		t.Fatalf("got %v", got)
	}
	if nullableString("") != nil {
		t.Fatal("an empty string must stay NULL")
	}
	if got := nullableString("x"); got == nil || *got != "x" {
		t.Fatalf("got %v", got)
	}
}

func TestDecodeHelpers(t *testing.T) {
	if got := decodeInts(nil); len(got) != 0 {
		t.Fatalf("ints = %v", got)
	}
	sample := "[1,2,3]"
	if got := decodeInts(&sample); len(got) != 3 {
		t.Fatalf("ints = %v", got)
	}
	bad := "{"
	if got := decodeInts(&bad); len(got) != 0 {
		t.Fatalf("bad ints = %v", got)
	}

	tags := `["en","de"]`
	if got := decodeStrings(&tags); len(got) != 2 {
		t.Fatalf("strings = %v", got)
	}
	if got := decodeStrings(nil); len(got) != 0 {
		t.Fatalf("strings = %v", got)
	}

	images := `[{"src":"/a.png","alt":"a"}]`
	decoded := decodeImages(&images)
	if len(decoded) != 1 || decoded[0].Src != "/a.png" {
		t.Fatalf("images = %v", decoded)
	}
	if got := decodeImages(&bad); len(got) != 0 {
		t.Fatalf("bad images = %v", got)
	}

	details := `{"statusCode":404}`
	parsed := decodeDetails(&details)
	if parsed == nil || parsed["statusCode"] != float64(404) {
		t.Fatalf("details = %v", parsed)
	}
	if decodeDetails(nil) != nil || decodeDetails(&bad) != nil {
		t.Fatal("bad details must decode to nil")
	}
}

func TestDefaultSlices(t *testing.T) {
	if got := defaultIntSlice(nil); got == nil || len(got) != 0 {
		t.Fatalf("ints = %v", got)
	}
	if got := defaultImageSlice(nil); got == nil || len(got) != 0 {
		t.Fatalf("images = %v", got)
	}
	if got := defaultStringSlice(nil); got == nil || len(got) != 0 {
		t.Fatalf("strings = %v", got)
	}
}

func TestPageArgsFlattensLinks(t *testing.T) {
	page := samplePage("page-1", "https://example.com/")
	page.Links = []PageLink{
		{TargetURL: "https://example.com/a", IsInternal: true},
		{TargetURL: "https://other.com/b", IsInternal: false},
		{TargetURL: "https://example.com/c", IsInternal: true},
	}
	args := pageArgs("audit-1", page)
	if len(args) != 35 {
		t.Fatalf("args = %d, want 35", len(args))
	}
	// The internal and external link counts are columns 27 and 28 (1-based).
	if args[26] != 2 {
		t.Errorf("internal link count = %v, want 2", args[26])
	}
	if args[27] != 1 {
		t.Errorf("external link count = %v, want 1", args[27])
	}
	if args[0] != "page-1" || args[1] != "audit-1" {
		t.Errorf("ids = %v/%v", args[0], args[1])
	}
}

func TestMarshalAuditConfigRoundTrip(t *testing.T) {
	config := AuditConfig{MaxPages: 123, LighthouseStrategy: LighthouseNone, RenderJavaScript: true, SitePlatform: "shopify", CrawlerCredentialID: "cred-1"}
	encoded, err := MarshalAuditConfig(config)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, ok := ParseAuditConfig(encoded)
	if !ok || parsed != config {
		t.Fatalf("round trip = %+v ok=%v", parsed, ok)
	}
}

func TestFormatISOMillisOpt(t *testing.T) {
	value := time.Unix(0, 0)
	got := formatISOMillisOpt(&value)
	if got == nil || *got != "1970-01-01T00:00:00.000Z" {
		t.Fatalf("got %v", got)
	}
}

func TestSlimPageFromStatus(t *testing.T) {
	// A nil status code must not panic the duplicate candidate check.
	page := SlimPage{ID: "a", URL: "https://example.com/", FetchClass: FetchOK, IsIndexable: true}
	if isDuplicateCandidate(page) {
		t.Fatal("a page with no status code is not a duplicate candidate")
	}
}
