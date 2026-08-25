package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry *prometheus.Registry

	HTTPRequestsTotal  *prometheus.CounterVec
	HTTPRequestLatency *prometheus.HistogramVec
	DBQueryLatency     *prometheus.HistogramVec
	CacheHits          prometheus.Counter
	CacheMisses        prometheus.Counter
	BookingConflicts   prometheus.Counter
	NotificationsSent  *prometheus.CounterVec
	QueueDepth         prometheus.Gauge
}

func New() *Metrics {
	reg := prometheus.NewRegistry()
	f := promauto.With(reg)

	m := &Metrics{
		Registry: reg,
		HTTPRequestsTotal: f.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, route and status.",
		}, []string{"method", "route", "status"}),
		HTTPRequestLatency: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		DBQueryLatency: f.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Database query latency in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		}, []string{"operation"}),
		CacheHits: f.NewCounter(prometheus.CounterOpts{
			Name: "cache_hits_total",
		}),
		CacheMisses: f.NewCounter(prometheus.CounterOpts{
			Name: "cache_misses_total",
		}),
		BookingConflicts: f.NewCounter(prometheus.CounterOpts{
			Name: "appointment_booking_conflicts_total",
			Help: "Rejected bookings due to slot conflicts.",
		}),
		NotificationsSent: f.NewCounterVec(prometheus.CounterOpts{
			Name: "notifications_total",
			Help: "Notification processing outcomes.",
		}, []string{"status"}),
		QueueDepth: f.NewGauge(prometheus.GaugeOpts{
			Name: "notification_queue_depth",
		}),
	}
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.InstrumentMetricHandler(
		m.Registry,
		promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}),
	)
}
