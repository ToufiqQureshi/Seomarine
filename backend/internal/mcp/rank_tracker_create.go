package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/ranktracking"
	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

func createRankTrackerTool() *tool {
	return &tool{
		Name: "create_rank_tracker", Title: "Create rank tracker",
		Description: "Create an empty rank tracking configuration. No check or credit spend starts during creation. Defaults to the project domain and market, mobile device, depth 40, and a manual schedule. Scheduled checks can spend credits after keywords are added; estimate the cost and get user approval before adding keywords.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"projectId":{"type":"string","minLength":1},"domain":{"type":"string","minLength":1,"maxLength":255},"locationCode":{"type":"integer","minimum":1},"languageCode":{"type":"string","minLength":2,"maxLength":8},"locationName":{"type":"string","minLength":1,"maxLength":200},"devices":{"type":"string","enum":["desktop","mobile","both"]},"serpDepth":{"type":"integer","minimum":10,"maximum":100,"multipleOf":10},"scheduleInterval":{"type":"string","enum":["manual","daily","weekly","monthly"]},"scheduleTime":{"type":"object","properties":{"weekday":{"type":"integer","minimum":0,"maximum":6},"hour":{"type":"integer","minimum":0,"maximum":23},"minute":{"type":"integer","minimum":0,"maximum":59},"timeZone":{"type":"string"}},"required":["hour","minute"],"additionalProperties":false}},"required":["projectId"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"trackerId":{"type":"string"},"config":{"type":"object"}},"required":["trackerId","config"],"additionalProperties":true}`),
		Annotations: map[string]any{"readOnlyHint": false, "openWorldHint": false, "destructiveHint": false},
		Handler: handleCreateRankTracker,
	}
}

