package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// testStore opens the test database or skips the test.
func testStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, ctx
}

func testUser(t *testing.T, s *Store, ctx context.Context, name string) core.User {
	t.Helper()
	u, err := s.CreateUser(ctx, fmt.Sprintf("%s-%d@example.com", name, time.Now().UnixNano()), name, "hash")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// testNewCampaign stores three fresh rules files and returns a campaign using them.
func testNewCampaign(t *testing.T, s *Store, ctx context.Context, name string) core.NewCampaign {
	t.Helper()
	tag := fmt.Sprint(time.Now().UnixNano())
	var hashes [3]string
	for i, kind := range []core.RulesetKind{core.KindVanilla, core.KindBattleCompanies, core.KindGlossary} {
		r := testRuleset(kind, "Camp "+tag, "1", fmt.Sprintf(`{"k":%d,"t":"%s"}`, i, tag))
		if err := s.Rulesets().Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
		hashes[i] = r.Hash
	}
	return core.NewCampaign{Name: name, Description: "d", Vanilla: hashes[0], BattleCompanies: hashes[1], Glossary: hashes[2],
		Settings: core.CampaignSettings{ManualRolls: true, ShakeTheTable: true}, Rounds: 5}
}

func TestCampaigns(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Campaigns()
	owner, other := testUser(t, s, ctx, "org"), testUser(t, s, ctx, "out")

	in := testNewCampaign(t, s, ctx, "First")
	first, err := repo.Create(ctx, owner, in)
	if err != nil || first.ID == 0 || first.Status != core.CampaignActive || first.Rounds != 5 {
		t.Fatalf("create: %+v, %v", first, err)
	}
	in2 := testNewCampaign(t, s, ctx, "Second")
	second, err := repo.Create(ctx, owner, in2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE campaigns SET status = 'finished' WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	third, err := repo.Create(ctx, owner, testNewCampaign(t, s, ctx, "Third"))
	if err != nil {
		t.Fatal(err)
	}

	bad := in
	bad.Glossary = in.Vanilla // right file, wrong kind
	if _, err := repo.Create(ctx, owner, bad); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("wrong kind: %v", err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM campaigns WHERE created_by = $1`, owner.ID).Scan(&n); err != nil || n != 3 {
		t.Fatalf("a failed create must leave nothing: %d, %v", n, err)
	}

	list, err := repo.List(ctx, owner.ID, true)
	if err != nil || len(list) != 3 || list[0].ID != third.ID || list[1].ID != second.ID || list[2].ID != first.ID {
		t.Fatalf("list with finished (unfinished newest first, then finished): %+v, %v", list, err)
	}
	if list, _ = repo.List(ctx, owner.ID, false); len(list) != 2 {
		t.Fatalf("list without finished: %+v", list)
	}
	if list, _ = repo.List(ctx, other.ID, true); len(list) != 0 {
		t.Fatalf("a stranger sees nothing: %+v", list)
	}

	d, err := repo.Get(ctx, owner.ID, second.ID)
	if err != nil || d.Organiser != "org" || d.OrganiserID != owner.ID || d.Players != 1 || !d.Settings.ManualRolls || !d.Settings.ShakeTheTable ||
		d.Settings.CustomOnTheirOwnPath || len(d.Rulesets) != 3 || d.Rulesets[0].Hash != in2.Vanilla || d.Rulesets[2].Kind != core.KindGlossary {
		t.Fatalf("get: %+v, %v", d, err)
	}
	if _, err := repo.Get(ctx, other.ID, second.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("non-member: %v", err)
	}
	h, set, err := repo.Hashes(ctx, second.ID)
	if err != nil || h != (core.RulesetHashes{Vanilla: in2.Vanilla, BattleCompanies: in2.BattleCompanies, Glossary: in2.Glossary}) || !reflect.DeepEqual(set, in2.Settings) {
		t.Fatalf("hashes: %+v %+v %v", h, set, err)
	}
	if _, _, err := repo.Hashes(ctx, 0); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}

	// Companies: the figures come from the rows.
	co, err := s.Companies().Create(ctx, second.ID, owner, core.NewCompany{Name: "Wolves", Faction: "Rohan", Gold: 3,
		Units: []core.UnitState{{Base: "Warrior of Rohan", Role: core.RoleWarrior}, {Base: "Captain", Role: core.RoleLeader}}})
	if err != nil || co.Units != 2 || co.Owner != "org" || co.OwnerID != owner.ID {
		t.Fatalf("company: %+v, %v", co, err)
	}
	if list, _ = repo.List(ctx, owner.ID, false); list[1].Company == nil || list[1].Company.ID != co.ID || list[0].Company != nil {
		t.Fatalf("list company: %+v", list)
	}
	all, err := repo.Companies(ctx, second.ID)
	if err != nil || len(all) != 1 || all[0] != co {
		t.Fatalf("companies: %+v, %v", all, err)
	}
	if got, err := repo.Company(ctx, second.ID, co.ID); err != nil || got != co {
		t.Fatalf("company: %+v, %v", got, err)
	}
	if _, err := repo.Company(ctx, first.ID, co.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("company of another campaign: %v", err)
	}
	if d, _ = repo.Get(ctx, owner.ID, second.ID); d.Company == nil || d.Company.ID != co.ID {
		t.Fatalf("detail company: %+v", d.Company)
	}
}
