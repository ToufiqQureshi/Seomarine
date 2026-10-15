// Package httpapi wires the HTTP routes.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/toufiqqureshi/uplumo/backend/internal/activation"
	"github.com/toufiqqureshi/uplumo/backend/internal/ahrefs"
	"github.com/toufiqqureshi/uplumo/backend/internal/aisearch"
	"github.com/toufiqqureshi/uplumo/backend/internal/analytics"
	"github.com/toufiqqureshi/uplumo/backend/internal/apikeys"
	"github.com/toufiqqureshi/uplumo/backend/internal/audit"
	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/backlinks"
	"github.com/toufiqqureshi/uplumo/backend/internal/billing"
	"github.com/toufiqqureshi/uplumo/backend/internal/branding"
	"github.com/toufiqqureshi/uplumo/backend/internal/contenteditor"
	"github.com/toufiqqureshi/uplumo/backend/internal/crawleraccess"
	"github.com/toufiqqureshi/uplumo/backend/internal/dashboardoverview"
	"github.com/toufiqqureshi/uplumo/backend/internal/domain"
	"github.com/toufiqqureshi/uplumo/backend/internal/emailauth"
	"github.com/toufiqqureshi/uplumo/backend/internal/ga4"
	"github.com/toufiqqureshi/uplumo/backend/internal/google"
	"github.com/toufiqqureshi/uplumo/backend/internal/gsc"
	"github.com/toufiqqureshi/uplumo/backend/internal/keywords"
	"github.com/toufiqqureshi/uplumo/backend/internal/localseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/mcp"
	"github.com/toufiqqureshi/uplumo/backend/internal/onboarding"
	planCatalog "github.com/toufiqqureshi/uplumo/backend/internal/plans"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
	"github.com/toufiqqureshi/uplumo/backend/internal/projectcontext"
	"github.com/toufiqqureshi/uplumo/backend/internal/projects"
	"github.com/toufiqqureshi/uplumo/backend/internal/ranktracking"
	"github.com/toufiqqureshi/uplumo/backend/internal/reports"
	"github.com/toufiqqureshi/uplumo/backend/internal/sam"
	"github.com/toufiqqureshi/uplumo/backend/internal/site"
	"github.com/toufiqqureshi/uplumo/backend/internal/sitepublish"
	"github.com/toufiqqureshi/uplumo/backend/internal/team"
)

// Pinger reports whether a dependency is reachable. *pgxpool.Pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// PingFunc adapts a function to Pinger, e.g. a Redis client's ping.
type PingFunc func(ctx context.Context) error

// Ping calls f.
func (f PingFunc) Ping(ctx context.Context) error { return f(ctx) }

