package main

import (
	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// rollLevel makes a unit's level-up roll with the typed result (a warrior's D6, or the sums of a hero's two 2D6).
func (w *world) rollLevel(co *wco, u core.CompanyUnit, entered ...int) core.LevelRoll {
	r, err := w.c().RollLevelUp(w.ctx, co.user, co.camp.id, co.id, u.ID, entered, false)
	w.must("level-up roll of "+u.Title(), err)
	return r
}

func (w *world) chooseLevel(co *wco, u core.CompanyUnit, keys ...string) core.LevelRoll {
	r, err := w.c().ChooseLevelUp(w.ctx, co.user, co.camp.id, co.id, u.ID, keys)
	w.must("level-up choice of "+u.Title(), err)
	return r
}

// pendingPromotion gives the first of the units that has a promotion to take 5 experience and a roll of 4; the ones before it
// have none and just get their Courage and Intelligence raised.
func (w *world) pendingPromotion(co *wco, candidates []core.CompanyUnit) {
	for _, u := range candidates {
		w.xp(co, u.ID, 5, "Scouted the pass")
		if r := w.rollLevel(co, u, 4); !r.Done {
			return
		}
	}
}

// levelCampaign leaves level-ups of every kind waiting: not yet rolled, a promotion to choose, a path to choose, a Sorcerer's
// first power, several rolls piled up, a hero's two results.
func (w *world) levelCampaign() {
	c := w.campaign("you", "Level-ups waiting: warriors, heroes, paths and a Sorcerer",
		"Units with experience to spend: rolls not made yet, a promotion or a path to choose, a Sorcerer's first power, three rolls piled up, a hero choosing between two results.", defaultSettings)
	lor := w.company(c, "galadriel", "Galadhrim with experience to spend", "Lothlorien", true)
	w.grant(lor, 10, 30)
	w.recruitAll(lor)
	l, s, ws := w.roster(lor)
	w.xp(lor, ws[0].ID, 5, "Won the duel at the ford")  // a roll waiting, not made
	w.xp(lor, ws[1].ID, 5, "Saved the standard")        // rolls a 6: hero in the making
	w.xp(lor, ws[2].ID, 5, "Held the bridge")           // rolls a 6, chooses the Sorcerer
	w.xp(lor, ws[4].ID, 15, "Three campaigns of glory") // three rolls waiting
	w.xp(lor, ws[5].ID, 5, "Carried the wounded")       // rolled a 2: nothing
	w.xp(lor, l.ID, 10, "Led the company well")         // a hero with two rolls waiting, one made
	w.xp(lor, s[0].ID, 5, "Slew the troll")             // a hero's roll waiting
	w.rollLevel(lor, ws[1], 6)
	sorc := w.rollLevel(lor, ws[2], 6)
	_ = sorc
	w.chooseLevel(lor, ws[2], "path:Path of the Sorcerer") // it must choose its first power
	r := w.rollLevel(lor, ws[5], 2)
	if !r.Done {
		w.gap("a D6 of 2 did not end the level-up")
	}
	hr := w.rollLevel(lor, l, 7, 9)
	_ = hr

	shire := w.company(c, "bilbo", "Hobbits with big ideas", "The Shire", true)
	_, _, hs := w.roster(shire)
	w.xp(shire, hs[0].ID, 5, "Found the lost pipe-weed")
	w.xp(shire, hs[1].ID, 10, "Fought the Lobelia incident")
	w.pendingPromotion(shire, hs[2:]) // a roll of 4 that leaves a promotion to choose
	w.rollLevel(shire, hs[0], 5)      // another promotion to choose
	w.show(lor)
	w.show(shire)
}
