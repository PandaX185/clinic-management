// Package wiring constructs the application's full dependency graph and
// HTTP router, keeping cmd/api/main.go a thin entry point concerned only with
// lifecycle (config, logger, connections, graceful shutdown).
package wiring

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	bookingapi "github.com/PandaX185/lahza/internal/booking/api"
	bookingrepo "github.com/PandaX185/lahza/internal/booking/repo"
	bookingsvc "github.com/PandaX185/lahza/internal/booking/service"
	catapi "github.com/PandaX185/lahza/internal/catalog/api"
	catrepo "github.com/PandaX185/lahza/internal/catalog/repo"
	catsvc "github.com/PandaX185/lahza/internal/catalog/service"
	clinicapi "github.com/PandaX185/lahza/internal/clinic/api"
	clinicrepo "github.com/PandaX185/lahza/internal/clinic/repo"
	clinicsvc "github.com/PandaX185/lahza/internal/clinic/service"
	idapi "github.com/PandaX185/lahza/internal/identity/api"
	idrepo "github.com/PandaX185/lahza/internal/identity/repo"
	idsvc "github.com/PandaX185/lahza/internal/identity/service"
	paymentapi "github.com/PandaX185/lahza/internal/payments/api"
	paymentrepo "github.com/PandaX185/lahza/internal/payments/repo"
	paymentsvc "github.com/PandaX185/lahza/internal/payments/service"
	portalapi "github.com/PandaX185/lahza/internal/portal/api"
	portalrepo "github.com/PandaX185/lahza/internal/portal/repo"
	portalsvc "github.com/PandaX185/lahza/internal/portal/service"
	queueapi "github.com/PandaX185/lahza/internal/queue/api"
	queuerepo "github.com/PandaX185/lahza/internal/queue/repo"
	queuesvc "github.com/PandaX185/lahza/internal/queue/service"
	schedapi "github.com/PandaX185/lahza/internal/scheduling/api"
	schedrepo "github.com/PandaX185/lahza/internal/scheduling/repo"
	schedsvc "github.com/PandaX185/lahza/internal/scheduling/service"
	server "github.com/PandaX185/lahza/internal/server"

	notif "github.com/PandaX185/lahza/internal/notification"
	"github.com/PandaX185/lahza/internal/platform/config"
	"github.com/PandaX185/lahza/internal/platform/metrics"
	natsclient "github.com/PandaX185/lahza/internal/platform/nats"
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
// configured gin.Engine and returns it alongside the metrics registry and an
// optional notification worker (nil when NATS is unavailable) so the caller
// can expose metrics and run worker goroutines.
func Build(d Deps) (*gin.Engine, *metrics.Metrics, *notif.Worker, error) {
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
		pool:     d.Pool,
		store:    clinicStore,
		profiles: profileStore,
	}
	authSvc.WithClinicMemberships(membershipProvider)

	aptRepo := schedrepo.NewPostgresRepository(d.Pool)
	var aptPublisher schedsvc.EventPublisher
	if d.NATS != nil {
		aptPublisher = notif.AppointmentEventPublisher{Bus: d.NATS, Subject: natsclient.SubjectNotify, Log: d.Log, Errors: m.EventPublishErrorsTotal}
	}
	aptSvc := schedsvc.NewServiceWithIdentity(aptRepo, aptPublisher, schedrepo.NewPostgresIdentityResolver(d.Pool), d.Cfg.IdempotencyTTL)
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

	paymentRepo := paymentrepo.NewPostgresRepository(d.Pool)
	var paymentPublisher schedsvc.EventPublisher
	if d.NATS != nil {
		paymentPublisher = notif.AppointmentEventPublisher{Bus: d.NATS, Subject: natsclient.SubjectNotify, Log: d.Log, Errors: m.EventPublishErrorsTotal}
	}
	paymentSvc := paymentsvc.NewService(paymentRepo, paymentPublisher, "EGP")
	paymentH := paymentapi.NewHandler(paymentSvc)

	patientSvc := portalsvc.NewService(patientRepo, aptSvc, queueSvc, paymentSvc)
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
		PaymentH:        paymentH,
		Metrics:         m,
	})

	// Liveness/readiness probes live at the root (outside /api/v1) so
	// orchestrators can check them without auth or a tenant context.
	server.NewHealth(d.Cfg, d.Pool, d.RDB, d.NATS).RegisterRoutes(r)

	var notifWorker *notif.Worker
	if d.NATS != nil {
		notifWorker = notif.NewWorker(d.NATS, &notif.Stub{Log: d.Log, M: m}, d.Log)
	}

	return r, m, notifWorker, nil
}
