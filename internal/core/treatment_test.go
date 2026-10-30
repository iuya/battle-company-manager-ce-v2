package core

import (
	"errors"
	"strings"
	"testing"
)

func (f *companyFixture) treater(t *testing.T, dice ...int) *TreatmentService {
	t.Helper()
	f.update()
	f.settings.ManualRolls = true
	return NewTreatmentService(f.campaigns, f.companies, staticRules{shippedRules(t)}, &FixedDice{Rolls: dice})
}

func TestTreatingAWarriorsInjuryCostsInfluence(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.treater(t)
	before := f.detail(t)
	if before.InjuredUnits == 0 || before.Injuries.WarriorCost != 1 {
		t.Fatal("the fixture company has an injured warrior and the file says what treating costs")
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 7, Treatment{Method: "warrior"}); err != nil {
		t.Fatal(err)
	}
	after := f.detail(t)
	if after.InjuredUnits != before.InjuredUnits-1 || after.Influence != before.Influence-1 || after.Points <= before.Points {
		t.Errorf("healing removes the injury, costs influence and returns the points: %+v -> %+v", before.InjuredUnits, after.InjuredUnits)
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 7, Treatment{Method: "warrior"}); !errors.Is(err, ErrRefused) {
		t.Errorf("a healthy unit is not treated: %v", err)
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 7, Treatment{Method: "attempt"}); !errors.Is(err, ErrRefused) {
		t.Errorf("a warrior has no roll: %v", err)
	}
	if _, err := s.TreatInjury(f.ctx, User{ID: 8}, 5, 1, 4, Treatment{Method: "warrior"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
	f.state.Units[6].Injured, f.state.Influence = true, 0
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 7, Treatment{Method: "warrior"}); !errors.Is(err, ErrRefused) || !f.state.Units[6].Injured {
		t.Errorf("not enough influence: %v", err)
	}
}

func TestTreatingAHeroByAttemptOrByTheHealers(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.treater(t)
	f.state.Influence = 5
	// a roll that misses by a little waits: extra influence is decided after seeing it
	res, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound", Roll: 4})
	if err != nil || res.Healed || !res.Pending || res.Boost != 1 || res.Needed != 5 {
		t.Fatalf("a 4 misses 5+ by one: %+v %v", res, err)
	}
	if f.state.Influence != 4 {
		t.Errorf("the attempt cost its influence: %d", f.state.Influence)
	}
	if st, _ := s.TreatmentState(f.ctx, f.user, 5, 1, 4); st == nil || !st.Pending || st.Roll != 4 {
		t.Error("the roll waits for the decision")
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "healers", Injury: "legWound"}); !errors.Is(err, ErrRefused) {
		t.Errorf("decide first: %v", err)
	}
	// accepting the failure: the influence is lost and the injury stays
	if res, err = s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Resolve: true, Extra: 0}); err != nil || res.Healed || res.Pending {
		t.Fatalf("accept: %+v %v", res, err)
	}
	if f.state.Influence != 4 || len(f.unit(t, 4).Injuries) != 2 {
		t.Errorf("nothing else was spent: %d", f.state.Influence)
	}
	if st, _ := s.TreatmentState(f.ctx, f.user, 5, 1, 4); st != nil {
		t.Error("nothing waits any more")
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Resolve: true}); !errors.Is(err, ErrRefused) {
		t.Errorf("nothing to resolve: %v", err)
	}
	// try again and spend the extra influence
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound", Roll: 4}); err != nil {
		t.Fatal(err)
	}
	res, err = s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Resolve: true, Extra: 1})
	if err != nil || !res.Healed || res.Total != 5 {
		t.Fatalf("a 4 and 1 extra influence is a 5: %+v %v", res, err)
	}
	if n := len(f.unit(t, 4).Injuries); n != 1 {
		t.Errorf("the arm wound is gone: %d injuries left", n)
	}
	if f.state.Influence != 2 { // 4 - 1 (second attempt) - 1 (extra)
		t.Errorf("influence %d", f.state.Influence)
	}
	// a natural success needs no decision
	if res, _ = s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "justAScratch", Roll: 6}); !res.Healed || res.Pending || f.unit(t, 4).Injured {
		t.Errorf("a successful attempt lets the hero play: %+v", res)
	}
	// the healers cannot remove a missed game; they can remove an injury, and the hero misses the next one
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "healers", Injury: "justAScratch"}); !errors.Is(err, ErrRefused) {
		t.Errorf("the healers do not remove a missed game: %v", err)
	}
	if res, err = s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "healers", Injury: "legWound"}); err != nil || res.Rolled || !res.Healed {
		t.Fatalf("healers: %+v %v", res, err)
	}
	if u := f.unit(t, 4); !u.Injured || len(u.Injuries) != 0 {
		t.Errorf("the hero is healed but misses the next game: %+v", u.Injuries)
	}
	// the game missed at the healers is the price of the healing: a field treatment cannot cancel it
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "justAScratch", Roll: 6}); !errors.Is(err, ErrRefused) || !f.unit(t, 4).Injured {
		t.Errorf("the healers' absence stays: %v", err)
	}
	f.state.Influence = 0
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound", Roll: 6}); !errors.Is(err, ErrRefused) {
		t.Errorf("not enough influence: %v", err)
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "voodoo"}); !errors.Is(err, ErrRefused) {
		t.Errorf("unknown treatment: %v", err)
	}
}

