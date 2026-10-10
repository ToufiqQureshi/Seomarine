package mcp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/ranktracking"
)

func getRankTrackerTool() *tool {
	return &tool{
		Name:         "get_rank_tracker",
		Title:        "Get rank tracker",
		Description:  "Read rank tracker configs and their latest results. With trackerId, returns the config and latest position per keyword; without it, lists all active trackers. Uses no credits.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"trackerId":{"type":"string","format":"uuid"}},"required":["projectId"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"configs":{"type":"array","items":{"type":"object"}},"config":{"type":"object"},"results":{"type":"object","properties":{"rows":{"type":"array","items":{"type":"object"}},"run":{"type":["object","null"]}},"additionalProperties":true}},"additionalProperties":true}`),
		Annotations:  readOnlyAnnotations(),
		Handler:      handleGetRankTracker,
	}
}

func handleGetRankTracker(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string `json:"projectId"`
		TrackerID string `json:"trackerId"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	if args.TrackerID != "" {
		if !validTrackerID(args.TrackerID) {
			return nil, newAppErrorf("VALIDATION_ERROR", "trackerId must be a valid id")
		}
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil {
		return nil, err
	}
	if env.deps.RankTracking == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Rank tracking is not available on this server.")
	}
	configs, err := env.deps.RankTracking.ListConfigs(ctx, access.Project.ID)
	if err != nil {
		env.deps.Logger.ErrorContext(ctx, "MCP list rank trackers", "project_id", access.Project.ID, "err", err)
		return nil, newAppErrorf("INTERNAL_ERROR", "Unable to load rank trackers.")
	}
	meta := metaFields{ProjectID: access.Project.ID, URL: buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/rank-tracking", nil)}
	if args.TrackerID == "" {
		lines := make([]string, 0, len(configs))
		for _, config := range configs {
			location := ""
			if config.LocationName != nil && *config.LocationName != "" {
				location = fmt.Sprintf("  location:%q", *config.LocationName)
			}
			lines = append(lines, fmt.Sprintf("- %s  %s  loc:%d%s  schedule:%s", config.ID, config.Domain, config.LocationCode, location, config.ScheduleInterval))
		}
		text := fmt.Sprintf("Rank trackers (%d):\n%s", len(configs), strings.Join(lines, "\n"))
		if len(configs) == 0 {
			text = "No rank trackers configured for this project."
		}
		return mcpResponse(text, map[string]any{"configs": configs}, meta), nil
	}
	var config *ranktracking.Config
	for i := range configs {
		if configs[i].ID == args.TrackerID {
			config = &configs[i]
			break
		}
	}
	if config == nil {
		return nil, newAppError("NOT_FOUND")
	}
	results, err := env.deps.RankTracking.LatestResults(ctx, access.Project.ID, config.ID, "7d")
	if err != nil {
		if errors.Is(err, ranktracking.ErrNotFound) {
			return nil, newAppError("NOT_FOUND")
		}
		env.deps.Logger.ErrorContext(ctx, "MCP read rank tracker results", "project_id", access.Project.ID, "tracker_id", config.ID, "err", err)
		return nil, newAppErrorf("INTERNAL_ERROR", "Unable to load rank tracker results.")
	}
	meta.URL = buildDashboardURL(env.auth.BaseURL, "/p/"+access.Project.ID+"/rank-tracking/"+config.ID, nil)
	runText := "Latest run: never"
	if results.Run != nil {
		switch results.Run.Status {
		case "failed":
			message := "Unknown error"
			if results.Run.ErrorMessage != nil {
				message = *results.Run.ErrorMessage
			}
			runText = "Latest run: failed — " + message
		case "completed":
			if results.Run.CompletedAt != nil {
				runText = "Latest run: " + results.Run.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
			} else if results.Run.LastCheckedAt != nil {
				runText = "Latest run: " + results.Run.LastCheckedAt.Format("2006-01-02T15:04:05Z07:00")
			} else {
				runText = "Latest run: completed"
			}
		default:
			runText = "Latest run: " + results.Run.Status
		}
	}
	lines := make([]string, 0, len(results.Rows))
	for _, row := range results.Rows {
		lines = append(lines, fmt.Sprintf("- %s | desktop: %s (prev %s) | mobile: %s (prev %s)", row.Keyword, positionText(row.Desktop.Position), positionText(row.Desktop.PreviousPosition), positionText(row.Mobile.Position), positionText(row.Mobile.PreviousPosition)))
	}
	if len(lines) == 0 {
		lines = append(lines, "No keywords tracked yet.")
	}
	text := fmt.Sprintf("Tracker %s (%s%s):\nSchedule: %s, devices: %s, depth: %d\n%s\nKeywords (%d):\n%s", config.ID, config.Domain, locationText(config.LocationName), config.ScheduleInterval, config.Devices, config.SerpDepth, runText, len(results.Rows), strings.Join(lines, "\n"))
	return mcpResponse(text, map[string]any{"config": config, "results": results}, meta), nil
}

func validTrackerID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	decoded := make([]byte, 16)
	_, err := hex.Decode(decoded, []byte(compact))
	return err == nil
}

func positionText(value *int) string {
	if value == nil {
		return "—"
	}
	return strconv.Itoa(*value)
}

func locationText(value *string) string {
	if value == nil || *value == "" {
		return ""
	}
	return ", " + *value
}
