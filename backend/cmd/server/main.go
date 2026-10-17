// Command server runs the Uplumo HTTP API.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/uplumo/backend/internal/activation"
	"github.com/toufiqqureshi/uplumo/backend/internal/ahrefs"
	"github.com/toufiqqureshi/uplumo/backend/internal/aisearch"
	"github.com/toufiqqureshi/uplumo/backend/internal/analytics"
	"github.com/toufiqqureshi/uplumo/backend/internal/analytics/geo"
	"github.com/toufiqqureshi/uplumo/backend/internal/apikeys"
	"github.com/toufiqqureshi/uplumo/backend/internal/audit"
	"github.com/toufiqqureshi/uplumo/backend/internal/auth"
	"github.com/toufiqqureshi/uplumo/backend/internal/backlinks"
	"github.com/toufiqqureshi/uplumo/backend/internal/billing"
	"github.com/toufiqqureshi/uplumo/backend/internal/branding"
	"github.com/toufiqqureshi/uplumo/backend/internal/config"
	"github.com/toufiqqureshi/uplumo/backend/internal/contentauthors"
	"github.com/toufiqqureshi/uplumo/backend/internal/contenteditor"
	"github.com/toufiqqureshi/uplumo/backend/internal/crawleraccess"
	"github.com/toufiqqureshi/uplumo/backend/internal/dashboardoverview"
	"github.com/toufiqqureshi/uplumo/backend/internal/database"
	"github.com/toufiqqureshi/uplumo/backend/internal/domain"
	"github.com/toufiqqureshi/uplumo/backend/internal/email"
	"github.com/toufiqqureshi/uplumo/backend/internal/emailauth"
	"github.com/toufiqqureshi/uplumo/backend/internal/ga4"
	"github.com/toufiqqureshi/uplumo/backend/internal/google"
	"github.com/toufiqqureshi/uplumo/backend/internal/gsc"
	"github.com/toufiqqureshi/uplumo/backend/internal/httpapi"
	"github.com/toufiqqureshi/uplumo/backend/internal/keywords"
	"github.com/toufiqqureshi/uplumo/backend/internal/kv"
	"github.com/toufiqqureshi/uplumo/backend/internal/linksuggest"
	"github.com/toufiqqureshi/uplumo/backend/internal/localseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/mcp"
	"github.com/toufiqqureshi/uplumo/backend/internal/onboarding"
	planCatalog "github.com/toufiqqureshi/uplumo/backend/internal/plans"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/errreport"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/httpx"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/jobs"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/openrouter"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/pgdb"
	"github.com/toufiqqureshi/uplumo/backend/internal/platform/spend"
	"github.com/toufiqqureshi/uplumo/backend/internal/projectcontext"
	"github.com/toufiqqureshi/uplumo/backend/internal/projects"
	"github.com/toufiqqureshi/uplumo/backend/internal/ranktracking"
	"github.com/toufiqqureshi/uplumo/backend/internal/razorpay"
	"github.com/toufiqqureshi/uplumo/backend/internal/reports"
	"github.com/toufiqqureshi/uplumo/backend/internal/sam"
	"github.com/toufiqqureshi/uplumo/backend/internal/site"
	"github.com/toufiqqureshi/uplumo/backend/internal/sitepublish"
	"github.com/toufiqqureshi/uplumo/backend/internal/team"
)

const (
	startupTimeout = 10 * time.Second
	// The first deploy creates every table, one statement per round trip, and
	// the database can be in another region.
	migrateTimeout  = 5 * time.Minute
	shutdownTimeout = 15 * time.Second
	// How long the last error reports get to leave before the process exits.
	reportFlushTimeout = 2 * time.Second
)

func main() {
	if err := run(slog.New(slog.NewJSONHandler(os.Stdout, nil))); err != nil {
		os.Exit(1)
	}
}

