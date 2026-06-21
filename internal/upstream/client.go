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
