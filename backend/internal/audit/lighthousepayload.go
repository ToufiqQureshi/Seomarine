package audit

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// LighthouseCategories are the four Lighthouse category keys, mirroring
// LIGHTHOUSE_CATEGORIES.
var LighthouseCategories = []string{"performance", "accessibility", "best-practices", "seo"}

// StoredLighthouseMetric is one Lighthouse metric with its score and value.
type StoredLighthouseMetric struct {
	Score        *int     `json:"score"`
	DisplayValue *string  `json:"displayValue"`
	NumericValue *float64 `json:"numericValue"`
}

// StoredLighthouseMetrics is the compact set of metrics the UI shows.
type StoredLighthouseMetrics struct {
	FirstContentfulPaint   StoredLighthouseMetric `json:"firstContentfulPaint"`
	LargestContentfulPaint StoredLighthouseMetric `json:"largestContentfulPaint"`
	TotalBlockingTime      StoredLighthouseMetric `json:"totalBlockingTime"`
	CumulativeLayoutShift  StoredLighthouseMetric `json:"cumulativeLayoutShift"`
	SpeedIndex             StoredLighthouseMetric `json:"speedIndex"`
	TimeToInteractive      StoredLighthouseMetric `json:"timeToInteractive"`
	InteractionToNextPaint StoredLighthouseMetric `json:"interactionToNextPaint"`
	ServerResponseTime     StoredLighthouseMetric `json:"serverResponseTime"`
}