func run(logger *slog.Logger) (err error) {
	var reporter *errreport.Reporter
	// Logged here, not in main, so the reason the server stopped is reported
	// too and is delivered before the process exits.
	defer func() {
		if err != nil {
			logger.Error("server stopped", "err", err)
		}
		reporter.Flush(reportFlushTimeout)
	}()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	reporter, err = errreport.New(errreport.Options{DSN: cfg.SentryDSN, Environment: cfg.Environment, Release: cfg.Release})
	if err != nil {
		return err
	}
	logger = slog.New(reporter.Handler(logger.Handler()))

	pages, err := site.New(cfg.PublicURL)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	db, err := pgdb.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	migrateCtx, cancelMigrate := context.WithTimeout(ctx, migrateTimeout)
	defer cancelMigrate()
	if err := database.Migrate(migrateCtx, db); err != nil {
		return err
	}

	// Migrations may have used up the first startup deadline.
	redisCtx, cancelRedis := context.WithTimeout(ctx, startupTimeout)
	defer cancelRedis()
	rdb, err := kv.Open(redisCtx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Error("close redis", "err", err)
		}
	}()
	countryLookup, err := geo.New()
	if err != nil {
		return err
	}

	credits := billing.NewCreditLedgerService(db)
	go func() {
		if expired, err := credits.ExpireCredits(ctx); err != nil {
			logger.Error("expire billing credits", "err", err)
		} else if expired > 0 {
			logger.Info("expired billing credits", "credits", expired)
		}
		timer := time.NewTicker(time.Hour)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				expired, err := credits.ExpireCredits(ctx)
				if err != nil {
					logger.Error("expire billing credits", "err", err)
				} else if expired > 0 {
					logger.Info("expired billing credits", "credits", expired)
				}
			}
		}
	}()
	var billingSvc *billing.Service
	billingRoutes := billing.NewReadOnlyService(db)
	if rzp := cfg.Razorpay; rzp != nil {
		billingSvc = billing.NewPlanService(db, razorpay.NewClient(razorpay.BaseURL, rzp.KeyID, rzp.KeySecret), rzp.PlanIDs, rzp.WebhookSecret)
		billingRoutes = billingSvc
	} else {
		logger.Warn("RAZORPAY_* not set; billing status is read-only and checkout is unavailable")
	}
	// The owner's and staff's own workspaces are never limited by a plan,
	// with or without payments configured.
	billingRoutes.SetStaffEmails(cfg.StaffEmails)
	// A live deployment must enforce entitlements even while checkout is
	// unavailable. The read-only service still knows the stored subscription.
	gateSvc := planGateService(cfg.Environment, billingSvc, billingRoutes)
	credits.Unlimited = billingRoutes.IsUnlimited
	// Every paid DataForSEO request and AI call goes through this guard.
	// Without payments (self-hosted) nobody is charged credits, but the daily
	// provider spend limit still applies.
	spendGuard, err := billing.NewSpendGuard(db, credits, cfg.ProviderDailyLimitUSD, billingSvc != nil, logger)
	if err != nil {
		return fmt.Errorf("create spend guard: %w", err)
	}
	spendGuard.WorkspaceDailyLimitUSD = billingRoutes.WorkspaceDailyLimitUSD
	spendGuard.WithinPlanLimits = billingRoutes.WithinPlanLimits

	var aiSearchSvc *aisearch.Service
	var backlinksSvc *backlinks.Service
	var domainSvc *domain.Service
	var locationSvc *keywords.LocationService
	var rankLocationChecker ranktracking.LocationChecker // stays a nil interface without a provider key
	var keywordResearch *keywords.ResearchService
	var dfClient *dataforseo.Client
	if cfg.DataForSEOAPIKey != "" {
		client, err := dataforseo.NewClient(dataforseo.Options{APIKey: cfg.DataForSEOAPIKey, Recorder: dataforseo.NewUsageRecorder(db), Guard: spendGuard})
		if err != nil {
			return fmt.Errorf("create DataForSEO client: %w", err)
		}
		dfClient = client
		aiSearchSvc = aisearch.NewService(client, rdb, logger)
		backlinksSvc = backlinks.NewService(client, rdb, logger)
		domainSvc = domain.NewService(client, rdb, logger)
		locationSvc = keywords.NewLocationService(client, rdb, logger)
		rankLocationChecker = ranktracking.RegistryChecker{Registry: locationSvc}
		keywordResearch = keywords.NewResearchService(keywords.DataForSEOProvider{Client: client}, rdb, locationSvc, keywords.SavedRepository{DB: db}, logger)
	} else {
		logger.Warn("DATAFORSEO_API_KEY not set; AI search, backlinks and domain endpoints answer 503")
	}
	var googleOAuthSvc *google.OAuthService
	var googleAPIClient *google.APIClient
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		key := cfg.GoogleTokenEncryptionKey
		keyID := "v1"
		if key == "" {
			derived := sha256.Sum256([]byte("uplumo-google-tokens-v1:" + cfg.BetterAuthSecret))
			key = base64.StdEncoding.EncodeToString(derived[:])
			keyID = "derived-v1"
		}
		cipher, err := google.NewTokenCipher(keyID, key)
		if err != nil {
			return fmt.Errorf("configure google token encryption: %w", err)
		}
		if keyID != "derived-v1" {
			derived := sha256.Sum256([]byte("uplumo-google-tokens-v1:" + cfg.BetterAuthSecret))
			if err := cipher.AddDecryptionKey("derived-v1", base64.StdEncoding.EncodeToString(derived[:])); err != nil {
				return err
			}
		}
		legacyCipher, err := google.NewLegacyTokenCipher(cfg.BetterAuthSecret)
		if err != nil {
			return err
		}
		tokenService := &google.TokenService{Pool: db, Cipher: cipher, LegacyCipher: legacyCipher, ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret}
		googleOAuthSvc = &google.OAuthService{
			States:       google.StateStore{Pool: db},
			Tokens:       tokenService,
			Verifier:     &google.IDTokenVerifier{},
			PublicOrigin: cfg.PublicURL.String(),
		}
		googleAPIClient = &google.APIClient{Tokens: tokenService}
	}
	var ga4Svc *ga4.Service
	ga4Setup := &ga4.ConnectionOperations{Store: ga4.SetupRepository{DB: db}, OAuthReady: cfg.GoogleClientID != "" && cfg.GoogleClientSecret != ""}
	if googleOAuthSvc != nil {
		ga4Svc = &ga4.Service{Connections: ga4.ConnectionRepository{DB: db}, Google: &google.APIClient{Tokens: googleOAuthSvc.Tokens}}
		ga4Setup.NewAdmin = func(userID, accountID string) ga4.PropertyAdmin {
			return ga4.GooglePropertyAdmin{API: googleAPIClient, UserID: userID, AccountID: accountID}
		}
	}

	sitePublish, err := buildSitePublish(cfg, db)
	if err != nil {
		return err
	}

	rankChecks, err := buildRankChecks(ctx, logger, db, gateSvc, dfClient)
	if err != nil {
		return err
	}
	crawlerAccessSvc := &crawleraccess.Service{DB: db, Secret: cfg.BetterAuthSecret}
	auditSvc, err := buildAuditService(ctx, logger, db, rdb, gateSvc, cfg.PageSpeedAPIKey, cfg.CrUXAPIKey, crawlerAccessSvc)
	if err != nil {
		return err
	}
	rankTrackingService := ranktracking.NewService(ranktracking.Store{DB: db}, ranktracking.Store{DB: db}, rankLocationChecker)
	rankTrackingService.Metrics = keywordResearch
	rankTrackingService.Limits, rankTrackingService.Usage = billingRoutes, ranktracking.Store{DB: db}
	if gateSvc != nil {
		rankTrackingService.Plans = gateSvc.Gate(planCatalog.RankTracking)
	}
	reportsSvc := &reports.Service{Store: reports.Repository{DB: db}}

	authService := auth.NewService(db, cfg.BetterAuthSecret)
	savedKeywordsSvc := &keywords.SavedService{Store: keywords.SavedRepository{DB: db}}
	gscService := buildGSCService(db, googleAPIClient)
	contentAuthors := &contentauthors.Repository{DB: db}
	linkSuggestSvc := &linksuggest.Service{
		Sources: []linksuggest.Source{linksuggest.GSC{Service: gscService}, linksuggest.AuditSource{DB: db}},
		Redis:   linksuggest.RedisCache{Client: rdb}, Logger: logger,
	}
	projectContextSvc := &projectcontext.Service{Repo: projectcontext.Repository{DB: db}}
	var contentImages contenteditor.ImageStore
	if cfg.SupabaseStorage != nil {
		contentImages, err = contenteditor.NewSupabaseImageStore(cfg.SupabaseStorage)
		if err != nil {
			return fmt.Errorf("configure Supabase content image storage: %w", err)
		}
	} else {
		logger.Warn("SUPABASE_* image storage variables not set; content editor image uploads are unavailable")
	}
	var localSEOData localseo.Data
	// keywordResearch is nil when DATAFORSEO_API_KEY is not set.
	if keywordResearch != nil {
		if provider, ok := keywordResearch.Data.(localseo.Data); ok {
			localSEOData = provider
		}
	}
	localSEOService := &localseo.Service{Data: localSEOData}
	if gateSvc != nil {
		localSEOService.Plans = gateSvc.Gate(planCatalog.Keywords)
	}
	mcpDeps := &mcp.Deps{
		Logger: logger, DB: db, Redis: rdb, Auth: authService, Billing: gateSvc,
		Credits:   credits,
		PublicURL: cfg.PublicURL,
		Audit:     auditSvc, Backlinks: backlinksSvc, Domain: domainSvc, Locations: locationSvc, SavedKeywords: savedKeywordsSvc, KeywordResearch: keywordResearch, ProjectContext: projectContextSvc,
		GA4: ga4Svc, GSC: gscService, Reports: reportsSvc,
		RankTracking: rankTrackingService, RankChecks: rankChecks,
	}
	emailAuthSvc := buildEmailAuth(cfg, logger, db, rdb, authService)
	samModel := strings.TrimSpace(cfg.OpenRouterModel)
	if samModel == "" {
		samModel = openrouter.DefaultModel
	}
	samChat := buildSAMChat(cfg, logger, db, projectContextSvc, mcpDeps, spendGuard.AIMeter(spend.AIAssistant, "openrouter/"+samModel))
	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.NewHandler(httpapi.Deps{
			Logger:                  logger,
			DB:                      db,
			Redis:                   httpapi.PingFunc(func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
			Auth:                    authService,
			Analytics:               analytics.NewService(db, rdb, countryLookup),
			TrustedProxyCIDRs:       cfg.TrustedProxyCIDRs,
			Billing:                 gateSvc,
			BillingRoutes:           billingRoutes,
			Branding:                branding.NewService(db),
			OpenAIAppsChallenge:     cfg.OpenAIAppsChallenge,
			AISearch:                aiSearchSvc,
			Ahrefs:                  ahrefs.New(rdb),
			Backlinks:               backlinksSvc,
			Domain:                  domainSvc,
			DashboardOverview:       newDashboardOverview(db, backlinksSvc),
			GoogleAccounts:          google.AccountRepository{Pool: db},
			GoogleOAuth:             googleOAuthSvc,
			GA4:                     ga4Svc,
			GA4Setup:                ga4Setup,
			GSC:                     gscService,
			GSCConnections:          buildGSCConnectionOperations(db, googleAPIClient, googleOAuthSvc != nil),
			ProjectMarkets:          domain.ProjectMarketRepository{DB: db},
			Locations:               locationSvc,
			Audit:                   auditSvc,
			RankTracking:            rankTrackingService,
			RankChecks:              rankChecks,
			Projects:                &projects.Service{Store: projects.Repository{DB: db}, Plans: billingRoutes},
			ContentEditor:           &contenteditor.Repository{DB: db},
			ContentAuthors:          contentAuthors,
			LinkSuggest:             linkSuggestSvc,
			SitePublish:             sitePublish,
			ContentWriter:           buildContentWriter(cfg, logger, spendGuard.AIMeter(spend.AIWriting, "openrouter/"+contenteditor.WriterModel), billingRoutes.Gate(planCatalog.AIWriting)),
			ContentBriefs:           buildContentBriefs(db, dfClient),
			ProjectsRepo:            projects.Repository{DB: db},
			ContentImages:           contentImages,
			ContentImageLimiter:     contenteditor.RedisImageUploadLimiter{Client: rdb},
			SAMSessions:             &sam.Service{Store: sam.Repository{DB: db}},
			EmailAuth:               emailAuthSvc,
			APIKeys:                 &apikeys.Service{DB: db, Auth: authService, Logger: logger},
			SAMChat:                 samChat,
			Onboarding:              &onboarding.Service{Store: onboarding.Repository{DB: db}},
			Team:                    &team.Service{Limits: billingRoutes, DB: db, Redis: rdb, LoopsAPIKey: cfg.LoopsAPIKey, InvitationTemplateID: cfg.LoopsInvitationTemplateID, BaseURL: cfg.BetterAuthURL},
			CrawlerAccess:           crawlerAccessSvc,
			Activation:              &activation.Service{Store: activation.Repository{DB: db}},
			Reports:                 reportsSvc,
			PublicURL:               cfg.PublicURL,
			DataForSEOConfigured:    strings.TrimSpace(cfg.DataForSEOAPIKey) != "",
			OpenRouterConfigured:    strings.TrimSpace(cfg.OpenRouterAPIKey) != "",
			ContentWriterConfigured: strings.TrimSpace(cfg.OpenRouterAPIKey) != "",
			Credits:                 credits,
			MCP:                     mcpDeps,
			SavedKeywords:           savedKeywordsSvc,
			ProjectContext:          projectContextSvc,
			KeywordResearch:         keywordResearch,
			LocalSEO:                localSEOService,
			Site:                    pages,
			StaticDir:               cfg.StaticDir,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func planGateService(environment string, payments, readOnly *billing.Service) *billing.Service {
	if payments != nil {
		return payments
	}
	if environment == "production" {
		return readOnly
	}
	return nil
}

// buildSitePublish wires publishing to customers' websites. Their passwords
// are encrypted with a key derived from BETTER_AUTH_SECRET: rotating that
// secret means every customer reconnects their website.
func buildSitePublish(cfg config.Config, db *pgxpool.Pool) (*sitepublish.Service, error) {
	derived := sha256.Sum256([]byte("uplumo-site-credentials-v1:" + cfg.BetterAuthSecret))
	cipher, err := google.NewTokenCipher("site-v1", base64.StdEncoding.EncodeToString(derived[:]))
	if err != nil {
		return nil, fmt.Errorf("configure site credential encryption: %w", err)
	}
	return &sitepublish.Service{
		Store:     sitepublish.Repository{DB: db},
		Drafts:    contenteditor.Repository{DB: db},
		Cipher:    cipher,
		WordPress: sitepublish.WordPress{HTTP: audit.NewGuard().NewClient(30 * time.Second)},
		Authors:   &contentauthors.Repository{DB: db},
	}, nil
}

func buildContentWriter(cfg config.Config, logger *slog.Logger, meter contenteditor.WriterMeter, plans contenteditor.WriterPlanAccess) contenteditor.WriterService {
	if strings.TrimSpace(cfg.OpenRouterAPIKey) == "" {
		return nil
	}
	return &contenteditor.WriterAgent{
		LLM:    &openrouter.Client{APIKey: cfg.OpenRouterAPIKey},
		Meter:  meter,
		Plans:  plans,
		Logger: logger,
	}
}

// buildContentBriefs wires search briefs. Without DataForSEO there is no
// brief service; the route then answers "not available yet". It returns a nil
// interface, never a typed nil, so that check works.
func buildContentBriefs(db *pgxpool.Pool, dfClient *dataforseo.Client) contenteditor.BriefGetter {
	if dfClient == nil {
		return nil
	}
	return contenteditor.BriefService{
		Cache:    contenteditor.PostgresBriefCache{DB: db},
		Provider: contenteditor.DataForSEOBriefProvider{Client: dfClient},
	}
}

// rankTickInterval is how often the rank tracking scheduler looks for due checks.
const rankTickInterval = 5 * time.Minute

// buildEmailAuth wires the email and password routes. They need the public
// URL; email verification and password reset need Loops, and without it those
// two features answer "not enabled".
func buildEmailAuth(cfg config.Config, logger *slog.Logger, db *pgxpool.Pool, rdb *redis.Client, sessions *auth.Service) *emailauth.Service {
	if cfg.BetterAuthURL == nil {
		return nil
	}
	svc := emailauth.Service{
		DB: db, Sessions: emailauth.AuthSessions{Auth: sessions}, Logger: logger,
		Limiter:  emailauth.RedisLimiter{Client: rdb},
		ClientIP: func(r *http.Request) string { return httpx.ClientIP(r, cfg.TrustedProxyCIDRs) },
		Cfg: emailauth.Config{
			Secret: cfg.BetterAuthSecret, BaseURL: cfg.BetterAuthURL,
			BypassEmailVerification: cfg.BypassEmailVerification,
			BlockDisposableEmail:    true,
		},
	}
	if cfg.LoopsAPIKey != "" && cfg.LoopsVerifyEmailTemplateID != "" && cfg.LoopsResetPasswordTemplateID != "" {
		svc.Mailer = emailauth.LoopsMailer{
			Client:           email.LoopsClient{APIKey: cfg.LoopsAPIKey},
			VerifyTemplateID: cfg.LoopsVerifyEmailTemplateID,
			ResetPasswordID:  cfg.LoopsResetPasswordTemplateID,
		}
	}
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		svc.Google = &emailauth.GoogleProvider{ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret}
	}
	if cfg.TurnstileSecretKey != "" {
		svc.Captcha = emailauth.Turnstile{Secret: cfg.TurnstileSecretKey}
	}
	return emailauth.New(svc)
}

// buildSAMChat wires the SAM agent. The model key is Uplumo's own, never the
// customer's: it returns nil without OPENROUTER_API_KEY and the chat routes
// then answer 503. Every turn is gated on and charged to the organization's
// credits.
func buildSAMChat(cfg config.Config, logger *slog.Logger, db *pgxpool.Pool, memory *projectcontext.Service, mcpDeps *mcp.Deps, meter sam.Meter) *sam.Chat {
	if cfg.OpenRouterAPIKey == "" {
		return nil
	}
	model := cfg.OpenRouterModel
	if model == "" {
		model = openrouter.DefaultModel
	}
	agent := &sam.Agent{
		LLM:     &openrouter.Client{APIKey: cfg.OpenRouterAPIKey},
		Model:   model,
		Store:   sam.MessageRepository{DB: db},
		Titles:  sam.Repository{DB: db},
		Memory:  sam.ProjectMemory{Service: memory},
		Runner:  mcp.NewRunner(*mcpDeps),
		Guard:   audit.NewGuard(),
		BaseURL: cfg.PublicURL.String(),
		Logger:  logger,
	}
	agent.Meter = meter
	return sam.NewChat(agent, sam.ContextLoader{DB: db}, sam.MessageRepository{DB: db})
}

func buildGSCService(db *pgxpool.Pool, api *google.APIClient) *gsc.Service {
	if api == nil {
		return nil
	}
	return &gsc.Service{
		Connections: gsc.ConnectionRepository{DB: db},
		NewClient: func(userID, accountID string) gsc.SearchClient {
			return &gsc.Client{API: api, UserID: userID, AccountID: accountID}
		},
	}
}

func buildGSCConnectionOperations(db *pgxpool.Pool, api *google.APIClient, oauthConfigured bool) *gsc.ConnectionOperations {
	repository := gsc.PropertyRepository{DB: db}
	operations := &gsc.ConnectionOperations{
		Connections: repository,
		Grants:      repository,
		Manager:     repository,
		OAuthReady:  oauthConfigured,
	}
	if api != nil {
		operations.NewClient = func(userID, accountID string) gsc.SearchConsoleClient {
			return &gsc.Client{API: api, UserID: userID, AccountID: accountID}
		}
	}
	return operations
}

// buildRankChecks starts the rank check worker and returns the service that
// queues checks. Without a DataForSEO key it returns nil and the check route
// answers 503. Checks run ungated only in self-hosted development.
func buildRankChecks(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, billingSvc *billing.Service, dfClient *dataforseo.Client) (*ranktracking.Checks, error) {
	if dfClient == nil {
		return nil, nil
	}
	queue, err := jobs.New(db)
	if err != nil {
		return nil, fmt.Errorf("create rank check job queue: %w", err)
	}
	store := ranktracking.Store{DB: db}
	var plans ranktracking.PaidPlans
	var depthLimits ranktracking.SerpDepthLimits
	if billingSvc != nil {
		plans = billingSvc.Gate(planCatalog.RankTracking)
		depthLimits = billingSvc
	}
	checks := &ranktracking.Checks{
		Repo: store, Results: store, Runs: store, Serp: ranktracking.DataForSEOSerp{Client: dfClient},
		Queued: ranktracking.DataForSEOSerp{Client: dfClient}, Tasks: store,
		Scheduler: ranktracking.QueueScheduler{Queue: queue}, Plans: plans, DepthLimits: depthLimits, Logger: logger, Now: time.Now,
	}
	worker := jobs.Worker{
		Queue: queue, QueueName: ranktracking.QueueName, Handle: checks.JobHandler(),
		Lease: time.Minute, PollInterval: 2 * time.Second,
	}
	go func() {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("rank check worker stopped", "err", err)
		}
	}()
	// Every instance ticks: each due config is claimed with a compare-and-set,
	// so only one of them starts it.
	ticker := &ranktracking.Ticker{
		Store: store, Starter: checks, Plans: plans, Logger: logger,
		Schedule: ranktracking.Scheduler{Now: time.Now, Rand: ranktracking.CryptoRand},
	}
	go func() {
		timer := time.NewTicker(rankTickInterval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if _, err := ticker.Tick(ctx); err != nil && ctx.Err() == nil {
					logger.Error("rank tracking scheduler tick failed", "err", err)
				}
			}
		}
	}()
	return checks, nil
}

