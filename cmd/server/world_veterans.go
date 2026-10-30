package main

import (
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The Veterans: two companies whose heroes have levelled up many times, in a match that is being played, so that the live cards show
// long trackers (Might, Will, Fate and Wounds well above their start) and path boxes full of heroic actions and rules.

const veteransName = "Veterans: heroes with long trackers and full paths"

// veteranStats are the hero stats a veteran prefers, one per level-up in turn.
var veteranStats = []string{"might", "will", "fate", "wounds"}

// veteranSums are the 2D6 results of the hero's path chart whose row offers the stat (a row with a choice of options counts), in chart
// order; every row when none does.
func (w *world) veteranSums(path, stat string) []int {
	var all, sums []int
	for _, p := range w.shippedRules().BC.Progression().Paths {
		if p.Name != path {
			continue
		}
		for _, row := range p.Rows {
			all = append(all, row.Roll)
			if slices.ContainsFunc(row.Options, func(o core.ProgOption) bool { return offersStat(o, stat) }) {
				sums = append(sums, row.Roll)
			}
		}
	}
	if len(sums) == 0 {
		return all
	}
	return sums
}

func offersStat(o core.ProgOption, stat string) bool {
	return (o.Type == "improveStat" && slices.Contains(o.Stats, stat)) || slices.ContainsFunc(o.OneOf, func(x core.ProgOption) bool { return offersStat(x, stat) })
}

// veteran levels a hero up n times: each time it enters the roll of its path chart that offers the next of veteranStats and takes it
// (any hero stat, else the first thing offered once the stat is at its limit: a special rule, a heroic action, a power), so that
// both its trackers and its path box grow.
func (w *world) veteran(co *wco, id int64, n int) {
	for i := range n {
		u := w.unitByID(co, id)
		path := ""
		if u.Path != nil {
			path = u.Path.Name
		}
		stat := veteranStats[i%len(veteranStats)]
		sums := w.veteranSums(path, stat)
		sum := sums[(i/len(veteranStats))%len(sums)]
		w.xp(co, id, 5, "Another season of war")
		r, err := w.c().RollLevelUp(w.ctx, co.user, co.camp.id, co.id, id, []int{sum, sum}, false)
		if !w.try("level-up roll of "+u.Title(), err) {
			return
		}
		for guard := 0; !r.Done && guard < 4; guard++ {
			key := veteranChoice(r, stat)
			if key == "" {
				break
			}
			if r, err = w.c().ChooseLevelUp(w.ctx, co.user, co.camp.id, co.id, id, []string{key}); !w.try("level-up choice of "+u.Title(), err) {
				return
			}
		}
	}
}

// veteranChoice picks from the roll's offers: the preferred stat, then any hero stat, then the first choice.
func veteranChoice(r core.LevelRoll, prefer string) string {
	var choices []core.LevelChoice
	for _, o := range r.Offers {
		choices = append(choices, o.Choices...)
	}
	for _, want := range []func(string) bool{
		func(k string) bool { return strings.HasPrefix(k, "stat:"+prefer+":") },
		func(k string) bool {
			for _, s := range veteranStats {
				if strings.HasPrefix(k, "stat:"+s+":") {
					return true
				}
			}
			return false
		},
		func(string) bool { return true },
	} {
		for _, c := range choices {
			if want(c.Key) {
				return c.Key
			}
		}
	}
	return ""
}

// veteransCampaign loads the campaign: You's Minas Tirith against Éowyn's Rohan (its Leader on the Path of the Warlord, which ends
// with Might, Will, Fate and Wounds at their maximum of 3), every hero levelled up twenty times, the match open and
// live on turn 2 with some Might, Will, Fate and Wounds spent, so the bars show full and empty segments.
func (w *world) veteransCampaign() {
	c := w.campaign("you", veteransName,
		"For checking how the live cards look with big heroes: every hero has levelled up twenty times (hero stats first, then rules and heroic actions), and the match is being played.", defaultSettings)
	gondor := w.company(c, "you", "Veterans of the White Tower", "Minas Tirith", true)
	rohan := w.company(c, "eowyn", "Veterans of the Riddermark", "Rohan", false)
	l, _, _ := w.roster(rohan)
	w.pathOf(rohan, l.ID, "Path of the Warlord", "") // the one path whose chart raises Might, Will, Fate and Wounds all to 3
	w.choosePaths(rohan, 0)
	for _, co := range []*wco{gondor, rohan} {
		w.grant(co, 10, 40)
		w.recruitAll(co)
		l, s, _ := w.roster(co)
		w.veteran(co, l.ID, 20)
		for _, u := range s {
			w.veteran(co, u.ID, 20)
		}
	}

	m := w.rulebook(c, w.newMatch(c, []*wco{gondor}, []*wco{rohan}), "Hold the Line")
	w.must("start turn 1", w.liveApply(gondor, m, core.LiveAction{Kind: core.LiveTurn, Turn: 1}))
	w.try("start turn 2", w.liveApply(gondor, m, core.LiveAction{Kind: core.LiveTurn, Turn: 2}))
	for _, co := range []*wco{gondor, rohan} {
		l, s, _ := w.roster(co)
		for i, u := range append([]core.CompanyUnit{l}, s...) {
			for _, st := range veteranStats[:i%len(veteranStats)+1] { // the leader spends Might, the next Might and Will...
				w.try("spend "+st+" of "+u.Title(), w.liveApply(co, m, core.LiveAction{Kind: core.LiveStatChange, UnitID: u.ID, Stat: st, Delta: -1}))
			}
		}
	}
	w.show(gondor)
	w.show(rohan)
}
