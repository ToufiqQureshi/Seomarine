// Package backlinks exposes the organization-scoped backlinks reporting API.
package backlinks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

const maxBody = 32 << 10

// PaidPlans checks whether an organization can use paid backlinks reports.
type PaidPlans interface {
	HasPaidPlan(context.Context, string) (bool, error)
}

// Deps contains the services and middleware required to mount backlinks routes.
type Deps struct {
	Logger            *slog.Logger
	Service           *Service
	Plans             PaidPlans
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// Mount registers the backlinks reporting routes on mux.
func Mount(mux *http.ServeMux, d Deps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	for _, route := range []struct {
		path string
		fn   func(Deps) http.HandlerFunc
	}{{"overview", overviewHandler}, {"backlinks", rowsHandler}, {"referring-domains", domainsHandler}, {"top-pages", pagesHandler}} {
		mux.Handle("POST /api/v1/projects/{projectId}/backlinks/"+route.path, protect(route.fn(d)))
	}
}

type lookupRequest struct {
	Target string `json:"target"`
	Scope  string `json:"scope"`
}
type pageRequest struct {
	Target    string `json:"target"`
	Scope     string `json:"scope"`
	Page      int    `json:"page"`
	PageSize  int    `json:"pageSize"`
	SortOrder string `json:"sortOrder"`
}
type rowsRequest struct {
	pageRequest
	HideSpam  *bool       `json:"hideSpam"`
	SortField string      `json:"sortField"`
	Filters   rowsFilters `json:"filters"`
	Mode      string      `json:"mode"`
}
type domainsRequest struct {
	pageRequest
	SortField string         `json:"sortField"`
	Filters   domainsFilters `json:"filters"`
}
type pagesRequest struct {
	pageRequest
	SortField string       `json:"sortField"`
	Filters   pagesFilters `json:"filters"`
}

func overviewHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, org string) (any, error) {
		var req lookupRequest
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		if _, err := normalizeTarget(req.Target, req.Scope); err != nil {
			return nil, err
		}
		return d.Service.Overview(r.Context(), org, lookupInput(req))
	})
}
func rowsHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, org string) (any, error) {
		var req rowsRequest
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		in, paging, err := validatePage(req.pageRequest, req.Target, req.Scope)
		if err != nil {
			return nil, err
		}
		if req.SortField == "" {
			req.SortField = "firstSeen"
		}
		sort, ok := map[string]string{"rank": "rank", "domainRank": "domain_from_rank", "spamScore": "backlink_spam_score", "firstSeen": "first_seen"}[req.SortField]
		if !ok {
			return nil, inputError("sortField is invalid.")
		}
		mode := req.Mode
		if mode == "" {
			mode = "one_per_domain"
		}
		if mode != "one_per_domain" && mode != "as_is" {
			return nil, inputError("mode must be one_per_domain or as_is.")
		}
		if err := validateRowsFilters(req.Filters); err != nil {
			return nil, err
		}
		hideSpam := true
		if req.HideSpam != nil {
			hideSpam = *req.HideSpam
		}
		target, err := normalizeTarget(req.Target, req.Scope)
		if err != nil {
			return nil, err
		}
		if countFilters(buildRowsFilters(req.Filters))+countFilters(scopeFilters("url_to", target))+boolInt(hideSpam)*2 > maxFilterConditions {
			return nil, inputError("Too many filter conditions (maximum 8).")
		}
		in.Target = req.Target
		in.Scope = req.Scope
		return d.Service.Rows(r.Context(), org, in, pageInput{Page: paging.Page, PageSize: paging.PageSize, Sort: sort + "," + paging.Sort, Mode: mode}, req.Filters, hideSpam)
	})
}
func domainsHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, org string) (any, error) {
		var req domainsRequest
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		in, paging, err := validatePage(req.pageRequest, req.Target, req.Scope)
		if err != nil {
			return nil, err
		}
		if req.SortField == "" {
			req.SortField = "backlinks"
		}
		sort, ok := map[string]string{"domain": "domain", "backlinks": "backlinks", "referringPages": "referring_pages", "rank": "rank", "spamScore": "backlinks_spam_score", "firstSeen": "first_seen", "brokenBacklinks": "broken_backlinks"}[req.SortField]
		if !ok {
			return nil, inputError("sortField is invalid.")
		}
		if err := validateDomainsFilters(req.Filters); err != nil {
			return nil, err
		}
		if countFilters(buildDomainsFilters(req.Filters)) > maxFilterConditions {
			return nil, inputError("Too many filter conditions (maximum 8).")
		}
		in.Target = req.Target
		in.Scope = req.Scope
		return d.Service.Domains(r.Context(), org, in, pageInput{Page: paging.Page, PageSize: paging.PageSize, Sort: sort + "," + paging.Sort}, req.Filters)
	})
}
func pagesHandler(d Deps) http.HandlerFunc {
	return handle(d, func(w http.ResponseWriter, r *http.Request, org string) (any, error) {
		var req pagesRequest
		if err := decode(w, r, &req); err != nil {
			return nil, err
		}
		in, paging, err := validatePage(req.pageRequest, req.Target, req.Scope)
		if err != nil {
			return nil, err
		}
		if req.SortField == "" {
			req.SortField = "backlinks"
		}
		sort, ok := map[string]string{"backlinks": "backlinks", "referringDomains": "referring_domains", "rank": "rank", "brokenBacklinks": "broken_backlinks"}[req.SortField]
		if !ok {
			return nil, inputError("sortField is invalid.")
		}
		if err := validatePagesFilters(req.Filters); err != nil {
			return nil, err
		}
		target, err := normalizeTarget(req.Target, req.Scope)
		if err != nil {
			return nil, err
		}
		if countFilters(buildPagesFilters(req.Filters))+countFilters(scopeFilters("url", target)) > maxFilterConditions {
			return nil, inputError("Too many filter conditions (maximum 8).")
		}
		in.Target = req.Target
		in.Scope = req.Scope
		return d.Service.Pages(r.Context(), org, in, pageInput{Page: paging.Page, PageSize: paging.PageSize, Sort: sort + "," + paging.Sort}, req.Filters)
	})
}

