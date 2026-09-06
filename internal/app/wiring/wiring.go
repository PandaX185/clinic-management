// Package wiring constructs the application's full dependency graph and
// HTTP router, keeping cmd/api/main.go a thin entry point concerned only with
// lifecycle (config, logger, connections, graceful shutdown).
package wiring

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	bookingapi "github.com/PandaX185/clinic-management/internal/booking/api"
	bookingrepo "github.com/PandaX185/clinic-management/internal/booking/repo"
	bookingsvc "github.com/PandaX185/clinic-management/internal/booking/service"
	catapi "github.com/PandaX185/clinic-management/internal/catalog/api"
	catrepo "github.com/PandaX185/clinic-management/internal/catalog/repo"
	catsvc "github.com/PandaX185/clinic-management/internal/catalog/service"
	clinicapi "github.com/PandaX185/clinic-management/internal/clinic/api"
	clinicrepo "github.com/PandaX185/clinic-management/internal/clinic/repo"
	clinicsvc "github.com/PandaX185/clinic-management/internal/clinic/service"
	idapi "github.com/PandaX185/clinic-management/internal/identity/api"
	idrepo "github.com/PandaX185/clinic-management/internal/identity/repo"
	idsvc "github.com/PandaX185/clinic-management/internal/identity/service"
	portalapi "github.com/PandaX185/clinic-management/internal/portal/api"
	portalrepo "github.com/PandaX185/clinic-management/internal/portal/repo"
	portalsvc "github.com/PandaX185/clinic-management/internal/portal/service"
	queueapi "github.com/PandaX185/clinic-management/internal/queue/api"
	queuerepo "github.com/PandaX185/clinic-management/internal/queue/repo"
	queuesvc "github.com/PandaX185/clinic-management/internal/queue/service"
	schedapi "github.com/PandaX185/clinic-management/internal/scheduling/api"
	schedrepo "github.com/PandaX185/clinic-management/internal/scheduling/repo"
	schedsvc "github.com/PandaX185/clinic-management/internal/scheduling/service"
	server "github.com/PandaX185/clinic-management/internal/server"

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

	authRepo := idrepo.NewPostgresRepository(d.Pool)
	tokens := idsvc.NewTokenManager(d.Cfg.JWTSecret, d.Cfg.JWTRefreshSecret, d.Cfg.AccessTokenTTL, d.Cfg.RefreshTokenTTL)
	authSvc := idsvc.NewService(authRepo, tokens, d.Cfg.BcryptCost)
	authH := idapi.NewHandler(authSvc)

	clinicStore := clinicrepo.NewPostgresStore(d.Pool)
	profileStore := clinicrepo.NewScopedProfileStore(d.Pool)
	clinicSvc := clinicsvc.NewService(clinicStore, profileStore, clinicStore)
	clinicH := clinicapi.NewHandler(clinicSvc)

	// Real "my clinics" resolution for /auth/clinics: global user_tenants
	// index + per-tenant role lookup. Defined here to keep identity→clinic acyclic.
	membershipProvider := &clinicMembershipProvider{
		pool:  d.Pool,
		store: clinicStore,
	}
	authSvc.WithClinicMemberships(membershipProvider)

	aptRepo := schedrepo.NewPostgresRepository(d.Pool)
	aptSvc := schedsvc.NewServiceWithIdentity(aptRepo, nil, schedrepo.NewPostgresIdentityResolver(d.Pool), d.Cfg.IdempotencyTTL)
	aptH := schedapi.NewHandler(aptSvc)

	dirRepo := catrepo.NewPostgresRepo(d.Pool)
	dirSvc := catsvc.NewService(dirRepo)
	dirH := catapi.NewHandler(dirSvc)

	// Public clinic discovery (unauthenticated) and the patient portal
	// (JWT-only, no X-Tenant-ID). The patient service reuses the appointment
	// service, pinning the tenant schema from the clinic the patient picks.
	publicRepo := bookingrepo.NewPostgresRepository(d.Pool)
	publicSvc := bookingsvc.NewService(publicRepo)
	publicH := bookingapi.NewHandler(publicSvc)

	patientRepo := portalrepo.NewPostgresRepository(d.Pool)

	queueRepo := queuerepo.NewPostgresRepository(d.Pool)
	queueSvc := queuesvc.NewService(queueRepo)
	queueH := queueapi.NewHandler(queueSvc)

	patientSvc := portalsvc.NewService(patientRepo, aptSvc, queueSvc)
	patientH := portalapi.NewHandler(patientSvc)

	r := server.NewRouter(server.RouterDeps{
		Cfg:             d.Cfg,
		RDB:             d.RDB,
		Logger:          d.Log,
		AuthH:           authH,
		AuthSvc:         authSvc,
		AppointH:        aptH,
		ClinicH:         clinicH,
		ClinicSvc:       clinicSvc,
		ProfileResolver: profileStore,
		DirectoryH:      dirH,
		PublicH:         publicH,
		PatientH:        patientH,
		QueueH:          queueH,
		Metrics:         m,
	})

	// Liveness/readiness probes live at the root (outside /api/v1) so
	// orchestrators can check them without auth or a tenant context.
	server.NewHealth(d.Cfg, d.Pool, d.RDB, d.NATS).RegisterRoutes(r)

	return r, m, nil
}
