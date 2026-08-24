package server

import (
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	appt "github.com/PandaX185/clinic-management/internal/appointment"
	auth "github.com/PandaX185/clinic-management/internal/auth"
	doctor "github.com/PandaX185/clinic-management/internal/doctor"
	patient "github.com/PandaX185/clinic-management/internal/patient"
	tenant "github.com/PandaX185/clinic-management/internal/tenant"

	"github.com/PandaX185/clinic-management/internal/platform/config"
	"github.com/PandaX185/clinic-management/internal/platform/metrics"
)

type RouterDeps struct {
	Cfg             config.Config
	RDB             *redis.Client
	Logger          Logger
	AuthH           *auth.Handler
	AuthSvc         *auth.Service
	PatientH        *patient.Handler
	DoctorH         *doctor.Handler
	AppointH        *appt.Handler
	TenantH         *tenant.Handler
	TenantSvc       *tenant.Service
	ProfileResolver ProfileResolver
	Metrics         *metrics.Metrics
}

func NewRouter(deps RouterDeps) *gin.Engine {
	if deps.Cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	// Never trust client-supplied X-Forwarded-For: only the direct peer is
	// a valid ClientIP source unless an explicit proxy list is configured
	// (SEC-05). Behind a reverse proxy, set GIN_TRUSTED_PROXIES.
	if trusted := os.Getenv("GIN_TRUSTED_PROXIES"); trusted != "" {
		_ = r.SetTrustedProxies(strings.Split(trusted, ","))
	} else {
		_ = r.SetTrustedProxies(nil)
	}

	r.Use(gin.Recovery())
	r.Use(RequestID())
	if deps.Metrics != nil {
		r.Use(MetricsMiddleware(deps.Metrics))
	}
	// ErrorHandler must run BEFORE route middleware so it can convert
	// aborts (c.Error + c.Abort) from e.g. TenantMiddleware into responses.
	r.Use(ErrorHandler(deps.Logger))
	r.Use(RateLimit(deps.RDB, deps.Cfg.RateLimitPerMinute))
	r.Use(SecurityHeaders())
	r.Use(BodyLimit(maxBodyBytes))

	apiV1 := r.Group("/api/v1")

	deps.AuthH.RegisterRoutes(apiV1)

	// Global (auth-only) routes: tenant browsing needs no X-Tenant-ID.
	global := apiV1.Group("")
	global.Use(auth.Middleware(deps.AuthSvc))

	// Tenant-scoped routes: X-Tenant-ID required; role resolved per clinic.
	protected := apiV1.Group("")
	protected.Use(auth.Middleware(deps.AuthSvc))
	protected.Use(TenantMiddleware(deps.TenantSvc, deps.ProfileResolver))

	staffOnly := auth.RequireRoles(string(auth.RoleAdmin), string(auth.RoleStaff))

	deps.TenantH.RegisterRoutes(global) // GET /tenants — browse clinics

	deps.PatientH.RegisterRoutes(protected, staffOnly)
	deps.DoctorH.RegisterRoutes(protected, staffOnly)
	deps.AppointH.RegisterRoutes(protected)

	return r
}
