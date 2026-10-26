package main

import (
	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// defaultSettings are what most demo campaigns use: players may type in their rolls or throw the dice.
var defaultSettings = core.CampaignSettings{ShakeTheTable: true, ManualRolls: true}

// choosePathOf gives one officer the idx-th path it may take (no Sorcerer: that one needs a power).
func (w *world) choosePathOf(co *wco, u core.CompanyUnit, idx int) {
	offer, err := w.c().PathOptions(w.ctx, co.user, co.camp.id, u.ID)
	w.must("path options", err)
	var open []core.PathOption
	for _, p := range offer.Paths {
		if p.Blocked == "" && !p.StartPower {
			open = append(open, p)
		}
	}
	w.must("choose path for "+u.Title(), w.c().ChoosePath(w.ctx, co.user, co.camp.id, u.ID, open[idx%len(open)].Name, ""))
}

// customHeroes picks the last three units that may lead as the leader and the sergeants (not the company's suggestion).
func (w *world) customHeroes(c *wcamp, faction string) []core.StartHero {
	profiles, err := w.c().CompanyProfiles(w.ctx, c.id)
	w.must("profiles", err)
	for _, p := range profiles {
		if p.Name != faction {
			continue
		}
		var idx []int
		for i, s := range p.StartList() {
			if p.CanLeadStart(s) {
				idx = append(idx, i)
			}
		}
		if len(idx) < 3 {
			return nil
		}
		n := len(idx)
		pick := []core.StartHero{{Index: idx[n-1], Role: core.RoleLeader}, {Index: idx[n-2], Role: core.RoleSergeant}, {Index: idx[n-3], Role: core.RoleSergeant}}
		if _, err := p.ValidateStart(pick); err != nil { // the company needs a given unit among its officers (a hero by profile): its suggestion is used
			return nil
		}
		return pick
	}
	return nil
}

func (w *world) stepsCampaigns() {
	// 1. brand new: no companies; one player joined, one invite link is waiting
	c1 := w.campaign("you", worldMarker, "Nobody has built a company yet. Open the invite link, create a company, see the empty ranking.", defaultSettings)
	w.join(c1, "aragorn")
	_, err := w.c().Invite(w.ctx, c1.org, c1.id)
	w.must("invite", err)

	// 2. companies being built
	c2 := w.campaign("you", "Companies being built: paths still to choose",
		"Five players at the start: officers without a path, one path chosen, a ready company, a custom choice of officers, and a player who has not made a company.", defaultSettings)
	a := w.company(c2, "eowyn", "Éowyn: no paths yet", "Rohan", false)
	b := w.company(c2, "sauron", "Sauron: leader only", "Mordor", false)
	w.choosePathOf(b, w.mustUnit(b, "leader", isRole(core.RoleLeader)), 0)
	r := w.company(c2, "aragorn", "Aragorn: ready to play", "Arnor", true)
	_ = a
	_ = r
	gimli := w.user("gimli")
	w.join(c2, "gimli")
	_, err = w.c().CreateCompany(w.ctx, gimli, c2.id, core.CompanyInput{Name: "Gimli: my own officers", Faction: "Durin's Folk", Heroes: w.customHeroes(c2, "Durin's Folk")})
	w.must("custom officers", err)
	w.join(c2, "bilbo") // joined, no company
	w.grant(a, 6, 4)
	w.grant(b, 6, 4)
	w.grant(r, 6, 4)
}