// Deps are the dependencies of the root handler.
type Deps struct {
	Logger *slog.Logger
	DB     Pinger
	Redis  Pinger
	Auth   *auth.Service
	// Activation records dashboard setup progress.
	Activation *activation.Service
	Analytics  *analytics.Service
	// Billing is nil when Razorpay is not configured; paid feature gates then
	// retain their existing self-hosted behavior.
	Billing *billing.Service
	// BillingRoutes serves read-only plan and credit status even without payments.
	BillingRoutes *billing.Service
	// Branding is the active organization's white-label report branding.
	Branding *branding.Service
	// OpenAIAppsChallenge is served at /.well-known/openai-apps-challenge.
	OpenAIAppsChallenge string
	// AISearch is nil when no DataForSEO key is configured; its routes then
	// answer 503.
	AISearch *aisearch.Service
	// Ahrefs serves optional, free domain-rating lookups.
	Ahrefs *ahrefs.Service
	// Backlinks is nil when no DataForSEO key is configured.
	Backlinks *backlinks.Service
	// Domain is nil when no DataForSEO key is configured.
	Domain            *domain.Service
	DashboardOverview *dashboardoverview.Service
	// GoogleAccounts removes a user's Google data grants and mappings.
	GoogleAccounts google.AccountRepository
	// GoogleOAuth owns consent start and callback routes.
	GoogleOAuth *google.OAuthService
	// GA4 serves read-only Google Analytics reports; nil answers 503.
	GA4      *ga4.Service
	GA4Setup *ga4.ConnectionOperations
	// GSC serves read-only Search Console performance reports.
	GSC *gsc.Service
	// GSCConnections serves project property selection and status.
	GSCConnections *gsc.ConnectionOperations
	// ProjectMarkets reads the authorized project's default market for domain lookups.
	ProjectMarkets domain.ProjectMarkets
	// Locations serves the authenticated city and region picker.
	Locations *keywords.LocationService
	// Audit is nil when the audit engine is not configured; its routes then
	// answer 503.
	Audit *audit.Service
	// KeywordResearch is nil when no DataForSEO key is configured; research,
	// SERP and refresh routes then answer 503.
	KeywordResearch *keywords.ResearchService
	LocalSEO        *localseo.Service
	// SavedKeywords serves saved keywords, their metrics and tags.
	SavedKeywords  *keywords.SavedService
	ProjectContext *projectcontext.Service
	// RankTracking serves rank tracking configs and keywords. City-level
	// configs answer 503 until the provider city check is wired in.
	RankTracking *ranktracking.Service
	// RankChecks is nil when no DataForSEO key is configured; the check route
	// then answers 503.
	RankChecks *ranktracking.Checks
	// Projects serves organization-scoped project management.
	Projects            *projects.Service
	ContentEditor       *contenteditor.Repository
	ContentWriter       contenteditor.WriterService
	ContentImages       contenteditor.ImageStore
	ContentImageLimiter contenteditor.ImageUploadLimiter
	// SitePublish connects a project's website and publishes drafts to it.
	SitePublish *sitepublish.Service
	// SAMSessions serves the project chat-session registry.
	SAMSessions *sam.Service
	// EmailAuth serves sign-up, sign-in, verification and password reset; nil
	// leaves those routes with the legacy app.
	EmailAuth *emailauth.Service
	// APIKeys manages MCP API keys; nil leaves those routes with the legacy app.
	APIKeys *apikeys.Service
	// SAMChat serves SAM conversations; nil answers 503 (no OpenRouter key).
	SAMChat *sam.Chat
	// Onboarding stores account-scoped signup answers and the Search Console nudge.
	Onboarding    *onboarding.Service
	Team          *team.Service
	CrawlerAccess *crawleraccess.Service
	// Reports owns project report documents, templates, and sharing.
	Reports                 *reports.Service
	PublicURL               *url.URL
	DataForSEOConfigured    bool
	OpenRouterConfigured    bool
	ContentWriterConfigured bool
	Credits                 *billing.CreditLedgerService
	// MCP serves API-key and Go-issued OAuth requests through the Go dispatcher.
	MCP *mcp.Deps
	// Site is the public landing and pricing pages.
	Site *site.Site
	// StaticDir is the built React app that serves every route not listed here.
	StaticDir         string
	TrustedProxyCIDRs []netip.Prefix
}

const (
	readinessTimeout = 2 * time.Second
)

