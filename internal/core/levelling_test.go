package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func (f *companyFixture) leveller(t *testing.T, dice ...int) *LevelService {
	t.Helper()
	f.update()
	f.settings.ManualRolls = true
	return NewLevelService(f.campaigns, f.companies, staticRules{shippedRules(t)}, &FixedDice{Rolls: dice})
}

func (f *companyFixture) detail(t *testing.T) CompanyDetail {
	t.Helper()
	return BuildDetail(shippedRules(t), f.settings, f.state)
}

func TestAWarriorLevelUpFollowsTheD6Table(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	// Dorlas has 5 XP: one roll waiting. A 1 changes nothing but uses the roll.
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 7, []int{1}, false)
	if err != nil || !r.Done || f.unit(t, 7).Level != 1 {
		t.Fatalf("a 1: %+v %v", r, err)
	}
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 7, []int{4}, false); !errors.Is(err, ErrRefused) {
		t.Errorf("no roll left: %v", err)
	}
	// a 6 is a hero in the making: the warrior picks a path and becomes a hero
	r, err = s.RollLevelUp(f.ctx, f.user, 5, 8, []int{6}, false)
	if err != nil || r.Done || len(r.Offers) != 1 {
		t.Fatalf("a 6 is a hero in the making: %+v %v", r, err)
	}
	var path string
	for _, c := range r.Offers[0].Choices {
		if strings.HasPrefix(c.Key, "path:") {
			path = c.Key
			break
		}
	}
	if path == "" {
		t.Fatalf("it can pick a path: %+v", r.Offers[0].Choices)
	}
	if state, _ := s.LevelUpState(f.ctx, f.user, 5, 8); state == nil || state.Kind != "warrior" {
		t.Errorf("the roll waits for its choice, stored with the unit: %+v", state)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 8, []string{path}); err != nil {
		t.Fatal(err)
	}
	hero := f.unit(t, 8)
	if !hero.IsHero() || !hero.HasHeroStats() || hero.Path == nil || "path:"+hero.Path.Name != path {
		t.Errorf("it is a hero now with Might, Will and Fate: %+v", hero.Role)
	}
	if state, _ := s.LevelUpState(f.ctx, f.user, 5, 8); state != nil {
		t.Errorf("nothing waits any more: %+v", state)
	}
}

func TestTheAppRollsTheDiceWhenNoneAreGiven(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t, 3)
	f.settings.ManualRolls = false
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 7, nil, false)
	if err != nil || r.D6 != 3 {
		t.Errorf("the dice gave a 3: %+v %v", r, err)
	}
}

func TestAHeroLevelUpOffersTwoResultsAndAppliesTheChoice(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	// Hador (Shieldbearer, 15 XP, three rolls waiting) rolls a 4 and a 8: both give a stat
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 2, []int{4, 8}, false)
	if err != nil || r.Done || len(r.Offers) != 2 {
		t.Fatalf("two offers: %+v %v", r, err)
	}
	if st, _ := s.LevelUpState(f.ctx, f.user, 5, 2); st == nil || len(st.Offers) != 2 {
		t.Error("the roll waits for the choice")
	}
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 2, nil, false); !errors.Is(err, ErrRefused) {
		t.Errorf("finish the first roll: %v", err)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{"stat:made-up:1"}); !errors.Is(err, ErrRefused) {
		t.Errorf("only an offered result can be chosen: %v", err)
	}
	choice := r.Offers[0].Choices[0]
	before := f.unit(t, 2)
	done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{choice.Key})
	if err != nil || !done.Done {
		t.Fatalf("choose: %+v %v", done, err)
	}
	after := f.unit(t, 2)
	if after.Level != before.Level+1 || after.PendingLevelUps() != before.PendingLevelUps()-1 {
		t.Errorf("the roll is used: level %d -> %d", before.Level, after.Level)
	}
	changed := false
	for i, a := range after.Stats {
		changed = changed || a.Value != before.Stats[i].Value
	}
	if !changed {
		t.Errorf("a stat changed: %+v -> %+v", before.Stats, after.Stats)
	}
	if err := (func() error { _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{choice.Key}); return err })(); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing left to choose: %v", err)
	}
}

