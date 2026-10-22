package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"go.uber.org/mock/gomock"
)

// companyFixture is a service over mocks, with one campaign (id 5) in which user 9 owns company 1 (the state it holds).
type companyFixture struct {
	campaigns *MockcampaignRepository
	companies *MockcompanyRepository
	state     CompanyState
	settings  CampaignSettings // what the campaign plays with
	mu        sync.Mutex       // serialises updates like the row lock of the real repository
	user      User
	ctx       context.Context
}

func newCompanyFixture(t *testing.T) *companyFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := &companyFixture{campaigns: NewMockcampaignRepository(ctrl), companies: NewMockcompanyRepository(ctrl), state: gondor(), user: User{ID: 9}, ctx: context.Background()}
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(5)).DoAndReturn(func(context.Context, int64) (RulesetHashes, CampaignSettings, error) {
		return RulesetHashes{}, f.settings, nil
	}).AnyTimes()
	// user 9 owns company 1; any other user is a member without a company
	f.campaigns.EXPECT().Get(gomock.Any(), int64(9), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Company: &CampaignCompany{ID: 1}}}, nil).AnyTimes()
	f.campaigns.EXPECT().Get(gomock.Any(), int64(8), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5}}, nil).AnyTimes()
	f.campaigns.EXPECT().Get(gomock.Any(), int64(7), int64(5)).Return(CampaignDetail{}, ErrNotFound).AnyTimes()
	f.companies.EXPECT().Read(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(context.Context, int64, int64) (CompanyState, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.state.Clone(), nil
	}).AnyTimes()
	return f
}

// update makes the mock repository run fn on the fixture's state like the real one: a copy that is kept only when fn succeeds.
func (f *companyFixture) update() {
	f.companies.EXPECT().Update(gomock.Any(), int64(5), int64(1), gomock.Any()).DoAndReturn(func(_ context.Context, _, _ int64, fn func(*CompanyState) error) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		st := f.state.Clone()
		if err := fn(&st); err != nil {
			return err
		}
		next := int64(1000)
		for i := range st.Units { // the repository numbers new units
			if st.Units[i].ID == 0 {
				st.Units[i].ID = next
				next++
			}
		}
		f.state = st
		return nil
	}).AnyTimes()
}

func (f *companyFixture) service(t *testing.T) *CompanyService {
	return NewCompanyService(f.campaigns, f.companies, staticRules{shippedRules(t)})
}

func TestCompanyProfilesComeFromTheCampaignRules(t *testing.T) {
	f := newCompanyFixture(t)
	profiles, err := f.service(t).CompanyProfiles(f.ctx, 5)
	if err != nil || len(profiles) < 30 {
		t.Fatalf("profiles = %d, %v", len(profiles), err)
	}
	if len(profiles[0].Previews) == 0 {
		t.Error("profiles carry their unit previews")
	}
}

