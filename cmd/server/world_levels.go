package main

import (
	"strings"

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

// gainStat levels a hero up with a typed roll that offers the stat, taking it: partial progress for the dots of the window.
func (w *world) gainStat(co *wco, id int64, sum int, stat string) {
	u := w.unitByID(co, id)
	w.xp(co, id, 5, "A season of war")
	r := w.rollLevel(co, u, sum, sum)
	for _, o := range r.Offers {
		for _, ch := range o.Choices {
			if strings.HasPrefix(ch.Key, "stat:"+stat+":") {
				w.chooseLevel(co, u, ch.Key)
				return
			}
		}
	}
	w.gap("%s: a roll of %d does not offer %s", u.Title(), sum, stat)
}

// rollWaiting leaves a hero with a typed roll to choose from.
func (w *world) rollWaiting(co *wco, id int64, first, second int) {
	w.xp(co, id, 5, "Ready for the next step")
	w.rollLevel(co, w.unitByID(co, id), first, second)
}

// levelOptionsCampaign leaves one hero per way the options of a level-up go together (docs/frontend-level-up.md, "Options that also take
// the other result"). Galadhrim have Courage 5+, so they can improve it until the Warlord's best of 3+.
func (w *world) levelOptionsCampaign() {
	c := w.campaign("you", "Level-up options: results that take the other too",
		"One hero per case of a level-up roll: a stat result beside another, with nothing to take, beside a normal option, both results taking the other; Courage at its limit; stats part of the way.", defaultSettings)
	co := w.company(c, "galadriel", "Galadhrim with choices to make", "Lothlorien", true)
	w.topUp(co)
	w.recruitAll(co)
	_, _, ws := w.roster(co)
	w.choosePaths(co, 0)
	hero := func(i int, path string) int64 {
		if i >= len(ws) {
			w.gap("%s: not enough warriors for every case of the level-up options", co.name)
			return 0
		}
		w.becomeHero(co, ws[i].ID, path, "")
		return ws[i].ID
	}
	wait := func(id int64, first, second int) {
		if id != 0 {
			w.rollWaiting(co, id, first, second)
		}
	}
	// A: Courage or Intelligence (a 5) beside a result with options (an 8): Courage is marked
	wait(hero(0, "Path of the Tactician"), 5, 8)
	// A with Courage at its limit: Intelligence is marked
	limit := hero(1, "Path of the Warlord")
	if limit != 0 {
		w.xp(co, limit, 5, "Stood firm twice") // a 5 and a 5, Courage from both: 5+ to the Warlord's best, 3+
		w.rollLevel(co, w.unitByID(co, limit), 5, 5)
		w.chooseLevel(co, w.unitByID(co, limit), "stat:courage:1", "stat:courage:1")
	}
	wait(limit, 5, 8)
	// B: the other result has nothing to take (a 2 is Spy, which it has): the options apply at once
	spy := hero(2, "Path of the Tactician")
	if spy != 0 {
		w.xp(co, spy, 5, "Listened at the door")
		w.rollLevel(co, w.unitByID(co, spy), 2, 2)
		w.chooseLevel(co, w.unitByID(co, spy), "rule:Spy")
	}
	wait(spy, 5, 2)
	// C: Heroic Resolve Mastery beside Courage and Intelligence (the Warlord's 7): the normal option applies alone
	wait(hero(3, "Path of the Warlord"), 7, 8)
	// D: both results take the other too, a mark in each and Apply both
	wait(hero(4, "Path of the Tactician"), 5, 5)
	// D on the Warlord: the 7's normal option can take that result's mark, with the 5's stat
	wait(hero(5, "Path of the Warlord"), 5, 7)
	// stat options part of the way to their limit
	partial := hero(6, "Path of the Tactician")
	if partial != 0 {
		w.gainStat(co, partial, 8, "fight")
	}
	wait(partial, 8, 8)
	w.show(co)
}

// levelCampaign leaves level-ups of every kind waiting: not yet rolled, a Hero in the Making to accept, a hero with no path yet,
// a promotion to choose, several rolls piled up, a hero's two results.
func (w *world) levelCampaign() {
	c := w.campaign("you", "Level-ups waiting: warriors, heroes, paths and a Sorcerer",
		"Units with experience to spend: rolls not made yet, a promotion or a path to choose, a Sorcerer's first power, three rolls piled up, a hero choosing between two results.", defaultSettings)
	lor := w.company(c, "galadriel", "Galadhrim with experience to spend", "Lothlorien", true)
	w.grant(lor, 10, 30)
	w.recruitAll(lor)
	l, s, ws := w.roster(lor)
	w.xp(lor, ws[0].ID, 5, "Won the duel at the ford")  // a roll waiting, not made
	w.xp(lor, ws[1].ID, 5, "Saved the standard")        // rolls a 6: hero in the making, not accepted yet
	w.xp(lor, ws[2].ID, 5, "Held the bridge")           // rolls a 6 and accepts: a hero with no path yet
	w.xp(lor, ws[4].ID, 15, "Three campaigns of glory") // three rolls waiting: a typed 2 is no change with rolls left
	w.xp(lor, ws[5].ID, 5, "Carried the wounded")       // rolled a 2: nothing
	w.xp(lor, l.ID, 10, "Led the company well")         // a hero with two rolls waiting, one made
	w.xp(lor, s[0].ID, 5, "Slew the troll")             // a hero's roll waiting
	w.rollLevel(lor, ws[1], 6)
	w.rollLevel(lor, ws[2], 6)
	w.chooseLevel(lor, ws[2], "hero")
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
