package proxy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/store"
)

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
	o := newOptimizer(store.NewMemory(), &config.Config{OptimizeThresholdTokens: 1500, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20})
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
	o := newOptimizer(mem, &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20})
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
	o := newOptimizer(store.NewMemory(), &config.Config{OptimizeThresholdTokens: 1, ExpandTTL: time.Minute, MaxRawBytes: 10})
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

func TestShadowModeReturnsFullButCaches(t *testing.T) {
	mem := store.NewMemory()
	o := newOptimizer(mem, &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20, ShadowMode: true})
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
