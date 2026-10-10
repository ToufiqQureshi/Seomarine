package ranktracking

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

const (
	maxBody         = 256 << 10
	maxKeywordBatch = 2000
)

// ProjectMarkets reads a project's default market.
type ProjectMarkets interface {
	Get(ctx context.Context, organizationID, projectID string) (market.Pair, error)
}

// Deps contains the route middleware and service for the rank-tracking API.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	Checks            *Checks // nil answers 503 on the check route
	ProjectMarkets    ProjectMarkets
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers the rank-tracking routes. Like the audit API, every route
// is a POST with a JSON body, and unknown fields are rejected.
func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	base := "POST /api/v1/projects/{projectId}/rank-tracking/"
	mux.Handle(base+"configs/list", protect(handle(d, listConfigs)))
	mux.Handle(base+"configs/create", protect(handle(d, createConfig)))
	mux.Handle(base+"configs/update", protect(handle(d, updateConfig)))
	mux.Handle(base+"keywords/list", protect(handle(d, listKeywords)))
	mux.Handle(base+"keywords/add", protect(handle(d, addKeywords)))
	mux.Handle(base+"keywords/remove", protect(handle(d, removeKeywords)))
	mux.Handle(base+"keywords/metrics/refresh", protect(handle(d, refreshKeywordMetrics)))
	mux.Handle(base+"estimate-cost", protect(handle(d, estimateCost)))
	mux.Handle(base+"checks/start", protect(handle(d, startCheck)))
	mux.Handle(base+"results/latest", protect(handle(d, latestResults)))
	mux.Handle(base+"runs/latest", protect(handle(d, latestRun)))
	mux.Handle(base+"results/history", protect(handle(d, keywordHistory)))
	mux.Handle(base+"results/trend", protect(handle(d, configTrend)))
	mux.Handle(base+"results/matrix", protect(handle(d, positionMatrix)))
}

type request struct {
	d              Deps
	r              *http.Request
	w              http.ResponseWriter
	organizationID string
	projectID      string
}

func handle(d Deps, fn func(request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "rank_tracking_unavailable", "Rank tracking is not set up on this server.")
			return
		}
		organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
		if !ok {
			d.Logger.ErrorContext(r.Context(), "rank tracking request without project organization")
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		result, err := fn(request{d: d, r: r, w: w, organizationID: organizationID, projectID: r.PathValue("projectId")})
		writeResult(d.Logger, w, r, result, err)
	}
}

func writeResult(logger *slog.Logger, w http.ResponseWriter, r *http.Request, result any, err error) {
	invalid, isInvalid := errors.AsType[ValidationError](err)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, result)
	case isInvalid:
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", invalid.Error())
	case errors.Is(err, ErrChecksUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "checks_unavailable", "Rank checks are not available on this server.")
	case errors.Is(err, ErrMetricsUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "metrics_unavailable", "Rank tracking metrics are not available on this server.")
	case errors.Is(err, ErrPaymentRequired):
		httpx.WriteError(w, http.StatusPaymentRequired, "payment_required", "Upgrade to the paid plan to run rank checks.")
	case errors.Is(err, ErrRunActive):
		blocking := ""
		if active, ok := errors.AsType[*ActiveRunError](err); ok {
			blocking = active.RunID
		}
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":         httpx.Error{Code: "already_running", Message: "A rank check is already running for this domain."},
			"blockingRunId": blocking,
		})
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "config_not_found", "Rank tracking config not found.")
	case errors.Is(err, ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "already_tracked", "This domain and location combination is already being tracked.")
	case errors.Is(err, ErrLimit):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "limit_reached", err.Error())
	case errors.Is(err, ErrLocationUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "location_check_unavailable", "City-level tracking is not available right now.")
	case r.Context().Err() != nil:
		logger.InfoContext(r.Context(), "rank tracking request canceled", "err", err)
	default:
		logger.ErrorContext(r.Context(), "rank tracking request failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
	}
}

func (q request) decode(dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(q.w, q.r.Body, maxBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if _, tooBig := errors.AsType[*http.MaxBytesError](err); tooBig {
			return ValidationError("The request is too large.")
		}
		return ValidationError("The request is not valid JSON.")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ValidationError("The request must contain exactly one JSON value.")
	}
	return nil
}

type scheduleTimeBody struct {
	Weekday  *int    `json:"weekday"`
	Hour     int     `json:"hour"`
	Minute   int     `json:"minute"`
	TimeZone *string `json:"timeZone"`
}

