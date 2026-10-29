package core

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.uber.org/mock/gomock"
)

// Several companies per player (docs/multi-company-spec.md): the cap setting, the single ownership helper, creation rules, and
// two companies of one owner in the services, in a match and in its aftermath.

func TestCompanyCapReadsAbsentAndOutOfRangeAsOne(t *testing.T) {
	for in, want := range map[int]int{0: 1, 1: 1, 2: 2, 4: 4, 5: 1, -1: 1} {
		if got := (CampaignSettings{CompaniesPerPlayer: in}).CompanyCap(); got != want {
			t.Errorf("CompaniesPerPlayer %d: cap %d, want %d", in, got, want)
		}
	}
}

func TestCampaignInputValidatesCompaniesPerPlayer(t *testing.T) {
	for n, ok := range map[int]bool{0: true, 1: true, 4: true, 5: false, -1: false} {
		_, err := CampaignInput{Name: "C", Settings: CampaignSettings{CompaniesPerPlayer: n}}.Validate()
		if (err == nil) != ok || (err != nil && !errors.Is(err, ErrInvalidCampaign)) {
			t.Errorf("%d per player: %v, want ok=%v", n, err, ok)
		}
	}
}

func TestCampaignDetailOwnAndCanCreateCompany(t *testing.T) {
	two := []CampaignCompany{{ID: 3}, {ID: 8}}
	tests := []struct {
		name   string
		d      CampaignDetail
		create bool
	}{
		{"none, cap 1", CampaignDetail{}, true},
		{"one, cap 1 (default)", CampaignDetail{CampaignSummary: CampaignSummary{Companies: two[:1]}}, false},
		{"one, cap 2", CampaignDetail{CampaignSummary: CampaignSummary{Companies: two[:1]}, Settings: CampaignSettings{CompaniesPerPlayer: 2}}, true},
		{"two, cap 2", CampaignDetail{CampaignSummary: CampaignSummary{Companies: two}, Settings: CampaignSettings{CompaniesPerPlayer: 2}}, false},
		{"two, cap lowered to 1", CampaignDetail{CampaignSummary: CampaignSummary{Companies: two}, Settings: CampaignSettings{CompaniesPerPlayer: 1}}, false},
		{"finished", CampaignDetail{CampaignSummary: CampaignSummary{Status: CampaignFinished}, Settings: CampaignSettings{CompaniesPerPlayer: 3}}, false},
	}
	for _, tt := range tests {
		if got := tt.d.CanCreateCompany(); got != tt.create {
			t.Errorf("%s: CanCreateCompany = %v", tt.name, got)
		}
	}
	d := CampaignDetail{CampaignSummary: CampaignSummary{Companies: two}}
	if c, ok := d.Own(8); !ok || c.ID != 8 {
		t.Errorf("Own(8) = %+v, %v", c, ok)
	}
	if _, ok := d.Own(4); ok {
		t.Error("Own(4): not the viewer's")
	}
}

func TestOwnCompanyResolvesAndAuthorisesEveryOwnerAction(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockcampaignRepository(ctrl)
	d := companyDeps{campaigns: repo}
	ctx := context.Background()
	mine := []CampaignCompany{{ID: 3}, {ID: 8}}
	repo.EXPECT().Get(ctx, int64(1), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Companies: mine}}, nil).AnyTimes()
	repo.EXPECT().Get(ctx, int64(2), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5}, OrganiserID: 2}, nil).AnyTimes() // organiser without a company
	repo.EXPECT().Get(ctx, int64(3), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Status: CampaignFinished, Companies: mine}}, nil).AnyTimes()
	repo.EXPECT().Get(ctx, int64(4), int64(5)).Return(CampaignDetail{}, ErrNotFound).AnyTimes()
	tests := []struct {
		name      string
		user      int64
		company   int64
		read, chg error
	}{
		{"first own", 1, 3, nil, nil},
		{"second own", 1, 8, nil, nil},
		{"another player's company", 1, 9, ErrNotFound, ErrNotFound},
		{"organiser probing a player's company", 2, 3, ErrNotFound, ErrNotFound},
		{"finished campaign: readable, not changeable", 3, 8, nil, ErrRefused},
		{"not in the campaign", 4, 3, ErrNotFound, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := d.ownCompany(ctx, User{ID: tt.user}, 5, tt.company)
			if !errors.Is(err, tt.read) && !(tt.read == nil && err == nil) || (err == nil && id != tt.company) {
				t.Errorf("ownCompany = %d, %v, want %v", id, err, tt.read)
			}
			id, err = d.ownCompanyToChange(ctx, User{ID: tt.user}, 5, tt.company)
			if !errors.Is(err, tt.chg) && !(tt.chg == nil && err == nil) || (err == nil && id != tt.company) {
				t.Errorf("ownCompanyToChange = %d, %v, want %v", id, err, tt.chg)
			}
		})
	}
}

