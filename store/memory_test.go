package store

import (
	"context"
	"errors"
	"sync"
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

func TestMemoryHandleReadableMultipleTimes(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	raw := []byte(`{"n":1}`)
	handle, err := s.Put(ctx, "cred", raw, 60)
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	// A handle is a multi-read cache entry, not a one-shot token: repeated Get
	// calls must keep succeeding until the TTL elapses.
	for i := 0; i < 3; i++ {
		e, err := s.Get(ctx, handle)
		if err != nil {
			t.Fatalf("Get() #%d error: %v", i, err)
		}
		if string(e.Raw) != string(raw) {
			t.Fatalf("Get() #%d Raw = %q, want %q", i, e.Raw, raw)
		}
	}
}

func TestMemoryGetExpiredReturnsNotFound(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	// TTL of 0 seconds expires immediately on the next read.
	handle, err := s.Put(ctx, "cred", []byte(`1`), 0)
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	if _, err := s.Get(ctx, handle); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired Get() error = %v, want ErrNotFound", err)
	}
}

func TestMemoryConcurrentPutGet(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()
	// Exercise the mutex under -race with concurrent writers and readers.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := s.Put(ctx, "cred", []byte(`{"x":1}`), 60)
			if err != nil {
				t.Errorf("Put() error: %v", err)
				return
			}
			if _, err := s.Get(ctx, h); err != nil {
				t.Errorf("Get() error: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestMemoryGetUnknownHandleReturnsNotFound(t *testing.T) {
	s := NewMemory()
	ctx := context.Background()

	_, err := s.Get(ctx, "no-such-handle")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}
