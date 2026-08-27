package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	appt "github.com/PandaX185/clinic-management/internal/appointment"
	auth "github.com/PandaX185/clinic-management/internal/auth"
	server "github.com/PandaX185/clinic-management/internal/server"
	tenant "github.com/PandaX185/clinic-management/internal/tenant"

	"github.com/PandaX185/clinic-management/internal/platform/config"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	"github.com/PandaX185/clinic-management/internal/platform/logger"
	"github.com/PandaX185/clinic-management/internal/platform/metrics"
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
	lg := &loggerAdapter{log}
	defer log.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", zap.Error(err))
		return err
	}
	defer pool.Close()
	log.Info("connected to postgres")

	rdb := tryRedis(ctx, cfg.RedisURL, log)

	m := metrics.New()

	authRepo := auth.NewPostgresRepository(pool)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTRefreshSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	authSvc := auth.NewService(authRepo, tokens)
	authHandler := auth.NewHandler(authSvc)

	tenantStore := tenant.NewPostgresStore(pool)
	tenantSvc := tenant.NewService(tenantStore, tenant.NewScopedProfileStore(pool), tenantStore)
	tenantHandler := tenant.NewHandler(tenantSvc)

	aptRepo := appt.NewPostgresRepository(pool)
	aptSvc := appt.NewServiceWithIdentity(aptRepo, nil, appt.NewPostgresIdentityResolver(pool), cfg.IdempotencyTTL)
	aptHandler := appt.NewHandler(aptSvc)

	router := server.NewRouter(server.RouterDeps{
		Cfg:             cfg,
		RDB:             rdb,
		Logger:          lg,
		AuthH:           authHandler,
		AuthSvc:         authSvc,
		AppointH:        aptHandler,
		TenantH:         tenantHandler,
		TenantSvc:       tenantSvc,
		ProfileResolver: tenant.NewScopedProfileStore(pool),
		Metrics:         m,
	})

	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.Handle("/", router)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting api server", zap.String("addr", srv.Addr))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", zap.Error(err))
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownPeriod)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}

	if rdb != nil {
		rdb.Close()
	}
	log.Info("server stopped")
	return nil
}

func tryRedis(ctx context.Context, url string, log *zap.Logger) *redisclient.Client {
	client, err := redisclient.New(ctx, url)
	if err != nil {
		log.Warn("redis unavailable; continuing degraded", zap.Error(err))
		return nil
	}
	log.Info("connected to redis")
	return client
}

// loggerAdapter adapts zap.Logger to the server.Logger interface.
type loggerAdapter struct {
	log *zap.Logger
}

func (l *loggerAdapter) Info(msg string, args ...any) {
	fields := []zap.Field{}
	for i := 0; i < len(args); i += 2 {
		if k, ok := args[i].(string); ok {
			fields = append(fields, zap.Any(k, args[i+1]))
		}
	}
	l.log.Info(msg, fields...)
}

func (l *loggerAdapter) Warn(msg string, args ...any) {
	fields := []zap.Field{}
	for i := 0; i < len(args); i += 2 {
		if k, ok := args[i].(string); ok {
			fields = append(fields, zap.Any(k, args[i+1]))
		}
	}
	l.log.Warn(msg, fields...)
}

func (l *loggerAdapter) Error(msg string, args ...any) {
	fields := []zap.Field{}
	for i := 0; i < len(args); i += 2 {
		if k, ok := args[i].(string); ok {
			fields = append(fields, zap.Any(k, args[i+1]))
		}
	}
	l.log.Error(msg, fields...)
}

