package core

import (
	"slices"
	"strings"
	"testing"
)

func TestOptionItemsSplitsGrammar(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"War horn, shield, and throwing spears", []string{"War horn", "shield", "throwing spears"}},
		{"Shield and Spear", []string{"Shield", "Spear"}},
		{"Shield, Spear", []string{"Shield", "Spear"}},
		{"Exchange lance for banner", []string{"lance", "banner"}},
	} {
		if got := OptionItems(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("OptionItems(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A unit under a company's own label offers exactly what the vanilla profile it labels offers.
func TestAliasUnitsOfferWhatTheirProfileOffers(t *testing.T) {
	rs := shippedRules(t)
	n := 0
	for name, c := range rs.BC.companies {
		p, _ := rs.BC.Profile(name)
		sh := shop{rs: rs, bcC: c}
		for _, a := range p.Aliases {
			if _, ok := rs.Vanilla.Unit(a.Unit); !ok {
				continue
			}
			n++
			want := offeredWargear(sh, CompanyUnit{Type: a.Unit, Role: RoleWarrior})
			got := offeredWargear(sh, CompanyUnit{Type: a.Label, Role: RoleWarrior})
			if !slices.Equal(got, want) {
				t.Errorf("%s: %s offers %q, its profile %s offers %q", name, a.Label, got, a.Unit, want)
			}
		}
	}
	if n == 0 {
		t.Fatal("no aliases found")
	}
}

// Wargear in the charts, the starting companies and the promotions is spelled as the armoury spells it, for every company.
func TestChartWargearIsSpelledAsTheArmouryDoes(t *testing.T) {
	rs := shippedRules(t)
	spell := map[string]string{}
	for _, o := range rs.BC.armoury {
		for _, n := range append([]string{o.Name}, o.AlsoCalled...) {
			if _, ok := spell[lower(n)]; !ok {
				spell[lower(n)] = n
			}
		}
	}
	check := func(where string, w []string) {
		for _, n := range w {
			if c, ok := spell[lower(n)]; ok && c != n {
				t.Errorf("%s: wargear %q, the armoury spells it %q", where, n, c)
			}
		}
	}
	for name, c := range rs.BC.companies {
		for _, chart := range [][]ChartRow{c.Reinforcements, c.SpecialChart} {
			for _, r := range chart {
				check(name+" chart "+r.Result.Unit, r.Result.Wargear)
			}
		}
		p, _ := rs.BC.Profile(name)
		for _, s := range p.Start {
			check(name+" start "+s.Unit, s.Wargear)
		}
		for _, a := range p.Advancements {
			for _, to := range a.To {
				check(name+" promotion "+to.Unit, to.Wargear)
			}
		}
	}
}

func lower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// A rule written with a parameter resolves to the parameterised rule's text and to its glossary term.
func TestParameterisedRulesResolve(t *testing.T) {
	rs := shippedRules(t)
	for _, name := range []string{"Hatred (Spirit)", "Hatred (Man)", "Poisoned Attacks", "Poisoned Attacks (bow)", "Poisoned Attacks (blowpipe)", "Terror (2)"} {
		if d, ok := rs.Keyword("", name); !ok || d == "" {
			t.Errorf("rule %q has no text", name)
		}
		if _, ok := rs.Glossary.Find(name); !ok {
			t.Errorf("rule %q has no glossary term", name)
		}
	}
	if _, ok := rs.Keyword("", "No Such Rule (x)"); ok {
		t.Error("an unknown rule resolved")
	}
}

// Beasts (Battle Companies armoury rule) take no wargear and only equipment that exclusively gives a special rule.
func TestBeastsCarryOnlyRuleGivingEquipment(t *testing.T) {
	rs := shippedRules(t)
	c, ok := rs.BC.Company("Cirith Ungol")
	if !ok {
		t.Fatal("no Cirith Ungol")
	}
	sh := shop{rs: rs, bcC: c}
	for _, role := range []UnitRole{RoleWarrior, RoleHero} {
		spider := CompanyUnit{Type: "Mirkwood Giant Spider", Role: role, Kind: KindBeast}
		for _, o := range rs.BC.Armoury(c) {
			why, _ := sh.whyNot(spider, o)
			allowed := o.Size != "" && len(o.Grants) == 1 && !o.WeaponModifier
			if !allowed && why == "" {
				t.Errorf("%s spider may buy %s", role, o.Name)
			}
		}
	}
	pelt, ok := rs.BC.Offer(c, "Trophy Pelt")
	if !ok {
		t.Fatal("no Trophy Pelt")
	}
	if why, _ := sh.whyNot(CompanyUnit{Type: "Mirkwood Giant Spider", Role: RoleWarrior, Kind: KindBeast}, pelt); why == "Beasts carry no wargear and only equipment that gives a special rule" {
		t.Errorf("a spider may carry a Trophy Pelt, got %q", why)
	}
	spear, _ := rs.BC.Offer(c, "Spear")
	if why, _ := sh.whyNot(CompanyUnit{Type: "Mordor Orc Warrior", Role: RoleWarrior, Kind: KindInfantry}, spear); why != "" && !strings.HasPrefix(why, "Costs") {
		t.Errorf("an Orc may buy a Spear, got %q", why)
	}
}

// A hero may buy what a promoted unit of its company carries (Battle Companies: "including those that could be promoted").
func TestHeroesMayBuyWhatPromotionsCarry(t *testing.T) {
	rs := shippedRules(t)
	c, ok := rs.BC.Company("Minas Morgul")
	if !ok {
		t.Fatal("no Minas Morgul")
	}
	sh := shop{rs: rs, bcC: c}
	hero := CompanyUnit{Type: "Mordor Orc Warrior", Role: RoleHero, Kind: KindInfantry}
	warrior := CompanyUnit{Type: "Mordor Orc Warrior", Role: RoleWarrior, Kind: KindInfantry}
	for _, name := range []string{"Armoured horse", "Lance"} {
		offer, ok := rs.BC.Offer(c, name)
		if !ok {
			t.Fatalf("no %s in the armoury", name)
		}
		if !sh.legalWargear(hero, offer) {
			t.Errorf("a hero cannot buy %s, which the Black Numenorean Knight (a promotion) carries", name)
		}
		if sh.legalWargear(warrior, offer) {
			t.Errorf("an Orc warrior may buy %s, which is no option of its profile", name)
		}
	}
}

// A Warrior of Rohan with a shield and throwing spears is the 3-point option, not the 28-point one that also gives a war horn.
func TestShieldAndThrowingSpearsIsNotTheWarHornOption(t *testing.T) {
	rs := shippedRules(t)
	bcCo, _ := rs.BC.Company("Rohan")
	u := buildUnit(rs, bcCo, UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleWarrior, Gear: []string{"Throwing Spear", "Shield"}}, CampaignSettings{})
	if u.Points != 6+3 {
		t.Errorf("a Warrior of Rohan with throwing spears and a shield is worth 9, got %d (%+v)", u.Points, u.Equipment)
	}
}

// A vanilla typo in a profile's wargear ("Claws and teeth (hand weapons") reads as the annotated item, like its siblings.
func TestVanillaWargearTypoIsSpelledAsTheAnnotatedItem(t *testing.T) {
	rs := shippedRules(t)
	for _, unit := range []string{"Wild Warg", "Werewolf", "Fell Warg"} {
		v, ok := rs.Vanilla.Unit(unit)
		if !ok {
			t.Fatalf("no %s", unit)
		}
		if !slices.Equal(v.Wargear, []string{"Claws and teeth (hand weapon)"}) {
			t.Errorf("%s carries %q", unit, v.Wargear)
		}
	}
}

// A chart result under a company's own name that selects an option (Helminga: Warrior of Rohan with Helmingas) is that unit.
func TestChartResultUnderAnOptionAliasIsTheAlias(t *testing.T) {
	rs := shippedRules(t)
	bcCo, _ := rs.BC.Company("Helm's Deep")
	r, ok := Roll(bcCo.Reinforcements, 4)
	if !ok || r.Unit != "Helminga" {
		t.Fatalf("roll 4 is a Helminga: %+v", r)
	}
	u := buildUnit(rs, bcCo, UnitState{ID: 1, Base: r.Unit, Role: RoleWarrior}, CampaignSettings{})
	if u.Points != 6+1 {
		t.Errorf("a Helminga costs 6 + 1 for Helmingas, got %d", u.Points)
	}
}

// Elven-made weaponry is for Elf models (BC armoury): a Man cannot buy an Elven spear, an Elf can.
func TestElvenWeaponryIsForElves(t *testing.T) {
	rs := shippedRules(t)
	bcCo, _ := rs.BC.Company("Fornost")
	sh := shop{rs: rs, bcC: bcCo}
	offer, ok := rs.BC.Offer(bcCo, "Elven spear")
	if !ok {
		t.Fatal("no Elven spear in the armoury")
	}
	if why, _ := sh.whyNot(CompanyUnit{Type: "Warrior of Minas Tirith", Role: RoleHero}, offer); why != "Elven-made weaponry is for Elves" {
		t.Errorf("a Man hero cannot take an Elven spear: %q", why)
	}
	if why, _ := sh.whyNot(CompanyUnit{Type: "Rivendell Warrior", Role: RoleHero}, offer); why == "Elven-made weaponry is for Elves" {
		t.Errorf("an Elf hero can: %q", why)
	}
}

// A promotion that asks for "Uruk bow" is open to a Scout carrying the Uruk-Hai Bow (the same item under another name).
func TestPromotionWargearMatchesByItemIdentity(t *testing.T) {
	rs := shippedRules(t)
	c, ok := rs.BC.Company("Muster of Isengard")
	if !ok {
		t.Fatal("no Muster of Isengard")
	}
	bow, ok := rs.BC.Offer(c, "Uruk-Hai Bow")
	if !ok {
		t.Fatal("no Uruk-Hai Bow")
	}
	scout := CompanyUnit{Type: "Uruk-hai Scout", Equipment: []Equipment{{Name: bow.Name, Spellings: append([]string{bow.Name}, bow.AlsoCalled...)}}}
	if a, ok := rs.BC.Annotation("Uruk-Hai Bow"); ok {
		scout.Equipment[0].Spellings = append([]string{a.Name}, a.Aliases...)
	}
	var crossbow bool
	for _, adv := range rs.BC.Advancements(c) {
		if adv.Matches(scout) {
			for _, to := range adv.To {
				crossbow = crossbow || slices.Contains(to.Wargear, "Crossbow")
			}
		}
	}
	if !crossbow {
		t.Error("a Scout with an Uruk-Hai Bow cannot be promoted to a Warrior with a crossbow")
	}
}

// Sharkey's Rogues exchange Hatred (Rohan) for Hatred (Hobbit): the models carry the new rule, not the old.
func TestRuleExchangeReplacesTheRuleOnTheModels(t *testing.T) {
	rs := shippedRules(t)
	st := CompanyState{
		Company: CampaignCompany{ID: 1, Name: "The Rogues", Faction: "Sharkey's Rogues"},
		Units: []UnitState{
			{ID: 1, Name: "Boss", Base: "Hill Tribesman", Role: RoleLeader},
			{ID: 2, Base: "Hill Tribesman", Role: RoleWarrior},
			{ID: 3, Base: "Ruffian", Role: RoleWarrior},
		},
	}
	d := BuildDetail(rs, CampaignSettings{}, st)
	for _, id := range []int64{1, 2} {
		u := unitByID(t, d, id)
		if _, ok := ruleNamed(u, "Hatred (Rohan)"); ok {
			t.Errorf("%s still has Hatred (Rohan)", u.Title())
		}
		r, ok := ruleNamed(u, "Hatred (Hobbit)")
		if !ok || r.Description == "" || r.Source != RuleFromCompany {
			t.Errorf("%s: Hatred (Hobbit) = %+v, %v", u.Title(), r, ok)
		}
	}
	if _, ok := ruleNamed(unitByID(t, d, 3), "Hatred (Hobbit)"); ok {
		t.Error("a Ruffian never had Hatred (Rohan) and gets no Hatred (Hobbit)")
	}
	// another company keeps its Hatred
	st = CompanyState{Company: CampaignCompany{ID: 2, Name: "Dunland", Faction: "Hill Tribes of Dunland"}, Units: []UnitState{{ID: 1, Base: "Hill Tribesman", Role: RoleWarrior}}}
	if _, ok := ruleNamed(unitByID(t, BuildDetail(rs, CampaignSettings{}, st), 1), "Hatred (Rohan)"); !ok {
		t.Error("the Hill Tribes of Dunland keep Hatred (Rohan)")
	}
}

// The price rule holds for every company: a unit's points are the vanilla base cost plus its options, nothing else.
func TestPriceRuleHoldsForEveryCompany(t *testing.T) {
	rs := shippedRules(t)
	report := AuditRules(rs, AuditInput{})
	for _, f := range report.Flags() {
		if f.Kind == FlagPriceMismatch {
			t.Errorf("%s / %s: %s", f.Company, f.Subject, f.Detail)
		}
	}
}

func TestOptionsCoverPricesByOptions(t *testing.T) {
	horse := Equipment{Name: "Armoured horse"}
	lance := Equipment{Name: "Lance"}
	spear := Equipment{Name: "Spear"}
	shield := Equipment{Name: "Shield"}
	bow := Equipment{Name: "Bow"}
	opts := []ChoiceOption{{Name: "Armoured horse and lance", Points: 9}, {Name: "Armoured horse and banner", Points: 33}, {Name: "Shield and Spear", Points: 2}, {Name: "Bow", Points: 2}}
	for _, tc := range []struct {
		name  string
		items []Equipment
		want  int
		rest  int
	}{
		{"none", nil, 0, 0},
		{"exact option", []Equipment{horse, lance}, 9, 0},
		{"part of an option costs the option", []Equipment{horse}, 9, 0},
		{"spear alone is part of shield and spear", []Equipment{spear}, 2, 0},
		{"every option at once", []Equipment{horse, lance, bow}, 11, 0},
		{"no option explains it", []Equipment{{Name: "Mattock"}}, 0, 1},
		{"options plus unexplained", []Equipment{shield, spear, {Name: "Mattock"}}, 2, 1},
	} {
		got, rest := OptionsCover(tc.items, opts)
		if got != tc.want || len(rest) != tc.rest {
			t.Errorf("%s: got %d points and %d unexplained, want %d and %d", tc.name, got, len(rest), tc.want, tc.rest)
		}
	}
}

func TestKnightOnArmouredHorseIsTwentyPoints(t *testing.T) {
	rs := shippedRules(t)
	bcCo, _ := rs.BC.Company("Fiefdoms")
	u := buildUnit(rs, bcCo, UnitState{Base: "Knight of Dol Amroth", Role: RoleWarrior, Gear: []string{"Armoured horse"}}, CampaignSettings{})
	if u.Points != 20 {
		t.Errorf("Knight of Dol Amroth on armoured horse: %d points, want 11 + 9", u.Points)
	}
}

// Limits round up and a model that joins counts in numerator and denominator; the Helm's Deep Elf limit follows the company.
func TestKeywordLimitFollowsTheCompanySize(t *testing.T) {
	rs := shippedRules(t)
	bcCo, _ := rs.BC.Company("Helm's Deep")
	profile, _ := rs.BC.Profile("Helm's Deep")
	if len(profile.KeywordLimits) == 0 {
		t.Skip("no keyword limit")
	}
	kw := profile.KeywordLimits[0]
	var elf string
	for _, v := range rs.Vanilla.units {
		if elf == "" && hasKeyword(v, kw.Keyword) && v.Points < 20 && slices.Contains(v.UnitTypes, "Warrior") {
			elf = v.Name
		}
	}
	if elf == "" {
		t.Skip("no elf warrior found")
	}
	roster := func(elves, men int) []CompanyUnit {
		var out []CompanyUnit
		for range elves {
			out = append(out, CompanyUnit{Type: elf, Role: RoleWarrior})
		}
		for range men {
			out = append(out, CompanyUnit{Type: "Warrior of Rohan", Role: RoleWarrior})
		}
		return out
	}
	if why, _ := companyBlock(rs, bcCo, roster(2, 4), elf); why != "" {
		t.Errorf("a 3rd Elf in 7 models is allowed (33%% of 7 rounds up to 3): %s", why)
	}
	if why, kind := companyBlock(rs, bcCo, roster(3, 3), elf); why == "" || kind != blockLimit {
		t.Errorf("a 4th Elf in 7 models is not allowed")
	}
	if why, _ := companyBlock(rs, bcCo, roster(3, 8), elf); why != "" {
		t.Errorf("a 4th Elf in 12 models is allowed: %s", why)
	}
}

// A recruit that carries a bow counts in both the bows and the company: 6 warriors with 2 bows may take a 7th with a bow (3 of 7).
func TestRecruitWithABowFollowsTheRoundedUpLimit(t *testing.T) {
	rs := shippedRules(t)
	state := func(bows int) CompanyState {
		st := CompanyState{Company: CampaignCompany{ID: 1, Name: "Rohan", Faction: "Rohan"}}
		for i := range 6 {
			u := UnitState{ID: int64(i + 1), Base: "Warrior of Rohan", Role: RoleWarrior, Gear: []string{"Shield"}}
			if i < bows {
				u.Gear = []string{"Bow"}
			}
			st.Units = append(st.Units, u)
		}
		return st
	}
	res := Reinforcement{Type: "unit", Count: 1, Unit: "Warrior of Rohan", Wargear: []string{"Bow"}}
	if why, _ := newRecruitEnv(rs, CampaignSettings{}, state(2)).block(res); why != "" {
		t.Errorf("the 7th model may carry the 3rd bow: %s", why)
	}
	if why, kind := newRecruitEnv(rs, CampaignSettings{}, state(3)).block(res); why == "" || kind != blockLimit {
		t.Errorf("the 7th model may not carry a 4th bow (limit 3 of 7): %q", why)
	}
	plain := Reinforcement{Type: "unit", Count: 1, Unit: "Warrior of Rohan", Wargear: []string{"Shield"}}
	if why, _ := newRecruitEnv(rs, CampaignSettings{}, state(3)).block(plain); why != "" {
		t.Errorf("a recruit without a bow is not stopped by the bow limit: %s", why)
	}
}

// A row that names one piece of a bundled option gives the whole bundle and costs the option's points (user decision 2026-10-21).
func TestPartialBundleRowsGetTheWholeOption(t *testing.T) {
	rs := shippedRules(t)
	hills, _ := rs.BC.Company("Iron Hills")
	u := buildUnit(rs, hills, UnitState{ID: 1, Base: "Iron Hills Warrior", Role: RoleWarrior, Gear: []string{"Spear"}}, CampaignSettings{})
	var names []string
	for _, e := range u.Equipment {
		names = append(names, e.Name)
	}
	if !slices.Contains(names, "Spear") || !slices.Contains(names, "Shield") || u.Points != 12 {
		t.Errorf("Iron Hills Warrior (Spear): %v, %d points (Shield and Spear, 10 + 2)", names, u.Points)
	}
	// a unit that has the pieces already, or whose wargear is an option of its own, is not changed
	both := buildUnit(rs, hills, UnitState{ID: 2, Base: "Iron Hills Warrior", Role: RoleWarrior, Gear: []string{"Shield", "Spear"}}, CampaignSettings{})
	if len(both.Equipment) != len(u.Equipment) || both.Points != 12 {
		t.Errorf("both pieces: %d items, %d points", len(both.Equipment), both.Points)
	}
	mattock := buildUnit(rs, hills, UnitState{ID: 3, Base: "Iron Hills Warrior", Role: RoleWarrior, Gear: []string{"Mattock"}}, CampaignSettings{})
	if mattock.Points != 11 || len(mattock.Equipment) != len(buildUnit(rs, hills, UnitState{ID: 4, Base: "Iron Hills Warrior", Role: RoleWarrior}, CampaignSettings{}).Equipment)+1 {
		t.Errorf("a single-item option stays as it is: %d points", mattock.Points)
	}
	// a piece dropped and bought back costs its own armoury price, not the bundle again
	rebought := buildUnit(rs, hills, UnitState{ID: 5, Base: "Iron Hills Warrior", Role: RoleHero, Bonus: HeroBonus(nil), Gear: []string{"Spear"}, Dropped: []string{"Shield"}, Bought: []string{"Shield"}}, CampaignSettings{})
	n := 0
	for _, e := range rebought.Equipment {
		if e.Name == "Shield" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the shield is carried once: %d", n)
	}
	// the audit lists every such row
	found := map[string]bool{}
	flags := AuditRules(rs, AuditInput{}).Flags()
	for _, f := range flags {
		if f.Kind == FlagPartialBundle {
			found[f.Company+" / "+f.Subject] = true
		}
	}
	for _, want := range []string{"Iron Hills / Iron Hills Warrior", "Fiefdoms / Knight of Dol Amroth"} {
		if !found[want] {
			t.Errorf("the audit flags %s: %v", want, found)
		}
	}
	// and after the completion nothing is priced by rating: no price-mismatch anywhere
	for _, f := range flags {
		if f.Kind == FlagPriceMismatch {
			t.Errorf("%s / %s: %s", f.Company, f.Subject, f.Detail)
		}
	}
}
