package core

import (
	"slices"
	"sort"
	"testing"
)

// chartUnits are the profiles a company's charts and special units name.
func chartUnits(c BCCompany) []string {
	set := map[string]bool{}
	for _, ch := range [][]ChartRow{c.Reinforcements, c.SpecialChart} {
		for _, r := range ch {
			if r.Result.Unit != "" {
				set[r.Result.Unit] = true
			}
			for _, o := range r.Result.Options {
				set[o.Unit] = true
			}
		}
	}
	for _, s := range c.SpecialUnits {
		set[s.Unit] = true
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func offeredWargear(sh shop, u CompanyUnit) []string {
	var out []string
	for _, o := range sh.rs.BC.Armoury(sh.bcC) {
		if o.Size == "" && sh.legalWargear(u, o) {
			out = append(out, o.Name)
		}
	}
	return out
}

// The Battle Companies armoury rule for every company: a warrior may only buy wargear that is one of its profile's options
// (a profile without options buys none), a hero wargear that is an option or starting wargear of a profile of its charts or promotions.
func TestOfferedWargearIsAlwaysAnOptionOfTheProfile(t *testing.T) {
	rs := shippedRules(t)
	if len(rs.BC.companies) != 37 {
		t.Fatalf("expected 37 companies, got %d", len(rs.BC.companies))
	}
	for name, c := range rs.BC.companies {
		sh := shop{rs: rs, bcC: c}
		heroGear := map[string]bool{}
		units := chartUnits(c)
		var promotedWargear []string
		if profile, ok := rs.BC.Profile(name); ok { // and what the units of the starting company carry (a hero may buy any wargear of a member)
			for _, st := range profile.Start {
				units = append(units, st.Unit)
				promotedWargear = append(promotedWargear, st.Wargear...)
			}
			for _, pr := range profile.Promotions {
				units = append(units, pr.Becomes)
			}
		}
		for _, adv := range rs.BC.Advancements(c) { // heroes may buy what units they could be promoted to carry
			for _, to := range adv.To {
				units = append(units, to.Unit)
				promotedWargear = append(promotedWargear, to.Wargear...)
			}
		}
		for _, o := range rs.BC.Armoury(c) {
			if slices.ContainsFunc(promotedWargear, func(p string) bool { return GearMatchesOption(o, p) }) {
				heroGear[o.Name] = true
			}
		}
		for _, ty := range units {
			v, ok := sh.profileOf(ty) // a company's own label (Noldorin Exile) is the profile it labels, with its option selected
			if !ok {
				continue
			}
			var parts []string
			for _, o := range v.Options {
				parts = append(parts, o.Name)
			}
			parts = append(parts, v.Wargear...)
			for _, o := range rs.BC.Armoury(c) {
				if slices.ContainsFunc(parts, func(p string) bool { return GearMatchesOption(o, p) }) {
					heroGear[o.Name] = true
				}
			}
			var allowed []string
			for _, o := range v.Options {
				allowed = append(allowed, o.Name)
			}
			for _, got := range offeredWargear(sh, CompanyUnit{Type: ty, Role: RoleWarrior}) {
				offer, _ := rs.BC.Offer(c, got)
				if !slices.ContainsFunc(allowed, func(a string) bool { return GearMatchesOption(offer, a) }) {
					t.Errorf("%s: %s is offered %s, which is none of its options %q", name, ty, got, allowed)
				}
			}
			if len(v.Options) == 0 {
				if got := offeredWargear(sh, CompanyUnit{Type: ty, Role: RoleWarrior}); len(got) != 0 {
					t.Errorf("%s: %s has no options but is offered %q", name, ty, got)
				}
			}
		}
		for _, role := range []UnitRole{RoleLeader, RoleSergeant, RoleHero} {
			for _, got := range offeredWargear(sh, CompanyUnit{Type: "", Role: role}) {
				if !heroGear[got] {
					t.Errorf("%s: a %s is offered %s, which no profile of the company's charts has", name, role, got)
				}
			}
		}
	}
}

func TestMinasTirithArmouryOfWarriorsAndHeroes(t *testing.T) {
	rs := shippedRules(t)
	c, _ := rs.BC.Company("Minas Tirith")
	sh := shop{rs: rs, bcC: c}
	for _, tc := range []struct {
		unit string
		want []string
	}{
		{"Warrior of Minas Tirith", []string{"Bow", "Spear", "Shield"}},
		{"Citadel Guard", []string{"Longbow", "Spear"}},
		{"Knight of Minas Tirith", []string{"Lance", "Shield"}},
		{"Guard of the Fountain Court", nil},
	} {
		got := offeredWargear(sh, CompanyUnit{Type: tc.unit, Role: RoleWarrior})
		a, b := slices.Clone(got), slices.Clone(tc.want)
		sort.Strings(a)
		sort.Strings(b)
		if !slices.Equal(a, b) {
			t.Errorf("%s may buy %q, want %q", tc.unit, got, tc.want)
		}
	}
	hero := offeredWargear(sh, CompanyUnit{Role: RoleLeader})
	for _, no := range []string{"Pike", "Elf Bow", "Crossbow", "Two-handed weapon", "Elven spear"} {
		if slices.Contains(hero, no) {
			t.Errorf("a hero of Minas Tirith must not be offered %s", no)
		}
	}
	for _, yes := range []string{"Horse", "Longbow", "Spear", "Shield", "Bow", "Lance"} {
		if !slices.Contains(hero, yes) {
			t.Errorf("a hero of Minas Tirith may buy %s (a profile of its charts has it)", yes)
		}
	}
}

// A warrior whose profile has no options buys equipment (small and large items) but no wargear; a Hero it becomes uses the hero pool
// (register Top-7). Every such unit of every company is checked.
func TestWarriorsWithoutOptionsBuyEquipmentButNoWargear(t *testing.T) {
	rs := shippedRules(t)
	checked := 0
	for _, p := range rs.BC.Profiles() {
		names := map[string]bool{}
		for _, s := range p.Start {
			names[s.Unit] = true
		}
		for _, r := range p.Reinforcements {
			names[r.Result.Unit] = true
			for _, o := range r.Result.Options {
				names[o.Unit] = true
			}
		}
		for name := range names {
			if name == "" {
				continue
			}
			st := CompanyState{Company: CampaignCompany{ID: 1, Name: "T", Faction: p.Name}, Influence: 200, Gold: 200,
				Units: []UnitState{{ID: 1, Base: name, Role: RoleWarrior}}}
			sh := newShop(rs, CampaignSettings{}, st)
			u, ok := sh.unit(1)
			if !ok || u.IsHero() || u.Kind == KindBeast {
				continue
			}
			if v, ok := sh.profileOf(u.Type); !ok || len(v.Options) > 0 {
				continue
			}
			checked++
			items, err := sh.items(1)
			if err != nil {
				t.Fatal(err)
			}
			equipment := 0
			for _, it := range items {
				switch {
				case it.ArmourStage > 0 || (it.Size == "" && it.Only == ""):
					t.Errorf("%s / %s: no options, yet %s (wargear) is offered", p.Name, name, it.Name)
				case it.Size != "":
					equipment++
				}
			}
			if equipment == 0 {
				t.Errorf("%s / %s: no options, but no equipment is offered either", p.Name, name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no unit without options was found: the test checks nothing")
	}
	t.Logf("%d units without options checked", checked)
}

func TestAHeroMadeOfAUnitWithoutOptionsUsesTheHeroPool(t *testing.T) {
	rs := shippedRules(t)
	st := CompanyState{Company: CampaignCompany{ID: 1, Name: "T", Faction: "Minas Morgul"}, Influence: 200,
		Units: []UnitState{{ID: 1, Base: "Black Numenorean Warrior", Role: RoleHero, Bonus: HeroBonus(nil)}}}
	sh := newShop(rs, CampaignSettings{}, st)
	items, err := sh.items(1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(items, func(it ArmouryItem) bool { return it.Size == "" && it.ArmourStage == 0 && it.Buyable() }) {
		t.Error("a Hero may buy wargear from the pool of its company's units")
	}
}

// R12: the Ranger of the North is a start unit of the Shire and has a Spear option, so any Shire Hero may buy a Spear.
func TestShireHeroesMayBuyASpear(t *testing.T) {
	rs := shippedRules(t)
	p, _ := rs.BC.Profile("The Shire")
	if !slices.ContainsFunc(p.Start, func(s StartUnit) bool { return s.Unit == "Ranger of the North" }) {
		t.Fatal("the Ranger of the North is a start unit of the Shire")
	}
	st := CompanyState{Company: CampaignCompany{ID: 1, Name: "T", Faction: "The Shire"}, Influence: 100,
		Units: []UnitState{{ID: 1, Base: "Hobbit Militia", Role: RoleHero, Bonus: HeroBonus(nil), Dropped: []string{"Spear"}}}}
	items, err := newShop(rs, CampaignSettings{}, st).items(1)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(items, func(it ArmouryItem) bool { return it.Name == "Spear" }); i < 0 || !items[i].Buyable() {
		t.Errorf("a Shire hero may buy a Spear: %+v", items)
	}
}
