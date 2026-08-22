package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/axiom/clinic-appointment/internal/auth"
	"github.com/axiom/clinic-appointment/internal/platform/config"
	"github.com/axiom/clinic-appointment/internal/platform/db"
	sqlc "github.com/axiom/clinic-appointment/internal/platform/db/sqlc"
	"github.com/axiom/clinic-appointment/internal/platform/logger"
	"github.com/axiom/clinic-appointment/internal/platform/metrics"
	"github.com/axiom/clinic-appointment/internal/platform/redis"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Pinger abstracts a dependency health check so /ready can be tested with
// stubs instead of live connections.
type Pinger interface {
	Ping(ctx context.Context) error
}

func setupRouter(logr *logger.Logger, dbpool *db.Pool, redisClient *redis.Client, _ jetstream.JetStream, cfg *config.Config) http.Handler {
	queries := sqlc.New(dbpool)
	return newRouter(logr,
		auth.NewSQLUserStore(queries),
		auth.NewRedisTokenStore(redisClient.Client),
		cfg,
		dbpool, redisClient,
	)
}

// newRouter builds the gin engine. Kept separate from setupRouter so tests
// can wire it with fake stores and stub pingers.
func newRouter(
	logr *logger.Logger,
	users auth.UserStore,
	tokens auth.TokenStore,
	cfg *config.Config,
	dbp Pinger,
	redisP Pinger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(logr.HTTPLogger())
	r.Use(metrics.HTTPMetrics())
	r.Use(auth.MaxBody(auth.MaxBodyBytes))

	// Health checks + metrics
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/health", healthHandler)
	r.GET("/ready", readyHandler(dbp, redisP))

	authHandler := auth.NewHandler(users, tokens, auth.Config{
		AccessSecret:  []byte(cfg.JWTSecret),
		RefreshSecret: []byte(cfg.JWTRefreshSecret),
		AccessTTL:     mustDuration(cfg.JWTAccessTTL, 15*time.Minute),
		RefreshTTL:    mustDuration(cfg.JWTRefreshTTL, 168*time.Hour),
	})

	api := r.Group("/api/v1")

	// Auth routes (public)
	authPub := api.Group("/auth")
	{
		authPub.POST("/register", authHandler.Register)
		authPub.POST("/login", authHandler.Login)
		authPub.POST("/refresh", authHandler.Refresh)
	}

	// Protected routes
	protected := api.Group("", auth.RequireAuth([]byte(cfg.JWTSecret)))
	{
		protected.POST("/auth/logout", authHandler.Logout)
	}

	return r
}

// mustDuration parses a duration config value, falling back only when the
// value is empty. A malformed value is fatal — we refuse to guess.
func mustDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Fatalf("invalid duration %q: %v", s, err)
	}
	return d
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func readyHandler(dbpool Pinger, redisP Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := dbpool.Ping(ctx); err != nil {
			c.String(http.StatusServiceUnavailable, "database not ready")
			return
		}
		if err := redisP.Ping(ctx); err != nil {
			c.String(http.StatusServiceUnavailable, "redis not ready")
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
