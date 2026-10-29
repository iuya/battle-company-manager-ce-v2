package core

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

// itemRoster is a small roster with consumables: Aldor (Leader, Healing Herbs), Brand (Sergeant, Wondrous Cram), Cene (Sergeant,
// nothing), Edo (Warrior, Healing Herbs), Fen (Warrior, Wondrous Cram).
func itemRoster() []CompanyUnit {
	herbs, cram := Equipment{Name: "Healing Herbs"}, Equipment{Name: "Wondrous Cram"}
	return []CompanyUnit{
		{ID: 1, Name: "Aldor", Role: RoleLeader, Equipment: []Equipment{herbs}},
		{ID: 2, Name: "Brand", Role: RoleSergeant, Equipment: []Equipment{cram}},
		{ID: 3, Name: "Cene", Role: RoleSergeant},
		{ID: 5, Name: "Edo", Role: RoleWarrior, Equipment: []Equipment{herbs}},
		{ID: 6, Name: "Fen", Role: RoleWarrior, Equipment: []Equipment{cram}},
	}
}

// itemSheet builds a sheet: everyone took part and survived, with the given outcomes and elections applied.
func itemSheet(outcomes map[int64]ModelOutcome, uses map[int64]string) AftermathInput {
	in := DefaultSheet(itemRoster())
	for i, m := range in.Models {
		if o, ok := outcomes[m.UnitID]; ok {
			in.Models[i].Outcome = o
		}
		in.Models[i].UsesItem = uses[m.UnitID]
	}
	return in
}

func TestParseAftermathItems(t *testing.T) {
	rules := realAftermathRules(t)
	herbs, ok := rules.ItemEffect("Healing Herbs")
	if !ok || herbs.Key != "healingHerbs" || herbs.Kind != kindRollModifier || herbs.Modifier != 1 || herbs.Stackable ||
		herbs.ClampMin != 2 || herbs.ClampMax != 12 || herbs.Carrier != "hero" || !slices.Contains(herbs.Requires, "notCasualty") {
		t.Fatalf("Healing Herbs: %+v", herbs)
	}
	cram, ok := rules.ItemEffect("wondrous cram")
	if !ok || cram.Kind != kindRollReplacement || !cram.NoRoll || cram.Result != "fullRecovery" || cram.Carrier != "anyModel" || !slices.Contains(cram.Requires, "casualty") {
		t.Fatalf("Wondrous Cram: %+v", cram)
	}
	if _, ok := rules.ItemEffect("Spear"); ok {
		t.Error("a spear activates nothing")
	}
	if rules.itemName("healingHerbs") != "Healing Herbs" {
		t.Errorf("item name: %q", rules.itemName("healingHerbs"))
	}
	if _, err := customRulesetErr(t, func(doc map[string]any) {
		for _, g := range doc["gear"].([]any) {
			if g := g.(map[string]any); g["name"] == "Healing Herbs" {
				g["aftermathEffect"] = "nope"
			}
		}
	}); !errors.Is(err, ErrInvalidRuleset) {
		t.Errorf("an item naming an unknown effect must be refused: %v", err)
	}
}

func TestRollModifierApply(t *testing.T) {
	rules := realAftermathRules(t)
	herbs := RollModifierOf(rules, itemSheet(nil, map[int64]string{1: "healingHerbs"}))
	none := RollModifierOf(rules, itemSheet(nil, nil))
	tests := []struct {
		name        string
		mod         RollModifier
		total       int
		want, shown int
	}{
		{"2 becomes 3", herbs, 2, 3, 1},
		{"6 becomes 7", herbs, 6, 7, 1},
		{"11 becomes 12", herbs, 11, 12, 1},
		{"12 stays 12, a real 12", herbs, 12, 12, 1},
		{"no election, no change", none, 9, 9, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, shown := tt.mod.Apply(tt.total); got != tt.want || shown != tt.shown {
				t.Errorf("Apply(%d) = %d (+%d), want %d (+%d)", tt.total, got, shown, tt.want, tt.shown)
			}
		})
	}
	// two Herbs never make +2
	two := RollModifierOf(rules, itemSheet(nil, map[int64]string{1: "healingHerbs", 5: "healingHerbs"}))
	if two.Amount != 1 {
		t.Errorf("two Healing Herbs are not cumulative, got +%d", two.Amount)
	}
}

