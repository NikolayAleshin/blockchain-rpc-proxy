package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the RED instruments (Rate, Errors, Duration) plus an in-flight
// gauge, registered on a caller-provided registry (no global state).
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inflight prometheus.Gauge
}

// NewMetrics registers the instruments on a fresh registry.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "rpc", Name: "requests_total",
			Help: "Total proxied requests by HTTP method and status.",
		}, []string{"method", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "rpc", Name: "request_duration_seconds",
			Help:    "Request duration in seconds by HTTP method and status.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "status"}),
		inflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "rpc", Name: "requests_in_flight",
			Help: "In-flight proxied requests.",
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.inflight)
	return m
}

// Handler exposes the metrics for Prometheus scraping (GET /metrics).
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Middleware records RED metrics around the next handler.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.inflight.Inc()
		defer m.inflight.Dec()

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		labels := prometheus.Labels{"method": r.Method, "status": strconv.Itoa(sw.status)}
		m.requests.With(labels).Inc()
		m.duration.With(labels).Observe(time.Since(start).Seconds())
	})
}
