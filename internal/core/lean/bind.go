package lean

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// The strict binder: JSON to a generic tree (jsontree.go), then reflection over the model, filling it and
// appending an Issue for every mistake instead of stopping at the first. Unknown keys, wrong types, closed vocabularies, ranges and
// missing required keys are all reported with their JSON path. Tags are described in model.go.

// Binder is implemented by (pointers to) types that read a polymorphic value: a roll that is a number or a range, a rating that is
// a number or an object. It is called for a present, non-null value; it reports its own errors with at as the path.
type Binder interface {
	BindJSON(v any, at string, r *Reporter)
}

var binderType = reflect.TypeOf((*Binder)(nil)).Elem()

// Bind decodes raw into dst (a pointer to the model) and reports every problem. It returns false when the bytes were not usable
// JSON at all (the report says why); true does not mean the report has no errors.
func Bind(file string, raw []byte, dst any, rep *Report) bool {
	r := NewReporter(file, rep)
	tree, ok := parseJSON(raw, r)
	if !ok {
		return false
	}
	bindTree(dst, tree, r)
	return true
}

func bindTree(dst any, tree any, r *Reporter) {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		panic("lean.Bind: dst must be a non-nil pointer")
	}
	bindValue(rv.Elem(), tree, "", r, nil)
}

// BindValue binds an already parsed value into dst (a pointer); Binder implementations use it for the parts they delegate.
func BindValue(dst any, v any, at string, r *Reporter) {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		panic("lean.BindValue: dst must be a non-nil pointer")
	}
	bindValue(rv.Elem(), v, at, r, nil)
}

// field is the parsed tag set of one struct field.
type field struct {
	index      int
	name       string
	required   bool
	omitempty  bool
	enum       []string
	noun       string
	min, max   *float64
	emin, emax *float64
	rest       bool
}

type structInfo struct {
	fields []field
	byName map[string]int
	rest   int // index of the rest map field, -1 none
	names  []string
}

var structCache sync.Map

func infoOf(t reflect.Type) *structInfo {
	if c, ok := structCache.Load(t); ok {
		return c.(*structInfo)
	}
	si := &structInfo{byName: map[string]int{}, rest: -1}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		tag := sf.Tag.Get("json")
		if tag == "" || !sf.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		f := field{index: i, name: name, omitempty: strings.Contains(opts, "omitempty"), required: sf.Tag.Get("required") == "true", noun: sf.Tag.Get("noun")}
		if sf.Tag.Get("rest") == "true" {
			si.rest = i
			continue
		}
		if name == "" || name == "-" {
			continue
		}
		if e := sf.Tag.Get("enum"); e != "" {
			f.enum = strings.Split(e, "|")
		}
		f.min, f.max = numTag(sf, "min"), numTag(sf, "max")
		f.emin, f.emax = numTag(sf, "emin"), numTag(sf, "emax")
		si.byName[name] = len(si.fields)
		si.fields = append(si.fields, f)
		si.names = append(si.names, name)
	}
	structCache.Store(t, si)
	return si
}

func numTag(sf reflect.StructField, key string) *float64 {
	s, ok := sf.Tag.Lookup(key)
	if !ok {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(fmt.Sprintf("lean: bad %s tag %q on %s", key, s, sf.Name))
	}
	return &f
}

// bindValue fills dst from v. f carries the tags of the field being bound (nil for the root and for list elements, whose element
// tags are passed down as a derived field).
func bindValue(dst reflect.Value, v any, at string, r *Reporter, f *field) {
	t := dst.Type()
	if dst.CanAddr() && reflect.PointerTo(t).Implements(binderType) {
		dst.Addr().Interface().(Binder).BindJSON(v, at, r)
		return
	}
	switch t.Kind() {
	case reflect.Pointer:
		n := reflect.New(t.Elem())
		bindValue(n.Elem(), v, at, r, f)
		dst.Set(n)
	case reflect.Interface:
		dst.Set(reflect.ValueOf(v))
	case reflect.String:
		s, ok := v.(string)
		if !ok {
			typeErr(r, at, "a string", v)
			return
		}
		checkString(s, at, r, f)
		dst.SetString(s)
	case reflect.Bool:
		b, ok := v.(bool)
		if !ok {
			typeErr(r, at, "true or false", v)
			return
		}
		dst.SetBool(b)
	case reflect.Int, reflect.Int64:
		n, ok := v.(json.Number)
		if !ok {
			typeErr(r, at, "a whole number", v)
			return
		}
		i, err := strconv.ParseInt(n.String(), 10, 64)
		if err != nil {
			if fl, e2 := strconv.ParseFloat(n.String(), 64); e2 == nil && fl == math.Trunc(fl) && math.Abs(fl) < 1e15 {
				i, err = int64(fl), nil
			}
		}
		if err != nil {
			r.Errorf(at, "%s is not a whole number", n.String())
			return
		}
		if !checkRange(float64(i), n.String(), at, r, f) {
			return
		}
		dst.SetInt(i)
	case reflect.Float64:
		n, ok := v.(json.Number)
		if !ok {
			typeErr(r, at, "a number", v)
			return
		}
		fl, err := strconv.ParseFloat(n.String(), 64)
		if err != nil || math.IsInf(fl, 0) {
			r.Errorf(at, "%s is not a usable number", n.String())
			return
		}
		if !checkRange(fl, n.String(), at, r, f) {
			return
		}
		dst.SetFloat(fl)
	case reflect.Slice:
		list, ok := v.([]any)
		if !ok {
			typeErr(r, at, "a list", v)
			return
		}
		checkLen(len(list), "item", at, r, f)
		if len(list) == 0 {
			return // an empty list is the zero value, so Encode and Bind round trip
		}
		out := reflect.MakeSlice(t, len(list), len(list))
		var ef *field
		if f != nil {
			ef = &field{enum: f.enum, noun: f.noun, min: f.emin, max: f.emax}
		}
		for i, e := range list {
			if e == nil {
				r.Errorf(Idx(at, i), "must not be null")
				continue
			}
			bindValue(out.Index(i), e, Idx(at, i), r, ef)
		}
		dst.Set(out)
	case reflect.Map:
		obj, ok := v.(map[string]any)
		if !ok {
			typeErr(r, at, "an object", v)
			return
		}
		if len(obj) == 0 {
			return
		}
		out := reflect.MakeMapWithSize(t, len(obj))
		for _, k := range SortedKeys(obj) {
			if obj[k] == nil {
				r.Errorf(Join(at, k), "must not be null")
				continue
			}
			e := reflect.New(t.Elem()).Elem()
			bindValue(e, obj[k], Join(at, k), r, nil)
			out.SetMapIndex(reflect.ValueOf(k), e)
		}
		dst.Set(out)
	case reflect.Struct:
		bindStruct(dst, v, at, r)
	default:
		panic("lean: unsupported model kind " + t.Kind().String())
	}
}