func TestSheetValidateItems(t *testing.T) {
	rules := realAftermathRules(t)
	loaned := []LoanedItem{{UnitID: 3, Item: "Healing Herbs"}, {UnitID: 5, Item: "Healing Herbs"}}
	tests := []struct {
		name    string
		outcome map[int64]ModelOutcome
		uses    map[int64]string
		loaned  []LoanedItem
		took    map[int64]bool // units that did NOT take part
		wantErr string         // "" for valid
	}{
		{name: "no election"},
		{name: "a Hero carrier uses Healing Herbs", uses: map[int64]string{1: "healingHerbs"}},
		{name: "a fled Hero counts as not a casualty", outcome: map[int64]ModelOutcome{1: OutcomeFled}, uses: map[int64]string{1: "healingHerbs"}},
		{name: "Herbs on a Warrior are refused", uses: map[int64]string{5: "healingHerbs"}, wantErr: "only a Hero can use Healing Herbs"},
		{name: "Herbs on a casualty are refused", outcome: map[int64]ModelOutcome{1: OutcomeCasualty}, uses: map[int64]string{1: "healingHerbs"}, wantErr: "is a casualty and cannot use Healing Herbs"},
		{name: "Herbs on a Hero that did not take part", took: map[int64]bool{1: true}, uses: map[int64]string{1: "healingHerbs"}, wantErr: "did not take part"},
		{name: "Herbs not carried", uses: map[int64]string{3: "healingHerbs"}, wantErr: "does not carry Healing Herbs"},
		{name: "kit Herbs on a Hero are electable", uses: map[int64]string{3: "healingHerbs"}, loaned: loaned},
		{name: "kit Herbs on a Warrior are refused", uses: map[int64]string{5: "healingHerbs"}, loaned: loaned, wantErr: "only a Hero can use"},
		{name: "two Herbs do not stack", uses: map[int64]string{1: "healingHerbs", 3: "healingHerbs"}, loaned: loaned, wantErr: "Healing Herbs do not stack"},
		{name: "Cram on a casualty", outcome: map[int64]ModelOutcome{6: OutcomeCasualty}, uses: map[int64]string{6: "wondrousCram"}},
		{name: "Cram on a casualty Hero", outcome: map[int64]ModelOutcome{2: OutcomeCasualty}, uses: map[int64]string{2: "wondrousCram"}},
		{name: "Cram on a survivor is refused", uses: map[int64]string{6: "wondrousCram"}, wantErr: "only as a casualty"},
		{name: "Cram on a fled model is refused", outcome: map[int64]ModelOutcome{6: OutcomeFled}, uses: map[int64]string{6: "wondrousCram"}, wantErr: "only as a casualty"},
		{name: "Cram not carried", outcome: map[int64]ModelOutcome{5: OutcomeCasualty}, uses: map[int64]string{5: "wondrousCram"}, wantErr: "does not carry Wondrous Cram"},
		{name: "an unknown item", uses: map[int64]string{1: "tinFoil"}, wantErr: "is not an item that can be used"},
		{name: "an effect that is not an item", uses: map[int64]string{1: "cargo"}, wantErr: "is not an item that can be used"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := itemSheet(tt.outcome, tt.uses)
			for i, m := range in.Models {
				if tt.took[m.UnitID] {
					in.Models[i].Gains = nil
				}
			}
			out, err := in.ValidateWith(itemRoster(), tt.loaned, rules, CompanyResult{})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				for _, m := range out.Models {
					if m.UsesItem != tt.uses[m.UnitID] {
						t.Errorf("unit %d lost its election: %q", m.UnitID, m.UsesItem)
					}
				}
				return
			}
			if !errors.Is(err, ErrInvalidAftermath) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestItemChoices(t *testing.T) {
	rules := realAftermathRules(t)
	loaned := []LoanedItem{{UnitID: 3, Item: "Healing Herbs"}, {UnitID: 5, Item: "Healing Herbs"}}
	in := itemSheet(map[int64]ModelOutcome{2: OutcomeCasualty, 6: OutcomeCasualty}, nil)
	got := in.ItemChoices(itemRoster(), loaned, rules)
	want := map[int64][]string{1: {"healingHerbs"}, 2: {"wondrousCram"}, 3: {"healingHerbs"}, 6: {"wondrousCram"}}
	if len(got) != len(want) {
		t.Fatalf("choices: %v", got)
	}
	for id, keys := range want {
		if !slices.Equal(got[id], keys) {
			t.Errorf("unit %d may elect %v, want %v", id, got[id], keys)
		}
	}
	if len(got[5]) != 0 {
		t.Errorf("a Warrior cannot elect kit Herbs: %v", got[5])
	}
}

func TestPreviewItems(t *testing.T) {
	rules := realAftermathRules(t)
	in := itemSheet(map[int64]ModelOutcome{3: OutcomeCasualty, 6: OutcomeCasualty}, map[int64]string{1: "healingHerbs", 6: "wondrousCram"})
	p := Preview(AftermathContext{Rules: rules, Outcome: "win", Roster: itemRoster()}, in)
	if p.Rolls != 1 {
		t.Errorf("the Cram casualty does not roll: %d rolls", p.Rolls)
	}
	if len(p.Items) != 2 || p.Items[0].Text != "Healing Herbs: +1 to every injury roll" || p.Items[1].Text != "Wondrous Cram on Fen: no roll" || p.Items[1].UnitID != 6 {
		t.Errorf("items: %+v", p.Items)
	}
}

// --- the service ---

// withItems gives Hador (a Sergeant) Healing Herbs and Cirion (a Warrior) and Baranor (a Sergeant) Wondrous Cram.
func (f *amFixture) withItems() {
	f.state.Units[1].Bought = append(f.state.Units[1].Bought, "Healing Herbs")
	f.state.Units[5].Bought = append(f.state.Units[5].Bought, "Wondrous Cram")
	f.state.Units[2].Bought = append(f.state.Units[2].Bought, "Wondrous Cram")
}

func (f *amFixture) bought(id int64) []string {
	for _, u := range f.state.Units {
		if u.ID == id {
			return u.Bought
		}
	}
	return nil
}

// elect is a sheet where the given units are casualties and uses[id] is each unit's item.
func elect(casualty []int64, uses map[int64]string) AftermathInput {
	in := casualties(casualty...)
	for i := range in.Models {
		in.Models[i].Gains = map[string]int{participatedKey: 1}
	}
	for id, item := range uses {
		if i := slices.IndexFunc(in.Models, func(m ModelSheet) bool { return m.UnitID == id }); i >= 0 {
			in.Models[i].UsesItem = item
		} else {
			in.Models = append(in.Models, ModelSheet{UnitID: id, Outcome: OutcomeSurvived, Gains: map[string]int{participatedKey: 1}, UsesItem: item})
		}
	}
	return in
}

func casOf(a Aftermath, id int64) Casualty {
	for _, c := range a.Casualties {
		if c.UnitID == id {
			return c
		}
	}
	return Casualty{}
}

func TestHealingHerbsAddOneToEveryRoll(t *testing.T) {
	f := newAmFixture(t)
	f.withItems()
	f.dice.Rolls = []int{4}
	a := f.start(t, elect([]int64{6, 3}, map[int64]string{2: "healingHerbs"}))
	if a.Status != AftermathInjuries || len(a.Preview.Items) != 1 {
		t.Fatalf("start: %s %+v", a.Status, a.Preview.Items)
	}
	if !slices.Contains(f.bought(2), "Healing Herbs") {
		t.Fatal("the Herbs are consumed at Finish, not before")
	}
	a = f.roll(t, 6, 5, 5) // a Warrior: 10 + 1 = 11
	if c := casOf(a, 6); c.Total != 11 || c.Modifier != 1 || !slices.Equal(c.Dice, []int{5, 5}) || c.ResultKey != "fullRecovery" {
		t.Fatalf("warrior: %+v", c)
	}
	// a Hero: 2 + 3 = 5, +1 = 6 is T'is Just a Scratch (without Herbs it would be Broken Honour)
	a = f.roll(t, 3, 2, 3)
	c := casOf(a, 3)
	if c.Total != 6 || c.Modifier != 1 || c.ResultKey != "justAScratch" || c.Decision == nil {
		t.Fatalf("hero: %+v", c)
	}
	// the Scratch roll again takes the +1 too (and a second Scratch is a miss, not another roll)
	if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "scratch", Answer: "reroll"}); err != nil {
		t.Fatal(err)
	}
	a = f.roll(t, 3, 1, 3) // 4 + 1 = 5: Broken Honour
	if c := casOf(a, 3); c.Total != 5 || c.Modifier != 1 || c.ResultKey != "brokenHonour" {
		t.Fatalf("scratch re-roll: %+v", c)
	}
	if a.Status != AftermathLevelUps {
		t.Fatalf("status %s", a.Status)
	}
	if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.bought(2), "Healing Herbs") || !slices.Contains(f.bought(2), "Spear") {
		t.Errorf("Finish removes the Herbs and nothing else: %v", f.bought(2))
	}
	if !slices.Contains(f.bought(6), "Wondrous Cram") {
		t.Error("an unused Wondrous Cram stays")
	}
	if !slices.ContainsFunc(f.rec.Lines, func(l AftermathLine) bool { return l.Text == "Hador (Sergeant) used Healing Herbs" }) {
		t.Errorf("lines: %+v", f.rec.Lines)
	}
}

