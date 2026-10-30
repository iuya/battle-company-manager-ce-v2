package core

import (
	"os"
	"strings"
	"testing"
)

const glossaryDoc = `{"name":"G","version":"1","terms":[
 {"name":"Stand Fast","category":"x","definition":"A model that stands fast ignores Break Tests.\n\nSee Heroic Actions too.","see":["Heroic Action","Nope"],"sources":[{"section":"s","rulebookPage":5}]},
 {"name":"Heroic Action","aliases":["Heroic Actions"],"category":"x","definition":"Heroic Actions can be used by Heroes. A Heroic Action is Stand Fast, among others.","sources":[{"section":"s","rulebookPage":6}]},
 {"name":"Break Test","category":"x","definition":"Roll a dice.","sources":[{"section":"s","rulebookPage":7}]},
 {"name":"Will","category":"x","definition":"Spend Will to resist.","sources":[{"section":"s","rulebookPage":4}]},
 {"name":"Mentions","category":"x","definition":"A hero will spend Will points.","sources":[{"section":"s","rulebookPage":4}]}
]}`

func TestBuildGlossaryLinksTerms(t *testing.T) {
	g, err := BuildGlossary(RulesetInfo{Name: "G"}, []byte(glossaryDoc))
	if err != nil {
		t.Fatal(err)
	}
	stand, _ := g.Term("stand-fast") // terms are sorted, not in file order
	if stand.ID != "stand-fast" || len(stand.Paragraphs) != 2 {
		t.Fatalf("term: %+v", stand)
	}
	// "stands fast" is not the name "Stand Fast" (whole words); "Break Tests" is not "Break Test" either.
	for _, seg := range stand.Paragraphs[0] {
		if seg.TermID != "" {
			t.Errorf("unexpected link %+v", seg)
		}
	}
	var linked []string
	for _, seg := range stand.Paragraphs[1] {
		if seg.TermID != "" {
			linked = append(linked, seg.TermID+"="+seg.Text)
		}
	}
	if strings.Join(linked, ",") != "heroic-action=Heroic Actions" {
		t.Errorf("paragraph 2 links: %v", linked)
	}
	if len(stand.See) != 1 || stand.See[0].ID != "heroic-action" || stand.Pages[0] != 5 {
		t.Errorf("see/pages: %+v %+v", stand.See, stand.Pages)
	}

	// No self links; each target once per definition; the text is preserved.
	heroic, _ := g.Term("heroic-action")
	var text strings.Builder
	counts := map[string]int{}
	for _, para := range heroic.Paragraphs {
		for _, seg := range para {
			text.WriteString(seg.Text)
			if seg.TermID != "" {
				counts[seg.TermID]++
			}
		}
	}
	if counts["heroic-action"] != 0 || counts["stand-fast"] != 1 {
		t.Errorf("links in Heroic Action: %v", counts)
	}
	if !strings.HasPrefix(text.String(), "Heroic Actions can be used") {
		t.Errorf("text changed: %q", text.String())
	}
}

func TestSingleLowerCaseWordsAreNotLinked(t *testing.T) {
	g, _ := BuildGlossary(RulesetInfo{}, []byte(glossaryDoc))
	var linked []string
	mentions, _ := g.Term("mentions")
	for _, seg := range mentions.Paragraphs[0] { // "A hero will spend Will points."
		if seg.TermID != "" {
			linked = append(linked, seg.Text)
		}
	}
	if strings.Join(linked, ",") != "Will" {
		t.Errorf("only the capitalised mention links: %v", linked)
	}
}

func TestGlossaryFindByNameOrAlias(t *testing.T) {
	g, _ := BuildGlossary(RulesetInfo{}, []byte(glossaryDoc))
	if term, ok := g.Find("heroic actions"); !ok || term.ID != "heroic-action" {
		t.Errorf("by alias: %+v %v", term, ok)
	}
	if _, ok := g.Find("Nothing like it"); ok {
		t.Error("unknown name")
	}
}

func TestSummaryAndLink(t *testing.T) {
	g, _ := BuildGlossary(RulesetInfo{}, []byte(glossaryDoc))
	term, _ := g.Term("stand-fast")
	if s := term.Summary(20); len([]rune(s)) > 21 || !strings.HasSuffix(s, "…") {
		t.Errorf("summary: %q", s)
	}
	segs := g.Link("A Heroic Action is great. Heroic Action again.")
	links := 0
	for _, s := range segs {
		if s.TermID == "heroic-action" {
			links++
		}
	}
	if links != 1 {
		t.Errorf("each term is linked once per text: %+v", segs)
	}
}

