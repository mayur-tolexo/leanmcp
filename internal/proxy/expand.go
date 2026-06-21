package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mayur-tolexo/leanmcp/internal/jsonpath"
	"github.com/mayur-tolexo/leanmcp/store"
)

// ExpandArgs are the typed inputs to the expand_result tool.
type ExpandArgs struct {
	Handle string   `json:"handle"`
	Path   string   `json:"path,omitempty"`
	Fields []string `json:"fields,omitempty"`
	Offset int      `json:"offset,omitempty"`
	Limit  int      `json:"limit,omitempty"`
}

// expand loads the cached entry for args.Handle, requires it to be bound to
// credHash, applies path/fields selection, and returns the selected JSON string.
// A missing handle and a credential mismatch return the same error to avoid
// leaking handle existence to callers with a different credential.
func expand(ctx context.Context, s store.Store, credHash string, args ExpandArgs) (string, error) {
	e, err := s.Get(ctx, args.Handle)
	if err != nil {
		return "", fmt.Errorf("result expired or not found; re-run the original tool")
	}
	// Reject when the stored credential hash does not match the caller's hash.
	if e.CredHash != credHash {
		return "", fmt.Errorf("result expired or not found; re-run the original tool")
	}

	var doc any
	if err := json.Unmarshal(e.Raw, &doc); err != nil {
		return "", err
	}

	selected := doc
	if args.Path != "" {
		selected, err = jsonpath.Get(doc, args.Path)
		if err != nil {
			return "", err
		}
	}

	if len(args.Fields) > 0 {
		selected = jsonpath.Project(selected, args.Fields)
	}

	out, err := json.Marshal(selected)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