func TestTheAppRollsAHeroTreatmentWhenNoRollIsGiven(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.treater(t, 6)
	f.settings.ManualRolls = false
	res, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "legWound"})
	if err != nil || !res.Healed || res.Roll != 6 {
		t.Errorf("the dice gave a 6: %+v %v", res, err)
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound", Roll: 5}); !errors.Is(err, ErrRefused) {
		t.Errorf("a typed roll needs the setting: %v", err)
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound", Roll: 5, Thrown: true}); !errors.Is(err, ErrRefused) {
		t.Errorf("a thrown roll needs the setting: %v", err)
	}
}

func TestAnInjuryCountGoesDownOneAtATimeAndTheEffectsFollow(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.treater(t)
	f.state.Influence = 5
	moved := func() int {
		for _, a := range f.unit(t, 5).Stats {
			if a.Name == "Movement" {
				return a.Boost
			}
		}
		return 99
	}
	f.state.Units[4].Injuries = map[string]int{"legWound": 2}
	if moved() != -2 {
		t.Fatalf("two leg wounds take 2 off Movement: %d", moved())
	}
	if _, err := s.TreatInjury(f.ctx, f.user, 5, 1, 5, Treatment{Method: "healers", Injury: "legWound"}); err != nil {
		t.Fatal(err)
	}
	if moved() != -1 || f.state.Units[4].Injuries["legWound"] != 1 {
		t.Errorf("one wound is treated, one stays: %d %v", moved(), f.state.Units[4].Injuries)
	}
	if !f.state.Units[4].SentToHealers {
		t.Error("the hero is at the healers")
	}
}

// treatDice makes the field treatment roll 2d6 against 8, raised up to 12 (hero[attempt].roll of the rules file).
func treatDice(t *testing.T, doc map[string]any) {
	for _, m := range jsonAt(t, doc, "campaign", "injuries", "treatment", "hero").([]any) {
		if h := m.(map[string]any); h["key"] == "attempt" {
			roll := h["roll"].(map[string]any)
			roll["dice"], roll["target"] = "2d6", 8.0
			roll["modifier"].(map[string]any)["ceiling"] = 12.0
		}
	}
}

