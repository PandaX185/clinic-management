package server

import (
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	appt "github.com/PandaX185/clinic-management/internal/appointment"
	auth "github.com/PandaX185/clinic-management/internal/auth"
	doctor "github.com/PandaX185/clinic-management/internal/doctor"
	patient "github.com/PandaX185/clinic-management/internal/patient"

	"github.com/PandaX185/clinic-management/internal/platform/config"
)

type RouterDeps struct {
	Cfg      config.Config
	RDB      *redis.Client
	Logger   Logger
	AuthH    *auth.Handler
	AuthSvc  *auth.Service
	PatientH *patient.Handler
	DoctorH  *doctor.Handler
	AppointH *appt.Handler
}

func NewRouter(deps RouterDeps) *gin.Engine {
	if deps.Cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(RequestID())
	r.Use(ErrorHandler(deps.Logger))
	r.Use(RateLimit(deps.RDB, deps.Cfg.RateLimitPerMinute))

	apiV1 := r.Group("/api/v1")

	deps.AuthH.RegisterRoutes(apiV1)

	protected := apiV1.Group("")
	protected.Use(auth.Middleware(deps.AuthSvc))

	staffOnly := auth.RequireRoles(string(auth.RoleAdmin), string(auth.RoleStaff))

	deps.PatientH.RegisterRoutes(protected, staffOnly)
	deps.DoctorH.RegisterRoutes(protected, staffOnly)
	deps.AppointH.RegisterRoutes(protected)
	deps.AuthH.RegisterProtectedRoutes(protected)

	return r
}
