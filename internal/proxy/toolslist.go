package proxy

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// paginate returns the slice of tools starting at the offset encoded in cursor,
// limited to pageSize (0 = all), plus the next cursor ("" when exhausted).
// Cursors are base64-encoded decimal offsets, independent of any upstream cursor.
func paginate(all []*mcp.Tool, cursor string, pageSize int) ([]*mcp.Tool, string, error) {
	offset := 0
	if cursor != "" {
		b, err := base64.StdEncoding.DecodeString(cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		offset, err = strconv.Atoi(string(b))
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor offset value: %w", err)
		}
		// A negative offset can only come from a corrupted or forged cursor;
		// reject it rather than panic on a negative slice bound below.
		if offset < 0 {
			return nil, "", fmt.Errorf("invalid cursor: negative offset %d", offset)
		}
	}

	// Clamp offset so it never exceeds the list length.
	if offset > len(all) {
		offset = len(all)
	}

	// pageSize <= 0 means return everything from offset onward with no next cursor.
	if pageSize <= 0 {
		return all[offset:], "", nil
	}

	end := offset + pageSize
	// When end reaches or exceeds the slice end, return the remainder and signal
	// that pagination is exhausted by returning an empty cursor.
	if end >= len(all) {
		return all[offset:], "", nil
	}

	// Encode the next start position as the cursor for the following page.
	next := base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(end)))
	return all[offset:end], next, nil
}
