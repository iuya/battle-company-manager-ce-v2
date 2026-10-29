package core

import (
	"reflect"
	"testing"
)

// A campaign keeps its rules file, but names stored in a company (a unit, an option, an item) can stop existing when the file is
// replaced by a later one, and players type some names themselves. Every lookup of the rule books that takes only strings, numbers
// and booleans must answer "not found" for a name that does not exist, never panic.
func TestRuleBookLookupsSurviveUnknownNames(t *testing.T) {
	rs := shippedRules(t)
	b, v := rs.BC, rs.Vanilla
	args := [][]string{{"", "", ""}, {"No Such Name", "No Such Option", "No Such Item"}, {"\x00", "ß", "%s%d"}}
	called := 0
	for _, target := range []any{b, v, rs} {
		rt := reflect.TypeOf(target)
		for i := 0; i < rt.NumMethod(); i++ {
			m := rt.Method(i)
			in := []reflect.Type{}
			for j := 1; j < m.Type.NumIn(); j++ {
				in = append(in, m.Type.In(j))
			}
			if m.Type.IsVariadic() {
				continue
			}
			for _, set := range args {
				vals := []reflect.Value{reflect.ValueOf(target)}
				ok, si := true, 0
				for _, p := range in {
					switch p.Kind() {
					case reflect.String:
						vals = append(vals, reflect.ValueOf(set[si%3]).Convert(p))
						si++
					case reflect.Int, reflect.Int64:
						vals = append(vals, reflect.ValueOf(-1).Convert(p))
					case reflect.Bool:
						vals = append(vals, reflect.ValueOf(false))
					default:
						ok = false
					}
				}
				if !ok {
					break
				}
				func() {
					defer func() {
						if p := recover(); p != nil {
							t.Errorf("%s.%s%q panicked: %v", rt, m.Name, set, p)
						}
					}()
					m.Func.Call(vals)
					called++
				}()
			}
		}
	}
	if called < 30 {
		t.Errorf("only %d lookups were tried: did the books change shape?", called)
	}
}