func TestAnAgainstTheOddsRerollReplacesOneResult(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t, 3, 4) // the new pair: 7
	if _, err := s.RerollLevelUp(f.ctx, f.user, 5, 2, 0); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing to re-roll yet: %v", err)
	}
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 2, []int{4, 8}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RerollLevelUp(f.ctx, f.user, 5, 2, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("the company has no re-roll: %v", err)
	}
	f.state.Rerolls = 1
	r, err := s.RerollLevelUp(f.ctx, f.user, 5, 2, 1)
	if err != nil || len(r.Offers) != 2 || r.Offers[0].Roll != 4 || r.Offers[1].Roll != 7 || len(r.Notes) == 0 {
		t.Fatalf("the second result was rolled again: %+v %v", r, err)
	}
	if f.state.Rerolls != 0 {
		t.Errorf("the re-roll is spent: %d", f.state.Rerolls)
	}
	if _, err := s.RerollLevelUp(f.ctx, f.user, 5, 2, 0); !errors.Is(err, ErrRefused) {
		t.Errorf("no re-roll left: %v", err)
	}
	if got, _ := s.LevelUpState(f.ctx, f.user, 5, 2); got == nil || got.Offers[1].Roll != 7 {
		t.Errorf("the new result is the one stored: %+v", got)
	}
}

func TestAWarriorRerollRollsTheD6Again(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t, 6)
	f.state.Rerolls = 1
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 8, []int{6}, false); err != nil {
		t.Fatal(err)
	}
	r, err := s.RerollLevelUp(f.ctx, f.user, 5, 8, 0)
	if err != nil || r.D6 != 6 || r.Done || f.state.Rerolls != 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestANewSorcererChoosesAPowerAndGetsItsHeroicAction(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 8, []int{6}, false); err != nil {
		t.Fatal(err)
	}
	r, err := s.ChooseLevelUp(f.ctx, f.user, 5, 8, []string{"path:Path of the Sorcerer"})
	if err != nil || r.Done || r.Stage != "startingPower" || len(r.Offers[0].Choices) < 20 {
		t.Fatalf("a Sorcerer picks its first power from the list: %+v %v", r.Stage, err)
	}
	if got, _ := s.LevelUpState(f.ctx, f.user, 5, 8); got == nil || got.Stage != "startingPower" {
		t.Error("the half-made choice is stored")
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 8, []string{"power:Black Dart"}); err != nil {
		t.Fatal(err)
	}
	hero := f.unit(t, 8)
	hasPower, hasAction := false, false
	for _, p := range hero.Path.Powers {
		hasPower = hasPower || (p.Name == "Black Dart" && p.Casting == 6 && p.Range == 6)
	}
	for _, a := range hero.Path.HeroicActions {
		hasAction = hasAction || a.Name == "Heroic Channeling"
	}
	if !hasPower || !hasAction {
		t.Errorf("it has Black Dart (6+, 6\") and Heroic Channeling: %+v %+v", hero.Path.Powers, hero.Path.HeroicActions)
	}
}

func TestOnlyOneHeroOfTheCompanyTakesThePathOfTheSorcerer(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 8, []int{6}, false); err != nil {
		t.Fatal(err)
	}
	r, err := s.ChooseLevelUp(f.ctx, f.user, 5, 8, []string{"path:Path of the Sorcerer"})
	if err != nil {
		t.Fatal(err)
	}
	fury := 0
	for _, c := range r.Offers[0].Choices {
		if strings.HasPrefix(c.Key, "power:Fury (") {
			fury++
		}
		if c.Key == "power:Fury" {
			t.Error("Fury is learned for a species")
		}
	}
	if fury < 5 {
		t.Errorf("one choice per species: %d", fury)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 8, []string{"power:Fury (Mordor Orc)"}); err != nil {
		t.Fatal(err)
	}
	spell := false
	for _, p := range f.unit(t, 8).Path.Powers {
		spell = spell || p.Name == "Fury (Mordor Orc)"
	}
	if !spell {
		t.Error("it knows Fury (Mordor Orc)")
	}
	// a second warrior rolling a 6 is not offered the Sorcerer any more
	f.state.Units = append(f.state.Units, UnitState{ID: 90, Base: "Warrior of Minas Tirith", Role: RoleWarrior, XP: 5})
	r, err = s.RollLevelUp(f.ctx, f.user, 5, 90, []int{6}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range r.Offers[0].Choices {
		if c.Key == "path:Path of the Sorcerer" {
			t.Error("the company already has its Sorcerer")
		}
	}
	if len(r.Notes) == 0 {
		t.Error("it says why")
	}
}

