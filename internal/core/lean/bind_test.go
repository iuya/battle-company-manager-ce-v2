package lean

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// probe is a small model with one field of every kind the binder reads.
type probe struct {
	Name   string         `json:"name" required:"true" min:"1"`
	Kind   string         `json:"kind,omitempty" enum:"wargear|equipment|creature"`
	Stat   string         `json:"stat,omitempty" enum:"fight|shoot" noun:"characteristic"`
	N      int            `json:"n,omitempty" min:"0" max:"20"`
	Low    int            `json:"low,omitempty" min:"5"`
	High   int            `json:"high,omitempty" max:"5"`
	F      float64        `json:"f,omitempty" min:"0" max:"100"`
	On     bool           `json:"on,omitempty"`
	Maybe  *bool          `json:"maybe,omitempty"`
	List   []string       `json:"list,omitempty" min:"1" max:"3" enum:"a|b|c"`
	Pair   []int          `json:"pair,omitempty" min:"2" max:"2" emin:"-6" emax:"6"`
	Roll   Roll           `json:"roll,omitempty"`
	Dice   DiceSpec       `json:"dice,omitempty"`
	Rating Rating         `json:"rating,omitempty"`
	Ref    *RuleRef       `json:"ref,omitempty"`
	Ver    Scalar         `json:"version,omitempty"`
	Sub    *probeSub      `json:"sub,omitempty"`
	Subs   []probeSub     `json:"subs,omitempty"`
	Table  map[string]int `json:"table,omitempty" `
	Any    Free           `json:"any,omitempty"`
	Rest   map[string]any `json:"-" rest:"true"`
}

type probeSub struct {
	ID   string    `json:"id" required:"true"`
	Next *probeSub `json:"next,omitempty"`
}

type strictProbe struct {
	Start string `json:"start,omitempty"`
	Stat  string `json:"stat,omitempty"`
}

func bindProbe[T any](t *testing.T, src string) (T, Report, bool) {
	t.Helper()
	var v T
	var rep Report
	ok := Bind("f.json", []byte(src), &v, &rep)
	return v, rep, ok
}

func lines(rep Report) []string {
	var out []string
	for _, i := range rep.Issues {
		out = append(out, i.Path+": "+i.Message)
	}
	return out
}

