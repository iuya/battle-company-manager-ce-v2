package main

import (
	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// roster returns the units of the company by role: leader, sergeants, warriors (not injured, not wanderers).
func (w *world) roster(co *wco) (leader core.CompanyUnit, sergeants, warriors []core.CompanyUnit) {
	for _, u := range w.detail(co).Roster {
		switch {
		case u.Role == core.RoleLeader:
			leader = u
		case u.Role == core.RoleSergeant:
			sergeants = append(sergeants, u)
		case u.Role == core.RoleWarrior && !u.Wanderer && !u.OwnPath:
			warriors = append(warriors, u)
		}
	}
	return
}

// injuryCampaign plays one match with a hero of every kind of wound and warriors hurt or killed, then treats some of them.
func (w *world) injuryCampaign() {
	c := w.campaign("you", "Injuries and treatment: every wound on the chart",
		"Warriors injured, killed or lucky; heroes with arm and leg wounds, broken honour, a Scratch; treatments done, failed, near-missed (waiting for extra influence) and not affordable.", defaultSettings)
	rohan := w.company(c, "eowyn", "Wounded Riders of the Mark", "Rohan", true)
	mordor := w.company(c, "sauron", "Scarred Orcs of the Black Gate", "Mordor", true)
	arnor := w.company(c, "aragorn", "Retired Rangers of Arnor", "Arnor", true)
	w.grant(rohan, 10, 40)
	w.recruitAll(rohan)
	w.grant(rohan, 0, 8)
	w.grant(mordor, 0, 1)
	w.grant(arnor, 0, 6)

	// the Rangers of Arnor sit out the match: one hero got an arm wound long ago, another retires with a second leg wound
	_, sg, wr := w.roster(arnor)
	_, err := w.c().Injure(w.ctx, c.id, arnor.id, sg[0].ID, "armWound", "an old skirmish")
	w.must("arm wound", err)
	_, err = w.c().Injure(w.ctx, c.id, arnor.id, sg[1].ID, "legWound", "an old skirmish")
	w.must("leg wound", err)
	retired, err := w.c().Injure(w.ctx, c.id, arnor.id, sg[1].ID, "legWound", "Match 1")
	w.must("second leg wound", err)
	if !retired {
		w.gap("the second leg wound did not retire the hero")
	}
	w.must("bury a warrior", w.c().Bury(w.ctx, c.id, arnor.id, wr[0].ID, "Dead", "an old skirmish"))

	m := w.played(c, "Seize Ground", outcome{winner: 1, vp: []int{5, 2}, note: "Rohan held three of the five markers."}, []*wco{rohan}, []*wco{mordor})

	// Rohan: every kind of result on the heroes' and the warriors' chart
	l, s, ws := w.roster(rohan)
	in := w.sheet(rohan)
	set(&in, l.ID, core.OutcomeCasualty, map[string]int{"woundedEnemy": 1})
	set(&in, s[0].ID, core.OutcomeCasualty, nil)
	set(&in, s[1].ID, core.OutcomeCasualty, nil)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[1].ID, core.OutcomeCasualty, nil)
	set(&in, ws[2].ID, core.OutcomeCasualty, nil)
	set(&in, ws[3].ID, core.OutcomeCasualty, nil)
	set(&in, ws[4].ID, core.OutcomeCasualty, nil)
	w.settle(rohan, m, in, map[int64][3]int{
		l.ID: {1, 2, 1}, s[0].ID: {2, 2, 1}, s[1].ID: {3, 2, 1}, // arm wound, leg wound, broken honour
		ws[0].ID: {2, 2, 1}, ws[1].ID: {1, 1, 1}, ws[2].ID: {6, 6, 5}, ws[3].ID: {3, 3, 1}, ws[4].ID: {2, 3, 1}, // injured, dead, lesson learned (+3), recovered, injured
	}, true)

	// Mordor: a Scratch (kept), the Wounds of a Hero, an injured warrior
	l2, s2, w2 := w.roster(mordor)
	in = w.sheet(mordor)
	set(&in, l2.ID, core.OutcomeCasualty, nil)
	set(&in, s2[0].ID, core.OutcomeCasualty, nil)
	set(&in, w2[0].ID, core.OutcomeCasualty, nil)
	a := w.settle(mordor, m, in, map[int64][3]int{l2.ID: {3, 3, 1}, s2[0].ID: {6, 6, 4}, w2[0].ID: {2, 3, 1}}, false)
	a = w.decide(mordor, m, core.AftermathAnswer{Kind: "scratch", UnitID: l2.ID, Answer: "keep"})
	if a.Status == core.AftermathInjuries {
		w.decide(mordor, m, core.AftermathAnswer{Kind: "continue"})
	}
	w.finishAftermath(mordor, m)

	_ = err

	// treatments by Rohan: a warrior is treated, broken honour goes to the healers, a leg wound is a near miss waiting for
	// extra influence, an arm wound stays
	_, s, ws = w.roster(rohan)
	w.treatWarriors(rohan)
	for _, u := range w.detail(rohan).Roster {
		for _, inj := range u.Injuries {
			switch inj.Key {
			case "brokenHonour":
				_, err := w.c().TreatInjury(w.ctx, rohan.user, c.id, u.ID, core.Treatment{Method: "healers", Injury: "brokenHonour"})
				w.try("healers", err)
			case "legWound":
				_, err := w.c().TreatInjury(w.ctx, rohan.user, c.id, u.ID, core.Treatment{Method: "attempt", Injury: "legWound", Roll: 4})
				w.try("near miss", err)
			}
		}
	}
	// Mordor cannot afford a treatment attempt twice: the first one fails for good
	for _, u := range w.detail(mordor).Roster {
		if u.Role == core.RoleLeader && u.Injured {
			_, err := w.c().TreatInjury(w.ctx, mordor.user, c.id, u.ID, core.Treatment{Method: "attempt", Injury: "justAScratch", Roll: 2})
			w.try("failed attempt", err)
		}
	}
	w.show(rohan)
	w.show(mordor)
	w.show(arnor)
}

// treatWarriors treats the first injured warrior of the company, leaving the others injured.
func (w *world) treatWarriors(co *wco) {
	for _, u := range w.detail(co).Roster {
		if u.Role == core.RoleWarrior && u.Injured {
			_, err := w.c().TreatInjury(w.ctx, co.user, co.camp.id, u.ID, core.Treatment{Method: "warrior"})
			w.try("treat a warrior", err)
			return
		}
	}
}