// The treatment roll is rolled and checked on the dice of the rules (campaign.injuries.treatment.hero[].roll.dice), not on a D6.
func TestTheTreatmentRollUsesTheDiceOfTheRules(t *testing.T) {
	rs := customRuleset(t, func(doc map[string]any) { treatDice(t, doc) })
	if m := rs.BC.Injuries().Attempt; m.TreatDice != NdM(2, 6) || m.Target != 8 || m.Ceiling != 12 {
		t.Fatalf("attempt = %+v", m)
	}
	if m := shippedRules(t).BC.Injuries().Attempt; m.TreatDice != NdM(1, 6) || m.Target != 5 {
		t.Fatalf("the shipped attempt is a d6 against 5: %+v", m)
	}
	treater := func(dice ...int) (*TreatmentService, *companyFixture) {
		f := newCompanyFixture(t)
		f.update()
		f.settings.ManualRolls = true
		f.state.Influence = 5
		return NewTreatmentService(f.campaigns, f.companies, staticRules{rs}, &FixedDice{Rolls: dice}), f
	}
	// the server rolls two dice: 4 and 5 make 9, which reaches 8
	s, f := treater(4, 5)
	res, err := s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound"})
	if err != nil || !res.Healed || res.Roll != 9 || len(res.Dice) != 2 || res.Dice[0] != 4 || res.Dice[1] != 5 || res.Needed != 8 {
		t.Errorf("2d6 rolled by the server: %+v %v", res, err)
	}
	// a typed total within 2..12 is taken as it is; 7 misses by one and waits for the decision on extra influence
	s, f = treater(6, 6)
	res, err = s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound", Roll: 7})
	if err != nil || res.Roll != 7 || res.Dice != nil || !res.Pending || res.Boost != 1 {
		t.Errorf("a typed 7: %+v %v", res, err)
	}
	// a typed 13 is no total of 2d6: the server rolls (6 and 6 make 12)
	s, f = treater(6, 6)
	res, err = s.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound", Roll: 13})
	if err != nil || res.Roll != 12 || len(res.Dice) != 2 {
		t.Errorf("a typed 13 is refused as a roll and rolled again: %+v %v", res, err)
	}
	// on the shipped d6 a typed 7 is not a roll either
	f = newCompanyFixture(t)
	d6 := f.treater(t, 3)
	f.state.Influence = 5
	res, err = d6.TreatInjury(f.ctx, f.user, 5, 1, 4, Treatment{Method: "attempt", Injury: "armWound", Roll: 7})
	if err != nil || res.Roll != 3 || len(res.Dice) != 1 {
		t.Errorf("a typed 7 on a d6: %+v %v", res, err)
	}
}

func TestCustomRulesetRefusesBadTreatmentDice(t *testing.T) {
	attempt := func(doc map[string]any) map[string]any {
		for _, m := range jsonAt(t, doc, "campaign", "injuries", "treatment", "hero").([]any) {
			if h := m.(map[string]any); h["key"] == "attempt" {
				return h["roll"].(map[string]any)
			}
		}
		t.Fatal("no attempt")
		return nil
	}
	tests := []struct {
		name   string
		mutate func(doc map[string]any)
		want   string
	}{
		{"dice missing", func(doc map[string]any) { delete(attempt(doc), "dice") }, "campaign.injuries.treatment.hero[0].roll.dice: missing"},
		{"not a spec", func(doc map[string]any) { attempt(doc)["dice"] = "2d6+1" }, `campaign.injuries.treatment.hero[0].roll.dice: "2d6+1" is not a dice spec`},
		{"target above the highest roll", func(doc map[string]any) { attempt(doc)["target"] = 7.0 }, "roll.target: 7 is outside 1..6, the highest roll of d6"},
		{"target 0", func(doc map[string]any) { attempt(doc)["target"] = 0.0 }, "roll.target: 0 is outside 1..6"},
		{"ceiling below the target", func(doc map[string]any) { attempt(doc)["modifier"].(map[string]any)["ceiling"] = 4.0 }, "roll.modifier.ceiling: 4 is below the target 5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := customRulesetErr(t, tt.mutate)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}