func TestBuildGlossaryRejectsBadJSON(t *testing.T) {
	if _, err := BuildGlossary(RulesetInfo{}, []byte("{")); err == nil {
		t.Error("expected an error")
	}
}

// The real glossary file builds, and every link points at a term that exists.
func TestBuildRealGlossary(t *testing.T) {
	content, err := os.ReadFile("../../definitions/glossary/mesbg_glossary.json")
	if err != nil {
		t.Skip("definitions not available: ", err)
	}
	g, err := BuildGlossary(RulesetInfo{}, content)
	if err != nil || len(g.Terms) < 100 {
		t.Fatalf("%d terms, %v", len(g.Terms), err)
	}
	ids, links := map[string]bool{}, 0
	for _, term := range g.Terms {
		if ids[term.ID] {
			t.Errorf("duplicate id %q", term.ID)
		}
		ids[term.ID] = true
	}
	for _, term := range g.Terms {
		for _, para := range term.Paragraphs {
			for _, seg := range para {
				if seg.TermID != "" {
					links++
					if !ids[seg.TermID] || seg.TermID == term.ID {
						t.Errorf("%s links to %q", term.Name, seg.TermID)
					}
				}
			}
		}
	}
	if links == 0 {
		t.Error("no links were made")
	}
	t.Logf("%d terms, %d links", len(g.Terms), links)
}

func TestGlossaryIsSortedByCategoryThenName(t *testing.T) {
	doc := `{"terms":[{"name":"Zeta","category":"b","definition":"z"},{"name":"alpha","category":"b","definition":"a"},{"name":"Mid","category":"a","definition":"m"},{"name":"Beta","category":"A","definition":"b"}]}`
	g, _ := BuildGlossary(RulesetInfo{}, []byte(doc))
	var names []string
	for _, term := range g.Terms {
		names = append(names, term.Name)
	}
	if strings.Join(names, ",") != "Beta,Mid,alpha,Zeta" {
		t.Errorf("order: %v", names)
	}
}

func TestLinkingCaseRules(t *testing.T) {
	g, err := BuildGlossary(RulesetInfo{}, []byte(`{"name":"G","version":"1","terms":[
 {"name":"Will","category":"x","aliases":["Willpower"],"definition":"Points."},
 {"name":"Stand Fast","category":"x","definition":"Rally."},
 {"name":"Shoot","category":"x","aliases":["shooting"],"definition":"Fire."},
 {"name":"Probe","category":"y","definition":"[A] will not link. [B] Will links. [C] STAND FAST and stand fast link once. [D] Shooting is not shooting."}
]}`))
	if err != nil {
		t.Fatal(err)
	}
	var linked []string
	probe, _ := g.Term("probe")
	for _, seg := range probe.Paragraphs[0] {
		if seg.TermID != "" {
			linked = append(linked, seg.Text)
		}
	}
	// "will" no; "Will" yes; multi-word case insensitive (first only); alias spelled "shooting" links only in lower case
	if got := strings.Join(linked, ","); got != "Will,STAND FAST,shooting" {
		t.Errorf("linked %q", got)
	}
	if _, ok := g.Find("WILL"); !ok {
		t.Error("Find stays case insensitive")
	}
}

func TestSummaryIsRememberedPerLengthAndEqualsTheFreshText(t *testing.T) {
	g, err := BuildGlossary(RulesetInfo{}, []byte(`{"terms":[{"name":"Alpha","category":"x","definition":"One two three four five six seven eight nine ten.\n\nEleven twelve thirteen."}]}`))
	if err != nil {
		t.Fatal(err)
	}
	term, _ := g.Find("Alpha")
	for _, max := range []int{10, 25, 400} {
		fresh := term.summary(max)
		for range 2 {
			if got := term.Summary(max); got != fresh {
				t.Fatalf("Summary(%d) = %q, want %q", max, got, fresh)
			}
		}
	}
	if (GlossaryTerm{Paragraphs: term.Paragraphs}).Summary(25) != term.summary(25) {
		t.Fatal("a term without a memo must compute the same text")
	}
}
