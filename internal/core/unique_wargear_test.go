package core

import "testing"

// A company's unique wargear that names who may take it (The Shire: Hobbit Heroes: Pony; City of Umbar: Corsair: Smoke Bombs;
// Grand Army of the South: Mahûd: War Camel) is not one of the profile's options, so the options rule must not refuse it.
func TestUniqueWargearForAKindOfModelIsBuyable(t *testing.T) {
	rs := shippedRules(t)
	for _, tc := range []struct {
		company, item, unit string
		role                UnitRole
		want                string // the reason it is refused, empty when buyable
	}{
		{"The Shire", "Pony", "Hobbit Militia", RoleLeader, ""},
		{"The Shire", "Pony", "Hobbit Militia", RoleWarrior, "Heroes only"},
		{"The Shire", "Pony", "Ranger of the North", RoleLeader, "Hobbit only"},
	} {
		c := rs.BC.companies[tc.company]
		sh := shop{rs: rs, bcC: c, d: CompanyDetail{Influence: 20}}
		o, ok := rs.BC.Offer(c, tc.item)
		if !ok {
			t.Fatalf("%s: no %s in the armoury", tc.company, tc.item)
		}
		got, _ := sh.whyNot(CompanyUnit{Type: tc.unit, Role: tc.role}, o)
		if got != tc.want {
			t.Errorf("%s: %s for %s %s: reason %q, want %q", tc.company, tc.item, tc.role, tc.unit, got, tc.want)
		}
	}
}

// A company's unique wargear is spelled in its charts as its page spells it, not as the gear annotation or vanilla option does.
func TestUniqueWargearSpelledAsTheCompanyPageDoes(t *testing.T) {
	rs := shippedRules(t)
	p, _ := rs.BC.Profile("Halls of Thranduil")
	n := 0
	for _, s := range p.Start {
		for _, w := range s.Wargear {
			if lower(w) == "elven glaive" || lower(w) == "elven-made glaive" {
				n++
				if w != "Elven-made Glaive" {
					t.Errorf("start wargear %q, the company page spells it Elven-made Glaive", w)
				}
			}
		}
	}
	for _, r := range rs.BC.companies["Halls of Thranduil"].Reinforcements {
		for _, w := range r.Result.Wargear {
			n++
			if lower(w) != "shield" && lower(w) != "elf bow" && w != "Elven-made Glaive" {
				t.Errorf("chart wargear %q", w)
			}
		}
	}
	if n == 0 {
		t.Fatal("no glaive found")
	}
}
