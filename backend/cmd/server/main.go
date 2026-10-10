// Command server runs the Seomarine HTTP API.
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
	"github.com/toufiqqureshi/seomarine/backend/internal/activation"
	"github.com/toufiqqureshi/seomarine/backend/internal/ahrefs"
	"github.com/toufiqqureshi/seomarine/backend/internal/aisearch"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics/geo"
	"github.com/toufiqqureshi/seomarine/backend/internal/audit"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
	"github.com/toufiqqureshi/seomarine/backend/internal/billing"
	"github.com/toufiqqureshi/seomarine/backend/internal/branding"
	"github.com/toufiqqureshi/seomarine/backend/internal/config"
	"github.com/toufiqqureshi/seomarine/backend/internal/dashboardoverview"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/domain"
	"github.com/toufiqqureshi/seomarine/backend/internal/ga4"
	"github.com/toufiqqureshi/seomarine/backend/internal/google"
	"github.com/toufiqqureshi/seomarine/backend/internal/gsc"
	"github.com/toufiqqureshi/seomarine/backend/internal/httpapi"
	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
	"github.com/toufiqqureshi/seomarine/backend/internal/kv"
	"github.com/toufiqqureshi/seomarine/backend/internal/mcp"
	"github.com/toufiqqureshi/seomarine/backend/internal/onboarding"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/jobs"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
	"github.com/toufiqqureshi/seomarine/backend/internal/projectcontext"
	"github.com/toufiqqureshi/seomarine/backend/internal/projects"
	"github.com/toufiqqureshi/seomarine/backend/internal/ranktracking"
	"github.com/toufiqqureshi/seomarine/backend/internal/razorpay"
	"github.com/toufiqqureshi/seomarine/backend/internal/reports"
	"github.com/toufiqqureshi/seomarine/backend/internal/sam"
	"github.com/toufiqqureshi/seomarine/backend/internal/site"
)

