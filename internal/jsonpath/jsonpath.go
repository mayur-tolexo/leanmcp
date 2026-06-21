// Package jsonpath provides minimal dot-path access and field projection over
// JSON values decoded into any (map[string]any / []any / scalars). It supports
// segments like "data" and "data[3].field"; no wildcards or filters.
package jsonpath

import (
	"fmt"
	"strconv"
	"strings"
)

// Get returns the value at the dot-path within v, or an error if any segment is
// missing or the path shape does not match the data.
func Get(v any, path string) (any, error) {
	if path == "" || path == "." {
		return v, nil
	}
	cur := v
	for _, seg := range strings.Split(path, ".") {
		name, indices, err := parseSegment(seg)
		if err != nil {
			return nil, fmt.Errorf("path %q: %w", path, err)
		}
		if name != "" {
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("path %q: %q is not an object", path, name)
			}
			child, ok := m[name]
			if !ok {
				return nil, fmt.Errorf("path %q: key %q not found", path, name)
			}
			cur = child
		}
		for _, idx := range indices {
			arr, ok := cur.([]any)
			if !ok {
				return nil, fmt.Errorf("path %q: not an array at index %d", path, idx)
			}
			if idx < 0 || idx >= len(arr) {
				return nil, fmt.Errorf("path %q: index %d out of range", path, idx)
			}
			cur = arr[idx]
		}
	}
	return cur, nil
}

// Project returns a new map containing only the named top-level dot-paths that
// exist in v; missing paths are skipped. Used for opt-in field whitelisting.
func Project(v any, fields []string) map[string]any {
	out := map[string]any{}
	for _, f := range fields {
		if got, err := Get(v, f); err == nil {
			out[f] = got
		}
	}
	return out
}

// parseSegment splits a segment like "data[1][2]" into its name and indices.
// A malformed bracket group (empty or non-numeric) is an error rather than a
// silently dropped index, which would otherwise resolve to the wrong node.
func parseSegment(seg string) (string, []int, error) {
	name := seg
	var indices []int
	if i := strings.IndexByte(seg, '['); i >= 0 {
		name = seg[:i]
		for _, part := range strings.Split(seg[i:], "[") {
			// The first split element is the text before the first '[' (empty here).
			if part == "" {
				continue
			}
			if !strings.HasSuffix(part, "]") {
				return "", nil, fmt.Errorf("malformed index segment %q", seg)
			}
			part = strings.TrimSpace(strings.TrimSuffix(part, "]"))
			n, err := strconv.Atoi(part)
			if err != nil {
				return "", nil, fmt.Errorf("invalid array index %q", part)
			}
			indices = append(indices, n)
		}
	}
	return name, indices, nil
}
