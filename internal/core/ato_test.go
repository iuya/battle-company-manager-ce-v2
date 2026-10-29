package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

func realATO(t *testing.T) ATORules {
	t.Helper()
	return shippedRules(t).BC.ATO()
}

func TestParseATO(t *testing.T) {
	r := realATO(t)
	if !r.Inclusive || r.MaxKits != 5 || len(r.Kits) != 5 || len(r.Bonuses) != 6 || len(r.Wanderers) == 0 {
		t.Fatalf("rules: inclusive=%v kits=%d/%d bonuses=%d wanderers=%d", r.Inclusive, len(r.Kits), r.MaxKits, len(r.Bonuses), len(r.Wanderers))
	}
	for _, c := range []struct {
		key    string
		rating int
	}{{"reroll", 15}, {"influence", 15}, {"experience", 30}, {"toolKit", 30}, {"wanderer", 45}, {"ambush", 60}} {
		if b, ok := r.Bonus(c.key); !ok || b.Rating != c.rating {
			t.Errorf("%s: %+v", c.key, b)
		}
	}
	healer, ok := r.Kit("healer")
	if !ok || healer.Name != "Healer's Kit" || len(healer.Items) != 2 {
		t.Fatalf("healer: %+v", healer)
	}
	if herbs := healer.Items[1]; herbs.Item != "Healing Herbs" || herbs.Count != 2 || !herbs.HeroOnly {
		t.Errorf("herbs: %+v", herbs)
	}
	if cram := healer.Items[0]; cram.Item != "Wondrous Cram" || cram.Count != 5 || cram.HeroOnly {
		t.Errorf("cram: %+v", cram)
	}
	if hunter, _ := r.Kit("hunter"); !hunter.Items[0].Detail {
		t.Errorf("Envenom Weapon names its weapon: %+v", hunter.Items[0])
	}
	if w := r.Wanderers[0]; w.OwnPrice == 0 || !strings.Contains(w.Summary, "Fight") {
		t.Errorf("wanderer: %+v", w)
	}
}

func atoRoster() []CompanyUnit {
	return []CompanyUnit{
		{ID: 1, Name: "Aldor", Role: RoleLeader},
		{ID: 2, Name: "Brand", Role: RoleSergeant},
		{ID: 5, Name: "Edo", Role: RoleWarrior},
		{ID: 7, Name: "Gul", Role: RoleWarrior, Injured: true},
	}
}

