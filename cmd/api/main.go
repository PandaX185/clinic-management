package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/axiom/clinic-appointment/internal/platform/config"
	"github.com/axiom/clinic-appointment/internal/platform/db"
	"github.com/axiom/clinic-appointment/internal/platform/logger"
	"github.com/axiom/clinic-appointment/internal/platform/nats"
	"github.com/axiom/clinic-appointment/internal/platform/redis"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	logr := logger.New(cfg.LogLevel)
	logr.Info("Starting Clinic Appointment System", zap.String("version", "dev"))

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
		if err := redisClient.Close(); err != nil {
			logr.Error("Redis close error", zap.Error(err))
		}
	}()

	// NATS JetStream connection
	nc, js, err := nats.Connect(cfg.NATSURL)
	if err != nil {
		logr.Error("Failed to connect to NATS", zap.Error(err))
		os.Exit(1)
	}
	defer nc.Close()

	if err := nats.SetupStreams(js); err != nil {
		logr.Error("Failed to setup NATS streams", zap.Error(err))
		os.Exit(1)
	}

	router := setupRouter(logr, dbpool, redisClient, js, cfg)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
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
