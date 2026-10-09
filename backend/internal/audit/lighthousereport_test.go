package audit

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func testIssue(category, key string, score *int, impactMs, impactBytes *float64) StoredLighthouseIssue {
	return StoredLighthouseIssue{
		Category: category, AuditKey: key, Title: key, Description: "d", Score: score,
		ImpactMs: impactMs, ImpactBytes: impactBytes, Severity: "warning", Items: []string{"i"},
	}
}

func storedPayloadFixture(t *testing.T, issues ...StoredLighthouseIssue) string {
	t.Helper()
	stored := StoredLighthousePayload{
		Version: 2, Source: "dataforseo-lighthouse", HasIssueDetails: true,
		Metadata: StoredLighthouseMetadata{FinalURL: "https://example.com/final", Strategy: "mobile"},
		Issues:   issues,
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func detailFixture(payload string) LighthouseDetail {
	return LighthouseDetail{
		ID: "result-1", Strategy: "mobile", PageURL: "https://example.com/",
		StartedAt: time.Date(2026, 3, 23, 19, 27, 33, 0, time.UTC), PayloadJSON: payload,
	}
}

func issueKeys(issues []StoredLighthouseIssue) string {
	keys := make([]string, 0, len(issues))
	for _, issue := range issues {
		keys = append(keys, issue.AuditKey)
	}
	return strings.Join(keys, ",")
}

func TestBuildLighthouseIssuesSortsByImpactThenScore(t *testing.T) {
	payload := storedPayloadFixture(t,
		testIssue("seo", "low-score", ptr(10), nil, nil),
		testIssue("seo", "high-score", ptr(90), nil, nil),
		testIssue("seo", "no-score", nil, nil, nil),
		testIssue("performance", "big-bytes", ptr(50), nil, ptr(5000.0)),
		testIssue("performance", "slow", ptr(50), ptr(2.0), nil),
	)
	result, err := BuildLighthouseIssues(detailFixture(payload))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got, want := issueKeys(result.Issues), "slow,big-bytes,low-score,high-score,no-score"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if result.FinalURL != "https://example.com/final" || !result.HasIssueDetails || result.Scores == nil {
		t.Fatalf("result = %+v", result)
	}
	if result.CreatedAt != "2026-03-23T19:27:33.000Z" {
		t.Fatalf("createdAt = %s", result.CreatedAt)
	}
}

func TestBuildLighthouseIssuesPayloadShapes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantErr error
	}{
		{"valid json but unknown version", `{"version":1}`, nil},
		{"valid json but other source", `{"version":2,"source":"x"}`, nil},
		{"not json", `not json`, errInvalidPayload},
		{"empty", ``, errInvalidPayload},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := BuildLighthouseIssues(detailFixture(tc.payload))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if result.HasIssueDetails || result.Scores != nil || result.Metrics != nil {
				t.Fatalf("unrecognised payload must carry no details: %+v", result)
			}
			if result.FinalURL != "https://example.com/" || result.Issues == nil || len(result.Issues) != 0 {
				t.Fatalf("fallback result = %+v", result)
			}
		})
	}
}

func TestBuildLighthouseExport(t *testing.T) {
	payload := storedPayloadFixture(t,
		testIssue("performance", "unused-javascript", ptr(40), ptr(5.0), nil),
		testIssue("accessibility", "color-contrast <b>&", ptr(60), nil, nil),
	)
	const base = "lighthouse-mobile-2026-03-23T19-27-33-000Z"
	cases := []struct {
		name         string
		mode         ExportMode
		category     LighthouseCategory
		filename     string
		wantCategory string
		wantKeys     string
	}{
		{"all issues", ExportIssues, "", base + "-issues.json", "all", "unused-javascript,color-contrast <b>&"},
		{"one category", ExportCategory, "accessibility", base + "-accessibility-issues.json", "accessibility", "color-contrast <b>&"},
		{"category mode without category exports all", ExportCategory, "", base + "-issues.json", "all", "unused-javascript,color-contrast <b>&"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := BuildLighthouseExport(detailFixture(payload), tc.mode, tc.category)
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			if file.Filename != tc.filename {
				t.Fatalf("filename = %s, want %s", file.Filename, tc.filename)
			}
			var content struct {
				ResultID string                  `json:"resultId"`
				Category string                  `json:"category"`
				Issues   []StoredLighthouseIssue `json:"issues"`
			}
			if err := json.Unmarshal([]byte(file.Content), &content); err != nil {
				t.Fatalf("content is not JSON: %v", err)
			}
			if content.ResultID != "result-1" || content.Category != tc.wantCategory || issueKeys(content.Issues) != tc.wantKeys {
				t.Fatalf("content = %+v keys = %s", content, issueKeys(content.Issues))
			}
			if strings.Contains(file.Content, `<`) {
				t.Fatal("HTML characters must not be escaped")
			}
			if strings.HasSuffix(file.Content, "\n") || !strings.Contains(file.Content, "\n  \"") {
				t.Fatalf("content must be 2-space indented with no trailing newline: %q", file.Content)
			}
		})
	}
}

func TestBuildLighthouseExportFullReturnsPayloadUntouched(t *testing.T) {
	file, err := BuildLighthouseExport(detailFixture(`not even json`), ExportFull, "")
	if err != nil {
		t.Fatalf("full export must not parse the payload: %v", err)
	}
	if file.Content != "not even json" || !strings.HasSuffix(file.Filename, "-payload.json") {
		t.Fatalf("file = %+v", file)
	}
	if _, err := BuildLighthouseExport(detailFixture(`not even json`), ExportIssues, ""); !errors.Is(err, errInvalidPayload) {
		t.Fatalf("issues export of an invalid payload err = %v", err)
	}
}
