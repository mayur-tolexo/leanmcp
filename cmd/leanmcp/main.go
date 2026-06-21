// Command leanmcp runs the leanmcp optimizing MCP proxy: it loads configuration,
// builds the cache store and upstream client, assembles the proxy MCP server, and
// serves it over Streamable HTTP alongside health endpoints.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/metrics"
	"github.com/mayur-tolexo/leanmcp/internal/proxy"
	"github.com/mayur-tolexo/leanmcp/internal/upstream"
	"github.com/mayur-tolexo/leanmcp/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// healthHandler returns a handler that reports liveness with a 200 "ok".
func healthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// main loads config, constructs the proxy server, mounts health and /mcp routes,
// and serves until the process is terminated.
func main() {
	cfg, err := config.Load(os.Getenv("LEANMCP_CONFIG"))
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Build the cache store backing compacted-result handles.
	st, err := store.New(cfg.Store.Type, cfg.Store.RedisURL)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	// Warn when no cache secret is set: handles are then bound by a weak key and
	// this is acceptable only for local development.
	if cfg.CacheSecret == "" {
		log.Println("warning: cache_secret is empty; expand handles will be weakly bound (dev only)")
	}

	// Initialise Prometheus metrics collectors; all counters/histograms are
	// registered on a private registry so the default Go runtime metrics are
	// not exposed alongside leanmcp's own metrics.
	m := metrics.New()

	srv := proxy.NewServer(cfg, upstream.New(cfg.UpstreamMCPURL), st, cfg.CacheSecret, m)

	// Serve the proxy MCP server over a stateless, JSON Streamable HTTP handler.
	mcpHandler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})

	mux := http.NewServeMux()
	mux.Handle("/healthz", healthHandler())
	mux.Handle("/readyz", healthHandler())
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/metrics", m.Handler())

	// ReadHeaderTimeout bounds slow-header (slow-loris) clients. WriteTimeout is
	// left unset because a single /mcp request proxies a tools/call to the
	// upstream whose duration is unbounded from the proxy's view; a fixed write
	// deadline would truncate legitimately long tool responses mid-stream.
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("leanmcp listening on %s (upstream %s)", cfg.ListenAddr, cfg.UpstreamMCPURL)
	log.Fatal(httpSrv.ListenAndServe())
}