// TestCreateCompanyHonoursTheCap: cap 1 (the default) refuses a second company, cap 2 allows it, a lowered cap keeps the extra companies
// and blocks new ones; the cap travels to the repository, which checks it again atomically.
func TestCreateCompanyHonoursTheCap(t *testing.T) {
	own := func(names ...string) []CampaignCompany {
		var out []CampaignCompany
		for i, n := range names {
			out = append(out, CampaignCompany{ID: int64(i + 1), Name: n})
		}
		return out
	}
	tests := []struct {
		name     string
		cap      int // CompaniesPerPlayer as stored (0: absent)
		owned    []CampaignCompany
		newName  string
		wantErr  error
		wantCap  int
		finished bool
	}{
		{"first company, default cap", 0, nil, "Rangers", nil, 1, false},
		{"default cap refuses a second", 0, own("Rangers"), "Riders", ErrCompanyLimit, 0, false},
		{"cap 1 refuses a second", 1, own("Rangers"), "Riders", ErrCompanyLimit, 0, false},
		{"cap 2 allows a second", 2, own("Rangers"), "Riders", nil, 2, false},
		{"cap 2 refuses a third", 2, own("Rangers", "Riders"), "Scouts", ErrCompanyLimit, 0, false},
		{"lowered below the count: existing stay, new blocked", 1, own("Rangers", "Riders", "Scouts"), "More", ErrCompanyLimit, 0, false},
		{"cap 4 allows a fourth", 4, own("A", "B", "C"), "D", nil, 4, false},
		{"name unique per owner, case ignored", 3, own("Rangers"), "  rANGERS ", ErrCompanyNameTaken, 0, false},
		{"name clash is an invalid company", 3, own("Rangers"), "Rangers", ErrInvalidCompany, 0, false},
		{"a finished campaign refuses", 3, nil, "Rangers", ErrRefused, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCompanyFixture(t)
			status := CampaignActive
			if tt.finished {
				status = CampaignFinished
			}
			f.campaigns.EXPECT().Get(gomock.Any(), int64(10), int64(5)).Return(CampaignDetail{
				CampaignSummary: CampaignSummary{ID: 5, Status: status, Companies: tt.owned}, Settings: CampaignSettings{CompaniesPerPlayer: tt.cap}}, nil)
			if tt.wantErr == nil {
				f.companies.EXPECT().Create(gomock.Any(), int64(5), User{ID: 10}, gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, _ User, c NewCompany) (CampaignCompany, error) {
					if c.Cap != tt.wantCap {
						t.Errorf("NewCompany.Cap = %d, want %d", c.Cap, tt.wantCap)
					}
					return CampaignCompany{ID: 1, Name: c.Name}, nil
				})
			} // else Create is never expected: a refusal must not reach the repository
			_, err := f.service(t).CreateCompany(f.ctx, User{ID: 10}, 5, CompanyInput{Name: tt.newName, Faction: "Minas Tirith"})
			if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRenameCompanyKeepsNamesUniquePerOwner(t *testing.T) {
	f := newCompanyFixture(t)
	f.campaigns.EXPECT().Get(gomock.Any(), int64(11), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Companies: []CampaignCompany{{ID: 1, Name: "Rangers"}, {ID: 2, Name: "Riders"}}}}, nil).AnyTimes()
	f.companies.EXPECT().Update(gomock.Any(), int64(5), int64(1), gomock.Any()).Return(nil).AnyTimes()
	s := f.service(t)
	u := User{ID: 11}
	if err := s.RenameCompany(f.ctx, u, 5, 1, " riders "); !errors.Is(err, ErrCompanyNameTaken) {
		t.Errorf("another company of the owner has the name: %v", err)
	}
	if err := s.RenameCompany(f.ctx, u, 5, 1, "RANGERS"); err != nil {
		t.Errorf("changing the case of its own name is fine: %v", err)
	}
	if err := s.RenameCompany(f.ctx, u, 5, 1, "Scouts"); err != nil {
		t.Errorf("a free name: %v", err)
	}
}

// twoCompanyFixture is user 9 with companies 1 and 2 in campaign 5, each with a state of its own.
type twoCompanyFixture struct {
	*companyFixture
	second CompanyState
}

