package lean

import (
	"reflect"
	"strings"
	"testing"
)

// fieldAt resolves a ParameterIDs path ("campaign.x[].y") against the model and returns the field.
func fieldAt(t *testing.T, path string) (reflect.StructField, bool) {
	t.Helper()
	cur := reflect.TypeOf(Document{})
	var f reflect.StructField
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSuffix(part, "[]")
		for cur.Kind() == reflect.Pointer || cur.Kind() == reflect.Slice {
			cur = cur.Elem()
		}
		found := false
		for i := 0; i < cur.NumField(); i++ {
			if n, _, _ := strings.Cut(cur.Field(i).Tag.Get("json"), ","); n == part {
				f, found = cur.Field(i), true
				cur = f.Type
				break
			}
		}
		if !found {
			return f, false
		}
	}
	return f, true
}

// TestParameterIDs: every parameter id of design 12.2 appears exactly once and resolves to a field of the model; the engine
// parameters under campaign are required.
func TestParameterIDs(t *testing.T) {
	want := []string{"A1", "A2", "A3", "A4", "A7", "A8", "A9", "B1", "B3", "B4", "B6", "B7", "B8", "B9", "B10", "C1", "C2", "C3", "C4", "C5", "D1", "D2", "D3", "E1", "E4", "E6", "F1", "F5", "G3"}
	if len(ParameterIDs) != len(want) {
		t.Errorf("%d ids, want %d", len(ParameterIDs), len(want))
	}
	seenPath := map[string]string{}
	for _, id := range want {
		path, ok := ParameterIDs[id]
		if !ok {
			t.Errorf("parameter %s is missing", id)
			continue
		}
		if other, dup := seenPath[path]; dup {
			t.Errorf("%s and %s share the path %s", id, other, path)
		}
		seenPath[path] = id
		f, found := fieldAt(t, path)
		if !found {
			t.Errorf("%s: no field at %s", id, path)
			continue
		}
		if strings.HasPrefix(path, "campaign.") && id != "A7" && id != "D2" && id != "D3" && f.Tag.Get("required") != "true" {
			t.Errorf("%s: %s must be required (no code default)", id, path)
		}
	}
}
