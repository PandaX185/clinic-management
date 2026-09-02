// Package wiring constructs the application's full dependency graph and
// HTTP router, keeping cmd/api/main.go a thin entry point concerned only with
// lifecycle (config, logger, connections, graceful shutdown).
package wiring

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	apptapi "github.com/PandaX185/clinic-management/internal/appointment/api"
	apptrepo "github.com/PandaX185/clinic-management/internal/appointment/repo"
	apptsvc "github.com/PandaX185/clinic-management/internal/appointment/service"
	authapi "github.com/PandaX185/clinic-management/internal/auth/api"
	authrepo "github.com/PandaX185/clinic-management/internal/auth/repo"
	authsvc "github.com/PandaX185/clinic-management/internal/auth/service"
	directoryapi "github.com/PandaX185/clinic-management/internal/directory/api"
	directoryrepo "github.com/PandaX185/clinic-management/internal/directory/repo"
	directorysvc "github.com/PandaX185/clinic-management/internal/directory/service"
	server "github.com/PandaX185/clinic-management/internal/server"
	tenantapi "github.com/PandaX185/clinic-management/internal/tenant/api"
	tenantrepo "github.com/PandaX185/clinic-management/internal/tenant/repo"
	tenantsvc "github.com/PandaX185/clinic-management/internal/tenant/service"

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

	authRepo := authrepo.NewPostgresRepository(d.Pool)
	tokens := authsvc.NewTokenManager(d.Cfg.JWTSecret, d.Cfg.JWTRefreshSecret, d.Cfg.AccessTokenTTL, d.Cfg.RefreshTokenTTL)
	authSvc := authsvc.NewService(authRepo, tokens, d.Cfg.BcryptCost)
	authH := authapi.NewHandler(authSvc)

	tenantStore := tenantrepo.NewPostgresStore(d.Pool)
	profileStore := tenantrepo.NewScopedProfileStore(d.Pool)
	tenantSvc := tenantsvc.NewService(tenantStore, profileStore, tenantStore)
	tenantH := tenantapi.NewHandler(tenantSvc)

	// Real "my clinics" resolution for /auth/tenants: global user_tenants
	// index + per-tenant role lookup. Defined here to keep auth→tenant acyclic.
	membershipProvider := &tenantMembershipProvider{
		pool:  d.Pool,
		store: tenantStore,
	}
	authSvc.WithTenantMemberships(membershipProvider)

	aptRepo := apptrepo.NewPostgresRepository(d.Pool)
	aptSvc := apptsvc.NewServiceWithIdentity(aptRepo, nil, apptrepo.NewPostgresIdentityResolver(d.Pool), d.Cfg.IdempotencyTTL)
	aptH := apptapi.NewHandler(aptSvc)

	dirRepo := directoryrepo.NewPostgresRepo(d.Pool)
	dirSvc := directorysvc.NewService(dirRepo)
	dirH := directoryapi.NewHandler(dirSvc)

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
		DirectoryH:      dirH,
		Metrics:         m,
	})

	// Liveness/readiness probes live at the root (outside /api/v1) so
	// orchestrators can check them without auth or a tenant context.
	server.NewHealth(d.Cfg, d.Pool, d.RDB, d.NATS).RegisterRoutes(r)

	return r, m, nil
}