func bindStruct(dst reflect.Value, v any, at string, r *Reporter) {
	obj, ok := v.(map[string]any)
	if !ok {
		typeErr(r, at, "an object", v)
		return
	}
	si := infoOf(dst.Type())
	for i := range si.fields {
		f := &si.fields[i]
		val, present := obj[f.name]
		switch {
		case !present:
			if f.required {
				r.Errorf(Join(at, f.name), "missing")
			}
		case val == nil:
			if f.required {
				r.Errorf(Join(at, f.name), "must not be null; it is required")
			}
		default:
			bindValue(dst.Field(f.index), val, Join(at, f.name), r, f)
		}
	}
	var unknown []string
	for _, k := range SortedKeys(obj) {
		if _, known := si.byName[k]; !known {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return
	}
	if si.rest >= 0 {
		m := reflect.MakeMap(dst.Field(si.rest).Type())
		for _, k := range unknown {
			m.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(obj).MapIndex(reflect.ValueOf(k)))
		}
		dst.Field(si.rest).Set(m)
		return
	}
	known := append([]string(nil), si.names...)
	sort.Strings(known)
	for _, k := range unknown {
		iss := Issue{Path: Join(at, k), Message: "unknown field", Hint: "known fields: " + strings.Join(known, ", ")}
		if c := Closest(known, k); c != "" {
			iss.Message += fmt.Sprintf("; did you mean %q?", c)
			iss.Closest = c
		}
		r.Add(iss)
	}
}

func typeErr(r *Reporter, at, want string, got any) {
	r.Errorf(at, "expected %s, got %s", want, describe(got))
}

func describe(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case bool:
		return "true or false"
	case json.Number:
		return "a number"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	}
	return fmt.Sprintf("%T", v)
}

func nounOf(at string, f *field) string {
	if f != nil && f.noun != "" {
		return f.noun
	}
	// the last word of the path, without a trailing index
	name := at
	for strings.HasSuffix(name, "]") && !strings.HasSuffix(name, `"]`) {
		name = name[:strings.LastIndex(name, "[")]
	}
	if i := strings.LastIndexAny(name, ".["); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, "]")
	if name == "" || strings.ContainsAny(name, "0123456789\"") {
		return "value"
	}
	return name
}

func checkString(s, at string, r *Reporter, f *field) {
	if f == nil {
		return
	}
	if f.min != nil && float64(len([]rune(s))) < *f.min {
		if *f.min == 1 {
			r.Errorf(at, "must not be empty")
		} else {
			r.Errorf(at, "must have at least %d characters", int(*f.min))
		}
		return
	}
	if f.max != nil && float64(len([]rune(s))) > *f.max {
		r.Errorf(at, "must have at most %d characters", int(*f.max))
		return
	}
	if len(f.enum) > 0 {
		for _, e := range f.enum {
			if e == s {
				return
			}
		}
		iss := Issue{Path: at, Message: fmt.Sprintf("%q is not a valid %s; known: %s", s, nounOf(at, f), strings.Join(f.enum, ", "))}
		if c := Closest(f.enum, s); c != "" {
			iss.Closest = c
		}
		r.Add(iss)
	}
}

func checkRange(x float64, text, at string, r *Reporter, f *field) bool {
	if f == nil {
		return true
	}
	lo, hi := f.min, f.max
	switch {
	case lo != nil && hi != nil && (x < *lo || x > *hi):
		r.Errorf(at, "%s is out of range %s..%s", text, fmtNum(*lo), fmtNum(*hi))
	case lo != nil && x < *lo:
		r.Errorf(at, "%s is below the minimum %s", text, fmtNum(*lo))
	case hi != nil && x > *hi:
		r.Errorf(at, "%s is above the maximum %s", text, fmtNum(*hi))
	default:
		return true
	}
	return false
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func checkLen(n int, noun, at string, r *Reporter, f *field) {
	if f == nil {
		return
	}
	lo, hi := f.min, f.max
	switch {
	case lo != nil && hi != nil && *lo == *hi && float64(n) != *lo:
		r.Errorf(at, "needs exactly %s, has %d", plural(int(*lo), noun), n)
	case lo != nil && float64(n) < *lo:
		r.Errorf(at, "needs at least %s, has %d", plural(int(*lo), noun), n)
	case hi != nil && float64(n) > *hi:
		r.Errorf(at, "allows at most %s, has %d", plural(int(*hi), noun), n)
	}
}
