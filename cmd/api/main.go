// @title Lahza API
// @version 1.0.0
// @description Lahza (لحظة) — multi-tenant clinic operations: auth, clinic registry, booking, and appointments. Except for /auth/register, /auth/login and /auth/refresh, every endpoint requires a JWT bearer token (BearerAuth). Tenant-scoped endpoints additionally require the X-Tenant-ID header (see each operation).
// @host localhost:8080
// @BasePath /api/v1
//
// @Tag.name Identity
// @Tag.description Global accounts, auth, and memberships across clinics.
// @Tag.name Clinics
// @Tag.description Clinic registry and per-clinic staffing.
// @Tag.name Catalog
// @Tag.description Clinical catalog: profiles, doctors, services, and appointment types.
// @Tag.name Scheduling
// @Tag.description Appointment lifecycle: book, cancel, reschedule, and status transitions.
// @Tag.name Booking
// @Tag.description Public clinic discovery and doctor slot availability.
// @Tag.name Portal
// @Tag.description Patient portal: the patient's own profile and appointments.
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Authenticate as `Bearer <access_token>`.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/PandaX185/lahza/docs"

	"github.com/PandaX185/lahza/internal/app/wiring"
	"github.com/PandaX185/lahza/internal/platform/config"
	"github.com/PandaX185/lahza/internal/platform/database"
	"github.com/PandaX185/lahza/internal/platform/logger"
	natsclient "github.com/PandaX185/lahza/internal/platform/nats"
	redisclient "github.com/PandaX185/lahza/internal/platform/redis"
	schedrepo "github.com/PandaX185/lahza/internal/scheduling/repo"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log, err := logger.New(cfg.LogLevel, cfg.Format)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.New(ctx, cfg.DatabaseURL, cfg.DBConnect)
	if err != nil {
		log.Error("database connection failed", "error", err.Error())
		return err
	}
	defer pool.Close()
	log.Info("connected to postgres")

	rdb := redisclient.TryNew(ctx, cfg.RedisURL, cfg.RedisConnect, log)
	defer func() {
		if rdb != nil {
			_ = rdb.Close()
		}
	}()

	// NATS is optional: the app boots and serves traffic without it, with
	// /ready reporting the messaging dependency as unavailable.
	var natsCli *natsclient.Client
	natsCli, natsErr := natsclient.New(ctx, cfg.NATSURL, cfg.NATSConnect)
	if natsErr != nil {
		log.Warn("nats connection failed; running without messaging", "error", natsErr.Error())
	} else {
		defer natsCli.Close()
		log.Info("connected to nats")
	}

	// Expired idempotency keys are purged on an interval derived from the key
	// TTL so the table stays bounded (BR-07).
	cleaner := schedrepo.NewIdempotencyCleaner(pool, cfg.IdempotencyTTL)
	go cleaner.Run(ctx, log)
	defer cleaner.Stop()

	router, _, notifWorker, err := wiring.Build(wiring.Deps{
		Cfg:  cfg,
		Log:  log,
		Pool: pool,
		RDB:  rdb,
		NATS: natsCli,
	})
	if err != nil {
		return err
	}

	// The notification worker dequeues appointment events and hands them to
	// the notifier. It only exists when NATS is connected (see wiring).
	if notifWorker != nil {
		go func() {
			if err := notifWorker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("notification worker stopped", "error", err.Error())
			}
		}()
		log.Info("notification worker started")
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting api server", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err.Error())
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownPeriod)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err.Error())
	}

	log.Info("server stopped")
	return nil
}
