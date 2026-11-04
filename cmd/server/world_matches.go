package main

import (
	"fmt"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// pool makes the companies the match campaigns play with: one player each, paths chosen, a purse.
func (w *world) pool(c *wcamp, specs ...[3]string) []*wco {
	var out []*wco
	for _, s := range specs {
		co := w.company(c, s[0], s[1], s[2], true)
		w.grant(co, 8, 10)
		out = append(out, co)
	}
	return out
}

var matchPool = [][3]string{
	{"eowyn", "Eorlings of the Mark", "Rohan"},
	{"sauron", "Black Gate Vanguard", "Mordor"},
	{"aragorn", "Dunedain of the North", "Arnor"},
	{"galadriel", "Galadhrim Wardens", "Lothlorien"},
	{"gimli", "Longbeards of Erebor", "Durin's Folk"},
	{"bilbo", "Burglars of Bag End", "The Shire"},
}

// spare makes n more companies for the matches that stay open or unfinished: a company is in one open match at a time and
// cannot start another while an aftermath is pending, so every such match gets companies of its own. Their players are the
// gallery accounts (one per Battle Company), their factions the ones of the rules file in order.
func (w *world) spare(c *wcamp, n int) []*wco {
	profiles, err := w.c().CompanyProfiles(w.ctx, c.id)
	w.must("profiles", err)
	var out []*wco
	for len(out) < n {
		if w.spareNext >= len(profiles) {
			panic(worldAbort{fmt.Errorf("the gallery has no more players for %s", c.name)})
		}
		p := profiles[w.spareNext]
		w.spareNext++
		co := w.company(c, "g-"+slug(p.Name), p.Name+" Company", p.Name, true)
		w.grant(co, 8, 10)
		out = append(out, co)
	}
	return out
}

// finish ends the match and waives the aftermath of every company in it (the organiser's note), so the companies are free
// for the next match. The demo plays no aftermath here; the aftermath wards do.
func (w *world) finish(c *wcamp, m core.Match) core.Match {
	m = w.endMatch(c, m)
	for _, s := range m.Sides {
		for _, co := range s.Companies {
			_, err := w.c().WaiveAftermath(w.ctx, c.org, c.id, m.ID, co.ID, "Not played in the demo: waived to free the company")
			w.must("waive the aftermath of "+co.Name, err)
		}
	}
	return m
}

// matchCampaign opens a match for each scenario of the rulebook (and the other ways to set one up) and leaves them in every
// state a match can be in before the aftermath: open without scenario, scenario chosen, a result entered, ended, reopened.
// The six pool companies play the matches that end (their aftermaths waived); the matches that stay open or unfinished have
// companies of their own.
func (w *world) matchCampaign() {
	c := w.campaign("you", "Matches: every scenario and every state",
		"Eighteen rulebook scenarios chosen when the match was made, a one-off, two teams, three sides, Against the Odds taken on the new match form, results entered, an end waiting for confirmation, ended, a draw and a match reopened by an administrator.", defaultSettings)
	w.spareNext = 0
	cos := w.pool(c, matchPool...)
	a, g, d, b := cos[2], cos[3], cos[4], cos[5]

	t := w.spare(c, 2)
	w.newMatch(c, []*wco{t[0]}, []*wco{t[1]}) // open, no scenario yet

	list, err := w.c().Scenarios(w.ctx, c.id)
	w.must("scenarios", err)
	for i, sc := range list {
		if i%4 == 0 { // scenario chosen on the new match form, nothing reported: stays open with its own companies
			t := w.spare(c, 2)
			w.newMatchOn(c, sc.Name, []*wco{t[0]}, []*wco{t[1]})
			continue
		}
		left, right := cos[(2*(i%3))], cos[(2*(i%3))+1]
		m := w.newMatchOn(c, sc.Name, []*wco{left}, []*wco{right})
		info := w.scenarioInfo(c, sc.Name)
		ticks := map[*wco]map[string]int{}
		if len(info.Objectives) > 0 {
			t := map[string]int{}
			for _, o := range info.Objectives {
				t[o.Key] = o.Max
			}
			ticks[left] = t
		}
		switch i % 4 {
		case 1:
			m = w.report(c, m, outcome{winner: 1, ticks: ticks, note: fmt.Sprintf("%s: a close game.", sc.Name)})
		case 2:
			m = w.report(c, m, outcome{winner: 2, ticks: ticks})
		case 3:
			m = w.report(c, m, outcome{draw: true, ticks: ticks, note: "Both leaders fell in the last turn."})
		}
		w.finish(c, m)
	}

	// a one-off scenario the players invented, result entered, ended
	m := w.newMatch(c, []*wco{g}, []*wco{d})
	m = w.oneOff(c, m, "The Bridge of Sighs", "A narrow bridge over a ravine. Each side starts at one end; the middle plank counts as difficult ground.", "Hold the far end of the bridge when turn 8 ends, or the other side's Leader is slain.")
	m = w.report(c, m, outcome{winner: 2, note: "Gimli's Dwarves held the bridge."})
	w.finish(c, m)

	// three sides, ended
	m = w.newMatchOn(c, "The Wolves Attack", []*wco{d}, []*wco{b}, []*wco{a})
	w.finish(c, w.report(c, m, outcome{winner: 3}))

	// two companies on each side, open (own companies)
	t = w.spare(c, 4)
	w.newMatch(c, []*wco{t[0], t[1]}, []*wco{t[2], t[3]})

	// a result entered but the match not ended: the "End match" button is the next click
	t = w.spare(c, 2)
	m = w.newMatchOn(c, "Show of Strength", []*wco{t[0]}, []*wco{t[1]})
	w.report(c, m, outcome{winner: 1})

	// Against the Odds taken on the new match form by the owner of the lower rated company
	t = w.spare(c, 2)
	w.atoAtCreation(c, t[0], t[1])

	// played live, then the end proposed through the End match sheet (winner and MVP vote) and waiting for the rival's Confirm
	t = w.spare(c, 2)
	w.sheetEnd(c, t[0], t[1], false)

	// played live and ended through the new sheets: End match, then the rival's Confirm with their own MVP vote
	t = w.spare(c, 2)
	w.sheetEnd(c, t[0], t[1], true)

	// ended, the aftermath not started: both companies are locked until it is played
	t = w.spare(c, 2)
	w.played(c, "Chance Encounter", outcome{winner: 2}, []*wco{t[0]}, []*wco{t[1]})

	// played live and ended, the aftermath not started: Start aftermath skips the sheet, into the casualty's roll for one and the summary for the other
	t = w.spare(c, 2)
	m = w.newMatchOn(c, "Hold the Line", []*wco{t[0]}, []*wco{t[1]})
	_, _, warriors := w.roster(t[0])
	w.try("a Warrior falls", w.liveAs(t[0], m, core.LiveAction{Kind: core.LiveDied, UnitID: warriors[0].ID, Note: "Fell holding the line"}))
	_, _, warriors = w.roster(t[1])
	w.try("a Warrior leaves the table", w.liveAs(t[1], m, core.LiveAction{Kind: core.LiveRetreat, UnitID: warriors[0].ID, Why: "left"}))
	w.endThroughSheets(c, m, t[0], t[1], true)

	// ended and reopened by an administrator: the history shows both; open again, so the companies are in it
	t = w.spare(c, 2)
	m = w.played(c, "Hold the Line", outcome{draw: true}, []*wco{t[0]}, []*wco{t[1]})
	admin := w.user("admin")
	m, err = w.c().ReopenMatch(w.ctx, admin, c.id, m.ID, m.Revision)
	if w.try("reopen as administrator", err) {
		w.report(c, m, outcome{winner: 1, note: "Corrected: one more objective was scored."})
	}
}

// atoAtCreation makes a match whose lower rated company took Against the Odds bonuses on the new match form (docs/matches.md,
// "New match"): the owner of that company creates it, with the first bonus the gap affords that needs no detail.
func (w *world) atoAtCreation(c *wcamp, x, y *wco) {
	sides := []core.NewSide{{Companies: []int64{x.id}}, {Companies: []int64{y.id}}}
	fmt.Fprintln(w.out, "  DEBUGATO:", x.name, y.name, len(w.detail(x).Roster), len(w.detail(y).Roster))
	for _, r := range append(w.detail(x).Roster, w.detail(y).Roster...) {
		fmt.Fprintln(w.out, "  DEBUGU:", r.Title(), r.Points, r.PathPending, r.Path)
	}
	for _, who := range []*wco{x, y} {
		card, err := w.c().PreviewAgainstTheOdds(w.ctx, who.user, c.id, sides, false)
		w.must("Against the Odds of the new match", err)
		if !card.Eligible {
			w.gap("Against the Odds at creation: %s", card.Why)
			return
		}
		if card.Access != core.ATOEdit {
			continue // the other company is the lower rated one
		}
		for _, o := range card.Options {
			if o.Disabled || o.OverBudget || o.Key == "toolKit" || o.Key == "wanderer" || o.Key == "ambush" {
				continue
			}
			_, err := w.c().CreateMatch(w.ctx, who.user, c.id, core.NewMatch{Sides: sides,
				Scenario: &core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Seize Ground"},
				ATO:      &core.NewMatchATO{Selection: core.ATOSelection{Bonuses: []string{o.Key}}}})
			w.must("create a match with Against the Odds", err)
			return
		}
		w.gap("Against the Odds at creation: the gap affords no bonus without details")
		return
	}
}

// sheetEnd plays a match live and ends it the way the End match and Confirm sheets do: a live entry, the result, the proposer's MVP vote
// and the proposal; with confirm, the rival's vote and Confirm too.
func (w *world) sheetEnd(c *wcamp, x, y *wco, confirm bool) {
	m := w.newMatchOn(c, "Hold the Line", []*wco{x}, []*wco{y})
	if l, _, _ := w.roster(x); l.ID != 0 { // the game has started: a point of Might spent
		w.try("spend Might", w.liveAs(x, m, core.LiveAction{Kind: core.LiveStatChange, UnitID: l.ID, Stat: "might", Delta: -1}))
	}
	if m = w.endThroughSheets(c, m, x, y, confirm); !confirm {
		return
	}
	for _, co := range []*wco{x, y} { // their aftermaths are waived so the companies are free
		_, err := w.c().WaiveAftermath(w.ctx, c.org, c.id, m.ID, co.id, "Not played in the demo: waived to free the company")
		w.must("waive the aftermath of "+co.name, err)
	}
}

// endThroughSheets is the end of the End match and Confirm sheets: the result, the proposer's MVP vote and the proposal; with confirm, the
// rival's vote and Confirm too.
func (w *world) endThroughSheets(c *wcamp, m core.Match, x, y *wco, confirm bool) core.Match {
	vote := func(voter, of *wco) {
		if l, _, _ := w.roster(of); l.ID != 0 {
			_, _, err := w.c().Apply(w.ctx, voter.user, c.id, m.ID, of.id, core.LiveAction{Kind: core.LiveMVPVote, UnitID: l.ID})
			w.must(voter.name+" votes the MVP of "+of.name, err)
		}
	}
	m, err := w.c().SaveResult(w.ctx, x.user, c.id, m.ID, m.Revision, core.MatchResult{Decided: true, Winner: 1, Note: "Proposed from the End match sheet."})
	w.must("the result of the End match sheet", err)
	vote(x, y)
	m, err = w.c().ProposeEnd(w.ctx, x.user, c.id, m.ID, m.Revision)
	w.must("End match", err)
	if !confirm {
		return m
	}
	vote(y, x)
	m, err = w.c().ConfirmEnd(w.ctx, y.user, c.id, m.ID, m.Revision)
	w.must("Confirm", err)
	return m
}
