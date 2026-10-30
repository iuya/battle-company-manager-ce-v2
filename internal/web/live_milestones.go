package web

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// milestoneView is one milestone of a unit card in the field (the milestone template): what earns experience in the game, lit once
// earned (docs/live-match.md "Experience: which actions earn it"). For the owner it is a button: Get opens a window, Post records Vals
// at once (an action, or the undo of the entry that lit it); Locked (lit, and not undone by a press) disables it. Count is a number
// shown on it (Heroes and Monsters slain).
type milestoneView struct {
	Icon, Label, Tip string
	On, Locked       bool
	Get, Post, Vals  string
	Count            int
	CountTip         string
}

// Milestones are the milestones of a unit in the field. Wounded opens the window to pick who was wounded (it can happen again). Fate
// (heroes with Fate only: the mark alone, no Fate point is spent, so it can be pressed with none left), Courage (a passed Broken
// courage test) and the objective switch at once. A lit Fate or Courage undoes the unit's last entry of it (LiveUnit.FateEntry,
// BrokenEntry) and stays lit while an earlier one still earns it; lit with no entry of its own (a Fate point saved in a wound
// declaration) it is locked: undone from the log.
func (v liveView) Milestones(unitID int64, l core.LiveUnit) []milestoneView {
	unit := strconv.FormatInt(unitID, 10)
	actions := fmt.Sprintf("/campaigns/%d/matches/%d/live/%d/actions", v.CampaignID, v.MatchID, v.CompanyID)
	// toggle lights m when on: pressing it then undoes entry, or, with no entry to undo, it is locked; off, pressing it records action.
	toggle := func(m milestoneView, on bool, entry int64, action ...string) milestoneView {
		m.On = on
		if on && entry != 0 {
			m.Post = fmt.Sprintf("/campaigns/%d/matches/%d/live/log/%d/undo", v.CampaignID, v.MatchID, entry)
			m.Vals = hxVals("company", strconv.FormatInt(v.CompanyID, 10))
		} else {
			m.Post, m.Vals, m.Locked = actions, hxVals(action...), on
		}
		return m
	}
	out := []milestoneView{{Icon: "machete", Label: "Wounded", Tip: "Wounded an enemy", On: l.Gains[core.GainWoundedEnemy] > 0,
		Get:   fmt.Sprintf("/campaigns/%d/matches/%d/live/%d/units/%d/sheet/wound", v.CampaignID, v.MatchID, v.CompanyID, unitID),
		Count: l.Gains[core.GainSlainHeroMons], CountTip: "Heroes and Monsters slain"}}
	if l.Fate.Start > 0 {
		out = append(out, toggle(milestoneView{Icon: "shield-reflect", Label: "Fate", Tip: "Prevented a wound with Fate"},
			l.Gains[core.GainFatePrevented] > 0, l.FateEntry, "kind", "fate", "unit", unit, "count", "0"))
	}
	out = append(out, toggle(milestoneView{Icon: "mighty-force", Label: "Courage", Tip: "Passed a Broken courage test"},
		l.Gains[core.GainBrokenPassed] > 0, l.BrokenEntry, "kind", "broken", "unit", unit))
	on := "1"
	if l.Objective {
		on = "0"
	}
	// The objective is a switch: its action both lights and clears it, so it is never locked.
	out = append(out, milestoneView{Icon: "flying-flag", Label: "Objective", Tip: "Got the objective", On: l.Objective, Post: actions,
		Vals: hxVals("kind", "objective", "unit", unit, "on", on)})
	return out
}

// hxVals is the JSON object of an hx-vals attribute, its keys in the order given: hxVals("kind", "fate", "unit", "5").
func hxVals(kv ...string) string {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i+1 < len(kv); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(kv[i])
		v, _ := json.Marshal(kv[i+1])
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.String()
}