func TestCreateCompany(t *testing.T) {
	rs := shippedRules(t)
	profile, _ := rs.BC.Profile("Minas Tirith")
	f := newCompanyFixture(t)
	f.campaigns.EXPECT().Get(gomock.Any(), int64(10), int64(5)).Return(CampaignDetail{}, nil).AnyTimes()
	var got NewCompany
	f.companies.EXPECT().Create(gomock.Any(), int64(5), User{ID: 10}, gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, _ User, c NewCompany) (CampaignCompany, error) {
		got = c
		return CampaignCompany{ID: 1, Name: c.Name, Faction: c.Faction}, nil
	})
	co, err := f.service(t).CreateCompany(f.ctx, User{ID: 10}, 5, CompanyInput{Name: " The Guard ", Faction: "Minas Tirith"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "The Guard" || got.Faction != "Minas Tirith" || got.Influence != StartingInfluence || got.Gold != profile.StartGold {
		t.Errorf("stored company = %+v", got)
	}
	if want := len(profile.StartList()); len(got.Units) != want {
		t.Fatalf("starting units = %d, want %d", len(got.Units), want)
	}
	roles := map[UnitRole]int{}
	for _, u := range got.Units {
		roles[u.Role]++
		if u.ID != 0 {
			t.Errorf("unit %s has the preset id %d: the repository assigns ids", u.Base, u.ID)
		}
	}
	if roles[RoleLeader] != 1 || roles[RoleSergeant] != 2 {
		t.Errorf("officers = %v, want one leader and two sergeants", roles)
	}
	if co.Name != "The Guard" {
		t.Errorf("returned company = %+v", co)
	}
}

func TestCreateCompanyRefusals(t *testing.T) {
	tests := []struct {
		name string
		user User
		in   CompanyInput
		want error
	}{
		{"has one already", User{ID: 9}, CompanyInput{Name: "A", Faction: "Minas Tirith"}, ErrHasCompany},
		{"not a member", User{ID: 7}, CompanyInput{Name: "A", Faction: "Minas Tirith"}, ErrNotFound},
		{"no name", User{ID: 8}, CompanyInput{Name: " ", Faction: "Minas Tirith"}, ErrInvalidCompany},
		{"no faction", User{ID: 8}, CompanyInput{Name: "A"}, ErrInvalidCompany},
		{"unknown faction", User{ID: 8}, CompanyInput{Name: "A", Faction: "Nowhere"}, ErrInvalidCompany},
		{"bad officers", User{ID: 8}, CompanyInput{Name: "A", Faction: "Minas Tirith", Heroes: []StartHero{{Index: 0, Role: RoleLeader}}}, ErrInvalidCompany},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCompanyFixture(t) // Create is never expected: a refusal must not reach the repository
			if _, err := f.service(t).CreateCompany(f.ctx, tt.user, 5, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCreateCompanyPassesDuplicateFromTheRepository(t *testing.T) {
	f := newCompanyFixture(t)
	f.campaigns.EXPECT().Get(gomock.Any(), int64(10), int64(5)).Return(CampaignDetail{}, nil)
	f.companies.EXPECT().Create(gomock.Any(), int64(5), User{ID: 10}, gomock.Any()).Return(CampaignCompany{}, ErrHasCompany) // a concurrent second request
	if _, err := f.service(t).CreateCompany(f.ctx, User{ID: 10}, 5, CompanyInput{Name: "A", Faction: "Minas Tirith"}); !errors.Is(err, ErrHasCompany) {
		t.Errorf("err = %v", err)
	}
}

func TestCompanyDetailIsReadOnlyForEveryone(t *testing.T) {
	f := newCompanyFixture(t)
	f.campaigns.EXPECT().Company(gomock.Any(), int64(5), int64(1)).Return(CampaignCompany{Wins: 2, Draws: 1, Losses: 3}, nil)
	d, err := f.service(t).CompanyDetail(f.ctx, 5, 1)
	if err != nil || len(d.Roster) != len(f.state.Units) || d.Influence != 12 {
		t.Fatalf("detail = %d units, influence %d, %v", len(d.Roster), d.Influence, err)
	}
	if d.Wins != 2 || d.Draws != 1 || d.Losses != 3 || d.Played() != 6 { // the state read does not count matches
		t.Errorf("W/D/L = %d/%d/%d", d.Wins, d.Draws, d.Losses)
	}
}

func TestRenameCompany(t *testing.T) {
	f := newCompanyFixture(t)
	f.update()
	s := f.service(t)
	if err := s.RenameCompany(f.ctx, f.user, 5, "  The Black Company "); err != nil || f.state.Company.Name != "The Black Company" {
		t.Fatalf("rename: %v, name %q", err, f.state.Company.Name)
	}
	if err := s.RenameCompany(f.ctx, f.user, 5, " "); !errors.Is(err, ErrRefused) {
		t.Errorf("empty name: %v", err)
	}
	if err := s.RenameCompany(f.ctx, f.user, 5, strings.Repeat("x", 61)); !errors.Is(err, ErrRefused) {
		t.Errorf("long name: %v", err)
	}
	for _, stranger := range []User{{ID: 8}, {ID: 7}} {
		if err := s.RenameCompany(f.ctx, stranger, 5, "Mine now"); !errors.Is(err, ErrNotFound) {
			t.Errorf("user %d: %v, want ErrNotFound", stranger.ID, err)
		}
	}
	if f.state.Company.Name != "The Black Company" {
		t.Errorf("a stranger renamed the company: %q", f.state.Company.Name)
	}
}

func TestRenameUnit(t *testing.T) {
	f := newCompanyFixture(t)
	f.update()
	s := f.service(t)
	if err := s.RenameUnit(f.ctx, f.user, 5, 6, "  Cirion   the  Bold "); err != nil || f.state.Units[5].Name != "Cirion the Bold" {
		t.Fatalf("rename: %v, name %q", err, f.state.Units[5].Name)
	}
	if err := s.RenameUnit(f.ctx, f.user, 5, 6, ""); err != nil || f.state.Units[5].Name != "" {
		t.Errorf("empty name resets: %v, %q", err, f.state.Units[5].Name)
	}
	if err := s.RenameUnit(f.ctx, f.user, 5, 6, strings.Repeat("é", 41)); !errors.Is(err, ErrRefused) {
		t.Errorf("long name: %v", err)
	}
	if err := s.RenameUnit(f.ctx, f.user, 5, 999, "Ghost"); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign unit: %v", err)
	}
	if err := s.RenameUnit(f.ctx, User{ID: 8}, 5, 6, "Mine"); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
}

func TestSaveFormation(t *testing.T) {
	f := newCompanyFixture(t)
	f.update()
	s := f.service(t)
	all := make([]int64, 0, len(f.state.Units))
	for _, u := range f.state.Units {
		all = append(all, u.ID)
	}
	if len(all) < 3 {
		t.Fatalf("the fixture has units: %v", all)
	}
	ordered := append([]int64{all[2], all[0]}, all[1])
	ordered = append(ordered, all[3:]...)
	if err := s.SaveFormation(f.ctx, f.user, 5, ordered); err != nil {
		t.Fatal(err)
	}
	if got := f.state.Order; len(got) != len(all) || got[0] != all[2] || got[1] != all[0] || got[2] != all[1] {
		t.Errorf("order = %v", got)
	}
	d, _, _ := s.detail(f.ctx, 5, 1)
	if len(d.Roster) != len(f.state.Units) || d.Roster[0].ID != all[2] || d.Roster[1].ID != all[0] {
		t.Errorf("roster starts %d, %d; the saved order must lead", d.Roster[0].ID, d.Roster[1].ID)
	}
	saved := append([]int64(nil), f.state.Order...)
	bad := map[string][]int64{
		"unknown id": append(append([]int64{}, ordered[:len(ordered)-1]...), 12345),
		"repeat":     append(append([]int64{}, ordered[:len(ordered)-1]...), ordered[0]),
		"missing":    ordered[1:],
		"empty":      nil,
	}
	for name, order := range bad {
		if err := s.SaveFormation(f.ctx, f.user, 5, order); !errors.Is(err, ErrRefused) {
			t.Errorf("%s: want a refusal, got %v", name, err)
		}
		if got := f.state.Order; len(got) != len(saved) || got[0] != saved[0] {
			t.Errorf("%s: a refused order must change nothing: %v", name, got)
		}
	}
	if err := s.SaveFormation(f.ctx, User{ID: 8}, 5, ordered); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
}
