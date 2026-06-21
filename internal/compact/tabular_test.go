package compact

import (
	"encoding/json"
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