func TestATOSelectionValidate(t *testing.T) {
	r := realATO(t)
	scenarios := []string{"Domination", "Rescue"}
	wand := r.Wanderers[0].Name
	tests := []struct {
		name    string
		sel     ATOSelection
		roster  []CompanyUnit
		gap     int
		wantErr string
	}{
		{name: "nothing", gap: 0},
		{name: "a bonus that fits", sel: ATOSelection{Bonuses: []string{"influence"}}, gap: 20},
		{name: "exactly the gap is allowed", sel: ATOSelection{Bonuses: []string{"influence"}}, gap: 15},
		{name: "one over the gap", sel: ATOSelection{Bonuses: []string{"influence"}}, gap: 14, wantErr: "cost 15 Rating and the gap is 14"},
		{name: "everything but the kit", sel: ATOSelection{Bonuses: []string{"reroll", "influence", "experience"}}, gap: 60},
		{name: "unknown bonus", sel: ATOSelection{Bonuses: []string{"fortune"}}, gap: 99, wantErr: `"fortune" is not an Against the Odds bonus`},
		{name: "a bonus twice", sel: ATOSelection{Bonuses: []string{"reroll", "reroll"}}, gap: 99, wantErr: "chosen twice"},
		{name: "a kit costs 30 each", sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer", "hunter"}}, gap: 60},
		{name: "two kits do not fit 59", sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer", "hunter"}}, gap: 59, wantErr: "cost 60"},
		{name: "the same kit twice", sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer", "healer"}}, gap: 99, wantErr: "only have one of each kit"},
		{name: "unknown kit", sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"thief"}}, gap: 99, wantErr: "not a kit"},
		{name: "Tool Kit without a kit", sel: ATOSelection{Bonuses: []string{"toolKit"}}, gap: 99, wantErr: "at least one kit"},
		{name: "kits without the bonus", sel: ATOSelection{Kits: []string{"healer"}}, gap: 99, wantErr: "need the Tool Kit"},
		{name: "five kits", gap: 150, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer", "explorer", "scholar", "hunter", "raider"}}},
		{name: "items go to models taking part", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "healer", Item: "Wondrous Cram", Index: 4, UnitID: 5}, {Kit: "healer", Item: "Healing Herbs", Index: 0, UnitID: 1}, {Kit: "healer", Item: "Healing Herbs", Index: 1}}}},
		{name: "Herbs to a Warrior", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "healer", Item: "Healing Herbs", UnitID: 5}}}, wantErr: "can only go to a Hero"},
		{name: "an injured model misses the game", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "healer", Item: "Wondrous Cram", UnitID: 7}}}, wantErr: "misses this game"},
		{name: "a model that is not on the roster", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "healer", Item: "Wondrous Cram", UnitID: 99}}}, wantErr: "not on the roster"},
		{name: "a copy beyond the count", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "healer", Item: "Healing Herbs", Index: 2}}}, wantErr: "has no Healing Herbs (copy 3)"},
		{name: "an item of another kit", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "healer", Item: "Map"}}}, wantErr: "has no Map"},
		{name: "a kit that was not chosen", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "explorer", Item: "Map", UnitID: 1}}}, wantErr: "not one of the kits you chose"},
		{name: "a copy assigned twice", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "healer", Item: "Wondrous Cram", UnitID: 5}, {Kit: "healer", Item: "Wondrous Cram", UnitID: 2}}}, wantErr: "assigned twice"},
		{name: "a weapon for Envenom Weapon", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"hunter"},
			Items: []KitAssignment{{Kit: "hunter", Item: "Envenom Weapon (X)", UnitID: 5, Detail: "Spear"}}}},
		{name: "a weapon for something else", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"},
			Items: []KitAssignment{{Kit: "healer", Item: "Wondrous Cram", UnitID: 5, Detail: "Spear"}}}, wantErr: "takes no weapon"},
		{name: "a weapon too long", gap: 30, sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"hunter"},
			Items: []KitAssignment{{Kit: "hunter", Item: "Envenom Weapon (X)", UnitID: 5, Detail: strings.Repeat("x", 41)}}}, wantErr: "at most 40"},
		{name: "a Wanderer", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderer: wand}, gap: 45},
		{name: "a Wanderer one short", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderer: wand}, gap: 44, wantErr: "cost 45"},
		{name: "no Wanderer named", sel: ATOSelection{Bonuses: []string{"wanderer"}}, gap: 45, wantErr: "choose which Wanderer"},
		{name: "an unknown Wanderer", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderer: "Gandalf"}, gap: 45, wantErr: "choose which Wanderer"},
		{name: "a Wanderer without the bonus", sel: ATOSelection{Wanderer: wand}, gap: 45, wantErr: "needs the Wanderer bonus"},
		{name: "a permanent Wanderer on the roster", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderer: wand}, gap: 45,
			roster: append(atoRoster(), CompanyUnit{ID: 9, Name: "Wan", Role: RoleHero, Wanderer: true}), wantErr: "already has a Wanderer"},
		{name: "Ambush", sel: ATOSelection{Bonuses: []string{"ambush"}, Ambush: "Rescue"}, gap: 60},
		{name: "Ambush needs 60", sel: ATOSelection{Bonuses: []string{"ambush"}, Ambush: "Rescue"}, gap: 59, wantErr: "cost 60"},
		{name: "Ambush with no scenario", sel: ATOSelection{Bonuses: []string{"ambush"}}, gap: 60, wantErr: "choose the scenario"},
		{name: "Ambush with an unknown scenario", sel: ATOSelection{Bonuses: []string{"ambush"}, Ambush: "Picnic"}, gap: 60, wantErr: "choose the scenario"},
		{name: "a scenario without Ambush", sel: ATOSelection{Ambush: "Rescue"}, gap: 60, wantErr: "needs the Ambush! bonus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roster := tt.roster
			if roster == nil {
				roster = atoRoster()
			}
			out, err := tt.sel.Validate(r, roster, tt.gap, scenarios)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				if out.Spent(r) > tt.gap {
					t.Errorf("spent %d of %d", out.Spent(r), tt.gap)
				}
				return
			}
			if !errors.Is(err, ErrInvalidMatch) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestATOSelectionIsCleaned(t *testing.T) {
	r := realATO(t)
	out, err := ATOSelection{Bonuses: []string{"ambush", "reroll", "toolKit"}, Kits: []string{"hunter", "healer"}, Ambush: "Rescue",
		Items: []KitAssignment{{Kit: "hunter", Item: "Trophy Pelt", UnitID: 1}, {Kit: "healer", Item: "Wondrous Cram", Index: 1, UnitID: 5}, {Kit: "healer", Item: "Wondrous Cram", Index: 0, UnitID: 2}}}.
		Validate(r, atoRoster(), 200, []string{"Rescue"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Bonuses, []string{"reroll", "toolKit", "ambush"}) || !slices.Equal(out.Kits, []string{"healer", "hunter"}) {
		t.Errorf("the file's order: %+v", out)
	}
	if out.Items[0].Kit != "healer" || out.Items[0].Index != 0 || out.Items[1].Index != 1 || out.Items[2].Kit != "hunter" {
		t.Errorf("assignments sorted: %+v", out.Items)
	}
}

func TestATOAccessFor(t *testing.T) {
	m := Match{Sides: []MatchSide{{Companies: []CampaignCompany{{ID: 1, OwnerID: 10}}}, {Companies: []CampaignCompany{{ID: 2, OwnerID: 20}}}}}
	lower := m.Sides[0].Companies[0]
	for _, tt := range []struct {
		name  string
		u     User
		ended bool
		want  ATOAccess
	}{
		{"the owner of the lower company", User{ID: 10}, false, ATOEdit},
		{"the opposing owner", User{ID: 20}, false, ATONone},
		{"a stranger", User{ID: 7}, false, ATONone},
		{"the organiser", User{ID: 99}, false, ATOEdit},
		{"an administrator", User{ID: 5, Role: RoleAdmin}, false, ATOEdit},
		{"the owner after the end", User{ID: 10}, true, ATONone},
		{"the organiser after the end", User{ID: 99}, true, ATONone},
	} {
		m.Ended = tt.ended
		if got := ATOAccessFor(tt.u, m, lower, 99); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

// --- the service ---

type atoFixture struct {
	*matchesFixture
	repo *MockatoRepository
	rec  ATORecord
	// started is what the repository says about the game
	started bool
}

// newATOFixture is a two-company match: Rangers (company 1, owner 10) is rated 100 against Raiders (company 2, owner 20) rated 190, so
// the gap is 90.
func newATOFixture(t *testing.T) *atoFixture {
	t.Helper()
	f := &atoFixture{matchesFixture: newMatchesFixture(t)}
	f.repo = NewMockatoRepository(gomock.NewController(t))
	f.match = Match{ID: 1, Number: 1, Revision: 3, Sides: []MatchSide{
		{Name: "A", Companies: []CampaignCompany{{ID: 1, OwnerID: 10, Name: "Rangers"}}},
		{Name: "B", Companies: []CampaignCompany{{ID: 2, OwnerID: 20, Name: "Raiders"}}}}}
	f.companies = NewMockcompanyRepository(gomock.NewController(t))
	f.companies.EXPECT().Read(gomock.Any(), int64(5), gomock.Any()).DoAndReturn(func(_ context.Context, _, id int64) (CompanyState, error) {
		st := gondor()
		st.Company.ID, st.Company.Name = id, map[int64]string{1: "Rangers", 2: "Raiders"}[id]
		return st, nil
	}).AnyTimes()
	f.companies.EXPECT().ReadRoster(gomock.Any(), int64(5)).DoAndReturn(func(context.Context, int64) ([]CompanyState, error) {
		a, b := gondor(), gondor()
		a.Company.ID, b.Company.ID = 1, 2
		for i := int64(0); i < 12; i++ { // a bigger company: the gap is wide enough for Ambush!
			b.Units = append(b.Units, UnitState{ID: 90 + i, Base: "Warrior of Minas Tirith", Role: RoleWarrior})
		}
		return []CompanyState{a, b}, nil
	}).AnyTimes()
	f.svc = NewMatches(f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)}).WithATO(f.repo)
	f.repo.EXPECT().Get(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) (ATORecord, error) { return f.rec, nil }).AnyTimes()
	f.repo.EXPECT().GameStarted(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) (bool, error) { return f.started, nil }).AnyTimes()
	f.repo.EXPECT().Update(gomock.Any(), int64(5), int64(1), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ int64, _ User, fn func(*ATOState) error) (Match, error) {
			st := ATOState{Match: f.match, Record: f.rec, Started: f.started}
			if err := fn(&st); err != nil {
				return Match{}, err
			}
			st.Match.Revision++
			f.match, f.rec = st.Match, st.Record
			if st.Record.Selection.Empty() {
				f.rec = ATORecord{}
			}
			return st.Match, nil
		}).AnyTimes()
	return f
}

