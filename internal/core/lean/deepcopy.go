package lean

import (
	"encoding/json"
	"reflect"
)

// deepCopy returns an independent copy of v (structs, slices, maps, pointers and the generic JSON tree held by Free fields).
func deepCopy(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		n := reflect.New(v.Type().Elem())
		n.Elem().Set(deepCopy(v.Elem()))
		return n
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		n := reflect.New(v.Type()).Elem()
		n.Set(reflect.ValueOf(copyTree(v.Interface())))
		return n
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		n := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			n.Index(i).Set(deepCopy(v.Index(i)))
		}
		return n
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		n := reflect.MakeMapWithSize(v.Type(), v.Len())
		for _, k := range v.MapKeys() {
			n.SetMapIndex(k, deepCopy(v.MapIndex(k)))
		}
		return n
	case reflect.Struct:
		n := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if n.Field(i).CanSet() {
				n.Field(i).Set(deepCopy(v.Field(i)))
			}
		}
		return n
	}
	return v
}

// copyTree copies a generic JSON tree.
func copyTree(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[k] = copyTree(e)
		}
		return m
	case []any:
		l := make([]any, len(x))
		for i, e := range x {
			l[i] = copyTree(e)
		}
		return l
	case json.Number, string, bool, nil:
		return x
	}
	return v
}
