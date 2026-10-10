package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/ranktracking"
)

func addRankTrackingKeywordsTool() *tool {
	return &tool{Name: "add_rank_tracking_keywords", Title: "Add rank tracking keywords",
		Description:  "Add keywords to a tracker. The mutation uses no credits and does not start a check. Scheduled trackers require user approval of the recurring check estimate because future checks spend credits.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"trackerId":{"type":"string","format":"uuid"},"keywords":{"type":"array","items":{"type":"string","minLength":1,"maxLength":200},"minItems":1,"maxItems":2000},"matchCase":{"type":"boolean"},"maxEstimatedScheduledCheckCredits":{"type":"integer","minimum":1}},"required":["projectId","trackerId","keywords"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"trackerId":{"type":"string"},"requested":{"type":"integer"},"added":{"type":"integer"},"addedIds":{"type":"array","items":{"type":"string"}},"tooLong":{"type":"array","items":{"type":"string"}},"scheduledEstimate":{"type":"object","additionalProperties":true}},"additionalProperties":true}`),
		Annotations:  map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false}, Handler: handleAddRankTrackingKeywords}
}

func removeRankTrackingKeywordsTool() *tool {
	return &tool{Name: "remove_rank_tracking_keywords", Title: "Remove rank tracking keywords",
		Description:  "Stop tracking keywords by trackingKeywordId. Uses no credits and preserves historical snapshots. Missing and repeated IDs are ignored.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"trackerId":{"type":"string","format":"uuid"},"keywordIds":{"type":"array","items":{"type":"string","format":"uuid"},"minItems":1,"maxItems":2000}},"required":["projectId","trackerId","keywordIds"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"trackerId":{"type":"string"},"requested":{"type":"integer"},"removed":{"type":"integer"},"removedIds":{"type":"array","items":{"type":"string"}}},"additionalProperties":true}`),
		Annotations:  map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": true}, Handler: handleRemoveRankTrackingKeywords}
}

func handleAddRankTrackingKeywords(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a struct {
		ProjectID                         string   `json:"projectId"`
		TrackerID                         string   `json:"trackerId"`
		Keywords                          []string `json:"keywords"`
		MatchCase                         bool     `json:"matchCase"`
		MaxEstimatedScheduledCheckCredits *int     `json:"maxEstimatedScheduledCheckCredits"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || strings.TrimSpace(a.ProjectID) == "" || !validTrackerID(a.TrackerID) || len(a.Keywords) == 0 || len(a.Keywords) > 2000 {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId, a valid trackerId, and 1 to 2000 keywords are required")
	}
	for _, keyword := range a.Keywords {
		if strings.TrimSpace(keyword) == "" {
			return nil, newAppErrorf("VALIDATION_ERROR", "Keywords must not be empty")
		}
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	s := env.deps.RankTracking
	if s == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Rank tracking is not available on this server.")
	}
	estimate, err := s.EstimateCost(ctx, access.Project.ID, a.TrackerID, a.Keywords)
	if err != nil {
		return nil, safeRankTrackingError(ctx, env, "estimate rank keyword cost", a.TrackerID, err)
	}
	if scheduled := estimate.ScheduledEstimate; scheduled != nil && (a.MaxEstimatedScheduledCheckCredits == nil || *a.MaxEstimatedScheduledCheckCredits < scheduled.CostCredits) {
		return nil, newAppErrorf("APPROVAL_REQUIRED", fmt.Sprintf("Adding these keywords commits future checks estimated at %d credits each (~%d credits/month). Get explicit user approval, then retry with maxEstimatedScheduledCheckCredits at least %d. Live fallback may add separately billed credits.", scheduled.CostCredits, scheduled.MonthlyCostCredits, scheduled.CostCredits))
	}
	result, err := s.AddKeywords(ctx, access.Project.ID, a.TrackerID, a.Keywords, a.MatchCase)
	if err != nil {
		return nil, safeRankTrackingError(ctx, env, "add rank tracking keywords", a.TrackerID, err)
	}
	structured := map[string]any{"trackerId": a.TrackerID, "requested": len(a.Keywords), "added": result.Added, "addedIds": result.AddedIDs, "tooLong": result.TooLong}
	message := fmt.Sprintf("Added %d of %d requested keywords to tracker %s. No check was started and no credits were used.", result.Added, len(a.Keywords), a.TrackerID)
	if e := estimate.ScheduledEstimate; e != nil {
		structured["scheduledEstimate"] = map[string]any{"scheduleInterval": e.Interval, "costUsd": e.CostUSD, "costCredits": e.CostCredits, "checksPerMonth": e.ChecksPerMonth, "monthlyCostUsd": e.MonthlyCostUSD, "monthlyCostCredits": e.MonthlyCostCredits}
		message += fmt.Sprintf(" Future %s checks are estimated at %d credits each (~%d credits/month); live fallback may add separately billed credits.", e.Interval, e.CostCredits, e.MonthlyCostCredits)
	}
	return mcpResponse(message, structured, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/rank-tracking/"+a.TrackerID, nil)}), nil
}

func handleRemoveRankTrackingKeywords(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var a struct {
		ProjectID  string   `json:"projectId"`
		TrackerID  string   `json:"trackerId"`
		KeywordIDs []string `json:"keywordIds"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || strings.TrimSpace(a.ProjectID) == "" || !validTrackerID(a.TrackerID) || len(a.KeywordIDs) == 0 || len(a.KeywordIDs) > 2000 {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId, a valid trackerId, and 1 to 2000 keywordIds are required")
	}
	for _, id := range a.KeywordIDs {
		if !validTrackerID(id) {
			return nil, newAppErrorf("VALIDATION_ERROR", "Every keywordId must be a valid id")
		}
	}
	access, err := env.h.authorizeProject(ctx, env.auth, a.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.RankTracking == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Rank tracking is not available on this server.")
	}
	removed, err := env.deps.RankTracking.RemoveKeywords(ctx, access.Project.ID, a.TrackerID, a.KeywordIDs)
	if err != nil {
		return nil, safeRankTrackingError(ctx, env, "remove rank tracking keywords", a.TrackerID, err)
	}
	message := fmt.Sprintf("Removed %d of %d requested keyword IDs from tracker %s. Historical snapshots were preserved.", len(removed), len(a.KeywordIDs), a.TrackerID)
	return mcpResponse(message, map[string]any{"trackerId": a.TrackerID, "requested": len(a.KeywordIDs), "removed": len(removed), "removedIds": removed}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/rank-tracking/"+a.TrackerID, nil)}), nil
}

func safeRankTrackingError(ctx context.Context, env *callEnv, op, trackerID string, err error) error {
	if errors.Is(err, ranktracking.ErrNotFound) {
		return newAppError("NOT_FOUND")
	}
	var validation ranktracking.ValidationError
	if errors.As(err, &validation) {
		return newAppErrorf("VALIDATION_ERROR", validation.Error())
	}
	env.deps.Logger.ErrorContext(ctx, op, "tracker_id", trackerID, "err", err)
	return newAppErrorf("INTERNAL_ERROR", "Unable to update rank tracking keywords.")
}
