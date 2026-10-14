package audit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSelectLighthouseSampleNone(t *testing.T) {
	got := SelectLighthouseSample([]LighthouseSamplePage{{URL: "https://example.com/", StatusCode: 200, FetchClass: FetchOK}}, "https://example.com/", LighthouseNone)
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestSelectLighthouseSampleHomepageAndTemplates(t *testing.T) {
	pages := []LighthouseSamplePage{
		{URL: "https://example.com/", StatusCode: 200, FetchClass: FetchOK},
		{URL: "https://example.com/blog/my-great-post", StatusCode: 200, FetchClass: FetchOK},
		{URL: "https://example.com/blog/another-nice-post", StatusCode: 200, FetchClass: FetchOK},
		{URL: "https://example.com/products/123", StatusCode: 200, FetchClass: FetchOK},
		{URL: "https://example.com/missing", StatusCode: 404, FetchClass: FetchOK},
		{URL: "https://example.com/blocked", StatusCode: 200, FetchClass: FetchBlocked},
	}
	got := SelectLighthouseSample(pages, "https://example.com/", LighthouseAuto)
	want := []string{"https://example.com/", "https://example.com/blog/my-great-post", "https://example.com/products/123"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestSelectLighthouseSampleCapsAtTen(t *testing.T) {
	pages := []LighthouseSamplePage{{URL: "https://example.com/", StatusCode: 200, FetchClass: FetchOK}}
	for i := 0; i < 20; i++ {
		pages = append(pages, LighthouseSamplePage{
			URL: "https://example.com/section" + string(rune('a'+i)) + "/item-1-2", StatusCode: 200, FetchClass: FetchOK,
		})
	}
	got := SelectLighthouseSample(pages, "https://example.com/", LighthouseAuto)
	if len(got) != 10 {
		t.Fatalf("got %d samples, want 10", len(got))
	}
}

func TestSelectLighthouseSampleTrailingSlashTolerance(t *testing.T) {
	pages := []LighthouseSamplePage{{URL: "https://example.com/about/", StatusCode: 200, FetchClass: FetchOK}}
	got := SelectLighthouseSample(pages, "https://example.com/about", LighthouseAuto)
	if len(got) != 1 || got[0] != "https://example.com/about/" {
		t.Fatalf("got %v", got)
	}
}

func TestCanonicalURLKeyWithoutTrailingSlash(t *testing.T) {
	if got := canonicalURLKeyWithoutTrailingSlash("https://example.com/a/"); got != "https://example.com/a" {
		t.Fatalf("got %q", got)
	}
	if got := canonicalURLKeyWithoutTrailingSlash("https://example.com/"); got != "https://example.com/" {
		t.Fatalf("root must stay slash-terminated, got %q", got)
	}
}

const pageSpeedPayload = `{
  "loadingExperience": {
    "metrics": {
      "LARGEST_CONTENTFUL_PAINT_MS": {"percentile": 2100, "category": "FAST", "distributions": [{"proportion": 0.8}, {"proportion": 0.15}, {"proportion": 0.05}]},
      "CUMULATIVE_LAYOUT_SHIFT_SCORE": {"percentile": 5, "category": "FAST", "distributions": [{"proportion": 0.9}, {"proportion": 0.06}, {"proportion": 0.04}]},
      "INTERACTION_TO_NEXT_PAINT": {"percentile": 180, "category": "FAST", "distributions": [{"proportion": 0.85}, {"proportion": 0.1}, {"proportion": 0.05}]}
    },
    "overall_category": "FAST"
  },
  "lighthouseResult": {
    "requestedUrl": "https://example.com/",
    "finalUrl": "https://example.com/",
    "lighthouseVersion": "12.0.0",
    "categories": {
      "performance": {"score": 0.5, "auditRefs": [{"id": "unused-css"}, {"id": "total-blocking-time"}]},
      "seo": {"score": 1.0, "auditRefs": []}
    },
    "audits": {
      "unused-css": {"title": "Unused CSS", "description": "Remove unused CSS", "score": 0.2, "scoreDisplayMode": "numeric-extra", "details": {"overallSavingsBytes": 200000, "items": [{"url": "https://example.com/a.css", "wastedBytes": 1234}]}},
      "total-blocking-time": {"score": 0.8, "numericValue": 1234.5, "scoreDisplayMode": "numeric"},
      "largest-contentful-paint": {"score": 0.9, "numericValue": 2500, "displayValue": "2.5 s", "scoreDisplayMode": "numeric"}
    }
  }
}`

var testInput = LighthouseInput{URL: "https://example.com/", Strategy: "mobile"}

func TestParsePageSpeedPayload(t *testing.T) {
	stored, err := ParsePageSpeedPayload([]byte(pageSpeedPayload), testInput, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Version != 2 || stored.Source != "pagespeed-insights" || stored.Metadata.Strategy != "mobile" {
		t.Fatalf("stored = %+v", stored)
	}
	if stored.Scores.Performance == nil || *stored.Scores.Performance != 50 {
		t.Fatalf("performance = %v", stored.Scores.Performance)
	}
	if stored.Scores.SEO == nil || *stored.Scores.SEO != 100 {
		t.Fatalf("seo = %v", stored.Scores.SEO)
	}
	if stored.Metrics.LargestContentfulPaint.NumericValue == nil || *stored.Metrics.LargestContentfulPaint.NumericValue != 2500 {
		t.Fatalf("lcp = %v", stored.Metrics.LargestContentfulPaint.NumericValue)
	}
	found := false
	for _, issue := range stored.Issues {
		if issue.AuditKey == "unused-css" {
			found = true
			if issue.Severity != "critical" {
				t.Errorf("unused-css severity = %q, want critical", issue.Severity)
			}
			if len(issue.Items) != 1 {
				t.Errorf("items = %v", issue.Items)
			}
		}
		if issue.AuditKey == "total-blocking-time" {
			t.Errorf("numeric audits must be skipped, saw %v", issue.AuditKey)
		}
	}
	if !found {
		t.Fatalf("expected the unused-css issue, got %v", stored.Issues)
	}
	if !stored.HasIssueDetails {
		t.Error("expected hasIssueDetails")
	}
	field := stored.FieldData
	if field == nil || field.Scope != "page" || field.CoreWebVitals != "passed" {
		t.Fatalf("field data = %+v", field)
	}
	keys := []string{}
	for _, metric := range field.Metrics {
		keys = append(keys, metric.Key)
	}
	if strings.Join(keys, ",") != "lcp,inp,cls" {
		t.Fatalf("field metric order = %v", keys)
	}
	if cls := field.Metrics[2]; cls.Percentile != 0.05 || cls.Good != 0.9 || cls.Poor != 0.04 {
		t.Fatalf("cls = %+v, want the CrUX value divided by 100", cls)
	}
}

func TestBuildFieldData(t *testing.T) {
	metric := func(category string) rawFieldMetric {
		percentile := 100.0
		return rawFieldMetric{Percentile: &percentile, Category: category}
	}
	experience := func(metrics map[string]rawFieldMetric) *rawLoadingExperience {
		return &rawLoadingExperience{Metrics: metrics}
	}
	cases := []struct {
		name       string
		page       *rawLoadingExperience
		origin     *rawLoadingExperience
		wantNil    bool
		wantScope  string
		wantStatus string
	}{
		{"no data at all", nil, nil, true, "", ""},
		{"empty page and origin", experience(nil), experience(nil), true, "", ""},
		{"falls back to the origin", experience(nil), experience(map[string]rawFieldMetric{
			"LARGEST_CONTENTFUL_PAINT_MS": metric("FAST"), "CUMULATIVE_LAYOUT_SHIFT_SCORE": metric("FAST"),
		}), false, "origin", "passed"},
		{"slow INP fails", experience(map[string]rawFieldMetric{
			"LARGEST_CONTENTFUL_PAINT_MS": metric("FAST"), "CUMULATIVE_LAYOUT_SHIFT_SCORE": metric("FAST"), "INTERACTION_TO_NEXT_PAINT": metric("SLOW"),
		}), nil, false, "page", "failed"},
		{"average LCP fails", experience(map[string]rawFieldMetric{
			"LARGEST_CONTENTFUL_PAINT_MS": metric("AVERAGE"), "CUMULATIVE_LAYOUT_SHIFT_SCORE": metric("FAST"),
		}), nil, false, "page", "failed"},
		{"missing CLS cannot pass", experience(map[string]rawFieldMetric{
			"LARGEST_CONTENTFUL_PAINT_MS": metric("FAST"),
		}), nil, false, "page", "failed"},
		{"only unknown metrics", experience(map[string]rawFieldMetric{"SOMETHING_NEW": metric("FAST")}), nil, true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildFieldData(tc.page, tc.origin)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil || got.Scope != tc.wantScope || got.CoreWebVitals != tc.wantStatus {
				t.Fatalf("got %+v, want scope %q status %q", got, tc.wantScope, tc.wantStatus)
			}
		})
	}
}

func TestParsePageSpeedPayloadErrors(t *testing.T) {
	cases := map[string]string{
		"invalid JSON":       `not json`,
		"no lighthouse":      `{"loadingExperience":{}}`,
		"no category scores": `{"lighthouseResult":{"categories":{},"audits":{}}}`,
		"runtime error":      `{"lighthouseResult":{"runtimeError":{"code":"NO_FCP","message":"The page did not paint"},"categories":{"seo":{"score":1}}}}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePageSpeedPayload([]byte(payload), testInput, time.Unix(0, 0)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	_, err := ParsePageSpeedPayload([]byte(cases["runtime error"]), testInput, time.Unix(0, 0))
	if !strings.HasPrefix(err.Error(), lighthouseLoadErrorPrefix) || !strings.Contains(err.Error(), "NO_FCP") {
		t.Fatalf("runtime error = %q, want a load error naming the code", err)
	}
	if _, err := ParsePageSpeedPayload([]byte(`{"lighthouseResult":{"runtimeError":{"code":"NO_ERROR"},"categories":{"seo":{"score":1}}}}`), testInput, time.Unix(0, 0)); err != nil {
		t.Fatalf("NO_ERROR is not a failure: %v", err)
	}
}

type fakeQuota struct {
	allow bool
	err   error
	calls int
}

func (q *fakeQuota) Allow(context.Context, string) (bool, error) {
	q.calls++
	return q.allow, q.err
}

func TestPageSpeedProviderFetchLighthouse(t *testing.T) {
	var gotQuery url.Values
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery, gotKey = r.URL.Query(), r.Header.Get("X-Goog-Api-Key")
		_, _ = w.Write([]byte(pageSpeedPayload))
	}))
	defer server.Close()
	provider := &pageSpeedProvider{apiKey: "test-key", endpoint: server.URL, client: server.Client(), quota: &fakeQuota{allow: true}, now: time.Now}

	stored, err := provider.FetchLighthouse(t.Context(), "org-1", LighthouseInput{URL: "https://example.com/a", Strategy: "desktop"})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Source != pageSpeedSource {
		t.Fatalf("source = %q", stored.Source)
	}
	if gotKey != "test-key" || gotQuery.Has("key") {
		t.Fatalf("key header %q, query %v: the key must travel only in the header", gotKey, gotQuery)
	}
	if gotQuery.Get("url") != "https://example.com/a" || gotQuery.Get("strategy") != "DESKTOP" || len(gotQuery["category"]) != 4 {
		t.Fatalf("query = %v", gotQuery)
	}
}

func TestPageSpeedProviderRefusals(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	newProvider := func(quota PageSpeedQuota) *pageSpeedProvider {
		return &pageSpeedProvider{apiKey: "test-key", endpoint: server.URL, client: server.Client(), quota: quota, now: time.Now}
	}

	if _, err := newProvider(&fakeQuota{allow: false}).FetchLighthouse(t.Context(), "org-1", testInput); !errors.Is(err, ErrPageSpeedDailyLimit) {
		t.Fatalf("over the daily share: err = %v, want ErrPageSpeedDailyLimit", err)
	}
	redisDown := errors.New("redis down")
	if _, err := newProvider(&fakeQuota{allow: true, err: redisDown}).FetchLighthouse(t.Context(), "org-1", testInput); !errors.Is(err, redisDown) {
		t.Fatalf("quota error: err = %v, want it returned (fail closed)", err)
	}
	if _, err := newProvider(nil).FetchLighthouse(t.Context(), "org-1", testInput); err == nil {
		t.Fatal("a provider without a quota must refuse")
	}
	if requests != 0 {
		t.Fatalf("%d requests reached Google; refused checks must send none", requests)
	}
}

func TestPageSpeedProviderErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"page did not load", http.StatusInternalServerError, `{"error":{"code":500,"message":"Lighthouse returned error: NO_FCP. The page did not paint any content."}}`, lighthouseLoadErrorPrefix + " NO_FCP."},
		{"quota exhausted", http.StatusTooManyRequests, `{"error":{"code":429,"message":"Quota exceeded for quota metric 'Queries'"}}`, "PageSpeed Insights answered 429: Quota exceeded"},
		{"no JSON body", http.StatusBadGateway, `<html>bad gateway</html>`, "PageSpeed Insights answered 502: Bad Gateway"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			provider := &pageSpeedProvider{apiKey: "test-key", endpoint: server.URL, client: server.Client(), quota: &fakeQuota{allow: true}, now: time.Now}
			_, err := provider.FetchLighthouse(t.Context(), "org-1", testInput)
			if err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("err = %v, want prefix %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "test-key") {
				t.Fatal("the API key must never appear in an error")
			}
		})
	}
}

func TestPageSpeedProviderTransportErrorHidesKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()
	provider := &pageSpeedProvider{apiKey: "test-key", endpoint: endpoint, client: &http.Client{}, quota: &fakeQuota{allow: true}, now: time.Now}
	_, err := provider.FetchLighthouse(t.Context(), "org-1", testInput)
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("err = %v: want an error without the key", err)
	}
}

func TestSelectLighthouseSampleHomepageOnly(t *testing.T) {
	pages := []LighthouseSamplePage{
		{URL: "https://example.com/blog/post", StatusCode: 200, FetchClass: FetchOK},
		{URL: "https://example.com/", StatusCode: 200, FetchClass: FetchOK},
	}
	if got := SelectLighthouseSample(pages, "https://example.com/", LighthouseHomepage); len(got) != 1 || got[0] != "https://example.com/" {
		t.Fatalf("got %v, want the homepage alone", got)
	}
	if got := SelectLighthouseSample(pages[:1], "https://example.com/", LighthouseHomepage); len(got) != 0 {
		t.Fatalf("got %v, want nothing when the homepage was not crawled", got)
	}
}

func TestFetchLighthouseResultDailyLimit(t *testing.T) {
	provider := &pageSpeedProvider{apiKey: "test-key", endpoint: "http://unused.invalid", client: &http.Client{}, quota: &fakeQuota{allow: false}, now: time.Now}
	got := FetchLighthouseResult(t.Context(), provider, nil, "org-1", "https://example.com/", "page-1", "mobile")
	if got.Result.ErrorMessage == nil || !strings.Contains(*got.Result.ErrorMessage, "resets at midnight UTC") || got.PayloadJSON != "" {
		t.Fatalf("got %+v, want a failed check that tells the customer when it resets", got.Result)
	}
}

func TestFailedLighthouseFetch(t *testing.T) {
	got := FailedLighthouseFetch("https://example.com/", "page-1", "mobile", "boom")
	if got.Result.ErrorMessage == nil || *got.Result.ErrorMessage != "boom" {
		t.Fatalf("got %+v", got.Result)
	}
	if got.PayloadJSON != "" {
		t.Fatal("a failure carries no payload")
	}
}
