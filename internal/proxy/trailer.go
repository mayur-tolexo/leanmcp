// Package proxy assembles the MCP proxy server and its tool handlers.
package proxy

import "fmt"

// Trailer returns a short machine-readable note appended to a compacted result
// so the model knows it can retrieve the full payload via expand_result.
// handle is the cache key; totalItems is the untruncated count; shownItems is
// the count visible in the compacted view.
func Trailer(handle string, totalItems, shownItems int) string {
	return fmt.Sprintf(
		"\n[leanmcp: compacted losslessly; showing %d of %d items]\n"+
			"[expand: expand_result(handle=%q) for the full result; "+
			"add path=\"data[0]\" for a single record or offset/limit to page]",
		shownItems, totalItems, handle)
}
