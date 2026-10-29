package core

import (
	"slices"
	"strings"
	"testing"
)

// The three forms of a unit in the app (docs/customizing-rulesets.md, "Units"): an annotation of a vanilla unit, a unit that extends another
// (own or shared), and a brand-new unit with a profile. Each is fielded by Arnor in a custom ruleset and treated like a vanilla profile.

func barrowHound() map[string]any {
	return map[string]any{
		"name": "Barrow Hound", "aka": []any{"Hound of the Barrows"}, "points": 6.0,
		"profile": map[string]any{"movement": 9.0, "fight": 3.0, "shoot": "-", "strength": 3.0, "defence": 4.0, "attack": 1.0, "wounds": 1.0,
			"courage": 5.0, "intelligence": 3.0, "race": []any{"Hound"}, "unitType": []any{"Warrior", "Infantry"}, "specialRules": []any{"Fearless"}, "wargear": []any{"Spear"}},
		"options": []any{map[string]any{"name": "Shield", "points": 1.0}},
	}
}

// unitsMutation adds a brand-new unit and a shared extends unit to the shipped file and has Arnor field them everywhere a unit is named.
func unitsMutation(t *testing.T, doc map[string]any) {
	doc["units"] = append(doc["units"].([]any), barrowHound(),
		map[string]any{"name": "Arnor Scout", "aka": []any{"Scout of Arnor"}, "extends": "Ranger of Arnor", "with": []any{"Spear"}, "addKeywords": []any{"Dunedain"}},
		map[string]any{"name": "Senior Scout", "extends": "Arnor Scout", "addRules": []any{"Fearless"}}, // a shared unit on a shared unit
	)
	for _, c := range doc["companies"].([]any) {
		co := c.(map[string]any)
		if co["name"] != "Arnor" {
			continue
		}
		start := co["start"].(map[string]any)
		start["units"] = append(start["units"].([]any),
			map[string]any{"unit": "Barrow Hound", "count": 1.0}, map[string]any{"unit": "Arnor Scout", "count": 2.0})
		rows := co["reinforcements"].(map[string]any)["rows"].([]any)
		rows[3].(map[string]any)["unit"] = "Arnor Scout"
		rows[4].(map[string]any)["unit"] = "Hound of the Barrows" // by its aka
		co["advancements"] = append(co["advancements"].([]any),
			map[string]any{"from": "Warrior of Arnor", "to": []any{"Barrow Hound", "Senior Scout"}},
			map[string]any{"from": "Barrow Hound", "to": []any{"Arnor Scout"}})
		co["promotions"] = append(co["promotions"].([]any), map[string]any{"text": "A scout becomes a Barrow Hound.", "from": "Arnor Scout", "on": "heroInTheMaking", "becomes": "Ranger of the North", "pathChoice": "any"})
	}
}

