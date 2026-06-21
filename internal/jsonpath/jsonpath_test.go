package jsonpath

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	return v
}

func TestGetObjectField(t *testing.T) {
	v := decode(t, `{"data":{"price":{"monthly":9}}}`)
	got, err := Get(v, "data.price.monthly")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != float64(9) {
		t.Fatalf("got %v, want 9", got)
	}
}

func TestGetArrayIndex(t *testing.T) {
	v := decode(t, `{"data":[{"id":"a"},{"id":"b"}]}`)
	got, err := Get(v, "data[1].id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "b" {
		t.Fatalf("got %v, want b", got)
	}
}

func TestGetMissing(t *testing.T) {
	v := decode(t, `{"a":1}`)
	if _, err := Get(v, "a.b"); err == nil {
		t.Fatalf("expected error for missing path")
	}
}

func TestProject(t *testing.T) {
	v := decode(t, `{"id":"x","name":"n","secret":"s"}`)
	got := Project(v, []string{"id", "name"})
	want := map[string]any{"id": "x", "name": "n"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
