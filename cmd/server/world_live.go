package main

import "github.com/iuya/battle-company-manager-ce-v2/internal/core"

const (
	fogName        = "Fog of War: retreats, the dead and a Broken company"
	milestonesName = "Break and 25%: how far each company is from them"
	loweredName    = "Lowered limit: more companies than the campaign allows now"
)

// liveAs records an action of the company's owner.
func (w *world) liveAs(co *wco, m core.Match, a core.LiveAction) error {
	_, _, err := w.c().Apply(w.ctx, co.user, co.camp.id, m.ID, co.id, a)
	return err
}

// reverseFormation saves the company's roster back to front, so a player's own order is plain to see beside the default one.
func (w *world) reverseFormation(co *wco) {
	var order []int64
	for _, u := range w.detail(co).Roster {
		order = append([]int64{u.ID}, order...)
	}
	w.try("save a formation", w.c().SaveFormation(w.ctx, co.user, co.camp.id, co.id, order))
}

// fogCampaign plays Fog of War live: units retreated, died and one came back, so the Retreated and Field casualties zones are
// filled and the Tabletop Simulator export of the live match puts those units in a second warband.
func (w *world) fogCampaign() {
	c := w.campaign("you", fogName,
		"An open Fog of War game: units that left the table and one that returned, and the Rangers are Broken.", defaultSettings)
	dwarves := w.company(c, "gimli", "Dwarves who keep their counsel", "Durin's Folk", true)
	rangers := w.company(c, "aragorn", "Rangers of the silent road", "Arnor", true)
	for _, co := range []*wco{dwarves, rangers} {
		w.grant(co, 10, 30)
		w.recruitAll(co)
	}
	w.reverseFormation(dwarves) // the live match follows a saved order inside each zone; the Rangers keep the default one
	m := w.newMatchOn(c, "Fog of War", []*wco{dwarves}, []*wco{rangers})

	_, _, dw := w.roster(dwarves)
	_, _, rw := w.roster(rangers)
	if len(dw) < 2 || len(rw) < 2 {
		w.gap("Fog of War live: the companies have too few units to retreat and fall")
		return
	}
	// zones: a Dwarf left the table, a Ranger fled a courage test, another died, and one Dwarf came back
	w.try("a Dwarf retreats", w.liveAs(dwarves, m, core.LiveAction{Kind: core.LiveRetreat, UnitID: dw[0].ID, Why: "left", Note: "Carried the message away"}))
	w.try("a second Dwarf retreats", w.liveAs(dwarves, m, core.LiveAction{Kind: core.LiveRetreat, UnitID: dw[1].ID, Why: "left"}))
	w.try("it returns", w.liveAs(dwarves, m, core.LiveAction{Kind: core.LiveReturn, UnitID: dw[1].ID}))
	w.try("a Ranger fails a courage test", w.liveAs(rangers, m, core.LiveAction{Kind: core.LiveRetreat, UnitID: rw[0].ID, Why: "courage"}))
	w.try("a Ranger dies", w.liveAs(rangers, m, core.LiveAction{Kind: core.LiveDied, UnitID: rw[1].ID, Note: "Fell from the tower"}))
	// the Rangers lose more Warriors until their casualties exceed the Break Point: Broken at once, for both players to see
	for _, u := range rw[2:] {
		if lc, err := w.c().Live(w.ctx, rangers.user, c.id, rangers.id); err != nil || lc.Break.Broken {
			break
		}
		w.try("a Ranger falls", w.liveAs(rangers, m, core.LiveAction{Kind: core.LiveDied, UnitID: u.ID, Note: "Lost in the fog"}))
	}
	if lc, err := w.c().Live(w.ctx, rangers.user, c.id, rangers.id); err != nil || !lc.Break.Broken {
		w.gap("Fog of War live: the Rangers are not Broken (too few Warriors to lose)")
	}
	w.show(dwarves)
	w.show(rangers)
}

// loweredLimit shows a campaign whose limit was lowered after companies existed (Campaigns.SetCompaniesPerPlayer): the limit was
// 3 when Galadriel built three companies and Éowyn two, and is 2 now. Nothing is taken away; Galadriel is above the limit, Éowyn at
// it, and neither can create another company.
func (w *world) loweredLimit() {
	set := defaultSettings
	set.CompaniesPerPlayer = 3
	c := w.campaign("you", loweredName,
		"The organiser allowed three companies per player, then lowered the limit to two. Galadriel keeps her three (and cannot make more), Éowyn is at the new limit, Aragorn has one and may add another.", set)
	w.company(c, "galadriel", "Wardens of the first light", "Lothlorien", true)
	w.company(c, "galadriel", "Wardens of the second light", "Rivendell", true)
	w.company(c, "galadriel", "Wardens of the third light", "Mirkwood", true)
	w.company(c, "eowyn", "Riders of the east march", "Rohan", true)
	w.company(c, "eowyn", "Riders of the west march", "Helm's Deep", true)
	w.company(c, "aragorn", "Rangers of the north", "Arnor", true)
	w.must("lower the limit", w.c().SetCompaniesPerPlayer(w.ctx, c.org, c.id, 2))
}

// breakMilestonesCampaign holds the Break Point panel's states side by side, on a scenario that ends at 25%: one company has lost a few,
// one is about to break with models gone from the table (the 25% has moved closer than the Break), and one has reached the 25%.
func (w *world) breakMilestonesCampaign() {
	c := w.campaign("you", milestonesName,
		"Seize Ground ends at 25%. Gondor has lost a few, Rohan has models off the table that do not count, Arnor reached the 25% with casualties and a failed Courage test.", defaultSettings)
	gondor := w.company(c, "you", "Gondor", "Minas Tirith", true)
	rohan := w.company(c, "eowyn", "Rohan", "Rohan", true)
	arnor := w.company(c, "aragorn", "Arnor", "Arnor", true)
	dale := w.company(c, "gimli", "Dale", "Erebor & Dale", true)
	for _, co := range []*wco{gondor, rohan, arnor, dale} {
		w.grant(co, 10, 30)
		w.recruitAll(co)
	}
	m1 := w.newMatchOn(c, "Seize Ground", []*wco{gondor}, []*wco{rohan})
	m2 := w.newMatchOn(c, "Seize Ground", []*wco{arnor}, []*wco{dale})
	fall := func(co *wco, m core.Match, n int) {
		_, _, warriors := w.roster(co)
		for i := 0; i < n && i < len(warriors); i++ {
			w.try("a warrior falls", w.liveAs(co, m, core.LiveAction{Kind: core.LiveDied, UnitID: warriors[i].ID, Note: "Fell in the line"}))
		}
	}
	fall(gondor, m1, 3)
	fall(rohan, m1, 2)
	_, _, rw := w.roster(rohan)
	for i := 2; i < 4 && i < len(rw); i++ {
		w.try("a rider leaves the table", w.liveAs(rohan, m1, core.LiveAction{Kind: core.LiveRetreat, UnitID: rw[i].ID, Why: "left"}))
	}
	_, _, aw := w.roster(arnor)
	fall(arnor, m2, len(aw)-1)
	w.try("the last Warrior fails a courage test", w.liveAs(arnor, m2, core.LiveAction{Kind: core.LiveRetreat, UnitID: aw[len(aw)-1].ID, Why: "courage"}))
	if lc, err := w.c().Live(w.ctx, arnor.user, c.id, arnor.id); err != nil || lc.Break.ToEnd() > 0 {
		w.gap("Break and 25%%: Arnor has too few Warriors to reach it, the Heroes stay on the table")
	}
}