func handleCreateRankTracker(ctx context.Context, raw json.RawMessage, env *callEnv) (*callResult, error) {
	var args struct {
		ProjectID string `json:"projectId"`
		Domain string `json:"domain"`
		LocationCode int `json:"locationCode"`
		LanguageCode string `json:"languageCode"`
		LocationName *string `json:"locationName"`
		Devices string `json:"devices"`
		SerpDepth int `json:"serpDepth"`
		ScheduleInterval string `json:"scheduleInterval"`
		ScheduleTime *struct {
			Weekday *int `json:"weekday"`
			Hour *int `json:"hour"`
			Minute *int `json:"minute"`
			TimeZone string `json:"timeZone"`
		} `json:"scheduleTime"`
	}
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.ProjectID) == "" {
		return nil, newAppErrorf("VALIDATION_ERROR", "projectId is required")
	}
	if args.LocationCode < 0 || args.LocationCode == 0 && strings.Contains(string(raw), `"locationCode":0`) {
		return nil, newAppErrorf("VALIDATION_ERROR", "locationCode must be positive")
	}
	if args.LanguageCode != "" && (len(strings.TrimSpace(args.LanguageCode)) < 2 || len(strings.TrimSpace(args.LanguageCode)) > 8) {
		return nil, newAppErrorf("VALIDATION_ERROR", "languageCode must be 2 to 8 characters")
	}
	if args.LocationName != nil {
		*args.LocationName = strings.TrimSpace(*args.LocationName)
		if *args.LocationName == "" || len(utf16.Encode([]rune(*args.LocationName))) > 200 {
			return nil, newAppErrorf("VALIDATION_ERROR", "locationName must be 1 to 200 characters")
		}
	}
	switch args.Devices {
	case "", "desktop", "mobile", "both":
	default:
		return nil, newAppErrorf("VALIDATION_ERROR", "devices is invalid")
	}
	depth := args.SerpDepth
	if depth == 0 { depth = 40 }
	if depth < 10 || depth > 100 || depth%10 != 0 {
		return nil, newAppErrorf("VALIDATION_ERROR", "serpDepth must be a multiple of 10 from 10 to 100")
	}
	interval := args.ScheduleInterval
	if interval == "" { interval = "manual" }
	switch interval {
	case "manual", "daily", "weekly", "monthly":
	default: return nil, newAppErrorf("VALIDATION_ERROR", "scheduleInterval is invalid")
	}
	var schedule *ranktracking.ScheduleTime
	if args.ScheduleTime != nil {
		if args.ScheduleTime.Hour == nil || args.ScheduleTime.Minute == nil {
			return nil, newAppErrorf("VALIDATION_ERROR", "scheduleTime requires hour and minute")
		}
		schedule = &ranktracking.ScheduleTime{Weekday: args.ScheduleTime.Weekday, Hour: *args.ScheduleTime.Hour, Minute: *args.ScheduleTime.Minute}
		if args.ScheduleTime.TimeZone != "" {
			location, err := time.LoadLocation(args.ScheduleTime.TimeZone)
			if err != nil { return nil, newAppErrorf("VALIDATION_ERROR", "scheduleTime.timeZone must be a valid IANA time zone") }
			schedule.Location = location
		}
	}
	if env.deps.RankTracking == nil {
		return nil, newAppErrorf("SERVICE_UNAVAILABLE", "Rank tracking is not available on this server.")
	}
	access, err := env.h.authorizeProject(ctx, env.auth, args.ProjectID)
	if err != nil { return nil, err }
	domainName := strings.TrimSpace(args.Domain)
	if domainName == "" {
		if access.Project.Domain == nil || strings.TrimSpace(*access.Project.Domain) == "" {
			return nil, newAppErrorf("VALIDATION_ERROR", "Provide a domain or set the project's domain first")
		}
		domainName = *access.Project.Domain
	}
	pair, err := keywords.ResolveMarket(args.LocationCode, strings.TrimSpace(args.LanguageCode), market.Pair{
		LocationCode: access.Project.LocationCode, LanguageCode: access.Project.LanguageCode,
	})
	if err != nil { return nil, newAppErrorf("VALIDATION_ERROR", err.Error()) }
	config, err := env.deps.RankTracking.CreateConfig(ctx, ranktracking.CreateInput{
		OrganizationID: access.Auth.OrganizationID, ProjectID: access.Project.ID, ProjectMarket: pair,
		Domain: domainName, LocationCode: pair.LocationCode, LanguageCode: pair.LanguageCode,
		LocationName: args.LocationName, Devices: defaultString(args.Devices, "mobile"),
		SerpDepth: depth, ScheduleInterval: ranktracking.Interval(interval), ScheduleTime: schedule,
	})
	if err != nil {
		if errors.Is(err, ranktracking.ErrDuplicate) {
			return nil, newAppErrorf("CONFLICT", err.Error())
		}
		if errors.Is(err, ranktracking.ErrLimit) {
			return nil, newAppErrorf("LIMIT_REACHED", err.Error())
		}
		var validation ranktracking.ValidationError
		if errors.As(err, &validation) {
			return nil, newAppErrorf("VALIDATION_ERROR", validation.Error())
		}
		env.deps.Logger.ErrorContext(ctx, "MCP create rank tracker", "project_id", access.Project.ID, "err", err)
		return nil, newAppErrorf("INTERNAL_ERROR", "Unable to create rank tracker.")
	}
	next := ""
	if config.NextCheckAt != nil { next = " First scheduled check: "+config.NextCheckAt.Format(time.RFC3339)+"." }
	local := ""
	if config.LocationName != nil && *config.LocationName != "" { local = " Local tracking for "+*config.LocationName+"." }
	text := fmt.Sprintf("Created rank tracker %s for %s (%s, top %d, %s). No keywords were added, no check was started, and no credits were used.%s%s", config.ID, config.Domain, config.Devices, config.SerpDepth, config.ScheduleInterval, local, next)
	if config.ScheduleInterval != ranktracking.Manual {
		text += " Scheduled checks will spend credits after keywords are added; estimate and obtain approval before adding them."
	}
	return mcpResponse(text, map[string]any{"trackerId": config.ID, "config": config}, metaFields{
		ProjectID: access.Project.ID, URL: buildDashboardURL(access.Auth.BaseURL, "/p/"+access.Project.ID+"/rank-tracking/"+config.ID, nil),
	}), nil
}

func defaultString(value, fallback string) string {
	if value == "" { return fallback }
	return value
}
