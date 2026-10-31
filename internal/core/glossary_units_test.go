package core

import (
	"fmt"
	"strings"
	"testing"
)

const unitGlossaryDoc = `{"name":"G","version":"1","terms":[
 {"name":"Warrior","category":"x","definition":"A model that is not a Hero.","sources":[]},
 {"name":"Hero","category":"x","definition":"A named model.","sources":[]},
 {"name":"Fell Warg","category":"wargear","definition":"A mount.","sources":[]},
 {"name":"Promotions","category":"x","definition":"A Ranger of Arnor Warrior may become a Ranger of the North.","sources":[]},
 {"name":"Dwarf","category":"x","definition":"Hardy.","sources":[]}
]}`

func unitFixture(names ...string) []BaseUnit {
	out := make([]BaseUnit, len(names))
	for i, n := range names {
		out[i] = BaseUnit{ID: UnitAnchor(n), Name: n}
	}
	return out
}

func unitGlossary(t *testing.T, names ...string) Glossary {
	t.Helper()
	g, err := BuildGlossary(RulesetInfo{Name: "G"}, []byte(unitGlossaryDoc))
	if err != nil {
		t.Fatal(err)
	}
	return g.withUnits(unitFixture(names...))
}

// mark prints segments with the links in brackets: "a [U:u-x|Name] b [T:warrior|Warrior]".
func mark(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		switch {
		case s.UnitID != "":
			fmt.Fprintf(&b, "[U:%s|%s]", s.UnitID, s.Text)
		case s.TermID != "":
			fmt.Fprintf(&b, "[T:%s|%s]", s.TermID, s.Text)
		default:
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func TestUnitNamesInTexts(t *testing.T) {
	g := unitGlossary(t, "Ranger of Arnor", "Ranger of the North", "Dwarf Warrior", "Erebor Dwarf Warrior", "Warrior of Minas Tirith",
		"Uruk-hai Warrior", "Isengard Warg", "Isengard Warg Rider", "Ruffian", "Fell Warg", "Lake-town Guard")
	for name, c := range map[string]struct{ text, want string }{
		"the example of the issue: both units, the term between them": {
			"Should a Ranger of Arnor Warrior achieve a Hero result, they trade out their profile for a Ranger of the North.",
			"Should a [U:u-ranger-of-arnor|Ranger of Arnor] [T:warrior|Warrior] achieve a [T:hero|Hero] result, they trade out their profile for a [U:u-ranger-of-the-north|Ranger of the North]."},
		"the longest name wins, not the one inside it": {
			"An Erebor Dwarf Warrior and a Dwarf Warrior.",
			"An [U:u-erebor-dwarf-warrior|Erebor Dwarf Warrior] and a [U:u-dwarf-warrior|Dwarf Warrior]."},
		"a unit named inside a longer unit's name is not linked on its own": {
			"The Isengard Warg Rider.",
			"The [U:u-isengard-warg-rider|Isengard Warg Rider]."},
		"the shorter name alone links": {
			"The Isengard Warg.",
			"The [U:u-isengard-warg|Isengard Warg]."},
		"inside another word is not a mention": {
			"Rangers of Arnor, a Ranger of Arnorian blood, the Ranger of the Northern march.",
			"Rangers of Arnor, a Ranger of Arnorian blood, the Ranger of the Northern march."},
		"case does not matter for a name of several words": {
			"a ranger of ARNOR.",
			"a [U:u-ranger-of-arnor|ranger of ARNOR]."},
		"a name of one word only as it is spelled": {
			"A Ruffian, a ruffian, Ruffians.",
			"A [U:u-ruffian|Ruffian], a ruffian, Ruffians."},
		"punctuation next to the name is left alone": {
			"(Ranger of Arnor), Ranger of the North; \"Lake-town Guard\".",
			"([U:u-ranger-of-arnor|Ranger of Arnor]), [U:u-ranger-of-the-north|Ranger of the North]; \"[U:u-lake-town-guard|Lake-town Guard]\"."},
		"a hyphen is part of the name, not a boundary of another": {
			"An Uruk-hai Warrior, and Uruk-hai alone.",
			"An [U:u-uruk-hai-warrior|Uruk-hai Warrior], and Uruk-hai alone."},
		"the same unit twice: the first mention links": {
			"A Ranger of Arnor meets a Ranger of Arnor.",
			"A [U:u-ranger-of-arnor|Ranger of Arnor] meets a Ranger of Arnor."},
		"a name that is also a term stays the term": {
			"He rides a Fell Warg.",
			"He rides a [T:fell-warg|Fell Warg]."},
		"a term inside a text with no unit is as before": {
			"A Warrior and a Hero.",
			"A [T:warrior|Warrior] and a [T:hero|Hero]."},
		"the name at the very start and end of the text": {
			"Ranger of Arnor",
			"[U:u-ranger-of-arnor|Ranger of Arnor]"},
		"a trailing prefix of a name is not a mention": {
			"Ranger of",
			"Ranger of"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := mark(g.Link(c.text)); got != c.want {
				t.Errorf("Link(%q)\n got  %s\n want %s", c.text, got, c.want)
			}
		})
	}
}

// The text is never changed, only cut.
func TestUnitLinksKeepTheText(t *testing.T) {
	g := unitGlossary(t, "Ranger of Arnor", "Ranger of the North")
	for _, text := range []string{"", "Ranger of Arnor", "  Ranger of Arnor  and\nRanger of the North ", "ünï Ranger of Arnor ✓", "Ranger of the"} {
		if got := joinSegments(g.Link(text)); got != text {
			t.Errorf("Link(%q) joins to %q", text, got)
		}
	}
}

// A term of the glossary's own beats a unit of the same name; the glossary's terms are linked as before and unit names in their
// definitions link too.
func TestUnitsAndTerms(t *testing.T) {
	g := unitGlossary(t, "Ranger of Arnor", "Ranger of the North", "Fell Warg", "Dwarf Warrior")
	if g.link.units["fell warg"] != "" {
		t.Error("Fell Warg is a term of the glossary, so it must not be a unit link")
	}
	if got := mark(g.Link("a Fell Warg")); got != "a [T:fell-warg|Fell Warg]" {
		t.Errorf("the term is linked: %s", got)
	}
	promo, ok := g.Term("promotions")
	if !ok {
		t.Fatal("no Promotions term")
	}
	if got := mark(promo.Paragraphs[0]); got != "A [U:u-ranger-of-arnor|Ranger of Arnor] [T:warrior|Warrior] may become a [U:u-ranger-of-the-north|Ranger of the North]." {
		t.Errorf("a definition names two units: %s", got)
	}
	// "Dwarf" is a term, "Dwarf Warrior" a unit: the longer name is the unit, the term still links alone
	if got := mark(g.Link("A Dwarf Warrior, a Dwarf.")); got != "A [U:u-dwarf-warrior|Dwarf Warrior], a [T:dwarf|Dwarf]." {
		t.Errorf("unit over the term inside it: %s", got)
	}
	if c := g.UnitCard("u-ranger-of-arnor"); c == nil || c.Name != "Ranger of Arnor" {
		t.Errorf("UnitCard: %+v", c)
	}
	if g.UnitCard("u-nobody") != nil {
		t.Error("an unknown anchor has no card")
	}
}

// A glossary with no units, or a unit list with nothing usable, is the glossary it was: no UnitID anywhere.
func TestNoUnitsNoUnitLinks(t *testing.T) {
	g, err := BuildGlossary(RulesetInfo{Name: "G"}, []byte(unitGlossaryDoc))
	if err != nil {
		t.Fatal(err)
	}
	for _, with := range []Glossary{g, g.withUnits(nil), g.withUnits(unitFixture("Ox", ""))} {
		for _, seg := range with.Link("a Ranger of Arnor Warrior and an Ox") {
			if seg.UnitID != "" {
				t.Errorf("unexpected unit link %+v", seg)
			}
		}
	}
	// the glossary file on its own is not changed by building the campaign's
	withUnits := g.withUnits(unitFixture("Ranger of Arnor"))
	if mark(g.Link("Ranger of Arnor")) != "Ranger of Arnor" || mark(withUnits.Link("Ranger of Arnor")) == "Ranger of Arnor" {
		t.Error("the units of one glossary leaked into the other")
	}
	if len(g.Units) != 0 {
		t.Error("the file's glossary gained units")
	}
}

// A glossary file with no terms at all still links units.
func TestUnitsWithoutTerms(t *testing.T) {
	g, err := BuildGlossary(RulesetInfo{Name: "G"}, []byte(`{"name":"G","version":"1","terms":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	g = g.withUnits(unitFixture("Ranger of Arnor"))
	if got := mark(g.Link("A Ranger of Arnor.")); got != "A [U:u-ranger-of-arnor|Ranger of Arnor]." {
		t.Errorf("got %s", got)
	}
}

// Two cards for one name (two companies, two profiles): the text links the first, the plain anchor.
func TestUnitTwoCardsOneName(t *testing.T) {
	units := []BaseUnit{{ID: "u-guard", Name: "Guard"}, {ID: "u-guard-minas-tirith", Name: "Guard"}}
	g, _ := BuildGlossary(RulesetInfo{Name: "G"}, []byte(unitGlossaryDoc))
	g = g.withUnits(units)
	if got := mark(g.Link("A Guard.")); got != "A [U:u-guard|Guard]." {
		t.Errorf("got %s", got)
	}
}

// The shipped rule set: the Arnor promotion text of the issue links both units, to cards that exist, and every link of every
// rule text points at a card of the glossary.
func TestShippedUnitLinks(t *testing.T) {
	rs := shippedRules(t)
	g := rs.GlossaryWithUnits()
	if len(g.Units) < 100 {
		t.Fatalf("%d units", len(g.Units))
	}
	text := "Should a Ranger of Arnor Warrior achieve a Hero in the Making result on the Warrior Promotion Table, they instead trade out their profile in its entirety for a Ranger of the North."
	var units []string
	for _, seg := range g.Link(text) {
		if seg.UnitID != "" {
			if g.UnitCard(seg.UnitID) == nil {
				t.Errorf("%s has no card", seg.UnitID)
			}
			units = append(units, seg.Text)
		}
	}
	if strings.Join(units, "|") != "Ranger of Arnor|Ranger of the North" {
		t.Errorf("units linked in the Arnor text: %v", units)
	}
	// every unit name written in full links to its own card (apart from the names that are also terms)
	for _, u := range g.Units {
		if u.ID != UnitAnchor(u.Name) {
			continue
		}
		if _, term := g.Find(u.Name); term {
			continue
		}
		segs := g.Link("The " + u.Name + " charges.")
		want := "The [U:" + u.ID + "|" + u.Name + "] charges."
		if len(u.Name) >= minLinkLen && mark(segs) != want {
			t.Errorf("%s: %s", u.Name, mark(segs))
		}
	}
	// the terms' own texts link only to cards that exist
	n := 0
	for _, term := range g.Terms {
		for _, para := range term.Paragraphs {
			for _, seg := range para {
				if seg.UnitID != "" {
					n++
					if g.UnitCard(seg.UnitID) == nil {
						t.Errorf("term %s links %s, which has no card", term.Name, seg.UnitID)
					}
				}
			}
		}
	}
	t.Logf("%d unit links in the %d terms", n, len(g.Terms))
	// the rule set's glossary is built once, and the file's own glossary stays without units
	if again := rs.GlossaryWithUnits(); len(again.Terms) != len(g.Terms) || &again.Terms[0] != &g.Terms[0] {
		t.Error("the campaign glossary is built again on every call")
	}
	if len(rs.Glossary.Units) != 0 {
		t.Error("the file's glossary gained units")
	}
}

func TestUnitTip(t *testing.T) {
	u := BaseUnit{Name: "Ranger of the North", Includes: []string{"A", "B"}, Preview: UnitPreview{
		Points: 70, Types: []string{"Infantry", "Hero"},
		Stats:   []Attribute{{Abbr: "Mv", Value: 6, Trail: `"`}, {Abbr: "F", Value: 4, Trail: "/4+"}, {Name: "Might", Value: 1, HeroOnly: true}},
		Wargear: []string{"Bow", "Hand weapon"}, HeroicActions: []string{"Heroic Strike"}, Rules: []string{"Stealthy"}}}
	if got, want := u.TipMeta(), "Infantry · Hero · 70 pts"; got != want {
		t.Errorf("meta %q, want %q", got, want)
	}
	want := "Mv 6\" · F 4/4+ · Might 1\nMade of: A, B\nWargear: Bow, Hand weapon\nHeroic actions: Heroic Strike\nSpecial rules: Stealthy"
	if got := u.TipText(); got != want {
		t.Errorf("text %q, want %q", got, want)
	}
	if (BaseUnit{}).TipText() != "" || (BaseUnit{}).TipMeta() != "" {
		t.Error("an empty unit has an empty tip")
	}
}

// BenchmarkGlossaryWithUnits is the one-time cost, per rule set, of teaching the campaign glossary its units (cutting every term's text again):
// go test ./internal/core -run xxx -bench GlossaryWithUnits -benchmem
func BenchmarkGlossaryWithUnits(b *testing.B) {
	rs := shippedRules(b)
	units := rs.BaseUnits()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rs.Glossary.withUnits(units)
	}
}
