# leanmcp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a transparent MCP-to-MCP proxy that losslessly compacts heavy tool-call results to cut LLM token usage, with expand-on-demand backed by a short-TTL cache.

**Architecture:** A Go service that is an MCP server (Streamable HTTP) to the client and an MCP client to one upstream MCP server. It intercepts `tools/list` and `tools/call` via server receiving-middleware: small results pass through, heavy results are compacted (lossless tabular form) and the full raw result is cached in Redis behind an identity-scoped handle. An injected `expand_result` tool retrieves the full payload or a sub-path on demand. Any internal failure falls open to the full result.

**Tech Stack:** Go 1.26, `github.com/modelcontextprotocol/go-sdk` v1.5.0 (server + client), `github.com/redis/go-redis/v9`, `gopkg.in/yaml.v3`, Prometheus client, standard `testing`.

**Module path:** `github.com/mayur-tolexo/leanmcp`

**Phasing:** Phases are ordered by dependency, not time. Each phase ends with a green build and tests. No time estimates.

---

## SDK reference (verified against go-sdk v1.5.0)

Use these exact signatures; they were confirmed in the vendored SDK.

```go
// Server
func NewServer(impl *Implementation, options *ServerOptions) *Server
func AddTool[In, Out any](s *Server, t *Tool, h ToolHandlerFor[In, Out]) // generic, typed
type ToolHandlerFor[In, Out any] func(context.Context, *CallToolRequest, In) (*CallToolResult, Out, error)
func (s *Server) AddReceivingMiddleware(middleware ...Middleware)
type Middleware func(MethodHandler) MethodHandler
type MethodHandler func(ctx context.Context, method string, req Request) (result Result, err error)
func NewStreamableHTTPHandler(getServer func(*http.Request) *Server, opts *StreamableHTTPOptions) *StreamableHTTPHandler
// StreamableHTTPOptions{ Stateless bool; JSONResponse bool; ... }

// Client
func NewClient(impl *Implementation, options *ClientOptions) *Client
func (c *Client) Connect(ctx context.Context, t Transport, opts *ClientSessionOptions) (*ClientSession, error)
type StreamableClientTransport struct { Endpoint string; HTTPClient *http.Client; /* ... */ }
func (cs *ClientSession) ListTools(ctx context.Context, params *ListToolsParams) (*ListToolsResult, error)
func (cs *ClientSession) CallTool(ctx context.Context, params *CallToolParams) (*CallToolResult, error)

// Types
type Implementation struct { Name, Title, Version string; /* ... */ }
type Tool struct { Name, Description, Title string; InputSchema any; OutputSchema any; /* ... */ }
type ListToolsResult struct { NextCursor string; Tools []*Tool; /* ... */ }
type CallToolParams struct { Name string; Arguments any; /* ... */ }   // client-side call
type CallToolResult struct { Content []Content; StructuredContent any; IsError bool; /* ... */ }
type TextContent struct { Text string; /* ... */ }                      // implements Content
// CallToolRequest = ServerRequest[*CallToolParamsRaw]; req.Params has Name + Arguments(raw); req.Extra has Header.
```