func TestHealingHerbsClampAtTwelve(t *testing.T) {
	f := newAmFixture(t)
	f.withItems()
	f.dice.Rolls = []int{4}
	f.start(t, elect([]int64{6}, map[int64]string{2: "healingHerbs"}))
	a := f.roll(t, 6, 6, 6)
	if c := a.Casualties[0]; c.Total != 12 || c.ResultKey != "lessonLearned" {
		t.Fatalf("a modified total never exceeds 12: %+v", c)
	}
	f = newAmFixture(t)
	f.withItems()
	f.start(t, elect([]int64{6}, map[int64]string{2: "healingHerbs"}))
	if a = f.roll(t, 6, 5, 6); a.Casualties[0].Total != 12 || a.Casualties[0].ResultKey != "lessonLearned" {
		t.Fatalf("11 + 1 is a real 12: %+v", a.Casualties[0])
	}
}

func TestHealingHerbsAndAgainstTheOddsReroll(t *testing.T) {
	f := newAmFixture(t)
	f.withItems()
	in := elect([]int64{6}, map[int64]string{2: "healingHerbs"})
	in.AgainstTheOdds = []string{"reroll"}
	f.start(t, in)
	f.roll(t, 6, 1, 1) // 2 + 1 = 3: dead
	a, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 6, []int{2, 3}, false)
	if err != nil {
		t.Fatal(err)
	}
	if c := a.Casualties[0]; c.Total != 6 || c.Modifier != 1 || !c.Rerolled {
		t.Fatalf("the re-roll takes the +1: %+v", c)
	}
}

