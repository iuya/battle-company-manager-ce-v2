package rulesref

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// obj is a JSON object that keeps the order of its keys, as the schema file wrote them: the reference lists some keys in that order.
type obj struct {
	keys []string
	vals map[string]any
}

// Values are nil, bool, string, json.Number (the written text of the number), []any and *obj.

func (o *obj) get(k string) (any, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.vals[k]
	return v, ok
}

func (o *obj) has(k string) bool { _, ok := o.get(k); return ok }

// val is the value of a key, or nil.
func (o *obj) val(k string) any { v, _ := o.get(k); return v }

func (o *obj) str(k string) string { s, _ := o.val(k).(string); return s }

func (o *obj) obj(k string) *obj { x, _ := o.val(k).(*obj); return x }

func (o *obj) list(k string) []any { x, _ := o.val(k).([]any); return x }

func (o *obj) set(k string, v any) {
	if o.vals == nil {
		o.vals = map[string]any{}
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func parse(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := value(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after the JSON value")
	}
	return v, nil
}

func value(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	if d == '{' {
		o := &obj{vals: map[string]any{}}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := value(dec)
			if err != nil {
				return nil, err
			}
			o.set(kt.(string), v)
		}
		_, err := dec.Token()
		return o, err
	}
	arr := []any{}
	for dec.More() {
		v, err := value(dec)
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
	}
	_, err = dec.Token()
	return arr, err
}

// scalarStr is the plain text of a scalar (a whole number prints without a fraction, true and false in lower case).
func scalarStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case bool:
		if x {
			return "True"
		}
		return "False"
	case string:
		return x
	case json.Number:
		return x.String()
	}
	panic(fmt.Sprintf("scalarStr: unsupported %T", v))
}

// dumps writes JSON with indent two spaces and unicode kept, or compact with non-ASCII escaped.
func dumps(v any, indent, unicode bool) string {
	var b strings.Builder
	dump(&b, v, 0, indent, unicode)
	return b.String()
}

func dump(b *strings.Builder, v any, level int, indent, unicode bool) {
	nl, pad, end, itemSep, keySep := "", "", "", ", ", ": "
	if indent {
		nl, pad, end, itemSep = "\n", strings.Repeat("  ", level+1), strings.Repeat("  ", level), ","
	}
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		quote(b, x, unicode)
	case json.Number:
		b.WriteString(x.String())
	case *obj:
		if len(x.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{" + nl)
		for i, k := range x.keys {
			if i > 0 {
				b.WriteString(itemSep + nl)
			}
			b.WriteString(pad)
			quote(b, k, unicode)
			b.WriteString(keySep)
			dump(b, x.vals[k], level+1, indent, unicode)
		}
		b.WriteString(nl + end + "}")
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[" + nl)
		for i, e := range x {
			if i > 0 {
				b.WriteString(itemSep + nl)
			}
			b.WriteString(pad)
			dump(b, e, level+1, indent, unicode)
		}
		b.WriteString(nl + end + "]")
	default:
		panic(fmt.Sprintf("dump: unsupported %T", v))
	}
}

func quote(b *strings.Builder, s string, unicode bool) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || (!unicode && r > 0x7e):
			if r > 0xffff {
				r -= 0x10000
				fmt.Fprintf(b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
			} else {
				fmt.Fprintf(b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
