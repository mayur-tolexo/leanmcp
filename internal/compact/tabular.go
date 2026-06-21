// Package compact provides lossless transforms that reduce the token cost of a
// JSON payload without dropping data.
package compact

import (
	"encoding/json"
	"sort"
)

// table is the lossless wire form for an array of uniform objects.
type table struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Tabular attempts to rewrite a JSON array of objects as a single column header
// plus rows, removing the per-row repetition of keys. It returns the rewritten
// bytes and applied=true on success; if the payload is not an array of objects
// it returns applied=false and the input unchanged.
func Tabular(raw []byte) ([]byte, bool, error) {
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil || len(arr) == 0 {
		return raw, false, nil
	}
	cols := unionColumns(arr)
	rows := make([][]any, 0, len(arr))
	for _, obj := range arr {
		row := make([]any, len(cols))
		for i, c := range cols {
			row[i] = obj[c] // missing key => nil; lossless against JSON null/absent
		}
		rows = append(rows, row)
	}
	out, err := json.Marshal(map[string]any{
		"__leanmcp_table": table{Columns: cols, Rows: rows},
	})
	if err != nil {
		return raw, false, err
	}
	return out, true, nil
}

// unionColumns returns the sorted union of keys across all objects so the table
// represents heterogeneous-but-similar objects without losing any field.
func unionColumns(arr []map[string]any) []string {
	seen := map[string]struct{}{}
	for _, obj := range arr {
		for k := range obj {
			seen[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}
