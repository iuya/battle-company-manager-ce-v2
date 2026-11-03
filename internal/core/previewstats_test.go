package core

import (
	"slices"
	"strings"
	"testing"
)

func statValues(as []Attribute) map[string]int {
	m := map[string]int{}
	for _, a := range as {
		m[a.Name] = a.Value
	}
	return m
}

// A unit preview shows the stats the roster shows for the unit created with the same wargear, gear changes included.
func TestPreviewStatsEqualTheRosterStats(t *testing.T) {
	rs := shippedRules(t)
	checked, differ := 0, 0
	for _, p := range rs.BC.Profiles() {
		p = rs.finishPreviews(p.WithPreviews(rs.Vanilla))
		for key, pv := range p.Previews {
			if len(pv.Stats) == 0 {
				continue
			}
			var gear []string
			if _, g, ok := strings.Cut(key, "|"); ok && g != "" {
				gear = strings.Split(g, ",")
			}
			role := RoleWarrior
			if pv.HasHeroStats() {
				role = RoleHero
			}
			st := CompanyState{Company: CampaignCompany{ID: 1, Name: p.Name, Faction: p.Name}, Units: []UnitState{{ID: 1, Base: pv.Name, Role: role, Gear: gear}}}
			d := BuildDetail(rs, CampaignSettings{}, st)
			if len(d.Roster) == 0 {
				t.Errorf("%s: %s did not build", p.Name, key)
				continue
			}
			checked++
			got, want := statValues(pv.Stats), statValues(d.Roster[0].Stats)
			for k, v := range want {
				if got[k] != v {
					differ++
					t.Errorf("%s: preview %q %s = %d, the roster has %d", p.Name, key, k, got[k], v)
				}
			}
			var rules, gearNames []string
			for _, r := range d.Roster[0].Rules {
				rules = append(rules, r.Shown())
			}
			for _, e := range d.Roster[0].Equipment {
				gearNames = append(gearNames, e.Name)
			}
			if !slices.Equal(pv.Rules, rules) {
				differ++
				t.Errorf("%s: preview %q rules %v, the roster has %v", p.Name, key, pv.Rules, rules)
			}
			if !slices.Equal(pv.Wargear, gearNames) {
				differ++
				t.Errorf("%s: preview %q wargear %v, the roster has %v", p.Name, key, pv.Wargear, gearNames)
			}
		}
	}
	t.Logf("%d previews checked, %d stat differences", checked, differ)
	if checked < 100 {
		t.Errorf("only %d previews checked", checked)
	}
}

func TestPreviewShieldAddsDefence(t *testing.T) {
	rs := shippedRules(t)
	p, ok := rs.BC.Profile("Iron Hills")
	if !ok {
		t.Fatal("no Iron Hills")
	}
	p = rs.finishPreviews(p.WithPreviews(rs.Vanilla))
	pv, ok := p.Previews[PreviewKey("Iron Hills Warrior", []string{"shield", "spear"})]
	if !ok {
		t.Fatal("no preview of the Iron Hills Warrior with a spear and a shield")
	}
	if got := statValues(pv.Stats)["Defence"]; got != 7 {
		t.Errorf("with a shield: Defence %d, want 7", got)
	}
	if got := statValues(recruitStats(rs, p.BCCompany, "Iron Hills Warrior", "", []string{"spear"}, RoleWarrior))["Defence"]; got != 7 {
		t.Errorf("a spear alone completes the spear-and-shield option: Defence %d, want 7", got)
	}
	if got := statValues(recruitStats(rs, p.BCCompany, "Iron Hills Warrior", "", nil, RoleWarrior))["Defence"]; got != 6 {
		t.Errorf("without a shield: Defence %d, want 6", got)
	}
}

func TestPreviewListsWhatGearAndOptionsGive(t *testing.T) {
	rs := shippedRules(t)
	for _, c := range []struct{ company, unit, rule string }{
		{"Rivendell", "Noldorin Exile", "Stalk Unseen"},
	} {
		p, ok := rs.BC.Profile(c.company)
		if !ok {
			t.Fatalf("no %s", c.company)
		}
		p = rs.finishPreviews(p.WithPreviews(rs.Vanilla))
		found := false
		for _, pv := range p.Previews {
			if pv.Name != c.unit {
				continue
			}
			found = true
			if !slices.Contains(pv.Rules, c.rule) {
				t.Errorf("%s preview %q lacks %s: %v", c.unit, pv.Key, c.rule, pv.Rules)
			}
			if !slices.ContainsFunc(pv.RuleCards, func(r UnitRule) bool { return r.Shown() == c.rule && r.Description != "" }) {
				t.Errorf("%s preview %q: %s has no text for its tooltip", c.unit, pv.Key, c.rule)
			}
		}
		if !found {
			t.Errorf("%s has no preview in %s", c.unit, c.company)
		}
	}
}

func TestPreviewShowsTheChangesOfAnOption(t *testing.T) {
	rs := shippedRules(t)
	for _, c := range []struct {
		company, unit, stat string
		want                int
	}{{"Rivendell", "Noldorin Exile", "Movement", 8}, {"Wanderers in the Wild", "Battlin' Brandybuck", "Strength", 3}} {
		p, _ := rs.BC.Profile(c.company)
		p = rs.finishPreviews(p.WithPreviews(rs.Vanilla))
		if got := statValues(p.Preview(c.unit, nil).Stats)[c.stat]; got != c.want {
			t.Errorf("%s: %s %d, want %d", c.unit, c.stat, got, c.want)
		}
	}
}

// The preview of a bundle (the Vault Warden Team of Durin's Folk) carries a card for each member, with a profile and the points its company gives it.
func TestPreviewOfABundleHasItsMembers(t *testing.T) {
	rs := shippedRules(t)
	for _, p := range rs.BC.Profiles() {
		if p.Name != "Durin's Folk" {
			continue
		}
		p = rs.finishPreviews(p.WithPreviews(rs.Vanilla))
		pv := p.Preview("Vault Warden Team", nil)
		if len(pv.Members) != 2 {
			t.Fatalf("Vault Warden Team members = %d, want 2", len(pv.Members))
		}
		for _, m := range pv.Members {
			if len(m.Stats) == 0 || m.Points == 0 || m.Name == "" {
				t.Errorf("member %+v lacks a profile or its points", m)
			}
		}
		return
	}
	t.Fatal("no Durin's Folk")
}
