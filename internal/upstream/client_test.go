package upstream

import (
	"context"
	"net/http"
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
