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
