// Package metrics exposes Prometheus collectors for leanmcp.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the registered collectors. All record methods are nil-safe so
// call sites need not branch on whether metrics are enabled.
type Metrics struct {
	reg          *prometheus.Registry
	origBytes    *prometheus.HistogramVec
	compactBytes *prometheus.HistogramVec
	expands      prometheus.Counter
	failOpen     *prometheus.CounterVec
}

// New registers and returns the metric collectors.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		reg: reg,
		origBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "leanmcp_result_bytes_original", Help: "Original tool-result size in bytes.",
			Buckets: prometheus.ExponentialBuckets(256, 4, 8),
		}, []string{"tool"}),
		compactBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "leanmcp_result_bytes_compacted", Help: "Compacted tool-result size in bytes.",
			Buckets: prometheus.ExponentialBuckets(256, 4, 8),
		}, []string{"tool"}),
		expands: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "leanmcp_expand_total", Help: "expand_result calls.",
		}),
		failOpen: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leanmcp_fail_open_total", Help: "Fail-open events by reason.",
		}, []string{"reason"}),
	}
	reg.MustRegister(m.origBytes, m.compactBytes, m.expands, m.failOpen)
	return m
}

// RecordCompaction records original and compacted sizes for a tool. Nil-safe.
func (m *Metrics) RecordCompaction(tool string, orig, compacted int) {
	if m == nil {
		return
	}
	m.origBytes.WithLabelValues(tool).Observe(float64(orig))
	m.compactBytes.WithLabelValues(tool).Observe(float64(compacted))
}

// RecordExpand counts an expand_result call. Nil-safe.
func (m *Metrics) RecordExpand() {
	if m == nil {
		return
	}
	m.expands.Inc()
}

// RecordFailOpen counts a fail-open event by reason. Nil-safe.
func (m *Metrics) RecordFailOpen(reason string) {
	if m == nil {
		return
	}
	m.failOpen.WithLabelValues(reason).Inc()
}

// Handler returns the /metrics HTTP handler. Nil-safe (503 when nil).
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
	}
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}