func TestWondrousCramSkipsTheRoll(t *testing.T) {
	f := newAmFixture(t)
	f.withItems()
	a := f.start(t, elect([]int64{6}, map[int64]string{6: "wondrousCram"}))
	c := a.Casualties[0]
	if !c.Rolled || c.Item != "wondrousCram" || c.ResultKey != "fullRecovery" || c.Kind != CasualtyRecovered || len(c.Dice) != 0 || a.Preview.Rolls != 0 {
		t.Fatalf("casualty: %+v (rolls %d)", c, a.Preview.Rolls)
	}
	if a.Status != AftermathLevelUps || !c.Final {
		t.Fatalf("nothing is left to roll, the step closes: %s final=%v", a.Status, c.Final)
	}
	if f.unit(t, 6).Injured || len(f.state.Cemetery) != 0 {
		t.Error("Full Recovery: not injured, not dead")
	}
	if !slices.Contains(f.bought(6), "Wondrous Cram") {
		t.Fatal("the Cram is consumed at Finish")
	}
	if _, err := f.svc.RollCasualty(f.ctx, f.owner(), 5, 1, 1, 6, []int{6, 6}, false); err == nil {
		t.Error("there is nothing to roll")
	}
	if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.bought(6), "Wondrous Cram") {
		t.Error("Finish removes the Cram")
	}
	if !slices.ContainsFunc(f.rec.Lines, func(l AftermathLine) bool { return strings.HasPrefix(l.Text, "Cirion used Wondrous Cram") }) {
		t.Errorf("lines: %+v", f.rec.Lines)
	}
}

