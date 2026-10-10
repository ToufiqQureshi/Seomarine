package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/ranktracking"
)

func runRankTrackerTool() *tool {
	return &tool{
		Name: "run_rank_tracker", Title: "Run rank tracker",
		Description: "Start an explicit live rank check for every keyword and configured device. This spends credits: first estimate the cost, show it to the user, and pass the approved credits as maxCostCredits. The Go service checks the fresh estimate again and enforces the paid-plan gate. If a run is already active, no second run is created.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"trackerId":{"type":"string","format":"uuid"},"maxCostCredits":{"type":"integer","minimum":1}},"required":["projectId","trackerId","maxCostCredits"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"trackerId":{"type":"string"},"started":{"type":"boolean"},"runId":{"type":"string"},"blockingRunId":{"type":["string","null"]}},"required":["trackerId","started"],"additionalProperties":true}`),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": true, "destructiveHint": false},
		Handler: handleRunRankTracker,
	}
}

func handleRunRankTracker(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string `json:"projectId"`
		TrackerID string `json:"trackerId"`
		MaxCostCredits *int `json:"maxCostCredits"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" || !validTrackerID(args.TrackerID) || args.MaxCostCredits == nil || *args.MaxCostCredits < 1 {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId, a valid trackerId, and positive maxCostCredits are required")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }
	checks := env.deps.RankChecks
	if checks == nil { return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Rank checks are not available on this server.") }
	runID, err := checks.Start(ctx, ranktracking.StartInput{
		OrganizationID: access.Auth.OrganizationID, ProjectID: access.Project.ID, ConfigID: args.TrackerID,
		MaxCostCredits: args.MaxCostCredits,
	})
	if err != nil {
		switch {
		case errors.Is(err, ranktracking.ErrNotFound):
			return nil, newAppError("NOT_FOUND")
		case errors.Is(err, ranktracking.ErrPaymentRequired):
			return nil, newAppErrorf("PAYMENT_REQUIRED", "Upgrade to a paid plan to run rank checks.")
		case errors.Is(err, ranktracking.ErrChecksUnavailable):
			return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Rank checks are not available on this server.")
		case errors.Is(err, ranktracking.ErrRunActive):
			active, _ := errors.AsType[*ranktracking.ActiveRunError](err)
			blocking := ""
			if active != nil { blocking = active.RunID }
			message := fmt.Sprintf("A rank check is already running for tracker %s", args.TrackerID)
			if blocking != "" { message += " (run "+blocking+")" }
			message += ". No new run was created and no additional check was charged. Poll get_rank_tracker until lastCheckedAt advances."
			var blockingValue any
			if blocking != "" { blockingValue = blocking }
			return mcpResponse(message, map[string]any{
				"trackerId": args.TrackerID, "started": false, "blockingRunId": blockingValue,
			}, metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/rank-tracking/"+args.TrackerID, nil)}), nil
		}
		var validation ranktracking.ValidationError
		if errors.As(err, &validation) {
			return nil, newAppErrorf("VALIDATION_ERROR", validation.Error())
		}
		env.deps.Logger.ErrorContext(ctx, "MCP start rank tracker check", "project_id", access.Project.ID, "tracker_id", args.TrackerID, "err", err)
		return nil, newAppErrorf("INTERNAL_ERROR", "Unable to start rank check.")
	}
	return mcpResponse(fmt.Sprintf("Rank check %s started for tracker %s. Poll get_rank_tracker until lastCheckedAt advances, then read the updated positions.", runID, args.TrackerID),
		map[string]any{"trackerId": args.TrackerID, "started": true, "runId": runID},
		metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/rank-tracking/"+args.TrackerID, nil)}), nil
}
