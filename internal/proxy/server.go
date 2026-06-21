package proxy

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mayur-tolexo/leanmcp/internal/config"
	"github.com/mayur-tolexo/leanmcp/internal/cred"
	"github.com/mayur-tolexo/leanmcp/internal/upstream"
	"github.com/mayur-tolexo/leanmcp/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// expandToolDef is the static definition of the expand_result tool advertised to
// clients and used both for registration and for injection into tools/list.
var expandToolDef = &mcp.Tool{
	Name:        "expand_result",
	Description: "Retrieve the full or partial raw result of a prior compacted tool call.",
}

// NewServer assembles the leanmcp proxy MCP server. It registers expand_result
// as a typed tool and installs receiving middleware that proxies tools/list and
// tools/call to the upstream while applying compaction. cacheSecret keys the
// per-credential hash that binds cache handles.
func NewServer(cfg *config.Config, up *upstream.Client, st store.Store, cacheSecret string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "leanmcp", Version: "0.1.0"}, &mcp.ServerOptions{HasTools: true})

	opt := newOptimizer(st, cfg)

	// Register expand_result. The handler derives the caller's credential hash
	// from the inbound bearer, expands the stored result, and returns it as text;
	// any error is surfaced as an error result rather than a transport failure.
	mcp.AddTool(srv, expandToolDef,
		func(ctx context.Context, req *mcp.CallToolRequest, args ExpandArgs) (*mcp.CallToolResult, any, error) {
			credHash := cred.HMAC(cacheSecret, bearerFromExtra(req.Extra))
			text, err := expand(ctx, st, credHash, args)
			if err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				}, nil, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
		})

	// Install receiving middleware that intercepts list/call and proxies them.
	srv.AddReceivingMiddleware(proxyMiddleware(cfg, up, opt, cacheSecret))

	return srv
}

// proxyMiddleware returns receiving middleware that forwards tools/list and
// tools/call to the upstream, injecting the expand_result tool into listings and
// compacting heavy call results; all other methods pass through to next.
func proxyMiddleware(cfg *config.Config, up *upstream.Client, opt *optimizer, cacheSecret string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case "tools/list":
				return handleList(ctx, up, req)
			case "tools/call":
				return handleCall(ctx, up, opt, cacheSecret, next, method, req)
			default:
				return next(ctx, method, req)
			}
		}
	}
}

// handleList fetches the upstream tool list (forwarding the inbound header),
// appends the expand_result definition, paginates by the request cursor, and
// returns the page as a ListToolsResult.
func handleList(ctx context.Context, up *upstream.Client, req mcp.Request) (mcp.Result, error) {
	lr, _ := req.(*mcp.ListToolsRequest)

	hctx := withInboundHeader(ctx, req)
	tools, err := up.ListTools(hctx)
	if err != nil {
		return nil, err
	}

	// Build the merged list in a freshly allocated slice. Appending directly to
	// the slice returned by ListTools could mutate its backing array if it has
	// spare capacity, so copy into a new slice sized for the extra tool.
	merged := make([]*mcp.Tool, 0, len(tools)+1)
	merged = append(merged, tools...)
	merged = append(merged, expandToolDef)

	cursor := ""
	if lr != nil && lr.Params != nil {
		cursor = lr.Params.Cursor
	}
	page, next, err := paginate(merged, cursor, 0)
	if err != nil {
		return nil, err
	}
	return &mcp.ListToolsResult{Tools: page, NextCursor: next}, nil
}

// handleCall proxies a tools/call to the upstream. The expand_result tool is
// delegated to its registered handler via next. Other tools are called upstream
// (forwarding the inbound header). An upstream tool-level error result is passed
// back unchanged so the model can see and self-correct on it; otherwise the
// result's first text block is run through the optimizer. When the optimizer
// compacts, a single compacted text block is returned; when it does not, the
// upstream result is returned unchanged so non-text and multi-block content
// survive intact.
func handleCall(ctx context.Context, up *upstream.Client, opt *optimizer, cacheSecret string,
	next mcp.MethodHandler, method string, req mcp.Request) (mcp.Result, error) {
	cr, ok := req.(*mcp.CallToolRequest)
	if !ok || cr.Params == nil {
		return next(ctx, method, req)
	}

	name := cr.Params.Name
	// expand_result is served by the registered typed tool handler.
	if name == "expand_result" {
		return next(ctx, method, req)
	}

	// Decode the raw upstream arguments into a generic map for forwarding.
	var args map[string]any
	if len(cr.Params.Arguments) > 0 {
		if err := json.Unmarshal(cr.Params.Arguments, &args); err != nil {
			return nil, err
		}
	}

	hctx := withInboundHeader(ctx, req)
	res, err := up.CallTool(hctx, name, args)
	if err != nil {
		return nil, err
	}

	// A tool-level error from the upstream must be relayed verbatim. Compacting
	// or caching an error payload would both lie about success and hand back an
	// expand handle for an error string, so return it untouched.
	if res == nil || res.IsError {
		return res, nil
	}

	raw := firstText(res)
	credHash := cred.HMAC(cacheSecret, bearerFromExtra(cr.Extra))
	oc, err := opt.process(ctx, credHash, name, []byte(raw))
	if err != nil {
		return nil, err
	}

	// When nothing was compacted, return the upstream result unchanged so image,
	// resource, structured and multi-block content are preserved losslessly.
	if !oc.Compacted {
		return res, nil
	}

	// Compaction applied: replace the content with the single compacted text
	// block carrying the expand_result trailer, preserving any structured output.
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: oc.Text}},
		StructuredContent: res.StructuredContent,
	}, nil
}

// withInboundHeader threads the inbound HTTP header carried on the server request
// into ctx so the upstream client forwards it to the upstream endpoint.
func withInboundHeader(ctx context.Context, req mcp.Request) context.Context {
	extra := req.GetExtra()
	if extra == nil || extra.Header == nil {
		return ctx
	}
	return upstream.WithHeaders(ctx, extra.Header)
}

// firstText returns the text of the first TextContent in res, or "" if there is
// none. The first text block is treated as the tool's raw payload.
func firstText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// bearerFromExtra extracts the bearer token from the Authorization header in
// extra, stripping a case-insensitive "Bearer " prefix; it returns "" when no
// header or token is present.
func bearerFromExtra(extra *mcp.RequestExtra) string {
	if extra == nil || extra.Header == nil {
		return ""
	}
	return parseBearer(extra.Header.Get("Authorization"))
}

// parseBearer returns the token portion of an Authorization header value with a
// case-insensitive "Bearer " prefix removed, or "" when the value lacks that
// prefix or is empty.
func parseBearer(auth string) string {
	const prefix = "bearer "
	if len(auth) >= len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return auth[len(prefix):]
	}
	return ""
}
