package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

// expectFiles makes the campaign's pinned files and the app's defaults: pinned[i] is the hash of vanilla, Battle Companies, glossary.
func (f campaignFixture) expectFiles(pinned [3]string, name string) {
	kinds := []RulesetKind{KindVanilla, KindBattleCompanies, KindGlossary}
	cur := [3]string{"dv", "db", "dg"}
	for i, k := range kinds {
		f.rulesets.EXPECT().Info(gomock.Any(), pinned[i]).Return(RulesetInfo{Kind: k, Hash: pinned[i], Name: name, Version: "old"}, nil).AnyTimes()
		f.rulesets.EXPECT().Default(gomock.Any(), k).Return(RulesetInfo{Kind: k, Hash: cur[i], Name: "File", Version: "new", Default: true}, nil).AnyTimes()
	}
}

func rulesState(status CampaignStatus, open, aftermaths int) CampaignRulesState {
	return CampaignRulesState{OrganiserID: 1, Name: "Siege", Status: status, Hashes: RulesetHashes{Vanilla: "v", BattleCompanies: "b", Glossary: "g"}, OpenMatches: open, PendingAftermaths: aftermaths}
}

func unitsOf(rs *RuleSet) CompanyState {
	return CompanyState{Company: CampaignCompany{ID: 5, Name: "Gondor", Faction: "Minas Tirith", OwnerID: 2}, Influence: 3, Gold: 4,
		Units: []UnitState{{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleWarrior}}}
}

