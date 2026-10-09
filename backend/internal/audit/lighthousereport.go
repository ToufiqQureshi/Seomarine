package audit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	storedPayloadVersion = 2
	storedPayloadSource  = "dataforseo-lighthouse"
)

// ExportMode selects what a Lighthouse export contains.
type ExportMode string

// The supported Lighthouse export modes.
const (
	ExportFull     ExportMode = "full"
	ExportIssues   ExportMode = "issues"
	ExportCategory ExportMode = "category"
)

// LighthouseCategory is one Lighthouse report category.
type LighthouseCategory string

// IsValid reports whether c is one of the four Lighthouse categories.
func (c LighthouseCategory) IsValid() bool {
	switch c {
	case "performance", "accessibility", "best-practices", "seo":
		return true
	}
	return false
}

// errInvalidPayload marks a stored payload that is not valid JSON.
var errInvalidPayload = errors.New("invalid Lighthouse payload JSON")

// LighthouseDetail is one Lighthouse result with the context an issue view needs.
type LighthouseDetail struct {
	ID          string
	Strategy    string
	PageURL     string
	StartedAt   time.Time
	PayloadJSON string
}

// LighthouseIssuesResult is the issue view of one stored Lighthouse result.
type LighthouseIssuesResult struct {
	ID              string                   `json:"id"`
	FinalURL        string                   `json:"finalUrl"`
	Strategy        string                   `json:"strategy"`
	CreatedAt       string                   `json:"createdAt"`
	HasIssueDetails bool                     `json:"hasIssueDetails"`
	Scores          *StoredLighthouseScores  `json:"scores"`
	Metrics         *StoredLighthouseMetrics `json:"metrics"`
	Issues          []StoredLighthouseIssue  `json:"issues"`
}

// LighthouseExportFile is a downloadable Lighthouse export.
type LighthouseExportFile struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// parseStoredPayload decodes a stored payload. It returns nil when the JSON is
// valid but not a payload this version understands, and an error when the text
// is not JSON at all.
func parseStoredPayload(payloadJSON string) (*StoredLighthousePayload, error) {
	var stored StoredLighthousePayload
	if err := json.Unmarshal([]byte(payloadJSON), &stored); err == nil &&
		stored.Version == storedPayloadVersion && stored.Source == storedPayloadSource {
		return &stored, nil
	}
	if !json.Valid([]byte(payloadJSON)) {
		return nil, errInvalidPayload
	}
	return nil, nil
}

// reportIssues returns the payload's issues, optionally for one category, most
// impactful first (impact in ms, then bytes, then lower score first).
func reportIssues(stored *StoredLighthousePayload, category LighthouseCategory) []StoredLighthouseIssue {
	issues := []StoredLighthouseIssue{}
	if stored == nil {
		return issues
	}
	for _, issue := range stored.Issues {
		if category == "" || LighthouseCategory(issue.Category) == category {
			if issue.Items == nil {
				issue.Items = []string{}
			}
			issues = append(issues, issue)
		}
	}
	slices.SortStableFunc(issues, func(a, b StoredLighthouseIssue) int {
		if byImpact := impactRank(b) - impactRank(a); byImpact != 0 {
			if byImpact > 0 {
				return 1
			}
			return -1
		}
		return scoreOrTop(a.Score) - scoreOrTop(b.Score)
	})
	return issues
}

func impactRank(issue StoredLighthouseIssue) float64 {
	return derefFloat(issue.ImpactMs)*1000 + derefFloat(issue.ImpactBytes)
}

// scoreOrTop treats a missing score as a perfect one so it sorts last.
func scoreOrTop(score *int) int {
	if score == nil {
		return 100
	}
	return *score
}

// BuildLighthouseIssues builds the issue view of a stored Lighthouse result.
func BuildLighthouseIssues(detail LighthouseDetail) (LighthouseIssuesResult, error) {
	stored, err := parseStoredPayload(detail.PayloadJSON)
	if err != nil {
		return LighthouseIssuesResult{}, err
	}
	result := LighthouseIssuesResult{
		ID: detail.ID, FinalURL: detail.PageURL, Strategy: detail.Strategy,
		CreatedAt: formatISO(detail.StartedAt), Issues: reportIssues(stored, ""),
	}
	if stored != nil {
		result.FinalURL = stored.Metadata.FinalURL
		result.HasIssueDetails = stored.HasIssueDetails
		result.Scores = &stored.Scores
		result.Metrics = &stored.Metrics
	}
	return result, nil
}

// BuildLighthouseExport builds an export file for a stored Lighthouse result.
func BuildLighthouseExport(detail LighthouseDetail, mode ExportMode, category LighthouseCategory) (LighthouseExportFile, error) {
	createdAt := formatISO(detail.StartedAt)
	base := fmt.Sprintf("lighthouse-%s-%s", detail.Strategy, strings.NewReplacer(":", "-", ".", "-").Replace(createdAt))
	if mode == ExportFull {
		return LighthouseExportFile{Filename: base + "-payload.json", Content: detail.PayloadJSON}, nil
	}
	stored, err := parseStoredPayload(detail.PayloadJSON)
	if err != nil {
		return LighthouseExportFile{}, err
	}
	exportCategory := LighthouseCategory("")
	filename := base + "-issues.json"
	if mode == ExportCategory && category != "" {
		exportCategory = category
		filename = fmt.Sprintf("%s-%s-issues.json", base, category)
	}
	label := string(exportCategory)
	if label == "" {
		label = "all"
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(struct {
		ResultID  string                  `json:"resultId"`
		FinalURL  string                  `json:"finalUrl"`
		Strategy  string                  `json:"strategy"`
		CreatedAt string                  `json:"createdAt"`
		Category  string                  `json:"category"`
		Issues    []StoredLighthouseIssue `json:"issues"`
	}{detail.ID, detail.PageURL, detail.Strategy, createdAt, label, reportIssues(stored, exportCategory)})
	if err != nil {
		return LighthouseExportFile{}, fmt.Errorf("encode Lighthouse export: %w", err)
	}
	return LighthouseExportFile{Filename: filename, Content: strings.TrimSuffix(buffer.String(), "\n")}, nil
}