// StoredLighthouseIssue is one failing Lighthouse audit, compacted for storage.
type StoredLighthouseIssue struct {
	Category         string   `json:"category"`
	AuditKey         string   `json:"auditKey"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Score            *int     `json:"score"`
	ScoreDisplayMode *string  `json:"scoreDisplayMode"`
	DisplayValue     *string  `json:"displayValue"`
	ImpactMs         *float64 `json:"impactMs"`
	ImpactBytes      *float64 `json:"impactBytes"`
	Severity         string   `json:"severity"`
	Items            []string `json:"items"`
}

// StoredLighthouseScores are the category scores as percentages.
type StoredLighthouseScores struct {
	Performance   *int `json:"performance"`
	Accessibility *int `json:"accessibility"`
	BestPractices *int `json:"best-practices"`
	SEO           *int `json:"seo"`
}

// StoredLighthouseMetadata identifies a stored Lighthouse report.
type StoredLighthouseMetadata struct {
	RequestedURL      string   `json:"requestedUrl"`
	FinalURL          string   `json:"finalUrl"`
	Strategy          string   `json:"strategy"`
	FetchedAt         string   `json:"fetchedAt"`
	LighthouseVersion *string  `json:"lighthouseVersion"`
	TaskID            *string  `json:"taskId"`
	Cost              *float64 `json:"cost"`
}

// StoredLighthousePayload is the compact Lighthouse report stored and served.
type StoredLighthousePayload struct {
	Version         int                      `json:"version"`
	Source          string                   `json:"source"`
	HasIssueDetails bool                     `json:"hasIssueDetails"`
	Metadata        StoredLighthouseMetadata `json:"metadata"`
	Scores          StoredLighthouseScores   `json:"scores"`
	Metrics         StoredLighthouseMetrics  `json:"metrics"`
	Issues          []StoredLighthouseIssue  `json:"issues"`
}

// scoreToPercent converts a Lighthouse 0-1 score to a rounded percentage.
func scoreToPercent(score *float64) *int {
	if score == nil || math.IsNaN(*score) {
		return nil
	}
	value := int(math.Round(*score * 100))
	return &value
}

// rawLighthouseAudit is the provider's audit shape, decoded loosely.
type rawLighthouseAudit struct {
	Title            *string  `json:"title"`
	Description      *string  `json:"description"`
	Score            *float64 `json:"score"`
	ScoreDisplayMode *string  `json:"scoreDisplayMode"`
	DisplayValue     *string  `json:"displayValue"`
	NumericValue     *float64 `json:"numericValue"`
	Details          *struct {
		OverallSavingsMs    *float64        `json:"overallSavingsMs"`
		OverallSavingsBytes *float64        `json:"overallSavingsBytes"`
		Items               json.RawMessage `json:"items"`
	} `json:"details"`
}

// rawLighthouseCategory is the provider's category shape.
type rawLighthouseCategory struct {
	Score     *float64 `json:"score"`
	AuditRefs []struct {
		ID *string `json:"id"`
	} `json:"auditRefs"`
}

// diagnosticAuditKeys are audits that describe the environment rather than
// actionable problems.
var diagnosticAuditKeys = map[string]struct{}{
	"largest-contentful-paint-element": {}, "layout-shifts": {}, "diagnostics": {},
	"metrics": {}, "network-requests": {}, "network-rtt": {}, "network-server-latency": {},
	"main-thread-tasks": {}, "screenshot-thumbnails": {}, "final-screenshot": {},
	"script-treemap-data": {}, "resource-summary": {},
}

// buildStoredMetric compacts one audit into a stored metric.
func buildStoredMetric(audit *rawLighthouseAudit) StoredLighthouseMetric {
	metric := StoredLighthouseMetric{}
	if audit == nil {
		return metric
	}
	metric.Score = scoreToPercent(audit.Score)
	metric.DisplayValue = audit.DisplayValue
	if audit.NumericValue != nil && !math.IsNaN(*audit.NumericValue) {
		metric.NumericValue = audit.NumericValue
	}
	return metric
}

// buildStoredLighthouseMetrics maps the metric audits by their well-known keys.
func buildStoredLighthouseMetrics(audits map[string]rawLighthouseAudit) StoredLighthouseMetrics {
	return StoredLighthouseMetrics{
		FirstContentfulPaint:   buildStoredMetric(lookupAudit(audits, "first-contentful-paint")),
		LargestContentfulPaint: buildStoredMetric(lookupAudit(audits, "largest-contentful-paint")),
		TotalBlockingTime:      buildStoredMetric(lookupAudit(audits, "total-blocking-time")),
		CumulativeLayoutShift:  buildStoredMetric(lookupAudit(audits, "cumulative-layout-shift")),
		SpeedIndex:             buildStoredMetric(lookupAudit(audits, "speed-index")),
		TimeToInteractive:      buildStoredMetric(lookupAudit(audits, "interactive")),
		InteractionToNextPaint: buildStoredMetric(lookupAudit(audits, "interaction-to-next-paint")),
		ServerResponseTime:     buildStoredMetric(lookupAudit(audits, "server-response-time")),
	}
}

func lookupAudit(audits map[string]rawLighthouseAudit, key string) *rawLighthouseAudit {
	audit, ok := audits[key]
	if !ok {
		return nil
	}
	return &audit
}

// lightHouseSeverity grades an issue by its savings and score.
func lightHouseSeverity(score *int, impactMs, impactBytes *float64) string {
	savingsMs := derefFloat(impactMs)
	savingsBytes := derefFloat(impactBytes)
	if savingsMs >= 300 || savingsBytes >= 150_000 {
		return "critical"
	}
	if score != nil && *score < 50 {
		return "critical"
	}
	if savingsMs >= 100 || savingsBytes >= 50_000 {
		return "warning"
	}
	if score != nil && *score < 90 {
		return "warning"
	}
	return "info"
}

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

// buildStoredLighthouseIssues walks each category's audit refs and keeps the
// failing, actionable audits. Mirrors buildStoredLighthouseIssues.
func buildStoredLighthouseIssues(audits map[string]rawLighthouseAudit, categories map[string]rawLighthouseCategory) (bool, []StoredLighthouseIssue) {
	hasIssueDetails := false
	for _, category := range LighthouseCategories {
		if len(categories[category].AuditRefs) > 0 {
			hasIssueDetails = true
			break
		}
	}

	issues := []StoredLighthouseIssue{}
	for _, category := range LighthouseCategories {
		for _, ref := range categories[category].AuditRefs {
			if ref.ID == nil || *ref.ID == "" {
				continue
			}
			auditKey := *ref.ID
			audit, ok := audits[auditKey]
			if !ok {
				continue
			}
			score := scoreToPercent(audit.Score)
			var scoreDisplayMode *string
			if audit.ScoreDisplayMode != nil {
				mode := *audit.ScoreDisplayMode
				scoreDisplayMode = &mode
			}
			if scoreDisplayMode != nil && *scoreDisplayMode == "numeric" {
				continue
			}
			if _, isDiagnostic := diagnosticAuditKeys[auditKey]; isDiagnostic {
				continue
			}
			if isPassingAudit(score, scoreDisplayMode) {
				continue
			}

			var impactMs, impactBytes *float64
			if audit.Details != nil {
				impactMs = audit.Details.OverallSavingsMs
				impactBytes = audit.Details.OverallSavingsBytes
			}
			items := compactAuditItems(audit)

			title := auditKey
			if audit.Title != nil {
				title = *audit.Title
			}
			description := ""
			if audit.Description != nil {
				description = *audit.Description
			}
			issues = append(issues, StoredLighthouseIssue{
				Category:         category,
				AuditKey:         auditKey,
				Title:            title,
				Description:      description,
				Score:            score,
				ScoreDisplayMode: scoreDisplayMode,
				DisplayValue:     audit.DisplayValue,
				ImpactMs:         impactMs,
				ImpactBytes:      impactBytes,
				Severity:         lightHouseSeverity(score, impactMs, impactBytes),
				Items:            items,
			})
		}
	}
	return hasIssueDetails, issues
}

// isPassingAudit reports whether an audit should not be reported as an issue.
func isPassingAudit(score *int, scoreDisplayMode *string) bool {
	if score == nil || *score >= 90 {
		return true
	}
	if scoreDisplayMode == nil {
		return false
	}
	switch *scoreDisplayMode {
	case "notApplicable", "informative", "manual", "error":
		return true
	}
	return false
}

// compactAuditItems renders up to ten audit detail items as compact JSON.
func compactAuditItems(audit rawLighthouseAudit) []string {
	if audit.Details == nil || len(audit.Details.Items) == 0 {
		return []string{}
	}
	raw := strings.TrimSpace(string(audit.Details.Items))
	items := []string{}
	switch {
	case strings.HasPrefix(raw, "["):
		var list []json.RawMessage
		if err := json.Unmarshal(audit.Details.Items, &list); err != nil {
			return items
		}
		for _, entry := range list {
			if len(items) == 10 {
				break
			}
			if compact, ok := compactItem(entry); ok {
				items = append(items, compact)
			}
		}
	case strings.HasPrefix(raw, "{"):
		if compact, ok := compactItem(audit.Details.Items); ok {
			items = append(items, compact)
		}
	}
	return items
}

// preferredItemKeys are the item fields worth keeping.
var preferredItemKeys = []string{"url", "source", "nodeLabel", "snippet", "totalBytes", "wastedBytes", "wastedMs", "label", "value"}

// compactItem renders one audit detail item as a small JSON object.
func compactItem(raw json.RawMessage) (string, bool) {
	entries, err := decodeOrderedObject(raw)
	if err != nil {
		return "", false
	}
	present := map[string]json.RawMessage{}
	for _, entry := range entries {
		present[entry.key] = entry.value
	}
	output := make([]orderedEntry, 0, len(preferredItemKeys))
	for _, key := range preferredItemKeys {
		if value, ok := present[key]; ok && string(value) != "null" {
			output = append(output, orderedEntry{key: key, value: value})
		}
	}
	if len(output) == 0 {
		limit := min(len(entries), 6)
		output = append(output, entries[:limit]...)
	}
	encoded, err := encodeOrderedObject(output)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

// orderedEntry is one key/value pair of a JSON object in document order.
type orderedEntry struct {
	key   string
	value json.RawMessage
}

// decodeOrderedObject decodes a JSON object preserving key order.
func decodeOrderedObject(raw json.RawMessage) ([]orderedEntry, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("not a JSON object")
	}
	entries := []orderedEntry{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("invalid object key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		entries = append(entries, orderedEntry{key: key, value: value})
	}
	return entries, nil
}

// encodeOrderedObject encodes ordered key/value pairs as a JSON object.
func encodeOrderedObject(entries []orderedEntry) ([]byte, error) {
	var builder strings.Builder
	builder.WriteByte('{')
	for i, entry := range entries {
		if i > 0 {
			builder.WriteByte(',')
		}
		key, err := json.Marshal(entry.key)
		if err != nil {
			return nil, err
		}
		builder.Write(key)
		builder.WriteByte(':')
		builder.Write(entry.value)
	}
	builder.WriteByte('}')
	return []byte(builder.String()), nil
}

// LighthouseInput identifies one requested Lighthouse check.
type LighthouseInput struct {
	URL      string
	Strategy string
}

// ParseDataForseoLighthousePayload reduces a DataForSEO Lighthouse response into
// the compact stored payload, failing when the provider returned no usable
// report. Mirrors parseDataforseoLighthousePayload.
func ParseDataForseoLighthousePayload(payload []byte, input LighthouseInput, now time.Time) (StoredLighthousePayload, error) {
	var envelope struct {
		StatusCode    *int   `json:"status_code"`
		StatusMessage string `json:"status_message"`
		Tasks         []struct {
			ID            *string  `json:"id"`
			Cost          *float64 `json:"cost"`
			StatusCode    *int     `json:"status_code"`
			StatusMessage string   `json:"status_message"`
			Result        []struct {
				RequestedURL      *string                          `json:"requestedUrl"`
				FinalURL          *string                          `json:"finalUrl"`
				LighthouseVersion *string                          `json:"lighthouseVersion"`
				Categories        map[string]rawLighthouseCategory `json:"categories"`
				Audits            map[string]rawLighthouseAudit    `json:"audits"`
			} `json:"result"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return StoredLighthousePayload{}, fmt.Errorf("DataForSEO Lighthouse returned an invalid response: %w", err)
	}
	if envelope.StatusCode == nil || *envelope.StatusCode != 20000 {
		return StoredLighthousePayload{}, fmt.Errorf("%s", orDefault(envelope.StatusMessage, "DataForSEO Lighthouse request failed"))
	}
	if len(envelope.Tasks) == 0 {
		return StoredLighthousePayload{}, fmt.Errorf("DataForSEO Lighthouse response missing task")
	}
	task := envelope.Tasks[0]
	if task.StatusCode == nil || *task.StatusCode != 20000 {
		return StoredLighthousePayload{}, fmt.Errorf("%s", orDefault(task.StatusMessage, "DataForSEO Lighthouse task failed"))
	}
	if len(task.Result) == 0 {
		return StoredLighthousePayload{}, fmt.Errorf("DataForSEO Lighthouse response missing result")
	}
	result := task.Result[0]

	categories := result.Categories
	if categories == nil {
		categories = map[string]rawLighthouseCategory{}
	}
	audits := result.Audits
	if audits == nil {
		audits = map[string]rawLighthouseAudit{}
	}
	hasIssueDetails, issues := buildStoredLighthouseIssues(audits, categories)
	metrics := buildStoredLighthouseMetrics(audits)

	finalURL := orDefault(derefString(result.FinalURL), input.URL)
	requestedURL := orDefault(derefString(result.RequestedURL), input.URL)
	stored := StoredLighthousePayload{
		Version:         2,
		Source:          "dataforseo-lighthouse",
		HasIssueDetails: hasIssueDetails,
		Metadata: StoredLighthouseMetadata{
			RequestedURL:      requestedURL,
			FinalURL:          finalURL,
			Strategy:          input.Strategy,
			FetchedAt:         now.UTC().Format(time.RFC3339Nano),
			LighthouseVersion: result.LighthouseVersion,
			TaskID:            task.ID,
			Cost:              task.Cost,
		},
		Scores: StoredLighthouseScores{
			Performance:   scoreToPercent(categoryScore(categories, "performance")),
			Accessibility: scoreToPercent(categoryScore(categories, "accessibility")),
			BestPractices: scoreToPercent(categoryScore(categories, "best-practices")),
			SEO:           scoreToPercent(categoryScore(categories, "seo")),
		},
		Metrics: metrics,
		Issues:  issues,
	}
	if stored.Scores.Performance == nil && stored.Scores.Accessibility == nil &&
		stored.Scores.BestPractices == nil && stored.Scores.SEO == nil {
		return StoredLighthousePayload{}, fmt.Errorf("DataForSEO Lighthouse returned no category scores for %s", finalURL)
	}
	return stored, nil
}

func categoryScore(categories map[string]rawLighthouseCategory, key string) *float64 {
	category, ok := categories[key]
	if !ok {
		return nil
	}
	return category.Score
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
