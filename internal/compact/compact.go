package compact

// Options selects the compaction behavior for a single payload.
type Options struct {
	Mode string // "tabular" (default when empty) or "none"
}

// Result is the outcome of a compaction attempt.
type Result struct {
	View          []byte // the compact (or original) payload to return to the model
	Applied       bool   // whether a transform actually changed the payload
	OriginalBytes int
	CompactBytes  int
}

// Compact applies the selected lossless transform to raw. Unknown or "none"
// modes pass the payload through unchanged. Compaction never errors fatally:
// on any internal failure it falls back to the original payload.
func Compact(raw []byte, opts Options) Result {
	res := Result{View: raw, OriginalBytes: len(raw), CompactBytes: len(raw)}
	mode := opts.Mode
	if mode == "" {
		mode = "tabular"
	}
	switch mode {
	case "tabular":
		out, applied, err := Tabular(raw)
		if err != nil || !applied {
			return res
		}
		// Never emit a payload larger than the original: for small inputs the
		// table wrapper overhead can exceed the savings, so keep the original.
		if len(out) >= len(raw) {
			return res
		}
		res.View = out
		res.Applied = true
		res.CompactBytes = len(out)
	case "none":
		// pass through
	}
	return res
}
