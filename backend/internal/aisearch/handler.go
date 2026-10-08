package aisearch

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const (
	maxBody = 8 << 10

	maxQueryChars       = 250
	maxCompetitors      = 5
	maxPromptChars      = 500
	defaultLocationCode = 2840
	defaultLanguageCode = "en"
)

// PaidPlans says whether an organization is on a paid plan.
type PaidPlans interface {
	HasPaidPlan(ctx context.Context, organizationID string) (bool, error)
}

// Deps contains the AI search service and the root router's middleware.
type Deps struct {
	Logger  *slog.Logger
	Service *Service
	// Plans gates the endpoints behind a paid plan, because every uncached call
	// is billed. Nil lets every organization in, for deployments that have no
	// billing and pay the data provider themselves.
	Plans             PaidPlans
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers the AI search routes on mux. Service may be nil when no
// DataForSEO key is configured; the routes then answer 503.
func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	mux.Handle("POST /api/v1/projects/{projectId}/ai-search/brand-lookup", protect(brandLookupHandler(d)))
	mux.Handle("POST /api/v1/projects/{projectId}/ai-search/prompt-explorer", protect(promptExplorerHandler(d)))
}

type brandLookupRequest struct {
	Query        string   `json:"query"`
	Competitors  []string `json:"competitors"`
	Scope        Scope    `json:"scope"`
	LocationCode *int     `json:"locationCode"`
	LanguageCode *string  `json:"languageCode"`
}

// validate trims and checks the request and fills in the defaults.
func (r brandLookupRequest) validate() (BrandLookupInput, string) {
	in := BrandLookupInput{
		Query:        strings.TrimSpace(r.Query),
		Competitors:  []string{},
		Scope:        r.Scope,
		LocationCode: defaultLocationCode,
		LanguageCode: defaultLanguageCode,
	}
	if n := utf16Len(in.Query); n < 1 || n > maxQueryChars {
		return in, "query must be 1 to 250 characters."
	}
	if len(r.Competitors) > maxCompetitors {
		return in, "competitors must have at most 5 entries."
	}
	for _, c := range r.Competitors {
		c = strings.TrimSpace(c)
		if n := utf16Len(c); n < 1 || n > maxQueryChars {
			return in, "each competitor must be 1 to 250 characters."
		}
		in.Competitors = append(in.Competitors, c)
	}
	if r.Scope != "" && !r.Scope.valid() {
		return in, "scope must be exact_url, subfolder, domain or subdomains."
	}
	if r.LocationCode != nil {
		if *r.LocationCode <= 0 {
			return in, "locationCode must be a positive number."
		}
		in.LocationCode = *r.LocationCode
	}
	if r.LanguageCode != nil {
		if n := utf16Len(*r.LanguageCode); n < 2 || n > 8 {
			return in, "languageCode must be 2 to 8 characters."
		}
		in.LanguageCode = *r.LanguageCode
	}
	return in, ""
}

type promptExplorerRequest struct {
	Prompt               string   `json:"prompt"`
	Models               []string `json:"models"`
	HighlightBrand       *string  `json:"highlightBrand"`
	WebSearch            *bool    `json:"webSearch"`
	WebSearchCountryCode string   `json:"webSearchCountryCode"`
}

// validate trims and checks the request and fills in the defaults.
func (r promptExplorerRequest) validate() (PromptExplorerInput, string) {
	in := PromptExplorerInput{Prompt: strings.TrimSpace(r.Prompt), WebSearch: true}
	if n := utf16Len(in.Prompt); n < 1 || n > maxPromptChars {
		return in, "prompt must be 1 to 500 characters."
	}
	if len(r.Models) < 1 || len(r.Models) > len(promptModels) {
		return in, "models must have 1 to 4 entries."
	}
	for _, m := range r.Models {
		if !slices.Contains(promptModels, m) {
			return in, "models may only be chat_gpt, claude, gemini or perplexity."
		}
		// A repeated model would fan out to a second paid call for the same answer.
		if !slices.Contains(in.Models, m) {
			in.Models = append(in.Models, m)
		}
	}
	if r.HighlightBrand != nil {
		in.HighlightBrand = strings.TrimSpace(*r.HighlightBrand)
		if n := utf16Len(in.HighlightBrand); n < 1 || n > maxQueryChars {
			return in, "highlightBrand must be 1 to 250 characters."
		}
	}
	if r.WebSearch != nil {
		in.WebSearch = *r.WebSearch
	}
	if r.WebSearchCountryCode != "" {
		if !slices.Contains(webSearchCountries, r.WebSearchCountryCode) {
			return in, "webSearchCountryCode must be an ISO 3166-1 alpha-2 country code in capitals."
		}
		in.WebSearchCountryCode = r.WebSearchCountryCode
	}
	return in, ""
}

func brandLookupHandler(d Deps) http.HandlerFunc {
	return handle(d, func(r *http.Request, organizationID, projectID string) (any, error) {
		var req brandLookupRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		in, msg := req.validate()
		if msg != "" {
			return nil, badRequest(msg)
		}
		return d.Service.BrandLookup(r.Context(), organizationID, projectID, in)
	})
}

func promptExplorerHandler(d Deps) http.HandlerFunc {
	return handle(d, func(r *http.Request, organizationID, projectID string) (any, error) {
		var req promptExplorerRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		in, msg := req.validate()
		if msg != "" {
			return nil, badRequest(msg)
		}
		return d.Service.ExplorePrompt(r.Context(), organizationID, projectID, in)
	})
}

// badRequest is a request the caller must fix.
type badRequest string

func (e badRequest) Error() string { return string(e) }

func decode(r *http.Request, v any) error {
	err := httpx.DecodeJSONLimit(r.Body, maxBody, v)
	if errors.Is(err, httpx.ErrPayloadTooLarge) {
		return badRequest("The request is too large.")
	}
	if err != nil {
		return badRequest("The request is not valid JSON.")
	}
	return nil
}

// handle runs fn for the signed-in user's project after the plan check and
// turns its result or error into the response.
func handle(d Deps, fn func(r *http.Request, organizationID, projectID string) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "ai_search_unavailable", "AI search is not set up on this server.")
			return
		}
		organizationID, ok := auth.ProjectOrganizationFromContext(r.Context())
		if !ok {
			// requireProjectAccess always sets it; failing closed keeps a route
			// mounted without the middleware from running unattributed.
			d.Logger.ErrorContext(r.Context(), "ai search request without project organization")
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		if d.Plans != nil {
			included, err := d.Plans.HasPaidPlan(r.Context(), organizationID)
			if err != nil {
				d.Logger.ErrorContext(r.Context(), "check ai search plan", "err", err)
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
				return
			}
			if !included {
				httpx.WriteError(w, http.StatusPaymentRequired, "payment_required", "Upgrade to the paid plan to use AI Visibility.")
				return
			}
		}

		result, err := fn(r, organizationID, r.PathValue("projectId"))
		var invalid badRequest
		var input inputError
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, result)
		case errors.As(err, &invalid):
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", invalid.Error())
		case errors.As(err, &input):
			httpx.WriteError(w, http.StatusBadRequest, "invalid_input", input.Error())
		case r.Context().Err() != nil:
			// The client went away; there is nobody to answer.
			d.Logger.InfoContext(r.Context(), "ai search request canceled", "err", err)
		case errors.Is(err, dataforseo.ErrBillingIssue):
			d.Logger.ErrorContext(r.Context(), "data provider account needs attention", "err", err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "provider_billing_issue", "AI search data is temporarily unavailable. Please try again later.")
		default:
			d.Logger.ErrorContext(r.Context(), "ai search", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
		}
	}
}
