package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/axiom/clinic-appointment/internal/auth"
	"github.com/axiom/clinic-appointment/internal/platform/config"
	"github.com/axiom/clinic-appointment/internal/platform/db"
	sqlc "github.com/axiom/clinic-appointment/internal/platform/db/sqlc"
	"github.com/axiom/clinic-appointment/internal/platform/logger"
	"github.com/axiom/clinic-appointment/internal/platform/metrics"
	"github.com/axiom/clinic-appointment/internal/platform/nats"
	"github.com/axiom/clinic-appointment/internal/platform/redis"
	"github.com/axiom/clinic-appointment/internal/platform/tracing"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// DEBUG: Print loaded config
	log.Printf("DEBUG: Loaded config - Port: %s, DB: %s, Redis: %s, NATS: %s", cfg.Port, cfg.DatabaseURL, cfg.RedisURL, cfg.NATSURL)

	// Initialize logger
	logr := logger.New(cfg.LogLevel)
	logr.Info("Starting Clinic Appointment System", zap.String("version", "dev"))

	// Initialize OpenTelemetry
	shutdownOTEL, err := tracing.Init(&tracing.Config{
		ServiceName:    "clinic-appointment",
		ServiceVersion: "dev",
	})
	if err != nil {
		logr.Error("Failed to initialize tracing", zap.Error(err))
	}
	defer func() {
		_ = shutdownOTEL(context.Background())
	}()

	// Initialize metrics
	metrics.Init()

	// Database connection
	dbpool, err := db.NewPool(cfg.DatabaseURL)
	if err != nil {
		logr.Error("Failed to connect to database", zap.Error(err))
		os.Exit(1)
	}
	defer dbpool.Close()

	// Redis connection
	redisClient, err := redis.NewClient(cfg.RedisURL)
	if err != nil {
		logr.Error("Failed to connect to Redis", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		_ = redisClient.Close()
	}()

	// NATS JetStream connection
	nc, js, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logr.Error("Failed to connect to NATS", zap.Error(err))
		os.Exit(1)
	}
	defer nc.Close()

	// Initialize JetStream streams
	if err := nats.SetupStreams(js); err != nil {
		logr.Error("Failed to setup NATS streams", zap.Error(err))
		os.Exit(1)
	}

	// HTTP router
	router := setupRouter(logr, dbpool, redisClient, js, cfg)

	// HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logr.Info("Shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			logr.Error("Server shutdown error", zap.Error(err))
		}
	}()

	logr.Info("Server starting", zap.String("port", cfg.Port))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logr.Error("Server error", zap.Error(err))
		os.Exit(1)
	}

	logr.Info("Server stopped")
}

func setupRouter(logr *logger.Logger, dbpool *db.Pool, redisClient *redis.Client, js jetstream.JetStream, cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(logr.HTTPLogger)
	r.Use(metrics.HTTPMetrics)

	// Health checks
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

	r.Route("/api/v1", func(r chi.Router) {
		// Auth routes (public)
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/refresh", authHandler.Refresh)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth([]byte(cfg.JWTSecret)))
			r.Post("/auth/logout", authHandler.Logout)

			// Patients - TODO: implement patientHandler
			r.Post("/patients", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
			r.Get("/patients/{id}", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
			r.Patch("/patients/{id}", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})

			// Doctors - TODO: implement doctorHandler
			r.Post("/doctors", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
			r.Get("/doctors", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
			r.Get("/doctors/{id}/availability", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})

			// Appointments - TODO: implement appointmentHandler
			r.Post("/appointments", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
			r.Get("/appointments/{id}", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
			r.Get("/appointments", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
			r.Post("/appointments/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
			r.Post("/appointments/{id}/reschedule", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = w.Write([]byte(`{"error":"not implemented"}`))
			})
		})
	})

	return r
}

func mustDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("invalid duration %q, using fallback %v", s, fallback)
		return fallback
	}
	return d
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyHandler(dbpool *db.Pool, redisClient *redis.Client) http.HandlerFunc {
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
