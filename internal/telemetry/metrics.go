package telemetry

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	Registry = prometheus.NewRegistry()

	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zephyrcache",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests.",
		},
		[]string{"op", "status"},
	)

	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "zephyrcache",
			Name:      "request_duration_seconds",
			Help:      "Latency of HTTP requests.",
			// Tune buckets to your SLOs. This covers 1ms .. ~4s.
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 13),
		},
		[]string{"op"},
	)

	InFlight = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "zephyrcache",
			Name:      "in_flight_requests",
			Help:      "Current number of in-flight HTTP requests.",
		},
		[]string{"op"},
	)

	// ---- Process / build info ----
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "zephyrcache",
			Name:      "build_info",
			Help:      "Build info (constant 1, labeled by version and git_sha).",
		},
		[]string{"version", "git_sha"},
	)

	startTime = time.Now()
	uptime    = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: "zephyrcache",
			Name:      "uptime_seconds",
			Help:      "Process uptime in seconds.",
		},
		func() float64 { return time.Since(startTime).Seconds() },
	)
)

func init() {
	Registry.MustRegister(RequestsTotal, RequestDuration, InFlight, buildInfo, uptime)
}

// MetricsHandler exposes /metrics. Mount it with mux.Handle("/metrics", telemetry.MetricsHandler()).
func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}

// SetBuildInfo should be called once at startup, e.g. with ldflags-provided values.
func SetBuildInfo(version, gitSHA string) {
	buildInfo.WithLabelValues(version, gitSHA).Set(1)
}

// ---- Middleware instrumentation ----

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Instrument wraps an http.Handler to record metrics under the provided "op" label.
// Example:
//
//	mux.HandleFunc("/info", telemetry.Instrument("info", http.HandlerFunc(s.info)).ServeHTTP)
func Instrument(op string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()

		InFlight.WithLabelValues(op).Inc()
		defer InFlight.WithLabelValues(op).Dec()

		next.ServeHTTP(sw, r)

		class := strconv.Itoa(sw.status/100) + "xx"
		RequestsTotal.WithLabelValues(op, class).Inc()
		RequestDuration.WithLabelValues(op).Observe(time.Since(start).Seconds())
	})
}
