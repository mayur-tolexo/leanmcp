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

func TestExpandOffsetLimitPagesArray(t *testing.T) {
	s := store.NewMemory()
	ctx := context.Background()
	h, _ := s.Put(ctx, "credA", []byte(`{"data":[0,1,2,3,4]}`), 60)
	// Page the inner array via path + offset/limit.
	out, err := expand(ctx, s, "credA", ExpandArgs{Handle: h, Path: "data", Offset: 1, Limit: 2})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if strings.TrimSpace(out) != `[1,2]` {
		t.Fatalf("offset/limit slice = %s, want [1,2]", out)
	}
}

func TestExpandOffsetLimitClampsBounds(t *testing.T) {
	s := store.NewMemory()
	ctx := context.Background()
	h, _ := s.Put(ctx, "credA", []byte(`[10,20,30]`), 60)
	// Offset past the end yields an empty page, not an error or panic.
	out, err := expand(ctx, s, "credA", ExpandArgs{Handle: h, Offset: 99, Limit: 5})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if strings.TrimSpace(out) != `[]` {
		t.Fatalf("out-of-range offset = %s, want []", out)
	}
	// Limit larger than the remainder returns the remainder.
	out, err = expand(ctx, s, "credA", ExpandArgs{Handle: h, Offset: 1, Limit: 99})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if strings.TrimSpace(out) != `[20,30]` {
		t.Fatalf("clamped limit = %s, want [20,30]", out)
	}
}