func TestRulesUpdatePreview(t *testing.T) {
	ctx := context.Background()
	organiser, stranger, admin := User{ID: 1}, User{ID: 9}, User{ID: 8, Role: RoleAdmin}

	t.Run("only a manager may ask", func(t *testing.T) {
		f := newCampaignFixture(t)
		f.campaigns.EXPECT().RulesState(ctx, int64(3)).Return(rulesState(CampaignActive, 0, 0), nil).Times(2)
		if _, err := f.svc.RulesUpdatePreview(ctx, stranger, 3); !errors.Is(err, ErrRefused) {
			t.Fatalf("a stranger: %v", err)
		}
		f.hashes()
		f.expectFiles([3]string{"v", "b", "g"}, "File")
		f.campaigns.EXPECT().Companies(ctx, int64(3)).Return(nil, nil)
		if _, err := f.svc.RulesUpdatePreview(ctx, admin, 3); err != nil {
			t.Fatalf("an administrator: %v", err)
		}
	})

	t.Run("nothing differs: nothing to offer", func(t *testing.T) {
		f := newCampaignFixture(t)
		f.campaigns.EXPECT().RulesState(ctx, int64(3)).Return(CampaignRulesState{OrganiserID: 1, Name: "Siege", Status: CampaignActive, Hashes: RulesetHashes{Vanilla: "dv", BattleCompanies: "db", Glossary: "dg"}}, nil)
		f.expectFiles([3]string{"dv", "db", "dg"}, "File")
		u, err := f.svc.RulesUpdatePreview(ctx, organiser, 3)
		if err != nil || u.Differs || u.Clean() || len(u.Companies) != 0 {
			t.Fatalf("%+v %v", u, err)
		}
	})

	t.Run("a finished campaign, open matches and pending aftermaths block", func(t *testing.T) {
		for name, st := range map[string]CampaignRulesState{"ended": rulesState(CampaignFinished, 0, 0), "open match": rulesState(CampaignActive, 2, 0), "aftermath": rulesState(CampaignActive, 0, 1)} {
			f := newCampaignFixture(t)
			f.hashes()
			f.expectFiles([3]string{"v", "b", "g"}, "File")
			f.campaigns.EXPECT().RulesState(ctx, int64(3)).Return(st, nil)
			f.campaigns.EXPECT().Companies(ctx, int64(3)).Return(nil, nil)
			u, err := f.svc.RulesUpdatePreview(ctx, organiser, 3)
			if err != nil || len(u.Blockers) != 1 || u.Clean() || !u.Differs {
				t.Fatalf("%s: %+v %v", name, u, err)
			}
		}
	})

	t.Run("a company with something the new files lack blocks; one that works is clean", func(t *testing.T) {
		f := newCampaignFixture(t)
		f.hashes()
		f.expectFiles([3]string{"v", "b", "g"}, "File")
		rs := shippedRules(t)
		bad := unitsOf(rs)
		bad.Units = append(bad.Units, UnitState{ID: 2, Base: "Unit Of A Lost Age", Role: RoleWarrior}, UnitState{ID: 3, Base: "Warrior of Minas Tirith", Role: RoleWarrior, Bought: []string{"Sword Of Forgotten Lore"}})
		good := unitsOf(rs)
		good.Company.ID, good.Company.Name = 6, "Fine"
		f.campaigns.EXPECT().RulesState(ctx, int64(3)).Return(rulesState(CampaignActive, 0, 0), nil)
		f.campaigns.EXPECT().Companies(ctx, int64(3)).Return([]CampaignCompany{{ID: 5}, {ID: 6}}, nil)
		f.companies.EXPECT().Read(ctx, int64(3), int64(5)).Return(bad, nil)
		f.companies.EXPECT().Read(ctx, int64(3), int64(6)).Return(good, nil)
		u, err := f.svc.RulesUpdatePreview(ctx, organiser, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(u.Companies) != 2 || len(u.Companies[0].Problems) != 2 || len(u.Companies[1].Problems) != 0 || u.Clean() {
			t.Fatalf("companies = %+v", u.Companies)
		}
		if p := strings.Join(u.Companies[0].Problems, "|"); !strings.Contains(p, "Unit Of A Lost Age") || !strings.Contains(p, "Sword Of Forgotten Lore") {
			t.Errorf("problems = %s", p)
		}
		if u.Problems() != 2 {
			t.Errorf("Problems() = %d", u.Problems())
		}
	})
}

func TestUpdateRules(t *testing.T) {
	ctx := context.Background()
	organiser := User{ID: 1}
	run := func(t *testing.T, st CampaignRulesState, switchCustom bool, name string, companies ...CompanyState) (RulesetHashes, error) {
		f := newCampaignFixture(t)
		f.hashes()
		f.expectFiles([3]string{"v", "b", "g"}, name)
		var ids []CampaignCompany
		for _, c := range companies {
			ids = append(ids, CampaignCompany{ID: c.Company.ID})
			f.companies.EXPECT().Read(ctx, int64(3), c.Company.ID).Return(c, nil).AnyTimes()
		}
		f.campaigns.EXPECT().Companies(ctx, int64(3)).Return(ids, nil).AnyTimes()
		var moved RulesetHashes
		f.campaigns.EXPECT().SetRulesets(ctx, int64(3), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, fn func(CampaignRulesState) (RulesetHashes, error)) error {
			h, err := fn(st)
			moved = h
			return err
		})
		err := f.svc.UpdateRules(ctx, organiser, 3, switchCustom)
		return moved, err
	}
	rs := shippedRules(t)

	if h, err := run(t, rulesState(CampaignActive, 0, 0), false, "File", unitsOf(rs)); err != nil || h != (RulesetHashes{Vanilla: "dv", BattleCompanies: "db", Glossary: "dg"}) {
		t.Fatalf("a clean campaign moves to the defaults: %+v %v", h, err)
	}
	bad := unitsOf(rs)
	bad.Units[0].Base = "Unit Of A Lost Age"
	if _, err := run(t, rulesState(CampaignActive, 0, 0), false, "File", bad); !errors.Is(err, ErrRulesUpdateBlocked) {
		t.Fatalf("a company that does not work: %v", err)
	}
	if _, err := run(t, rulesState(CampaignActive, 1, 0), false, "File", unitsOf(rs)); !errors.Is(err, ErrRulesUpdateBlocked) {
		t.Fatalf("an open match: %v", err)
	}
	if _, err := run(t, rulesState(CampaignActive, 0, 0), false, "Another file", unitsOf(rs)); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "files of its own") {
		t.Fatalf("a custom file needs the explicit switch: %v", err)
	}
	if _, err := run(t, rulesState(CampaignActive, 0, 0), true, "Another file", unitsOf(rs)); err != nil {
		t.Fatalf("a custom file with the switch: %v", err)
	}

	t.Run("a stranger is refused inside the lock", func(t *testing.T) {
		f := newCampaignFixture(t)
		f.campaigns.EXPECT().SetRulesets(ctx, int64(3), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, fn func(CampaignRulesState) (RulesetHashes, error)) error {
			_, err := fn(rulesState(CampaignActive, 0, 0))
			return err
		})
		if err := f.svc.UpdateRules(ctx, User{ID: 9}, 3, false); !errors.Is(err, ErrRefused) {
			t.Fatalf("got %v", err)
		}
	})
}
