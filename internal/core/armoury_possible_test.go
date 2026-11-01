package core

import (
	"strings"
	"testing"
)

// possibleEntries is the armoury of a fresh company of the faction by item name.
func possibleEntries(t *testing.T, faction string) map[string]ArmouryEntry {
	t.Helper()
	sh, st := armourShop(t, faction)
	return entriesByName(t, sh, st)
}

func entriesByName(t *testing.T, sh shop, _ CompanyState) map[string]ArmouryEntry {
	t.Helper()
	out := map[string]ArmouryEntry{}
	for _, e := range sh.entries() {
		out[e.Name] = e
	}
	return out
}

// TestPossiblePurchasesPerCompany is the owner's rule (issue 57): an item is possible in a company when some unit that can exist
// there (its units now, and the profiles its charts, advancements and promotions give) could buy it under the rules, whatever the
// units carry, the influence or the slots now. Impossible ones are the only ones the armoury leaves out.
func TestPossiblePurchasesPerCompany(t *testing.T) {
	tests := []struct {
		company              string
		possible, impossible []string
	}{
		// Arnor: every officer is a Warrior of Arnor in Heavy Armour and every warrior is blocked, yet a Ranger of the North (hero)
		// may buy Armour and then Heavy Armour; every unit carries a shield now, a new one could buy it; nobody has a Warg or a halberd.
		{"Arnor", []string{"Armour", "Heavy Armour", "Shield", "Horse"}, []string{"Warg", "Fell Warg", "Easterling Halberd", "Goat", "Dunlending War Axe"}},
		// The Shire: Armour yes (a hobbit hero or a Ranger of the North may wear it), Heavy Armour no unit of the company ever has,
		// and hobbits have no shield.
		{"The Shire", []string{"Armour"}, []string{"Heavy Armour", "Shield", "Warg", "Easterling Halberd"}},
		// Rohan: every profile starts in Armour or better, so nobody can buy Armour, but Heavy Armour is the next stage for most;
		// no profile has a spear.
		{"Rohan", []string{"Heavy Armour", "Shield", "Horse"}, []string{"Armour", "Spear", "Warg", "Goat", "Easterling Halberd"}},
		{"Moria", []string{"Heavy Armour"}, []string{"Armour", "Galadhrim Steed", "Easterling Halberd"}},
	}
	for _, tt := range tests {
		t.Run(tt.company, func(t *testing.T) {
			entries := possibleEntries(t, tt.company)
			for _, name := range tt.possible {
				e, ok := entries[name]
				switch {
				case !ok:
					t.Errorf("%s: no entry for %s", tt.company, name)
				case !e.Possible:
					t.Errorf("%s: %s must be possible", tt.company, name)
				}
			}
			for _, name := range tt.impossible {
				if e, ok := entries[name]; ok && e.Possible {
					t.Errorf("%s: %s must be impossible (no unit can ever buy it)", tt.company, name)
				}
			}
		})
	}
}

// TestArnorArmourUpgradesAreListedWithTheirReason is the case of the issue: in a fresh Arnor company no unit can take Armour or Heavy
// Armour now, a Ranger of the North may later.
func TestArnorArmourUpgradesAreListedWithTheirReason(t *testing.T) {
	entries := possibleEntries(t, "Arnor")
	for _, name := range []string{"Armour", "Heavy Armour"} {
		e := entries[name]
		if !e.Possible || e.CanBuy() {
			t.Errorf("%s: possible %v, buyable now %v; want possible and not buyable now", name, e.Possible, e.CanBuy())
		}
		if !strings.HasPrefix(e.Why, "No unit can take it now: ") {
			t.Errorf("%s: why = %q", name, e.Why)
		}
		if !strings.Contains(e.Why, "Ranger of the North") {
			t.Errorf("%s: the reason says who could take it later: %q", name, e.Why)
		}
	}
	if !strings.Contains(entries["Heavy Armour"].Why, "after Armour") {
		t.Errorf("armour goes one stage at a time: %q", entries["Heavy Armour"].Why)
	}
	if entries["Warg"].Possible || entries["Easterling Halberd"].Possible {
		t.Error("no Arnor unit can ever have a Warg or a halberd")
	}
}

// TestWhatSomeUnitCanBuyNowIsPossibleAndNeedsNoReason: an item bought now is possible, and the "no unit can take it now" sentence
// is only for what nobody can buy now.
func TestWhatSomeUnitCanBuyNowIsPossibleAndNeedsNoReason(t *testing.T) {
	for _, company := range []string{"Arnor", "Rohan", "The Shire", "Moria"} {
		for name, e := range possibleEntries(t, company) {
			switch {
			case e.CanBuy() && e.Why != "":
				t.Errorf("%s, %s: can be bought now and has the reason %q", company, name, e.Why)
			case e.CanBuy() && !e.Possible:
				t.Errorf("%s, %s: some unit buys it now, so it is possible", company, name)
			case e.Possible && !e.CanBuy() && e.Why == "":
				t.Errorf("%s, %s: possible, nobody can take it now and no reason is given", company, name)
			case !e.Possible && e.Why != "":
				t.Errorf("%s, %s: impossible items have no reason to show: %q", company, name, e.Why)
			}
		}
	}
}

// TestCreaturesAndWanderersFollowTheSameRule: a creature is possible unless the rules give no officer one, a wanderer unless the
// company could not hire it even with an empty roster.
func TestCreaturesAndWanderersFollowTheSameRule(t *testing.T) {
	sh, _ := armourShop(t, "Arnor")
	for _, c := range sh.creatures() {
		if !c.Possible {
			t.Errorf("creature %s must be possible in a company whose officers may have one", c.Name)
		}
	}
	for _, w := range sh.wanderers() {
		if !w.Possible {
			t.Errorf("wanderer %s: only a rule that rules the company out for good hides it (%s)", w.Name, w.Reason)
		}
	}
	sh.d.Influence = 0
	for _, w := range sh.wanderers() {
		if !w.Possible {
			t.Errorf("wanderer %s: lacking influence is no reason to hide it", w.Name)
		}
	}
}

// TestAReplacedProfileChangesWhatIsPossible: the company's own file decides who can exist in it. Without the promotion that turns a
// Ranger of Arnor into a Ranger of the North, nobody in Arnor starts below Armour, so Armour is no longer a purchase anyone can
// make, while a hero Ranger of Arnor may still upgrade to Heavy Armour.
func TestAReplacedProfileChangesWhatIsPossible(t *testing.T) {
	rs := customRuleset(t, func(doc map[string]any) { companyDoc(t, doc, "Arnor")["promotions"] = []any{} })
	sh, _ := armourShopOf(t, rs, "Arnor")
	got := map[string]ArmouryEntry{}
	for _, e := range sh.entries() {
		got[e.Name] = e
	}
	if got["Armour"].Possible {
		t.Error("with no Ranger of the North every profile of Arnor starts in Armour or better: Armour is impossible")
	}
	heavy := got["Heavy Armour"]
	if !heavy.Possible || !strings.Contains(heavy.Why, "Ranger of Arnor") || strings.Contains(heavy.Why, "after") {
		t.Errorf("a Ranger of Arnor wears Armour and may take Heavy Armour next: %+v %q", heavy.Possible, heavy.Why)
	}
}
