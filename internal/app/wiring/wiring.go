// Package wiring constructs the application's full dependency graph and
// HTTP router, keeping cmd/api/main.go a thin entry point concerned only with
// lifecycle (config, logger, connections, graceful shutdown).
package wiring

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	appt "github.com/PandaX185/clinic-management/internal/appointment"
	auth "github.com/PandaX185/clinic-management/internal/auth"
	server "github.com/PandaX185/clinic-management/internal/server"
	tenant "github.com/PandaX185/clinic-management/internal/tenant"

	"github.com/PandaX185/clinic-management/internal/platform/config"
	"github.com/PandaX185/clinic-management/internal/platform/metrics"
	natsclient "github.com/PandaX185/clinic-management/internal/platform/nats"
)

// Logger is the minimal logging surface the app depends on. *slog.Logger
// satisfies it directly (Info/Warn/Error with positional args).
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Deps are the raw building blocks supplied by the entry point. NATS is
// optional: a nil client simply reports "unavailable" on /ready and the
// readiness state degrades instead of failing.
type Deps struct {
	Cfg  config.Config
	Log  Logger
	Pool *pgxpool.Pool
	RDB  *redis.Client
	NATS *natsclient.Client
}

// Build wires repositories, services, handlers and middleware into a fully
// configured gin.Engine and returns it alongside the metrics registry so the
// caller can expose them.
func Build(d Deps) (*gin.Engine, *metrics.Metrics, error) {
	m := metrics.New()

	authRepo := auth.NewPostgresRepository(d.Pool)
	tokens := auth.NewTokenManager(d.Cfg.JWTSecret, d.Cfg.JWTRefreshSecret, d.Cfg.AccessTokenTTL, d.Cfg.RefreshTokenTTL)
	authSvc := auth.NewService(authRepo, tokens)
	authH := auth.NewHandler(authSvc)

	tenantStore := tenant.NewPostgresStore(d.Pool)
	profileStore := tenant.NewScopedProfileStore(d.Pool)
	tenantSvc := tenant.NewService(tenantStore, profileStore, tenantStore)
	tenantH := tenant.NewHandler(tenantSvc)

	aptRepo := appt.NewPostgresRepository(d.Pool)
	aptSvc := appt.NewServiceWithIdentity(aptRepo, nil, appt.NewPostgresIdentityResolver(d.Pool), d.Cfg.IdempotencyTTL)
	aptH := appt.NewHandler(aptSvc)

	r := server.NewRouter(server.RouterDeps{
		Cfg:             d.Cfg,
		RDB:             d.RDB,
		Logger:          d.Log,
		AuthH:           authH,
		AuthSvc:         authSvc,
		AppointH:        aptH,
		TenantH:         tenantH,
		TenantSvc:       tenantSvc,
		ProfileResolver: profileStore,
		Metrics:         m,
	})

	// Liveness/readiness probes live at the root (outside /api/v1) so
	// orchestrators can check them without auth or a tenant context.
	server.NewHealth(d.Cfg, d.Pool, d.RDB, d.NATS).RegisterRoutes(r)

	return r, m, nil
}