func newTwoCompanyFixture(t *testing.T) *twoCompanyFixture {
	t.Helper()
	f := newCompanyFixture(t)
	g := &twoCompanyFixture{companyFixture: f, second: gondor()}
	g.second.Company.ID, g.second.Company.Name, g.second.Influence = 2, "The Second Company", 7
	f.campaigns.EXPECT().Get(gomock.Any(), int64(21), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Companies: []CampaignCompany{{ID: 1}, {ID: 2}}}}, nil).AnyTimes()
	f.user = User{ID: 21}
	f.companies.EXPECT().Read(gomock.Any(), int64(5), int64(2)).DoAndReturn(func(context.Context, int64, int64) (CompanyState, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return g.second.Clone(), nil
	}).AnyTimes()
	f.companies.EXPECT().Update(gomock.Any(), int64(5), int64(2), gomock.Any()).DoAndReturn(func(_ context.Context, _, _ int64, fn func(*CompanyState) error) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		st := g.second.Clone()
		if err := fn(&st); err != nil {
			return err
		}
		g.second = st
		return nil
	}).AnyTimes()
	f.update()
	return g
}

// TestTwoCompaniesOfOneOwnerShareNothing: what one company buys, recruits, renames or levels never touches the other.
func TestTwoCompaniesOfOneOwnerShareNothing(t *testing.T) {
	g := newTwoCompanyFixture(t)
	rules := staticRules{shippedRules(t)}
	arm := NewArmouryService(g.campaigns, g.companies, rules)
	if err := arm.BuyEquipment(g.ctx, g.user, 5, 2, 2, "Badge of Courage", ""); err != nil {
		t.Fatal(err)
	}
	if g.second.Influence != 5 || g.state.Influence != 12 {
		t.Errorf("influence after buying in company 2: company 2 = %d (was 7), company 1 = %d (was 12)", g.second.Influence, g.state.Influence)
	}
	cs := NewCompanyService(g.campaigns, g.companies, rules)
	if err := cs.RenameUnit(g.ctx, g.user, 5, 1, 6, "First's Cirion"); err != nil {
		t.Fatal(err)
	}
	if g.state.Units[5].Name != "First's Cirion" || g.second.Units[5].Name == "First's Cirion" {
		t.Errorf("renaming a unit of company 1 changed company 2: %q / %q", g.state.Units[5].Name, g.second.Units[5].Name)
	}
	if err := cs.RenameCompany(g.ctx, g.user, 5, 2, "Renamed"); err != nil || g.second.Company.Name != "Renamed" || g.state.Company.Name == "Renamed" {
		t.Errorf("rename company 2: %v, %q / %q", err, g.second.Company.Name, g.state.Company.Name)
	}
	order := make([]int64, len(g.second.Units))
	for i := range order {
		order[len(order)-1-i] = g.second.Units[i].ID
	}
	if err := cs.SaveFormation(g.ctx, g.user, 5, 2, order); err != nil || len(g.state.Order) != 0 || !slices.Equal(g.second.Order, order) {
		t.Errorf("formation of company 2: %v, company 1 order %v", err, g.state.Order)
	}
	// a company that is not the user's is not found, whatever its number
	if err := cs.RenameUnit(g.ctx, g.user, 5, 3, 6, "Nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another player's company: %v", err)
	}
	// a recruit in progress is per company
	rec := NewRecruitService(g.campaigns, g.companies, rules, &FixedDice{Rolls: []int{3}})
	if err := rec.RollRecruit(g.ctx, g.user, 5, 2, 0, false); err != nil {
		t.Fatalf("recruit roll in company 2: %v", err)
	}
	if r, err := rec.RecruitState(g.ctx, g.user, 5, 1); err != nil || r != nil {
		t.Errorf("company 1 has no recruit in progress: %+v, %v", r, err)
	}
	if r, err := rec.RecruitState(g.ctx, g.user, 5, 2); err != nil || r == nil {
		t.Errorf("company 2 has its recruit in progress: %+v, %v", r, err)
	}
}