func TestUnitFormsAreFieldedByACompany(t *testing.T) {
	rs := customRuleset(t, func(doc map[string]any) { unitsMutation(t, doc) })
	p, ok := rs.BC.Profile("Arnor")
	if !ok {
		t.Fatal("no company Arnor")
	}

	t.Run("a brand-new unit resolves to its own profile, by name and by aka", func(t *testing.T) {
		for _, name := range []string{"Barrow Hound", "Hound of the Barrows"} {
			v, alias, found := rs.ResolveUnit(p, name)
			if !found || alias != nil || v.Name != "Barrow Hound" || v.Points != 6 || v.Stats["movement"] != 9 || v.Stats["courage"] != 5 {
				t.Fatalf("%s: found %v alias %v profile %+v", name, found, alias, v)
			}
			if !slices.Equal(v.Race, []string{"Hound"}) || !slices.Equal(v.UnitTypes, []string{"Warrior", "Infantry"}) || !slices.Contains(v.SpecialRules, "Fearless") ||
				!slices.Equal(v.Wargear, []string{"Spear"}) || len(v.Options) != 1 || v.Options[0].Name != "Shield" || v.Options[0].Points != 1 {
				t.Errorf("%s: the profile lost its words, rules, wargear or options: %+v", name, v)
			}
		}
		if !rs.Vanilla.IsRace("Hound") {
			t.Error("the race of a brand-new unit is a race of the game")
		}
	})
	t.Run("a shared unit that extends another resolves to the base profile with the delta, a chain too", func(t *testing.T) {
		v, alias, found := rs.ResolveUnit(p, "Scout of Arnor")
		if !found || alias == nil || v.Name != "Ranger of Arnor" || alias.Label != "Arnor Scout" || !slices.Equal(alias.Selected, []string{"Spear"}) || !slices.Equal(alias.Keywords, []string{"Dunedain"}) {
			t.Fatalf("found %v alias %+v profile %s", found, alias, v.Name)
		}
		v, alias, found = rs.ResolveUnit(p, "Senior Scout")
		if !found || v.Name != "Ranger of Arnor" || alias == nil || !slices.Equal(alias.AddRules, []string{"Fearless"}) || !slices.Equal(alias.Selected, []string{"Spear"}) {
			t.Fatalf("chain: found %v alias %+v profile %s", found, alias, v.Name)
		}
		if _, _, found := rs.ResolveUnit(p, "Nobody"); found {
			t.Error("a name that resolves nowhere is not found")
		}
	})
	t.Run("the charts and advancements name them", func(t *testing.T) {
		co, _ := rs.BC.Company("Arnor")
		if r := co.Reinforcements[3].Result; r.Unit != "Arnor Scout" {
			t.Errorf("chart row 4 = %+v", r)
		}
		if r := co.Reinforcements[4].Result; r.Unit != "Hound of the Barrows" {
			t.Errorf("chart row 5 = %+v", r)
		}
		for _, name := range chartUnits(co) {
			if _, _, found := rs.ResolveUnit(p, name); !found {
				t.Errorf("chart unit %q does not resolve", name)
			}
		}
		for _, adv := range rs.BC.Advancements(co) {
			for _, to := range adv.To {
				if _, _, found := rs.ResolveUnit(p, to.Unit); !found {
					t.Errorf("advancement target %q does not resolve", to.Unit)
				}
			}
		}
	})
	t.Run("the company builds, rates and keywords them like vanilla units", func(t *testing.T) {
		st := CompanyState{Company: CampaignCompany{ID: 1, Name: "Arnor", Faction: "Arnor"}, Influence: 20, Gold: 20, Units: StartingUnits(p, p.DefaultHeroes())}
		for i := range st.Units {
			st.Units[i].ID = int64(i + 1)
		}
		d := BuildDetail(rs, CampaignSettings{}, st)
		by := map[string]CompanyUnit{}
		for _, u := range d.Roster {
			if _, seen := by[u.Type]; !seen {
				by[u.Type] = u
			}
		}
		hound, ok := by["Barrow Hound"]
		if !ok {
			t.Fatalf("the Barrow Hound is not on the roster: %v", d.Roster)
		}
		if hound.Points != 6 || hound.Kind != KindInfantry || len(hound.Stats) == 0 {
			t.Errorf("hound: points %d kind %q stats %v", hound.Points, hound.Kind, hound.Stats)
		}
		if !hasKeyword2(hound.Keywords, "Hound") {
			t.Errorf("hound keywords %+v", hound.Keywords)
		}
		scout, ok := by["Arnor Scout"]
		if !ok {
			t.Fatal("no Arnor Scout on the roster")
		}
		if scout.Points <= 0 || !hasKeyword2(scout.Keywords, "Dunedain") {
			t.Errorf("scout: points %d keywords %+v", scout.Points, scout.Keywords)
		}
		// the armoury: a hound buys what its profile's options let it
		sh := newShop(rs, CampaignSettings{}, st)
		var shield, spear bool
		for _, o := range offeredWargear(sh, hound) {
			shield, spear = shield || o == "Shield", spear || o == "Spear"
		}
		if !shield {
			t.Error("the Shield option of the Barrow Hound is not offered")
		}
		if spear {
			t.Error("a Spear is not an option of the Barrow Hound (only wargear it starts with)")
		}
		bought := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: st.Company, Units: []UnitState{{ID: 1, Base: "Barrow Hound", Role: RoleWarrior, Bought: []string{"Shield"}}}}).Roster[0]
		if bought.Points <= 6 {
			t.Errorf("a hound with a bought Shield is rated %d, more than its 6 points", bought.Points)
		}
		promoted := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: st.Company, Units: []UnitState{{ID: 1, Base: "Arnor Scout", Role: RoleWarrior}}}).Roster[0]
		if promoted.Type != "Arnor Scout" || promoted.Points != scout.Points {
			t.Errorf("a unit promoted to the shared unit: type %q points %d (want %d)", promoted.Type, promoted.Points, scout.Points)
		}
		if d.MaxUnits == 0 || len(d.Roster) != len(st.Units) {
			t.Errorf("roster %d of %d", len(d.Roster), len(st.Units))
		}
	})
	t.Run("a promotion keeps what the new profile lists", func(t *testing.T) {
		co, _ := rs.BC.Company("Arnor")
		if got := listedNames(rs, co, "Barrow Hound", nil); !slices.Contains(got, "Shield") || !slices.Contains(got, "Spear") {
			t.Errorf("listed by the hound: %v", got)
		}
		if got := listedNames(rs, co, "Arnor Scout", nil); !slices.Contains(got, "Spear") {
			t.Errorf("listed by the scout: %v", got)
		}
	})
}