// buildAuditService wires the audit engine: the SSRF-guarded crawler, the
// Lighthouse provider, the Postgres-backed job scheduler and the background
// worker that runs audits.
func buildAuditService(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, rdb *redis.Client, billingSvc *billing.Service, pageSpeedAPIKey, cruxAPIKey string, crawlerAccess audit.CrawlerAccessResolver) (*audit.Service, error) {
	repository := audit.NewRepository(db)
	progress := audit.NewProgress(rdb)
	guard := audit.NewGuard()
	crawler := audit.NewCrawler(audit.CrawlerOptions{Guard: guard})

	var lighthouseProvider audit.LighthouseProvider
	if pageSpeedAPIKey != "" {
		lighthouseProvider = audit.NewPageSpeedProvider(pageSpeedAPIKey, audit.RedisPageSpeedQuota{Client: rdb})
	}
	var fieldData audit.FieldDataProvider
	if cruxAPIKey != "" {
		fieldData = audit.NewCrUXProvider(cruxAPIKey, audit.RedisFieldDataCache{Client: rdb}, logger)
	}

	queue, err := jobs.New(db)
	if err != nil {
		return nil, fmt.Errorf("create audit job queue: %w", err)
	}

	// Live deployments gate audits even when checkout is unavailable; a
	// self-hosted development server without billing runs audits ungated.
	var plans audit.PaidPlans
	if billingSvc != nil {
		plans = billingSvc.Gate(planCatalog.FullAudit)
	}

	service := audit.NewService(audit.ServiceConfig{
		Repository: repository, Progress: progress, Guard: guard, Crawler: crawler,
		Lighthouse: lighthouseProvider, Scheduler: audit.NewQueueScheduler(queue), Plans: plans,
		Resolver: crawlerAccess,
		Hosted:   billingSvc != nil, Env: os.Getenv, Logger: logger,
	})

	startAuditWorker(ctx, logger, queue, repository, progress, guard, crawler, lighthouseProvider, fieldData, crawlerAccess)
	return service, nil
}