func TestSetCompaniesPerPlayer(t *testing.T) {
	tests := []struct {
		name   string
		actor  User
		status CampaignStatus
		n      int
		want   error
		saved  int
	}{
		{"organiser raises it", User{ID: 1}, CampaignActive, 3, nil, 3},
		{"organiser lowers it", User{ID: 1}, CampaignActive, 1, nil, 1},
		{"paused is fine", User{ID: 1}, CampaignPaused, 2, nil, 2},
		{"administrator", User{ID: 9, Role: RoleAdmin}, CampaignActive, 4, nil, 4},
		{"a player may not", User{ID: 2}, CampaignActive, 2, ErrRefused, 0},
		{"finished is frozen", User{ID: 1}, CampaignFinished, 2, ErrRefused, 0},
		{"zero", User{ID: 1}, CampaignActive, 0, ErrInvalidCampaign, 0},
		{"five", User{ID: 1}, CampaignActive, 5, ErrInvalidCampaign, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockcampaignRepository(ctrl)
			svc := &Campaigns{companyDeps: companyDeps{campaigns: repo}}
			saved := 0
			repo.EXPECT().UpdateSettings(gomock.Any(), int64(5), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, fn func(int64, CampaignStatus, CampaignSettings) (CampaignSettings, error)) error {
				next, err := fn(1, tt.status, CampaignSettings{ShakeTheTable: true})
				if err == nil {
					saved = next.CompaniesPerPlayer
					if !next.ShakeTheTable {
						t.Error("the other settings are kept")
					}
				}
				return err
			}).AnyTimes()
			err := svc.SetCompaniesPerPlayer(context.Background(), tt.actor, 5, tt.n)
			if tt.want == nil && err != nil || tt.want != nil && !errors.Is(err, tt.want) || saved != tt.saved {
				t.Errorf("err = %v (want %v), saved %d (want %d)", err, tt.want, saved, tt.saved)
			}
		})
	}
}

func TestSamePlayersNotesOwnersWithSeveralCompanies(t *testing.T) {
	co := func(id, owner int64, name string) CampaignCompany {
		return CampaignCompany{ID: id, OwnerID: owner, Owner: name}
	}
	sides := []MatchSide{
		{Name: "A", Companies: []CampaignCompany{co(1, 10, "Ana"), co(2, 10, "Ana"), co(3, 11, "Bo")}},
		{Name: "B", Companies: []CampaignCompany{co(4, 10, "Ana"), co(5, 12, "Cy")}},
	}
	got := SamePlayers(sides)
	if len(got) != 1 || got[0].OwnerID != 10 || got[0].Companies != 3 || !got[0].AcrossSides || !slices.Equal(got[0].SameSide, []int{0}) {
		t.Errorf("Ana plays three companies, two together on side 1 and one against herself: %+v", got)
	}
	if got := SamePlayers(sides[1:]); got != nil {
		t.Errorf("one company each: %+v", got)
	}
	both := Match{Sides: []MatchSide{{Companies: []CampaignCompany{co(1, 10, "Ana")}}, {Companies: []CampaignCompany{co(2, 10, "Ana")}}}}
	if g := both.SamePlayers(); len(g) != 1 || !g[0].AcrossSides || len(g[0].SameSide) != 0 {
		t.Errorf("one owner on both sides: %+v", g)
	}
	// nothing is refused: ids are all ValidateSides looks at
	if _, err := ValidateSides([]NewSide{{Companies: []int64{1, 2}}, {Companies: []int64{3}}}); err != nil {
		t.Errorf("sides: %v", err)
	}
}

// TestAnOwnerOfTwoCompaniesInOneMatch: access is per company, both aftermaths are the owner's, and one tap confirms both ends.
func TestAnOwnerOfTwoCompaniesInOneMatch(t *testing.T) {
	a, b := CampaignCompany{ID: 1, OwnerID: 7}, CampaignCompany{ID: 2, OwnerID: 7}
	ended := Match{Ended: true, Sides: []MatchSide{{Companies: []CampaignCompany{a}}, {Companies: []CampaignCompany{b}}}}
	for _, c := range []CampaignCompany{a, b} {
		if AftermathAccessFor(User{ID: 7}, ended, c, 9) != AftermathEdit {
			t.Errorf("company %d: its owner edits its aftermath", c.ID)
		}
		if AftermathAccessFor(User{ID: 8}, ended, c, 9) != AftermathNone {
			t.Errorf("company %d: another player does not", c.ID)
		}
	}
	open := Match{Sides: ended.Sides}
	if LiveAccessFor(User{ID: 7}, open, a, 9) != LiveEdit || LiveAccessFor(User{ID: 7}, open, b, 9) != LiveEdit {
		t.Error("the owner plays both companies live")
	}
}

func TestOwnerOfTwoCompaniesConfirmsTheEndWithOneTap(t *testing.T) {
	f := endFixture(t)
	f.match.Sides[1].Companies[1].OwnerID = 20 // Raiders and Wolves belong to the same player
	m, err := f.svc.ProposeEnd(f.ctx, playerOf(10, "Hador"), 5, 1, 3)
	if err != nil || len(m.EndProposal.Waiting()) != 2 {
		t.Fatalf("propose: %+v %v", m.EndProposal, err)
	}
	m, err = f.svc.ConfirmEnd(f.ctx, playerOf(20, "Gor"), 5, 1, m.Revision)
	if err != nil || !m.Ended {
		t.Fatalf("one confirmation covers both companies of the owner: %+v %v", m.EndProposal, err)
	}
}
