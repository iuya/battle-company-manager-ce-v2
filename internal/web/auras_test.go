package web

import (
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestAuraChipWords(t *testing.T) {
	a := core.Aura{ID: "standfast-6", Kind: "command", Radius: 6, Affects: "friendly", When: core.AuraAlways, Group: core.AuraGroupAlways, LOS: true, Label: "Stand Fast, With Me!", Tip: "x", Concept: "Stand Fast"}
	if got := auraText(a); got != `Aura 6"` {
		t.Errorf("text: %q", got)
	}
	if got := auraMeta(a); got != "Command · friendly models · always · needs line of sight" {
		t.Errorf("meta: %q", got)
	}
	if got := auraLabel(a); got != "Aura, 6 inches: Stand Fast, With Me!; works for friendly models, always, needs line of sight" {
		t.Errorf("label: %q", got)
	}
	if auraOff(a) || auraThick(a) || auraLetter(a) != "C" {
		t.Error("an always aura is on by default, not thick, and has the letter of its kind")
	}
	horn := core.Aura{Kind: "enemy", Radius: 4.5, Group: core.AuraGroupLarge, Affects: "enemy", When: core.AuraCast, Keywords: []string{"Orc"}}
	if !auraOff(horn) || !auraThick(horn) || auraText(horn) != `Aura 4.5"` || !strings.Contains(auraMeta(horn), "off by default") || !strings.Contains(auraMeta(horn), "enemy models (Orc)") {
		t.Errorf("an off-by-default enemy aura: %q", auraMeta(horn))
	}
}

func TestAuraRowKeepsThreeChipsAndCountsTheRest(t *testing.T) {
	as := make([]core.Aura, 5)
	if v := auraRow(as, 0); len(v.Shown) != 3 || len(v.Rest) != 2 {
		t.Errorf("3 and +2: %d %d", len(v.Shown), len(v.Rest))
	}
	if v := auraRow(as[:3], 3); len(v.Shown) != 3 || len(v.Rest) != 0 {
		t.Error("three auras need no +N")
	}
	if v := auraRow(nil, 3); len(v.Shown) != 0 {
		t.Error("none")
	}
}

func renderAuraRow(t *testing.T, as []core.Aura) string {
	t.Helper()
	r, err := newRenderer()
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := r.pages["campaign_my_company"].ExecuteTemplate(&buf, "aura_row", as); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestAuraRowDrawsChipsLikeRuleChips(t *testing.T) {
	as := []core.Aura{
		{ID: "banner-3", Kind: "banner", Radius: 3, Affects: "friendly", When: core.AuraAlways, Group: core.AuraGroupAlways, Label: "Company Standard", Tip: "Re-roll Duel dice of 1.", Concept: "Company Standard"},
		{ID: "standfast-6", Kind: "command", Radius: 6, Affects: "friendly", When: core.AuraAlways, Group: core.AuraGroupAlways, LOS: true, Label: "Stand Fast, With Me!", Tip: "Stand Fast.", Concept: "Stand Fast"},
		{ID: "tirade-6", Kind: "enemy", Radius: 6, Affects: "enemy", When: core.AuraAlways, Group: core.AuraGroupAlways, Label: "Furious Tirade", Tip: "t"},
		{ID: "voice-12", Kind: "command", Radius: 12, Affects: "friendly", When: core.AuraAlways, Group: core.AuraGroupAlways, Label: "Voice of Command", Tip: "v", Concept: "Voice of Command"},
		{ID: "horn-24", Kind: "command", Radius: 24, Affects: "friendly", When: core.AuraAlways, Group: core.AuraGroupLarge, Label: "Company Horn", Tip: "h"},
	}
	html := renderAuraRow(t, as)
	for _, want := range []string{`aria-label="Auras"`, `class="aura-chip aura-k-banner has-tip"`, `tabindex="0"`, `data-concept="Company Standard"`, `data-tip-title="Stand Fast, With Me!"`,
		`aria-label="Aura, 3 inches: Company Standard; works for friendly models, always"`, `Aura 3&#34;`, `data-tip-meta="Command · friendly models · always · needs line of sight"`,
		"aura-ring-thick", `>+2<span class="sr-only"> more auras, show all</span>`, `:aria-expanded="more"`, `x-show="more"`, "aura-off"} {
		if !strings.Contains(html, want) {
			t.Errorf("the row lacks %q", want)
		}
	}
	if n := strings.Count(html, `class="aura-chip`); n != 5 {
		t.Errorf("all five chips are in the markup (two behind +N): %d", n)
	}
	if got := renderAuraRow(t, nil); strings.TrimSpace(got) != "" {
		t.Errorf("a unit without an aura has no row: %q", got)
	}
}
