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
		{name: "a Wanderer", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{wand}}, gap: 45},
		{name: "a Wanderer one short", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{wand}}, gap: 44, wantErr: "cost 45"},
		{name: "no Wanderer named", sel: ATOSelection{Bonuses: []string{"wanderer"}}, gap: 45, wantErr: "choose which Wanderer"},
		{name: "an unknown Wanderer", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{"Gandalf"}}, gap: 45, wantErr: "not a Wanderer"},
		{name: "two different Wanderers", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{wand, r.Wanderers[1].Name}}, gap: 90},
		{name: "two different Wanderers one short", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{wand, r.Wanderers[1].Name}}, gap: 89, wantErr: "cost 90"},
		{name: "the same Wanderer twice", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{wand, wand}}, gap: 90, wantErr: "can only join once"},
		{name: "three Influence", sel: ATOSelection{Bonuses: []string{"influence", "influence", "influence"}}, gap: 45},
		{name: "three Influence one short", sel: ATOSelection{Bonuses: []string{"influence", "influence", "influence"}}, gap: 44, wantErr: "cost 45"},
		{name: "two Experience and two Reroll", sel: ATOSelection{Bonuses: []string{"experience", "reroll", "experience", "reroll"}}, gap: 90},
		{name: "Ambush twice", sel: ATOSelection{Bonuses: []string{"ambush", "ambush"}, Ambush: "Rescue"}, gap: 120, wantErr: "chosen twice"},
		{name: "the Tool Kit bonus twice", sel: ATOSelection{Bonuses: []string{"toolKit", "toolKit"}, Kits: []string{"healer"}}, gap: 60, wantErr: "chosen twice"},
		{name: "the Wanderer bonus twice", sel: ATOSelection{Bonuses: []string{"wanderer", "wanderer"}, Wanderers: []string{wand}}, gap: 90, wantErr: "chosen twice"},
		{name: "a Wanderer without the bonus", sel: ATOSelection{Wanderers: []string{wand}}, gap: 45, wantErr: "needs the Wanderer bonus"},
		{name: "a permanent Wanderer on the roster", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{wand}}, gap: 45,
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

