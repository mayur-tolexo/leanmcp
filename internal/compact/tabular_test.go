package compact

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTabularArrayOfObjects(t *testing.T) {
	// Use enough rows so the column-header savings outweigh the wrapper overhead.
	in := []byte(`[{"id":"1","name":"alice"},{"id":"2","name":"bob"},{"id":"3","name":"carol"},{"id":"4","name":"dave"},{"id":"5","name":"eve"},{"id":"6","name":"frank"},{"id":"7","name":"grace"},{"id":"8","name":"heidi"},{"id":"9","name":"ivan"},{"id":"10","name":"judy"}]`)
	out, applied, err := Tabular(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !applied {
		t.Fatalf("expected tabular to apply")
	}
	if len(out) >= len(in) {
		t.Fatalf("expected compaction to shrink payload: %d >= %d", len(out), len(in))
	}
	var wrap struct {
		Table struct {
			Columns []string `json:"columns"`
			Rows    [][]any  `json:"rows"`
		} `json:"__leanmcp_table"`
	}
	if err := json.Unmarshal(out, &wrap); err != nil {
		t.Fatalf("output not valid table json: %v", err)
	}
	if len(wrap.Table.Columns) != 2 || len(wrap.Table.Rows) != 10 {
		t.Fatalf("unexpected shape: %+v", wrap.Table)
	}
}

func TestTabularLosslessRoundTrip(t *testing.T) {
	// Heterogeneous objects with a missing field, nested values, and mixed types
	// must all be reconstructable from the column/row form without loss.
	in := []byte(`[{"id":"1","tags":["a","b"],"score":9.5},{"id":"2","score":null,"extra":true}]`)
	out, applied, err := Tabular(in)
	if err != nil || !applied {
		t.Fatalf("expected tabular to apply: applied=%v err=%v", applied, err)
	}

	var wrap struct {
		Table struct {
			Columns []string `json:"columns"`
			Rows    [][]any  `json:"rows"`
		} `json:"__leanmcp_table"`
	}
	if err := json.Unmarshal(out, &wrap); err != nil {
		t.Fatalf("output not valid table json: %v", err)
	}

	// Reconstruct the original array of objects from columns + rows.
	got := make([]map[string]any, 0, len(wrap.Table.Rows))
	for _, row := range wrap.Table.Rows {
		obj := map[string]any{}
		for i, col := range wrap.Table.Columns {
			obj[col] = row[i]
		}
		got = append(got, obj)
	}

	// Compare against the original decoded with the same union-of-keys shape: a
	// missing key reconstructs as the JSON null that absence maps to in the table.
	var orig []map[string]any
	if err := json.Unmarshal(in, &orig); err != nil {
		t.Fatalf("bad input: %v", err)
	}
	cols := wrap.Table.Columns
	for i := range orig {
		for _, c := range cols {
			if _, ok := orig[i][c]; !ok {
				orig[i][c] = nil
			}
		}
	}
	if !reflect.DeepEqual(got, orig) {
		t.Fatalf("reconstruction mismatch:\n got=%v\nwant=%v", got, orig)
	}
}

func TestTabularNotApplicable(t *testing.T) {
	// A single object is not an array of uniform objects.
	if _, applied, _ := Tabular([]byte(`{"id":"1"}`)); applied {
		t.Fatalf("did not expect tabular to apply to a single object")
	}
	// An array of scalars is not uniform objects.
	if _, applied, _ := Tabular([]byte(`[1,2,3]`)); applied {
		t.Fatalf("did not expect tabular to apply to scalar array")
	}
}
