package store

import "testing"

func TestNewMemoryStore(t *testing.T) {
	s, err := New("memory", "")
	if err != nil {
		t.Fatalf("New(\"memory\",\"\") error: %v", err)
	}
	if s == nil {
		t.Fatal("New(\"memory\",\"\") returned nil Store")
	}
}

func TestNewEmptyTypeReturnsMemory(t *testing.T) {
	s, err := New("", "")
	if err != nil {
		t.Fatalf("New(\"\",\"\") error: %v", err)
	}
	if s == nil {
		t.Fatal("New(\"\",\"\") returned nil Store")
	}
}

func TestNewRedisWithoutURLErrors(t *testing.T) {
	_, err := New("redis", "")
	if err == nil {
		t.Fatal("New(\"redis\",\"\") expected error, got nil")
	}
}

func TestNewUnknownTypeErrors(t *testing.T) {
	_, err := New("bogus", "")
	if err == nil {
		t.Fatal("New(\"bogus\",\"\") expected error, got nil")
	}
}
