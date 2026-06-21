package store

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestRedisPutGetRoundTrip(t *testing.T) {
	url := os.Getenv("LEANMCP_TEST_REDIS_URL")
	if url == "" {
		t.Skip("LEANMCP_TEST_REDIS_URL not set; skipping redis integration test")
	}

	s, err := NewRedis(url)
	if err != nil {
		t.Fatalf("NewRedis() error: %v", err)
	}

	ctx := context.Background()
	raw := []byte(`{"result":"from-redis"}`)

	handle, err := s.Put(ctx, "credhash-redis", raw, 60)
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	if handle == "" {
		t.Fatal("Put() returned empty handle")
	}

	entry, err := s.Get(ctx, handle)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if entry.CredHash != "credhash-redis" {
		t.Fatalf("CredHash = %q, want %q", entry.CredHash, "credhash-redis")
	}
	if string(entry.Raw) != string(raw) {
		t.Fatalf("Raw = %q, want %q", entry.Raw, raw)
	}
}

func TestRedisGetUnknownHandleReturnsNotFound(t *testing.T) {
	url := os.Getenv("LEANMCP_TEST_REDIS_URL")
	if url == "" {
		t.Skip("LEANMCP_TEST_REDIS_URL not set; skipping redis integration test")
	}

	s, err := NewRedis(url)
	if err != nil {
		t.Fatalf("NewRedis() error: %v", err)
	}

	ctx := context.Background()
	_, err = s.Get(ctx, "no-such-handle-xyz")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}
