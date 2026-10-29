package core

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

// staticRules serves one RuleSet whatever the hashes.
type staticRules struct{ rs *RuleSet }

func (s staticRules) For(context.Context, RulesetHashes) (*RuleSet, error) { return s.rs, nil }

type campaignFixture struct {
	svc       *Campaigns
	campaigns *MockcampaignRepository
	companies *MockcompanyRepository
	rulesets  *MockrulesetRepository
}

func newCampaignFixture(t *testing.T) campaignFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	f := campaignFixture{campaigns: NewMockcampaignRepository(ctrl), companies: NewMockcompanyRepository(ctrl), rulesets: NewMockrulesetRepository(ctrl)}
	f.svc = NewCampaigns(f.campaigns, f.companies, f.rulesets, staticRules{shippedRules(t)}, nil)
	return f
}

func (f campaignFixture) hashes() {
	f.campaigns.EXPECT().Hashes(gomock.Any(), gomock.Any()).Return(RulesetHashes{Vanilla: "v", BattleCompanies: "b", Glossary: "g"}, CampaignSettings{}, nil).AnyTimes()
}

func TestCampaignsCreateResolvesDefaults(t *testing.T) {
	f := newCampaignFixture(t)
	ctx := context.Background()
	owner := User{ID: 1, CanCreateCampaigns: true}
	f.rulesets.EXPECT().Default(ctx, KindVanilla).Return(RulesetInfo{Kind: KindVanilla, Hash: "dv"}, nil)
	f.rulesets.EXPECT().Info(ctx, "mine").Return(RulesetInfo{Kind: KindBattleCompanies, Hash: "mine"}, nil)
	f.rulesets.EXPECT().Default(ctx, KindGlossary).Return(RulesetInfo{Kind: KindGlossary, Hash: "dg"}, nil)
	settings := CampaignSettings{ManualRolls: true}
	want := NewCampaign{Name: "Siege", Description: "d", Vanilla: "dv", BattleCompanies: "mine", Glossary: "dg", Settings: settings}
	f.campaigns.EXPECT().Create(ctx, owner, want).Return(CampaignSummary{ID: 7, Name: "Siege"}, nil)
	got, err := f.svc.Create(ctx, owner, CampaignInput{Name: " Siege ", Description: "d", BattleCompanies: "mine", Settings: settings})
	if err != nil || got.ID != 7 {
		t.Fatalf("Create = %+v, %v", got, err)
	}
}

func TestCampaignsCreateRefusals(t *testing.T) {
	ctx := context.Background()
	maker := User{ID: 1, CanCreateCampaigns: true}
	tests := []struct {
		name  string
		user  User
		in    CampaignInput
		setup func(f campaignFixture)
		want  error
	}{
		{"plain user", User{ID: 2}, CampaignInput{Name: "x"}, nil, ErrRefused},
		{"admin may", User{ID: 2, Role: RoleAdmin}, CampaignInput{Name: ""}, nil, ErrInvalidCampaign},
		{"empty name", maker, CampaignInput{Name: " "}, nil, ErrInvalidCampaign},
		{"unknown hash", maker, CampaignInput{Name: "x", Vanilla: "nope"}, func(f campaignFixture) {
			f.rulesets.EXPECT().Info(ctx, "nope").Return(RulesetInfo{}, ErrNotFound)
		}, ErrInvalidCampaign},
		{"wrong kind", maker, CampaignInput{Name: "x", Glossary: "bc"}, func(f campaignFixture) {
			f.rulesets.EXPECT().Default(ctx, KindVanilla).Return(RulesetInfo{Kind: KindVanilla, Hash: "dv"}, nil)
			f.rulesets.EXPECT().Default(ctx, KindBattleCompanies).Return(RulesetInfo{Kind: KindBattleCompanies, Hash: "db"}, nil)
			f.rulesets.EXPECT().Info(ctx, "bc").Return(RulesetInfo{Kind: KindBattleCompanies, Hash: "bc"}, nil)
		}, ErrInvalidCampaign},
		{"no default stored", maker, CampaignInput{Name: "x"}, func(f campaignFixture) {
			f.rulesets.EXPECT().Default(ctx, KindVanilla).Return(RulesetInfo{}, ErrNotFound)
		}, ErrInvalidCampaign},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCampaignFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			if tt.name == "admin may" { // passes the permission check and fails on the name: Create is never reached
				tt.in.Name = ""
			}
			_, err := f.svc.Create(ctx, tt.user, tt.in)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCampaignsRateCompaniesFromTheRules(t *testing.T) {
	f := newCampaignFixture(t)
	ctx := context.Background()
	f.hashes()
	state := gondor()
	want := BuildDetail(shippedRules(t), CampaignSettings{}, state).CampaignCompany
	if want.Points == 0 || want.FullPoints <= want.Points || want.Units == 0 {
		t.Fatalf("the fixture should have injured units: %+v", want)
	}
	f.companies.EXPECT().Read(ctx, int64(5), int64(1)).Return(state, nil).AnyTimes()
	f.companies.EXPECT().ReadRoster(ctx, int64(5)).Return([]CompanyState{state}, nil).AnyTimes()
	listed := CampaignCompany{ID: 1, Name: "The White Company", Wins: 2, Owner: "Ann"}

	t.Run("Company", func(t *testing.T) {
		f.campaigns.EXPECT().Company(ctx, int64(5), int64(1)).Return(listed, nil)
		got, err := f.svc.Company(ctx, 5, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got.Points != want.Points || got.FullPoints != want.FullPoints || got.Units != want.Units || got.Wins != 2 || got.Owner != "Ann" {
			t.Errorf("Company = %+v, want the figures %+v with the repository's record", got, want)
		}
	})
	t.Run("Companies", func(t *testing.T) {
		f.campaigns.EXPECT().Companies(ctx, int64(5)).Return([]CampaignCompany{listed}, nil)
		got, err := f.svc.Companies(ctx, 5)
		if err != nil || len(got) != 1 || got[0].Points != want.Points {
			t.Errorf("Companies = %+v, %v", got, err)
		}
	})
	t.Run("Get", func(t *testing.T) {
		f.campaigns.EXPECT().Get(ctx, int64(9), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Companies: []CampaignCompany{listed}}, Settings: CampaignSettings{ManualRolls: true}}, nil)
		got, err := f.svc.Get(ctx, 9, 5)
		if err != nil || len(got.Companies) != 1 || got.Companies[0].Points != want.Points || !got.Settings.ManualRolls {
			t.Errorf("Get = %+v, %v", got, err)
		}
		if listed.Points != 0 {
			t.Error("the repository's value must not be modified in place")
		}
	})
	t.Run("List", func(t *testing.T) {
		f.campaigns.EXPECT().List(ctx, int64(9), true).Return([]CampaignSummary{{ID: 5, Companies: []CampaignCompany{listed}}, {ID: 6}}, nil)
		got, err := f.svc.List(ctx, 9, true)
		if err != nil || len(got) != 2 || got[0].Companies[0].FullPoints != want.FullPoints || got[1].Companies != nil {
			t.Errorf("List = %+v, %v", got, err)
		}
	})
}

func TestCampaignsNotFoundPassesThrough(t *testing.T) {
	f := newCampaignFixture(t)
	ctx := context.Background()
	f.campaigns.EXPECT().Get(ctx, int64(9), int64(5)).Return(CampaignDetail{}, ErrNotFound)
	if _, err := f.svc.Get(ctx, 9, 5); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get err = %v", err)
	}
	f.campaigns.EXPECT().Company(ctx, int64(5), int64(2)).Return(CampaignCompany{}, ErrNotFound)
	if _, err := f.svc.Company(ctx, 5, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("Company err = %v", err)
	}
}