func TestATOStatusCanDecideFollowsATOAccessFor(t *testing.T) {
	st := ATOStatus{Pending: true, OwnerID: 10}
	for _, tt := range []struct {
		name string
		u    User
		want bool
	}{
		{"the owner of the lower company", User{ID: 10}, true},
		{"the opposing owner", User{ID: 20}, false},
		{"the organiser", User{ID: 99}, true},
		{"an administrator", User{ID: 5, Role: RoleAdmin}, true},
	} {
		if got := st.CanDecide(tt.u, 99); got != tt.want {
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

// newATOFixture is a two-company match: Rangers (company 1, owner 10) is rated 238 against Raiders (company 2, owner 20) rated 326, so
// the gap is 88.
func newATOFixture(t *testing.T) *atoFixture { return newATOFixtureGap(t, 90) }

// newATOFixtureGap is newATOFixture with a company bigger by gap/8 Warriors of Minas Tirith: about that gap, whatever their Rating makes it.
func newATOFixtureGap(t *testing.T, gap int) *atoFixture {
	t.Helper()
	f := &atoFixture{matchesFixture: newMatchesFixture(t)}
	f.repo = NewMockatoRepository(gomock.NewController(t))
	f.match = Match{ID: 1, Number: 1, Revision: 3, Scenario: &MatchScenario{Kind: ScenarioCustom, Name: "The bridge"}, Sides: []MatchSide{
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
		for i := int64(0); i < int64(gap)/8; i++ { // a bigger company: the gap is wide enough for Ambush!
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

// filled completes a selection the way a player must: it adds what still fits until nothing does.
func (f *atoFixture) filled(t *testing.T, sel ATOSelection) ATOSelection {
	t.Helper()
	r, card := realATO(t), f.card(t, 10)
	c := ATOContext{Gap: card.Gap, Practice: card.Practice, HasWanderer: card.HasWanderer}
	for {
		rest := sel.StillAffordable(r, c)
		if len(rest) == 0 {
			return sel
		}
		switch b := rest[0]; b.Key {
		case atoToolKit:
			if !sel.Has(atoToolKit) {
				sel.Bonuses = append(sel.Bonuses, atoToolKit)
			}
			sel.Kits = append(sel.Kits, r.Kits[len(sel.Kits)].Key)
		case atoWanderer:
			if !sel.Has(atoWanderer) {
				sel.Bonuses = append(sel.Bonuses, atoWanderer)
			}
			sel.Wanderers = append(sel.Wanderers, r.Wanderers[len(sel.Wanderers)].Name)
		default:
			sel.Bonuses = append(sel.Bonuses, b.Key)
		}
	}
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
	sel := f.filled(t, ATOSelection{Bonuses: []string{"reroll"}})
	c, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, sel)
	if err != nil || !c.Selection.Has("reroll") || c.Spent != sel.Spent(realATO(t)) || gap-c.Spent >= 15 || c.Revision != 4 || f.rec.CompanyID != 1 || f.rec.MatchID != 1 {
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
	// the organiser edits too
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 99}, 5, 1, 4, f.filled(t, ATOSelection{Bonuses: []string{"experience"}})); err != nil || !f.rec.Selection.Has("experience") {
		t.Fatalf("the organiser: %v %+v", err, f.rec)
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
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, f.filled(t, ATOSelection{Bonuses: []string{"ambush"}, Ambush: other})); err != nil {
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
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 4, f.filled(t, ATOSelection{Bonuses: []string{"ambush"}, Ambush: third})); err != nil {
		t.Fatal(err)
	}
	if f.match.Scenario.Name != third || f.rec.Previous == nil || f.rec.Previous.Name != rolled.Name {
		t.Errorf("%+v %+v", f.match.Scenario, f.rec.Previous)
	}
	// dropping Ambush puts the rolled one back
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 5, f.filled(t, ATOSelection{Bonuses: []string{"reroll"}})); err != nil {
		t.Fatal(err)
	}
	if f.match.Scenario == nil || f.match.Scenario.Name != rolled.Name || f.rec.Previous != nil {
		t.Errorf("restored: %+v %+v", f.match.Scenario, f.rec)
	}
	// an older match whose only scenario was Ambush!'s: dropping it would leave none, and without a scenario nothing is confirmed
	f.match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: other}
	f.rec = ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"ambush"}, Ambush: other}}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 6, ATOSelection{}); !errors.Is(err, ErrRefused) {
		t.Errorf("no scenario left: %v", err)
	}
	// somebody chose another scenario by hand meanwhile: Ambush no longer owns it, so dropping it leaves that choice
	f.match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: third}
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 6, f.filled(t, ATOSelection{})); err != nil {
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

func TestATOStillAffordable(t *testing.T) {
	r := realATO(t)
	wand := r.Wanderers[0].Name
	keys := func(s ATOSelection, c ATOContext) string {
		var out []string
		for _, b := range s.StillAffordable(r, c) {
			out = append(out, b.Key)
		}
		return strings.Join(out, " ")
	}
	repeat := func(key string, n int) []string { return slices.Repeat([]string{key}, n) }
	all := make([]string, len(r.Wanderers))
	for i, w := range r.Wanderers {
		all[i] = w.Name
	}
	tests := []struct {
		name string
		sel  ATOSelection
		c    ATOContext
		want string
	}{
		{name: "a gap under the cheapest bonus", c: ATOContext{Gap: 14}},
		{name: "a bonus that costs exactly the gap counts", c: ATOContext{Gap: 15}, want: "influence reroll"},
		{name: "the owner's 40: the cost that fits is offered", c: ATOContext{Gap: 40}, want: "influence experience reroll toolKit"},
		{name: "after a 15, the 30s no longer fit", sel: ATOSelection{Bonuses: []string{"influence"}}, c: ATOContext{Gap: 40}, want: "influence reroll"},
		{name: "one way to finish 40", sel: ATOSelection{Bonuses: []string{"influence", "reroll"}}, c: ATOContext{Gap: 40}},
		{name: "another way to finish 40", sel: ATOSelection{Bonuses: []string{"experience"}}, c: ATOContext{Gap: 40}},
		{name: "a kit bought counts", sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"}}, c: ATOContext{Gap: 45}, want: "influence reroll"},
		{name: "45 pays for a Wanderer or three Influence", c: ATOContext{Gap: 45}, want: "influence experience reroll toolKit wanderer"},
		{name: "three Influence finish 45", sel: ATOSelection{Bonuses: repeat("influence", 3)}, c: ATOContext{Gap: 45}},
		{name: "a permanent Wanderer shuts the temporary one out", c: ATOContext{Gap: 45, HasWanderer: true}, want: "influence experience reroll toolKit"},
		{name: "another Wanderer is still open", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{wand}}, c: ATOContext{Gap: 90}, want: "influence experience reroll toolKit wanderer"},
		{name: "every Wanderer taken", sel: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: all}, c: ATOContext{Gap: 45*len(all) + 45}, want: "influence experience reroll toolKit"},
		{name: "Ambush is open once", c: ATOContext{Gap: 60, CanAmbush: true}, want: "influence experience reroll toolKit wanderer ambush"},
		{name: "Ambush taken", sel: ATOSelection{Bonuses: []string{"ambush"}, Ambush: "Rescue"}, c: ATOContext{Gap: 75, CanAmbush: true}, want: "influence reroll"},
		{name: "Ambush with no scenario to choose", c: ATOContext{Gap: 60}, want: "influence experience reroll toolKit wanderer"},
		{name: "a practice match: no reward bonus", c: ATOContext{Gap: 30, Practice: true}, want: "toolKit"},
		{name: "a practice match: all it can take", sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"}}, c: ATOContext{Gap: 40, Practice: true}},
		{name: "a practice match: one kit after another", sel: ATOSelection{Bonuses: []string{"toolKit"}, Kits: []string{"healer"}}, c: ATOContext{Gap: 60, Practice: true}, want: "toolKit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keys(tt.sel, tt.c); got != tt.want {
				t.Errorf("still affordable: %q, want %q", got, tt.want)
			}
		})
	}
}