func TestAStaleOfferOfTheSorcererPathIsRefusedOnceTheCompanyHasOne(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.state.Units = append(f.state.Units, UnitState{ID: 90, Base: "Warrior of Minas Tirith", Role: RoleWarrior, XP: 5})
	for _, id := range []int64{8, 90} { // both roll their 6 while the path is still free
		if _, err := s.RollLevelUp(f.ctx, f.user, 5, id, []int{6}, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 8, []string{"path:Path of the Sorcerer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 90, []string{"path:Path of the Sorcerer"}); !errors.Is(err, ErrRefused) {
		t.Errorf("the second Sorcerer is refused: %v", err)
	}
}

func TestARollAgainResultIsChosenTogetherWithTheOtherOne(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 2, []int{5, 4}, false)
	if err != nil || len(r.Offers) != 2 {
		t.Fatalf("a 5 and a 4: %+v %v", r, err)
	}
	var again, other LevelChoice
	for _, o := range r.Offers {
		for _, c := range o.Choices {
			if o.Roll == 5 && again.Key == "" {
				again = c
			}
			if o.Roll == 4 && other.Key == "" {
				other = c
			}
		}
	}
	if !again.RollAgain || other.Key == "" {
		t.Fatalf("the 5 asks for the other roll too: %+v %+v", again, other)
	}
	before := f.unit(t, 2)
	// a second choice that is not on offer changes nothing
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{again.Key, "stat:made-up:1"}); !errors.Is(err, ErrRefused) {
		t.Fatalf("an unavailable second choice: %v", err)
	}
	if after := f.unit(t, 2); after.Level != before.Level || len(f.state.Units[1].Progress.Bonus) != 0 {
		t.Error("nothing was applied")
	}
	if got, _ := s.LevelUpState(f.ctx, f.user, 5, 2); got == nil {
		t.Error("the roll still waits")
	}
	done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{again.Key, other.Key})
	if err != nil || !done.Done {
		t.Fatalf("both at once: %+v %v", done, err)
	}
	after := f.unit(t, 2)
	if after.Level != before.Level+1 {
		t.Errorf("one roll is used for both results: level %d -> %d", before.Level, after.Level)
	}
	changed := 0
	for i, a := range after.Stats {
		if a.Value != before.Stats[i].Value {
			changed++
		}
	}
	if changed < 2 {
		t.Errorf("both results applied: %d stats changed", changed)
	}
}

func TestARollAgainResultCanBeChosenOneAtATime(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 2, []int{5, 4}, false)
	if err != nil {
		t.Fatal(err)
	}
	var again LevelChoice
	for _, o := range r.Offers {
		if o.Roll == 5 && len(o.Choices) > 0 {
			again = o.Choices[0]
		}
	}
	step, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{again.Key})
	if err != nil || step.Done || !step.Again || len(step.Offers) != 1 || step.Offers[0].Roll != 4 {
		t.Fatalf("the other result is offered next: %+v %v", step, err)
	}
	if f.unit(t, 2).Level != 0 {
		t.Error("the roll is not used up yet")
	}
	done, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{step.Offers[0].Choices[0].Key})
	if err != nil || !done.Done || f.unit(t, 2).Level != 1 {
		t.Fatalf("%+v %v", done, err)
	}
}

