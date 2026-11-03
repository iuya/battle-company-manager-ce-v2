// Package rulesref writes docs/customizing-rulesets-reference.md from definitions/battle_companies/battle_companies.schema.json.
//
// The schema is generated from the Go model (internal/core/lean), and its descriptions live in internal/core/lean/schema_docs_test.go, so
// the reference lists exactly what the app accepts: every object and key with its type, whether it is required, its default, its allowed
// values or range, and what it means. The output is deterministic: the same schema gives the same bytes.
package rulesref

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// formsOrder lists the value forms that read several shapes: described once, linked from the keys that use them.
var formsOrder = []string{"Roll", "DiceSpec", "Rating", "RuleRef", "AdvanceTo", "Scalar"}

//go:embed header.txt
var header string

// Render is the reference for a schema document.
func Render(schemaJSON []byte) (text string, err error) {
	v, err := parse(schemaJSON)
	if err != nil {
		return "", err
	}
	schema, ok := v.(*obj)
	if !ok || schema.obj("$defs") == nil {
		return "", fmt.Errorf("the schema has no $defs object")
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the schema does not have the shape the reference needs: %v", r)
		}
	}()
	return render(schema), nil
}

var sentenceEnd = regexp.MustCompile(` +`)

// sentence is a description that ends in a full stop.
func sentence(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || strings.ContainsRune(".!?:", rune(text[len(text)-1])) {
		return text
	}
	return text + "."
}

// splitSentences splits after [.!?] where spaces follow.
func splitSentences(text string) []string {
	var parts []string
	last := 0
	for _, loc := range sentenceEnd.FindAllStringIndex(text, -1) {
		if loc[0] > 0 && strings.ContainsRune(".!?", rune(text[loc[0]-1])) {
			parts = append(parts, text[last:loc[0]])
			last = loc[1]
		}
	}
	return append(parts, text[last:])
}

// description is the description of an object, without the maintainer note about how the schema is generated.
func description(node *obj) string {
	var keep []string
	for _, x := range splitSentences(sentence(node.str("description"))) {
		if strings.HasPrefix(x, "Generated from") || strings.HasPrefix(x, "Never edit") || strings.Contains(strings.ToLower(x), "never edit by hand") {
			continue
		}
		keep = append(keep, x)
	}
	return strings.Join(keep, " ")
}

type ref struct {
	defs  *obj
	forms []string
}

func newRef(schema *obj) *ref {
	r := &ref{defs: schema.obj("$defs")}
	for _, f := range formsOrder {
		if r.defs.has(f) {
			r.forms = append(r.forms, f)
		}
	}
	return r
}

func (r *ref) name(s string) string { return s[strings.LastIndex(s, "/")+1:] }

func (r *ref) def(name string) *obj {
	d := r.defs.obj(name)
	if d == nil {
		panic("no definition " + name)
	}
	return d
}

func (r *ref) isForm(n string) bool {
	for _, f := range r.forms {
		if f == n {
			return true
		}
	}
	return false
}

var anchorStrip = regexp.MustCompile(`[^a-z0-9 -]`)

func anchor(name string) string {
	return strings.ReplaceAll(anchorStrip.ReplaceAllString(strings.ToLower(name), ""), " ", "-")
}

func (r *ref) link(name string) string { return "[" + name + "](#" + anchor(name) + ")" }

// notNull is the alternatives of an anyOf that are not the explicit null.
func notNull(v any) []*obj {
	var out []*obj
	for _, a := range v.([]any) {
		if o := a.(*obj); o.val("type") != "null" {
			out = append(out, o)
		}
	}
	return out
}

func first(alts []*obj) *obj {
	if len(alts) == 0 {
		panic("anyOf without a non-null alternative")
	}
	return alts[0]
}

func md(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ")
}

func num(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, _ := n.Float64()
	return f, true
}