func TestBindIssues(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want []string
	}{
		{"ok", `{"name":"x","n":3,"list":["a"],"kind":"wargear","roll":"2-3","dice":"2d6","rating":{"warrior":1,"hero":5}}`, nil},
		{"missing required", `{}`, []string{"name: missing"}},
		{"null required", `{"name":null}`, []string{"name: must not be null; it is required"}},
		{"null optional is absent", `{"name":"x","n":null,"sub":null}`, nil},
		{"empty string", `{"name":""}`, []string{"name: must not be empty"}},
		{"wrong type string", `{"name":3}`, []string{"name: expected a string, got a number"}},
		{"wrong type int", `{"name":"x","n":"3"}`, []string{"n: expected a whole number, got a string"}},
		{"fraction", `{"name":"x","n":1.5}`, []string{"n: 1.5 is not a whole number"}},
		{"integral float ok", `{"name":"x","n":3.0}`, nil},
		{"huge", `{"name":"x","n":1e400}`, []string{"n: 1e400 is not a whole number"}},
		{"range", `{"name":"x","n":40}`, []string{"n: 40 is out of range 0..20"}},
		{"range negative", `{"name":"x","n":-1}`, []string{"n: -1 is out of range 0..20"}},
		{"min only", `{"name":"x","low":1}`, []string{"low: 1 is below the minimum 5"}},
		{"max only", `{"name":"x","high":9}`, []string{"high: 9 is above the maximum 5"}},
		{"float range", `{"name":"x","f":100.5}`, []string{"f: 100.5 is out of range 0..100"}},
		{"bool type", `{"name":"x","on":"yes"}`, []string{"on: expected true or false, got a string"}},
		{"pointer bool", `{"name":"x","maybe":false}`, nil},
		{"enum", `{"name":"x","kind":"wargaer"}`, []string{`kind: "wargaer" is not a valid kind; known: wargear, equipment, creature`}},
		{"enum noun", `{"name":"x","stat":"figth"}`, []string{`stat: "figth" is not a valid characteristic; known: fight, shoot`}},
		{"list type", `{"name":"x","list":"a"}`, []string{"list: expected a list, got a string"}},
		{"list too short", `{"name":"x","list":[]}`, []string{"list: needs at least 1 item, has 0"}},
		{"list too long", `{"name":"x","list":["a","a","a","a"]}`, []string{"list: allows at most 3 items, has 4"}},
		{"list element enum", `{"name":"x","list":["a","z"]}`, []string{`list[1]: "z" is not a valid list; known: a, b, c`}},
		{"list null element", `{"name":"x","list":["a",null]}`, []string{"list[1]: must not be null"}},
		{"pair length", `{"name":"x","pair":[1]}`, []string{"pair: needs exactly 2 items, has 1"}},
		{"pair element range", `{"name":"x","pair":[1,9]}`, []string{"pair[1]: 9 is out of range -6..6"}},
		{"object expected", `{"name":"x","sub":[]}`, []string{"sub: expected an object, got a list"}},
		{"nested path", `{"name":"x","subs":[{"id":"a"},{"next":{"next":{}}}]}`,
			[]string{"subs[1].id: missing", "subs[1].next.id: missing", "subs[1].next.next.id: missing"}},
		{"map value type", `{"name":"x","table":{"a":1,"b":"z"}}`, []string{"table.b: expected a whole number, got a string"}},
		{"map odd key", `{"name":"x","table":{"a b":"z"}}`, []string{`table["a b"]: expected a whole number, got a string`}},
		{"rest keeps unknown", `{"name":"x","extra":{"a":1}}`, nil},
		{"all errors collected", `{"n":99,"kind":"q","on":1,"sub":{}}`,
			[]string{"name: missing", `kind: "q" is not a valid kind; known: wargear, equipment, creature`, "n: 99 is out of range 0..20", "on: expected true or false, got a number", "sub.id: missing"}},
		{"roll number", `{"name":"x","roll":3}`, nil},
		{"roll pair", `{"name":"x","roll":[2,3]}`, nil},
		{"roll bad string", `{"name":"x","roll":"a-b"}`, []string{`roll: "a-b" is not a roll; expected a number, a range like "2-3" or a pair [lo, hi]`}},
		{"roll bad pair", `{"name":"x","roll":[1,2,3]}`, []string{`roll: a roll pair needs 2 numbers, has 3; expected a number, a range like "2-3" or a pair [lo, hi]`}},
		{"roll bool", `{"name":"x","roll":true}`, []string{`roll: expected a number, a range like "2-3" or a pair [lo, hi], got true or false`}},
		{"roll huge", `{"name":"x","roll":99999}`, []string{"roll: 99999 is not a roll; expected a whole number 0..1000"}},
		{"dice bad", `{"name":"x","dice":"2d6x"}`, []string{`dice: "2d6x" is not a dice spec; expected [N]dM, N 1..10, M 2..100`}},
		{"dice type", `{"name":"x","dice":6}`, []string{`dice: expected a dice spec string like "d6" or "2d6", got a number`}},
		{"rating number", `{"name":"x","rating":5}`, nil},
		{"rating object missing", `{"name":"x","rating":{"warrior":1}}`, []string{"rating.hero: missing"}},
		{"rating range", `{"name":"x","rating":5000}`, []string{"rating: 5000 is out of range 0..1000"}},
		{"rating type", `{"name":"x","rating":"5"}`, []string{"rating: expected a number or {warrior, hero}, got a string"}},
		{"ref name", `{"name":"x","ref":"Spy"}`, nil},
		{"ref inline", `{"name":"x","ref":{"name":"Spy","definition":"d"}}`, nil},
		{"ref inline bad", `{"name":"x","ref":{"definition":"d"}}`, []string{"ref.name: missing"}},
		{"ref type", `{"name":"x","ref":3}`, []string{"ref: expected a rule name or a rule definition object, got a number"}},
		{"version number", `{"name":"x","version":1.1}`, nil},
		{"version bool", `{"name":"x","version":true}`, []string{"version: expected a string or a number, got true or false"}},
		{"any keeps anything", `{"name":"x","any":{"a":[1,"b",null]}}`, nil},
		{"duplicate key", `{"name":"x","name":"y"}`, []string{`name: key "name" is written twice; the last one would win silently`}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, rep, ok := bindProbe[probe](t, c.src)
			if !ok {
				t.Fatalf("not usable: %v", lines(rep))
			}
			if got := lines(rep); !reflect.DeepEqual(got, c.want) {
				t.Errorf("issues\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestBindUnknownKeys(t *testing.T) {
	_, rep, _ := bindProbe[strictProbe](t, `{"strat":"a","stat":"b","zzzz":1}`)
	if len(rep.Issues) != 2 {
		t.Fatalf("issues %v", lines(rep))
	}
	first := rep.Issues[0]
	if first.Path != "strat" || first.Message != `unknown field; did you mean "start"?` || first.Closest != "start" || first.File != "f.json" || !strings.Contains(first.Hint, "known fields: start, stat") {
		t.Errorf("first %+v", first)
	}
	second := rep.Issues[1]
	if second.Path != "zzzz" || second.Message != "unknown field" || second.Closest != "" {
		t.Errorf("second %+v", second)
	}
}

func TestBindRestKeepsUnknown(t *testing.T) {
	v, rep, _ := bindProbe[probe](t, `{"name":"x","extra":{"a":1},"other":2}`)
	if len(rep.Issues) != 0 || len(v.Rest) != 2 || v.Rest["other"] != json.Number("2") {
		t.Fatalf("%v %+v", lines(rep), v.Rest)
	}
}

func TestBindValues(t *testing.T) {
	v, rep, _ := bindProbe[probe](t, `{"name":"x","roll":"2-3","dice":"D6","rating":{"warrior":1,"hero":5},"ref":"Spy","version":1.1,"list":["a","b"],"pair":[-1,1],"maybe":false,"sub":{"id":"a","next":{"id":"b"}}}`)
	if len(rep.Issues) != 0 {
		t.Fatal(lines(rep))
	}
	if v.Roll != (Roll{2, 3}) || v.Dice != (DiceSpec{1, 6}) || v.Rating != (Rating{1, 5}) || v.Ref == nil || v.Ref.Name != "Spy" || v.Ver != "1.1" ||
		!reflect.DeepEqual(v.List, []string{"a", "b"}) || !reflect.DeepEqual(v.Pair, []int{-1, 1}) || v.Maybe == nil || *v.Maybe || v.Sub.Next.ID != "b" {
		t.Errorf("%+v", v)
	}
	r, _, _ := bindProbe[probe](t, `{"name":"x","rating":5,"roll":4,"ref":{"name":"Spy","definition":"d"}}`)
	if r.Rating != (Rating{5, 5}) || r.Roll != (Roll{4, 4}) || r.Ref.Def == nil || r.Ref.Name != "Spy" {
		t.Errorf("%+v", r)
	}
}

func TestBindSyntaxAndShape(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		usable          bool
	}{
		{"syntax", "{\n  \"name\": \"x\",\n  \"n\": }", "not valid JSON at line 3, column 9", false},
		{"truncated", `{"name":"x"`, "unexpected end of the file", false},
		{"empty", ``, "unexpected end of the file", false},
		{"trailing", `{"name":"x"} {}`, "unexpected data after", false},
		{"depth", strings.Repeat("[", 70) + strings.Repeat("]", 70), "nested more than 64 levels", false},
		{"top-level list", `[1]`, "expected an object, got a list", true},
		{"bom", "\xef\xbb\xbf" + `{"name":"x"}`, "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, rep, ok := bindProbe[probe](t, c.src)
			if ok != c.usable {
				t.Errorf("usable = %v, want %v (%v)", ok, c.usable, lines(rep))
			}
			if c.want != "" {
				if len(rep.Issues) == 0 || !strings.Contains(rep.Issues[0].Message, c.want) {
					t.Errorf("issues %v, want %q", lines(rep), c.want)
				}
			} else if len(rep.Issues) != 0 {
				t.Errorf("unexpected %v", lines(rep))
			}
		})
	}
}

func TestBindDepthJustInside(t *testing.T) {
	src := `{"name":"x","any":` + strings.Repeat("[", 60) + strings.Repeat("]", 60) + `}`
	if _, rep, ok := bindProbe[probe](t, src); !ok || len(rep.Issues) != 0 {
		t.Fatalf("%v", lines(rep))
	}
}

func TestBindDeterministic(t *testing.T) {
	src := `{"zz":1,"aa":2,"name":3,"mm":{"x":1},"kk":[1]}`
	_, a, _ := bindProbe[strictProbe](t, src)
	for i := 0; i < 20; i++ {
		_, b, _ := bindProbe[strictProbe](t, src)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("report order changed between runs")
		}
	}
	got := lines(a)
	if got[0] != "aa: unknown field" || got[len(got)-1] != "zz: unknown field" {
		t.Errorf("unknown keys are not in sorted order: %q", got)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	src := `{"name":"x","kind":"wargear","roll":"2-3","dice":"2d6","rating":{"warrior":1,"hero":5},"ref":{"name":"Spy","definition":"d"},"version":"1.1","list":["a"],"pair":[-1,1],"maybe":false,"sub":{"id":"a"},"table":{"a":1},"any":{"k":[1,"s",null]},"extra":{"z":1}}`
	a, rep, _ := bindProbe[probe](t, src)
	if len(rep.Issues) != 0 {
		t.Fatal(lines(rep))
	}
	out, err := Encode(a)
	if err != nil {
		t.Fatal(err)
	}
	b, rep2, _ := bindProbe[probe](t, string(out))
	if len(rep2.Issues) != 0 {
		t.Fatalf("%v\n%s", lines(rep2), out)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("round trip differs\n%+v\n%+v\n%s", a, b, out)
	}
}

func TestBinders(t *testing.T) {
	for _, c := range []struct {
		in   any
		want Roll
		bad  bool
	}{
		{json.Number("3"), Roll{3, 3}, false},
		{"3", Roll{3, 3}, false},
		{" 2 - 3 ", Roll{2, 3}, false},
		{"3-2", Roll{3, 2}, false}, // bound as written; validation reports it
		{[]any{json.Number("1"), json.Number("2")}, Roll{1, 2}, false},
		{json.Number("1.5"), Roll{}, true},
		{json.Number("-1"), Roll{}, true},
		{"", Roll{}, true},
		{"1-2-3", Roll{}, true},
		{nil, Roll{}, true},
	} {
		var r Roll
		var rep Report
		r.BindJSON(c.in, "roll", NewReporter("f", &rep))
		if (len(rep.Issues) > 0) != c.bad || (!c.bad && r != c.want) {
			t.Errorf("Roll from %#v = %+v, issues %v", c.in, r, lines(rep))
		}
	}
	if !(Roll{3, 2}).Empty() || (Roll{2, 2}).Empty() || !(Roll{2, 4}).Covers(3) || (Roll{2, 4}).Covers(5) {
		t.Error("Roll helpers")
	}
	if (Roll{2, 2}).String() != "2" || (Roll{2, 3}).String() != "2-3" {
		t.Error("Roll.String")
	}
}
