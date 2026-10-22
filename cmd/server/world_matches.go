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
		"Eighteen rulebook scenarios, a one-off, two teams, three sides, results entered, ended, a draw, a company that fled and a match reopened by an administrator.", defaultSettings)
	w.spareNext = 0
	cos := w.pool(c, matchPool...)
	a, g, d, b := cos[2], cos[3], cos[4], cos[5]

	t := w.spare(c, 2)
	w.newMatch(c, []*wco{t[0]}, []*wco{t[1]}) // open, no scenario yet

	list, err := w.c().Scenarios(w.ctx, c.id)
	w.must("scenarios", err)
	for i, sc := range list {
		if i%4 == 0 { // scenario chosen, nothing reported: stays open with its own companies
			t := w.spare(c, 2)
			w.rulebook(c, w.newMatch(c, []*wco{t[0]}, []*wco{t[1]}), sc.Name)
			continue
		}
		left, right := cos[(2*(i%3))], cos[(2*(i%3))+1]
		m := w.newMatch(c, []*wco{left}, []*wco{right})
		m = w.rulebook(c, m, sc.Name)
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
			m = w.report(c, m, outcome{winner: 1, vp: []int{4 + i%3, 2}, ticks: ticks, note: fmt.Sprintf("%s: a close game.", sc.Name)})
		case 2:
			m = w.report(c, m, outcome{winner: 2, vp: []int{1, 3 + i%2}, ticks: ticks})
		case 3:
			m = w.report(c, m, outcome{draw: true, vp: []int{3, 3}, ticks: ticks, note: "Both leaders fell in the last turn."})
		}
		w.finish(c, m)
	}

	// a one-off scenario the players invented, result entered, ended
	m := w.newMatch(c, []*wco{g}, []*wco{d})
	m = w.oneOff(c, m, "The Bridge of Sighs", "A narrow bridge over a ravine. Each side starts at one end; the middle plank counts as difficult ground.", "Hold the far end of the bridge when turn 8 ends, or the other side's Leader is slain.")
	m = w.report(c, m, outcome{winner: 2, vp: []int{0, 1}, note: "Gimli's Dwarves held the bridge."})
	w.finish(c, m)

	// three sides, ended
	m = w.newMatch(c, []*wco{d}, []*wco{b}, []*wco{a})
	m = w.rulebook(c, m, "The Wolves Attack")
	w.finish(c, w.report(c, m, outcome{winner: 3, vp: []int{2, 1, 4}}))

	// two companies on each side, open (own companies)
	t = w.spare(c, 4)
	w.newMatch(c, []*wco{t[0], t[1]}, []*wco{t[2], t[3]})

	// a result entered but the match not ended: the "End match" button is the next click
	t = w.spare(c, 2)
	m = w.newMatch(c, []*wco{t[0]}, []*wco{t[1]})
	m = w.rulebook(c, m, "Show of Strength")
	w.report(c, m, outcome{winner: 1, vp: []int{6, 0}})

	// ended, the aftermath not started: both companies are locked until it is played
	t = w.spare(c, 2)
	w.played(c, "Chance Encounter", outcome{winner: 2, vp: []int{2, 5}}, []*wco{t[0]}, []*wco{t[1]})

	// ended and reopened by an administrator: the history shows both; open again, so the companies are in it
	t = w.spare(c, 2)
	m = w.played(c, "Hold the Line", outcome{draw: true, vp: []int{3, 3}}, []*wco{t[0]}, []*wco{t[1]})
	admin := w.user("admin")
	m, err = w.c().ReopenMatch(w.ctx, admin, c.id, m.ID, m.Revision)
	if w.try("reopen as administrator", err) {
		w.report(c, m, outcome{winner: 1, vp: []int{4, 3}, note: "Corrected: one more objective was scored."})
	}
}
