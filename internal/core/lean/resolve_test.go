package lean

import (
	"reflect"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

func resolved(t *testing.T, extra string, lower Lower) (*Document, *Index, Report) {
	t.Helper()
	doc, nrep := normalised(t, minimalWith(t, extra))
	var rep Report
	rep.Merge(nrep)
	ix := Resolve(doc, lower, NewReporter("f.json", &rep))
	return doc, ix, rep
}

func issueLines(rep Report) []string {
	var out []string
	for _, i := range rep.Issues {
		out = append(out, i.Path+": "+i.Message)
	}
	return out
}

func TestResolveClassification(t *testing.T) {
	_, ix, rep := resolved(t, `
"units":[{"name":"Angmar Warg Rider","tts":{"foot":"Ranger of Gondor"}},
         {"name":"Ranger of Ithilien","extends":"Ranger of Gondor"},
         {"name":"Mordor Hound","profile":{"m":1},"points":3},
         {"name":"Ranger of the North","overrides":true,"profile":{"m":1}}],
"rules":[{"name":"Stand Fast","auras":[{"radius":6,"kind":"command","affects":"friendly","when":"always"}]},
         {"name":"Hatred","overrides":true,"definition":"mine"},
         {"name":"Fresh","definition":"new"}]`, newFakeLower())
	if len(rep.Issues) != 0 {
		t.Fatal(rep)
	}
	for _, c := range []struct {
		kind Kind
		name string
		want Class
	}{
		{KindUnit, "Angmar Warg Rider", ClassAnnotation},
		{KindUnit, "Ranger of Ithilien", ClassExtends},
		{KindUnit, "Mordor Hound", ClassNew},
		{KindUnit, "Ranger of the North", ClassShadow},
		{KindRule, "Stand Fast", ClassAnnotation},
		{KindRule, "Hatred", ClassShadow},
		{KindRule, "Fresh", ClassNew},
	} {
		e, ok := ix.Lookup(c.kind, c.name, "")
		if !ok || e.Class != c.want {
			t.Errorf("%s %q: %+v %v, want class %v", c.kind, c.name, e, ok, c.want)
		}
	}
	if _, ok := ix.Lookup(KindUnit, "Ranger of Gondor", ""); ok {
		t.Error("a vanilla name is not an entry of the file")
	}
}

func TestResolveAkaAndNameKey(t *testing.T) {
	_, ix, rep := resolved(t, `"gear":[{"name":"Bow","aka":["Dwarf Bow","Orc-bow"],"kind":"wargear","influence":1,"rating":1}],
"units":[{"name":"Lake-Town Guard","profile":{"m":1},"points":3,"aka":["Esgaroth Guard"]}]`, newFakeLower())
	if len(rep.Issues) != 0 {
		t.Fatal(rep)
	}
	for _, c := range []struct {
		kind Kind
		in   string
		ok   bool
	}{
		{KindGear, "Bow", true}, {KindGear, "dwarf bow", true}, {KindGear, "Orc Bow", true}, {KindGear, "Bows", false}, {KindGear, "Short Bow", false},
		{KindUnit, "lake town guard", true}, {KindUnit, "Esgaroth guard", true}, {KindUnit, "Lake-Town Guards", false},
	} {
		if e, ok := ix.Lookup(c.kind, c.in, ""); ok != c.ok || (ok && e.Name == "") {
			t.Errorf("Lookup(%s, %q) = %+v %v, want %v", c.kind, c.in, e, ok, c.ok)
		}
	}
}

func TestResolveCompanyScope(t *testing.T) {
	doc, ix, rep := resolved(t, `
"units":[{"name":"Shared Scout","extends":"Ranger of Gondor"}],
"companies":[
 {"name":"Alpha","alignment":"good","description":"d","units":[{"name":"Alpha Scout","extends":"Shared Scout"}],
  "start":{"gold":0,"units":[{"unit":"Alpha Scout"},{"unit":"Shared Scout"},{"unit":"Ranger of Gondor"}]}},
 {"name":"Beta","alignment":"good","description":"d",
  "start":{"gold":0,"units":[{"unit":"Alpha Scout"}]}}]`, newFakeLower())
	got := issueLines(rep)
	want := []string{`companies[1].start.units[0].unit: no unit "Alpha Scout" in this company, the shared units or vanilla`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if rep.Issues[0].Closest != "" {
		t.Errorf("another company's unit is not a candidate: %q", rep.Issues[0].Closest)
	}
	_ = doc
	if e, ok := ix.Lookup(KindUnit, "Alpha Scout", "Alpha"); !ok || e.Scope != "Alpha" {
		t.Errorf("company scope lookup: %+v %v", e, ok)
	}
	if _, ok := ix.Lookup(KindUnit, "Alpha Scout", "Beta"); ok {
		t.Error("Beta does not see Alpha's units")
	}
	if _, ok := ix.Lookup(KindUnit, "Shared Scout", "Beta"); !ok {
		t.Error("shared units are visible to every company")
	}
}

func TestResolveOptions(t *testing.T) {
	_, _, rep := resolved(t, `
"units":[{"name":"Dwarf Hunter","extends":"Hobbit Archer"},{"name":"Brand New","profile":{"m":1},"points":4}],
"gear":[{"name":"House Spear","kind":"wargear","influence":1,"rating":1}],
"companies":[{"name":"A","alignment":"good","description":"d",
 "start":{"gold":0,"units":[{"unit":"Dwarf Hunter","with":["Bow"]},{"unit":"Dwarf Hunter","with":["Hand Weapon","Banner","House Spear"]},{"unit":"Brand New","with":["Whatever"]},{"unit":"Dwarf Hunter","with":["Bo"]}]}}]`, newFakeLower())
	got := issueLines(rep)
	want := []string{`companies[0].start.units[3].with[0]: "Bo" is not an option of "Dwarf Hunter" nor a gear item`}
	if !reflect.DeepEqual(got, want) || rep.Issues[0].Closest != "Bow" {
		t.Fatalf("got %q closest %q", got, rep.Issues[0].Closest)
	}
}

func TestResolveHoisting(t *testing.T) {
	_, ix, rep := resolved(t, `
"rules":[{"name":"Spy","definition":"same"}],
"paths":[{"name":"P","rows":[{"roll":2,"options":[{"rule":{"name":"Spy","definition":"same"}},{"rule":{"name":"Inline One","definition":"x"}}]}]}],
"companies":[{"name":"A","alignment":"good","description":"d","rules":[{"name":"Inline One","definition":"x"}],
  "heroUpgrades":[{"name":"Inline Two","definition":"two"},{"name":"Fearless"}]}]`, newFakeLower())
	if len(rep.Issues) != 0 {
		t.Fatal(rep)
	}
	var names []string
	for _, h := range ix.Hoisted {
		names = append(names, h.Name)
	}
	if !reflect.DeepEqual(names, []string{"Spy", "Inline One", "Inline Two"}) {
		t.Errorf("hoisted %v", names)
	}
}

func TestResolveDuplicatesAndAliases(t *testing.T) {
	_, _, rep := resolved(t, `
"gear":[{"name":"Bow","aka":["Short Bow"],"kind":"wargear","influence":1,"rating":1},
        {"name":"Short  bow","kind":"wargear","influence":1,"rating":1},
        {"name":"Crossbow","aka":["Crossbow","Bow"],"kind":"wargear","influence":1,"rating":1},
        {"name":"!!!","kind":"wargear","influence":1,"rating":1}]`, newFakeLower())
	got := issueLines(rep)
	want := []string{
		`gear[1].name: "Short  bow" is already defined at gear[0]; names are unique`,
		`gear[2].aka[0]: "Crossbow" is the name itself; an alias spells it differently`,
		`gear[2].aka[1]: "Bow" is already defined at gear[0]; names and aliases are unique`,
		`gear[3].name: "!!!" has no letter or digit; a name needs at least one`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestResolveShadowCompanyLocalOverShared(t *testing.T) {
	_, _, rep := resolved(t, `
"units":[{"name":"Shared Scout","extends":"Ranger of Gondor"}],
"companies":[{"name":"A","alignment":"good","description":"d","units":[{"name":"Shared Scout","extends":"Hobbit Archer"}]},
 {"name":"B","alignment":"good","description":"d","units":[{"name":"Shared Scout","overrides":true,"extends":"Hobbit Archer"}]}]`, newFakeLower())
	got := issueLines(rep)
	want := []string{`companies[0].units[0]: "Shared Scout" exists in the shared units or vanilla; add "overrides": true to replace it or choose another name`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSkipsKindsNobodyLists(t *testing.T) {
	l := newFakeLower()
	delete(l.names, KindKeyword)
	delete(l.names, KindRace)
	delete(l.names, KindPower)
	_, _, rep := resolved(t, `"rules":[{"name":"R","definition":"d","target":{"keywords":["Nobody"]}}],
"paths":[{"name":"P","spells":["No Such Power"],"rows":[{"roll":2,"options":[{"power":"Nope"}]}]}]`, l)
	if len(rep.Issues) != 0 {
		t.Fatalf("unlisted kinds are not checked:\n%s", rep)
	}
	_, _, rep = resolved(t, `"rules":[{"name":"R","definition":"d","target":{"keywords":["Nobody"]}}],
"paths":[{"name":"P","spells":["Fortify Spirt"],"rows":[{"roll":2,"options":[{"power":"Nope"}]}]}]`, newFakeLower())
	got := issueLines(rep)
	if len(got) != 3 || rep.Issues[1].Closest != "Fortify Spirit" {
		t.Fatalf("listed kinds are checked: %q", got)
	}
}

// TestResolveMockLowerExactLookups: the lower layer is asked with the names as written, never a fuzzy form.
func TestResolveMockLowerExactLookups(t *testing.T) {
	ctrl := gomock.NewController(t)
	lower := NewMockLower(ctrl)
	lower.EXPECT().Has(KindUnit, "Ranger of Arnr").Return(false).MinTimes(1)
	lower.EXPECT().Names(KindUnit).Return([]string{"Ranger of Arnor"}).AnyTimes()
	lower.EXPECT().Has(gomock.Any(), gomock.Any()).Return(false).AnyTimes()
	lower.EXPECT().Names(gomock.Any()).Return(nil).AnyTimes()
	_, _, rep := resolved(t, `"companies":[{"name":"A","alignment":"good","description":"d","start":{"gold":0,"units":[{"unit":"Ranger of Arnr"}]}}]`, lower)
	if len(rep.Issues) != 1 || rep.Issues[0].Closest != "Ranger of Arnor" {
		t.Fatal(rep)
	}
}
