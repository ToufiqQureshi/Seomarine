package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/ranktracking"
)

func estimateRankTrackerCostTool() *tool {
	return &tool{
		Name: "estimate_rank_tracker_cost", Title: "Estimate rank tracker cost",
		Description: "Estimate rank tracker cost without spending credits or starting a check. The live estimate covers one explicit run_rank_tracker check. Scheduled trackers also include recurring queued costs. Estimates are not hard spend caps; failed queued tasks can incur separately billed live fallback.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"trackerId":{"type":"string","format":"uuid"},"additionalKeywordCount":{"type":"integer","minimum":0,"maximum":1000},"additionalKeywords":{"type":"array","items":{"type":"string"},"maxItems":1000}},"required":["projectId","trackerId"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"trackerId":{"type":"string"},"costUsd":{"type":"number"},"costCredits":{"type":"integer"},"keywordCount":{"type":"integer"},"devicesCount":{"type":"integer"},"totalChecks":{"type":"integer"},"method":{"type":"string","const":"live"},"existingKeywordCount":{"type":"integer"},"additionalKeywordCount":{"type":"integer"},"scheduledEstimate":{"type":"object","additionalProperties":true}},"required":["trackerId","costUsd","costCredits","keywordCount","devicesCount","totalChecks","method","existingKeywordCount","additionalKeywordCount"],"additionalProperties":true}`),
		Annotations: readOnlyAnnotations(),
		Handler: handleEstimateRankTrackerCost,
	}
}

func handleEstimateRankTrackerCost(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string `json:"projectId"`
		TrackerID string `json:"trackerId"`
		AdditionalKeywordCount *int `json:"additionalKeywordCount"`
		AdditionalKeywords []string `json:"additionalKeywords"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" || !validTrackerID(args.TrackerID) {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId and a valid trackerId are required")
	}
	if args.AdditionalKeywordCount != nil && (*args.AdditionalKeywordCount < 0 || *args.AdditionalKeywordCount > 1000) {
		return nil, newAppErrorf("VALIDATION_ERROR", "additionalKeywordCount must be between 0 and 1000")
	}
	if len(args.AdditionalKeywords) > 1000 {
		return nil, newAppErrorf("VALIDATION_ERROR", "additionalKeywords must contain at most 1000 items")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }
	service := env.deps.RankTracking
	if service == nil { return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Rank tracking is not available on this server.") }
	additional := args.AdditionalKeywords
	if additional == nil && args.AdditionalKeywordCount != nil {
		additional = make([]string, *args.AdditionalKeywordCount)
		for i := range additional { additional[i] = "" }
	}
	estimate, err := service.EstimateCost(ctx, access.Project.ID, args.TrackerID, additional)
	if err != nil {
		if errors.Is(err, ranktracking.ErrNotFound) { return nil, newAppError("NOT_FOUND") }
		env.deps.Logger.ErrorContext(ctx, "MCP estimate rank tracker cost", "project_id", access.Project.ID, "tracker_id", args.TrackerID, "err", err)
		return nil, newAppErrorf("INTERNAL_ERROR", "Unable to estimate rank tracker cost.")
	}
	payload := map[string]any{
		"trackerId": args.TrackerID, "costUsd": estimate.CostUSD, "costCredits": estimate.CostCredits,
		"keywordCount": estimate.KeywordCount, "devicesCount": estimate.DevicesCount,
		"totalChecks": estimate.TotalChecks, "method": estimate.Method,
		"existingKeywordCount": estimate.ExistingKeywordCount, "additionalKeywordCount": estimate.AdditionalKeywordCount,
	}
	if scheduled := estimate.ScheduledEstimate; scheduled != nil {
		payload["scheduledEstimate"] = map[string]any{
			"scheduleInterval": scheduled.Interval, "costUsd": scheduled.CostUSD,
			"costCredits": scheduled.CostCredits, "checksPerMonth": scheduled.ChecksPerMonth,
			"monthlyCostUsd": scheduled.MonthlyCostUSD, "monthlyCostCredits": scheduled.MonthlyCostCredits,
		}
	}
	checkLabel, deviceLabel := "checks", "devices"
	if estimate.TotalChecks == 1 { checkLabel = "check" }
	if estimate.DevicesCount == 1 { deviceLabel = "device" }
	text := fmt.Sprintf("One live check for tracker %s is estimated at $%.4f (%d credits): %d keywords × %d %s = %d SERP %s.", args.TrackerID, estimate.CostUSD, estimate.CostCredits, estimate.KeywordCount, estimate.DevicesCount, deviceLabel, estimate.TotalChecks, checkLabel)
	if estimate.AdditionalKeywordCount > 0 {
		label := "keywords"
		if estimate.AdditionalKeywordCount == 1 { label = "keyword" }
		text += fmt.Sprintf(" This projects %d additional %s.", estimate.AdditionalKeywordCount, label)
	}
	if scheduled := estimate.ScheduledEstimate; scheduled != nil {
		text += fmt.Sprintf(" Its %s queued checks have a nominal estimate of $%.4f (%d credits) each, or about $%.4f (%d credits) per month. Rejected, failed, or timed-out queued tasks may use additional separately billed live fallback; use the per-check estimate as the approval ceiling when adding keywords.", scheduled.Interval, scheduled.CostUSD, scheduled.CostCredits, scheduled.MonthlyCostUSD, scheduled.MonthlyCostCredits)
	}
	text += " No check was started."
	return mcpResponse(text, payload, metaFields{
		ProjectID: access.Project.ID,
		URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/rank-tracking/"+args.TrackerID, nil),
	}), nil
}