func (f *atoFixture) card(t *testing.T, user int64) ATO {
	t.Helper()
	c, err := f.svc.AgainstTheOdds(f.ctx, User{ID: user}, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAgainstTheOddsCard(t *testing.T) {
	f := newATOFixture(t)
	c := f.card(t, 10)
	if !c.Eligible || c.CompanyID != 1 || c.OwnRating >= c.OppRating || c.Gap != c.OppRating-c.OwnRating || c.Gap < 60 {
		t.Fatalf("card: %+v", c)
	}
	if c.OppName != "Raiders" || len(c.Options) != 6 || len(c.Kits) != 5 || len(c.Wanderers) == 0 || len(c.Models) == 0 || c.Access != ATOEdit || c.Revision != 3 {
		t.Errorf("card: %+v", c)
	}
	for _, m := range c.Models {
		if m.UnitID == 7 {
			t.Error("an injured model takes no part: it cannot carry a kit item")
		}
	}
	if o := f.card(t, 20); o.Access != ATONone || o.CompanyID != 1 {
		t.Errorf("the opposing owner sees the card read only: %+v", o)
	}
	if o := f.card(t, 99); o.Access != ATOEdit {
		t.Errorf("the organiser edits: %+v", o)
	}
	// a bonus that does not fit is disabled with the reason
	f.rec = ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"influence"}}}
	left := c.Gap - 15
	for _, o := range f.card(t, 10).Options {
		if o.Key == "ambush" && left < 60 && (!o.Disabled || o.Reason != fmt.Sprintf("Needs 60, %d left", left)) {
			t.Errorf("ambush: %+v (left %d)", o, left)
		}
		if o.Key == "influence" && (!o.Taken || o.Disabled) {
			t.Errorf("the taken bonus stays enabled: %+v", o)
		}
	}
	// shapes without a card
	f.match.Sides = append(f.match.Sides, MatchSide{Name: "C", Companies: []CampaignCompany{{ID: 3, OwnerID: 30, Name: "Wolves"}}})
	if o := f.card(t, 10); o.Eligible || !strings.Contains(o.Why, "two-company") {
		t.Errorf("three sides: %+v", o)
	}
}

