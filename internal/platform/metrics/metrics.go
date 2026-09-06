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

	NotificationsDeliveredTotal prometheus.Counter
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
		NotificationsDeliveredTotal: f.NewCounter(prometheus.CounterOpts{
			Name: "notifications_delivered_total",
			Help: "Total notifications delivered by the async worker.",
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