func (b *scheduleTimeBody) parse() (*ScheduleTime, error) {
	if b == nil {
		return nil, nil
	}
	t := &ScheduleTime{Weekday: b.Weekday, Hour: b.Hour, Minute: b.Minute}
	if b.TimeZone != nil {
		loc, err := time.LoadLocation(*b.TimeZone)
		if err != nil || *b.TimeZone == "" || *b.TimeZone == "Local" {
			return nil, ValidationError("Unknown IANA timezone")
		}
		t.Location = loc
	}
	if err := t.Validate(); err != nil {
		return nil, ValidationError(capitalize(err.Error()))
	}
	return t, nil
}

// optionalString tells "field absent" from "field is null", for columns that
// can be cleared.
type optionalString struct {
	Set   bool
	Value *string
}

func (o *optionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.Value)
}

type configIDBody struct {
	ConfigID string `json:"configId"`
}

func validID(id string) error {
	if len(id) != 36 {
		return ValidationError("configId must be a valid id.")
	}
	return nil
}

func listConfigs(q request) (any, error) {
	var body struct{}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	configs, err := q.d.Service.ListConfigs(q.r.Context(), q.projectID)
	if err != nil {
		return nil, err
	}
	if configs == nil {
		configs = []Config{}
	}
	return map[string]any{"configs": configs}, nil
}

