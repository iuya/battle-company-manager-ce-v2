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
}

func (w *world) aftermathWardOne() {
	c := w.campaign("you", "Aftermath ward 1: every step of the War's Aftermath",
		"Eight companies, four ended matches: aftermath not started, a draft sheet, a Scratch waiting for an answer, a fallen Leader waiting for a successor, level-ups waiting with a free item to choose, one finished with a perk, one waived, one company that fled.", defaultSettings)
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
	m1 := w.played(c, "Seize Ground", outcome{winner: 1, vp: []int{5, 2}}, []*wco{rohan}, []*wco{mordor})
	in := w.sheet(mordor)
	l, s, ws := w.roster(mordor)
	set(&in, l.ID, core.OutcomeSurvived, map[string]int{"woundedEnemy": 1, "slainHeroOrMonster": 2})
	set(&in, s[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[1].ID, core.OutcomeFled, nil)
	in.AgainstTheOdds = []string{"influence"}
	w.saveSheet(mordor, m1, in)

	// match 2: a hero's Scratch waits for the player; a Leader falls and the company must name a successor
	m2 := w.played(c, "Chance Encounter", outcome{winner: 1, vp: []int{4, 4}, note: "Decided on the tie-break."}, []*wco{arnor}, []*wco{loth})
	l, s, ws = w.roster(arnor)
	in = w.sheet(arnor)
	set(&in, s[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	w.settle(arnor, m2, in, map[int64][3]int{s[0].ID: {3, 3, 1}, ws[0].ID: {2, 3, 1}}, false)

	l, s, _ = w.roster(loth)
	in = w.sheet(loth)
	set(&in, l.ID, core.OutcomeCasualty, nil)
	w.settle(loth, m2, in, map[int64][3]int{l.ID: {1, 1, 1}}, false) // 2: dead; the sergeants may take over

	// match 3: level-ups waiting, and a free item to choose (all the markers scored)
	m3 := w.played(c, "Scavenge the Battlefield", outcome{winner: 1, vp: []int{9, 3},
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
	set(&in, l.ID, core.OutcomeSurvived, map[string]int{"woundedEnemy": 1})
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	w.settle(shire, m3, in, map[int64][3]int{ws[0].ID: {3, 5, 1}}, true) // finished

	// match 4: Mirkwood fled (All is Lost); Wanderers hold the Relic and finish with its blessing
	m4 := w.played(c, "Relic of Ages Past", outcome{winner: 2, vp: []int{0, 6}, fled: []*wco{mir},
		ticks: map[*wco]map[string]int{wild: {"possessesRelic": 1}}, note: "The Wargs ran at the first volley."}, []*wco{mir}, []*wco{wild})
	in = w.sheet(wild)
	_, _, ws = w.roster(wild)
	if len(ws) == 0 {
		panic(worldAbort{errNoWarrior})
	}
	for i := range in.Models {
		if in.Models[i].UnitID == ws[0].ID {
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
	m1 := w.played(c, "Seize Ground", outcome{draw: true, vp: []int{3, 3}}, []*wco{rohan}, []*wco{mordor})
	in := w.sheet(rohan)
	l, s, ws := w.roster(rohan)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	w.settle(rohan, m1, in, map[int64][3]int{ws[0].ID: {4, 3, 1}}, true)
	in = w.sheet(mordor)
	l, s, ws = w.roster(mordor)
	in.AgainstTheOdds = []string{"influence", "experience", "reroll"}
	set(&in, s[0].ID, core.OutcomeCasualty, nil)
	set(&in, ws[0].ID, core.OutcomeCasualty, nil)
	a := w.settle(mordor, m1, in, map[int64][3]int{s[0].ID: {1, 2, 1}, ws[0].ID: {2, 1, 1}}, false) // arm wound and dead: re-rolls left to spend
	_ = a
	w.dice.Set(5, 5)
	_, err := w.c().RerollCasualty(w.ctx, c.org, c.id, m1.ID, mordor.id, ws[0].ID, nil, false)
	w.try("re-roll a casualty", err)
	_ = l

	// a Leader fell in Arnor: the successor and a new Sergeant chosen, finished
	m2 := w.played(c, "Rescue", outcome{winner: 1, vp: []int{5, 1}}, []*wco{arnor}, []*wco{loth})
	l, s, _ = w.roster(arnor)
	in = w.sheet(arnor)
	set(&in, l.ID, core.OutcomeCasualty, nil)
	a = w.settle(arnor, m2, in, map[int64][3]int{l.ID: {1, 1, 1}}, false)
	if a.Succession != nil && a.Succession.NeedLeader && len(a.Succession.LeaderOptions) > 0 {
		a = w.decide(arnor, m2, core.AftermathAnswer{Kind: "succession", Answer: itoa(a.Succession.LeaderOptions[0])})
	}
	if a.Succession != nil && a.Succession.NeedSergeant && len(a.Succession.SergeantOptions) > 0 {
		a = w.decide(arnor, m2, core.AftermathAnswer{Kind: "sergeant", Answer: itoa(a.Succession.SergeantOptions[0])})
	}
	if a.Status == core.AftermathLevelUps {
		w.finishAftermath(arnor, m2)
	} else {
		w.gap("the succession of %s was not settled (status %s)", arnor.name, a.Status)
	}
	// the Elves survived the Rescue: the Leader earns the extra Experience (survival of the fittest)
	in = w.sheet(loth)
	w.settle(loth, m2, in, nil, true)

	// scenario rewards: the Elves win the Secure the Area and get the vantage point perk; the Dwarves score all the cargo
	m3 := w.played(c, "Secure the Area", outcome{winner: 1, vp: []int{6, 2}}, []*wco{loth}, []*wco{rohan})
	w.settle(loth, m3, w.sheet(loth), nil, true)
	m4 := w.played(c, "Baggage Train", outcome{winner: 1, vp: []int{8, 2}, ticks: map[*wco]map[string]int{iron: {"allCargo": 1}}}, []*wco{iron}, []*wco{shire})
	w.settle(iron, m4, w.sheet(iron), nil, true)
	w.settle(shire, m4, w.sheet(shire), nil, true) // the Hobbits' aftermath is done before they play again
	// (Mordor's first aftermath is still open, so the Hobbits' next game is against the Dwarves, whose aftermath is done)
	m5 := w.played(c, "Chance Encounter", outcome{winner: 2, vp: []int{1, 6}}, []*wco{iron}, []*wco{shire})
	w.settle(iron, m5, w.sheet(iron), nil, true)
	w.settle(shire, m5, w.sheet(shire), nil, true)

	// organiser's grants, shown in each company's history
	w.grant(iron, 5, 2)
	_, l2, _ := w.roster(iron)
	_ = l2
	w.show(arnor)
	w.show(mordor)
}
