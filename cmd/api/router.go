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
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Pinger abstracts a dependency health check so /ready can be tested with
// stubs instead of live connections.
type Pinger interface {
	Ping(ctx context.Context) error
}

func setupRouter(logr *logger.Logger, dbpool *db.Pool, redisClient *redis.Client, _ jetstream.JetStream, cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.Recoverer)
	r.Use(logr.HTTPLogger)
	r.Use(metrics.HTTPMetrics)

	// Health checks + metrics
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler(dbpool, redisClient))

	// API routes
	queries := sqlc.New(dbpool)
	authHandler := auth.NewHandler(
		auth.NewSQLUserStore(queries),
		auth.NewRedisTokenStore(redisClient.Client),
		auth.Config{
			AccessSecret:  []byte(cfg.JWTSecret),
			RefreshSecret: []byte(cfg.JWTRefreshSecret),
			AccessTTL:     mustDuration(cfg.JWTAccessTTL, 15*time.Minute),
			RefreshTTL:    mustDuration(cfg.JWTRefreshTTL, 168*time.Hour),
		},
	)

	// NOTE (PLATFORM-001 phase 2): the nine inline 501 placeholder routes
	// were deleted. Unregistered paths now return chi's 404; the OpenAPI
	// spec is the contract of record for not-yet-implemented domains.
	r.Route("/api/v1", func(r chi.Router) {
		// Auth routes (public)
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/refresh", authHandler.Refresh)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth([]byte(cfg.JWTSecret)))
			r.Post("/auth/logout", authHandler.Logout)
		})
	})

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

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyHandler(dbpool Pinger, redisClient Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := dbpool.Ping(ctx); err != nil {
			http.Error(w, "database not ready", http.StatusServiceUnavailable)
			return
		}
		if err := redisClient.Ping(ctx); err != nil {
			http.Error(w, "redis not ready", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}