func TestWondrousCramOnAHeroStillHealsAnInjury(t *testing.T) {
	f := newAmFixture(t)
	f.withItems()
	f.state.Units[2].Injuries = map[string]int{"armWound": 1} // Baranor
	a := f.start(t, elect([]int64{3}, map[int64]string{3: "wondrousCram"}))
	c := a.Casualties[0]
	if c.Decision == nil || c.Decision.Kind != "heal" || a.Status != AftermathInjuries {
		t.Fatalf("the heal question is open: %+v", c)
	}
	if _, err := f.svc.RerollCasualty(f.ctx, f.owner(), 5, 1, 1, 3, []int{1, 1}, false); err == nil {
		t.Error("a Cram casualty cannot be re-rolled")
	}
	if _, err := f.svc.Decide(f.ctx, f.owner(), 5, 1, 1, AftermathAnswer{UnitID: 3, Kind: "heal", Answer: "armWound"}); err != nil {
		t.Fatal(err)
	}
	if u := f.unit(t, 3); len(u.Injuries) != 0 {
		t.Errorf("injuries: %+v", u.Injuries)
	}
}

func TestItemElectionRefusals(t *testing.T) {
	tests := []struct {
		name string
		in   AftermathInput
		want string
	}{
		{"Herbs not carried", elect(nil, map[int64]string{6: "healingHerbs"}), "does not carry"},
		{"Cram on a survivor", elect(nil, map[int64]string{6: "wondrousCram"}), "only as a casualty"},
		{"Herbs on a casualty", elect([]int64{2}, map[int64]string{2: "healingHerbs"}), "is a casualty"},
		{"an item that is not carried", elect([]int64{9}, map[int64]string{9: "wondrousCram"}), "does not carry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAmFixture(t)
			f.withItems()
			_, err := f.svc.StartAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision, tt.in)
			if !errors.Is(err, ErrInvalidAftermath) {
				t.Fatalf("err = %v", err)
			}
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestOnlyOneHealingHerbsCounts(t *testing.T) {
	f := newAmFixture(t)
	f.withItems()
	f.state.Units[2].Bought = append(f.state.Units[2].Bought, "Healing Herbs") // Baranor too
	_, err := f.svc.StartAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision, elect(nil, map[int64]string{2: "healingHerbs", 3: "healingHerbs"}))
	if !errors.Is(err, ErrInvalidAftermath) || !strings.Contains(err.Error(), "do not stack") {
		t.Fatalf("err = %v", err)
	}
	f.start(t, elect([]int64{6}, map[int64]string{3: "healingHerbs"}))
	if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err == nil {
		// not yet rolled: Finish refuses, nothing consumed
		t.Fatal("finish before the injuries")
	}
	f.roll(t, 6, 3, 3)
	if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.bought(2), "Healing Herbs") || slices.Contains(f.bought(3), "Healing Herbs") {
		t.Errorf("exactly the elected Herbs are used: Hador %v, Baranor %v", f.bought(2), f.bought(3))
	}
}

