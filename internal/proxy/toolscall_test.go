package proxy

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/metrics"
	"github.com/mayur-tolexo/leanmcp/store"
)

// failingStore always errors on Put, simulating an unavailable backend.
type failingStore struct{}

// Put always fails so the optimizer's fail-open path can be exercised.
func (failingStore) Put(context.Context, string, []byte, int) (string, error) {
	return "", errors.New("store down")
}

// Get always reports not found; unused by the fail-open test.
func (failingStore) Get(context.Context, string) (*store.Entry, error) {
	return nil, store.ErrNotFound
}

func bigArray() []byte {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < 200; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"id":"x","name":"some-fairly-long-name","status":"active"}`)
	}
	b.WriteByte(']')
	return []byte(b.String())
}

func TestOptimizeSmallPassesThrough(t *testing.T) {
	o := newOptimizer(store.NewMemory(), &config.Config{OptimizeThresholdTokens: 1500, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20}, nil)
	out, err := o.process(context.Background(), "credA", "small-tool", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.Text != `{"ok":true}` || out.Compacted {
		t.Fatalf("small should pass through: %+v", out)
	}
}

func TestOptimizeHeavyCompactsAndCaches(t *testing.T) {
	mem := store.NewMemory()
	o := newOptimizer(mem, &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20}, nil)
	out, err := o.process(context.Background(), "credA", "list-tool", bigArray())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !out.Compacted {
		t.Fatalf("heavy should compact")
	}
	if out.Handle == "" {
		t.Fatalf("expected handle")
	}
	if !strings.Contains(out.Text, "expand_result") {
		t.Fatalf("expected trailer")
	}
	if _, err := mem.Get(context.Background(), out.Handle); err != nil {
		t.Fatalf("raw should be cached: %v", err)
	}
}

func TestOptimizeOversizeFailsOpen(t *testing.T) {
	o := newOptimizer(store.NewMemory(), &config.Config{OptimizeThresholdTokens: 1, ExpandTTL: time.Minute, MaxRawBytes: 10}, nil)
	raw := bigArray()
	out, err := o.process(context.Background(), "credA", "list-tool", raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.Compacted || out.Handle != "" {
		t.Fatalf("oversize must fail open, no handle")
	}
	if out.Text != string(raw) {
		t.Fatalf("oversize must return full raw")
	}
}

func bigObject() []byte {
	// A large single JSON object: heavy by token count but not an array, so the
	// tabular transform cannot apply.
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i < 200; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"key`)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`":"some-fairly-long-value-string"`)
	}
	b.WriteByte('}')
	return []byte(b.String())
}

func TestOptimizeHeavyButUncompactablePassesThrough(t *testing.T) {
	o := newOptimizer(store.NewMemory(), &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20}, nil)
	raw := bigObject()
	out, err := o.process(context.Background(), "credA", "obj-tool", raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Compaction did not apply, so the returned text must never exceed the
	// original (no trailer appended, no handle, no enlargement).
	if out.Compacted || out.Handle != "" {
		t.Fatalf("uncompactable heavy result must not issue a handle: %+v", out)
	}
	if out.Text != string(raw) {
		t.Fatalf("uncompactable result must pass through unchanged")
	}
	if len(out.Text) > len(raw) {
		t.Fatalf("returned payload must never be larger than original")
	}
}

func TestOptimizeStoreErrorFailsOpen(t *testing.T) {
	o := newOptimizer(failingStore{}, &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20}, nil)
	raw := bigArray()
	out, err := o.process(context.Background(), "credA", "list-tool", raw)
	if err != nil {
		t.Fatalf("store error must not propagate: %v", err)
	}
	if out.Handle != "" || out.Compacted {
		t.Fatalf("store error must fail open with no handle: %+v", out)
	}
	if out.Text != string(raw) {
		t.Fatalf("store error must return full raw result")
	}
}

func TestShadowModeReturnsFullButCaches(t *testing.T) {
	mem := store.NewMemory()
	o := newOptimizer(mem, &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20, ShadowMode: true}, nil)
	out, err := o.process(context.Background(), "credA", "list-tool", bigArray())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.Compacted {
		t.Fatalf("shadow must not compact client-facing result")
	}
	if !strings.Contains(out.Text, `"status":"active"`) {
		t.Fatalf("shadow must return full result")
	}
}

// TestMetricsEnabledOptimizerNoPanic verifies that an optimizer wired with a
// real *Metrics processes a heavy compactable array without panicking, and that
// the metrics handler is non-nil after recording.
func TestMetricsEnabledOptimizerNoPanic(t *testing.T) {
	m := metrics.New()
	o := newOptimizer(store.NewMemory(), &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20}, m)
	_, err := o.process(context.Background(), "credA", "list-tool", bigArray())
	if err != nil {
		t.Fatalf("process with metrics: %v", err)
	}
	if m.Handler() == nil {
		t.Fatal("metrics handler must be non-nil after recording")
	}
}
