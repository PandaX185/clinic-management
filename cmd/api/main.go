package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	appt "github.com/PandaX185/clinic-management/internal/appointment"
	auth "github.com/PandaX185/clinic-management/internal/auth"
	doctor "github.com/PandaX185/clinic-management/internal/doctor"
	notification "github.com/PandaX185/clinic-management/internal/notification"
	patient "github.com/PandaX185/clinic-management/internal/patient"
	server "github.com/PandaX185/clinic-management/internal/server"

	"github.com/PandaX185/clinic-management/internal/platform/config"
	"github.com/PandaX185/clinic-management/internal/platform/database"
	"github.com/PandaX185/clinic-management/internal/platform/logger"
	"github.com/PandaX185/clinic-management/internal/platform/metrics"
	natsclient "github.com/PandaX185/clinic-management/internal/platform/nats"
	redisclient "github.com/PandaX185/clinic-management/internal/platform/redis"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log, err := logger.New(cfg.LogLevel, cfg.Format)
	if err != nil {
		return err
	}
	lg := newLogger(log)
	defer log.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", zap.Error(err))
		return err
	}
	defer pool.Close()
	log.Info("connected to postgres")

	rdb := tryRedis(ctx, cfg.RedisURL, log)
	natsClient := tryNATS(ctx, cfg.NATSURL, log)

	if natsClient != nil {
		store := notification.NewPostgresStore(pool)
		worker := notification.NewWorker(
			natsClient.Jet,
			notification.NewConsumer(store, notification.NewJetAdapter(natsClient.Jet), &notification.LogProvider{Logger: lg}, lg, 5*time.Second).Handle,
			lg,
		)
		go func() {
			if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("notification worker stopped", zap.Error(err))
			}
		}()
	}

	m := metrics.New()

	authRepo := auth.NewPostgresRepository(pool)
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTRefreshSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	authSvc := auth.NewService(authRepo, tokens)
	authHandler := auth.NewHandler(authSvc)

	patientRepo := patient.NewPostgresRepository(pool)
	patientSvc := patient.NewService(patientRepo)
	patientHandler := patient.NewHandler(patientSvc)

	doctorRepo := doctor.NewPostgresRepository(pool)
	userBridge := newDoctorUserAdapter(authRepo, cfg.BcryptCost)
	doctorSvc := doctor.NewService(doctorRepo, userBridge)
	doctorHandler := doctor.NewHandler(doctorSvc)

	apptRepo := appt.NewPostgresRepository(pool)

	var eventForwarder appt.EventPublisher = noopPublisher{}
	if natsClient != nil {
		eventForwarder = notification.NewEventForwarder(notification.PublisherDeps{
			JetPublisher: notification.NewJetAdapter(natsClient.Jet),
			Store:        notification.NewPostgresStore(pool),
		})
	}

	apptSvc := appt.NewService(apptRepo, eventForwarder, newAuditWriter(pool), cfg.IdempotencyTTL)
	apptHandler := appt.NewHandler(apptSvc)

	router := server.NewRouter(server.RouterDeps{
		Cfg:      cfg,
		RDB:      rdb,
		Logger:   lg,
		AuthH:    authHandler,
		AuthSvc:  authSvc,
		PatientH: patientHandler,
		DoctorH:  doctorHandler,
		AppointH: apptHandler,
	})

	health := server.NewHealth(cfg, pool, rdb, natsClient)
	health.RegisterRoutes(router)

	mux := http.NewServeMux()
	mux.Handle("/metrics", m.Handler())
	mux.Handle("/", router)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting api server", zap.String("addr", srv.Addr))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", zap.Error(err))
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownPeriod)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}
	if natsClient != nil {
		natsClient.Close()
	}
	if rdb != nil {
		rdb.Close()
	}
	log.Info("server stopped")
	return nil
}

func tryRedis(ctx context.Context, url string, log *zap.Logger) *redisclient.Client {
	client, err := redisclient.New(ctx, url)
	if err != nil {
		log.Warn("redis unavailable; continuing degraded", zap.Error(err))
		return nil
	}
	log.Info("connected to redis")
	return client
}

func tryNATS(ctx context.Context, url string, log *zap.Logger) *natsclient.Client {
	client, err := natsclient.New(ctx, url)
	if err != nil {
		log.Warn("nats unavailable; notifications disabled", zap.Error(err))
		return nil
	}
	log.Info("connected to nats")
	return client
}