// sample is a placeholder value of the right shape for a property.
func sample(p *obj, r *ref, depth int) any {
	if p.has("anyOf") {
		p = first(notNull(p.val("anyOf")))
	}
	if d, ok := p.get("default"); ok && d != nil {
		return d
	}
	if p.has("$ref") {
		d := r.def(r.name(p.str("$ref")))
		if d.has("properties") && depth < 1 {
			return example(d, r, depth+1)
		}
		if d.has("properties") {
			return &obj{}
		}
		if alts := d.list("oneOf"); len(alts) > 0 {
			return sample(alts[0].(*obj), r, depth)
		}
		return "text"
	}
	if c, ok := p.get("const"); ok {
		return c
	}
	if e, ok := p.get("enum"); ok {
		for _, x := range e.([]any) {
			if x != nil {
				return x
			}
		}
		return nil
	}
	t := p.val("type")
	if l, ok := t.([]any); ok {
		t = nil
		for _, k := range l {
			if k != "null" {
				t = k
				break
			}
		}
	}
	one := func() any {
		if m, ok := p.get("minimum"); ok {
			return m
		}
		return json.Number("1")
	}
	switch t {
	case "integer", "number":
		return one()
	case "boolean":
		return true
	case "array":
		if items := p.obj("items"); items != nil && depth < 1 {
			return []any{sample(items, r, depth)}
		}
		return []any{}
	case "object":
		return &obj{}
	case "string":
		return "text"
	}
	return nil
}

// example is a small example object: the required keys, or the first keys when none is required.
func example(node *obj, r *ref, depth int) *obj {
	props := node.obj("properties")
	var keys []string
	for _, k := range node.list("required") {
		if props.has(k.(string)) {
			keys = append(keys, k.(string))
		}
	}
	if len(keys) == 0 && props != nil {
		keys = props.keys
		if len(keys) > 3 {
			keys = keys[:3]
		}
	}
	out := &obj{}
	for _, k := range keys {
		out.set(k, sample(props.obj(k), r, depth))
	}
	return out
}

func (r *ref) typeOf(p *obj) string {
	if p.has("anyOf") {
		return r.typeOf(first(notNull(p.val("anyOf")))) + " or null"
	}
	if p.has("$ref") {
		n := r.name(p.str("$ref"))
		if r.isForm(n) || r.def(n).has("properties") {
			return r.link(n)
		}
		return n
	}
	t := p.val("type")
	if l, ok := t.([]any); ok {
		base := "any"
		for _, k := range l {
			if k != "null" {
				base = k.(string)
				break
			}
		}
		return base + " or null"
	}
	switch t {
	case "array":
		return "list of " + r.typeOf(p.obj("items"))
	case "object":
		if add := p.obj("additionalProperties"); add != nil {
			return "map of " + r.typeOf(add)
		}
		return "object"
	case nil:
		return "any"
	}
	return t.(string)
}

func (r *ref) allowed(p *obj) string {
	q := p
	if p.val("type") == "array" {
		q = p.obj("items")
		if q == nil {
			q = &obj{}
		}
	}
	var out []string
	if e, ok := q.get("enum"); ok {
		var vals []string
		for _, x := range e.([]any) {
			if x == nil {
				vals = append(vals, "`null`")
			} else {
				vals = append(vals, "`"+scalarStr(x)+"`")
			}
		}
		out = append(out, strings.Join(vals, ", "))
	}
	if c, ok := p.get("const"); ok {
		out = append(out, "`"+scalarStr(c)+"`")
	} else {
		out = append(out, bounds(q, "minimum", "maximum", ""))
	}
	out = append(out, bounds(q, "minLength", "maxLength", " characters"))
	if p.val("type") == "array" {
		out = append(out, bounds(p, "minItems", "maxItems", " items"))
	}
	var kept []string
	for _, o := range out {
		if o != "" {
			kept = append(kept, o)
		}
	}
	if len(kept) == 0 {
		return "-"
	}
	return strings.Join(kept, "; ")
}

func (r *ref) deflt(p *obj) string {
	if d, ok := p.get("default"); ok {
		return "`" + dumps(d, false, false) + "`"
	}
	return "-"
}