func TestAgainstTheOddsEqualRatings(t *testing.T) {
	f := newATOFixture(t)
	f.companies = NewMockcompanyRepository(gomock.NewController(t))
	f.companies.EXPECT().ReadRoster(gomock.Any(), int64(5)).DoAndReturn(func(context.Context, int64) ([]CompanyState, error) {
		a, b := gondor(), gondor()
		a.Company.ID, b.Company.ID = 1, 2
		return []CompanyState{a, b}, nil
	}).AnyTimes()
	f.svc = NewMatches(f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)}).WithATO(f.repo)
	c := f.card(t, 10)
	if c.Eligible || !strings.HasPrefix(c.Why, "Against the Odds: nobody. Both companies are rated") || c.Access != ATONone {
		t.Errorf("equal ratings: %+v", c)
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{Bonuses: []string{"reroll"}}); !errors.Is(err, ErrRefused) {
		t.Errorf("saving without a card: %v", err)
	}
}

func TestSaveAgainstTheOdds(t *testing.T) {
	f := newATOFixture(t)
	gap := f.card(t, 10).Gap
	sel := ATOSelection{Bonuses: []string{"reroll", "influence"}}
	c, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, sel)
	if err != nil || !c.Selection.Has("reroll") || c.Spent != 30 || c.Revision != 4 || f.rec.CompanyID != 1 || f.rec.MatchID != 1 {
		t.Fatalf("save: %+v %v (record %+v)", c, err, f.rec)
	}
	// refusals
	for _, tt := range []struct {
		name string
		user int64
		rev  int
		sel  ATOSelection
		want error
	}{
		{"the opposing owner", 20, 4, sel, ErrRefused},
		{"a stranger", 7, 4, sel, ErrRefused},
		{"a stale revision", 10, 3, sel, ErrMatchChanged},
		{"over budget", 10, 4, ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer", "explorer", "scholar", "hunter", "raider"}}, ErrInvalidMatch},
	} {
		if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: tt.user}, 5, 1, tt.rev, tt.sel); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
	if gap < 150 {
		t.Log("gap", gap)
	}
	if f.match.Revision != 4 {
		t.Errorf("refused saves change nothing: revision %d", f.match.Revision)
	}
	// the organiser edits too; an empty selection clears
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 99}, 5, 1, 4, ATOSelection{}); err != nil || !f.rec.Selection.Empty() || f.rec.CompanyID != 0 {
		t.Fatalf("clear: %v %+v", err, f.rec)
	}
	// locked once the game began
	f.started = true
	_, err = f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 5, sel)
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "game has started") {
		t.Errorf("locked: %v", err)
	}
	if c := f.card(t, 10); !c.Locked || c.LockedWhy != "The game has started." || c.Access != ATONone {
		t.Errorf("the card is read only: %+v", c)
	}
	f.started = false
	if c := f.card(t, 10); c.Locked || c.Access != ATOEdit {
		t.Errorf("undoing every entry unlocks it: %+v", c)
	}
}

