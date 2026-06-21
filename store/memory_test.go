package store

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryPutGetRoundTrip(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	raw := []byte(`{"result":"ok"}`)

	handle, err := s.Put(ctx, "cred123", raw, 60)
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
	if entry.CredHash != "cred123" {
		t.Fatalf("CredHash = %q, want %q", entry.CredHash, "cred123")
	}
	if string(entry.Raw) != string(raw) {
		t.Fatalf("Raw = %q, want %q", entry.Raw, raw)
	}
}

func TestMemoryGetUnknownHandleReturnsNotFound(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()

	_, err := s.Get(ctx, "no-such-handle")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}
