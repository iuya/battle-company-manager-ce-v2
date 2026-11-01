package core

import (
	"strings"
	"testing"
)

const itemGlossaryDoc = `{"name":"G","version":"1","terms":[
 {"name":"Mountain Dweller","category":"special rules","definition":"Ignores difficult terrain.","sources":[{"section":"s","rulebookPage":1}]},
 {"name":"War Drum","category":"wargear","definition":"A drum.","sources":[{"section":"s","rulebookPage":2}]},
 {"name":"Spear","aliases":["Spears"],"category":"weapons","definition":"A spear.","sources":[{"section":"s","rulebookPage":3}]},
 {"name":"Lance","category":"weapons","definition":"A lance.","sources":[{"section":"s","rulebookPage":3}]},
 {"name":"Backpack","category":"wargear","definition":"A pack.","sources":[{"section":"s","rulebookPage":3}]},
 {"name":"Will","category":"heroes","definition":"Will.","sources":[{"section":"s","rulebookPage":3}]}
]}`

func linkedIn(segs []Segment) string {
	var out []string
	for _, s := range segs {
		if s.TermID != "" {
			out = append(out, s.TermID+"="+s.Text)
		}
	}
	return strings.Join(out, ",")
}

func TestLinkExceptSkipsTheItemsOwnName(t *testing.T) {
	g, err := BuildGlossary(RulesetInfo{Name: "G"}, []byte(itemGlossaryDoc))
	if err != nil {
		t.Fatal(err)
	}
	// Two terms in one description; weapon names link in any case, other single words only as spelled.
	if got := linkedIn(g.LinkExcept("Counts as a lance while mounted and a spear while dismounted.", "War spear")); got != "lance=lance,spear=spear" {
		t.Errorf("two terms: %q", got)
	}
	if got := linkedIn(g.LinkExcept("It will not use will.", "Thing")); got != "" {
		t.Errorf("ordinary word: %q", got)
	}
	// The longest phrase wins over its prefix.
	if got := linkedIn(g.LinkExcept("It is a war drum.", "Company drum")); got != "war-drum=war drum" {
		t.Errorf("longest match: %q", got)
	}
	// The item's own name stays plain, in every form; other terms still link.
	for _, name := range []string{"Backpack", "Backpacks", "Backpack (small)"} {
		if got := linkedIn(g.LinkExcept("A model with a Backpack, or the Backpack's owner, uses a spear.", name)); got != "spear=spear" {
			t.Errorf("%s: %q", name, got)
		}
	}
	// A plural name skips its singular term.
	if got := linkedIn(g.LinkExcept("A spear is thrown.", "Spears")); got != "" {
		t.Errorf("plural name: %q", got)
	}
	// The text is kept whole.
	var b strings.Builder
	for _, s := range g.LinkExcept("A model with a Backpack uses a spear.", "Backpack") {
		b.WriteString(s.Text)
	}
	if b.String() != "A model with a Backpack uses a spear." {
		t.Errorf("text: %q", b.String())
	}
	if got := linkedIn(g.LinkExcept("Gives Mountain Dweller.")); got != "mountain-dweller=Mountain Dweller" {
		t.Errorf("no names: %q", got)
	}
}
