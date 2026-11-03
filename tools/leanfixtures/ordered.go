package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Obj is a JSON object that remembers the order in which its keys were first set, in insertion order.
type Obj struct {
	keys []string
	vals map[string]any
}

// List is a JSON array with reference semantics, (a pointer to a slice).
type List struct{ items []any }

// IM is an object keyed by integers (only pathRows reads it).
type IM map[int]any

// O builds an object from key, value pairs.
func O(kv ...any) *Obj {
	o := &Obj{vals: map[string]any{}}
	for i := 0; i < len(kv); i += 2 {
		o.Set(kv[i].(string), kv[i+1])
	}
	return o
}

// L builds a list.
func L(items ...any) *List { return &List{items: append([]any{}, items...)} }

// Rng is the list of f(i) for i in [lo, hi).
func Rng(lo, hi int, f func(int) any) *List {
	l := L()
	for i := lo; i < hi; i++ {
		l.items = append(l.items, f(i))
	}
	return l
}

// MapL is the list of f(x) for each x of l.
func MapL(l *List, f func(any) any) *List {
	out := L()
	for _, x := range l.items {
		out.items = append(out.items, f(x))
	}
	return out
}

// Set sets a key, keeping its position when it already exists.
func (o *Obj) Set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *Obj) del(k string) {
	if _, ok := o.vals[k]; !ok {
		panic("no key " + k)
	}
	delete(o.vals, k)
	for i, x := range o.keys {
		if x == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func step(x any, idx any) any {
	switch c := x.(type) {
	case *Obj:
		v, ok := c.vals[idx.(string)]
		if !ok {
			panic(fmt.Sprintf("no key %v", idx))
		}
		return v
	case *List:
		return c.items[idx.(int)]
	}
	panic(fmt.Sprintf("cannot index %T", x))
}

// At follows a path of keys and indexes.
func At(x any, path ...any) any {
	for _, p := range path {
		x = step(x, p)
	}
	return x
}

// Put sets the element at the end of the path.
func Put(x any, v any, path ...any) {
	parent := At(x, path[:len(path)-1]...)
	switch c := parent.(type) {
	case *Obj:
		c.Set(path[len(path)-1].(string), v)
	case *List:
		c.items[path[len(path)-1].(int)] = v
	default:
		panic(fmt.Sprintf("cannot assign into %T", parent))
	}
}

// Del removes the key at the end of the path.
func Del(x any, path ...any) {
	At(x, path[:len(path)-1]...).(*Obj).del(path[len(path)-1].(string))
}

// Pop removes a key and returns its value.
func Pop(x any, k string) any {
	o := x.(*Obj)
	v := o.vals[k]
	o.del(k)
	return v
}

// Append adds an item to the list at the end of the path.
func Append(x any, v any, path ...any) {
	l := At(x, path...).(*List)
	l.items = append(l.items, v)
}

// Slice is the first n items of a list, as a new list.
func Slice(x any, n int) any { return L(x.(*List).items[:n]...) }

// Merge is a shallow copy of o with the pairs of over set on top.
func Merge(o any, over *Obj) any {
	src := o.(*Obj)
	out := &Obj{vals: map[string]any{}}
	for _, k := range src.keys {
		out.Set(k, src.vals[k])
	}
	for _, k := range over.keys {
		out.Set(k, over.vals[k])
	}
	return out
}

// DeepCopy copies objects and lists all the way down.
func DeepCopy(x any) any {
	switch c := x.(type) {
	case *Obj:
		out := &Obj{vals: map[string]any{}}
		for _, k := range c.keys {
			out.Set(k, DeepCopy(c.vals[k]))
		}
		return out
	case *List:
		out := L()
		for _, v := range c.items {
			out.items = append(out.items, DeepCopy(v))
		}
		return out
	}
	return x
}

func dumpsASCII(x any) string { return dumps(x, false) }

// dumps writes JSON indented by two spaces two-space indent, key order kept; with unicode false, non-ASCII is escaped.
func dumps(x any, unicode bool) string {
	var b strings.Builder
	dump(&b, x, 0, unicode)
	return b.String()
}

func dump(b *strings.Builder, x any, level int, unicode bool) {
	pad := strings.Repeat("  ", level+1)
	end := strings.Repeat("  ", level)
	switch c := x.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(c))
	case int:
		b.WriteString(strconv.Itoa(c))
	case string:
		quote(b, c, unicode)
	case *Obj:
		if len(c.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, k := range c.keys {
			if i > 0 {
				b.WriteString(",\n")
			}
			b.WriteString(pad)
			quote(b, k, unicode)
			b.WriteString(": ")
			dump(b, c.vals[k], level+1, unicode)
		}
		b.WriteString("\n" + end + "}")
	case *List:
		if len(c.items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, v := range c.items {
			if i > 0 {
				b.WriteString(",\n")
			}
			b.WriteString(pad)
			dump(b, v, level+1, unicode)
		}
		b.WriteString("\n" + end + "]")
	default:
		panic(fmt.Sprintf("cannot encode %T", x))
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
