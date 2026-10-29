package core

import (
	"strings"
	"testing"
)

// TestCustomRulesetElvenWeaponry: Elven-made weaponry is restricted to Elf models; a company may let other models of its own buy and wear
// it (elvenWeaponry, a Who), and says so in its company rules. Fornost has a Rivendell Warrior (Elf), Warriors of Minas Tirith (Man) and
// a Hobbit Archer.
func TestCustomRulesetElvenWeaponry(t *testing.T) {
	type who = map[string]any
	const elfOnly = "Elven-made weaponry is for Elves"
	for _, tt := range []struct {
		name     string
		option   any // the elvenWeaponry of Fornost; nil: not set
		unit     string
		refusal  string
		sentence string // the company rule the page prints; empty for none
	}{
		{"not set: a Man cannot buy it", nil, "Warrior of Minas Tirith", elfOnly, ""},
		{"not set: an Elf can", nil, "Rivendell Warrior", "", ""},
		{"all models: a Man can", who{}, "Warrior of Minas Tirith", "", "Every model of the company may buy and wear Elven-made weaponry, which is otherwise restricted to Elf models."},
		{"all models: a Hobbit can", who{}, "Hobbit Archer", "", "Every model of the company may buy and wear Elven-made weaponry, which is otherwise restricted to Elf models."},
		{"units: the named unit can", who{"units": []any{"Warrior of Minas Tirith"}}, "Warrior of Minas Tirith", "", "Warrior of Minas Tirith models may buy and wear Elven-made weaponry, which is otherwise restricted to Elf models."},
		{"units: another unit cannot", who{"units": []any{"Warrior of Minas Tirith"}}, "Hobbit Archer", elfOnly, ""},
		{"units: the Elf still can", who{"units": []any{"Warrior of Minas Tirith"}}, "Rivendell Warrior", "", ""},
		{"keywords: a unit with the keyword can", who{"keywords": []any{"Man"}}, "Warrior of Minas Tirith", "", "Man models may buy and wear Elven-made weaponry, which is otherwise restricted to Elf models."},
		{"keywords: a unit without it cannot", who{"keywords": []any{"Man"}}, "Hobbit Archer", elfOnly, ""},
		{"heroes only: a hero can", who{"hero": true}, "Warrior of Minas Tirith", "", "Hero models may buy and wear Elven-made weaponry, which is otherwise restricted to Elf models."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) {
				if tt.option != nil {
					companyDoc(t, doc, "Fornost")["elvenWeaponry"] = tt.option
				}
			})
			bcCo, _ := rs.BC.Company("Fornost")
			sh := shop{rs: rs, bcC: bcCo}
			sh.d.Influence = 10
			offer, ok := rs.BC.Offer(bcCo, "Elven spear")
			if !ok {
				t.Fatal("no Elven spear in the armoury")
			}
			why, _ := sh.whyNot(CompanyUnit{Type: tt.unit, Role: RoleHero}, offer)
			if why != tt.refusal {
				t.Errorf("%s: refusal %q, want %q", tt.unit, why, tt.refusal)
			}
			p, _ := rs.BC.Profile("Fornost")
			var got string
			for _, r := range p.OwnRules() {
				if strings.Contains(r, "Elven-made weaponry") {
					got = r
				}
			}
			if (tt.sentence != "" && got != tt.sentence) || (tt.option == nil && got != "") {
				t.Errorf("company rule %q, want %q", got, tt.sentence)
			}
		})
	}
}

// The items a company keeps for certain units stay for them: the option opens the Elven-made weaponry, not the unique wargear.
func TestElvenWeaponryKeepsUniqueWargearExclusive(t *testing.T) {
	rs := customRuleset(t, func(doc map[string]any) { companyDoc(t, doc, "City of Umbar")["elvenWeaponry"] = map[string]any{} })
	bcCo, _ := rs.BC.Company("City of Umbar")
	sh := shop{rs: rs, bcC: bcCo}
	sh.d.Influence = 10
	smoke, ok := rs.BC.Offer(bcCo, "Smoke Bombs")
	if !ok || !smoke.Exclusive || smoke.Who == nil {
		t.Fatalf("Smoke Bombs: %+v", smoke)
	}
	if why := sh.whoReason(CompanyUnit{Type: "Haradrim Warrior", Role: RoleHero}, smoke); why == "" {
		t.Error("a unique item for the Corsairs is open to a Haradrim Warrior because the company may wear Elven-made weaponry")
	}
	if why := sh.whoReason(CompanyUnit{Type: "Corsair of Umbar", Role: RoleHero}, smoke); why != "" {
		t.Errorf("a Corsair is refused its own item: %q", why)
	}
}
