package proxy

import (
	"encoding/base64"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func tools(names ...string) []*mcp.Tool {
	out := make([]*mcp.Tool, len(names))
	for i, n := range names {
		out[i] = &mcp.Tool{Name: n}
	}
	return out
}

func TestPaginateFirstPage(t *testing.T) {
	page, next, err := paginate(tools("a", "b", "c", "d"), "", 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(page) != 2 || page[0].Name != "a" || page[1].Name != "b" {
		t.Fatalf("bad page: %+v", page)
	}
	if next == "" {
		t.Fatalf("expected a next cursor")
	}
}

func TestPaginateSecondPageAndEnd(t *testing.T) {
	_, next, _ := paginate(tools("a", "b", "c", "d"), "", 2)
	page, next2, err := paginate(tools("a", "b", "c", "d"), next, 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(page) != 2 || page[0].Name != "c" {
		t.Fatalf("bad second page: %+v", page)
	}
	if next2 != "" {
		t.Fatalf("expected end of pagination, got %q", next2)
	}
}

func TestPaginateZeroPageSizeReturnsAll(t *testing.T) {
	page, next, _ := paginate(tools("a", "b", "c"), "", 0)
	if len(page) != 3 || next != "" {
		t.Fatalf("zero page size should return all with no cursor")
	}
}

func TestPaginateNegativeCursorOffsetRejected(t *testing.T) {
	// A cursor decoding to a negative offset must error, not panic on a bad slice.
	cursor := base64.StdEncoding.EncodeToString([]byte("-1"))
	if _, _, err := paginate(tools("a", "b"), cursor, 1); err == nil {
		t.Fatalf("expected error for negative cursor offset")
	}
}

func TestPaginateGarbageCursorRejected(t *testing.T) {
	// Non-base64 cursor input must error rather than panic.
	if _, _, err := paginate(tools("a", "b"), "!!!not-base64!!!", 1); err == nil {
		t.Fatalf("expected error for non-base64 cursor")
	}
}