// NewHandler returns the root handler.
//
// /healthz is liveness: the process is up and serving. /readyz is readiness:
// Postgres and Redis answer, so a load balancer can stop routing to an
// instance that has lost one without restarting it.
//
// /t.js and /collect are the public analytics tracker and its ingest, and
// /webhooks/razorpay is authenticated by its signature, not a session.
//
// GET /pricing and the landing page are public marketing pages. The landing
// page lives at GET /, where signed-in users get the app instead.
//
// Everything under /api/v1/ needs a session, and everything under
// /api/v1/projects/{projectId}/ also needs membership of the project's
// organization, so a route added there cannot forget either check. Every
// other request goes to the legacy app.
func NewHandler(d Deps) http.Handler {
	mux := http.NewServeMux()

	// Says only that the server answers; it reveals no configuration.
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()
		for name, dep := range map[string]Pinger{"postgres": d.DB, "redis": d.Redis} {
			if err := dep.Ping(ctx); err != nil {
				d.Logger.WarnContext(ctx, "readiness check failed", "dependency", name, "err", err)
				httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
				return
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	app := spaHandler(d.StaticDir)

	mux.HandleFunc("GET /{$}", landing(d.Logger, d.Auth, d.Site, app))
	mux.HandleFunc("GET /pricing", d.Site.ServePricing)
	for _, path := range site.InfoPaths() {
		mux.HandleFunc("GET "+path, d.Site.ServeInfo)
	}
	mux.HandleFunc("GET /blog", d.Site.ServeBlog)
	mux.HandleFunc("GET /blog/{slug}", d.Site.ServeBlogPost)
	mux.HandleFunc("GET /robots.txt", d.Site.ServeRobots)
	mux.HandleFunc("GET /sitemap.xml", d.Site.ServeSitemap)
	mux.HandleFunc("GET "+site.AssetsPrefix, d.Site.ServeAsset)

	mux.HandleFunc("GET /t.js", serveTracker())
	mux.HandleFunc("GET /.well-known/openai-apps-challenge", func(w http.ResponseWriter, r *http.Request) {
		if d.OpenAIAppsChallenge == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, d.OpenAIAppsChallenge)
	})
	withSession := func(next http.Handler) http.Handler { return requireSession(d.Logger, d.Auth, next) }
	auth.MountOrganization(mux, d.Auth, withSession)
	auth.MountCurrentSession(mux, d.Auth, d.Logger)
	auth.MountSession(mux, d.Auth, d.Logger)
	auth.MountBetterAuthOrganization(mux, d.Auth, d.Logger)
	if d.EmailAuth != nil {
		emailauth.Mount(mux, d.EmailAuth)
	}
	if d.APIKeys != nil {
		apikeys.Mount(mux, d.APIKeys)
	}
	team.Mount(mux, d.Team, withSession)
	crawleraccess.Mount(mux, d.CrawlerAccess, withSession, func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) })
	mux.Handle("GET /api/v1/config/seo-api-key-status", withSession(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"configured": d.DataForSEOConfigured})
	})))
	mux.Handle("GET /api/v1/config/content-writer-status", withSession(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		configured := d.ContentWriterConfigured
		message := ""
		if !configured {
			message = "AI writing is temporarily unavailable because its approved provider model or API key is not configured."
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"configured": configured, "message": message})
	})))
	withSAMProjectAccess := func(next http.Handler) http.Handler {
		return withSession(requireProjectAccess(d.Logger, d.Auth, next))
	}
	mux.Handle("POST /api/v1/projects/{projectId}/sam/access-setup-status", withSAMProjectAccess(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		dec.DisallowUnknownFields()
		if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "Choose a valid SAM setup request.")
			return
		}
		enabled := d.OpenRouterConfigured
		var message *string
		if !enabled {
			text := "SAM is temporarily unavailable. Please try again in a few minutes."
			message = &text
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "errorMessage": message})
	})))
	analytics.Mount(mux, analytics.Deps{
		Logger:            d.Logger,
		Service:           d.Analytics,
		TrustedProxyCIDRs: d.TrustedProxyCIDRs,
		WithSession:       withSession,
		WithProjectAccess: func(next http.Handler) http.Handler {
			return requireProjectAccess(d.Logger, d.Auth, next)
		},
	})
	billingRoutes := d.BillingRoutes
	if billingRoutes == nil {
		billingRoutes = d.Billing
	}
	billing.Mount(mux, billing.Deps{Logger: d.Logger, Service: billingRoutes, WithSession: withSession, Credits: d.Credits})
	// Without billing the server has no plans to gate on, so every feature
	// below is open.
	var brandingPlans branding.PaidPlans
	if d.Billing != nil {
		brandingPlans = d.Billing.Gate(planCatalog.WhiteLabel)
	}
	branding.Mount(mux, branding.Deps{Logger: d.Logger, Service: d.Branding, WithSession: withSession, Plans: brandingPlans})
	var plans aisearch.PaidPlans
	if d.Billing != nil {
		plans = d.Billing.Gate(planCatalog.AIVisibility)
	}
	aisearch.Mount(mux, aisearch.Deps{
		Logger:      d.Logger,
		Service:     d.AISearch,
		Plans:       plans,
		WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler {
			return requireProjectAccess(d.Logger, d.Auth, next)
		},
	})
	var backlinkPlans backlinks.PaidPlans
	if d.Billing != nil {
		backlinkPlans = d.Billing.Gate(planCatalog.Backlinks)
	}
	backlinks.Mount(mux, backlinks.Deps{
		Logger: d.Logger, Service: d.Backlinks, Plans: backlinkPlans, WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	ahrefs.Mount(mux, d.Ahrefs, withSession, func(next http.Handler) http.Handler {
		return requireProjectAccess(d.Logger, d.Auth, next)
	})
	var domainPlans domain.PaidPlans
	if d.Billing != nil {
		domainPlans = d.Billing.Gate(planCatalog.DomainResearch)
	}
	domain.Mount(mux, domain.Deps{
		Logger: d.Logger, Service: d.Domain, Plans: domainPlans, ProjectMarkets: d.ProjectMarkets, WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	dashboardoverview.Mount(mux, dashboardoverview.Deps{Logger: d.Logger, Service: d.DashboardOverview, WithSession: withSession, WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) }})
	keywords.MountSaved(mux, keywords.SavedDeps{
		Logger: d.Logger, Service: d.SavedKeywords, ProjectMarkets: d.ProjectMarkets, WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	var researchPlans keywords.PaidPlans
	if d.Billing != nil {
		researchPlans = d.Billing.Gate(planCatalog.Keywords)
	}
	keywords.MountResearch(mux, keywords.ResearchDeps{
		Logger: d.Logger, Service: d.KeywordResearch, Saved: d.SavedKeywords, ProjectMarkets: d.ProjectMarkets, Plans: researchPlans,
		WithSession:       withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	keywords.MountLocations(mux, keywords.LocationDeps{Logger: d.Logger, Service: d.Locations, WithSession: withSession})
	localseo.Mount(mux, d.LocalSEO, withSession, func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) })
	ranktracking.Mount(mux, ranktracking.Deps{
		Logger: d.Logger, Service: d.RankTracking, Checks: d.RankChecks, ProjectMarkets: d.ProjectMarkets, WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	activation.Mount(mux, activation.Deps{Logger: d.Logger, Service: d.Activation, WithSession: withSession, WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) }})
	reports.Mount(mux, reports.Deps{Logger: d.Logger, Service: d.Reports, PublicURL: d.PublicURL, WithSession: withSession,
		View:              reportViewDeps(d),
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	projects.Mount(mux, projects.Deps{Logger: d.Logger, Service: d.Projects, WithSession: withSession, WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) }})
	if d.ContentEditor != nil {
		contenteditor.Mount(mux, d.ContentEditor, d.ContentWriter, d.ContentImages, d.ContentImageLimiter, withSession, func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) })
	}
	var publishPlans sitepublish.PaidPlans
	if d.Billing != nil {
		publishPlans = d.Billing.Gate(planCatalog.Publishing)
	}
	sitepublish.Mount(mux, sitepublish.Deps{
		Logger: d.Logger, Service: d.SitePublish, Plans: publishPlans, WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	sam.Mount(mux, sam.Deps{
		Logger: d.Logger, Service: d.SAMSessions, WithSession: withSession, Chat: d.SAMChat,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	onboarding.Mount(mux, onboarding.Deps{Logger: d.Logger, Service: d.Onboarding, WithSession: withSession})
	audit.Mount(mux, audit.Deps{
		Logger: d.Logger, Service: d.Audit, WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	google.MountAccounts(mux, google.AccountDeps{
		Repository:  d.GoogleAccounts,
		Logger:      d.Logger,
		WithSession: withSession,
	})
	var googlePlans google.PaidPlans
	if d.Billing != nil {
		googlePlans = d.Billing.Gate(planCatalog.GoogleData)
	}
	google.MountOAuth(mux, google.OAuthDeps{Service: d.GoogleOAuth, Auth: d.Auth, Logger: d.Logger, WithSession: withSession, Plans: googlePlans})
	ga4.Mount(mux, ga4.Deps{Logger: d.Logger, Service: d.GA4, Setup: d.GA4Setup, WithSession: withSession,
		SearchConsole:     d.GSC,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	gsc.MountPerformance(mux, gsc.PerformanceDeps{
		Logger: d.Logger, Service: d.GSC, WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	gsc.MountConnections(mux, gsc.ConnectionDeps{
		Logger: d.Logger, Operations: d.GSCConnections, WithSession: withSession,
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	if d.MCP != nil {
		deps := *d.MCP
		if deps.Logger == nil {
			deps.Logger = d.Logger
		}
		mcp.Mount(mux, deps)
	}

	api := http.NewServeMux()
	projectcontext.Mount(api, projectcontext.HandlerDeps{
		Logger: d.Logger, Service: d.ProjectContext,
		WithSession:       func(next http.Handler) http.Handler { return next },
		WithProjectAccess: func(next http.Handler) http.Handler { return requireProjectAccess(d.Logger, d.Auth, next) },
	})
	api.HandleFunc("/", notFound())
	mux.Handle("/api/v1/", requireSession(d.Logger, d.Auth, api))

	mux.Handle("/", app)

	return httpx.RequestLogging(d.Logger)(httpx.Recover(d.Logger)(mux))
}

// requireSession answers 401 unless the request carries a valid session,
// and passes the session's user to next in the request context.
func requireSession(logger *slog.Logger, authn *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := authn.Authenticate(r)
		if errors.Is(err, auth.ErrUnauthenticated) {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "authenticate request", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), user)))
	})
}

// requireProjectAccess answers 404 unless the signed-in user is a member of
// the organization that owns the {projectId} in the path, and passes that
// organization to next in the request context. It runs inside requireSession;
// without a user in the context it fails closed, because no member row has an
// empty user id.
func requireProjectAccess(logger *slog.Logger, authz *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.UserFromContext(r.Context())
		orgID, err := authz.AuthorizeProject(r.Context(), user.ID, r.PathValue("projectId"))
		if errors.Is(err, auth.ErrProjectNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "project_not_found", "Project not found.")
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "authorize project", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "Something went wrong. Please try again.")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithProjectOrganization(r.Context(), orgID)))
	})
}

// landing serves the landing page to visitors without a valid session and
// the app to everyone else. A session lookup that fails for another reason,
// such as a database error, comes from a correctly signed cookie, so the
// visitor is most likely signed in and goes to the app.
func landing(logger *slog.Logger, authn *auth.Service, pages *site.Site, app http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The response depends on the session cookie, so caches must key on it.
		w.Header().Add("Vary", "Cookie")
		_, err := authn.Authenticate(r)
		if errors.Is(err, auth.ErrUnauthenticated) {
			pages.ServeLanding(w, r)
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "authenticate landing visitor", "err", err)
		}
		app.ServeHTTP(w, r)
	}
}

func notFound() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "No such endpoint.")
	}
}

// reportViewDeps wires the member report viewer and share page to the session,
// project authorization and branding services.
func reportViewDeps(d Deps) reports.ViewDeps {
	v := reports.ViewDeps{Authenticate: d.Auth.Authenticate, AuthorizeProject: d.Auth.AuthorizeProject}
	if d.Branding != nil {
		v.Branding = d.Branding.Get
	}
	return v
}