func TestMinorRulesWithATargetAskForOne(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 2, []int{10, 3}, false) // Hador, a sergeant: a 10 offers a minor special rule
	if err != nil {
		t.Fatal(err)
	}
	var weapons, heroes []string
	for _, c := range r.Offers[0].Choices {
		if strings.HasPrefix(c.Key, "minor:Poisoned Attacks|weapon:") {
			weapons = append(weapons, strings.TrimPrefix(c.Key, "minor:Poisoned Attacks|weapon:"))
		}
		if strings.HasPrefix(c.Key, "minor:Combat Synergy|unit:") {
			heroes = append(heroes, c.Label)
		}
		if c.Key == "minor:Poisoned Attacks" || c.Key == "minor:Combat Synergy" {
			t.Errorf("a rule with a target is never offered without one: %s", c.Key)
		}
	}
	if len(weapons) == 0 || !slices.Contains(weapons, "Hand weapon") {
		t.Fatalf("one choice per weapon: %v", weapons)
	}
	if len(heroes) == 0 || strings.Contains(strings.Join(heroes, ","), "Hador") {
		t.Errorf("Combat Synergy is aimed at another hero, never at itself: %v", heroes)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{"minor:Poisoned Attacks|weapon:Hand weapon"}); err != nil {
		t.Fatal(err)
	}
	hador := f.unit(t, 2)
	rule, poisoned := false, false
	for _, ru := range hador.Path.Rules {
		rule = rule || ru.Name == "Poisoned Attacks (Hand weapon)"
	}
	for _, e := range hador.Equipment {
		poisoned = poisoned || (e.Name == "Hand weapon" && e.Poisoned)
	}
	if !rule || !poisoned {
		t.Errorf("the rule and the poisoned weapon: %v %v", rule, poisoned)
	}
	r, _ = s.RollLevelUp(f.ctx, f.user, 5, 2, []int{10, 3}, false)
	var target string
	for _, c := range r.Offers[0].Choices {
		if c.Key == "minor:Poisoned Attacks|weapon:Hand weapon" {
			t.Error("a poisoned weapon cannot be poisoned again")
		}
		if target == "" && strings.HasPrefix(c.Key, "minor:Combat Synergy|unit:") {
			target = c.Key
		}
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{target}); err != nil {
		t.Fatal(err)
	}
	synergy := func() string {
		for _, ru := range f.unit(t, 2).Path.Rules {
			if strings.HasPrefix(ru.Name, "Combat Synergy - ") {
				return ru.Name
			}
		}
		return ""
	}
	before := synergy()
	if before == "" {
		t.Fatal("the hero has a Combat Synergy")
	}
	var id int64
	fmt.Sscanf(strings.TrimPrefix(target, "minor:Combat Synergy|unit:"), "%d", &id)
	i := slices.IndexFunc(f.state.Units, func(u UnitState) bool { return u.ID == id })
	f.state.Units[i].Name = "Faramir the Younger"
	if after := synergy(); after != "Combat Synergy - Faramir the Younger" || after == before {
		t.Errorf("the rule follows the unit's name: %q -> %q", before, after)
	}
}

func TestAHeroMaySwapAResultForItsCompanysUpgrade(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 2, []int{4, 8}, false)
	if err != nil || r.Upgrade == nil || len(r.Upgrade.Choices) != 1 || r.Upgrade.Choices[0].Key != "upgrade:The Charge of Gondor" {
		t.Fatalf("the company's upgrade is offered besides the two results: %+v %v", r.Upgrade, err)
	}
	if r.Upgrade.Choices[0].Hint == "" {
		t.Error("with its rule text")
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, []string{"upgrade:The Charge of Gondor"}); err != nil {
		t.Fatal(err)
	}
	var rule UnitRule
	for _, ru := range f.unit(t, 2).Path.Rules {
		if ru.Name == "The Charge of Gondor" {
			rule = ru
		}
	}
	if rule.Source != RuleFromCompany || rule.Description == "" {
		t.Errorf("the hero has the upgrade as a company-exclusive rule: %+v", rule)
	}
	r, _ = s.RollLevelUp(f.ctx, f.user, 5, 2, []int{4, 8}, false)
	if r.Upgrade != nil {
		t.Error("a hero cannot take its upgrade twice")
	}
}

