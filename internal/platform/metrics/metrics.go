package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Database metrics
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Database query latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"query", "status"},
	)

	DBPoolConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "db_pool_connections",
			Help: "Current database pool connections",
		},
		[]string{"state"},
	)

	// Redis metrics
	RedisCommandsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_commands_total",
			Help: "Total number of Redis commands",
		},
		[]string{"command", "status"},
	)

	RedisCommandDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_command_duration_seconds",
			Help:    "Redis command latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"command"},
	)

	// Cache metrics
	CacheHitsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	CacheMissesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
		},
	)

	// NATS/JetStream metrics
	NATSMessagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nats_messages_published_total",
			Help: "Total number of NATS messages published",
		},
		[]string{"subject", "status"},
	)

	NATSMessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nats_messages_consumed_total",
			Help: "Total number of NATS messages consumed",
		},
		[]string{"subject", "status"},
	)

	NATSMessageProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nats_message_processing_duration_seconds",
			Help:    "NATS message processing latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"subject"},
	)

	// Appointment metrics
	AppointmentBookingConflicts = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "appointment_booking_conflicts_total",
			Help: "Total number of appointment booking conflicts",
		},
	)

	AppointmentCreatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "appointment_created_total",
			Help: "Total number of appointments created",
		},
	)

	// Notification metrics
	NotificationsSentTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_sent_total",
			Help: "Total number of notifications sent",
		},
		[]string{"type", "status"},
	)

	NotificationRetriesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "notification_retries_total",
			Help: "Total number of notification retries",
		},
	)

	NotificationDLQTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "notification_dlq_total",
			Help: "Total number of notifications sent to DLQ",
		},
	)
)

func Init() {
	// Metrics are auto-registered via promauto
}

func HTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start).Seconds()
		HTTPRequestsTotal.WithLabelValues(r.Method, r.URL.Path, fmt.Sprintf("%d", wrapped.statusCode)).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}