func TestAmbushSetsAndRestoresTheScenario(t *testing.T) {
	f := newATOFixture(t)
	rolled := &MatchScenario{Kind: ScenarioRulebook, Name: f.firstScenario(t).Name}
	f.match.Scenario = rolled
	f.match.Result.Companies = []CompanyResult{{CompanyID: 1, Objectives: map[string]int{"x": 1}}}
	var other string
	for _, sc := range mustScenarios(t, f) {
		if sc.Name != rolled.Name {
			other = sc.Name
			break
		}
	}
	gap := f.card(t, 10).Gap
	if gap < 60 {
		t.Skipf("the fixture's gap is %d", gap)
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, ATOSelection{Bonuses: []string{"ambush"}, Ambush: other}); err != nil {
		t.Fatal(err)
	}
	if f.match.Scenario == nil || f.match.Scenario.Name != other || f.match.Scenario.Kind != ScenarioRulebook || f.match.Revision != 4 {
		t.Fatalf("scenario: %+v rev %d", f.match.Scenario, f.match.Revision)
	}
	if f.rec.Previous == nil || f.rec.Previous.Name != rolled.Name || !f.rec.HadPrevious {
		t.Errorf("the rolled scenario is remembered: %+v", f.rec)
	}
	if f.match.Result.Companies[0].Objectives != nil {
		t.Error("the objectives belonged to the old scenario")
	}
	// choosing another scenario with Ambush keeps remembering the first one
	var third string
	for _, sc := range mustScenarios(t, f) {
		if sc.Name != rolled.Name && sc.Name != other {
			third = sc.Name
			break
		}
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 4, ATOSelection{Bonuses: []string{"ambush"}, Ambush: third}); err != nil {
		t.Fatal(err)
	}
	if f.match.Scenario.Name != third || f.rec.Previous == nil || f.rec.Previous.Name != rolled.Name {
		t.Errorf("%+v %+v", f.match.Scenario, f.rec.Previous)
	}
	// dropping Ambush puts the rolled one back
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 5, ATOSelection{Bonuses: []string{"reroll"}}); err != nil {
		t.Fatal(err)
	}
	if f.match.Scenario == nil || f.match.Scenario.Name != rolled.Name || f.rec.Previous != nil {
		t.Errorf("restored: %+v %+v", f.match.Scenario, f.rec)
	}
	// a match that had no scenario gets none back
	f.match.Scenario = nil
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 6, ATOSelection{Bonuses: []string{"ambush"}, Ambush: other}); err != nil {
		t.Fatal(err)
	}
	if f.match.Scenario == nil || f.rec.HadPrevious {
		t.Fatalf("%+v %+v", f.match.Scenario, f.rec)
	}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 7, ATOSelection{}); err != nil {
		t.Fatal(err)
	}
	if f.match.Scenario != nil {
		t.Errorf("no scenario before, none after: %+v", f.match.Scenario)
	}
	// somebody chose another scenario by hand meanwhile: Ambush no longer owns it, so dropping it leaves that choice
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 8, ATOSelection{Bonuses: []string{"ambush"}, Ambush: other}); err != nil {
		t.Fatal(err)
	}
	f.match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: third}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 9, ATOSelection{}); err != nil {
		t.Fatal(err)
	}
	if f.match.Scenario == nil || f.match.Scenario.Name != third {
		t.Errorf("a scenario changed by hand stays: %+v", f.match.Scenario)
	}
}

func mustScenarios(t *testing.T, f *atoFixture) []ScenarioInfo {
	t.Helper()
	list, err := f.svc.Scenarios(f.ctx, 5)
	if err != nil || len(list) < 3 {
		t.Fatalf("scenarios: %d %v", len(list), err)
	}
	return list
}
