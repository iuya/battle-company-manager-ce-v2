package main

import (
	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// fiveXP are the gains that make a unit earn five experience on a sheet: a level-up roll.
var fiveXP = map[string]int{"woundedEnemy": 1, "fatePrevented": 1, "objective": 1, "brokenCourageTestPassed": 1}

// aftermathCampaigns plays matches whose companies stop at every point of the War's Aftermath.
func (w *world) aftermathCampaigns() {
	w.aftermathWardOne()
	w.aftermathWardTwo()
	w.aftermathWardThree()
	w.aftermathWardFour()
	w.aftermathWardFive()
	w.aftermathWardSix()
}

// aftermathWardSix stops at every state of the Sheet step: the Against the Odds block (bonuses taken, none taken with a gap that paid for
// one, no block at all) and each reason that opens the sheet instead of the steps. Nothing is saved: the sheets are the unsaved defaults.
func (w *world) aftermathWardSix() {
	c := w.campaign("you", "Aftermath ward 6: what the sheet asks",
		"The Sheet step before anything is saved: Against the Odds bonuses taken, none taken with a gap that paid, no block at all; and why the sheet opened: nothing recorded, the Relic with no holder, a tied MVP vote.", defaultSettings)
	cos := w.pool(c, [][3]string{
		{"eowyn", "Riders who took a Tool Kit", "Rohan"},
		{"sauron", "Riders who hired a Wanderer", "Rohan"},
		{"aragorn", "Rangers who took nothing", "Rohan"},
		{"galadriel", "Elves who hired a Wanderer", "Rohan"},
		{"gimli", "Dwarves who forgot the Relic", "Rohan"},
		{"bilbo", "Hobbits who lost the Relic", "Rohan"},
		{"radagast", "Wargs with a tied vote", "Rohan"},
		{"pippin", "Pippin's voters", "Rohan"},
		{"frodo", "Frodo's voters", "Rohan"},
	}...)

	// the same faction starts level: the Wanderer alone makes the gap, which pays for the bonuses taken
	if !w.hireWanderer(cos[1], "Wandering Swordsman") || !w.hireWanderer(cos[3], "Wandering Swordsman") {
		w.gap("Aftermath ward 6: a Wanderer that makes the gap cannot be hired")
	}

	// the lower company took a bonus for the aftermath and one that does nothing in it
	m, err := w.c().CreateMatch(w.ctx, c.org, c.id, core.NewMatch{Sides: newSides([][]*wco{{cos[0]}, {cos[1]}}), Scenario: &core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Seize Ground"}})
	w.must("create the match of the bonuses", err)
	_, err = w.c().SaveAgainstTheOdds(w.ctx, cos[0].user, c.id, m.ID, m.Revision, core.ATOSelection{Bonuses: []string{"influence", "toolKit"}, Kits: []string{"healer"}})
	w.must("take the bonuses", err)
	m, err = w.c().Match(w.ctx, c.id, m.ID)
	w.must("read the match of the bonuses", err)
	w.endMatch(c, w.report(c, m, outcome{winner: 2}))

	// the gap paid for a bonus and the lower company took none (decideATO, in played)
	w.played(c, "Chance Encounter", outcome{winner: 2}, []*wco{cos[2]}, []*wco{cos[3]})

	// equal ratings and nothing tracked, and the result says the company holds a Relic nobody was given
	w.played(c, "Relic of Ages Past", outcome{winner: 1, ticks: map[*wco]map[string]int{cos[4]: {"possessesRelic": 1}}}, []*wco{cos[4]}, []*wco{cos[5]})

	// played live: a Warrior left the table, so only the two rival players' votes keep the sheet open (one vote each, on two units)
	tied, voters := cos[6], cos[7:9]
	m = w.newMatchOn(c, "Hold the Line", []*wco{tied}, voters)
	l, s, ws := w.roster(tied)
	w.try("a Warrior leaves the table", w.liveAs(tied, m, core.LiveAction{Kind: core.LiveRetreat, UnitID: ws[0].ID, Why: "left"}))
	for i, unit := range []int64{l.ID, s[0].ID} {
		_, _, err := w.c().Apply(w.ctx, voters[i].user, c.id, m.ID, tied.id, core.LiveAction{Kind: core.LiveMVPVote, UnitID: unit})
		w.must(voters[i].name+" votes the MVP of "+tied.name, err)
	}
	m, err = w.c().Match(w.ctx, c.id, m.ID)
	w.must("read the match of the tied vote", err)
	w.endMatch(c, w.report(c, m, outcome{winner: 2}))
}

// aftermathWardFour stops at the questions only a result asks: which persistent injury a hero's Full Recovery heals, and the bonus die
// of Lesson Learned when the dice are typed. The Leader needs an injury to heal first, so the company plays two matches.
func (w *world) aftermathWardFour() {
	c := w.campaign("you", "Aftermath ward 4: the questions of a recovery",
		"A hero's Full Recovery asks which injury it heals, and a Warrior's Lesson Learned asks for the die the player rolled: each answer is one press.", defaultSettings)
	cos := w.pool(c, [][3]string{
		{"eowyn", "Riders nursing their Captain", "Rohan"},
		{"sauron", "Orcs who watched from afar", "Mordor"},
	}...)
	rohan, mordor := cos[0], cos[1]

	first := w.played(c, "Seize Ground", outcome{winner: 1}, []*wco{rohan}, []*wco{mordor})
	leader, sergeants, warriors := w.roster(rohan)
	in := w.sheet(rohan)
	set(&in, leader.ID, core.OutcomeCasualty, nil)
	set(&in, warriors[3].ID, core.OutcomeCasualty, nil)
	w.settle(rohan, first, in, map[int64][3]int{leader.ID: {1, 2, 1}, warriors[3].ID: {2, 2, 0}}, true) // 3: an Arm Wound; 4: Injured, misses the next match
	w.settle(mordor, first, w.sheet(mordor), nil, true)

	second := w.played(c, "Chance Encounter", outcome{winner: 1}, []*wco{rohan}, []*wco{mordor})
	in = w.sheet(rohan)
	set(&in, leader.ID, core.OutcomeCasualty, nil)
	set(&in, warriors[0].ID, core.OutcomeCasualty, map[string]int{"woundedEnemy": 1})
	// the rest of the company, below the casualties: what it earned and who fled
	set(&in, warriors[1].ID, core.OutcomeFled, map[string]int{"objective": 1})
	set(&in, warriors[2].ID, core.OutcomeSurvived, map[string]int{"woundedEnemy": 1, "slainHeroOrMonster": 2, "mvp": 1})
	for i := range in.Models { // a Sergeant who stayed out of the game: last, with the Warrior who missed it hurt
		if in.Models[i].UnitID == sergeants[1].ID {
			in.Models[i].Gains = map[string]int{}
		}
	}
	w.startAftermath(rohan, second, in)
	// rolled directly: cast would answer the bonus die, and these questions are left open
	for unit, dice := range map[int64][]int{leader.ID: {3, 4}, warriors[0].ID: {6, 6}} { // 7: Full Recovery; 12: Lesson Learned
		_, err := w.c().RollCasualty(w.ctx, c.org, c.id, second.ID, rohan.id, unit, dice, false)
		w.must("roll the injury in "+rohan.name, err)
	}
}

// aftermathWardFive is ward 4's Lesson Learned in a campaign where players type their rolls but the table is not shaken: the bonus D3 is
// not thrown in the browser, "Roll D3" lets the server roll it and the die acts out the server's number.
func (w *world) aftermathWardFive() {
	c := w.campaign("you", "Aftermath ward 5: a bonus die the server rolls",
		"Shake the table is off (typed rolls are on): two Warriors' Lesson Learned ask for their D3; Roll D3 lets the server roll it and the 3D die shows the number, or the player types the die they rolled. The browser throws nothing.",
		core.CampaignSettings{ManualRolls: true})
	cos := w.pool(c, [][3]string{
		{"eowyn", "Riders with a lesson to learn", "Rohan"},
		{"sauron", "Orcs on the far hill", "Mordor"},
	}...)
	rohan, mordor := cos[0], cos[1]
	m := w.played(c, "Seize Ground", outcome{winner: 1}, []*wco{rohan}, []*wco{mordor})
	_, _, warriors := w.roster(rohan)
	in := w.sheet(rohan)
	set(&in, warriors[0].ID, core.OutcomeCasualty, nil)
	set(&in, warriors[1].ID, core.OutcomeCasualty, nil)
	w.startAftermath(rohan, m, in)
	for _, unit := range []int64{warriors[0].ID, warriors[1].ID} { // 12: Lesson Learned, its D3 waits (two of them: answering the first leaves the step open, so its die is seen rolling)
		_, err := w.c().RollCasualty(w.ctx, c.org, c.id, m.ID, rohan.id, unit, []int{6, 6}, false)
		w.must("roll the injury in "+rohan.name, err)
	}
	w.settle(mordor, m, w.sheet(mordor), nil, true)
}

func (w *world) aftermathWardOne() {
	c := w.campaign("you", "Aftermath ward 1: every step of the War's Aftermath",
		"Eight companies, four ended matches: aftermath not started, a draft sheet, a Scratch waiting for an answer, a fallen Leader waiting for a successor, level-ups waiting with a free item to choose, one finished with a perk, one waived, one fled (reported as the other side's win).", defaultSettings)
	cos := w.pool(c, [][3]string{
		{"eowyn", "Rohirrim waiting at the gate", "Rohan"},
		{"sauron", "Orcs with a half-filled sheet", "Mordor"},
		{"aragorn", "Rangers with a Scratch to decide", "Arnor"},
		{"galadriel", "Galadhrim whose Leader fell", "Lothlorien"},
		{"gimli", "Dwarves with level-ups to roll", "Durin's Folk"},
		{"bilbo", "Hobbits who finished", "The Shire"},
		{"radagast", "Wild Wargs that fled", "Mirkwood"},
		{"pippin", "Pippin's patient company", "Wanderers in the Wild"},
	}...)
	rohan, mordor, arnor, loth, dwarf, shire, mir, wild := cos[0], cos[1], cos[2], cos[3], cos[4], cos[5], cos[6], cos[7]

	// match 1: Rohan has not started, Mordor saved a draft
	m1 := w.played(c, "Seize Ground", outcome{winner: 1}, []*wco{rohan}, []*wco{mordor})
	in := w.sheet(mordor)
	l, s, ws := w.roster(mordor)
	set(&in, l.ID, core.OutcomeSurvived, map[string]int{"woundedEnemy": 1, "slainHeroOrMonster": 2})
	set(&in, s[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[1].ID, core.OutcomeFled, nil)
	in.AgainstTheOdds = []string{"influence", "influence"}
	w.saveSheet(mordor, m1, in)

	// match 2: a hero's Scratch waits for the player; a Leader falls and the company must name a successor
	m2 := w.played(c, "Chance Encounter", outcome{winner: 1, note: "Decided on the tie-break."}, []*wco{arnor}, []*wco{loth})
	l, s, ws = w.roster(arnor)
	in = w.sheet(arnor)
	set(&in, s[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	w.settle(arnor, m2, in, map[int64][3]int{s[0].ID: {3, 3, 1}, ws[0].ID: {2, 3, 1}}, false)

	l, _, _ = w.roster(loth)
	in = w.sheet(loth)
	set(&in, l.ID, core.OutcomeCasualty, nil)
	w.settle(loth, m2, in, map[int64][3]int{l.ID: {1, 1, 1}}, false) // 2: dead; the sergeants may take over

	// match 3: level-ups waiting, and a free item to choose (all the markers scored)
	m3 := w.played(c, "Scavenge the Battlefield", outcome{winner: 1,
		ticks: map[*wco]map[string]int{dwarf: {"allObjectiveMarkers": 1}}}, []*wco{dwarf}, []*wco{shire})
	l, s, ws = w.roster(dwarf)
	in = w.sheet(dwarf)
	set(&in, l.ID, core.OutcomeSurvived, fiveXP)
	set(&in, s[0].ID, core.OutcomeSurvived, fiveXP)
	set(&in, ws[0].ID, core.OutcomeSurvived, fiveXP)
	set(&in, ws[1].ID, core.OutcomeCasualty, nil)
	w.settle(dwarf, m3, in, map[int64][3]int{ws[1].ID: {4, 4, 1}}, false)

	in = w.sheet(shire)
	l, _, ws = w.roster(shire)
	set(&in, l.ID, core.OutcomeSurvived, fiveXP) // a level-up roll still waits on the finished page
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	w.settle(shire, m3, in, map[int64][3]int{ws[0].ID: {3, 5, 1}}, true) // finished
	w.xp(shire, ws[1].ID, 5, "Told the tale of the battle")              // a warrior's roll too: its no change comes back to this page

	// match 4: Mirkwood fled the field (All is Lost, reported as the Wanderers' win); Wanderers hold the Relic and finish with its blessing
	m4 := w.played(c, "Relic of Ages Past", outcome{winner: 2,
		ticks: map[*wco]map[string]int{wild: {"possessesRelic": 1}}, note: "The Wargs ran at the first volley."}, []*wco{mir}, []*wco{wild})
	in = w.sheet(wild)
	l, _, ws = w.roster(wild)
	carrier := l // the Wanderers are all heroes from the start: the Leader holds the Relic
	if len(ws) > 0 {
		carrier = ws[0]
	}
	for i := range in.Models {
		if in.Models[i].UnitID == carrier.ID {
			in.Models[i].Relic = true
		}
	}
	w.settle(wild, m4, in, nil, true)
	_, err := w.c().WaiveAftermath(w.ctx, c.org, c.id, m4.ID, mir.id, "The Wargs fled; the organiser waives the rest")
	w.must("waive", err)
	w.try("fled company", nil)
	_ = m1
}

func (w *world) aftermathWardTwo() {
	c := w.campaign("you", "Aftermath ward 2: draws, odds, perks and a succession",
		"A draw, Against the Odds with re-rolls and a bonus unit, a Leader's succession settled, the scenario rewards (Influence, Experience, a perk) and the organiser's grants.", defaultSettings)
	cos := w.pool(c, [][3]string{
		{"eowyn", "Rohan, who drew", "Rohan"},
		{"sauron", "Underdog Orcs with every bonus", "Mordor"},
		{"aragorn", "Rangers with a new Leader", "Arnor"},
		{"galadriel", "Elves with the vantage point", "Lothlorien"},
		{"gimli", "Dwarves with cargo", "Iron Hills"},
		{"bilbo", "Hobbits who rescued the Leader", "The Shire"},
	}...)
	rohan, mordor, arnor, loth, iron, shire := cos[0], cos[1], cos[2], cos[3], cos[4], cos[5]

	// a draw; Mordor was the Underdog and took Influence, Experience and the re-rolls
	m1 := w.played(c, "Seize Ground", outcome{draw: true}, []*wco{rohan}, []*wco{mordor})
	in := w.sheet(rohan)
	_, _, ws := w.roster(rohan)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	w.settle(rohan, m1, in, map[int64][3]int{ws[0].ID: {4, 3, 1}}, true)
	in = w.sheet(mordor)
	_, s, ws := w.roster(mordor)
	in.AgainstTheOdds = []string{"influence", "influence", "experience", "reroll"}
	set(&in, s[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	a := w.settle(mordor, m1, in, map[int64][3]int{s[0].ID: {1, 2, 1}, ws[0].ID: {2, 1, 1}}, false) // arm wound and dead: re-rolls left to spend
	_ = a
	w.dice.Set(5, 5)
	_, err := w.c().RerollCasualty(w.ctx, c.org, c.id, m1.ID, mordor.id, ws[0].ID, nil, false)
	w.try("re-roll a casualty", err)

	// a Leader fell in Arnor: the successor and a new Sergeant chosen, finished
	m2 := w.played(c, "Rescue", outcome{winner: 1}, []*wco{arnor}, []*wco{loth})
	l, _, _ := w.roster(arnor)
	in = w.sheet(arnor)
	set(&in, l.ID, core.OutcomeCasualty, nil)
	a = w.settle(arnor, m2, in, map[int64][3]int{l.ID: {1, 1, 1}}, false)
	if a.Succession != nil && a.Succession.NeedLeader && len(a.Succession.LeaderOptions) > 0 {
		a = w.decide(arnor, m2, core.AftermathAnswer{Kind: "succession", Answer: itoa(a.Succession.LeaderOptions[0])})
	}
	if a.Succession != nil && a.Succession.NeedSergeant && len(a.Succession.SergeantOptions) > 0 {
		a = w.decide(arnor, m2, core.AftermathAnswer{Kind: "sergeant", Answer: itoa(a.Succession.SergeantOptions[0])})
	}
	switch a.Status { // with no reward choice open the aftermath has finished on its own
	case core.AftermathLevelUps:
		w.finishAftermath(arnor, m2)
	case core.AftermathDone:
	default:
		w.gap("the succession of %s was not settled (status %s)", arnor.name, a.Status)
	}
	// the Elves survived the Rescue: the Leader earns the extra Experience (survival of the fittest)
	in = w.sheet(loth)
	w.settle(loth, m2, in, nil, true)

	// scenario rewards: the Elves win the Secure the Area and get the vantage point perk; the Dwarves score all the cargo
	m3 := w.played(c, "Secure the Area", outcome{winner: 1}, []*wco{loth}, []*wco{rohan})
	w.settle(loth, m3, w.sheet(loth), nil, true)
	m4 := w.played(c, "Baggage Train", outcome{winner: 1, ticks: map[*wco]map[string]int{iron: {"allCargo": 1}}}, []*wco{iron}, []*wco{shire})
	w.settle(iron, m4, w.sheet(iron), nil, true)
	w.settle(shire, m4, w.sheet(shire), nil, true) // the Hobbits' aftermath is done before they play again
	// (Mordor's first aftermath is still open, so the Hobbits' next game is against the Dwarves, whose aftermath is done)
	m5 := w.played(c, "Chance Encounter", outcome{winner: 2}, []*wco{iron}, []*wco{shire})
	w.settle(iron, m5, w.sheet(iron), nil, true)
	w.settle(shire, m5, w.sheet(shire), nil, true)

	// organiser's grants, shown in each company's history
	w.grant(iron, 5, 2)
	_, l2, _ := w.roster(iron)
	_ = l2
	w.show(arnor)
	w.show(mordor)
}

// aftermathWardThree leaves three companies at the screen where no hero is left to take over and a Warrior must be chosen: a
// Sergeant fell with one Warrior ahead in Experience (the rules make it a hero, the player only confirms), a Sergeant fell with two
// Warriors tied for the most Experience (the player chooses), and a Leader fell together with both Sergeants (a Leader first,
// then Sergeants, all from the Warriors).
func (w *world) aftermathWardThree() {
	c := w.campaign("you", "Aftermath ward 3: no hero left to step up",
		"A Sergeant fell and the company has no hero: the Warrior with the most Experience takes the place (one clear choice), two Warriors tied for it (the player chooses), and a Leader who fell with both of his Sergeants (a new Leader, then new Sergeants, all from the Warriors).", defaultSettings)
	cos := w.pool(c, [][3]string{
		{"sauron", "Orcs with one clear veteran", "Mordor"},
		{"gimli", "Dwarves with two equal veterans", "Durin's Folk"},
		{"aragorn", "Rangers who lost every officer", "Arnor"},
		{"galadriel", "Elves who held the line", "Lothlorien"},
	}...)
	mordor, dwarf, arnor, elves := cos[0], cos[1], cos[2], cos[3]

	// experience below a level-up (5) before the match: one veteran in Mordor, two equal ones in Durin's Folk, one in Arnor
	_, _, ws := w.roster(mordor)
	w.xp(mordor, ws[len(ws)-1].ID, 4, "Led the charge at the gate")
	_, _, ws = w.roster(dwarf)
	w.xp(dwarf, ws[0].ID, 3, "Held the door")
	w.xp(dwarf, ws[1].ID, 3, "Held the door")
	_, _, ws = w.roster(arnor)
	w.xp(arnor, ws[0].ID, 4, "Scouted the road")

	m := w.played(c, "Seize Ground", outcome{winner: 2}, []*wco{mordor, dwarf, arnor}, []*wco{elves})
	// the sheets: everybody survives except the officers named, who are casualties and roll a 2 (dead)
	fall := func(co *wco, sergeantsToo, leader bool) core.Aftermath {
		l, s, _ := w.roster(co)
		in := w.sheet(co)
		dice := map[int64][3]int{}
		for i, sg := range s {
			if i == 0 || sergeantsToo {
				set(&in, sg.ID, core.OutcomeCasualty, nil)
				dice[sg.ID] = [3]int{1, 1, 1}
			}
		}
		if leader {
			set(&in, l.ID, core.OutcomeCasualty, nil)
			dice[l.ID] = [3]int{1, 1, 1}
		}
		return w.settle(co, m, in, dice, false)
	}
	a := fall(mordor, false, false)
	if a.Succession == nil || len(a.Succession.Fallback) != 1 {
		w.gap("the Orcs' aftermath does not wait on one Warrior (succession %+v)", a.Succession)
	}
	a = fall(dwarf, false, false)
	if a.Succession == nil || len(a.Succession.Fallback) != 2 {
		w.gap("the Dwarves' aftermath does not wait on two tied Warriors (succession %+v)", a.Succession)
	}
	a = fall(arnor, true, true)
	if a.Succession == nil || !a.Succession.NeedLeader || len(a.Succession.Fallback) == 0 {
		w.gap("the Rangers' aftermath does not wait on a Leader from the Warriors (succession %+v)", a.Succession)
	}
	w.settle(elves, m, w.sheet(elves), nil, true)
}
