package compact

import "testing"

func TestCompactTabularMode(t *testing.T) {
	// Use enough rows so the column-header savings outweigh the wrapper overhead.
	in := []byte(`[{"id":"1","name":"alice"},{"id":"2","name":"bob"},{"id":"3","name":"carol"},{"id":"4","name":"dave"},{"id":"5","name":"eve"},{"id":"6","name":"frank"},{"id":"7","name":"grace"},{"id":"8","name":"heidi"},{"id":"9","name":"ivan"},{"id":"10","name":"judy"}]`)
	res := Compact(in, Options{Mode: "tabular"})
	if !res.Applied {
		t.Fatalf("expected applied")
	}
	if res.OriginalBytes != len(in) {
		t.Fatalf("OriginalBytes = %d, want %d", res.OriginalBytes, len(in))
	}
	if res.CompactBytes >= res.OriginalBytes {
		t.Fatalf("expected smaller output")
	}
}

func TestCompactNoneMode(t *testing.T) {
	in := []byte(`[{"id":"1"}]`)
	res := Compact(in, Options{Mode: "none"})
	if res.Applied {
		t.Fatalf("none mode must not transform")
	}
	if string(res.View) != string(in) {
		t.Fatalf("none mode must pass through unchanged")
	}
}