func TestATOSpentCountsEveryPurchase(t *testing.T) {
	r := realATO(t)
	s := ATOSelection{Bonuses: []string{"influence", "influence", "experience", "reroll", "reroll", "toolKit", "wanderer"}, Kits: []string{"healer", "raider"}, Wanderers: []string{r.Wanderers[0].Name, r.Wanderers[1].Name}}
	if got, want := s.Spent(r), 30+30+30+60+90; got != want {
		t.Errorf("spent %d, want %d", got, want)
	}
	if s.Count("influence") != 2 || s.Count("ambush") != 0 {
		t.Errorf("counts: %d %d", s.Count("influence"), s.Count("ambush"))
	}
}

// A bonus file that does not say repeatable keeps each bonus once: a campaign keeps the file it was created with.
func TestATOWithoutRepeatableEachBonusIsTakenOnce(t *testing.T) {
	r := realATO(t)
	r.Bonuses = slices.Clone(r.Bonuses)
	for i := range r.Bonuses {
		r.Bonuses[i].Repeatable = false
	}
	if _, err := (ATOSelection{Bonuses: []string{"influence", "influence"}}).Validate(r, atoRoster(), 30, nil); err == nil || !strings.Contains(err.Error(), "chosen twice") {
		t.Errorf("twice: %v", err)
	}
	if _, err := (ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{r.Wanderers[0].Name, r.Wanderers[1].Name}}).Validate(r, atoRoster(), 90, nil); err == nil || !strings.Contains(err.Error(), "only 1 Wanderer") {
		t.Errorf("two Wanderers: %v", err)
	}
	if got := (ATOSelection{Bonuses: []string{"influence", "reroll"}}).StillAffordable(r, ATOContext{Gap: 45}); len(got) != 0 {
		t.Errorf("only 15 left and both 15s taken: %v", got)
	}
}

// A free bonus would keep "still affordable" true for ever if it repeated: the decision could never be confirmed.
func TestATOFreeBonusNeverRepeats(t *testing.T) {
	r := ATORules{Inclusive: true, Bonuses: []ATOBonusRule{{Key: "gift", Name: "Gift", Rating: 0, Repeatable: true}}}
	if _, err := (ATOSelection{Bonuses: []string{"gift", "gift"}}).Validate(r, nil, 10, nil); err == nil {
		t.Error("twice was accepted")
	}
	if got := (ATOSelection{Bonuses: []string{"gift"}}).StillAffordable(r, ATOContext{Gap: 10}); len(got) != 0 {
		t.Errorf("still affordable: %v", got)
	}
}