func validatePage(r pageRequest, target, scope string) (lookupInput, pageInput, error) {
	if _, err := normalizeTarget(target, scope); err != nil {
		return lookupInput{}, pageInput{}, err
	}
	if r.Page == 0 {
		r.Page = 1
	}
	if r.Page < 1 || r.Page > 1_000_000 {
		return lookupInput{}, pageInput{}, inputError("page must be between 1 and 1000000.")
	}
	if r.PageSize == 0 {
		r.PageSize = 50
	}
	if !slices.Contains([]int{50, 100, 200}, r.PageSize) {
		return lookupInput{}, pageInput{}, inputError("pageSize must be 50, 100 or 200.")
	}
	if r.SortOrder == "" {
		r.SortOrder = "desc"
	}
	if r.SortOrder != "asc" && r.SortOrder != "desc" {
		return lookupInput{}, pageInput{}, inputError("sortOrder must be asc or desc.")
	}
	if int64(r.Page-1) > math.MaxInt64/int64(r.PageSize) {
		return lookupInput{}, pageInput{}, inputError("page is too large.")
	}
	return lookupInput{Target: target, Scope: scope}, pageInput{Page: r.Page, PageSize: r.PageSize, Sort: r.SortOrder}, nil
}

func validateRowsFilters(f rowsFilters) error {
	if err := validateFilterStrings(f.Include, f.Exclude); err != nil {
		return err
	}
	if f.DomainFrom != "" && (len(f.DomainFrom) > 255 || strings.ContainsAny(f.DomainFrom, "\r\n")) {
		return inputError("domainFrom is invalid.")
	}
	if f.LinkType != "" && f.LinkType != "dofollow" && f.LinkType != "nofollow" {
		return inputError("linkType must be dofollow or nofollow.")
	}
	if err := validateRanges([]*float64{f.MinDomainRank, f.MaxDomainRank, f.MinLinkAuthority, f.MaxLinkAuthority, f.MinSpamScore, f.MaxSpamScore}); err != nil {
		return err
	}
	return validateRangePairs([2]*float64{f.MinDomainRank, f.MaxDomainRank}, [2]*float64{f.MinLinkAuthority, f.MaxLinkAuthority}, [2]*float64{f.MinSpamScore, f.MaxSpamScore})
}
func validateDomainsFilters(f domainsFilters) error {
	if err := validateFilterStrings(f.Include, f.Exclude); err != nil {
		return err
	}
	if err := validateRanges([]*float64{f.MinBacklinks, f.MaxBacklinks, f.MinRank, f.MaxRank, f.MinSpamScore, f.MaxSpamScore}); err != nil {
		return err
	}
	return validateRangePairs([2]*float64{f.MinBacklinks, f.MaxBacklinks}, [2]*float64{f.MinRank, f.MaxRank}, [2]*float64{f.MinSpamScore, f.MaxSpamScore})
}
func validatePagesFilters(f pagesFilters) error {
	if err := validateFilterStrings(f.Include, f.Exclude); err != nil {
		return err
	}
	if err := validateRanges([]*float64{f.MinBacklinks, f.MaxBacklinks, f.MinReferringDomains, f.MaxReferringDomains, f.MinRank, f.MaxRank}); err != nil {
		return err
	}
	return validateRangePairs([2]*float64{f.MinBacklinks, f.MaxBacklinks}, [2]*float64{f.MinReferringDomains, f.MaxReferringDomains}, [2]*float64{f.MinRank, f.MaxRank})
}
func validateFilterStrings(values ...string) error {
	for _, v := range values {
		if len(v) > 500 {
			return inputError("filter text must be at most 500 characters.")
		}
		if len(filterTerms(v)) > 8 {
			return inputError("a filter may have at most 8 terms.")
		}
	}
	return nil
}
func validateRanges(values []*float64) error {
	for _, v := range values {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0) {
			return inputError("numeric filters must be finite and non-negative.")
		}
	}
	return nil
}

