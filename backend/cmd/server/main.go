// Command server runs the Seomarine HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/toufiqqureshi/seomarine/backend/internal/aisearch"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics"
	"github.com/toufiqqureshi/seomarine/backend/internal/analytics/geo"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/backlinks"
	"github.com/toufiqqureshi/seomarine/backend/internal/billing"
	"github.com/toufiqqureshi/seomarine/backend/internal/branding"
	"github.com/toufiqqureshi/seomarine/backend/internal/config"
	"github.com/toufiqqureshi/seomarine/backend/internal/database"
	"github.com/toufiqqureshi/seomarine/backend/internal/domain"
	"github.com/toufiqqureshi/seomarine/backend/internal/httpapi"
	"github.com/toufiqqureshi/seomarine/backend/internal/kv"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/dataforseo"
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
	if cfg.DataForSEOAPIKey != "" {
		client, err := dataforseo.NewClient(dataforseo.Options{APIKey: cfg.DataForSEOAPIKey, Recorder: dataforseo.NewUsageRecorder(db)})
		if err != nil {
			return fmt.Errorf("create DataForSEO client: %w", err)
		}
		aiSearchSvc = aisearch.NewService(client, rdb, logger)
		backlinksSvc = backlinks.NewService(client, rdb, logger)
		domainSvc = domain.NewService(client, rdb, logger)
	} else {
		logger.Warn("DATAFORSEO_API_KEY not set; AI search, backlinks and domain endpoints answer 503")
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
