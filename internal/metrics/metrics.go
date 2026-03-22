// Package metrics provides Prometheus instrumentation for WireGate.
package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP metrics.
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "wiregate",
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Total number of HTTP requests.",
	}, []string{"method", "route", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "wiregate",
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "HTTP request latency in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})

	HTTPRequestsInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wiregate",
		Subsystem: "http",
		Name:      "requests_in_flight",
		Help:      "Number of HTTP requests currently being processed.",
	})

	// WireGuard adapter metrics.
	WGPingSuccess = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wiregate",
		Subsystem: "wg",
		Name:      "ping_success",
		Help:      "1 if the last WireGuard adapter ping succeeded, 0 otherwise.",
	})

	WGPeersRuntime = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wiregate",
		Subsystem: "wg",
		Name:      "peers_runtime_total",
		Help:      "Number of peers currently configured on the WireGuard interface.",
	})

	// Business metrics.
	AgentsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "wiregate",
		Name:      "agents_total",
		Help:      "Total number of agents by status.",
	}, []string{"status"})

	PeersTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "wiregate",
		Name:      "peers_total",
		Help:      "Total number of peers by status.",
	}, []string{"status"})

	SessionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wiregate",
		Name:      "sessions_active_total",
		Help:      "Number of active (non-expired, non-revoked) sessions.",
	})

	EnrollmentTokensTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "wiregate",
		Name:      "enrollment_tokens_total",
		Help:      "Total enrollment tokens by status.",
	}, []string{"status"})

	ReconcileDrifted = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "wiregate",
		Name:      "reconcile_drifted_peers",
		Help:      "Number of peers with drift from intended state.",
	})
)

// Middleware wraps an http.Handler with request metrics.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		route := normalizeRoute(r.URL.Path)

		HTTPRequestsInFlight.Inc()
		defer HTTPRequestsInFlight.Dec()

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rw.status)

		HTTPRequestsTotal.WithLabelValues(r.Method, route, status).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// normalizeRoute collapses UUIDs and numeric IDs in paths to keep label
// cardinality bounded. e.g. /api/v1/agents/abc-123 → /api/v1/agents/:id
func normalizeRoute(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if looksLikeID(p) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func looksLikeID(s string) bool {
	if len(s) < 8 {
		return false
	}
	// UUID pattern (contains dashes and hex chars).
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	// Long hex or alphanumeric token.
	if len(s) >= 20 {
		for _, c := range s {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-') {
				return false
			}
		}
		return true
	}
	return false
}
