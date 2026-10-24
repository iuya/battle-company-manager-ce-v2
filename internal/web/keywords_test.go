package web

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func kw(name string, kind core.KeywordKind, concept string) core.UnitKeyword {
	return core.UnitKeyword{Name: name, Kind: kind, Concept: concept, Text: name + " text", Hidden: kind == core.KeywordType && (name == "Hero" || name == "Warrior" || name == "Infantry")}
}

// rohanRider is a Rohan hero with a bought Horse: race, factions, types (some hidden), and the mount's own keywords.
func rohanRider() core.UnitKeywords {
	return core.UnitKeywords{
		Model: []core.UnitKeyword{
			kw("Man", core.KeywordRace, "Keywords"), kw("Rohan", core.KeywordFaction, "Keywords"), kw("Edoras", core.KeywordFaction, "Keywords"),
			kw("Hero", core.KeywordType, "Hero"), kw("Infantry", core.KeywordType, "Infantry"), kw("Cavalry", core.KeywordType, "Cavalry"),
		},
		Mount: "Horse", MountKeys: []core.UnitKeyword{kw("Horse", core.KeywordRace, "Keywords"), kw("Mount", core.KeywordType, "Mount")},
	}
}

func renderKeywordRow(t *testing.T, data map[string]any) string {
	t.Helper()
	r, err := newRenderer()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := r.pages["campaign_my_company"].ExecuteTemplate(&buf, "keyword_row", data); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

var chipRe = regexp.MustCompile(`<li class="kw-chip[^"]*"[^>]*>([^<]*)</li>`)

// chips are the names in the compact row (the first list of the block).
func compactRow(t *testing.T, html string) (chips []string, tail string) {
	t.Helper()
	i := strings.Index(html, `aria-label="Keywords"`)
	j := strings.Index(html, `class="kw-full"`)
	if i < 0 || j < 0 {
		t.Fatalf("no compact row or full list in %s", html)
	}
	for _, m := range chipRe.FindAllStringSubmatch(html[i:j], -1) {
		chips = append(chips, m[1])
	}
	return chips, html[i:j]
}

func TestKeywordRowIsRaceFirstFiveChipsAndMore(t *testing.T) {
	k := core.UnitKeywords{Model: []core.UnitKeyword{
		kw("Elf", core.KeywordRace, "Keywords"), kw("Lothlórien", core.KeywordFaction, "Keywords"), kw("Galadhrim", core.KeywordFaction, "Keywords"),
		kw("Rivendell", core.KeywordFaction, "Keywords"), kw("Fellowship", core.KeywordFaction, "Keywords"), kw("Dunedain", core.KeywordFaction, "Keywords"),
		kw("Elite", core.KeywordType, "Elite"), kw("Warrior", core.KeywordType, "Warrior"),
	}}
	html := renderKeywordRow(t, map[string]any{"K": k})
	chips, row := compactRow(t, html)
	if got := strings.Join(chips, ","); got != "Elf,Lothlórien,Galadhrim,Rivendell,Fellowship" {
		t.Errorf("compact row = %s", got)
	}
	if !strings.Contains(row, `>+2<span class="sr-only"> more keywords, show all</span>`) {
		t.Errorf("+2 button missing (Dunedain, Elite): %s", row)
	}
	if !strings.Contains(row, `<button type="button" class="kw-chip kw-more"`) || !strings.Contains(row, `:aria-expanded="all || false"`) {
		t.Errorf("+N is a button with aria-expanded: %s", row)
	}
	full := html[strings.Index(html, `class="kw-full"`):]
	for _, want := range []string{"<dt>Race</dt>", "<dt>Faction</dt>", "<dt>Unit type</dt>", ">Dunedain<", ">Elite<"} {
		if !strings.Contains(full, want) {
			t.Errorf("full list lacks %q", want)
		}
	}
	if strings.Contains(html, ">Warrior<") {
		t.Errorf("Warrior is not shown anywhere")
	}
	if !strings.Contains(html, `x-show="all || false"`) || !strings.Contains(html, "x-cloak") {
		t.Errorf("the full list opens with +N and starts hidden")
	}
}

// The obvious keywords are drawn nowhere, neither in the row nor in the full list: Hero and Warrior (frame, insignia), Mount (the gear), and the
// movement types the kind icon says. Race, factions and the types with no icon stay.
func TestKeywordsTheIconAndInsigniaSayAreNotShown(t *testing.T) {
	k := core.UnitKeywords{Model: []core.UnitKeyword{
		kw("Man", core.KeywordRace, "Keywords"), kw("Rohan", core.KeywordFaction, "Keywords"),
		kw("Hero", core.KeywordType, "Hero"), kw("Warrior", core.KeywordType, "Warrior"), kw("Infantry", core.KeywordType, "Infantry"), kw("Cavalry", core.KeywordType, "Cavalry"),
		kw("Beast", core.KeywordType, "Beast"), kw("War Beast", core.KeywordType, "War Beast"), kw("Monster", core.KeywordType, "Monster"), kw("Mount", core.KeywordType, "Mount"),
		kw("Unique", core.KeywordType, "Unique"), kw("Elite", core.KeywordType, "Elite"), kw("Swarm", core.KeywordType, "Swarm"), kw("Chariot", core.KeywordType, "Chariot"),
	}, Mount: "Horse", MountKeys: []core.UnitKeyword{kw("Horse", core.KeywordRace, "Keywords")}}
	for name, data := range map[string]map[string]any{"live": {"K": k, "Open": "expanded === 3"}, "static": {"K": k, "Static": true}} {
		html := renderKeywordRow(t, data)
		for _, gone := range []string{">Hero<", ">Warrior<", ">Infantry<", ">Cavalry<", ">Beast<", ">War Beast<", ">Monster<", ">Mount<", "Mount:", ">Horse<", "kw-off"} {
			if strings.Contains(html, gone) {
				t.Errorf("%s: %q is drawn", name, gone)
			}
		}
		for _, want := range []string{">Man<", ">Rohan<", ">Unique<", ">Elite<", ">Swarm<", ">Chariot<"} {
			if !strings.Contains(html, want) {
				t.Errorf("%s: %q is missing", name, want)
			}
		}
	}
	// core still computes them (rule targets need them): only the web layer filters
	if len(k.Model) != 14 {
		t.Fatal("the set was changed")
	}
	// a unit with nothing to show draws no row
	only := core.UnitKeywords{Model: []core.UnitKeyword{kw("Hero", core.KeywordType, "Hero"), kw("Infantry", core.KeywordType, "Infantry")}}
	if got := strings.TrimSpace(renderKeywordRow(t, map[string]any{"K": only})); got != "" {
		t.Errorf("no chips, no row: %q", got)
	}
}

func TestKeywordChipsHaveTooltipsConceptsAndGroupLabels(t *testing.T) {
	html := renderKeywordRow(t, map[string]any{"K": rohanRider(), "Open": "expanded === 7"})
	chips, _ := compactRow(t, html)
	if got := strings.Join(chips, ","); got != "Man,Rohan,Edoras" {
		t.Errorf("row: %s", got)
	}
	for _, want := range []string{
		`aria-label="Race keyword: Man" tabindex="0" data-concept="Keywords" data-tip-title="Man" data-tip-meta="Race keyword" data-tip-text="Man text"`,
		`aria-label="Faction keyword: Rohan"`,
		`x-show="!(all || expanded === 7)"`, // the row hides when the card is expanded: the full list is always open there
		`x-show="all || expanded === 7"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("lacks %q\n%s", want, html)
		}
	}
	if strings.Contains(html, "onclick") || strings.Contains(html, "<script") {
		t.Errorf("no inline script or handler")
	}
	elite := core.UnitKeywords{Model: []core.UnitKeyword{kw("Elite", core.KeywordType, "Elite")}}
	h := renderKeywordRow(t, map[string]any{"K": elite})
	if !strings.Contains(h, `aria-label="Unit type keyword: Elite" tabindex="0" data-concept="Elite"`) || !strings.Contains(h, `data-tip-meta="Unit type"`) {
		t.Errorf("a type chip opens its own term: %s", h)
	}
}

func TestKeywordRowEmptyStaticAndEscaping(t *testing.T) {
	if got := strings.TrimSpace(renderKeywordRow(t, map[string]any{"K": core.UnitKeywords{}})); got != "" {
		t.Errorf("an empty set draws nothing: %q", got)
	}
	static := renderKeywordRow(t, map[string]any{"K": rohanRider(), "Static": true})
	if strings.Contains(static, "x-data") || strings.Contains(static, "<button") || strings.Contains(static, "data-tip") || strings.Contains(static, "x-show") {
		t.Errorf("the hover card list is plain and always open: %s", static)
	}
	bad := core.UnitKeywords{Model: []core.UnitKeyword{{Name: hostile, Kind: core.KeywordRace, Concept: hostile, Text: hostile}}}
	h := renderKeywordRow(t, map[string]any{"K": bad})
	if strings.Contains(h, "<img") || strings.Contains(h, "<script") {
		t.Errorf("not escaped: %s", h)
	}
}