// startAuditWorker consumes audits from the jobs queue until ctx ends. The
// worker's jobs are at-least-once, so a crashed worker's audit is retried and
// its deterministic row ids keep the retry idempotent.
func startAuditWorker(ctx context.Context, logger *slog.Logger, queue *jobs.Queue, repository *audit.Repository, progress *audit.Progress, guard *audit.Guard, crawler *audit.Crawler, lighthouseProvider audit.LighthouseProvider, fieldData audit.FieldDataProvider, crawlerAccess audit.CrawlerAccessResolver) {
	runner := audit.NewRunner(audit.RunnerConfig{
		Repository: repository, Progress: progress, Guard: guard, Crawler: crawler,
		Lighthouse: lighthouseProvider, FieldData: fieldData, Logger: logger, Resolver: crawlerAccess,
	})
	worker := jobs.Worker{
		Queue: queue, QueueName: audit.AuditQueueName, Handle: audit.RunnerHandler(runner),
		Lease: time.Minute, PollInterval: 2 * time.Second,
	}
	go func() {
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			logger.Error("audit worker stopped", "err", err)
		}
	}()
}

// newDashboardOverview leaves Backlinks as a nil interface when DataForSEO is
// not configured. Assigning the nil *backlinks.Service directly would make the
// interface non-nil and panic on the first refresh.
func newDashboardOverview(db *pgxpool.Pool, backlinksSvc *backlinks.Service) *dashboardoverview.Service {
	svc := &dashboardoverview.Service{Store: dashboardoverview.Repository{DB: db}}
	if backlinksSvc != nil {
		svc.Backlinks = backlinksSvc
	}
	return svc
}
