package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNilMetricsNoPanic verifies that all record methods on a nil *Metrics
// return without panicking, and Handler() returns a non-nil handler that
// responds 503.
func TestNilMetricsNoPanic(t *testing.T) {
	var m *Metrics

	// None of these must panic.
	m.RecordCompaction("tool", 1000, 200)
	m.RecordExpand()
	m.RecordFailOpen("reason")

	h := m.Handler()
	if h == nil {
		t.Fatal("nil Metrics.Handler() must return non-nil handler")
	}

	// Nil handler must serve 503.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != 503 {
		t.Fatalf("nil handler expected 503, got %d", w.Code)
	}
}

// TestNewMetricsNoPanic verifies that record methods on a real *Metrics
// returned by New() do not panic, and Handler() returns a non-nil handler
// that serves the registered metrics.
func TestNewMetricsNoPanic(t *testing.T) {
	m := New()

	// Record without panicking.
	m.RecordCompaction("my-tool", 5000, 1200)
	m.RecordExpand()
	m.RecordFailOpen("oversize")

	h := m.Handler()
	if h == nil {
		t.Fatal("New().Handler() must return non-nil handler")
	}

	// Handler must respond 200 and include the registered metric names.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != 200 {
		t.Fatalf("metrics handler expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, name := range []string{
		"leanmcp_result_bytes_original",
		"leanmcp_result_bytes_compacted",
		"leanmcp_expand_total",
		"leanmcp_fail_open_total",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("metrics output missing %q", name)
		}
	}
}
