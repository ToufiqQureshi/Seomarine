package keywords

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

const maxSavedBody = 1 << 20

// ProjectMarkets reads a project's default market.
type ProjectMarkets interface {
	Get(ctx context.Context, organizationID, projectID string) (market.Pair, error)
}

// SavedDeps are the dependencies of the saved keyword routes.
type SavedDeps struct {
	Logger            *slog.Logger
	Service           *SavedService
	ProjectMarkets    ProjectMarkets
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// MountSaved registers the saved keyword routes. Like the audit API, every
// route is a POST with a JSON body, and unknown fields are rejected.
func MountSaved(mux *http.ServeMux, d SavedDeps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	base := "POST /api/v1/projects/{projectId}/keywords/saved/"
	for path, fn := range map[string]func(savedRequest) (any, error){
		"save":        saveKeywords,
		"list":        listSaved,
		"export":      exportSaved,
		"remove":      removeSaved,
		"tags/assign": assignTags,
		"tags/update": updateTag,
		"tags/delete": deleteTag,
	} {
		mux.Handle(base+path, protect(handleSaved(d, fn)))
	}
}

type savedRequest struct {
	d              SavedDeps
	r              *http.Request
	w              http.ResponseWriter
	organizationID string
	projectID      string
}

func handleSaved(d SavedDeps, fn func(savedRequest) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "keywords_unavailable", "Saved keywords are not available on this server.")
			return
		}
		organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
		if !ok {
			d.Logger.ErrorContext(r.Context(), "saved keyword request without project organization")
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		result, err := fn(savedRequest{d: d, r: r, w: w, organizationID: organizationID, projectID: r.PathValue("projectId")})
		writeSaved(d.Logger, w, r, result, err)
	}
}

func writeSaved(logger *slog.Logger, w http.ResponseWriter, r *http.Request, result any, err error) {
	invalid, isInvalid := errors.AsType[ValidationError](err)
	inUse, isInUse := errors.AsType[*TagInUseError](err)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, result)
	case isInvalid:
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", invalid.Error())
	case isInUse:
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":           httpx.Error{Code: "tag_in_use", Message: inUse.Error()},
			"assignmentCount": inUse.AssignmentCount,
		})
	case errors.Is(err, errResearchUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "keywords_unavailable", "Keyword research is not available on this server.")
	case errors.Is(err, errPaymentRequired):
		httpx.WriteError(w, http.StatusPaymentRequired, "payment_required", "Upgrade to the paid plan to use keyword research.")
	case isUnknownLocation(err):
		httpx.WriteError(w, http.StatusBadRequest, "unknown_location", err.Error())
	case errors.Is(err, ErrLocationCache):
		httpx.WriteError(w, http.StatusServiceUnavailable, "locations_unavailable", "Location data is temporarily unavailable. Please try again.")
	case errors.Is(err, dataforseo.ErrBillingIssue) && r.Context().Err() == nil:
		logger.ErrorContext(r.Context(), "DataForSEO billing issue", "err", err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "provider_billing_issue", "Keyword data is temporarily unavailable. Please try again later.")
	case isProviderError(err) && r.Context().Err() == nil:
		logger.ErrorContext(r.Context(), "keyword data provider failed", "err", err)
		httpx.WriteError(w, http.StatusBadGateway, "provider_error", "Keyword data is temporarily unavailable. Please try again.")
	case errors.Is(err, ErrTagExists):
		httpx.WriteError(w, http.StatusConflict, "tag_exists", "A tag with that name already exists.")
	case r.Context().Err() != nil:
		logger.InfoContext(r.Context(), "saved keyword request canceled", "err", err)
	default:
		logger.ErrorContext(r.Context(), "saved keyword request failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
	}
}

func (q savedRequest) decode(dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(q.w, q.r.Body, maxSavedBody))
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

// optionalString tells "field absent" from "field is null".
type optionalString struct {
	Set   bool
	Value *string
}

func (o *optionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.Value)
}

func validIDs(ids []string, limit int, what string) error {
	if len(ids) == 0 || len(ids) > limit {
		return ValidationError("Send between 1 and " + strconv.Itoa(limit) + " " + what + ".")
	}
	for _, id := range ids {
		if id == "" || len(id) > 64 {
			return ValidationError("Every id must be 1 to 64 characters.")
		}
	}
	return nil
}

func validTagNames(names []string, limit int) error {
	if len(names) > limit {
		return ValidationError("Send at most " + strconv.Itoa(limit) + " tags.")
	}
	for _, n := range names {
		if t := strings.TrimSpace(n); t == "" || units(t) > MaxTagNameUnits {
			return ValidationError("Tag names are 1 to 64 characters.")
		}
	}
	return nil
}