func TestRollsNeedTheCampaignSettingsForLevelUps(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.settings = CampaignSettings{ShakeTheTable: true} // typed rolls off
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 2, []int{5, 4}, false); !errors.Is(err, ErrRefused) {
		t.Errorf("typed level-up rolls need the setting: %v", err)
	}
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 2, []int{5, 4}, true)
	if err != nil || len(r.Offers) != 2 || r.Offers[0].Roll != 5 {
		t.Fatalf("a thrown level-up is the roll: %+v %v", r, err)
	}
	f2 := newCompanyFixture(t)
	s2 := f2.leveller(t)
	f2.settings = CampaignSettings{}
	if _, err := s2.RollLevelUp(f2.ctx, f2.user, 5, 2, []int{5, 4}, true); !errors.Is(err, ErrRefused) {
		t.Errorf("not where Shake the table is off: %v", err)
	}
	if _, err := s2.RollLevelUp(f2.ctx, f2.user, 5, 2, nil, false); err != nil {
		t.Errorf("a server roll: %v", err)
	}
}

func TestLevelUpRefusals(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	if _, err := s.RollLevelUp(f.ctx, User{ID: 8}, 5, 2, []int{5, 4}, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 999, []int{5}, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown unit: %v", err)
	}
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 6, []int{5}, false); !errors.Is(err, ErrRefused) {
		t.Errorf("Cirion has 3 XP: %v", err)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 2, nil); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing chosen: %v", err)
	}
	if _, err := s.ChooseLevelUp(f.ctx, User{ID: 8}, 5, 2, []string{"stat:x:1"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner choosing: %v", err)
	}
	if _, err := s.LevelUpState(f.ctx, User{ID: 8}, 5, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner state: %v", err)
	}
	f.state.Units = append(f.state.Units, UnitState{ID: 91, Base: "Wandering Swordsman", Wanderer: "Wandering Swordsman", Role: RoleHero, XP: 9})
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 91, []int{5, 5}, false); !errors.Is(err, ErrRefused) {
		t.Errorf("a wanderer gains no experience: %v", err)
	}
}

func TestARangerOfArnorBecomesAHeroAndIsPromotedAtOnce(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("Arnor", 5, []UnitState{{ID: 90, Base: "Ranger of Arnor", Role: RoleWarrior, Gear: []string{"Spear", "Armour"}, XP: 5}})
	roll, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{6}, false)
	if err != nil || len(roll.Offers) != 1 {
		t.Fatalf("%+v %v", roll, err)
	}
	for _, ch := range roll.Offers[0].Choices {
		if strings.HasPrefix(ch.Key, "promo:") {
			t.Errorf("there is no promotion without the hero's journey: %+v", ch)
		}
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 90, []string{"path:Path of the Ranger"}); err != nil {
		t.Fatal(err)
	}
	u := f.unit(t, 90)
	if u.Type != "Ranger of the North" || !u.IsHero() {
		t.Errorf("a hero and a Ranger of the North: %+v", u)
	}
	for _, a := range u.Stats {
		if a.HeroOnly && a.Boost != 0 {
			t.Errorf("no extra Might, Will or Fate: %+v", a)
		}
	}
	names := ""
	for _, e := range u.Equipment {
		names += e.Name + ","
	}
	if !strings.Contains(names, "Spear") || strings.Contains(names, "Armour") {
		t.Errorf("it keeps its Spear and loses its Armour: %s", names)
	}
}

