package core

import (
	"slices"
	"testing"
)

// companyDoc is the company of the Battle Companies document with that name.
func companyDoc(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	for _, c := range doc["companies"].([]any) {
		if m := c.(map[string]any); m["name"] == name {
			return m
		}
	}
	t.Fatalf("no company %q", name)
	return nil
}

// TestCustomRulesetWho: one row per field of the Who selector, through the four places that carry one (the gear, the hero upgrades,
// the rules a company grants, the starting heroes). Fornost has a Rivendell Warrior (Elf, Rivendell), two Warriors of Minas Tirith
// and a Hobbit Archer; its alignment is good.
func TestCustomRulesetWho(t *testing.T) {
	type who = map[string]any
	gear := []struct {
		name  string
		who   who
		hero  bool // the unit is a hero
		unit  string
		wants string // the refusal; empty when admitted
	}{
		{"hero: a hero is admitted", who{"hero": true}, true, "Warrior of Minas Tirith", ""},
		{"hero: a warrior is refused", who{"hero": true}, false, "Warrior of Minas Tirith", "Heroes only"},
		{"hero false: a hero is refused", who{"hero": false}, true, "Warrior of Minas Tirith", "Warriors only"},
		{"hero false: a warrior is admitted", who{"hero": false}, false, "Warrior of Minas Tirith", ""},
		{"race: the race is admitted", who{"race": []any{"Elf"}}, true, "Rivendell Warrior", ""},
		{"race: another race is refused", who{"race": []any{"Elf"}}, true, "Warrior of Minas Tirith", "Elf only"},
		{"keywords: a faction of the unit is admitted", who{"keywords": []any{"Rivendell"}}, true, "Rivendell Warrior", ""},
		{"keywords: another unit is refused", who{"keywords": []any{"Rivendell"}}, true, "Warrior of Minas Tirith", "Rivendell only"},
		{"keywords: Cavalry is a model that rides, not one that could", who{"keywords": []any{"Cavalry"}}, true, "Warrior of Minas Tirith", "Cavalry only"},
		{"units: the exact name is admitted", who{"units": []any{"Hobbit Archer"}}, false, "Hobbit Archer", ""},
		{"units: another unit is refused", who{"units": []any{"Hobbit Archer"}}, false, "Warrior of Minas Tirith", "Hobbit Archer only"},
		{"units: part of a name is no name", who{"units": []any{"Isengard Warg"}}, true, "Isengard Warg Rider", "Isengard Warg only"},
		{"except: the named unit is refused", who{"except": []any{"Hobbit Archer"}}, false, "Hobbit Archer", "Not for Hobbit Archer"},
		{"except: the others are admitted", who{"except": []any{"Hobbit Archer"}}, false, "Warrior of Minas Tirith", ""},
		{"alignment: the company's is admitted", who{"alignment": "good"}, true, "Warrior of Minas Tirith", ""},
		{"alignment: the other is refused", who{"alignment": "evil"}, true, "Warrior of Minas Tirith", "Evil companies only"},
		{"refusal: the file words it", who{"hero": true, "refusal": "Only for the brave"}, false, "Warrior of Minas Tirith", "Only for the brave"},
		{"empty: everyone", who{}, false, "Warrior of Minas Tirith", ""},
	}
	for _, tt := range gear {
		t.Run("gear/"+tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) { gearAnnotation(t, doc, "Elven spear")["who"] = tt.who })
			bcCo, _ := rs.BC.Company("Fornost")
			sh := shop{rs: rs, bcC: bcCo}
			offer, ok := rs.BC.Offer(bcCo, "Elven spear")
			if !ok {
				t.Fatal("no Elven spear in the armoury")
			}
			role := RoleWarrior
			if tt.hero {
				role = RoleHero
			}
			if got := sh.whoReason(CompanyUnit{Type: tt.unit, Role: role}, offer); got != tt.wants {
				t.Errorf("%s as %v: refusal %q, want %q", tt.unit, role, got, tt.wants)
			}
		})
	}

	upgrades := []struct {
		name string
		who  who
		unit string
		want bool
	}{
		{"hero upgrade/race", who{"race": []any{"Elf"}}, "Rivendell Warrior", true},
		{"hero upgrade/race refuses", who{"race": []any{"Elf"}}, "Knight of Dol Amroth", false},
		{"hero upgrade/keywords", who{"keywords": []any{"Dol Amroth"}}, "Knight of Dol Amroth", true},
		{"hero upgrade/keywords refuses", who{"keywords": []any{"Dol Amroth"}}, "Axeman of Lossarnach", false},
		{"hero upgrade/units", who{"units": []any{"Axeman of Lossarnach"}}, "Axeman of Lossarnach", true},
		{"hero upgrade/units are exact", who{"units": []any{"Isengard Warg"}}, "Isengard Warg Rider", false},
		{"hero upgrade/except", who{"except": []any{"Axeman of Lossarnach"}}, "Axeman of Lossarnach", false},
		{"hero upgrade/except admits the others", who{"except": []any{"Axeman of Lossarnach"}}, "Knight of Dol Amroth", true},
		{"hero upgrade/none: everyone", nil, "Axeman of Lossarnach", true},
	}
	for _, tt := range upgrades {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) {
				up := companyDoc(t, doc, "Fiefdoms")["heroUpgrades"].([]any)[0].(map[string]any)
				if tt.who == nil {
					delete(up, "who")
				} else {
					up["who"] = tt.who
				}
			})
			p, _ := rs.BC.Profile("Fiefdoms")
			v, _, ok := rs.ResolveUnit(p, tt.unit)
			if !ok {
				t.Fatalf("no unit %q", tt.unit)
			}
			if got := p.HeroUpgrades[0].Allows(v); got != tt.want {
				t.Errorf("%s may take the upgrade = %v, want %v", tt.unit, got, tt.want)
			}
		})
	}

	start := []struct {
		name string
		who  who
		unit string
		want bool
	}{
		{"start heroes/units admits the named", who{"units": []any{"Hobbit Archer"}}, "Hobbit Archer", true},
		{"start heroes/units refuses the others", who{"units": []any{"Hobbit Archer"}}, "Rivendell Warrior", false},
		{"start heroes/units are exact", who{"units": []any{"Hobbit Archer", "Isengard Warg"}}, "Isengard Warg Rider", false},
		{"start heroes/except refuses the named", who{"except": []any{"Rivendell Warrior"}}, "Rivendell Warrior", false},
		{"start heroes/except admits the others", who{"except": []any{"Rivendell Warrior"}}, "Hobbit Archer", true},
	}
	for _, tt := range start {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) { companyDoc(t, doc, "Fornost")["heroes"] = who{"who": tt.who} })
			p, _ := rs.BC.Profile("Fornost")
			if got := p.CanLead(tt.unit); got != tt.want {
				t.Errorf("%s may lead = %v, want %v", tt.unit, got, tt.want)
			}
		})
	}

	granted := []struct {
		name string
		who  who
		want []string // the units of Rohan's company that get the rule, by type and role
	}{
		{"granted/hero", who{"hero": true}, []string{"Rider of Rohan hero"}},
		{"granted/warriors", who{"hero": false}, []string{"Rider of Rohan warrior", "Warrior of Rohan warrior"}},
		{"granted/keywords", who{"keywords": []any{"Rohan"}}, []string{"Rider of Rohan hero", "Rider of Rohan warrior", "Warrior of Rohan warrior"}},
		{"granted/units", who{"units": []any{"Warrior of Rohan"}}, []string{"Warrior of Rohan warrior"}},
		{"granted/except", who{"except": []any{"Warrior of Rohan"}}, []string{"Rider of Rohan hero", "Rider of Rohan warrior"}},
		{"granted/race", who{"race": []any{"Elf"}}, nil},
		{"granted/alignment", who{"alignment": "good"}, []string{"Rider of Rohan hero", "Rider of Rohan warrior", "Warrior of Rohan warrior"}},
		{"granted/none: everyone", nil, []string{"Rider of Rohan hero", "Rider of Rohan warrior", "Warrior of Rohan warrior"}},
	}
	for _, tt := range granted {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) {
				g := companyDoc(t, doc, "Rohan")["granted"].([]any)[0].(map[string]any)
				if tt.who == nil {
					delete(g, "who")
				} else {
					g["who"] = tt.who
				}
			})
			bcCo, _ := rs.BC.Company("Rohan")
			units := []CompanyUnit{{Type: "Rider of Rohan", Role: RoleLeader}, {Type: "Rider of Rohan", Role: RoleWarrior}, {Type: "Warrior of Rohan", Role: RoleWarrior}}
			applyCompanyRules(rs, bcCo, units)
			var got []string
			for _, u := range units {
				if slices.ContainsFunc(u.Rules, func(r UnitRule) bool { return r.Name == "Arise, Riders of Rohan!" }) {
					role := "warrior"
					if u.IsHero() {
						role = "hero"
					}
					got = append(got, u.Type+" "+role)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("get the rule: %v, want %v", got, tt.want)
			}
		})
	}
}