func TestItemElectionCanChangeBeforeTheFirstRoll(t *testing.T) {
	f := newAmFixture(t)
	f.withItems()
	f.start(t, elect([]int64{6}, map[int64]string{6: "wondrousCram"}))
	// the Cram casualty is already settled, so there is nothing rolled by the player: reopening is allowed
	a, err := f.svc.ReopenSheet(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision)
	if err == nil {
		t.Logf("reopened from %s", a.Status)
	}
	f = newAmFixture(t)
	f.withItems()
	f.start(t, elect([]int64{6, 3}, map[int64]string{2: "healingHerbs"}))
	if _, err := f.svc.ReopenSheet(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
		t.Fatalf("before the first roll: %v", err)
	}
	a, err = f.svc.StartAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision, elect([]int64{6, 3}, nil))
	if err != nil || len(a.Preview.Items) != 0 {
		t.Fatalf("restart without the item: %+v %v", a.Preview.Items, err)
	}
	a = f.roll(t, 6, 3, 3)
	if c := casOf(a, 6); c.Modifier != 0 || c.Total != 6 {
		t.Errorf("no election, no modifier: %+v", c)
	}
	if _, err := f.svc.ReopenSheet(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err == nil {
		t.Error("after the first roll the sheet stays locked")
	}
}

func TestWaivedAftermathConsumesNothing(t *testing.T) {
	f := newAmFixture(t)
	f.withItems()
	f.start(t, elect([]int64{6}, map[int64]string{2: "healingHerbs"}))
	if _, err := f.svc.WaiveAftermath(f.ctx, User{ID: amOrganiser}, 5, 1, 1, "table cancelled"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.bought(2), "Healing Herbs") {
		t.Error("waiving consumes nothing")
	}
}

// --- Against the Odds in the aftermath ---

func (f *amFixture) withATO(t *testing.T, rec *ATORecord) {
	t.Helper()
	repo := NewMockatoRepository(gomock.NewController(t))
	repo.EXPECT().Get(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) (ATORecord, error) { return *rec, nil }).AnyTimes()
	f.svc.WithATO(repo)
}

func TestAftermathSheetIsPrefilledFromTheMatchSelection(t *testing.T) {
	f := newAmFixture(t)
	rec := &ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"influence", "experience", "toolKit", "wanderer"}, Kits: []string{"raider"}, Wanderer: "x"}}
	f.withATO(t, rec)
	a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !a.FromMatch || !slices.Equal(a.Input.AgainstTheOdds, []string{"influence", "experience"}) {
		t.Fatalf("prefill: from=%v %v", a.FromMatch, a.Input.AgainstTheOdds)
	}
	var odds bool
	for _, l := range a.Preview.Influence {
		odds = odds || l.Key == "odds:influence"
	}
	if !odds {
		t.Errorf("the preview counts the bonus: %+v", a.Preview.Influence)
	}
	// the player changes it: the saved sheet wins and the badge goes
	in := a.Input
	in.AgainstTheOdds = nil
	a, err = f.svc.SaveAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision, in)
	if err != nil || a.FromMatch || len(a.Input.AgainstTheOdds) != 0 {
		t.Fatalf("saved: %+v %v", a.Input.AgainstTheOdds, err)
	}
	// the other company took them, not this one: nothing is prefilled
	rec.CompanyID = 2
	f2 := newAmFixture(t)
	f2.withATO(t, rec)
	if a, _ = f2.svc.Aftermath(f2.ctx, f2.owner(), 5, 1, 1); a.FromMatch || len(a.Input.AgainstTheOdds) != 0 {
		t.Errorf("another company's selection: %+v", a.Input.AgainstTheOdds)
	}
}

