package core

import (
	"slices"
	"strings"
	"testing"
)

func TestUnitAnchor(t *testing.T) {
	for name, want := range map[string]string{
		"Warrior of Minas Tirith": "u-warrior-of-minas-tirith",
		"Battlin' Brandybuck":     "u-battlin-brandybuck",
		"Lake-town Guard":         "u-lake-town-guard",
		"Mahûd Warrior":           "u-mahud-warrior",
		"  Ranger  of   Arnor ":   "u-ranger-of-arnor",
	} {
		if got := UnitAnchor(name); got != want {
			t.Errorf("UnitAnchor(%q) = %q, want %q", name, got, want)
		}
	}
}

// Every unit the shipped companies name has a card, with a unique anchor, a profile and the companies that field it.
func TestBaseUnitsShipped(t *testing.T) {
	rs := shippedRules(t)
	units := rs.BaseUnits()
	if len(units) < 100 {
		t.Fatalf("only %d base units for the shipped rule set", len(units))
	}
	ids := map[string]bool{}
	byName := map[string]BaseUnit{}
	for _, u := range units {
		if ids[u.ID] {
			t.Errorf("anchor %q is used twice", u.ID)
		}
		ids[u.ID] = true
		byName[u.Name] = u
		if !strings.HasPrefix(u.ID, "u-") || u.ID != strings.ToLower(u.ID) || strings.ContainsAny(u.ID, " '\"") {
			t.Errorf("%s: anchor %q is not a plain fragment", u.Name, u.ID)
		}
		if len(u.Preview.Stats) == 0 && len(u.Includes) == 0 {
			t.Errorf("%s: neither a profile nor the units it brings", u.Name)
		}
		if len(u.Companies) == 0 {
			t.Errorf("%s: no company fields it", u.Name)
		}
	}
	if !slices.IsSortedFunc(units, func(a, b BaseUnit) int { return strings.Compare(NameKey(a.Name), NameKey(b.Name)) }) {
		t.Error("the units are not sorted by name")
	}
	// every unit every company names is on the list
	for _, p := range rs.BC.Profiles() {
		for _, s := range p.Start {
			if _, ok := byName[s.Unit]; !ok {
				t.Errorf("%s starts with %q, which has no card", p.Name, s.Unit)
			}
		}
		for _, chart := range [][]ChartRow{p.Reinforcements, p.SpecialChart} {
			for _, r := range chart {
				if r.Result.Unit == "" {
					continue
				}
				if _, ok := byName[r.Result.Unit]; !ok {
					t.Errorf("%s recruits %q, which has no card", p.Name, r.Result.Unit)
				}
			}
		}
	}
	w, ok := byName["Warrior of Minas Tirith"]
	if !ok {
		t.Fatal("Warrior of Minas Tirith has no card")
	}
	if w.ID != "u-warrior-of-minas-tirith" || w.Preview.Points == 0 || len(w.Preview.Wargear) == 0 || len(w.Preview.Keywords.Model) == 0 {
		t.Errorf("Warrior of Minas Tirith card: %+v", w)
	}
	if !slices.Contains(w.Companies, "Minas Tirith") {
		t.Errorf("Minas Tirith fields the Warrior of Minas Tirith: %v", w.Companies)
	}
	// the list is remembered: the second call is the same slice
	if &rs.BaseUnits()[0] != &units[0] {
		t.Error("BaseUnits is rebuilt on every call")
	}
}

// A company's own name for a vanilla profile shows the base profile it stands for.
func TestBaseUnitsAlias(t *testing.T) {
	rs := shippedRules(t)
	for _, u := range rs.BaseUnits() {
		if u.Base != "" && NameKey(u.Base) == NameKey(u.Name) {
			t.Errorf("%s names itself as its base", u.Name)
		}
	}
}

func TestGlossaryUnitsAreForTheCampaignOnly(t *testing.T) {
	rs := shippedRules(t)
	if len(rs.Glossary.Units) != 0 {
		t.Error("the rule set's glossary file holds no units; ForCampaign adds them")
	}
	g := rs.GlossaryWithUnits()
	if len(g.Units) != len(rs.BaseUnits()) || len(g.Terms) != len(rs.Glossary.Terms) {
		t.Errorf("units %d terms %d", len(g.Units), len(g.Terms))
	}
}