func TestAnOfficerChoosesItsPathFromTheCompanyPage(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	rs := shippedRules(t)
	profile, _ := rs.BC.Profile("Rohan")
	f.state = CompanyState{Company: CampaignCompany{ID: 1, Faction: "Rohan"}, Units: StartingUnits(profile, profile.DefaultHeroes())}
	var officers []CompanyUnit
	for _, u := range f.detail(t).Roster {
		if u.IsHero() {
			if !u.PathPending {
				t.Fatalf("an officer has no path yet: %+v", u.Path)
			}
			officers = append(officers, u)
		}
	}
	if len(officers) != 3 {
		t.Fatalf("a leader and two sergeants: %d", len(officers))
	}
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, officers[0].ID, nil, false); !errors.Is(err, ErrRefused) {
		t.Errorf("the officer cannot level up without a path: %v", err)
	}
	offer, err := s.PathOptions(f.ctx, f.user, 5, officers[0].ID)
	if err != nil || len(offer.Paths) == 0 || len(offer.Powers) == 0 {
		t.Fatalf("%+v %v", offer, err)
	}
	if err := s.ChoosePath(f.ctx, f.user, 5, officers[0].ID, "Path of the Sorcerer", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("a Sorcerer needs a first power: %v", err)
	}
	if err := s.ChoosePath(f.ctx, f.user, 5, officers[0].ID, "Path of Nothing", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("an unknown path: %v", err)
	}
	if err := s.ChoosePath(f.ctx, User{ID: 8}, 5, officers[0].ID, "Path of the Soldier", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
	power := offer.Powers[0].Key[len("power:"):]
	for i, o := range officers {
		path, pw := "Path of the Soldier", ""
		if i == 0 {
			path, pw = "Path of the Sorcerer", power
		}
		if err := s.ChoosePath(f.ctx, f.user, 5, o.ID, path, pw); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ChoosePath(f.ctx, f.user, 5, officers[0].ID, "Path of the Soldier", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("a path is chosen once: %v", err)
	}
	for _, u := range f.detail(t).Roster {
		if u.PathPending {
			t.Errorf("every officer has a path now: %s", u.Title())
		}
		if u.ID == officers[0].ID && (u.Path.Name != "Path of the Sorcerer" || len(u.Path.Powers) != 1) {
			t.Errorf("the Sorcerer knows its first power: %+v", u.Path)
		}
	}
}

func TestUnitsOnTheirOwnPathLevelUpAsWarriorsWhenTheCampaignSaysSo(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.settings.CustomOnTheirOwnPath = true
	f.companyOf("Mirkwood", 5, []UnitState{{ID: 90, Base: "Mirkwood Giant Spider", Role: RoleWarrior, XP: 10}})
	spider := f.unit(t, 90)
	if spider.OwnPath || spider.IsHero() || spider.Path != nil || spider.PendingLevelUps() != 2 {
		t.Fatalf("it starts as a warrior that earns experience: %+v", spider)
	}
	// a 1 changes nothing and uses the roll
	r, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{1}, false)
	if err != nil || !r.Done || f.unit(t, 90).IsHero() {
		t.Fatalf("a 1: %+v %v", r, err)
	}
	// a 6 makes it a hero on its own path at once: no path to choose, no promotion to forgo for
	r, err = s.RollLevelUp(f.ctx, f.user, 5, 90, []int{6}, false)
	if err != nil || !r.Done || r.D6 != 6 || r.Result != "heroInTheMaking" {
		t.Fatalf("a 6 is a hero on the path without a choice: %+v %v", r, err)
	}
	if st, _ := s.LevelUpState(f.ctx, f.user, 5, 90); st != nil {
		t.Errorf("nothing waits for a choice: %+v", st)
	}
	hero := f.unit(t, 90)
	if hero.OwnPath || !hero.IsHero() || hero.PathPending || hero.Path == nil || hero.Path.Name != "Path of Their Own Making" {
		t.Fatalf("it follows the homebrew path as a hero: %+v", hero.Path)
	}
	values := map[string]int{}
	for _, a := range hero.Stats {
		if a.HeroOnly {
			values[a.Name] = a.Value
		}
	}
	if values["Might"] != 0 || values["Will"] != 0 || values["Fate"] != 1 {
		t.Errorf("0 Might, 0 Will, 1 Fate: %v", values)
	}
	if hero.Level != 2 || hero.PendingLevelUps() != 0 {
		t.Errorf("both rolls are used: level %d, pending %d", hero.Level, hero.PendingLevelUps())
	}
	f.state.Units[0].XP = 15 // one more roll: now on the path's chart
	if roll, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{2, 3}, false); err != nil || roll.Kind != "hero" || len(roll.Offers) == 0 {
		t.Fatalf("as a hero it rolls on the path's chart: %+v %v", roll, err)
	}
	// without the option the spider never levels up
	f.settings.CustomOnTheirOwnPath = false
	f.state = CompanyState{Company: f.state.Company, Units: []UnitState{{ID: 90, Base: "Mirkwood Giant Spider", Role: RoleWarrior, XP: 10}}}
	if u := f.unit(t, 90); !u.OwnPath || u.XP != 0 {
		t.Errorf("on its own path it earns no experience: %+v", u)
	}
	if _, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{6}, false); !errors.Is(err, ErrRefused) {
		t.Errorf("on its own path it never levels up: %v", err)
	}
}

