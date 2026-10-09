// Package domain serves the Domain Overview API: overview, keyword suggestions
// and the keywords and pages tabs, backed by DataForSEO Labs.
package domain

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"unicode/utf16"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxBody = 32 << 10

// Deps contains the route middleware and service needed for domain analysis.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	Plans             PaidPlans
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers domain analysis routes on the root mux.
func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	for _, route := range []struct {
		path string
		fn   func(Deps) http.HandlerFunc
	}{
		{"overview", overviewHandler},
		{"keyword-suggestions", keywordSuggestionsHandler},
		{"keywords", keywordsPageHandler},
		{"pages", pagesPageHandler},
	} {
		mux.Handle("POST /api/v1/projects/{projectId}/domain/"+route.path, protect(route.fn(d)))
	}
}

type badRequest string

func (e badRequest) Error() string { return string(e) }

type requestBase struct {
	Domain       string         `json:"domain"`
	Scope        string         `json:"scope,omitempty"`
	LocationCode *int           `json:"locationCode,omitempty"`
	LanguageCode *string        `json:"languageCode,omitempty"`
	Filters      keywordFilters `json:"filters,omitempty"`
	Search       string         `json:"search,omitempty"`
}

type pageRequest struct {
	requestBase
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"pageSize,omitempty"`
	SortMode  string `json:"sortMode,omitempty"`
	SortOrder string `json:"sortOrder,omitempty"`
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return badRequest("The request is too large.")
		}
		return badRequest("The request is not valid JSON.")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return badRequest("The request must contain exactly one JSON value.")
	}
	return nil
}

func handle(d Deps, fn func(http.ResponseWriter, *http.Request, string, string) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "domain_unavailable", "Domain analysis is not set up on this server.")
			return
		}
		organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
		if !ok {
			d.Logger.ErrorContext(r.Context(), "domain request without project organization")
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		if d.Plans != nil {
			paid, err := d.Plans.HasPaidPlan(r.Context(), organizationID)
			if err != nil {
				d.Logger.ErrorContext(r.Context(), "check domain plan", "err", err)
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
				return
			}
			if !paid {
				httpx.WriteError(w, http.StatusPaymentRequired, "payment_required", "Upgrade to the paid plan to use Domain Overview.")
				return
			}
		}
		result, err := fn(w, r, organizationID, r.PathValue("projectId"))
		invalid, isInvalid := errors.AsType[badRequest](err)
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, result)
		case isInvalid:
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", invalid.Error())
		case r.Context().Err() != nil:
			d.Logger.InfoContext(r.Context(), "domain request canceled", "err", err)
		case errors.Is(err, dataforseo.ErrBillingIssue):
			d.Logger.ErrorContext(r.Context(), "DataForSEO billing issue", "err", err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "provider_billing_issue", "Domain data is temporarily unavailable. Please try again later.")
		default:
			d.Logger.ErrorContext(r.Context(), "domain request failed", "err", err)
			httpx.WriteError(w, http.StatusBadGateway, "provider_error", "Domain data is temporarily unavailable. Please try again.")
		}
	}
}

func overviewHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, organizationID, projectID string) (any, error) {
		var req requestBase
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		if err := validateBase(req); err != nil {
			return nil, err
		}
		return d.Service.Overview(r.Context(), organizationID, projectID, req.Domain, Scope(req.Scope), locationCode(req), languageCode(req))
	})
}

func keywordSuggestionsHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, organizationID, projectID string) (any, error) {
		var req requestBase
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		if err := validateBase(req); err != nil {
			return nil, err
		}
		return d.Service.KeywordSuggestions(r.Context(), organizationID, projectID, req.Domain, Scope(req.Scope), locationCode(req), languageCode(req))
	})
}

func keywordsPageHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, organizationID, projectID string) (any, error) {
		var req pageRequest
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		if err := validatePageRequest(req); err != nil {
			return nil, err
		}
		if err := validateFilters(req.Filters); err != nil {
			return nil, err
		}
		return d.Service.KeywordsPage(r.Context(), organizationID, KeywordsPageInput{
			ProjectID: projectID, Domain: req.Domain, Scope: Scope(req.Scope), LocationCode: locationCode(req.requestBase),
			LanguageCode: languageCode(req.requestBase), Page: req.Page, PageSize: req.PageSize,
			SortMode: req.SortMode, SortOrder: req.SortOrder, Filters: req.Filters, Search: req.Search,
		})
	})
}

func pagesPageHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, organizationID, projectID string) (any, error) {
		var req pageRequest
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		if err := validatePageRequest(req); err != nil {
			return nil, err
		}
		if err := validateFilters(req.Filters); err != nil {
			return nil, err
		}
		return d.Service.PagesPage(r.Context(), organizationID, PagesPageInput{
			ProjectID: projectID, Domain: req.Domain, Scope: Scope(req.Scope), LocationCode: locationCode(req.requestBase),
			LanguageCode: languageCode(req.requestBase), Page: req.Page, PageSize: req.PageSize,
			SortMode: req.SortMode, SortOrder: req.SortOrder, Filters: req.Filters, Search: req.Search,
		})
	})
}

func validateBase(req requestBase) error {
	if strings.TrimSpace(req.Domain) == "" {
		return badRequest("Domain is required.")
	}
	if utf16Length(req.Domain) > 2048 {
		return badRequest("domain must be at most 2048 characters.")
	}
	if req.Scope != "" && req.Scope != string(ScopeDomain) && req.Scope != string(ScopeSubdomains) && req.Scope != string(ScopeSubfolder) && req.Scope != string(ScopeExactURL) {
		return badRequest("scope is invalid.")
	}
	if req.LocationCode != nil && *req.LocationCode <= 0 {
		return badRequest("locationCode must be a positive number.")
	}
	if req.LanguageCode != nil && (utf16Length(*req.LanguageCode) < 2 || utf16Length(*req.LanguageCode) > 8) {
		return badRequest("languageCode must be 2 to 8 characters.")
	}
	return nil
}

func validatePageRequest(req pageRequest) error {
	if err := validateBase(req.requestBase); err != nil {
		return err
	}
	if req.Page < 0 || req.Page > maxPage {
		return badRequest("page must be between 1 and 1000000.")
	}
	if req.PageSize != 0 && req.PageSize != 50 && req.PageSize != 100 && req.PageSize != 200 {
		return badRequest("pageSize must be 50, 100 or 200.")
	}
	return nil
}

func validateFilters(f keywordFilters) error {
	for _, value := range []string{f.Include, f.Exclude} {
		if utf16Length(value) > 500 {
			return badRequest("filter text must be at most 500 characters.")
		}
		if len(parseTerms(value)) > maxFilterConditions {
			return badRequest("a filter may have at most 8 terms.")
		}
	}
	ranges := [][2]*float64{{f.MinTraffic, f.MaxTraffic}, {f.MinVol, f.MaxVol}, {f.MinCPC, f.MaxCPC}, {f.MinKD, f.MaxKD}, {f.MinRank, f.MaxRank}}
	for _, bounds := range ranges {
		for _, value := range bounds {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0) {
				return badRequest("numeric filters must be finite and non-negative.")
			}
		}
		if bounds[0] != nil && bounds[1] != nil && *bounds[0] > *bounds[1] {
			return badRequest("filter minimum must be less than or equal to its maximum.")
		}
	}
	return nil
}

func locationCode(req requestBase) int {
	if req.LocationCode == nil {
		return defaultLocationCode
	}
	return *req.LocationCode
}
func languageCode(req requestBase) string {
	if req.LanguageCode == nil || strings.TrimSpace(*req.LanguageCode) == "" {
		return defaultLanguageCode
	}
	return strings.TrimSpace(*req.LanguageCode)
}
func utf16Length(value string) int { return len(utf16.Encode([]rune(value))) }
