package web

import (
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestMilestonesOfAUnit(t *testing.T) {
	v := liveView{CampaignID: 7, LiveCompany: core.LiveCompany{MatchID: 3, CompanyID: 1}}
	plain := v.Milestones(5, core.LiveUnit{})
	if len(plain) != 3 || plain[0].Label != "Wounded" || plain[1].Label != "Courage" || plain[2].Label != "Objective" {
		t.Fatalf("a unit without Fate has Wounded, Courage and the objective: %+v", plain)
	}
	if plain[1].Post != "/campaigns/7/matches/3/live/1/actions" || plain[1].Vals != `{"kind":"broken","unit":"5"}` || plain[1].On || plain[1].Locked {
		t.Errorf("an unlit Courage records a passed test: %+v", plain[1])
	}
	if plain[2].Vals != `{"kind":"objective","unit":"5","on":"1"}` {
		t.Errorf("an unlit objective switches it on: %+v", plain[2])
	}

	lit := v.Milestones(5, core.LiveUnit{Fate: core.LiveStat{Start: 2}, BrokenEntry: 44, Objective: true,
		Gains: map[string]int{core.GainFatePrevented: 1, core.GainBrokenPassed: 1, core.GainSlainHeroMons: 2}})
	fate, courage, objective := lit[1], lit[2], lit[3]
	if !fate.On || !fate.Locked {
		t.Errorf("Fate lit by a wound declaration has no entry to undo: locked: %+v", fate)
	}
	if !courage.On || courage.Locked || courage.Post != "/campaigns/7/matches/3/live/log/44/undo" || courage.Vals != `{"company":"1"}` {
		t.Errorf("a lit Courage undoes its entry: %+v", courage)
	}
	if !objective.On || objective.Locked || objective.Vals != `{"kind":"objective","unit":"5","on":"0"}` {
		t.Errorf("a lit objective switches it off: %+v", objective)
	}
	if lit[0].Count != 2 || lit[0].Get != "/campaigns/7/matches/3/live/1/units/5/sheet/wound" {
		t.Errorf("Wounded opens its window and counts the slain: %+v", lit[0])
	}
}
