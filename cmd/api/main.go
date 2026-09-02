// @title Clinic Management API
// @version 1.0.0
// @description Multi-tenant clinic management: auth, tenant registry, and appointments. Except for /auth/register, /auth/login and /auth/refresh, every endpoint requires a JWT bearer token (BearerAuth). Tenant-scoped endpoints additionally require the X-Tenant-ID header (see each operation).
// @host localhost:8080
// @BasePath /api/v1
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

	_ "github.com/PandaX185/clinic-management/docs"

	"github.com/PandaX185/clinic-management/internal/app/wiring"
	apptrepo "github.com/PandaX185/clinic-management/internal/appointment/repo"
	"github.com/PandaX185/clinic-management/internal/platform/config"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	"github.com/PandaX185/clinic-management/internal/platform/logger"
	natsclient "github.com/PandaX185/clinic-management/internal/platform/nats"
	redisclient "github.com/PandaX185/clinic-management/internal/platform/redis"
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
			rdb.Close()
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
	cleaner := apptrepo.NewIdempotencyCleaner(pool, cfg.IdempotencyTTL)
	go cleaner.Run(ctx, log)
	defer cleaner.Stop()

	router, _, err := wiring.Build(wiring.Deps{
		Cfg:  cfg,
		Log:  log,
		Pool: pool,
		RDB:  rdb,
		NATS: natsCli,
	})
	if err != nil {
		return err
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
