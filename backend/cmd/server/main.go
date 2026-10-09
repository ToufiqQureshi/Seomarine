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
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/toufiqqureshi/seomarine/backend/internal/aisearch"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics/geo"
	"github.com/toufiqqureshi/seomarine/backend/internal/audit"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
	"github.com/toufiqqureshi/seomarine/backend/internal/billing"
	"github.com/toufiqqureshi/seomarine/backend/internal/branding"
	"github.com/toufiqqureshi/seomarine/backend/internal/config"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/domain"
	"github.com/toufiqqureshi/seomarine/backend/internal/google"
	"github.com/toufiqqureshi/seomarine/backend/internal/httpapi"
	"github.com/toufiqqureshi/seomarine/backend/internal/keywords"
	"github.com/toufiqqureshi/seomarine/backend/internal/kv"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/jobs"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/pgdb"
	"github.com/toufiqqureshi/seomarine/backend/internal/razorpay"
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
	} else {
		logger.Warn("DATAFORSEO_API_KEY not set; AI search, backlinks and domain endpoints answer 503")
	}
	var googleOAuthSvc *google.OAuthService
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
		googleOAuthSvc = &google.OAuthService{
			States:       google.StateStore{Pool: db},
			Tokens:       &google.TokenService{Pool: db, Cipher: cipher, LegacyCipher: legacyCipher, ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret},
			Verifier:     &google.IDTokenVerifier{},
			PublicOrigin: cfg.PublicURL.String(),
		}
	}

	auditSvc, err := buildAuditService(ctx, logger, db, rdb, billingSvc, dfClient)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: httpapi.NewHandler(httpapi.Deps{
			Logger:            logger,
			DB:                db,
			Redis:             httpapi.PingFunc(func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
			Auth:              auth.NewService(db, cfg.BetterAuthSecret),
			Analytics:         analytics.NewService(db, rdb, countryLookup),
			TrustedProxyCIDRs: cfg.TrustedProxyCIDRs,
			Billing:           billingSvc,
			Branding:          branding.NewService(db),
			AISearch:          aiSearchSvc,
			Backlinks:         backlinksSvc,
			Domain:            domainSvc,
			GoogleAccounts:    google.AccountRepository{Pool: db},
			GoogleOAuth:       googleOAuthSvc,
			ProjectMarkets:    domain.ProjectMarketRepository{DB: db},
			Locations:         locationSvc,
			Audit:             auditSvc,
			Site:              pages,
			Upstream:          cfg.UpstreamAppURL,
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
