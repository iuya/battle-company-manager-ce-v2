package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// sql runs a statement the services cannot express (no service finishes a campaign, sets its round, or ages an invite); every
// use is listed in docs/demo-world.md under "Gaps".
func (w *world) sql(why, query string, args ...any) {
	if w.exec == nil {
		w.gap("%s: no database connection for raw SQL (%s)", why, query)
		return
	}
	w.must("raw SQL for "+why, w.exec(w.ctx, query, args...))
}

// rollLevelServer makes a level-up roll the server rolls (campaigns without typed rolls): the dice are fixed.
func (w *world) rollLevelServer(co *wco, u core.CompanyUnit, dice ...int) core.LevelRoll {
	w.rollWith(dice...)
	r, err := w.c().RollLevelUp(w.ctx, co.user, co.camp.id, u.ID, nil, false)
	w.must("level-up roll of "+u.Title(), err)
	return r
}

func (w *world) settingsCampaigns() {
	// server rolls only: no typing of rolls, no throwing of dice; the demo fixes the server's dice
	srv := w.campaign("you", "Settings: the server rolls every die",
		"ShakeTheTable and ManualRolls are off: every roll panel has only a Roll button. The demo fixed the server's dice, so the results are always the same.", core.CampaignSettings{})
	ro := w.company(srv, "eowyn", "Riders who trust the server", "Rohan", true)
	or := w.company(srv, "sauron", "Orcs who trust the server", "Mordor", true)
	w.grant(ro, 10, 30)
	w.grant(or, 10, 30)
	if s, err := w.recruit(ro, 0, 4); err == nil {
		fmt.Fprintln(w.out, "  ", s)
	}
	_, _, ws := w.roster(ro)
	w.xp(ro, ws[0].ID, 5, "Won the duel at the ford")
	w.rollLevelServer(ro, ws[0], 4) // a promotion to choose, or nothing
	m := w.played(srv, "Fog of War", outcome{winner: 1, vp: []int{3, 1}}, []*wco{ro}, []*wco{or})
	l, _, ows := w.roster(or)
	in := w.sheet(or)
	set(&in, l.ID, core.OutcomeCasualty, nil)
	set(&in, ows[0].ID, core.OutcomeCasualty, nil)
	w.settle(or, m, in, map[int64][3]int{l.ID: {3, 4, 1}, ows[0].ID: {2, 3, 1}}, true)
	w.show(or)

	// heroes may drop the gear of their profile
	drop := w.campaign("you", "Settings: heroes drop profile gear",
		"HeroesDropProfileGear is on: a hero can drop the equipment its unit profile came with (see the Equipment card of a hero).",
		core.CampaignSettings{ShakeTheTable: true, ManualRolls: true, HeroesDropProfileGear: true})
	gd := w.company(drop, "aragorn", "Rangers who travel light", "Arnor", true)
	l, _, _ = w.roster(gd)
	for _, e := range l.Equipment {
		if e.Profile && e.Drop == "" {
			w.try("drop "+e.Name, w.c().DropEquipment(w.ctx, gd.user, drop.id, l.ID, e.Name))
			break
		}
	}
	w.show(gd)

	// everything on, with the custom path for the units that are On Their Own Path
	all := w.campaign("you", "Settings: everything on, own paths level up",
		"ShakeTheTable, ManualRolls, HeroesDropProfileGear and CustomOnTheirOwnPath are on: units On Their Own Path earn experience and level up as heroes.",
		core.CampaignSettings{ShakeTheTable: true, ManualRolls: true, HeroesDropProfileGear: true, CustomOnTheirOwnPath: true})
	oc := w.company(all, "galadriel", "Own-path Elves", "Lothlorien", true)
	w.grant(oc, 10, 30)
	w.recruitAll(oc)
	if u, ok := w.unit(oc, func(u core.CompanyUnit) bool { return u.Path != nil && u.Path.Name == "Path of Their Own Making" }); ok {
		w.xp(oc, u.ID, 10, "Walked its own way")
	} else {
		w.gap("no unit of %s is on its own path with the custom setting", oc.name)
	}
	w.show(oc)

	// a campaign pinned to another edition of the Battle Companies rules: a fork of the app's file with its own version
	w.forkedEdition()

	// the finished campaign, and rounds in the others
	w.finishedCampaign()
	w.sql("rounds of the aftermath ward", `UPDATE campaigns SET round = 3, rounds = 12 WHERE name LIKE 'Aftermath ward 1%'`)
	w.must("pause the campaign", w.c().SetCampaignStatus(w.ctx, drop.org, drop.id, core.CampaignPaused, ""))
	w.sql("rounds of a paused campaign", `UPDATE campaigns SET round = 5, rounds = 8 WHERE name = $1`, "Settings: heroes drop profile gear")
}

// forkedEdition stores a copy of the Battle Companies file under another version and plays a campaign with it.
func (w *world) forkedEdition() {
	raw, err := os.ReadFile("definitions/battle_companies/battle_companies.json")
	w.must("read the Battle Companies file", err)
	var doc map[string]json.RawMessage
	w.must("parse the Battle Companies file", json.Unmarshal(raw, &doc))
	doc["name"] = json.RawMessage(`"Battle Companies 2nd CE (demo fork)"`)
	doc["version"] = json.RawMessage(`"2.1-demo"`)
	fork, err := json.Marshal(doc)
	w.must("fork", err)
	hash, err := core.RulesetHash(fork)
	w.must("hash", err)
	info, err := w.b.rulesets.StoreAs(w.ctx, w.user("admin"), core.KindBattleCompanies, hash, fork)
	w.must("store the forked file", err)
	c := w.campaignWith("you", "Pinned to another edition: Battle Companies 2.1-demo",
		"This campaign plays with a forked copy of the Battle Companies file (version 2.1-demo): the Rules card names the file and its short hash.", defaultSettings,
		core.CampaignInput{BattleCompanies: info.Hash})
	w.company(c, "gimli", "Dwarves of the other edition", "Iron Hills", true)
	w.company(c, "bilbo", "Hobbits of the other edition", "The Shire", true)
}

// finishedCampaign plays a short campaign to its end: two companies, two games with their aftermaths done.
func (w *world) finishedCampaign() {
	c := w.campaign("you", "Finished campaign: the Siege of Osgiliath",
		"Played to its end: the ranking is final, nothing can be granted or invited any more.", defaultSettings)
	a := w.company(c, "faramir", "Rangers of Ithilien", "Osgiliath & Ithilien", true)
	b := w.company(c, "sauron", "Orc raiders of the river", "Minas Morgul", true)
	for _, co := range []*wco{a, b} {
		w.grant(co, 6, 12)
	}
	for i, win := range []int{1, 2} {
		m := w.played(c, []string{"Seize Ground", "Plunder the Supplies"}[i], outcome{winner: win, vp: []int{4 + i, 3}}, []*wco{a}, []*wco{b})
		for _, co := range []*wco{a, b} {
			_, _, ws := w.roster(co)
			in := w.sheet(co)
			set(&in, ws[0].ID, core.OutcomeCasualty, nil)
			w.settle(co, m, in, map[int64][3]int{ws[0].ID: {3, 4, 1}}, true)
		}
	}
	w.must("end the campaign", w.c().SetCampaignStatus(w.ctx, c.org, c.id, core.CampaignFinished, c.name))
	w.sql("the rounds of a finished campaign", `UPDATE campaigns SET round = 12, rounds = 12 WHERE id = $1`, c.id)
}