func TestUnitNamesFollowTheLayers(t *testing.T) {
	t.Run("a company's own unit shadows a shared unit of the same name", func(t *testing.T) {
		rs := customRuleset(t, func(doc map[string]any) {
			unitsMutation(t, doc)
			for _, c := range doc["companies"].([]any) {
				if co := c.(map[string]any); co["name"] == "Arnor" {
					co["units"] = append(asList(co["units"]), map[string]any{"name": "Arnor Scout", "extends": "Warrior of Arnor", "overrides": true})
				}
			}
		})
		p, _ := rs.BC.Profile("Arnor")
		if v, _, found := rs.ResolveUnit(p, "Arnor Scout"); !found || v.Name != "Warrior of Arnor" {
			t.Errorf("own unit does not win: %s", v.Name)
		}
		other, _ := rs.BC.Profile("Rohan")
		if v, _, found := rs.ResolveUnit(other, "Arnor Scout"); !found || v.Name != "Ranger of Arnor" {
			t.Errorf("another company reads the shared unit: %s", v.Name)
		}
	})
	t.Run("a shared brand-new unit nobody fields is valid, and every company can field it", func(t *testing.T) {
		rs := customRuleset(t, func(doc map[string]any) { doc["units"] = append(doc["units"].([]any), barrowHound()) })
		for _, name := range []string{"Arnor", "Rohan"} {
			p, _ := rs.BC.Profile(name)
			if _, _, found := rs.ResolveUnit(p, "Barrow Hound"); !found {
				t.Errorf("%s cannot resolve the shared unit", name)
			}
		}
	})
	t.Run("a company-local brand-new unit is the company's", func(t *testing.T) {
		rs := customRuleset(t, func(doc map[string]any) {
			for _, c := range doc["companies"].([]any) {
				if co := c.(map[string]any); co["name"] == "Arnor" {
					co["units"] = append(asList(co["units"]), barrowHound())
				}
			}
		})
		p, _ := rs.BC.Profile("Arnor")
		if v, _, found := rs.ResolveUnit(p, "Barrow Hound"); !found || v.Points != 6 {
			t.Errorf("own brand-new unit: %+v", v)
		}
	})
	t.Run("a brand-new unit with a vanilla name needs overrides and then replaces the vanilla profile", func(t *testing.T) {
		clash := barrowHound()
		clash["name"] = "Warrior of Arnor"
		delete(clash, "aka")
		if _, err := customRulesetErr(t, func(doc map[string]any) { doc["units"] = append(doc["units"].([]any), clash) }); err == nil {
			t.Fatal("a profile with a vanilla name is refused without overrides")
		}
		clash["overrides"] = true
		rs := customRuleset(t, func(doc map[string]any) { doc["units"] = append(doc["units"].([]any), clash) })
		p, _ := rs.BC.Profile("Arnor")
		if v, _, _ := rs.ResolveUnit(p, "Warrior of Arnor"); v.Points != 6 {
			t.Errorf("overridden profile has %d points", v.Points)
		}
	})
	t.Run("an unknown unit is refused with the closest name", func(t *testing.T) {
		_, err := customRulesetErr(t, func(doc map[string]any) {
			unitsMutation(t, doc)
			for _, c := range doc["companies"].([]any) {
				if co := c.(map[string]any); co["name"] == "Arnor" {
					co["start"].(map[string]any)["units"].([]any)[0].(map[string]any)["unit"] = "Barow Hound"
				}
			}
		})
		if err == nil || !strings.Contains(err.Error(), `"Barrow Hound"`) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestBrandNewUnitProfileIsChecked(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(u map[string]any)
		want   string
	}{
		"missing characteristic": {func(u map[string]any) { delete(u["profile"].(map[string]any), "courage") }, "profile.courage: a unit needs its courage"},
		"no race":                {func(u map[string]any) { delete(u["profile"].(map[string]any), "race") }, "profile.race: a unit needs its race"},
		"unknown key":            {func(u map[string]any) { u["profile"].(map[string]any)["speed"] = 3.0 }, `profile.speed: unknown field "speed"`},
		"a word for a number":    {func(u map[string]any) { u["profile"].(map[string]any)["attack"] = "one" }, "profile.attack: attack is a number"},
		"unknown rule":           {func(u map[string]any) { u["profile"].(map[string]any)["specialRules"] = []any{"Fearles"} }, `"Fearles"`},
		"unknown wargear":        {func(u map[string]any) { u["profile"].(map[string]any)["wargear"] = []any{"Sp3ar"} }, `"Sp3ar"`},
	} {
		t.Run(name, func(t *testing.T) {
			u := barrowHound()
			tc.mutate(u)
			_, err := customRulesetErr(t, func(doc map[string]any) { doc["units"] = append(doc["units"].([]any), u) })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func hasKeyword2(k UnitKeywords, word string) bool {
	return slices.ContainsFunc(k.Model, func(w UnitKeyword) bool { return w.Name == word })
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}