const (
	startupTimeout  = 10 * time.Second
	shutdownTimeout = 15 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

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
	if err := database.Migrate(startupCtx, db); err != nil {
		return err
	}

	rdb, err := kv.Open(startupCtx, cfg.RedisURL)
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

	var billingSvc *billing.Service
	if rzp := cfg.Razorpay; rzp != nil {
		billingSvc = billing.NewService(db, razorpay.NewClient(razorpay.BaseURL, rzp.KeyID, rzp.KeySecret), rzp.PlanIDPro, rzp.WebhookSecret)
	} else {
		logger.Warn("RAZORPAY_* not set; billing endpoints answer 503")
	}

	var aiSearchSvc *aisearch.Service
	var backlinksSvc *backlinks.Service
	var domainSvc *domain.Service
	var locationSvc *keywords.LocationService
	var rankLocationChecker ranktracking.LocationChecker // stays a nil interface without a provider key
	var keywordResearch *keywords.ResearchService
	var dfClient *dataforseo.Client
	if cfg.DataForSEOAPIKey != "" {
		client, err := dataforseo.NewClient(dataforseo.Options{APIKey: cfg.DataForSEOAPIKey, Recorder: dataforseo.NewUsageRecorder(db)})
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
			derived := sha256.Sum256([]byte("seomarine-google-tokens-v1:" + cfg.BetterAuthSecret))
			key = base64.StdEncoding.EncodeToString(derived[:])
			keyID = "derived-v1"
		}
		cipher, err := google.NewTokenCipher(keyID, key)
		if err != nil {
			return fmt.Errorf("configure google token encryption: %w", err)
		}
		if keyID != "derived-v1" {
			derived := sha256.Sum256([]byte("seomarine-google-tokens-v1:" + cfg.BetterAuthSecret))
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

	rankChecks, err := buildRankChecks(ctx, logger, db, billingSvc, dfClient)
	if err != nil {
		return err
	}
	auditSvc, err := buildAuditService(ctx, logger, db, rdb, billingSvc, dfClient)
	if err != nil {
		return err
	}
	rankTrackingService := ranktracking.NewService(ranktracking.Store{DB: db}, ranktracking.Store{DB: db}, rankLocationChecker)
	rankTrackingService.Metrics = keywordResearch
	rankTrackingService.Plans = billingSvc
	reportsSvc := &reports.Service{Store: reports.Repository{DB: db}, Hosted: cfg.AuthMode == "hosted"}

	authService := auth.NewService(db, cfg.BetterAuthSecret)
	savedKeywordsSvc := &keywords.SavedService{Store: keywords.SavedRepository{DB: db}}
	gscService := buildGSCService(db, googleAPIClient)
	projectContextSvc := &projectcontext.Service{Repo: projectcontext.Repository{DB: db}}
	mcpDeps := &mcp.Deps{
		Logger: logger, DB: db, Redis: rdb, Auth: authService, Billing: billingSvc,
		Upstream: cfg.UpstreamAppURL, PublicURL: cfg.PublicURL,
		Audit: auditSvc, Locations: locationSvc, SavedKeywords: savedKeywordsSvc, ProjectContext: projectContextSvc,
		GA4: ga4Svc, GSC: gscService, Reports: reportsSvc,
		RankTracking: rankTrackingService,
	}
	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.NewHandler(httpapi.Deps{
			Logger:               logger,
			DB:                   db,
			Redis:                httpapi.PingFunc(func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
			Auth:                 authService,
			Analytics:            analytics.NewService(db, rdb, countryLookup),
			TrustedProxyCIDRs:    cfg.TrustedProxyCIDRs,
			Billing:              billingSvc,
			Branding:             branding.NewService(db),
			AISearch:             aiSearchSvc,
			Ahrefs:               ahrefs.New(rdb),
			Backlinks:            backlinksSvc,
			Domain:               domainSvc,
			DashboardOverview:    &dashboardoverview.Service{Store: dashboardoverview.Repository{DB: db}, Backlinks: backlinksSvc},
			GoogleAccounts:       google.AccountRepository{Pool: db},
			GoogleOAuth:          googleOAuthSvc,
			GA4:                  ga4Svc,
			GA4Setup:             ga4Setup,
			GSC:                  gscService,
			GSCConnections:       buildGSCConnectionOperations(db, googleAPIClient, googleOAuthSvc != nil),
			ProjectMarkets:       domain.ProjectMarketRepository{DB: db},
			Locations:            locationSvc,
			Audit:                auditSvc,
			RankTracking:         rankTrackingService,
			RankChecks:           rankChecks,
			Projects:             &projects.Service{Store: projects.Repository{DB: db}},
			SAMSessions:          &sam.Service{Store: sam.Repository{DB: db}},
			Onboarding:           &onboarding.Service{Store: onboarding.Repository{DB: db}},
			Activation:           &activation.Service{Store: activation.Repository{DB: db}},
			Reports:              reportsSvc,
			PublicURL:            cfg.PublicURL,
			DataForSEOConfigured: strings.TrimSpace(cfg.DataForSEOAPIKey) != "",
			OpenRouterConfigured: strings.TrimSpace(cfg.OpenRouterAPIKey) != "",
			HostedMode:           cfg.AuthMode == "hosted",
			MCP:                  mcpDeps,
			SavedKeywords:        savedKeywordsSvc,
			ProjectContext:       projectContextSvc,
			KeywordResearch:      keywordResearch,
			Site:                 pages,
			Upstream:             cfg.UpstreamAppURL,
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

// buildAuditService wires the audit engine: the SSRF-guarded crawler, the
// Lighthouse provider, the Postgres-backed job scheduler and the background
// worker that runs audits.
// rankTickInterval is how often the rank tracking scheduler looks for due checks.
const rankTickInterval = 5 * time.Minute

// buildRankChecks starts the rank check worker and returns the service that
// queues checks. Without a DataForSEO key it returns nil and the check route
// answers 503. Checks run ungated only when billing is not configured, as for
// a self-hosted deployment.
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
	if billingSvc != nil {
		plans = billingSvc
	}
	checks := &ranktracking.Checks{
		Repo: store, Results: store, Runs: store, Serp: ranktracking.DataForSEOSerp{Client: dfClient},
		Queued: ranktracking.DataForSEOSerp{Client: dfClient}, Tasks: store,
		Scheduler: ranktracking.QueueScheduler{Queue: queue}, Plans: plans, Logger: logger, Now: time.Now,
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

func buildAuditService(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, rdb *redis.Client, billingSvc *billing.Service, dfClient *dataforseo.Client) (*audit.Service, error) {
	repository := audit.NewRepository(db)
	progress := audit.NewProgress(rdb)
	guard := audit.NewGuard()
	crawler := audit.NewCrawler(audit.CrawlerOptions{Guard: guard})

	var lighthouseProvider audit.LighthouseProvider
	if dfClient != nil {
		lighthouseProvider = audit.NewDataForseoLighthouseProvider(dfClient)
	}

	queue, err := jobs.New(db)
	if err != nil {
		return nil, fmt.Errorf("create audit job queue: %w", err)
	}

	// Paid features gate on the plan only when billing is configured; a
	// self-hosted deployment without billing runs audits ungated.
	var plans audit.PaidPlans
	if billingSvc != nil {
		plans = billingSvc
	}

	service := audit.NewService(audit.ServiceConfig{
		Repository: repository, Progress: progress, Guard: guard, Crawler: crawler,
		Lighthouse: lighthouseProvider, Scheduler: audit.NewQueueScheduler(queue), Plans: plans,
		Hosted: billingSvc != nil, Env: os.Getenv, Logger: logger,
	})

	startAuditWorker(ctx, logger, queue, repository, progress, guard, crawler, lighthouseProvider)
	return service, nil
}

// startAuditWorker consumes audits from the jobs queue until ctx ends. The
// worker's jobs are at-least-once, so a crashed worker's audit is retried and
// its deterministic row ids keep the retry idempotent.
func startAuditWorker(ctx context.Context, logger *slog.Logger, queue *jobs.Queue, repository *audit.Repository, progress *audit.Progress, guard *audit.Guard, crawler *audit.Crawler, lighthouseProvider audit.LighthouseProvider) {
	runner := audit.NewRunner(audit.RunnerConfig{
		Repository: repository, Progress: progress, Guard: guard, Crawler: crawler,
		Lighthouse: lighthouseProvider, Logger: logger,
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