func validateRangePairs(pairs ...[2]*float64) error {
	for _, pair := range pairs {
		if pair[0] != nil && pair[1] != nil && *pair[0] > *pair[1] {
			return inputError("filter minimum must be less than or equal to its maximum.")
		}
	}
	return nil
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return inputError("The request is too large.")
		}
		return inputError("The request is not valid JSON.")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return inputError("The request must contain exactly one JSON value.")
	}
	return nil
}
func handle(d Deps, fn func(http.ResponseWriter, *http.Request, string) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Service == nil {
			httpx.WriteError(w, http.StatusServiceUnavailable, "backlinks_unavailable", "Backlinks data is not set up on this server.")
			return
		}
		org, ok := auth.ProjectOrganizationFromContext(r.Context())
		if !ok {
			d.Logger.ErrorContext(r.Context(), "backlinks request without project organization")
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		if d.Plans != nil {
			paid, err := d.Plans.HasPaidPlan(r.Context(), org)
			if err != nil {
				d.Logger.ErrorContext(r.Context(), "check backlinks plan", "err", err)
				httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
				return
			}
			if !paid {
				httpx.WriteError(w, http.StatusPaymentRequired, "payment_required", "Upgrade to the paid plan to use Backlinks.")
				return
			}
		}
		out, err := fn(w, r, org)
		var invalid inputError
		switch {
		case err == nil:
			httpx.WriteJSON(w, http.StatusOK, out)
		case errors.As(err, &invalid):
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", invalid.Error())
		case r.Context().Err() != nil:
			d.Logger.InfoContext(r.Context(), "backlinks request canceled", "err", err)
		case errors.Is(err, dataforseo.ErrBillingIssue):
			d.Logger.ErrorContext(r.Context(), "DataForSEO billing issue", "err", err)
			httpx.WriteError(w, http.StatusServiceUnavailable, "provider_billing_issue", "Backlinks data is temporarily unavailable. Please try again later.")
		default:
			d.Logger.ErrorContext(r.Context(), "backlinks request failed", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
		}
	}
}