func TestLoanedHealerKitItemsAreElectableAndSpent(t *testing.T) {
	f := newAmFixture(t)
	rec := &ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
		Items: []KitAssignment{{Kit: "healer", Item: "Healing Herbs", UnitID: 2}, {Kit: "healer", Item: "Wondrous Cram", UnitID: 6}, {Kit: "healer", Item: "Wondrous Cram", Index: 1, UnitID: 8}}}}
	f.withATO(t, rec)
	a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1) // Cirion survived on the default sheet: no Cram for him yet
	if err != nil || !slices.Equal(a.ItemChoices[2], []string{"healingHerbs"}) || len(a.ItemChoices[6]) != 0 {
		t.Fatalf("default sheet: %v %v", a.ItemChoices, err)
	}
	if a, err = f.svc.SaveAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision, elect([]int64{6}, nil)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.ItemChoices[2], []string{"healingHerbs"}) || len(a.ItemChoices[6]) != 1 || a.ItemChoices[6][0] != "wondrousCram" {
		t.Fatalf("choices: %v", a.ItemChoices)
	}
	if a.FromMatch {
		t.Error("the Tool Kit has no aftermath bonus to prefill")
	}
	in := elect([]int64{6}, map[int64]string{2: "healingHerbs", 6: "wondrousCram"})
	// Cirion's Cram is loaned: a casualty skips its roll, the Herbs add nothing to a roll nobody makes
	a = f.start(t, in)
	if c := casOf(a, 6); c.Item != "wondrousCram" || !c.Rolled || c.ResultKey != "fullRecovery" {
		t.Fatalf("loaned Cram: %+v", c)
	}
	if len(f.state.Units[1].Bought) != 1 { // Hador still has only his Spear: nothing was ever on the roster
		t.Fatalf("roster untouched: %v", f.state.Units[1].Bought)
	}
	if _, err := f.svc.FinishAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision); err != nil {
		t.Fatal(err)
	}
	if len(f.state.Units[1].Bought) != 1 || len(f.state.Units[5].Bought) != 1 {
		t.Errorf("nothing to remove from a roster that never had it: %v %v", f.state.Units[1].Bought, f.state.Units[5].Bought)
	}
	if !slices.ContainsFunc(f.rec.Lines, func(l AftermathLine) bool { return strings.HasPrefix(l.Text, "Hador (Sergeant) used Healing Herbs") }) ||
		!slices.ContainsFunc(f.rec.Lines, func(l AftermathLine) bool { return strings.HasPrefix(l.Text, "Cirion used Wondrous Cram") }) {
		t.Errorf("lines: %+v", f.rec.Lines)
	}
	// kit Herbs on a Warrior are refused even when somebody assigned them
	f = newAmFixture(t)
	rec = &ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
		Items: []KitAssignment{{Kit: "healer", Item: "Healing Herbs", UnitID: 6}}}}
	f.withATO(t, rec)
	if _, err := f.svc.StartAftermath(f.ctx, f.owner(), 5, 1, 1, f.rec.Revision, elect(nil, map[int64]string{6: "healingHerbs"})); !errors.Is(err, ErrInvalidAftermath) || !strings.Contains(err.Error(), "only a Hero") {
		t.Errorf("Herbs on a Warrior: %v", err)
	}
}

func TestAllIsLostKeepsAgainstTheOddsBonusesWithTheSelection(t *testing.T) {
	f := newAmFixture(t)
	f.match.Result.Companies = []CompanyResult{{CompanyID: 1, Fled: true}}
	f.match.Winner, f.match.Result.Winner = 2, 2
	f.withATO(t, &ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"influence", "experience"}}})
	a, err := f.svc.Aftermath(f.ctx, f.owner(), 5, 1, 1)
	if err != nil || !a.Fled {
		t.Fatalf("%+v %v", a.Outcome, err)
	}
	var keys []string
	for _, l := range a.Preview.Influence {
		keys = append(keys, l.Key)
	}
	if !slices.Contains(keys, "participating") || !slices.Contains(keys, "odds:influence") || slices.Contains(keys, "win") || slices.Contains(keys, "draw") {
		t.Errorf("a fled company: participation and its Against the Odds Influence, no Win or Draw: %v", keys)
	}
	var xp bool
	for _, g := range a.Preview.Experience {
		for _, l := range g.Lines {
			xp = xp || l.Key == "odds:experience"
		}
	}
	if !xp {
		t.Error("and its Against the Odds experience")
	}
	if len(a.Preview.Rewards) != 0 {
		t.Errorf("no scenario rewards: %+v", a.Preview.Rewards)
	}
}
