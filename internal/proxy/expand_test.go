package proxy

import (
	"context"
	"strings"
	"testing"

	"github.com/mayur-tolexo/leanmcp/store"
)

func TestExpandFullAndPath(t *testing.T) {
	s := store.NewMemory()
	ctx := context.Background()
	h, _ := s.Put(ctx, "credA", []byte(`{"data":[{"id":"a"},{"id":"b"}]}`), 60)
	full, err := expand(ctx, s, "credA", ExpandArgs{Handle: h})
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	if !strings.Contains(full, `"id":"a"`) {
		t.Fatalf("missing data: %s", full)
	}
	one, err := expand(ctx, s, "credA", ExpandArgs{Handle: h, Path: "data[1].id"})
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if strings.TrimSpace(one) != `"b"` {
		t.Fatalf("path = %s", one)
	}
}

func TestExpandRejectsForeignCredential(t *testing.T) {
	s := store.NewMemory()
	ctx := context.Background()
	h, _ := s.Put(ctx, "credA", []byte(`1`), 60)
	if _, err := expand(ctx, s, "credB", ExpandArgs{Handle: h}); err == nil {
		t.Fatalf("expected rejection for foreign credential")
	}
}

func TestExpandMissingHandle(t *testing.T) {
	s := store.NewMemory()
	if _, err := expand(context.Background(), s, "credA", ExpandArgs{Handle: "x"}); err == nil {
		t.Fatalf("expected error for missing handle")
	}
}
