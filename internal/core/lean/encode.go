package lean

import (
	"encoding/json"
	"reflect"
)

// Encode writes a model value back as JSON in the lean format: keys by their json tags (sorted by encoding/json), fields marked
// omitempty left out when empty, and the keys held in `rest` maps written back. It is the inverse of Bind for a document that bound
// without errors, which the fuzz test relies on.
func Encode(v any) ([]byte, error) {
	return json.Marshal(encodeTree(reflect.ValueOf(v)))
}

func reflectOf(v any) reflect.Value { return reflect.ValueOf(v) }

func reflectInt(p *int) reflect.Value { return reflect.ValueOf(p).Elem() }

func encodeTree(rv reflect.Value) any {
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.CanInterface() {
		if m, ok := rv.Interface().(json.Marshaler); ok {
			return m
		}
	}
	switch rv.Kind() {
	case reflect.Struct:
		si := infoOf(rv.Type())
		out := map[string]any{}
		for _, f := range si.fields {
			fv := rv.Field(f.index)
			if f.omitempty && isEmpty(fv) {
				continue
			}
			out[f.name] = encodeTree(fv)
		}
		if si.rest >= 0 {
			for _, k := range rv.Field(si.rest).MapKeys() {
				out[k.String()] = rv.Field(si.rest).MapIndex(k).Interface()
			}
		}
		return out
	case reflect.Slice:
		list := make([]any, rv.Len())
		for i := range list {
			list[i] = encodeTree(rv.Index(i))
		}
		return list
	case reflect.Map:
		out := map[string]any{}
		for _, k := range rv.MapKeys() {
			out[k.String()] = encodeTree(rv.MapIndex(k))
		}
		return out
	}
	return rv.Interface()
}

func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return v.IsNil() || (v.Kind() != reflect.Pointer && v.Kind() != reflect.Interface && v.Len() == 0)
	case reflect.Struct:
		return v.IsZero()
	}
	return v.IsZero()
}