// bounds is `1..50`, `at least 1`, `at most 50`, or nothing.
func bounds(q *obj, lo, hi, unit string) string {
	a, b := q.val(lo), q.val(hi)
	if unit == " characters" && b == nil {
		if f, ok := num(a); ok && f == 1 {
			return "not empty"
		}
	}
	if unit == " characters" && a != nil {
		if f, _ := num(a); f != 1 {
			unit = " characters"
		} else {
			unit = " character"
		}
	}
	switch {
	case a != nil && b != nil:
		return scalarStr(a) + ".." + scalarStr(b) + unit
	case a != nil:
		return "at least " + scalarStr(a) + unit
	case b != nil:
		return "at most " + scalarStr(b) + unit
	}
	return ""
}

// walk lists the object types in the order a reader meets them: the document, then depth first along each key (required keys first).
func walk(schema *obj, r *ref) (order []string, uses map[string][]string) {
	uses = map[string][]string{}
	keysOf := func(node *obj) []string {
		props := node.obj("properties")
		isReq := map[string]bool{}
		var out []string
		for _, k := range node.list("required") {
			if props.has(k.(string)) {
				isReq[k.(string)] = true
				out = append(out, k.(string))
			}
		}
		var rest []string
		if props != nil {
			for _, k := range props.keys {
				if !isReq[k] {
					rest = append(rest, k)
				}
			}
		}
		sort.Strings(rest)
		return append(out, rest...)
	}
	var visit func(node *obj, path, tname string)
	visit = func(node *obj, path, tname string) {
		if u, seen := uses[tname]; seen {
			if len(u) < 3 && !contains(u, path) {
				uses[tname] = append(u, path)
			}
			return
		}
		uses[tname] = []string{path}
		order = append(order, tname)
		for _, k := range keysOf(node) {
			child := node.obj("properties").obj(k)
			sub := path
			if path != "" {
				sub += "."
			}
			sub += k
			for {
				if child.has("anyOf") {
					child = first(notNull(child.val("anyOf")))
					continue
				}
				if child.has("$ref") {
					n := r.name(child.str("$ref"))
					if d := r.def(n); d.has("properties") {
						visit(d, sub, n)
					}
					break
				}
				if child.val("type") == "array" && child.has("items") {
					child = child.obj("items")
					sub += "[]"
					continue
				}
				if add := child.obj("additionalProperties"); add != nil {
					child = add
					sub += ".<name>"
					continue
				}
				break
			}
		}
	}
	visit(schema, "", "Document")
	return order, uses
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func render(schema *obj) string {
	r := newRef(schema)
	order, uses := walk(schema, r)
	out := []string{header}

	out = append(out, "## Contents\n")
	out = append(out, "- [The file](#document): the top-level keys")
	out = append(out, "- [Objects](#objects): every nested object, in the order they are met")
	var groupOrder []string
	groups := map[string][]string{}
	for _, tname := range order[1:] {
		path := uses[tname][0]
		t := path
		if i := strings.IndexAny(path, ".["); i >= 0 {
			t = path[:i]
		}
		if t == "" {
			t = "file"
		}
		if _, ok := groups[t]; !ok {
			groupOrder = append(groupOrder, t)
		}
		groups[t] = append(groups[t], tname)
	}
	for _, top := range groupOrder {
		var links []string
		for _, n := range groups[top] {
			links = append(links, r.link(n))
		}
		out = append(out, "  - `"+top+"`: "+strings.Join(links, ", "))
	}
	out = append(out, "- [Value forms](#value-forms): the keys that accept more than one shape (rolls, dice, ratings, rules, promotion targets)\n")

	for _, tname := range order {
		node := schema
		if tname != "Document" {
			node = r.def(tname)
		}
		if tname == "Document" {
			out = append(out, "## Document\n", description(node)+"\n", "The file is one JSON object. Its keys:\n")
		} else {
			if tname == order[1] {
				out = append(out, "## Objects\n")
			}
			out = append(out, "### "+tname+"\n")
			var where []string
			for _, u := range uses[tname] {
				if u == "" {
					where = append(where, "the file")
				} else {
					where = append(where, "`"+u+"`")
				}
			}
			out = append(out, description(node)+"\n", "Found at: "+strings.Join(where, ", ")+".\n")
			if node.val("additionalProperties") == true {
				out = append(out, "*Open object*: keys that are not listed are kept as they are (the app reads some of them).\n")
			}
		}
		props := node.obj("properties")
		req := map[string]bool{}
		var keys []string
		for _, k := range node.list("required") {
			req[k.(string)] = true
			if props.has(k.(string)) {
				keys = append(keys, k.(string))
			}
		}
		out = append(out, "Example:\n", "```json\n"+dumps(example(node, r, 0), true, true)+"\n```\n")
		var rest []string
		if props != nil {
			for _, k := range props.keys {
				if !req[k] {
					rest = append(rest, k)
				}
			}
		}
		sort.Strings(rest)
		keys = append(keys, rest...)
		out = append(out, "| Key | Type | Required | Default | Allowed | Description |", "|---|---|---|---|---|---|")
		for _, k := range keys {
			p := props.obj(k)
			if p == nil {
				panic("property " + k + " is not an object")
			}
			yes := "no"
			if req[k] {
				yes = "yes"
			}
			out = append(out, fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s |", k, md(r.typeOf(p)), yes, md(r.deflt(p)), md(r.allowed(p)), md(sentence(p.str("description")))))
		}
		out = append(out, "")
	}

	out = append(out, "## Value forms\n", "These keys accept more than one shape.\n")
	for _, n := range r.forms {
		d := r.def(n)
		out = append(out, "### "+n+"\n", strings.TrimSpace(d.str("description"))+"\n")
		if alts := d.list("oneOf"); len(alts) > 0 {
			out = append(out, "Accepted forms:\n")
			for _, a := range alts {
				out = append(out, "- "+describeForm(a.(*obj), r))
			}
			out = append(out, "")
		} else if d.val("type") == "string" {
			out = append(out, "A string matching `"+d.str("pattern")+"`.\n")
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), " \t\r\n\v\f") + "\n"
}

func describeForm(a *obj, r *ref) string {
	if a.has("$ref") {
		return "an object, see " + r.link(r.name(a.str("$ref")))
	}
	switch a.val("type") {
	case "integer":
		rng := ""
		if a.has("minimum") {
			rng = " " + scalarStr(a.val("minimum")) + ".." + scalarStr(a.val("maximum"))
		}
		return "a whole number" + rng
	case "number":
		return "a number"
	case "string":
		if a.has("pattern") && a.str("pattern") != `\S` {
			return "a string matching `" + a.str("pattern") + "`"
		}
		return "a non-empty string"
	case "array":
		n := ""
		if a.has("minItems") {
			n = scalarStr(a.val("minItems"))
		}
		return strings.ReplaceAll("a list of "+n+" whole numbers", "  ", " ")
	case "object":
		props := a.obj("properties")
		var all, req []string
		if props != nil {
			all = props.keys
		}
		for _, k := range a.list("required") {
			if props.has(k.(string)) {
				req = append(req, k.(string))
			}
		}
		keys := append([]string{}, req...)
		for _, k := range all {
			if !contains(req, k) {
				keys = append(keys, k)
			}
		}
		tick := func(l []string) string {
			var t []string
			for _, k := range l {
				t = append(t, "`"+k+"`")
			}
			return strings.Join(t, ", ")
		}
		if len(req) > 0 && len(req) == len(all) {
			return "an object with the keys " + tick(keys) + ", all required"
		}
		reqText := tick(req)
		if len(req) == 0 {
			reqText = "none"
		}
		return "an object with the keys " + tick(keys) + " (required: " + reqText + ")"
	}
	return dumps(a, false, false)
}
