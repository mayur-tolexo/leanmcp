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
