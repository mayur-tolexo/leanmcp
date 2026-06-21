package tokens

import "testing"

func TestEstimate(t *testing.T) {
	// ~4 chars/token heuristic.
	if got := Estimate([]byte("aaaaaaaa")); got != 2 { // 8/4
		t.Fatalf("Estimate = %d, want 2", got)
	}
	if got := Estimate([]byte("")); got != 0 {
		t.Fatalf("Estimate empty = %d, want 0", got)
	}
}

func TestExceeds(t *testing.T) {
	if !Exceeds([]byte("aaaaaaaaaaaaaaaa"), 3) { // 16/4 = 4 > 3
		t.Fatalf("expected exceeds")
	}
	if Exceeds([]byte("aaaa"), 3) { // 1 < 3
		t.Fatalf("did not expect exceeds")
	}
}