func TestPathOffersCarryTheirDescriptionAndSummary(t *testing.T) {
	rs := shippedRules(t)
	for _, p := range rs.BC.Progression().Paths {
		if p.Summary == "" {
			t.Errorf("%s has no summary for the picker's tooltip", p.Name)
		}
	}
}

// A promotion the company names (Ranger of Ithilien, Helminga) makes the unit that named unit, with the rules and options the
// company's alias adds.
func TestAPromotionKeepsTheCompanysNameForTheUnit(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.leveller(t)
	f.companyOf("Osgiliath & Ithilien", 5, []UnitState{{ID: 90, Base: "Ranger of Gondor", Role: RoleWarrior, XP: 5}})
	roll, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{4}, false)
	if err != nil || len(roll.Offers) == 0 {
		t.Fatalf("%+v %v", roll, err)
	}
	key := ""
	for _, ch := range roll.Offers[0].Choices {
		if strings.HasPrefix(ch.Key, "promo:") {
			key = ch.Key
			if ch.Label != "Ranger of Ithilien" {
				t.Errorf("the choice is named as the company names it: %q", ch.Label)
			}
		}
	}
	if key == "" {
		t.Fatalf("no promotion offered: %+v", roll.Offers[0].Choices)
	}
	if _, err := s.ChooseLevelUp(f.ctx, f.user, 5, 90, []string{key}); err != nil {
		t.Fatal(err)
	}
	u := f.unit(t, 90)
	if u.Type != "Ranger of Ithilien" {
		t.Errorf("the unit is a Ranger of Ithilien: %q", u.Type)
	}
	found := false
	for _, r := range u.Rules {
		found = found || r.Name == "Loyal to the Captain"
	}
	if !found {
		t.Errorf("a Ranger of Ithilien is loyal to the captain: %+v", u.Rules)
	}
}

// A promotion that only adds wargear to the same profile is offered once: a Knight on its armoured horse is not promoted onto one again.
func TestAPromotionThatIsAlreadyDoneIsNotOffered(t *testing.T) {
	promos := func(gear ...string) []string {
		f := newCompanyFixture(t)
		s := f.leveller(t)
		f.companyOf("Fiefdoms", 5, []UnitState{{ID: 90, Base: "Knight of Dol Amroth", Role: RoleWarrior, Gear: gear, XP: 5}})
		roll, err := s.RollLevelUp(f.ctx, f.user, 5, 90, []int{4}, false)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, o := range roll.Offers {
			for _, ch := range o.Choices {
				if strings.HasPrefix(ch.Key, "promo:") {
					out = append(out, ch.Label)
				}
			}
		}
		return out
	}
	if got := promos(); len(got) != 1 {
		t.Errorf("a Knight on foot is offered the armoured horse: %q", got)
	}
	if got := promos("Armoured horse"); len(got) != 0 {
		t.Errorf("a Knight on its armoured horse is offered nothing: %q", got)
	}
}

// A legacy name (Grimhammer for the Grim Hammer Warrior) is the same unit: promotions keep the unit's own name.
func TestALegacyNameDoesNotRenameThePromotedUnit(t *testing.T) {
	rs := shippedRules(t)
	bcCo, _ := rs.BC.Company("Erebor & Dale")
	for _, adv := range rs.BC.Advancements(bcCo) {
		for _, to := range adv.To {
			if got := promotedName(rs, bcCo, to); got != to.Unit {
				t.Errorf("%s is promoted to %q, want %q", adv.FromUnit, got, to.Unit)
			}
		}
	}
	prof, _ := rs.BC.Profile("Erebor & Dale")
	for _, adv := range prof.Advancements {
		for _, to := range adv.To {
			if to.Label != "" {
				t.Errorf("the page names %q by its label %q", to.Unit, to.Label)
			}
		}
	}
}

