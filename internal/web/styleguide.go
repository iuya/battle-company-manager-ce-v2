package web

import (
	"net/http"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

type palette struct{ Key, Name string }

type styleGuideView struct {
	pageBase
	Palettes []palette
	Modes    []string
	Keywords []keywordSample
}

// keywordSample is one state of the keyword chip row for the style guide.
type keywordSample struct {
	Label  string
	K      core.UnitKeywords
	Open   string // the Alpine expression that keeps the full list open ("true" shows the expanded card's)
	Static bool
}

func sampleKeywords() []keywordSample {
	w := func(name string, kind core.KeywordKind) core.UnitKeyword {
		return core.UnitKeyword{Name: name, Kind: kind, Concept: "Keywords", Text: name + " is a keyword of the sample unit."}
	}
	elf := core.UnitKeywords{Model: []core.UnitKeyword{w("Elf", core.KeywordRace), w("Lothlórien", core.KeywordFaction), w("Galadhrim", core.KeywordFaction), w("Fellowship", core.KeywordFaction),
		w("Rivendell", core.KeywordFaction), w("Wood Elves", core.KeywordFaction), w("Elite", core.KeywordType), w("Warrior", core.KeywordType), w("Infantry", core.KeywordType)}}
	rider := core.UnitKeywords{Model: []core.UnitKeyword{w("Man", core.KeywordRace), w("Rohan", core.KeywordFaction), w("Hero", core.KeywordType), w("Cavalry", core.KeywordType), w("Unique", core.KeywordType)},
		Mount: "Horse", MountKeys: []core.UnitKeyword{w("Horse", core.KeywordRace), w("Mount", core.KeywordType)}}
	return []keywordSample{
		{Label: "Compact row: race, factions, the types no icon says, then +N", K: elf},
		{Label: "The full list (always open on the expanded card)", K: elf, Open: "true"},
		{Label: "Hero, Cavalry and Mount are the insignia, the icon and the gear: not chips", K: rider, Open: "true"},
		{Label: "The hover card of a unit preview: everything shown, plain", K: elf, Static: true},
	}
}

// handleStyleGuide shows every design token and component in the active theme. Registered only when Config.StyleGuide
// is on, so it can be switched off in production.
func (s *Server) handleStyleGuide(w http.ResponseWriter, r *http.Request) {
	s.render.page(w, r, http.StatusOK, "styleguide", styleGuideView{
		pageBase: s.base(r),
		Palettes: []palette{{"forest", "Forest & Gold"}, {"ember", "Parchment & Ember"}, {"mithril", "Mithril & Night"}},
		Modes:    []string{"system", "light", "dark"},
		Keywords: sampleKeywords(),
	})
}