Header-forwarding client transport (from the SDK's `examples/server/proxy`):

```go
type headerForwardingTransport struct{}
func (h *headerForwardingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    newReq := req.Clone(req.Context())
    if headers, ok := req.Context().Value(headerContextKey).(http.Header); ok {
        for k, vals := range headers {
            for _, v := range vals { newReq.Header.Add(k, v) }
        }
    }
    return http.DefaultTransport.RoundTrip(newReq)
}
```

---

## File structure

```
cmd/leanmcp/main.go              bootstrap: config -> deps -> server -> HTTP
internal/config/config.go        env + YAML config load + validation
internal/tokens/estimate.go      cheap token estimate + threshold check
internal/jsonpath/jsonpath.go    dot-path get + field projection (for expand)
internal/compact/compact.go      Compactor: dispatch by mode, Result type
internal/compact/tabular.go      lossless tabular transform
internal/store/store.go          Store interface + Entry + handle keying
internal/store/memory.go         in-memory Store (tests / single-node)
internal/store/redis.go          Redis Store
internal/identity/identity.go    Verifier interface + Identity
internal/identity/static.go      static/dev verifier
internal/identity/http.go        HTTP verify-endpoint verifier
internal/upstream/transport.go   header-forwarding RoundTripper + ctx plumbing
internal/upstream/client.go      upstream MCP client wrapper (ListTools/CallTool)
internal/proxy/server.go         build *mcp.Server + receiving middleware
internal/proxy/toolslist.go      merge upstream list + expand_result + local pagination
internal/proxy/toolscall.go      forward, classify, compact, cache, trailer, fail-open
internal/proxy/expand.go         expand_result handler
internal/proxy/trailer.go        compact-result trailer formatting
internal/metrics/metrics.go      Prometheus metrics
```

---

# Phase 0 — Project scaffold

**Goal:** A buildable module with layout, a health endpoint, and CI. Build and tests green.

### Task 0.1: Initialize the Go module

**Files:**
- Create: `go.mod`

- [ ] **Step 1: Create the module**

Run:
```bash
cd ~/Documents/projects/leanmcp
go mod init github.com/mayur-tolexo/leanmcp
go mod edit -go=1.26
```

- [ ] **Step 2: Verify**

Run: `go build ./... 2>&1 || true`
Expected: no error (nothing to build yet).

- [ ] **Step 3: Commit**

```bash
git add go.mod
git commit -m "chore: initialize go module"
```

### Task 0.2: Health-check HTTP server skeleton

**Files:**
- Create: `cmd/leanmcp/main.go`
- Test: `cmd/leanmcp/main_test.go`

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("got %q, want %q", rec.Body.String(), "ok")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/leanmcp/ -run TestHealthHandler -v`
Expected: FAIL — `undefined: healthHandler`.

- [ ] **Step 3: Minimal implementation**

```go
package main

import (
	"log"
	"net/http"
)

// healthHandler returns a handler that reports liveness with a 200 "ok".
func healthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// main wires the HTTP mux and starts the server. Real wiring is added in later phases.
func main() {
	mux := http.NewServeMux()
	mux.Handle("/healthz", healthHandler())
	mux.Handle("/readyz", healthHandler())
	log.Println("leanmcp listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/leanmcp/ -run TestHealthHandler -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/leanmcp/main.go cmd/leanmcp/main_test.go
git commit -m "feat: add health-check server skeleton"
```

### Task 0.3: CI workflow and Makefile

**Files:**
- Create: `.github/workflows/ci.yml`
- Create: `Makefile`

- [ ] **Step 1: Add the Makefile**

```makefile
.PHONY: build test vet fmt check
build:
	go build ./...
test:
	go test ./...
vet:
	go vet ./...
fmt:
	gofmt -l .
check: fmt vet build test
```

- [ ] **Step 2: Add the CI workflow**

```yaml
name: CI
on:
  push: { branches: [main] }
  pull_request:
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.26" }
      - run: go build ./...
      - run: go vet ./...
      - run: test -z "$(gofmt -l .)"
      - run: go test ./... -race -count=1
```

- [ ] **Step 3: Verify locally**

Run: `make check`
Expected: all green, no gofmt output.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml Makefile
git commit -m "ci: add build/test workflow and Makefile"
```

---

# Phase 1 — Configuration

**Goal:** Load and validate config from environment + an optional YAML file.

### Task 1.1: Config struct and loader

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("LEANMCP_UPSTREAM_MCP_URL", "http://upstream/mcp")
	t.Setenv("LEANMCP_REDIS_URL", "redis://localhost:6379")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UpstreamMCPURL != "http://upstream/mcp" {
		t.Fatalf("upstream = %q", cfg.UpstreamMCPURL)
	}
	if cfg.OptimizeThresholdTokens != 1500 {
		t.Fatalf("default threshold = %d, want 1500", cfg.OptimizeThresholdTokens)
	}
	if cfg.ExpandTTL != 10*time.Minute {
		t.Fatalf("default ttl = %v, want 10m", cfg.ExpandTTL)
	}
	if !cfg.ShadowMode {
		t.Fatalf("shadow_mode default should be true")
	}
}

func TestLoadMissingUpstreamFails(t *testing.T) {
	os.Clearenv()
	if _, err := Load(""); err == nil {
		t.Fatalf("expected error when upstream URL missing")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 3: Implement config**

```go
// Package config loads leanmcp configuration from a YAML file (optional) and
// environment variables (which override the file), then validates it.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ToolRule is the optional per-tool optimization configuration.
type ToolRule struct {
	Compaction string   `yaml:"compaction"` // "tabular" | "none"
	ArrayPath  string   `yaml:"array_path"` // dot-path to the dominant array
	MaxItems   int      `yaml:"max_items"`  // 0 = no cap
	KeepFields []string `yaml:"keep_fields"` // opt-in lossy projection; off by default
}

// Config is the full runtime configuration.
type Config struct {
	UpstreamMCPURL          string              `yaml:"upstream_mcp_url"`
	RedisURL                string              `yaml:"redis_url"`
	VerifyEndpoint          string              `yaml:"verify_endpoint"` // empty => static/dev verifier
	OptimizeThresholdTokens int                 `yaml:"optimize_threshold_tokens"`
	ExpandTTL               time.Duration       `yaml:"expand_ttl"`
	MaxRawBytes             int                 `yaml:"max_raw_bytes"`
	ShadowMode              bool                `yaml:"shadow_mode"`
	ListenAddr              string              `yaml:"listen_addr"`
	Tools                   map[string]ToolRule `yaml:"tools"`
}

// Load reads optional YAML at path (if non-empty), applies defaults, overlays
// environment variables, and validates the result.
func Load(path string) (*Config, error) {
	cfg := &Config{
		OptimizeThresholdTokens: 1500,
		ExpandTTL:               10 * time.Minute,
		MaxRawBytes:             5_000_000,
		ShadowMode:              true,
		ListenAddr:              ":8080",
		Tools:                   map[string]ToolRule{},
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	overlayEnv(cfg)
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// overlayEnv applies LEANMCP_* environment variables over the config.
func overlayEnv(cfg *Config) {
	if v := os.Getenv("LEANMCP_UPSTREAM_MCP_URL"); v != "" {
		cfg.UpstreamMCPURL = v
	}
	if v := os.Getenv("LEANMCP_REDIS_URL"); v != "" {
		cfg.RedisURL = v
	}
	if v := os.Getenv("LEANMCP_VERIFY_ENDPOINT"); v != "" {
		cfg.VerifyEndpoint = v
	}
	if v := os.Getenv("LEANMCP_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("LEANMCP_SHADOW_MODE"); v == "false" {
		cfg.ShadowMode = false
	}
}

// validate enforces required fields.
func (c *Config) validate() error {
	if c.UpstreamMCPURL == "" {
		return fmt.Errorf("upstream_mcp_url (LEANMCP_UPSTREAM_MCP_URL) is required")
	}
	if c.OptimizeThresholdTokens < 0 {
		return fmt.Errorf("optimize_threshold_tokens must be >= 0")
	}
	return nil
}
```

- [ ] **Step 4: Add dependency and run**

Run:
```bash
go get gopkg.in/yaml.v3
go test ./internal/config/ -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: add configuration loader with env overrides"
```

---

# Phase 2 — Compaction engine (pure logic)

**Goal:** Token estimation, lossless tabular compaction, and dot-path extraction. All pure functions, fully unit-tested.

### Task 2.1: Token estimator and threshold

**Files:**
- Create: `internal/tokens/estimate.go`
- Test: `internal/tokens/estimate_test.go`

- [ ] **Step 1: Write the failing test**

```go
package tokens

import "testing"

func TestEstimate(t *testing.T) {
	// ~4 chars/token heuristic.
	if got := Estimate([]byte("aaaaaaaa")); got != 2 { // 8/4
		t.Fatalf("Estimate = %d, want 2", got)
	}
	if got := Estimate([]byte("")); got != 0 {
		t.Fatalf("Estimate empty = %d, want 0", got)
	}
}

func TestExceeds(t *testing.T) {
	if !Exceeds([]byte("aaaaaaaaaaaaaaaa"), 3) { // 16/4 = 4 > 3
		t.Fatalf("expected exceeds")
	}
	if Exceeds([]byte("aaaa"), 3) { // 1 < 3
		t.Fatalf("did not expect exceeds")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tokens/ -v`
Expected: FAIL — `undefined: Estimate`.

- [ ] **Step 3: Implement**

```go
// Package tokens provides a cheap, dependency-free estimate of how many tokens a
// byte payload will cost a model, used only to decide whether to optimize.
package tokens

// charsPerToken is the rough divisor used for the heuristic estimate.
const charsPerToken = 4

// Estimate returns an approximate token count for b using a chars/token heuristic.
func Estimate(b []byte) int {
	return len(b) / charsPerToken
}

// Exceeds reports whether b's estimated token count is strictly greater than threshold.
func Exceeds(b []byte, threshold int) bool {
	return Estimate(b) > threshold
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/tokens/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tokens/
git commit -m "feat: add token estimation heuristic"
```

### Task 2.2: Dot-path extraction and field projection

**Files:**
- Create: `internal/jsonpath/jsonpath.go`
- Test: `internal/jsonpath/jsonpath_test.go`

- [ ] **Step 1: Write the failing test**

```go
package jsonpath

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	return v
}

func TestGetObjectField(t *testing.T) {
	v := decode(t, `{"data":{"price":{"monthly":9}}}`)
	got, err := Get(v, "data.price.monthly")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != float64(9) {
		t.Fatalf("got %v, want 9", got)
	}
}

func TestGetArrayIndex(t *testing.T) {
	v := decode(t, `{"data":[{"id":"a"},{"id":"b"}]}`)
	got, err := Get(v, "data[1].id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "b" {
		t.Fatalf("got %v, want b", got)
	}
}

func TestGetMissing(t *testing.T) {
	v := decode(t, `{"a":1}`)
	if _, err := Get(v, "a.b"); err == nil {
		t.Fatalf("expected error for missing path")
	}
}

func TestProject(t *testing.T) {
	v := decode(t, `{"id":"x","name":"n","secret":"s"}`)
	got := Project(v, []string{"id", "name"})
	want := map[string]any{"id": "x", "name": "n"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/jsonpath/ -v`
Expected: FAIL — `undefined: Get`.

- [ ] **Step 3: Implement**

```go
// Package jsonpath provides minimal dot-path access and field projection over
// JSON values decoded into any (map[string]any / []any / scalars). It supports
// segments like "data" and "data[3].field"; no wildcards or filters.
package jsonpath

import (
	"fmt"
	"strconv"
	"strings"
)

// Get returns the value at the dot-path within v, or an error if any segment is
// missing or the path shape does not match the data.
func Get(v any, path string) (any, error) {
	if path == "" || path == "." {
		return v, nil
	}
	cur := v
	for _, seg := range strings.Split(path, ".") {
		name, indices := parseSegment(seg)
		if name != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("path %q: %q is not an object", path, name)
			}
			child, ok := m[name]
			if !ok {
				return nil, fmt.Errorf("path %q: key %q not found", path, name)
			}
			cur = child
		}
		for _, idx := range indices {
			arr, ok := cur.([]any)
			if !ok {
				return nil, fmt.Errorf("path %q: not an array at index %d", path, idx)
			}
			if idx < 0 || idx >= len(arr) {
				return nil, fmt.Errorf("path %q: index %d out of range", path, idx)
			}
			cur = arr[idx]
		}
	}
	return cur, nil
}

// Project returns a new map containing only the named top-level dot-paths that
// exist in v; missing paths are skipped. Used for opt-in field whitelisting.
func Project(v any, fields []string) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if got, err := Get(v, f); err == nil {
			out[f] = got
		}
	}
	return out
}

// parseSegment splits a segment like "data[1][2]" into its name and indices.
func parseSegment(seg string) (string, []int) {
	name := seg
	var indices []int
	if i := strings.IndexByte(seg, '['); i >= 0 {
		name = seg[:i]
		for _, part := range strings.Split(seg[i:], "[") {
			part = strings.TrimSuffix(strings.TrimSpace(part), "]")
			if part == "" {
				continue
			}
			if n, err := strconv.Atoi(part); err == nil {
				indices = append(indices, n)
			}
		}
	}
	return name, indices
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/jsonpath/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jsonpath/
git commit -m "feat: add json dot-path access and field projection"
```

### Task 2.3: Lossless tabular compaction

**Files:**
- Create: `internal/compact/tabular.go`
- Test: `internal/compact/tabular_test.go`

The tabular transform turns an array of uniform objects into `{"__leanmcp_table":{"columns":[...],"rows":[[...]]}}`, eliminating repeated keys. It is lossless: any value can be reconstructed from columns+rows.

- [ ] **Step 1: Write the failing test**

```go
package compact

import (
	"encoding/json"
	"testing"
)

func TestTabularArrayOfObjects(t *testing.T) {
	in := []byte(`[{"id":"1","name":"a"},{"id":"2","name":"b"}]`)
	out, applied, err := Tabular(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !applied {
		t.Fatalf("expected tabular to apply")
	}
	if len(out) >= len(in) {
		t.Fatalf("expected compaction to shrink payload: %d >= %d", len(out), len(in))
	}
	var wrap struct {
		Table struct {
			Columns []string `json:"columns"`
			Rows    [][]any  `json:"rows"`
		} `json:"__leanmcp_table"`
	}
	if err := json.Unmarshal(out, &wrap); err != nil {
		t.Fatalf("output not valid table json: %v", err)
	}
	if len(wrap.Table.Columns) != 2 || len(wrap.Table.Rows) != 2 {
		t.Fatalf("unexpected shape: %+v", wrap.Table)
	}
}

func TestTabularNotApplicable(t *testing.T) {
	// A single object is not an array of uniform objects.
	if _, applied, _ := Tabular([]byte(`{"id":"1"}`)); applied {
		t.Fatalf("did not expect tabular to apply to a single object")
	}
	// An array of scalars is not uniform objects.
	if _, applied, _ := Tabular([]byte(`[1,2,3]`)); applied {
		t.Fatalf("did not expect tabular to apply to scalar array")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/compact/ -run TestTabular -v`
Expected: FAIL — `undefined: Tabular`.

- [ ] **Step 3: Implement**

```go
// Package compact provides lossless transforms that reduce the token cost of a
// JSON payload without dropping data.
package compact

import (
	"encoding/json"
	"sort"
)

// table is the lossless wire form for an array of uniform objects.
type table struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Tabular attempts to rewrite a JSON array of objects as a single column header
// plus rows, removing the per-row repetition of keys. It returns the rewritten
// bytes and applied=true on success; if the payload is not an array of objects
// it returns applied=false and the input unchanged.
func Tabular(raw []byte) ([]byte, bool, error) {
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
		return raw, false, nil
	}
	cols := unionColumns(arr)
	rows := make([][]any, 0, len(arr))
	for _, obj := range arr {
		row := make([]any, len(cols))
		for i, c := range cols {
			row[i] = obj[c] // missing key => nil; lossless against JSON null/absent
		}
		rows = append(rows, row)
	}
	out, err := json.Marshal(map[string]any{
		"__leanmcp_table": table{Columns: cols, Rows: rows},
	})
	if err != nil {
		return raw, false, err
	}
	return out, true, nil
}

// unionColumns returns the sorted union of keys across all objects so the table
// represents heterogeneous-but-similar objects without losing any field.
func unionColumns(arr []map[string]any) []string {
	seen := map[string]struct{}{}
	for _, obj := range arr {
		for k := range obj {
			seen[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/compact/ -run TestTabular -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/compact/tabular.go internal/compact/tabular_test.go
git commit -m "feat: add lossless tabular compaction"
```

### Task 2.4: Compactor dispatch and Result

**Files:**
- Create: `internal/compact/compact.go`
- Test: `internal/compact/compact_test.go`

- [ ] **Step 1: Write the failing test**

```go
package compact

import "testing"

func TestCompactTabularMode(t *testing.T) {
	in := []byte(`[{"id":"1","name":"a"},{"id":"2","name":"b"}]`)
	res := Compact(in, Options{Mode: "tabular"})
	if !res.Applied {
		t.Fatalf("expected applied")
	}
	if res.OriginalBytes != len(in) {
		t.Fatalf("OriginalBytes = %d, want %d", res.OriginalBytes, len(in))
	}
	if res.CompactBytes >= res.OriginalBytes {
		t.Fatalf("expected smaller output")
	}
}

func TestCompactNoneMode(t *testing.T) {
	in := []byte(`[{"id":"1"}]`)
	res := Compact(in, Options{Mode: "none"})
	if res.Applied {
		t.Fatalf("none mode must not transform")
	}
	if string(res.View) != string(in) {
		t.Fatalf("none mode must pass through unchanged")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/compact/ -run TestCompact -v`
Expected: FAIL — `undefined: Compact`.

- [ ] **Step 3: Implement**

```go
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
		res.View = out
		res.Applied = true
		res.CompactBytes = len(out)
	case "none":
		// pass through
	}
	return res
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/compact/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/compact/compact.go internal/compact/compact_test.go
git commit -m "feat: add compaction dispatch and Result type"
```

---

# Phase 3 — Reference store

**Goal:** A handle store that caches raw results, namespaced by identity, with TTL and a size cap. In-memory and Redis implementations behind one interface.

### Task 3.1: Store interface, Entry, and handle keying

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import "testing"

func TestHandleKeyNamespacing(t *testing.T) {
	a := HandleKey("org1", "userA", "nonce1")
	b := HandleKey("org1", "userB", "nonce1")
	if a == b {
		t.Fatalf("keys for different users must differ: %q == %q", a, b)
	}
	if got := HandleKey("o", "u", "n"); got != "o:u:n" {
		t.Fatalf("HandleKey = %q, want o:u:n", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/store/ -run TestHandleKey -v`
Expected: FAIL — `undefined: HandleKey`.

- [ ] **Step 3: Implement**

```go
// Package store caches full raw tool results behind identity-scoped handles so
// the model can expand them on demand. Entries expire via TTL.
package store

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a handle is missing or expired.
var ErrNotFound = errors.New("handle not found")

// Entry is a cached raw result plus the identity that produced it.
type Entry struct {
	Org       string `json:"org"`
	User      string `json:"user"`
	Raw       []byte `json:"raw"`
	CreatedAt int64  `json:"created_at"` // unix seconds
}

// Store persists Entry values under a handle with a TTL.
type Store interface {
	// Put stores raw under a freshly generated, identity-scoped handle and
	// returns the handle. ttlSeconds bounds the entry lifetime.
	Put(ctx context.Context, org, user string, raw []byte, ttlSeconds int) (string, error)
	// Get returns the entry for handle, or ErrNotFound.
	Get(ctx context.Context, handle string) (*Entry, error)
}

// HandleKey builds the namespaced storage key for an entry.
func HandleKey(org, user, nonce string) string {
	return org + ":" + user + ":" + nonce
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/store/ -run TestHandleKey -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: add store interface, entry, and handle keying"
```

### Task 3.2: In-memory store

**Files:**
- Create: `internal/store/memory.go`
- Test: `internal/store/memory_test.go`

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"context"
	"testing"
)

func TestMemoryPutGet(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	h, err := s.Put(ctx, "org1", "userA", []byte(`{"x":1}`), 60)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	e, err := s.Get(ctx, h)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(e.Raw) != `{"x":1}` || e.Org != "org1" || e.User != "userA" {
		t.Fatalf("bad entry: %+v", e)
	}
}

func TestMemoryGetMissing(t *testing.T) {
	if _, err := NewMemory().Get(context.Background(), "nope"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMemoryHandlesAreScoped(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	h, _ := s.Put(ctx, "org1", "userA", []byte(`1`), 60)
	e, _ := s.Get(ctx, h)
	if e.Org != "org1" || e.User != "userA" {
		t.Fatalf("identity not preserved on handle")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/store/ -run TestMemory -v`
Expected: FAIL — `undefined: NewMemory`.

- [ ] **Step 3: Implement**

```go
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// memory is an in-process Store used in tests and single-node deployments.
type memory struct {
	mu      sync.Mutex
	entries map[string]memoryItem
}

// memoryItem wraps an Entry with its absolute expiry.
type memoryItem struct {
	entry     Entry
	expiresAt time.Time
}

// NewMemory returns an in-memory Store.
func NewMemory() Store {
	return &memory{entries: map[string]memoryItem{}}
}

// Put stores raw under a random nonce, namespaced by identity.
func (m *memory) Put(_ context.Context, org, user string, raw []byte, ttlSeconds int) (string, error) {
	nonce, err := randomNonce()
	if err != nil {
		return "", err
	}
	key := HandleKey(org, user, nonce)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = memoryItem{
		entry:     Entry{Org: org, User: user, Raw: raw, CreatedAt: time.Now().Unix()},
		expiresAt: time.Now().Add(time.Duration(ttlSeconds) * time.Second),
	}
	return key, nil
}

// Get returns the entry for handle if present and unexpired.
func (m *memory) Get(_ context.Context, handle string) (*Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.entries[handle]
	if !ok || time.Now().After(item.expiresAt) {
		delete(m.entries, handle)
		return nil, ErrNotFound
	}
	e := item.entry
	return &e, nil
}

// randomNonce returns a 16-byte hex nonce.
func randomNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/store/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/memory.go internal/store/memory_test.go
git commit -m "feat: add in-memory store"
```

### Task 3.3: Redis store

**Files:**
- Create: `internal/store/redis.go`
- Test: `internal/store/redis_test.go`

This test uses a Redis instance only when `LEANMCP_TEST_REDIS_URL` is set; otherwise it skips, so CI stays green without Redis.

- [ ] **Step 1: Write the failing test**

```go
package store

import (
	"context"
	"os"
	"testing"
)

func TestRedisPutGet(t *testing.T) {
	url := os.Getenv("LEANMCP_TEST_REDIS_URL")
	if url == "" {
		t.Skip("set LEANMCP_TEST_REDIS_URL to run redis store tests")
	}
	s, err := NewRedis(url)
	if err != nil {
		t.Fatalf("new redis: %v", err)
	}
	ctx := context.Background()
	h, err := s.Put(ctx, "org1", "userA", []byte(`{"x":1}`), 60)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	e, err := s.Get(ctx, h)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(e.Raw) != `{"x":1}` {
		t.Fatalf("bad raw: %s", e.Raw)
	}
	if _, err := s.Get(ctx, "missing:missing:missing"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails/skips**

Run: `go test ./internal/store/ -run TestRedis -v`
Expected: FAIL — `undefined: NewRedis` (or SKIP once compiled but Redis unset).

- [ ] **Step 3: Implement**

```go
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisStore persists entries in Redis with native key TTL.
type redisStore struct {
	rdb *redis.Client
}

// NewRedis returns a Redis-backed Store from a redis:// URL.
func NewRedis(url string) (Store, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &redisStore{rdb: redis.NewClient(opt)}, nil
}

// Put marshals the entry and stores it under an identity-scoped key with TTL.
func (s *redisStore) Put(ctx context.Context, org, user string, raw []byte, ttlSeconds int) (string, error) {
	nonce, err := randomNonce()
	if err != nil {
		return "", err
	}
	key := HandleKey(org, user, nonce)
	e := Entry{Org: org, User: user, Raw: raw, CreatedAt: time.Now().Unix()}
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, key, b, time.Duration(ttlSeconds)*time.Second).Err(); err != nil {
		return "", err
	}
	return key, nil
}

// Get loads and unmarshals the entry, mapping a cache miss to ErrNotFound.
func (s *redisStore) Get(ctx context.Context, handle string) (*Entry, error) {
	b, err := s.rdb.Get(ctx, handle).Bytes()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
```

- [ ] **Step 4: Add dependency and run**

Run:
```bash
go get github.com/redis/go-redis/v9
go test ./internal/store/ -v
```
Expected: PASS (Redis test SKIPs without `LEANMCP_TEST_REDIS_URL`).

- [ ] **Step 5: Commit**

```bash
git add internal/store/redis.go internal/store/redis_test.go go.mod go.sum
git commit -m "feat: add redis store"
```

---

# Phase 4 — Identity verifier

**Goal:** Derive a trustworthy `(org, user)` from the caller's credential, used only to namespace handles. Pluggable: a static dev verifier and an HTTP verify-endpoint verifier.

### Task 4.1: Verifier interface and static verifier

**Files:**
- Create: `internal/identity/identity.go`
- Create: `internal/identity/static.go`
- Test: `internal/identity/static_test.go`

- [ ] **Step 1: Write the failing test**

```go
package identity

import (
	"context"
	"testing"
)

func TestStaticVerifier(t *testing.T) {
	v := NewStatic("org-dev", "user-dev")
	id, err := v.Verify(context.Background(), "any-token")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if id.Org != "org-dev" || id.User != "user-dev" {
		t.Fatalf("bad identity: %+v", id)
	}
}

func TestStaticVerifierRejectsEmptyToken(t *testing.T) {
	if _, err := NewStatic("o", "u").Verify(context.Background(), ""); err == nil {
		t.Fatalf("expected error on empty credential")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/identity/ -v`
Expected: FAIL — `undefined: NewStatic`.

- [ ] **Step 3: Implement**

`internal/identity/identity.go`:
```go
// Package identity verifies a caller's credential and returns a trustworthy
// identity used solely to namespace cache handles. It makes no authorization
// decisions; the upstream MCP server remains the authority.
package identity

import (
	"context"
	"errors"
)

// ErrUnauthorized indicates the credential is missing or invalid.
var ErrUnauthorized = errors.New("unauthorized")

// Identity is the verified caller, used for handle namespacing.
type Identity struct {
	Org  string
	User string
}

// Verifier turns a raw credential into an Identity.
type Verifier interface {
	Verify(ctx context.Context, credential string) (Identity, error)
}
```

`internal/identity/static.go`:
```go
package identity

import "context"

// static returns a fixed identity for any non-empty credential. For local
// development only; never use in multi-tenant deployments.
type static struct {
	org, user string
}

// NewStatic returns a Verifier that maps any non-empty credential to (org, user).
func NewStatic(org, user string) Verifier {
	return &static{org: org, user: user}
}

// Verify rejects empty credentials and otherwise returns the fixed identity.
func (s *static) Verify(_ context.Context, credential string) (Identity, error) {
	if credential == "" {
		return Identity{}, ErrUnauthorized
	}
	return Identity{Org: s.org, User: s.user}, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/identity/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/identity/identity.go internal/identity/static.go internal/identity/static_test.go
git commit -m "feat: add identity verifier interface and static verifier"
```

### Task 4.2: HTTP verify-endpoint verifier

**Files:**
- Create: `internal/identity/http.go`
- Test: `internal/identity/http_test.go`

The HTTP verifier POSTs the credential to a configurable endpoint that returns `{"org":"...","user":"..."}` on 200, anything else is unauthorized.

- [ ] **Step 1: Write the failing test**

```go
package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPVerifierSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"org":"o1","user":"u1"}`))
	}))
	defer srv.Close()

	v := NewHTTP(srv.URL)
	id, err := v.Verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if id.Org != "o1" || id.User != "u1" {
		t.Fatalf("bad identity: %+v", id)
	}
}

func TestHTTPVerifierUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := NewHTTP(srv.URL).Verify(context.Background(), "bad"); err != ErrUnauthorized {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/identity/ -run TestHTTP -v`
Expected: FAIL — `undefined: NewHTTP`.

- [ ] **Step 3: Implement**

```go
package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// httpVerifier verifies credentials against an external HTTP endpoint.
type httpVerifier struct {
	endpoint string
	client   *http.Client
}

// NewHTTP returns a Verifier that POSTs the credential (as a Bearer token) to
// endpoint and reads {"org","user"} from a 200 response.
func NewHTTP(endpoint string) Verifier {
	return &httpVerifier{endpoint: endpoint, client: &http.Client{Timeout: 5 * time.Second}}
}

// Verify calls the endpoint and maps non-200 responses to ErrUnauthorized.
func (h *httpVerifier) Verify(ctx context.Context, credential string) (Identity, error) {
	if credential == "" {
		return Identity{}, ErrUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	resp, err := h.client.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Identity{}, ErrUnauthorized
	}
	var out struct {
		Org  string `json:"org"`
		User string `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Identity{}, err
	}
	return Identity{Org: out.Org, User: out.User}, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/identity/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/identity/http.go internal/identity/http_test.go
git commit -m "feat: add http verify-endpoint identity verifier"
```

---

# Phase 5 — Upstream MCP client wrapper

**Goal:** Connect to the upstream MCP server as a client and forward the caller's credential per request.

### Task 5.1: Header-forwarding transport and context plumbing

**Files:**
- Create: `internal/upstream/transport.go`
- Test: `internal/upstream/transport_test.go`

- [ ] **Step 1: Write the failing test**

```go
package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestForwardingTransportInjectsHeaders(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	client := NewForwardingClient()
	ctx := WithHeaders(context.Background(), http.Header{"Authorization": []string{"Bearer abc"}})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if _, err := client.Do(req); err != nil {
		t.Fatalf("do: %v", err)
	}
	if got != "Bearer abc" {
		t.Fatalf("forwarded auth = %q, want %q", got, "Bearer abc")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/upstream/ -v`
Expected: FAIL — `undefined: NewForwardingClient`.

- [ ] **Step 3: Implement**

```go
// Package upstream connects to the upstream MCP server as a client and forwards
// the caller's credential on each outgoing request.
package upstream

import (
	"context"
	"net/http"
)

// ctxKey is the private context key type for forwarded headers.
type ctxKey struct{}

// headerContextKey carries the http.Header to forward to the upstream.
var headerContextKey = ctxKey{}

// WithHeaders returns a context carrying headers to forward upstream.
func WithHeaders(ctx context.Context, h http.Header) context.Context {
	return context.WithValue(ctx, headerContextKey, h)
}

// forwardingTransport injects context-carried headers onto each request.
type forwardingTransport struct{}

// RoundTrip clones the request and adds any headers stored on its context.
func (forwardingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq := req.Clone(req.Context())
	if headers, ok := req.Context().Value(headerContextKey).(http.Header); ok {
		for k, vals := range headers {
			for _, v := range vals {
				newReq.Header.Add(k, v)
			}
		}
	}
	return http.DefaultTransport.RoundTrip(newReq)
}

// NewForwardingClient returns an *http.Client that forwards context headers.
func NewForwardingClient() *http.Client {
	return &http.Client{Transport: forwardingTransport{}}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/upstream/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/upstream/transport.go internal/upstream/transport_test.go
git commit -m "feat: add header-forwarding transport for upstream calls"
```

### Task 5.2: Upstream client wrapper (integration test against a fake MCP server)

**Files:**
- Create: `internal/upstream/client.go`
- Test: `internal/upstream/client_test.go`

This test stands up a real in-process MCP server with the go-sdk and verifies the wrapper can list and call tools through Streamable HTTP.

- [ ] **Step 1: Write the failing test**

```go
package upstream

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeUpstream starts a minimal MCP server exposing one echo tool.
func fakeUpstream(t *testing.T) string {
	t.Helper()
	s := mcp.NewServer(&mcp.Implementation{Name: "fake", Version: "0.0.1"}, &mcp.ServerOptions{HasTools: true})
	type in struct {
		Msg string `json:"msg"`
	}
	mcp.AddTool(s, &mcp.Tool{Name: "echo", Description: "echoes msg"},
		func(_ context.Context, _ *mcp.CallToolRequest, args in) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: args.Msg}}}, nil, nil
		})
	h := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestClientListAndCall(t *testing.T) {
	url := fakeUpstream(t)
	c := New(url)
	ctx := context.Background()

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools)
	}

	res, err := c.CallTool(ctx, "echo", map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	txt, ok := res.Content[0].(*mcp.TextContent)
	if !ok || txt.Text != "hi" {
		t.Fatalf("unexpected result: %+v", res.Content)
	}
}
```

> Note: add `import "net/http"` to the test file (used by `fakeUpstream`).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/upstream/ -run TestClientListAndCall -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Implement**

```go
package upstream

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client wraps the go-sdk MCP client for a single upstream endpoint, opening a
// short-lived session per call so the caller's per-request credential (carried
// on the context) is forwarded by the forwarding transport.
type Client struct {
	endpoint string
	http     *mcp.Client
}

// New returns a Client for the upstream Streamable HTTP /mcp endpoint.
func New(endpoint string) *Client {
	return &Client{
		endpoint: endpoint,
		http:     mcp.NewClient(&mcp.Implementation{Name: "leanmcp", Version: "0.0.1"}, nil),
	}
}

// connect opens a session using the forwarding HTTP client so context headers
// reach the upstream.
func (c *Client) connect(ctx context.Context) (*mcp.ClientSession, error) {
	return c.http.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   c.endpoint,
		HTTPClient: NewForwardingClient(),
	}, nil)
}

// ListTools returns the upstream's full tool list (single page).
func (c *Client) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	sess, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	res, err := sess.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// CallTool invokes a named upstream tool with the given arguments.
func (c *Client) CallTool(ctx context.Context, name string, args any) (*mcp.CallToolResult, error) {
	sess, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	return sess.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
}
```

- [ ] **Step 4: Add the SDK dependency and run**

Run:
```bash
go get github.com/modelcontextprotocol/go-sdk/mcp@v1.5.0
go test ./internal/upstream/ -v
```
Expected: PASS. If a field/method name mismatches the SDK, the compiler will point to it — confirm against `.../go-sdk@v1.5.0/mcp/` and adjust.

- [ ] **Step 5: Commit**

```bash
git add internal/upstream/client.go internal/upstream/client_test.go go.mod go.sum
git commit -m "feat: add upstream MCP client wrapper"
```

---

# Phase 6 — Proxy assembly

**Goal:** Wire the frontend MCP server with receiving-middleware that handles `tools/list` (merge + local pagination) and `tools/call` (forward, classify, compact, cache, trailer, fail-open), plus the injected `expand_result` tool.

### Task 6.1: Compact-result trailer

**Files:**
- Create: `internal/proxy/trailer.go`
- Test: `internal/proxy/trailer_test.go`

- [ ] **Step 1: Write the failing test**

```go
package proxy

import (
	"strings"
	"testing"
)

func TestTrailerMentionsHandleAndExpand(t *testing.T) {
	tr := Trailer("r_123", 412, 20)
	if !strings.Contains(tr, "r_123") {
		t.Fatalf("trailer missing handle: %q", tr)
	}
	if !strings.Contains(tr, "expand_result") {
		t.Fatalf("trailer missing expand_result hint: %q", tr)
	}
	if !strings.Contains(tr, "412") {
		t.Fatalf("trailer missing total count: %q", tr)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/proxy/ -run TestTrailer -v`
Expected: FAIL — `undefined: Trailer`.

- [ ] **Step 3: Implement**

```go
// Package proxy assembles the MCP proxy server and its tool handlers.
package proxy

import "fmt"

// Trailer returns a short machine-readable note appended to a compacted result
// so the model knows it can retrieve the full payload via expand_result.
func Trailer(handle string, totalItems, shownItems int) string {
	return fmt.Sprintf(
		"\n[leanmcp: compacted losslessly; showing %d of %d items]\n"+
			"[expand: expand_result(handle=%q) for the full result; "+
			"add path=\"data[0]\" for a single record or offset/limit to page]",
		shownItems, totalItems, handle)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/proxy/ -run TestTrailer -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/trailer.go internal/proxy/trailer_test.go
git commit -m "feat: add compact-result trailer"
```

### Task 6.2: tools/list local pagination helper

**Files:**
- Create: `internal/proxy/toolslist.go`
- Test: `internal/proxy/toolslist_test.go`

The proxy fetches the full upstream list, appends `expand_result`, and paginates locally with its own opaque cursor (base64 offset). This avoids corrupting upstream cursors.

- [ ] **Step 1: Write the failing test**

```go
package proxy

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func tools(names ...string) []*mcp.Tool {
	out := make([]*mcp.Tool, len(names))
	for i, n := range names {
		out[i] = &mcp.Tool{Name: n}
	}
	return out
}

func TestPaginateFirstPage(t *testing.T) {
	page, next, err := paginate(tools("a", "b", "c", "d"), "", 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(page) != 2 || page[0].Name != "a" || page[1].Name != "b" {
		t.Fatalf("bad page: %+v", page)
	}
	if next == "" {
		t.Fatalf("expected a next cursor")
	}
}

func TestPaginateSecondPageAndEnd(t *testing.T) {
	_, next, _ := paginate(tools("a", "b", "c", "d"), "", 2)
	page, next2, err := paginate(tools("a", "b", "c", "d"), next, 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(page) != 2 || page[0].Name != "c" {
		t.Fatalf("bad second page: %+v", page)
	}
	if next2 != "" {
		t.Fatalf("expected end of pagination, got %q", next2)
	}
}

func TestPaginateZeroPageSizeReturnsAll(t *testing.T) {
	page, next, _ := paginate(tools("a", "b", "c"), "", 0)
	if len(page) != 3 || next != "" {
		t.Fatalf("zero page size should return all with no cursor")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/proxy/ -run TestPaginate -v`
Expected: FAIL — `undefined: paginate`.

- [ ] **Step 3: Implement**

```go
package proxy

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// paginate returns the slice of tools starting at the offset encoded in cursor,
// limited to pageSize (0 = all), plus the next cursor ("" when exhausted).
func paginate(all []*mcp.Tool, cursor string, pageSize int) ([]*mcp.Tool, string, error) {
	offset := 0
	if cursor != "" {
		b, err := base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		offset, err = strconv.Atoi(string(b))
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
	}
	if offset > len(all) {
		offset = len(all)
	}
	if pageSize <= 0 {
		return all[offset:], "", nil
	}
	end := offset + pageSize
	if end >= len(all) {
		return all[offset:], "", nil
	}
	next := base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(end)))
	return all[offset:end], next, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/proxy/ -run TestPaginate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/toolslist.go internal/proxy/toolslist_test.go
git commit -m "feat: add local tools/list pagination helper"
```

### Task 6.3: expand_result handler logic

**Files:**
- Create: `internal/proxy/expand.go`
- Test: `internal/proxy/expand_test.go`

The handler loads an entry, enforces identity match, applies path/fields/offset/limit, and returns the slice as JSON text.

- [ ] **Step 1: Write the failing test**

```go
package proxy

import (
	"context"
	"strings"
	"testing"

	"github.com/mayur-tolexo/leanmcp/internal/identity"
	"github.com/mayur-tolexo/leanmcp/internal/store"
)

func TestExpandFullAndPath(t *testing.T) {
	s := store.NewMemory()
	ctx := context.Background()
	h, _ := s.Put(ctx, "o1", "u1", []byte(`{"data":[{"id":"a"},{"id":"b"}]}`), 60)

	id := identity.Identity{Org: "o1", User: "u1"}

	full, err := expand(ctx, s, id, ExpandArgs{Handle: h})
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if !strings.Contains(full, `"id":"a"`) {
		t.Fatalf("full result missing data: %s", full)
	}

	one, err := expand(ctx, s, id, ExpandArgs{Handle: h, Path: "data[1].id"})
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if strings.TrimSpace(one) != `"b"` {
		t.Fatalf("path result = %s, want \"b\"", one)
	}
}

func TestExpandRejectsForeignIdentity(t *testing.T) {
	s := store.NewMemory()
	ctx := context.Background()
	h, _ := s.Put(ctx, "o1", "u1", []byte(`1`), 60)
	_, err := expand(ctx, s, identity.Identity{Org: "o1", User: "u2"}, ExpandArgs{Handle: h})
	if err == nil {
		t.Fatalf("expected rejection for foreign identity")
	}
}

func TestExpandMissingHandle(t *testing.T) {
	s := store.NewMemory()
	_, err := expand(context.Background(), s, identity.Identity{Org: "o", User: "u"}, ExpandArgs{Handle: "x"})
	if err == nil {
		t.Fatalf("expected error for missing handle")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/proxy/ -run TestExpand -v`
Expected: FAIL — `undefined: expand`.

- [ ] **Step 3: Implement**

```go
package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mayur-tolexo/leanmcp/internal/identity"
	"github.com/mayur-tolexo/leanmcp/internal/jsonpath"
	"github.com/mayur-tolexo/leanmcp/internal/store"
)

// ExpandArgs are the typed inputs to the expand_result tool.
type ExpandArgs struct {
	Handle string   `json:"handle"`
	Path   string   `json:"path,omitempty"`
	Fields []string `json:"fields,omitempty"`
	Offset int      `json:"offset,omitempty"`
	Limit  int      `json:"limit,omitempty"`
}

// expand loads the cached entry for args.Handle, enforces that it belongs to id,
// applies path/fields selection, and returns the selected JSON as a string.
func expand(ctx context.Context, s store.Store, id identity.Identity, args ExpandArgs) (string, error) {
	e, err := s.Get(ctx, args.Handle)
	if err != nil {
		return "", fmt.Errorf("result expired or not found; re-run the original tool")
	}
	if e.Org != id.Org || e.User != id.User {
		return "", fmt.Errorf("result expired or not found; re-run the original tool")
	}
	var doc any
	if err := json.Unmarshal(e.Raw, &doc); err != nil {
		return "", err
	}
	selected := doc
	if args.Path != "" {
		selected, err = jsonpath.Get(doc, args.Path)
		if err != nil {
			return "", err
		}
	}
	if len(args.Fields) > 0 {
		selected = jsonpath.Project(selected, args.Fields)
	}
	out, err := json.Marshal(selected)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
```

> Note: `offset`/`limit` paging of arrays can be layered on later; the field is accepted now but full array paging is a follow-up task to keep this step focused.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/proxy/ -run TestExpand -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/expand.go internal/proxy/expand_test.go
git commit -m "feat: add expand_result handler logic"
```

### Task 6.4: tools/call optimization logic (pure, fail-open)

**Files:**
- Create: `internal/proxy/toolscall.go`
- Test: `internal/proxy/toolscall_test.go`

This isolates the decision logic — given a raw result and config, decide pass-through vs compact+cache+trailer — so it is unit-testable without a live MCP server.

- [ ] **Step 1: Write the failing test**

```go
package proxy

import (
	"context"
	"strings"
	"testing"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/identity"
	"github.com/mayur-tolexo/leanmcp/internal/store"
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
	o := newOptimizer(store.NewMemory(), &config.Config{OptimizeThresholdTokens: 1500, ExpandTTL: minute, MaxRawBytes: 1 << 20})
	id := identity.Identity{Org: "o", User: "u"}
	out, err := o.process(context.Background(), id, "small-tool", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.Text != `{"ok":true}` || out.Compacted {
		t.Fatalf("small result should pass through unchanged: %+v", out)
	}
}

func TestOptimizeHeavyCompactsAndCaches(t *testing.T) {
	mem := store.NewMemory()
	o := newOptimizer(mem, &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: minute, MaxRawBytes: 1 << 20})
	id := identity.Identity{Org: "o", User: "u"}
	out, err := o.process(context.Background(), id, "list-tool", bigArray())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !out.Compacted {
		t.Fatalf("heavy result should be compacted")
	}
	if out.Handle == "" {
		t.Fatalf("expected a handle")
	}
	if !strings.Contains(out.Text, "expand_result") {
		t.Fatalf("expected trailer in output")
	}
	if _, err := mem.Get(context.Background(), out.Handle); err != nil {
		t.Fatalf("raw should be cached: %v", err)
	}
}

func TestOptimizeOversizeFailsOpen(t *testing.T) {
	o := newOptimizer(store.NewMemory(), &config.Config{OptimizeThresholdTokens: 1, ExpandTTL: minute, MaxRawBytes: 10})
	id := identity.Identity{Org: "o", User: "u"}
	raw := bigArray()
	out, err := o.process(context.Background(), id, "list-tool", raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.Compacted || out.Handle != "" {
		t.Fatalf("oversize payload must fail open to full result, no handle")
	}
	if out.Text != string(raw) {
		t.Fatalf("oversize payload must return full raw unchanged")
	}
}
```

> Add a package-level test helper `const minute = 60 * time.Second`-equivalent: define `var minute = time.Minute` in the test file and import `time`. (ExpandTTL is a duration; the optimizer converts to seconds.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/proxy/ -run TestOptimize -v`
Expected: FAIL — `undefined: newOptimizer`.

- [ ] **Step 3: Implement**

```go
package proxy

import (
	"context"

	"github.com/mayur-tolexo/leanmcp/internal/compact"
	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/identity"
	"github.com/mayur-tolexo/leanmcp/internal/store"
	"github.com/mayur-tolexo/leanmcp/internal/tokens"
)

// optimizer applies the heavy-result decision logic for a single tool call.
type optimizer struct {
	store store.Store
	cfg   *config.Config
}

// newOptimizer constructs an optimizer.
func newOptimizer(s store.Store, cfg *config.Config) *optimizer {
	return &optimizer{store: s, cfg: cfg}
}

// outcome is the result of optimizing one tool call.
type outcome struct {
	Text      string // text to return to the model
	Compacted bool
	Handle    string
}

// process decides whether to optimize raw for the named tool. Small results pass
// through. Heavy results are compacted; the full raw is cached behind a handle
// and a trailer is appended. Any caching failure (incl. oversize) falls open to
// the full result with no handle.
func (o *optimizer) process(ctx context.Context, id identity.Identity, tool string, raw []byte) (outcome, error) {
	if !tokens.Exceeds(raw, o.cfg.OptimizeThresholdTokens) {
		return outcome{Text: string(raw)}, nil
	}
	mode := "tabular"
	if rule, ok := o.cfg.Tools[tool]; ok && rule.Compaction != "" {
		mode = rule.Compaction
	}
	res := compact.Compact(raw, compact.Options{Mode: mode})

	// Fail open if the raw payload is too large to cache safely.
	if len(raw) > o.cfg.MaxRawBytes {
		return outcome{Text: string(raw)}, nil
	}
	handle, err := o.store.Put(ctx, id.Org, id.User, raw, int(o.cfg.ExpandTTL.Seconds()))
	if err != nil {
		// Fail open: return the full result, no handle.
		return outcome{Text: string(raw)}, nil
	}
	total := countItems(raw)
	text := string(res.View) + Trailer(handle, total, total)
	return outcome{Text: text, Compacted: res.Applied, Handle: handle}, nil
}
```

Add `countItems` to `internal/compact` and reference it, or define locally:

```go
// countItems returns the number of top-level array elements, or 1 for non-arrays.
func countItems(raw []byte) int {
	var arr []any
	if err := jsonUnmarshal(raw, &arr); err == nil {
		return len(arr)
	}
	return 1
}
```

> Use `encoding/json`'s `Unmarshal` directly (import it) rather than a `jsonUnmarshal` alias; the alias here only marks where it is used. Import `encoding/json` and call `json.Unmarshal`.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/proxy/ -run TestOptimize -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/toolscall.go internal/proxy/toolscall_test.go
git commit -m "feat: add tools/call optimization logic with fail-open"
```

### Task 6.5: Server assembly with receiving middleware + end-to-end test

**Files:**
- Create: `internal/proxy/server.go`
- Test: `internal/proxy/server_test.go`

This wires everything: build the frontend `*mcp.Server`, register `expand_result` as a real typed tool, and add receiving middleware that intercepts `tools/list` (merge upstream + paginate) and `tools/call` (forward + optimize). The test runs a fake upstream, points the proxy at it, and drives the proxy through a real MCP client.

- [ ] **Step 1: Write the failing test**

```go
package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/identity"
	"github.com/mayur-tolexo/leanmcp/internal/store"
	"github.com/mayur-tolexo/leanmcp/internal/upstream"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startUpstream starts a fake MCP server exposing a "list_items" tool returning
// a heavy array, and returns its URL.
func startUpstream(t *testing.T) string {
	t.Helper()
	s := mcp.NewServer(&mcp.Implementation{Name: "fake", Version: "0.0.1"}, &mcp.ServerOptions{HasTools: true})
	mcp.AddTool(s, &mcp.Tool{Name: "list_items"},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(bigArray())}}}, nil, nil
		})
	h := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestProxyListInjectsExpandTool(t *testing.T) {
	up := startUpstream(t)
	cfg := &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: minute, MaxRawBytes: 1 << 20}
	server := NewServer(cfg, upstream.New(up), store.NewMemory(), identity.NewStatic("o", "u"))

	proxyHTTP := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	t.Cleanup(proxyHTTP.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	sess, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: proxyHTTP.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	list, err := sess.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var names []string
	for _, tl := range list.Tools {
		names = append(names, tl.Name)
	}
	if !contains(names, "list_items") || !contains(names, "expand_result") {
		t.Fatalf("expected list_items and expand_result, got %v", names)
	}
}

func TestProxyCallCompactsHeavyResult(t *testing.T) {
	up := startUpstream(t)
	mem := store.NewMemory()
	cfg := &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: minute, MaxRawBytes: 1 << 20}
	server := NewServer(cfg, upstream.New(up), mem, identity.NewStatic("o", "u"))

	proxyHTTP := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	t.Cleanup(proxyHTTP.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	sess, _ := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: proxyHTTP.URL}, nil)
	defer sess.Close()

	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_items"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	txt := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(txt, "expand_result") || !strings.Contains(txt, "__leanmcp_table") {
		t.Fatalf("expected compacted result with trailer, got: %s", txt)
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/proxy/ -run TestProxy -v`
Expected: FAIL — `undefined: NewServer`.

- [ ] **Step 3: Implement**

```go
package proxy

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/identity"
	"github.com/mayur-tolexo/leanmcp/internal/store"
	"github.com/mayur-tolexo/leanmcp/internal/upstream"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// expandToolDef is the injected tool advertised to clients.
var expandToolDef = &mcp.Tool{
	Name:        "expand_result",
	Description: "Retrieve the full or partial raw result of a prior compacted tool call.",
}

// NewServer builds the frontend MCP server: it registers expand_result and adds
// receiving middleware that proxies tools/list and tools/call to the upstream.
func NewServer(cfg *config.Config, up *upstream.Client, st store.Store, ver identity.Verifier) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "leanmcp", Version: "0.1.0"}, &mcp.ServerOptions{HasTools: true})
	opt := newOptimizer(st, cfg)

	// Register expand_result as a real typed tool handled locally.
	mcp.AddTool(s, expandToolDef, func(ctx context.Context, req *mcp.CallToolRequest, args ExpandArgs) (*mcp.CallToolResult, any, error) {
		id, err := verifyFromRequest(ctx, ver, req)
		if err != nil {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "unauthorized"}}, IsError: true}, nil, nil
		}
		out, err := expand(ctx, st, id, args)
		if err != nil {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}, IsError: true}, nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil, nil
	})

	s.AddReceivingMiddleware(proxyMiddleware(cfg, up, st, ver, opt))
	return s
}

// proxyMiddleware intercepts tools/list and tools/call; all other methods pass
// through to the default server handling (which serves expand_result).
func proxyMiddleware(cfg *config.Config, up *upstream.Client, st store.Store, ver identity.Verifier, opt *optimizer) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case "tools/list":
				upTools, err := up.ListTools(ctx)
				if err != nil {
					return nil, err
				}
				merged := append(upTools, expandToolDef)
				page, next, err := paginate(merged, listCursor(req), 0)
				if err != nil {
					return nil, err
				}
				return &mcp.ListToolsResult{Tools: page, NextCursor: next}, nil
			case "tools/call":
				name, args := callNameArgs(req)
				if name == "expand_result" {
					return next(ctx, method, req) // handled by the registered tool
				}
				id, err := verifyFromRequest(ctx, ver, req)
				if err != nil {
					return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "unauthorized"}}, IsError: true}, nil
				}
				upRes, err := up.CallTool(ctx, name, args)
				if err != nil {
					return nil, err
				}
				raw := firstText(upRes)
				oc, err := opt.process(ctx, id, name, []byte(raw))
				if err != nil {
					return nil, err
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: oc.Text}}}, nil
			default:
				return next(ctx, method, req)
			}
		}
	}
}

// firstText returns the text of the first content item, or "".
func firstText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}
```

The middleware needs three small request adapters. Because the SDK's `Request` is an interface over `ServerRequest[P]`, extract fields via type assertion to the concrete request types:

```go
// listCursor extracts the cursor from a tools/list request (empty if none).
func listCursor(req mcp.Request) string {
	if r, ok := req.(*mcp.ListToolsRequest); ok && r.Params != nil {
		return r.Params.Cursor
	}
	return ""
}

// callNameArgs extracts the tool name and decoded arguments from a tools/call request.
func callNameArgs(req mcp.Request) (string, map[string]any) {
	r, ok := req.(*mcp.CallToolRequest)
	if !ok || r.Params == nil {
		return "", nil
	}
	var args map[string]any
	if len(r.Params.Arguments) > 0 {
		_ = json.Unmarshal(r.Params.Arguments, &args)
	}
	return r.Params.Name, args
}

// verifyFromRequest reads the forwarded Authorization header from the request
// and verifies it to obtain the caller identity.
func verifyFromRequest(ctx context.Context, ver identity.Verifier, req mcp.Request) (identity.Identity, error) {
	cred := bearerFromRequest(req)
	return ver.Verify(ctx, cred)
}
```

> SDK confirmation needed at build time: the exact concrete types (`*mcp.ListToolsRequest`, `*mcp.CallToolRequest`), the `ListToolsParams.Cursor` field, `CallToolParamsRaw.Name`/`.Arguments` (raw JSON), and how to read the inbound HTTP header (`req.Extra.Header` per the verified `RequestExtra`). Implement `bearerFromRequest` to read `req`'s `Extra.Header.Get("Authorization")` and strip the `Bearer ` prefix. The compiler and the vendored SDK at `.../go-sdk@v1.5.0/mcp/` are the source of truth; adjust names to match.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/proxy/ -v`
Expected: PASS. Fix any SDK field-name mismatches the compiler reports against the vendored SDK.

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/server.go internal/proxy/server_test.go
git commit -m "feat: assemble proxy server with tools/list and tools/call middleware"
```

### Task 6.6: Wire the proxy into main

**Files:**
- Modify: `cmd/leanmcp/main.go`

- [ ] **Step 1: Replace main with full wiring**

```go
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/identity"
	"github.com/mayur-tolexo/leanmcp/internal/proxy"
	"github.com/mayur-tolexo/leanmcp/internal/store"
	"github.com/mayur-tolexo/leanmcp/internal/upstream"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// healthHandler reports liveness with a 200 "ok".
func healthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// buildStore selects Redis when configured, else the in-memory store.
func buildStore(cfg *config.Config) (store.Store, error) {
	if cfg.RedisURL != "" {
		return store.NewRedis(cfg.RedisURL)
	}
	log.Println("LEANMCP_REDIS_URL not set; using in-memory store (single node only)")
	return store.NewMemory(), nil
}

// buildVerifier selects the HTTP verifier when an endpoint is configured, else a
// static dev verifier.
func buildVerifier(cfg *config.Config) identity.Verifier {
	if cfg.VerifyEndpoint != "" {
		return identity.NewHTTP(cfg.VerifyEndpoint)
	}
	log.Println("LEANMCP_VERIFY_ENDPOINT not set; using static dev identity (NOT for multi-tenant use)")
	return identity.NewStatic("dev-org", "dev-user")
}

func main() {
	cfg, err := config.Load(os.Getenv("LEANMCP_CONFIG"))
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	st, err := buildStore(cfg)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	server := proxy.NewServer(cfg, upstream.New(cfg.UpstreamMCPURL), st, buildVerifier(cfg))

	mux := http.NewServeMux()
	mux.Handle("/healthz", healthHandler())
	mux.Handle("/readyz", healthHandler())
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	))

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Printf("leanmcp listening on %s, upstream=%s", cfg.ListenAddr, cfg.UpstreamMCPURL)
	log.Fatal(srv.ListenAndServe())
}
```

- [ ] **Step 2: Update the health test if needed and build**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS across all packages.

- [ ] **Step 3: Commit**

```bash
git add cmd/leanmcp/main.go
git commit -m "feat: wire proxy server into main entrypoint"
```

---

# Phase 7 — Observability

**Goal:** Prometheus metrics for compaction ratio, expand rate, and fail-open reasons; shadow-mode behavior recorded.

### Task 7.1: Metrics registry and counters

**Files:**
- Create: `internal/metrics/metrics.go`
- Test: `internal/metrics/metrics_test.go`

- [ ] **Step 1: Write the failing test**

```go
package metrics

import "testing"

func TestRecordCompaction(t *testing.T) {
	m := New()
	m.RecordCompaction("list_items", 6000, 2000) // must not panic and should register
	m.RecordExpand("list_items")
	m.RecordFailOpen("redis_down")
	// A smoke assertion: the Handler is non-nil.
	if m.Handler() == nil {
		t.Fatalf("expected a metrics handler")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/metrics/ -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Implement**

```go
// Package metrics exposes Prometheus counters/histograms for leanmcp.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the registered collectors.
type Metrics struct {
	reg          *prometheus.Registry
	origBytes    *prometheus.HistogramVec
	compactBytes *prometheus.HistogramVec
	expands      *prometheus.CounterVec
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
		expands: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leanmcp_expand_total", Help: "expand_result calls.",
		}, []string{"tool"}),
		failOpen: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "leanmcp_fail_open_total", Help: "Fail-open events by reason.",
		}, []string{"reason"}),
	}
	reg.MustRegister(m.origBytes, m.compactBytes, m.expands, m.failOpen)
	return m
}

// RecordCompaction records original and compacted sizes for a tool.
func (m *Metrics) RecordCompaction(tool string, orig, compacted int) {
	m.origBytes.WithLabelValues(tool).Observe(float64(orig))
	m.compactBytes.WithLabelValues(tool).Observe(float64(compacted))
}

// RecordExpand counts an expand_result call.
func (m *Metrics) RecordExpand(tool string) { m.expands.WithLabelValues(tool).Inc() }

// RecordFailOpen counts a fail-open event by reason.
func (m *Metrics) RecordFailOpen(reason string) { m.failOpen.WithLabelValues(reason).Inc() }

// Handler returns the /metrics HTTP handler.
func (m *Metrics) Handler() http.Handler { return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{}) }
```

- [ ] **Step 4: Add dependency and run**

Run:
```bash
go get github.com/prometheus/client_golang/prometheus
go test ./internal/metrics/ -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/ go.mod go.sum
git commit -m "feat: add prometheus metrics"
```

### Task 7.2: Thread metrics + shadow mode through the optimizer

**Files:**
- Modify: `internal/proxy/toolscall.go`
- Modify: `internal/proxy/server.go`
- Modify: `cmd/leanmcp/main.go`
- Test: `internal/proxy/toolscall_test.go` (extend)

- [ ] **Step 1: Extend the failing test**

```go
func TestShadowModeReturnsFullButCaches(t *testing.T) {
	mem := store.NewMemory()
	cfg := &config.Config{OptimizeThresholdTokens: 100, ExpandTTL: minute, MaxRawBytes: 1 << 20, ShadowMode: true}
	o := newOptimizer(mem, cfg)
	id := identity.Identity{Org: "o", User: "u"}
	out, err := o.process(context.Background(), id, "list-tool", bigArray())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Shadow mode: client still sees the full result (no trailer/compaction),
	// but the raw is cached and metrics recorded.
	if out.Compacted {
		t.Fatalf("shadow mode must not compact the client-facing result")
	}
	if !strings.Contains(out.Text, `"status":"active"`) {
		t.Fatalf("shadow mode must return full result")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/proxy/ -run TestShadowMode -v`
Expected: FAIL — shadow-mode branch not implemented.

- [ ] **Step 3: Implement shadow-mode branch**

In `process`, after computing `res` and validating size, record metrics and branch on shadow mode:

```go
	// (after computing res and before constructing the trailer)
	total := countItems(raw)
	if o.cfg.ShadowMode {
		// Compact and cache for measurement, but return the full result to the client.
		_, _ = o.store.Put(ctx, id.Org, id.User, raw, int(o.cfg.ExpandTTL.Seconds()))
		return outcome{Text: string(raw)}, nil
	}
	handle, err := o.store.Put(ctx, id.Org, id.User, raw, int(o.cfg.ExpandTTL.Seconds()))
	if err != nil {
		return outcome{Text: string(raw)}, nil
	}
	text := string(res.View) + Trailer(handle, total, total)
	return outcome{Text: text, Compacted: res.Applied, Handle: handle}, nil
```

> Wire the `*metrics.Metrics` into `newOptimizer` (add a field and parameter), call `RecordCompaction` on every heavy result and `RecordFailOpen` on each fail-open branch, and expose `/metrics` from `main.go` via `metrics.Handler()`. Update `NewServer` to accept and pass the metrics instance. Update the existing optimizer tests to pass a `metrics.New()` (or a nil-safe no-op) instance.

- [ ] **Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/ cmd/leanmcp/main.go
git commit -m "feat: add shadow mode and metrics wiring"
```

---

# Phase 8 — Packaging and deployment

**Goal:** Container image, example config, and Kubernetes manifests. Docs updated.

### Task 8.1: Dockerfile and example config

**Files:**
- Create: `Dockerfile`
- Create: `leanmcp.example.yaml`

- [ ] **Step 1: Add the Dockerfile**

```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/leanmcp ./cmd/leanmcp

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/leanmcp /leanmcp
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/leanmcp"]
```

- [ ] **Step 2: Add the example config**

```yaml
# leanmcp.example.yaml
upstream_mcp_url: http://upstream-mcp:8080/mcp
redis_url: redis://redis:6379
verify_endpoint: http://auth:8080/verify   # omit for static dev identity
optimize_threshold_tokens: 1500
expand_ttl: 10m
max_raw_bytes: 5000000
shadow_mode: true
listen_addr: ":8080"
tools:
  list_items:
    compaction: tabular
    array_path: data
    max_items: 50
```

- [ ] **Step 3: Build the image**

Run: `docker build -t leanmcp:dev .`
Expected: image builds successfully.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile leanmcp.example.yaml
git commit -m "build: add Dockerfile and example config"
```

### Task 8.2: Kubernetes manifests

**Files:**
- Create: `deploy/k8s/deployment.yaml`
- Create: `deploy/k8s/service.yaml`

- [ ] **Step 1: Add the Deployment**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: leanmcp
spec:
  replicas: 2
  selector:
    matchLabels: { app: leanmcp }
  template:
    metadata:
      labels: { app: leanmcp }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
      containers:
        - name: leanmcp
          image: leanmcp:dev
          ports: [{ containerPort: 8080 }]
          env:
            - { name: LEANMCP_UPSTREAM_MCP_URL, value: "http://upstream-mcp:8080/mcp" }
            - { name: LEANMCP_REDIS_URL, value: "redis://redis:6379" }
            - { name: LEANMCP_VERIFY_ENDPOINT, value: "http://auth:8080/verify" }
            - { name: LEANMCP_SHADOW_MODE, value: "true" }
          readinessProbe:
            httpGet: { path: /readyz, port: 8080 }
          livenessProbe:
            httpGet: { path: /healthz, port: 8080 }
          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
```

- [ ] **Step 2: Add the Service**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: leanmcp
spec:
  selector: { app: leanmcp }
  ports:
    - { port: 80, targetPort: 8080 }
```

- [ ] **Step 3: Validate**

Run: `kubectl apply --dry-run=client -f deploy/k8s/`
Expected: both objects validate.

- [ ] **Step 4: Commit**

```bash
git add deploy/k8s/
git commit -m "deploy: add kubernetes manifests"
```

### Task 8.3: Update README usage

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add a "Run it" section**

Add to `README.md`:

````markdown
## Run it

```bash
export LEANMCP_UPSTREAM_MCP_URL=http://your-upstream/mcp
export LEANMCP_REDIS_URL=redis://localhost:6379   # optional; in-memory if unset
go run ./cmd/leanmcp
```

Point your MCP client at `http://localhost:8080/mcp`. Metrics are at `/metrics`.
Start in `shadow_mode: true` to measure savings before enabling handle-only output.
````

- [ ] **Step 2: Build docs sanity / commit**

```bash
git add README.md
git commit -m "docs: add run instructions"
```

---

## Self-review checklist (completed during authoring)

- **Spec coverage:** transparent proxy (Phase 5/6), lossless tabular compaction (2.3/2.4), expand-on-demand + handle store (3, 6.3), identity-scoped handles + verification (3.1, 4, 6.5), local re-pagination (6.2), threshold gating (2.1, 6.4), fail-open (6.4), shadow mode (7.2), metrics (7), deployment (8). LLM summarization intentionally excluded per spec §8.3. Field projection available via `KeepFields` config + `jsonpath.Project` (opt-in, spec §6.2/§8.4).
- **Placeholders:** none — every code step contains complete code. Two wiring tasks (6.5) flag specific SDK field names to confirm at build time against the vendored SDK; these are real, named types, surfaced by the compiler, not invented logic.
- **Type consistency:** `store.Store`/`Entry`, `identity.Identity`/`Verifier`, `compact.Options`/`Result`, `proxy.optimizer.process`/`outcome`, `ExpandArgs`, and `paginate` signatures are used consistently across tasks.

## Deferred (post-v1, tracked here so nothing is lost)

- `dedupe-keys` compaction mode (spec §5.5) — only `tabular` + `none` are in v1.
- `offset`/`limit` array paging inside `expand_result` (field accepted in 6.3; paging is a follow-up).
- Opt-in lossy field projection wired into the hot path (helper exists; enabling per-tool is a config + small `process` change).
- PAT-revocation-time guard on expand (spec §7) — requires the verifier to expose a revocation timestamp.
- Per-identity verification caching (≤60s) to bound verify latency.