// Units On Their Own Path (Cave Troll, Crebain...) are never offered as officers, and never get a Hero in the Making result.
func TestOwnPathUnitsAreNeverOfficersOrHeroesInTheMaking(t *testing.T) {
	rs := shippedRules(t)
	seen := 0
	for _, p := range rs.BC.Profiles() {
		for _, name := range p.OwnPathUnits {
			seen++
			if p.CanLead(name) {
				t.Errorf("%s: %s is On Its Own Path and cannot lead", p.Name, name)
			}
		}
		heroes := p.DefaultHeroes()
		list := p.StartList()
		for _, h := range heroes {
			if p.OnOwnPath(list[h.Index].Unit) {
				t.Errorf("%s: the default officers include %s, which is on its own path", p.Name, list[h.Index].Unit)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no own-path units")
	}
}

// When several promotions match a unit (a general row and one for the wargear it carries) the player is offered each of them.
func TestEveryMatchingPromotionIsOffered(t *testing.T) {
	rs := shippedRules(t)
	multi := 0
	for _, p := range rs.BC.Profiles() {
		bcCo, _ := rs.BC.Company(p.Name)
		for _, adv := range rs.BC.Advancements(bcCo) {
			u := buildUnit(rs, bcCo, UnitState{ID: 1, Base: adv.FromUnit, Role: RoleWarrior, Gear: adv.FromWargear}, CampaignSettings{})
			choices := promotionChoices(rs, bcCo, u)
			want := 0
			for _, a := range rs.BC.Advancements(bcCo) {
				if a.Matches(u) {
					for _, to := range a.To {
						if !to.AlreadyIs(u) {
							want++
						}
					}
				}
			}
			if len(choices) != want {
				t.Errorf("%s: %s is offered %d promotions, %d match", p.Name, adv.FromUnit, len(choices), want)
			}
			if len(choices) > 1 {
				multi++
			}
		}
	}
	if multi == 0 {
		t.Error("no unit with several promotions to choose from")
	}
}

// In every company, with the campaign option, each unit On Its Own Path starts as a warrior that earns experience and takes the
// homebrew path with a 6; without the option it earns nothing.
func TestEveryOwnPathUnitStartsAWarriorAndTakesTheHomebrewPathOnASix(t *testing.T) {
	rs := shippedRules(t)
	n := 0
	for _, p := range rs.BC.Profiles() {
		for _, name := range p.OwnPathUnits {
			n++
			t.Run(p.Name+"/"+name, func(t *testing.T) {
				f := newCompanyFixture(t)
				l := f.leveller(t)
				f.settings.CustomOnTheirOwnPath = true
				f.companyOf(p.Name, 5, []UnitState{{ID: 90, Base: name, Role: RoleWarrior, XP: 5}})
				if u := f.unit(t, 90); u.OwnPath || u.IsHero() || u.Path != nil || u.PendingLevelUps() != 1 {
					t.Fatalf("a warrior with a roll waiting: hero %v own %v pending %d", u.IsHero(), u.OwnPath, u.PendingLevelUps())
				}
				if r, err := l.RollLevelUp(f.ctx, f.user, 5, 90, []int{6}, false); err != nil || !r.Done {
					t.Fatalf("%+v %v", r, err)
				}
				u := f.unit(t, 90)
				if !u.IsHero() || u.Path == nil || u.Path.Name != "Path of Their Own Making" || u.PathPending {
					t.Fatalf("a hero on its own path: %+v", u.Path)
				}
				f.settings.CustomOnTheirOwnPath = false
				f.state.Units[0] = UnitState{ID: 90, Base: name, Role: RoleWarrior, XP: 5}
				if u := f.unit(t, 90); !u.OwnPath || u.XP != 0 {
					t.Errorf("without the option it earns nothing: %+v", u.XP)
				}
			})
		}
	}
	if n == 0 {
		t.Fatal("no own-path units")
	}
}
