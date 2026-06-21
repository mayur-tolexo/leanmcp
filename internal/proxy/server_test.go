package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
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
	srv := NewServer(cfg, up, st, cfg.CacheSecret, nil)

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

// fakeUpstreamServerTool starts an upstream MCP server exposing a single tool
// whose handler is supplied by fn, and returns its base URL. It lets tests
// produce error results, non-text content, or empty content.
func fakeUpstreamServerTool(t *testing.T, toolName string,
	fn func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error)) string {
	t.Helper()
	s := mcp.NewServer(&mcp.Implementation{Name: "fake", Version: "0.0.1"}, &mcp.ServerOptions{HasTools: true})
	mcp.AddTool(s, &mcp.Tool{Name: toolName, Description: "test tool"}, fn)
	h := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestProxyCallRelaysUpstreamErrorResult(t *testing.T) {
	// An upstream tool-level error must reach the client as IsError, not be
	// compacted into a successful result.
	up := fakeUpstreamServerTool(t, "boom",
		func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "upstream failed"}},
			}, nil, nil
		})
	cfg := &config.Config{CacheSecret: "secret", OptimizeThresholdTokens: 1, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20}
	sess := proxyClientSession(t, up, cfg)

	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "boom", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError result, got %+v", res)
	}
	txt, ok := res.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(txt.Text, "upstream failed") {
		t.Fatalf("error text not relayed: %+v", res.Content)
	}
	if strings.Contains(txt.Text, "expand_result") {
		t.Fatalf("error result must not carry an expand trailer/handle: %q", txt.Text)
	}
}

func TestProxyCallPreservesNonTextContentWhenNotCompacted(t *testing.T) {
	// A small, multi-block result (text + image) that the optimizer leaves alone
	// must pass through with all content blocks intact, not collapse to one text.
	up := fakeUpstreamServerTool(t, "mixed",
		func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: `{"ok":true}`},
					&mcp.ImageContent{MIMEType: "image/png", Data: []byte{1, 2, 3}},
				},
			}, nil, nil
		})
	cfg := &config.Config{CacheSecret: "secret", OptimizeThresholdTokens: 1500, ExpandTTL: time.Minute, MaxRawBytes: 1 << 20}
	sess := proxyClientSession(t, up, cfg)

	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "mixed", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(res.Content) != 2 {
		t.Fatalf("expected 2 content blocks preserved, got %d: %+v", len(res.Content), res.Content)
	}
	if _, ok := res.Content[1].(*mcp.ImageContent); !ok {
		t.Fatalf("image content dropped: %T", res.Content[1])
	}
}

func TestHandleListInjectsExpandWithoutMutatingUpstream(t *testing.T) {
	// handleList must return the upstream tools plus expand_result, and must not
	// append into the backing array of the slice returned by the upstream client.
	// A slice with spare capacity beyond its length exposes any unsafe append.
	upTools := make([]*mcp.Tool, 1, 4)
	upTools[0] = &mcp.Tool{Name: "list_items"}
	sentinel := &mcp.Tool{Name: "sentinel"}
	upTools[:cap(upTools)][1] = sentinel

	merged := make([]*mcp.Tool, 0, len(upTools)+1)
	merged = append(merged, upTools...)
	merged = append(merged, expandToolDef)

	if upTools[:cap(upTools)][1] != sentinel {
		t.Fatalf("upstream backing array was overwritten by append: %+v", upTools[:cap(upTools)][1])
	}
	if merged[0].Name != "list_items" || merged[len(merged)-1] != expandToolDef {
		t.Fatalf("merged list malformed: %+v", merged)
	}
	if len(upTools) != 1 {
		t.Fatalf("upstream slice length changed: %d", len(upTools))
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

func TestProxyExpandRoundTripThroughProxy(t *testing.T) {
	// End-to-end: a heavy call returns a handle in its trailer, then expand_result
	// called back through the proxy returns the full raw result. The same (empty)
	// bearer is used on both calls, so the credential-bound handle redeems.
	raw := string(bigArray())
	up := fakeUpstreamServer(t, raw)
	cfg := &config.Config{
		CacheSecret:             "secret",
		OptimizeThresholdTokens: 100,
		ExpandTTL:               time.Minute,
		MaxRawBytes:             1 << 20,
		ShadowMode:              false,
	}
	sess := proxyClientSession(t, up, cfg)

	res, err := sess.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_items", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	txt := res.Content[0].(*mcp.TextContent).Text
	m := regexp.MustCompile(`handle="([^"]+)"`).FindStringSubmatch(txt)
	if m == nil {
		t.Fatalf("no handle in trailer: %q", txt)
	}
	handle := m[1]

	exp, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "expand_result",
		Arguments: map[string]any{"handle": handle},
	})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if exp.IsError {
		t.Fatalf("expand returned error: %+v", exp.Content)
	}
	got := exp.Content[0].(*mcp.TextContent).Text
	if got != raw {
		t.Fatalf("expand did not return full raw result;\n got: %s\nwant: %s", got, raw)
	}
}

func TestProxyExpandUnknownHandleReturnsErrorResult(t *testing.T) {
	// An unknown handle must come back as a tool error result (IsError), not a
	// transport-level error, so the model can self-correct.
	up := fakeUpstreamServer(t, "[]")
	cfg := &config.Config{CacheSecret: "secret"}
	sess := proxyClientSession(t, up, cfg)

	exp, err := sess.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "expand_result",
		Arguments: map[string]any{"handle": "does-not-exist"},
	})
	if err != nil {
		t.Fatalf("expand transport error (should be tool error result): %v", err)
	}
	if !exp.IsError {
		t.Fatalf("expected IsError for unknown handle, got %+v", exp)
	}
}
