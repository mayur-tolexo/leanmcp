package proxy

import (
	"context"
	"encoding/json"

	"github.com/mayur-tolexo/leanmcp/internal/compact"
	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/metrics"
	"github.com/mayur-tolexo/leanmcp/internal/tokens"
	"github.com/mayur-tolexo/leanmcp/store"
)

// optimizer applies the heavy-result decision logic for a single tool call.
type optimizer struct {
	store   store.Store
	cfg     *config.Config
	metrics *metrics.Metrics
}

// newOptimizer constructs an optimizer backed by s, governed by cfg, and
// reporting to m (nil disables metrics).
func newOptimizer(s store.Store, cfg *config.Config, m *metrics.Metrics) *optimizer {
	return &optimizer{store: s, cfg: cfg, metrics: m}
}

// outcome is the result of optimizing one tool call.
type outcome struct {
	Text      string
	Compacted bool
	Handle    string
}

// process decides whether to optimize raw for the named tool. Small results pass
// through unchanged. Heavy results are compacted; the full raw is cached bound to
// credHash and a trailer is appended. In shadow mode the full result is returned
// to the client but still cached for measurement. Any caching failure (including
// oversize payloads) fails open by returning the full raw result with no handle.
func (o *optimizer) process(ctx context.Context, credHash, tool string, raw []byte) (outcome, error) {
	// Pass small results through without caching.
	if !tokens.Exceeds(raw, o.cfg.OptimizeThresholdTokens) {
		return outcome{Text: string(raw)}, nil
	}

	// Select compaction mode: tool-specific rule takes precedence over the default.
	mode := "tabular"
	if rule, ok := o.cfg.Tools[tool]; ok && rule.Compaction != "" {
		mode = rule.Compaction
	}
	res := compact.Compact(raw, compact.Options{Mode: mode})

	// Determine the effective compacted size for metrics: use View length when
	// compaction applied, otherwise treat the size as equal to the original.
	compactedSize := len(raw)
	if res.Applied {
		compactedSize = len(res.View)
	}
	// Record compaction sizes for all heavy results (including shadow/oversize) so
	// the metric captures would-be savings regardless of whether caching proceeds.
	o.metrics.RecordCompaction(tool, len(raw), compactedSize)

	// If no transform shrank the payload, pass the full result through untouched.
	// Appending a trailer here would make the returned text larger than the
	// original for no compaction benefit, violating the never-enlarge guarantee.
	if !res.Applied {
		return outcome{Text: string(raw)}, nil
	}

	// Refuse to cache payloads that exceed the configured hard size limit; return
	// the full raw so the caller still gets a valid (if large) response.
	if len(raw) > o.cfg.MaxRawBytes {
		o.metrics.RecordFailOpen("oversize")
		return outcome{Text: string(raw)}, nil
	}

	// Shadow mode: cache for measurement but return the full result to the client
	// so compaction is invisible until it has been validated.
	if o.cfg.ShadowMode {
		_, _ = o.store.Put(ctx, credHash, raw, int(o.cfg.ExpandTTL.Seconds()))
		return outcome{Text: string(raw)}, nil
	}

	handle, err := o.store.Put(ctx, credHash, raw, int(o.cfg.ExpandTTL.Seconds()))
	if err != nil {
		// Fail open: return full raw if caching is unavailable.
		o.metrics.RecordFailOpen("store_error")
		return outcome{Text: string(raw)}, nil
	}

	total := countItems(raw)
	text := string(res.View) + Trailer(handle, total, total)
	return outcome{Text: text, Compacted: res.Applied, Handle: handle}, nil
}

// countItems returns the number of top-level array elements, or 1 for non-arrays.
// Used to populate the trailer's item count hint for the model.
func countItems(raw []byte) int {
	var arr []any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return len(arr)
	}
	return 1
}
