package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/upstream"
	"github.com/mayur-tolexo/leanmcp/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeUpstreamServer starts a minimal MCP server exposing a single list_items
// tool whose output text is supplied by reply, and returns its base URL.
func fakeUpstreamServer(t *testing.T, reply string) string {
	t.Helper()
	s := mcp.NewServer(&mcp.Implementation{Name: "fake", Version: "0.0.1"}, &mcp.ServerOptions{HasTools: true})
	mcp.AddTool(s, &mcp.Tool{Name: "list_items", Description: "lists items"},
		func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: reply}}}, nil, nil
		})
	h := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

// proxyClientSession builds the proxy around the given upstream URL and config,
// exposes it over a streamable HTTP handler, and returns a connected go-sdk
// client session pointed at it.
func proxyClientSession(t *testing.T, upstreamURL string, cfg *config.Config) *mcp.ClientSession {
	t.Helper()
	cfg.UpstreamMCPURL = upstreamURL
	st := store.NewMemory()
	up := upstream.New(upstreamURL)
	srv := NewServer(cfg, up, st, cfg.CacheSecret)

	h := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	sess, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess
}

func TestProxyListInjectsExpandTool(t *testing.T) {
	up := fakeUpstreamServer(t, "[]")
	cfg := &config.Config{CacheSecret: "secret"}
	sess := proxyClientSession(t, up, cfg)

	res, err := sess.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range res.Tools {
		names[tl.Name] = true
	}
	if !names["list_items"] {
		t.Fatalf("missing upstream tool list_items: %+v", names)
	}
	if !names["expand_result"] {
		t.Fatalf("missing injected expand_result: %+v", names)
	}
}

func TestProxyCallCompactsHeavyResult(t *testing.T) {
	up := fakeUpstreamServer(t, string(bigArray()))
	cfg := &config.Config{
		CacheSecret:             "secret",
		OptimizeThresholdTokens: 100,
		ExpandTTL:               time.Minute,
		MaxRawBytes:             1 << 20,
		ShadowMode:              false,
	}
	sess := proxyClientSession(t, up, cfg)

	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_items",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	txt, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	if !strings.Contains(txt.Text, "expand_result") {
		t.Fatalf("compacted text missing expand_result trailer: %q", txt.Text)
	}
	if !strings.Contains(txt.Text, "__leanmcp_table") {
		t.Fatalf("compacted text missing __leanmcp_table marker: %q", txt.Text)
	}
}