func validMetric(m Metric) error {
	switch {
	case m.Keyword == "":
		return ValidationError("Every metric needs a keyword.")
	case m.SearchVolume != nil && *m.SearchVolume < 0, m.CPC != nil && *m.CPC < 0:
		return ValidationError("Search volume and CPC cannot be negative.")
	case m.Competition != nil && (*m.Competition < 0 || *m.Competition > 1):
		return ValidationError("Competition is between 0 and 1.")
	case m.KeywordDifficulty != nil && (*m.KeywordDifficulty < 0 || *m.KeywordDifficulty > 100):
		return ValidationError("Keyword difficulty is between 0 and 100.")
	case m.Intent != nil && !slices.Contains(intents, *m.Intent):
		return ValidationError("Intent is not one of the known values.")
	}
	for _, ms := range m.MonthlySearches {
		if ms.Year < 1 || ms.Month < 1 || ms.Month > 12 || ms.SearchVolume < 0 {
			return ValidationError("Monthly searches need a year, a month from 1 to 12 and a volume of 0 or more.")
		}
	}
	return nil
}

func saveKeywords(q savedRequest) (any, error) {
	var body struct {
		Keywords     []string `json:"keywords"`
		LocationCode int      `json:"locationCode"`
		LanguageCode string   `json:"languageCode"`
		Tags         []string `json:"tags"`
		TagMode      string   `json:"tagMode"`
		Metrics      []Metric `json:"metrics"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if len(body.Keywords) == 0 || len(body.Keywords) > MaxSaveKeywords || slices.Contains(body.Keywords, "") {
		return nil, ValidationError("Send between 1 and 500 keywords.")
	}
	if body.LanguageCode != "" && (len(body.LanguageCode) < 2 || len(body.LanguageCode) > 8) {
		return nil, ValidationError("Location or language is not valid.")
	}
	if err := validTagNames(body.Tags, MaxTagsPerRequest); err != nil {
		return nil, err
	}
	if body.TagMode != "" && body.TagMode != "append" && body.TagMode != "replace" {
		return nil, ValidationError("Tag mode is append or replace.")
	}
	if len(body.Metrics) > MaxSaveKeywords {
		return nil, ValidationError("Send at most 500 metrics.")
	}
	for _, m := range body.Metrics {
		if err := validMetric(m); err != nil {
			return nil, err
		}
	}
	projectMarket, err := q.d.ProjectMarkets.Get(q.r.Context(), q.organizationID, q.projectID)
	if err != nil {
		return nil, err
	}
	pair, err := ResolveMarket(body.LocationCode, body.LanguageCode, projectMarket)
	if err != nil {
		return nil, err
	}
	saved, err := q.d.Service.Save(q.r.Context(), SaveInput{
		ProjectID: q.projectID, LocationCode: pair.LocationCode, LanguageCode: pair.LanguageCode,
		Keywords: body.Keywords, Tags: body.Tags, ReplaceTags: body.TagMode == "replace", Metrics: body.Metrics,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "savedKeywordIds": saved}, nil
}

type listBody struct {
	Search        string   `json:"search"`
	IncludeTerms  []string `json:"includeTerms"`
	ExcludeTerms  []string `json:"excludeTerms"`
	MinVolume     *int     `json:"minVolume"`
	MaxVolume     *int     `json:"maxVolume"`
	MinCPC        *float64 `json:"minCpc"`
	MaxCPC        *float64 `json:"maxCpc"`
	MinDifficulty *int     `json:"minDifficulty"`
	MaxDifficulty *int     `json:"maxDifficulty"`
	TagIDs        []string `json:"tagIds"`
	TagNames      []string `json:"tagNames"`
	Sort          string   `json:"sort"`
	Order         string   `json:"order"`
}

func (b listBody) query(projectID string) (ListQuery, error) {
	switch {
	case units(strings.TrimSpace(b.Search)) > MaxSearchTextUnits:
		return ListQuery{}, ValidationError("Search is at most 200 characters.")
	case len(b.IncludeTerms) > MaxListTerms || len(b.ExcludeTerms) > MaxListTerms:
		return ListQuery{}, ValidationError("Send at most 20 include and 20 exclude terms.")
	case len(b.TagIDs) > MaxFilterTags || len(b.TagNames) > MaxFilterTags:
		return ListQuery{}, ValidationError("Send at most 50 tags to filter by.")
	case b.MinVolume != nil && *b.MinVolume < 0, b.MaxVolume != nil && *b.MaxVolume < 0,
		b.MinCPC != nil && *b.MinCPC < 0, b.MaxCPC != nil && *b.MaxCPC < 0:
		return ListQuery{}, ValidationError("Volume and CPC limits cannot be negative.")
	case b.MinDifficulty != nil && (*b.MinDifficulty < 0 || *b.MinDifficulty > 100),
		b.MaxDifficulty != nil && (*b.MaxDifficulty < 0 || *b.MaxDifficulty > 100):
		return ListQuery{}, ValidationError("Difficulty limits are between 0 and 100.")
	case b.Order != "" && b.Order != "asc" && b.Order != "desc":
		return ListQuery{}, ValidationError("Order is asc or desc.")
	}
	for _, t := range b.IncludeTerms {
		if strings.TrimSpace(t) == "" {
			return ListQuery{}, ValidationError("Terms cannot be empty.")
		}
	}
	for _, t := range b.ExcludeTerms {
		if strings.TrimSpace(t) == "" {
			return ListQuery{}, ValidationError("Terms cannot be empty.")
		}
	}
	if err := validTagNames(b.TagNames, MaxFilterTags); err != nil {
		return ListQuery{}, err
	}
	return ListQuery{
		ProjectID: projectID, Search: b.Search, IncludeTerms: b.IncludeTerms, ExcludeTerms: b.ExcludeTerms,
		MinVolume: b.MinVolume, MaxVolume: b.MaxVolume, MinCPC: b.MinCPC, MaxCPC: b.MaxCPC,
		MinDifficulty: b.MinDifficulty, MaxDifficulty: b.MaxDifficulty, TagIDs: b.TagIDs, TagNames: b.TagNames,
		Sort: b.Sort, Descending: b.Order != "asc",
	}, nil
}

func listSaved(q savedRequest) (any, error) {
	var body struct {
		listBody
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if body.Page == 0 {
		body.Page = 1
	}
	if body.PageSize == 0 {
		body.PageSize = DefaultSavedPage
	}
	if body.Page < 1 || !slices.Contains(PageSizes, body.PageSize) {
		return nil, ValidationError("Page is 1 or more and the page size is 50, 100 or 250.")
	}
	query, err := body.query(q.projectID)
	if err != nil {
		return nil, err
	}
	query.Page, query.PageSize = body.Page, body.PageSize
	return q.d.Service.List(q.r.Context(), query)
}

func exportSaved(q savedRequest) (any, error) {
	var body listBody
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	query, err := body.query(q.projectID)
	if err != nil {
		return nil, err
	}
	res, err := q.d.Service.List(q.r.Context(), query)
	if err != nil {
		return nil, err
	}
	return map[string]any{"rows": res.Rows}, nil
}

func removeSaved(q savedRequest) (any, error) {
	var body struct {
		SavedKeywordIDs []string `json:"savedKeywordIds"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validIDs(body.SavedKeywordIDs, MaxBatchIDs, "keyword ids"); err != nil {
		return nil, err
	}
	deleted, err := q.d.Service.Remove(q.r.Context(), q.projectID, body.SavedKeywordIDs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "deletedCount": deleted}, nil
}

func assignTags(q savedRequest) (any, error) {
	var body struct {
		SavedKeywordIDs []string `json:"savedKeywordIds"`
		AddTags         []string `json:"addTags"`
		RemoveTagIDs    []string `json:"removeTagIds"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if err := validIDs(body.SavedKeywordIDs, MaxBatchIDs, "keyword ids"); err != nil {
		return nil, err
	}
	if err := validTagNames(body.AddTags, MaxTagsPerRequest); err != nil {
		return nil, err
	}
	if len(body.RemoveTagIDs) > MaxRemoveTagIDs {
		return nil, ValidationError("Remove at most 50 tags at a time.")
	}
	for _, id := range body.RemoveTagIDs {
		if id == "" || len(id) > 64 {
			return nil, ValidationError("Every id must be 1 to 64 characters.")
		}
	}
	return q.d.Service.UpdateAssignments(q.r.Context(), q.projectID, body.SavedKeywordIDs, body.AddTags, body.RemoveTagIDs)
}

func updateTag(q savedRequest) (any, error) {
	var body struct {
		TagID string         `json:"tagId"`
		Name  *string        `json:"name"`
		Color optionalString `json:"color"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if body.TagID == "" || len(body.TagID) > 64 {
		return nil, ValidationError("tagId must be 1 to 64 characters.")
	}
	var color **string
	if body.Color.Set {
		color = &body.Color.Value
	}
	tag, err := q.d.Service.UpdateTag(q.r.Context(), q.projectID, body.TagID, body.Name, color)
	if err != nil {
		return nil, err
	}
	if tag == nil {
		return map[string]any{"success": false}, nil
	}
	return map[string]any{"success": true, "tag": tag}, nil
}

func deleteTag(q savedRequest) (any, error) {
	var body struct {
		TagID string `json:"tagId"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if body.TagID == "" || len(body.TagID) > 64 {
		return nil, ValidationError("tagId must be 1 to 64 characters.")
	}
	err := q.d.Service.DeleteTag(q.r.Context(), q.projectID, body.TagID)
	if errors.Is(err, ErrTagNotFound) {
		return map[string]any{"success": false}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true}, nil
}

func isUnknownLocation(err error) bool {
	_, ok := errors.AsType[UnknownLocationError](err)
	return ok
}
