package core

import "testing"

func TestLostAsksForTheAbilityByItsKey(t *testing.T) {
	for _, key := range []string{AbilityStandFast, AbilityHeroicActionsOnCompany} {
		u := CompanyUnit{LostAbilities: []InjuryLoss{{Ability: key, Injury: "Broken Honour"}}}
		if !u.Lost(key) {
			t.Errorf("%s: not lost", key)
		}
		for _, other := range []string{AbilityStandFast, AbilityHeroicActionsOnCompany} {
			if other != key && u.Lost(other) {
				t.Errorf("%s lost with only %s taken", other, key)
			}
		}
	}
	if (CompanyUnit{}).Lost(AbilityStandFast) {
		t.Error("nothing lost")
	}
}
