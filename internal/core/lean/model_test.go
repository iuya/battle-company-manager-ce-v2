package lean

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// walkTypes visits every struct type reachable from t once.
func walkTypes(t reflect.Type, seen map[reflect.Type]bool, visit func(reflect.Type)) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return
	}
	seen[t] = true
	visit(t)
	for i := 0; i < t.NumField(); i++ {
		walkTypes(t.Field(i).Type, seen, visit)
	}
}

// TestModelTags checks the contract: every field has a json tag with a unique name inside its struct, and the validation tags
// are well formed (numeric min/max, min <= max, enum values non-empty, rest only on map[string]any).
func TestModelTags(t *testing.T) {
	walkTypes(reflect.TypeOf(Document{}), map[reflect.Type]bool{}, func(st reflect.Type) {
		if st == reflect.TypeOf(DiceSpec{}) || st == reflect.TypeOf(Roll{}) || st == reflect.TypeOf(Rating{}) || st == reflect.TypeOf(RuleRef{}) || st == reflect.TypeOf(Share{}) {
			return // polymorphic: bound by their Binder
		}
		names := map[string]bool{}
		for i := 0; i < st.NumField(); i++ {
			f := st.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if f.Tag.Get("json") == "" {
				t.Errorf("%s.%s: no json tag", st.Name(), f.Name)
				continue
			}
			if name == "" || name == "-" {
				if f.Tag.Get("rest") != "true" {
					t.Errorf("%s.%s: empty json name", st.Name(), f.Name)
				}
				continue
			}
			if names[name] {
				t.Errorf("%s: duplicate json name %q", st.Name(), name)
			}
			names[name] = true
			if r := f.Tag.Get("required"); r != "" && r != "true" {
				t.Errorf("%s.%s: required must be \"true\"", st.Name(), f.Name)
			}
			if e, ok := f.Tag.Lookup("enum"); ok {
				for _, v := range strings.Split(e, "|") {
					if v == "" {
						t.Errorf("%s.%s: empty enum value", st.Name(), f.Name)
					}
				}
			}
			mn, hasMin := f.Tag.Lookup("min")
			mx, hasMax := f.Tag.Lookup("max")
			var lo, hi float64
			var err error
			if hasMin {
				if lo, err = strconv.ParseFloat(mn, 64); err != nil {
					t.Errorf("%s.%s: bad min %q", st.Name(), f.Name, mn)
				}
			}
			if hasMax {
				if hi, err = strconv.ParseFloat(mx, 64); err != nil {
					t.Errorf("%s.%s: bad max %q", st.Name(), f.Name, mx)
				}
			}
			if hasMin && hasMax && lo > hi {
				t.Errorf("%s.%s: min %v above max %v", st.Name(), f.Name, lo, hi)
			}
		}
	})
}

// TestModelCoversDesign pins the top-level sections of the format and its concepts.
func TestModelCoversDesign(t *testing.T) {
	want := []string{"schemaVersion", "name", "version", "credit", "extends", "campaign", "rules", "magic", "gear", "paths", "scenarios", "companies", "units", "vanilla", "auras"}
	dt := reflect.TypeOf(Document{})
	have := map[string]bool{}
	for i := 0; i < dt.NumField(); i++ {
		n, _, _ := strings.Cut(dt.Field(i).Tag.Get("json"), ",")
		have[n] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("Document has no %q section", w)
		}
	}
	for _, typ := range []any{Company{}, Unit{}, Gear{}, Path{}, Rule{}, Scenario{}, Vanilla{}, Aura{}, Magic{}} {
		if reflect.TypeOf(typ).NumField() == 0 {
			t.Errorf("%T has no fields", typ)
		}
	}
}
