// Package jsonx holds the small helpers the rules tools use to read JSON documents loosely, without a Go type per file: a missing key
// reads as the zero value (the schemas and the app's own validator are what insist on keys), numbers keep their written form, and
// values print in the repr style (single-quoted texts, True/False/None), which the messages and generated files use.
package jsonx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Obj is a decoded JSON object.
type Obj = map[string]any

// Arr is a decoded JSON array.
type Arr = []any

// Decode parses JSON; numbers are json.Number so their text survives.
func Decode(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// ReadFile reads and decodes a JSON file.
func ReadFile(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	v, err := Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

// O is v as an object, or nil.
func O(v any) Obj {
	m, _ := v.(Obj)
	return m
}

// A is v as an array, or nil.
func A(v any) Arr {
	a, _ := v.(Arr)
	return a
}

// S is v as a string, or "".
func S(v any) string {
	s, _ := v.(string)
	return s
}

// Strs is the strings of an array (other elements are left out).
func Strs(v any) []string {
	var out []string
	for _, x := range A(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Float is a JSON number as a float (0 for anything else).
func Float(v any) float64 {
	switch n := v.(type) {
	case json.Number:
		f, _ := n.Float64()
		return f
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

// IsNum reports whether v is a number.
func IsNum(v any) bool {
	switch v.(type) {
	case json.Number, float64, int:
		return true
	}
	return false
}

// Truthy reports whether a value counts as set: null, false, 0, "", [] and {} are false.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		return Float(x) != 0
	case Arr:
		return len(x) > 0
	case Obj:
		return len(x) > 0
	}
	return true
}

// Slice is the first n characters (runes) of s.
func Slice(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Len is the number of characters (runes) of s.
func Len(s string) int { return len([]rune(s)) }

// ReprString quotes a string in the repr style: single quotes unless the text has one and no double quote.
func ReprString(s string) string {
	q := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = '"'
	}
	var b strings.Builder
	b.WriteByte(q)
	for _, r := range s {
		switch {
		case r == rune(q) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x7f || unicode.IsPrint(r):
			b.WriteRune(r)
		case r <= 0xff:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r <= 0xffff:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteByte(q)
	return b.String()
}

// Repr is repr() of a decoded value (object keys in sorted order).
func Repr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case string:
		return ReprString(x)
	case json.Number:
		return x.String()
	case Arr:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = Repr(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case Obj:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = ReprString(k) + ": " + Repr(x[k])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprint(v)
}

// Str is str() of a decoded value: a string as it is, anything else as its repr.
func Str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return Repr(v)
}

// Equal compares decoded values by value (1 equals 1.0).
func Equal(a, b any) bool {
	if IsNum(a) && IsNum(b) {
		return Float(a) == Float(b)
	}
	switch x := a.(type) {
	case Arr:
		y, ok := b.(Arr)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !Equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case Obj:
		y, ok := b.(Obj)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !Equal(v, w) {
				return false
			}
		}
		return true
	}
	return a == b
}

// KV is one key of an ordered object.
type KV struct {
	Key string
	Val any
}

// Ordered is an object whose keys keep their order when written.
type Ordered []KV

// Marshal writes v as JSON with a two-space indent, "key": value, non-ASCII kept.
// It knows Ordered, Obj (sorted keys), Arr, []string, []Ordered, []any, string, bool, nil, int and json.Number.
func Marshal(v any) []byte {
	var b bytes.Buffer
	write(&b, v, 0)
	return b.Bytes()
}

func quote(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

func write(b *bytes.Buffer, v any, depth int) {
	pad := func(d int) { b.WriteString(strings.Repeat("  ", d)) }
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case int:
		b.WriteString(strconv.Itoa(x))
	case json.Number:
		b.WriteString(x.String())
	case string:
		quote(b, x)
	case []string:
		a := make(Arr, len(x))
		for i, s := range x {
			a[i] = s
		}
		write(b, a, depth)
	case []Ordered:
		a := make(Arr, len(x))
		for i, s := range x {
			a[i] = s
		}
		write(b, a, depth)
	case Arr:
		if len(x) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, e := range x {
			pad(depth + 1)
			write(b, e, depth+1)
			if i < len(x)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		pad(depth)
		b.WriteByte(']')
	case Obj:
		kv := make(Ordered, 0, len(x))
		for k, e := range x {
			kv = append(kv, KV{k, e})
		}
		sort.Slice(kv, func(i, j int) bool { return kv[i].Key < kv[j].Key })
		write(b, kv, depth)
	case Ordered:
		if len(x) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, e := range x {
			pad(depth + 1)
			quote(b, e.Key)
			b.WriteString(": ")
			write(b, e.Val, depth+1)
			if i < len(x)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		pad(depth)
		b.WriteByte('}')
	default:
		panic(fmt.Sprintf("jsonx.Marshal: unsupported %T", v))
	}
}

// Number is a JSON number given by its text.
func Number(s string) json.Number { return json.Number(s) }
