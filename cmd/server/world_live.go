package main

import "github.com/iuya/battle-company-manager-ce-v2/internal/core"

const (
	fogName     = "Fog of War: secret notes, retreats and the dead"
	loweredName = "Lowered limit: more companies than the campaign allows now"
)

// liveAs records an action of the company's owner (the author of a secret note, who alone sees it while the match is open).
func (w *world) liveAs(co *wco, m core.Match, a core.LiveAction) error {
	_, _, err := w.c().Apply(w.ctx, co.user, co.camp.id, m.ID, co.id, a)
	return err
}

// fogCampaign plays Fog of War live: both players noted down their secret objectives (a Hero of their own, an enemy Hero and a terrain
// piece), one reported a tier, and units retreated, died and one came back, so the Retreated and Field casualties zones are
// filled and the Tabletop Simulator export of the live match puts those units in a second warband.
func (w *world) fogCampaign() {
	c := w.campaign("you", fogName,
		"An open Fog of War game: each player has secret notes only they see until the match ends, one report of a tier, units that left the table and one that returned, and the Rangers are Broken.", defaultSettings)
	dwarves := w.company(c, "gimli", "Dwarves who keep their counsel", "Durin's Folk", true)
	rangers := w.company(c, "aragorn", "Rangers of the silent road", "Arnor", true)
	for _, co := range []*wco{dwarves, rangers} {
		w.grant(co, 10, 30)
		w.recruitAll(co)
	}
	m := w.newMatchOn(c, "Fog of War", []*wco{dwarves}, []*wco{rangers})

	_, ds, dw := w.roster(dwarves)
	_, rs, rw := w.roster(rangers)
	if len(ds) == 0 || len(rs) == 0 || len(dw) < 2 || len(rw) < 2 {
		w.gap("Fog of War live: the companies have too few units to retreat and fall")
		return
	}
	// secret notes: a Sergeant of their own, a Sergeant of the enemy and a terrain piece, one tier reported
	w.try("Dwarves note their own hero", w.liveAs(dwarves, m, core.LiveAction{Kind: core.LiveSecretNominate, ObjectiveKey: "ownHeroSurvives", UnitID: ds[0].ID}))
	w.try("Dwarves note an enemy hero", w.liveAs(dwarves, m, core.LiveAction{Kind: core.LiveSecretNominate, ObjectiveKey: "enemyHeroHarmed", UnitID: rs[0].ID}))
	w.try("Dwarves note a terrain piece", w.liveAs(dwarves, m, core.LiveAction{Kind: core.LiveSecretNominate, ObjectiveKey: "terrainPiece", Note: "The ruined watchtower"}))
	w.try("Rangers note their own hero", w.liveAs(rangers, m, core.LiveAction{Kind: core.LiveSecretNominate, ObjectiveKey: "ownHeroSurvives", UnitID: rs[len(rs)-1].ID}))
	w.try("Rangers note an enemy hero", w.liveAs(rangers, m, core.LiveAction{Kind: core.LiveSecretNominate, ObjectiveKey: "enemyHeroHarmed", UnitID: ds[len(ds)-1].ID}))
	w.try("Rangers report a tier", w.liveAs(rangers, m, core.LiveAction{Kind: core.LiveSecretDone, ObjectiveKey: "ownHeroSurvives", Tier: 1}))

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
