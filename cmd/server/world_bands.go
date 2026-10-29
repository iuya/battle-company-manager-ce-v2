package main

import "github.com/iuya/battle-company-manager-ce-v2/internal/core"

// bandsCampaign is the campaign where a player may have several companies (docs/multi-company-spec.md): the setting is 3, Galadriel
// has all three, Éowyn two (so she may create another), Gimli and Sauron one. Éowyn's two companies play on one side against Sauron
// in a match that ended, one aftermath finished and the other still to play (each is locked on its own); Galadriel's first company is
// in an open match while her other two are free. The chip switcher shows for Éowyn and Galadriel, not for Gimli or Sauron.
func (w *world) bandsCampaign() {
	set := defaultSettings
	set.CompaniesPerPlayer = 3
	c := w.campaign("you", "Two bands: several companies per player",
		"Each player may have up to three companies. Éowyn has two (and may create a third), Galadriel has all three, Gimli and Sauron one. One match has two of Éowyn's companies on the same side; another has one of Galadriel's while her other two are free.", set)
	mark := w.company(c, "eowyn", "Eorlings of the Mark", "Rohan", true)
	deep := w.company(c, "eowyn", "Shieldmaidens of the Deep", "Helm's Deep", true)
	wood := w.company(c, "galadriel", "Wardens of the Golden Wood", "Lothlorien", true)
	vale := w.company(c, "galadriel", "Scouts of the Hidden Vale", "Rivendell", true)
	green := w.company(c, "galadriel", "Sentries of the Greenwood", "Mirkwood", true)
	mountain := w.company(c, "gimli", "Longbeards of the Lonely Mountain", "Durin's Folk", true)
	gate := w.company(c, "sauron", "Vanguard of the Black Gate", "Mordor", true)
	for _, co := range []*wco{mark, deep, wood, vale, green, mountain, gate} {
		w.grant(co, 8, 10)
	}

	// A practice match between Éowyn's own two companies (the "Same player" case): nothing to roll, so neither is locked afterwards
	w.playedPractice(c, "Chance Encounter", outcome{winner: 1, vp: []int{4, 2}, note: "Éowyn trained her two bands against each other."}, []*wco{mark}, []*wco{deep})

	// Éowyn's two companies fight on one side; each has its own aftermath, one done and one still to play
	m := w.played(c, "Seize Ground", outcome{winner: 1, vp: []int{6, 3}, note: "Both of Éowyn's bands took the field."}, []*wco{mark, deep}, []*wco{gate})
	w.settle(mark, m, w.sheet(mark), nil, true)

	// Galadriel's first company is in an open match; her other two are free, as is Éowyn's pair now locked by its aftermath
	w.newMatch(c, []*wco{wood}, []*wco{mountain})

	// An open practice match between two of Galadriel's other companies, with a live log: a turn and a point of Might spent
	open := w.newPracticeMatch(c, []*wco{vale}, []*wco{green})
	w.must("start turn 1", w.liveApply(vale, open, core.LiveAction{Kind: core.LiveTurn, Turn: 1}))
	if hero, ok := w.unit(vale, func(u core.CompanyUnit) bool { return u.IsHero() }); ok {
		w.must("spend Might", w.liveApply(vale, open, core.LiveAction{Kind: core.LiveStatChange, UnitID: hero.ID, Stat: "might", Delta: -1}))
	}
	w.show(mark)
	w.show(wood)
}

// liveApply records an action of the match's organiser for the company.
func (w *world) liveApply(co *wco, m core.Match, a core.LiveAction) error {
	_, _, err := w.c().Apply(w.ctx, co.camp.org, co.camp.id, m.ID, co.id, a)
	return err
}