func TestCampaignsSettings(t *testing.T) {
	f := newCampaignFixture(t)
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(5)).Return(RulesetHashes{}, CampaignSettings{HeroesDropProfileGear: true}, nil)
	got, err := f.svc.Settings(context.Background(), 5)
	if err != nil || !got.HeroesDropProfileGear || got.ManualRolls {
		t.Errorf("Settings = %+v, %v", got, err)
	}
}

func TestCampaignsHandOver(t *testing.T) {
	ctx := context.Background()
	// the repository runs allowed with the current organiser (id 1) and returns its verdict
	run := func(t *testing.T, actor User, newOrg int64) error {
		f := newCampaignFixture(t)
		f.campaigns.EXPECT().HandOver(ctx, int64(5), newOrg, gomock.Any()).DoAndReturn(
			func(_ context.Context, _, _ int64, allowed func(int64) error) error { return allowed(1) })
		return f.svc.HandOver(ctx, actor, 5, newOrg)
	}
	for _, c := range []struct {
		name   string
		actor  User
		newOrg int64
		refuse bool
	}{
		{"the organiser", User{ID: 1, Role: RoleUser}, 2, false},
		{"an admin who is not a member", User{ID: 9, Role: RoleAdmin}, 2, false},
		{"a superuser", User{ID: 9, Role: RoleSuperuser}, 2, false},
		{"another player", User{ID: 3, Role: RoleUser}, 2, true},
		{"to the current organiser", User{ID: 1, Role: RoleUser}, 1, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := run(t, c.actor, c.newOrg)
			if c.refuse != errors.Is(err, ErrRefused) || (!c.refuse && err != nil) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("errors pass through", func(t *testing.T) {
		f := newCampaignFixture(t)
		f.campaigns.EXPECT().HandOver(ctx, int64(5), int64(2), gomock.Any()).Return(ErrNotFound)
		if err := f.svc.HandOver(ctx, User{ID: 1}, 5, 2); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestSetCampaignStatusRules(t *testing.T) {
	ctx := context.Background()
	organiser, player, admin := User{ID: 1}, User{ID: 2}, User{ID: 9, Role: RoleAdmin}
	for _, tc := range []struct {
		name     string
		from, to CampaignStatus
		actor    User
		want     CampaignStatus // the status afterwards
		refused  bool
	}{
		{"pause", CampaignActive, CampaignPaused, organiser, CampaignPaused, false},
		{"resume", CampaignPaused, CampaignActive, organiser, CampaignActive, false},
		{"end an active campaign", CampaignActive, CampaignFinished, organiser, CampaignFinished, false},
		{"end a paused campaign", CampaignPaused, CampaignFinished, admin, CampaignFinished, false},
		{"a player", CampaignActive, CampaignPaused, player, CampaignActive, true},
		{"the organiser cannot reactivate", CampaignFinished, CampaignActive, organiser, CampaignFinished, true},
		{"an administrator reactivates", CampaignFinished, CampaignActive, admin, CampaignActive, false},
		{"no pausing a finished campaign", CampaignFinished, CampaignPaused, admin, CampaignFinished, true},
		{"same status", CampaignActive, CampaignActive, organiser, CampaignActive, true},
		{"unknown status", CampaignActive, "dormant", organiser, CampaignActive, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockcampaignRepository(ctrl)
			got := tc.from
			repo.EXPECT().SetStatus(ctx, int64(5), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, fn func(int64, string, CampaignStatus) (CampaignStatus, error)) error {
				to, err := fn(1, "Rangers", tc.from)
				if err != nil {
					return err
				}
				got = to
				return nil
			})
			c := &Campaigns{companyDeps: companyDeps{campaigns: repo}}
			err := c.SetCampaignStatus(ctx, tc.actor, 5, tc.to, "Rangers")
			if (err != nil) != tc.refused || (err != nil && !errors.Is(err, ErrRefused)) || got != tc.want {
				t.Errorf("err %v, status %s, want refused=%v status %s", err, got, tc.refused, tc.want)
			}
		})
	}
}

func TestEndingACampaignNeedsItsName(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	repo := NewMockcampaignRepository(ctrl)
	repo.EXPECT().SetStatus(ctx, int64(5), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, fn func(int64, string, CampaignStatus) (CampaignStatus, error)) error {
		_, err := fn(1, "Rangers", CampaignActive)
		return err
	}).Times(3)
	c := &Campaigns{companyDeps: companyDeps{campaigns: repo}}
	org := User{ID: 1}
	for _, confirm := range []string{"", "rangers", "Other"} {
		if err := c.SetCampaignStatus(ctx, org, 5, CampaignFinished, confirm); !errors.Is(err, ErrRefused) {
			t.Errorf("confirm %q: %v", confirm, err)
		}
	}
}

// The companies of a campaign are rated from one read of the rosters, not one read per company (no Read expectation: a call fails).
func TestCampaignsCompaniesReadTheRostersOnce(t *testing.T) {
	f := newCampaignFixture(t)
	ctx := context.Background()
	f.hashes()
	var states []CompanyState
	var listed []CampaignCompany
	for id := int64(1); id <= 3; id++ {
		st := gondor()
		st.Company.ID = id
		states = append(states, st)
		listed = append(listed, CampaignCompany{ID: id, Name: "c", Wins: int(id)})
	}
	f.campaigns.EXPECT().Companies(ctx, int64(5)).Return(listed, nil)
	f.companies.EXPECT().ReadRoster(ctx, int64(5)).Return(states, nil).Times(1)
	got, err := f.svc.Companies(ctx, 5)
	if err != nil || len(got) != 3 {
		t.Fatalf("Companies = %+v, %v", got, err)
	}
	for _, c := range got {
		if c.Points == 0 || c.Units == 0 || c.Wins != int(c.ID) {
			t.Errorf("company %d: %+v should be rated and keep the repository's record", c.ID, c)
		}
	}
}

func TestCampaignsMembers(t *testing.T) {
	ctx := context.Background()
	f := newCampaignFixture(t)
	want := []CampaignMember{{UserID: 1, Name: "Org", Organiser: true, Companies: []MemberCompany{{ID: 4, Name: "Rangers"}, {ID: 6, Name: "Riders"}}}, {UserID: 2, Name: "No company"}}
	f.campaigns.EXPECT().Members(ctx, int64(5)).Return(want, nil)
	got, err := f.svc.Members(ctx, 5)
	if err != nil || len(got) != 2 || len(got[0].Companies) != 2 || len(got[1].Companies) != 0 {
		t.Fatalf("Members = %+v, %v", got, err)
	}
	f.campaigns.EXPECT().Members(ctx, int64(6)).Return(nil, ErrNotFound)
	if _, err := f.svc.Members(ctx, 6); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown campaign: %v", err)
	}
}

// NavCampaigns lists id, name and status without reading a roster or loading rules (List rates every company).
func TestCampaignsNavCampaignsDoesNotRate(t *testing.T) {
	f := newCampaignFixture(t)
	ctx := context.Background()
	f.campaigns.EXPECT().List(ctx, int64(3), true).Return([]CampaignSummary{
		{ID: 1, Name: "A", Status: CampaignActive, Companies: []CampaignCompany{{ID: 9}}},
		{ID: 2, Name: "B", Status: CampaignFinished},
	}, nil)
	got, err := f.svc.NavCampaigns(ctx, 3, true)
	want := []CampaignNav{{ID: 1, Name: "A", Status: CampaignActive}, {ID: 2, Name: "B", Status: CampaignFinished}}
	if err != nil || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("NavCampaigns = %+v, %v", got, err)
	}
}
