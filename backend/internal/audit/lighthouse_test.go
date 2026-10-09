package audit

import (
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

const lighthousePayload = `{
  "status_code": 20000,
  "tasks": [{
    "id": "task-1",
    "cost": 0.01,
    "status_code": 20000,
    "result": [{
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
    }]
  }]
}`

func TestParseDataForseoLighthousePayload(t *testing.T) {
	stored, err := ParseDataForseoLighthousePayload([]byte(lighthousePayload), LighthouseInput{URL: "https://example.com/", Strategy: "mobile"}, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Version != 2 || stored.Source != "dataforseo-lighthouse" {
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
}

func TestParseDataForseoLighthousePayloadErrors(t *testing.T) {
	if _, err := ParseDataForseoLighthousePayload([]byte(`{"status_code":40200,"status_message":"Payment Required."}`), LighthouseInput{URL: "https://example.com/", Strategy: "mobile"}, time.Unix(0, 0)); err == nil {
		t.Fatal("expected a provider failure to error")
	}
	noScores := `{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"categories":{},"audits":{}}]}]}`
	if _, err := ParseDataForseoLighthousePayload([]byte(noScores), LighthouseInput{URL: "https://example.com/", Strategy: "mobile"}, time.Unix(0, 0)); err == nil {
		t.Fatal("expected a payload with no scores to error")
	}
	if _, err := ParseDataForseoLighthousePayload([]byte(`not json`), LighthouseInput{URL: "https://example.com/", Strategy: "mobile"}, time.Unix(0, 0)); err == nil {
		t.Fatal("expected invalid JSON to error")
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
