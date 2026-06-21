package proxy

import (
	"strings"
	"testing"
)

func TestTrailerMentionsHandleAndExpand(t *testing.T) {
	tr := Trailer("r_123", 412, 20)
	if !strings.Contains(tr, "r_123") {
		t.Fatalf("trailer missing handle: %q", tr)
	}
	if !strings.Contains(tr, "expand_result") {
		t.Fatalf("trailer missing expand_result hint: %q", tr)
	}
	if !strings.Contains(tr, "412") {
		t.Fatalf("trailer missing total count: %q", tr)
	}
}
