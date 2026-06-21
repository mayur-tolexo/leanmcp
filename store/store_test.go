package store

import "testing"

func TestNewHandleNonEmpty(t *testing.T) {
	h, err := newHandle()
	if err != nil {
		t.Fatalf("newHandle() error: %v", err)
	}
	if h == "" {
		t.Fatal("newHandle() returned empty string")
	}
}

func TestNewHandleTwoCallsDiffer(t *testing.T) {
	h1, err := newHandle()
	if err != nil {
		t.Fatalf("first newHandle() error: %v", err)
	}
	h2, err := newHandle()
	if err != nil {
		t.Fatalf("second newHandle() error: %v", err)
	}
	if h1 == h2 {
		t.Fatalf("newHandle() returned identical values: %q", h1)
	}
}
