package keywords

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/market"
)

// PaidPlans says whether an organization may use billed keyword data.
type PaidPlans interface {
	HasPaidPlan(ctx context.Context, organizationID string) (bool, error)
}

// ResearchDeps are the dependencies of the research, SERP and refresh routes.
type ResearchDeps struct {
	Logger            *slog.Logger
	Service           *ResearchService
	Saved             *SavedService
	ProjectMarkets    ProjectMarkets
	Plans             PaidPlans // nil runs ungated, like a self-hosted deployment
	WithSession       func(http.Handler) http.Handler
	WithProjectAccess func(http.Handler) http.Handler
}

// MountResearch registers the routes that spend provider credits. Every route
// is a POST with a JSON body and rejects unknown fields.
func MountResearch(mux *http.ServeMux, d ResearchDeps) {
	protect := func(h http.Handler) http.Handler { return d.WithSession(d.WithProjectAccess(h)) }
	saved := SavedDeps{Logger: d.Logger, Service: d.Saved, ProjectMarkets: d.ProjectMarkets}
	base := "POST /api/v1/projects/{projectId}/keywords/"
	for path, fn := range map[string]func(savedRequest, ResearchDeps) (any, error){
		"research":      research,
		"serp":          serpAnalysis,
		"saved/refresh": refreshSaved,
	} {
		mux.Handle(base+path, protect(handleBilled(saved, d, fn)))
	}
}

func handleBilled(saved SavedDeps, d ResearchDeps, fn func(savedRequest, ResearchDeps) (any, error)) http.HandlerFunc {
	return handleSaved(saved, func(q savedRequest) (any, error) {
		if d.Service == nil {
			return nil, errResearchUnavailable
		}
		if d.Plans != nil {
			paid, err := d.Plans.HasPaidPlan(q.r.Context(), q.organizationID)
			if err != nil {
				return nil, err
			}
			if !paid {
				return nil, errPaymentRequired
			}
		}
		return fn(q, d)
	})
}

var (
	errResearchUnavailable = errors.New("keyword research is not available")
	errPaymentRequired     = errors.New("a paid plan is required")
)

// marketBody is the market part of a research request.
type marketBody struct {
	LocationCode int     `json:"locationCode"`
	LanguageCode string  `json:"languageCode"`
	LocationName *string `json:"locationName"`
}

func (b marketBody) validate() error {
	if b.LanguageCode != "" && (len(b.LanguageCode) < 2 || len(b.LanguageCode) > 8) {
		return ValidationError("Location or language is not valid.")
	}
	if b.LocationName != nil {
		trimmed := strings.TrimSpace(*b.LocationName)
		if trimmed == "" || units(trimmed) > 200 {
			return ValidationError("Location name is 1 to 200 characters.")
		}
		*b.LocationName = trimmed
	}
	return nil
}

func research(q savedRequest, d ResearchDeps) (any, error) {
	var body struct {
		marketBody
		Keywords      []string `json:"keywords"`
		ResultLimit   int      `json:"resultLimit"`
		Mode          string   `json:"mode"`
		Clickstream   bool     `json:"clickstream"`
		GroupKeywords bool     `json:"groupKeywords"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if len(body.Keywords) == 0 || len(body.Keywords) > MaxResearchSeeds || slices.Contains(body.Keywords, "") {
		return nil, ValidationError("Send between 1 and 200 keywords.")
	}
	if body.ResultLimit == 0 {
		body.ResultLimit = ResultLimits[0]
	}
	if body.Mode == "" {
		body.Mode = "auto"
	}
	if !slices.Contains(ResultLimits, body.ResultLimit) || !slices.Contains(Modes, body.Mode) {
		return nil, ValidationError("Result limit is 150, 300 or 500, and the mode is auto, related, suggestions or ideas.")
	}
	if err := body.validate(); err != nil {
		return nil, err
	}
	pair, err := q.resolveMarket(body.marketBody)
	if err != nil {
		return nil, err
	}
	return d.Service.Research(q.r.Context(), ResearchInput{
		OrganizationID: q.organizationID, ProjectID: q.projectID, Keywords: body.Keywords,
		LocationCode: pair.LocationCode, LanguageCode: pair.LanguageCode, LocationName: body.LocationName,
		ResultLimit: body.ResultLimit, Mode: body.Mode, Clickstream: body.Clickstream, GroupKeywords: body.GroupKeywords,
	})
}

func serpAnalysis(q savedRequest, d ResearchDeps) (any, error) {
	var body struct {
		marketBody
		Keyword string `json:"keyword"`
		Depth   int    `json:"depth"`
	}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	if strings.TrimSpace(body.Keyword) == "" {
		return nil, ValidationError("Enter a keyword.")
	}
	if body.Depth == 0 {
		body.Depth = SerpShallowDepth
	}
	if body.Depth != SerpShallowDepth && body.Depth != SerpDeepDepth {
		return nil, ValidationError("Depth is 20 or 100.")
	}
	if err := body.validate(); err != nil {
		return nil, err
	}
	pair, err := q.resolveMarket(body.marketBody)
	if err != nil {
		return nil, err
	}
	return d.Service.SerpAnalysis(q.r.Context(), SerpAnalysisInput{
		OrganizationID: q.organizationID, ProjectID: q.projectID, Keyword: body.Keyword,
		LocationCode: pair.LocationCode, LanguageCode: pair.LanguageCode, LocationName: body.LocationName, Depth: body.Depth,
	})
}

func refreshSaved(q savedRequest, d ResearchDeps) (any, error) {
	var body struct{}
	if err := q.decode(&body); err != nil {
		return nil, err
	}
	updated, err := d.Service.RefreshSavedMetrics(q.r.Context(), q.organizationID, q.projectID, d.Saved)
	if err != nil {
		return nil, err
	}
	return map[string]any{"updated": updated}, nil
}

func (q savedRequest) resolveMarket(b marketBody) (market.Pair, error) {
	projectMarket, err := q.d.ProjectMarkets.Get(q.r.Context(), q.organizationID, q.projectID)
	if err != nil {
		return market.Pair{}, err
	}
	return ResolveMarket(b.LocationCode, b.LanguageCode, projectMarket)
}

// isProviderError reports a failure that came from DataForSEO rather than ours.
func isProviderError(err error) bool {
	if errors.Is(err, dataforseo.ErrUpstreamUnavailable) {
		return true
	}
	if _, ok := errors.AsType[*dataforseo.TaskError](err); ok {
		return true
	}
	_, ok := errors.AsType[*dataforseo.HTTPError](err)
	return ok
}