func createConfig(q request) (any, error) {
	var body struct {
		Domain           string            `json:"domain"`
		LocationCode     int               `json:"locationCode"`
		LanguageCode     string            `json:"languageCode"`
		LocationName     *string           `json:"locationName"`
		Devices          string            `json:"devices"`
		SerpDepth        int               `json:"serpDepth"`
		ScheduleInterval Interval          `json:"scheduleInterval"`
		ScheduleTime     *scheduleTimeBody `json:"scheduleTime"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	schedule, err := body.ScheduleTime.parse()
	if err != nil {
		return nil, err
	}
	projectMarket, err := q.d.ProjectMarkets.Get(q.r.Context(), q.organizationID, q.projectID)
	if err != nil {
		return nil, err
	}
	cfg, err := q.d.Service.CreateConfig(q.r.Context(), CreateInput{
		OrganizationID: q.organizationID, ProjectID: q.projectID, ProjectMarket: projectMarket, Domain: body.Domain,
		LocationCode: body.LocationCode, LanguageCode: body.LanguageCode, LocationName: body.LocationName,
		Devices: body.Devices, SerpDepth: body.SerpDepth, ScheduleInterval: body.ScheduleInterval, ScheduleTime: schedule,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"config": cfg}, nil
}

func updateConfig(q request) (any, error) {
	var body struct {
		configIDBody
		Domain           *string           `json:"domain"`
		LocationCode     *int              `json:"locationCode"`
		LanguageCode     *string           `json:"languageCode"`
		LocationName     optionalString    `json:"locationName"`
		Devices          *string           `json:"devices"`
		SerpDepth        *int              `json:"serpDepth"`
		ScheduleInterval *Interval         `json:"scheduleInterval"`
		ScheduleTime     *scheduleTimeBody `json:"scheduleTime"`
		IsActive         *bool             `json:"isActive"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	schedule, err := body.ScheduleTime.parse()
	if err != nil {
		return nil, err
	}
	cfg, err := q.d.Service.UpdateConfig(q.r.Context(), q.projectID, body.ConfigID, UpdateInput{
		OrganizationID: q.organizationID, Domain: body.Domain, LocationCode: body.LocationCode, LanguageCode: body.LanguageCode,
		SetLocationName: body.LocationName.Set, LocationName: body.LocationName.Value,
		Devices: body.Devices, SerpDepth: body.SerpDepth, ScheduleInterval: body.ScheduleInterval,
		ScheduleTime: schedule, IsActive: body.IsActive,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"config": cfg}, nil
}

func listKeywords(q request) (any, error) {
	var body configIDBody
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	keywords, err := q.d.Service.ListKeywords(q.r.Context(), q.projectID, body.ConfigID)
	if err != nil {
		return nil, err
	}
	if keywords == nil {
		keywords = []Keyword{}
	}
	return map[string]any{"keywords": keywords}, nil
}

func addKeywords(q request) (any, error) {
	var body struct {
		configIDBody
		Keywords  []string `json:"keywords"`
		MatchCase bool     `json:"matchCase"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	if len(body.Keywords) == 0 || len(body.Keywords) > maxKeywordBatch {
		return nil, ValidationError("Send between 1 and 2000 keywords.")
	}
	return q.d.Service.AddKeywords(q.r.Context(), q.projectID, body.ConfigID, body.Keywords, body.MatchCase)
}

func removeKeywords(q request) (any, error) {
	var body struct {
		configIDBody
		KeywordIDs []string `json:"keywordIds"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	if len(body.KeywordIDs) == 0 || len(body.KeywordIDs) > maxKeywordBatch {
		return nil, ValidationError("Send between 1 and 2000 keyword ids.")
	}
	removed, err := q.d.Service.RemoveKeywords(q.r.Context(), q.projectID, body.ConfigID, body.KeywordIDs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"removed": len(removed), "removedIds": removed}, nil
}

func refreshKeywordMetrics(q request) (any, error) {
	var body configIDBody
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	updated, err := q.d.Service.RefreshKeywordMetrics(q.r.Context(), q.organizationID, q.projectID, body.ConfigID)
	if err != nil {
		return nil, err
	}
	return map[string]int{"updated": updated}, nil
}

func estimateCost(q request) (any, error) {
	var body struct {
		configIDBody
		AdditionalKeywords []string `json:"additionalKeywords"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	if len(body.AdditionalKeywords) > maxKeywordBatch {
		return nil, ValidationError("Send at most 2000 additional keywords.")
	}
	return q.d.Service.EstimateCost(q.r.Context(), q.projectID, body.ConfigID, body.AdditionalKeywords)
}

func latestResults(q request) (any, error) {
	var body struct {
		configIDBody
		ComparePeriod string `json:"comparePeriod"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	return q.d.Service.LatestResults(q.r.Context(), q.projectID, body.ConfigID, body.ComparePeriod)
}

func latestRun(q request) (any, error) {
	var body configIDBody
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	run, err := q.d.Service.LatestRun(q.r.Context(), q.projectID, body.ConfigID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"run": run}, nil
}

func keywordHistory(q request) (any, error) {
	var body struct {
		configIDBody
		TrackingKeywordID string `json:"trackingKeywordId"`
		SinceDays         int    `json:"sinceDays"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	if err := validID(body.TrackingKeywordID); err != nil {
		return nil, ValidationError("trackingKeywordId must be a valid id.")
	}
	if body.SinceDays == 0 {
		body.SinceDays = DefaultHistoryDays
	}
	points, err := q.d.Service.KeywordHistory(q.r.Context(), q.projectID, body.ConfigID, body.TrackingKeywordID, body.SinceDays)
	if err != nil {
		return nil, err
	}
	return map[string]any{"points": points}, nil
}

func configTrend(q request) (any, error) {
	var body struct {
		configIDBody
		Device    string `json:"device"`
		SinceDays int    `json:"sinceDays"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	if body.SinceDays == 0 {
		body.SinceDays = DefaultHistoryDays
	}
	points, err := q.d.Service.ConfigTrend(q.r.Context(), q.projectID, body.ConfigID, body.Device, body.SinceDays)
	if err != nil {
		return nil, err
	}
	return map[string]any{"points": points}, nil
}

func positionMatrix(q request) (any, error) {
	var body struct {
		configIDBody
		Device   string `json:"device"`
		RunLimit int    `json:"runLimit"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	if body.RunLimit == 0 {
		body.RunLimit = DefaultMatrixRuns
	}
	points, err := q.d.Service.PositionMatrix(q.r.Context(), q.projectID, body.ConfigID, body.Device, body.RunLimit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"points": points}, nil
}

func startCheck(q request) (any, error) {
	var body struct {
		configIDBody
		KeywordIDs     []string `json:"keywordIds"`
		MaxCostCredits *int     `json:"maxCostCredits"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validID(body.ConfigID); err != nil {
		return nil, err
	}
	if len(body.KeywordIDs) > maxKeywordBatch || (body.MaxCostCredits != nil && *body.MaxCostCredits < 0) {
		return nil, ValidationError("Send at most 2000 keyword ids and a cost limit of 0 or more.")
	}
	if q.d.Checks == nil {
		return nil, ErrChecksUnavailable
	}
	runID, err := q.d.Checks.Start(q.r.Context(), StartInput{
		OrganizationID: q.organizationID, ProjectID: q.projectID, ConfigID: body.ConfigID,
		KeywordIDs: body.KeywordIDs, MaxCostCredits: body.MaxCostCredits,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "runId": runID}, nil
}