// docs/matches.md, "Against the Odds before the game": a forged POST cannot confirm while a bonus still fits, and nothing is stored.
func TestSaveRefusesWhileABonusStillFits(t *testing.T) {
	f := newATOFixture(t)
	for name, sel := range map[string]ATOSelection{
		"nothing":    {},
		"a little":   {Bonuses: []string{"reroll"}},
		"almost all": {Bonuses: slices.Repeat([]string{"influence"}, 4)},
	} {
		_, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, sel)
		if !errors.Is(err, ErrInvalidMatch) || !strings.Contains(err.Error(), "Still to take: ") {
			t.Errorf("%s: %v", name, err)
		}
		if f.match.ATODecided || !f.rec.Selection.Empty() || f.match.Revision != 3 {
			t.Errorf("%s stored something: %+v %+v", name, f.match, f.rec)
		}
	}
	// the organiser is not exempt
	if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 99}, 5, 1, 3, ATOSelection{}); !errors.Is(err, ErrInvalidMatch) {
		t.Errorf("the organiser: %v", err)
	}
	// two different ways to finish are both a decision
	for _, sel := range []ATOSelection{f.filled(t, ATOSelection{Bonuses: []string{"experience", "reroll", "reroll"}}), f.filled(t, ATOSelection{Bonuses: []string{"influence"}})} {
		f.match.Revision, f.match.ATODecided = 3, false
		if _, err := f.svc.SaveAgainstTheOdds(f.ctx, User{ID: 10}, 5, 1, 3, sel); err != nil || !f.match.ATODecided {
			t.Errorf("%v: %v", sel.Bonuses, err)
		}
	}
}

// The card offers what the gap can pay, keeps what was taken, and counts the purchases of a bonus that repeats.
func TestCardOffersOnlyWhatTheGapCanPay(t *testing.T) {
	f := newATOFixture(t)
	keys := func(c ATO) []string {
		var out []string
		for _, o := range c.Options {
			out = append(out, o.Key)
		}
		return out
	}
	r := realATO(t)
	all := make([]string, 0, len(r.Bonuses))
	for _, b := range r.Bonuses {
		all = append(all, b.Key)
	}
	c := f.card(t, 10)
	if !slices.Equal(keys(c), all) {
		t.Errorf("a gap of %d offers %v", c.Gap, keys(c))
	}
	// spending does not hide a row: it stays, disabled, with the reason
	f.rec = ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: slices.Repeat([]string{"influence"}, 5)}}
	c = f.card(t, 10)
	if !slices.Equal(keys(c), all) {
		t.Errorf("spent: %v", keys(c))
	}
	for _, o := range c.Options {
		switch o.Key {
		case "influence":
			if o.Count != 5 || !o.Taken || !o.Repeatable || o.Disabled {
				t.Errorf("influence: %+v", o)
			}
		case "experience":
			if !o.Disabled || !o.OverBudget || o.Reason != "Can't pay: 17 short" {
				t.Errorf("experience: %+v", o)
			}
		}
	}
	// a small gap: Wanderer and Ambush! are not offered at all
	small := newATOFixtureGap(t, 40)
	c = small.card(t, 10)
	if want := []string{"influence", "experience", "reroll", "toolKit"}; !slices.Equal(keys(c), want) {
		t.Errorf("a gap of %d offers %v, want %v", c.Gap, keys(c), want)
	}
	// what was taken stays on the card even if the gap no longer pays it, so it can be removed
	small.rec = ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"ambush"}, Ambush: "Rescue"}}
	if c = small.card(t, 10); !slices.Contains(keys(c), "ambush") {
		t.Errorf("the taken Ambush! is gone: %v", keys(c))
	}
}

func TestLiveHasOneUnitPerTemporaryWanderer(t *testing.T) {
	rec := &ATORecord{}
	f, _ := newATOLiveFixture(t, rec)
	r := realATO(t)
	*rec = ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"wanderer"}, Wanderers: []string{r.Wanderers[0].Name, r.Wanderers[1].Name}}}
	lc, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(lc.Wanderers) != 2 || lc.Wanderers[0].UnitID == lc.Wanderers[1].UnitID {
		t.Fatalf("wanderers: %+v", lc.Wanderers)
	}
	seen := map[int64]bool{}
	for _, u := range lc.Units {
		if u.Temporary {
			seen[u.UnitID] = true
			if _, ok := lc.TemporaryWanderer(u.UnitID); !ok {
				t.Errorf("unit %d has no Wanderer", u.UnitID)
			}
		}
	}
	if len(seen) != 2 {
		t.Errorf("temporary units: %v", seen)
	}
}
