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
		Settings: core.CampaignSettings{ManualRolls: true, ShakeTheTable: true}, Rounds: 5, DefaultLimit: 10}
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
	if list, _ = repo.List(ctx, owner.ID, false); len(list[1].Companies) != 1 || list[1].Companies[0].ID != co.ID || len(list[0].Companies) != 0 {
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
	if d, _ = repo.Get(ctx, owner.ID, second.ID); len(d.Companies) != 1 || d.Companies[0].ID != co.ID {
		t.Fatalf("detail company: %+v", d.Companies)
	}
}

// The limit of unfinished campaigns an account organises: the default counts for an account with none of its own, a limit of its own wins,
// ending a campaign frees a place, and a hand-over to an account that is full is refused.
func TestCampaignLimit(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Campaigns()
	org, other := testUser(t, s, ctx, "lim-org"), testUser(t, s, ctx, "lim-other")
	mk := func(name string, u core.User, def int) (core.CampaignSummary, error) {
		nc := testNewCampaign(t, s, ctx, name)
		nc.DefaultLimit = def
		return repo.Create(ctx, u, nc)
	}
	first, err := mk("Limit 1", org, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mk("Limit 2", org, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := mk("Limit 3", org, 2); !errors.Is(err, core.ErrCampaignLimit) {
		t.Fatalf("third campaign over a default of 2: %v", err)
	}
	if q, err := repo.Quota(ctx, org.ID, 2); err != nil || q.Open != 2 || q.Limit != 2 || q.Own || !q.Full() {
		t.Fatalf("quota = %+v, %v", q, err)
	}
	// a pause keeps the place; ending frees it
	if err := repo.SetStatus(ctx, first.ID, func(int64, string, core.CampaignStatus) (core.CampaignStatus, error) { return core.CampaignPaused, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := mk("Limit 3", org, 2); !errors.Is(err, core.ErrCampaignLimit) {
		t.Fatalf("a paused campaign still counts: %v", err)
	}
	if err := repo.SetStatus(ctx, first.ID, func(int64, string, core.CampaignStatus) (core.CampaignStatus, error) {
		return core.CampaignFinished, nil
	}); err != nil {
		t.Fatal(err)
	}
	third, err := mk("Limit 3", org, 2)
	if err != nil {
		t.Fatalf("an ended campaign frees a place: %v", err)
	}
	// an own limit wins over the default, both ways
	one := 1
	if err := s.SetMaxCampaigns(ctx, org.ID, &one, time.Now(), org.ID); err != nil {
		t.Fatal(err)
	}
	if q, _ := repo.Quota(ctx, org.ID, 50); q.Limit != 1 || !q.Own || q.Open != 2 {
		t.Fatalf("own limit 1 with 2 open: %+v (lowering never touches what exists)", q)
	}
	if _, err := mk("Limit 4", org, 50); !errors.Is(err, core.ErrCampaignLimit) {
		t.Fatalf("own limit 1: %v", err)
	}
	if err := s.SetMaxCampaigns(ctx, org.ID, nil, time.Now(), org.ID); err != nil {
		t.Fatal(err)
	}
	if q, _ := repo.Quota(ctx, org.ID, 50); q.Limit != 50 || q.Own {
		t.Fatalf("back to the default: %+v", q)
	}
	// the audit trail records both changes, once each
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action = 'campaign_limit.changed' AND subject_user_id = $1`, org.ID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit entries = %d, %v", n, err)
	}
	if err := s.SetMaxCampaigns(ctx, org.ID, nil, time.Now(), org.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action = 'campaign_limit.changed' AND subject_user_id = $1`, org.ID).Scan(&n); err != nil || n != 2 {
		t.Fatalf("an unchanged limit is not recorded: %d, %v", n, err)
	}
	// a hand-over to an account at its limit is refused; to one with room it works
	zero := 0
	if err := s.SetMaxCampaigns(ctx, other.ID, &zero, time.Now(), org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, third.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	yes := func(int64) error { return nil }
	if err := repo.HandOver(ctx, third.ID, other.ID, 10, yes); !errors.Is(err, core.ErrCampaignLimit) {
		t.Fatalf("hand-over to an account with limit 0: %v", err)
	}
	if err := s.SetMaxCampaigns(ctx, other.ID, nil, time.Now(), org.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.HandOver(ctx, third.ID, other.ID, 10, yes); err != nil {
		t.Fatalf("hand-over with room: %v", err)
	}
	if err := s.SetMaxCampaigns(ctx, -1, &zero, time.Now(), org.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
}

// A campaign's rules files change under a lock with the state read under it; the remembered hashes follow, a file that is not stored is
// ErrNotFound, and an error of the callback changes nothing.
func TestCampaignSetRulesets(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Campaigns()
	org := testUser(t, s, ctx, "rules-org")
	c, err := repo.Create(ctx, org, testNewCampaign(t, s, ctx, "Rules update"))
	if err != nil {
		t.Fatal(err)
	}
	was, _, err := repo.Hashes(ctx, c.ID) // remembered
	if err != nil {
		t.Fatal(err)
	}
	st, err := repo.RulesState(ctx, c.ID)
	if err != nil || st.OrganiserID != org.ID || st.Name != "Rules update" || st.Status != core.CampaignActive || st.Hashes != was || st.OpenMatches != 0 || st.PendingAftermaths != 0 {
		t.Fatalf("state = %+v, %v", st, err)
	}
	if _, err := repo.RulesState(ctx, -1); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown campaign: %v", err)
	}
	other := testNewCampaign(t, s, ctx, "Other files")
	next := core.RulesetHashes{Vanilla: other.Vanilla, BattleCompanies: other.BattleCompanies, Glossary: other.Glossary}

	denied := errors.New("denied")
	if err := repo.SetRulesets(ctx, c.ID, func(core.CampaignRulesState) (core.RulesetHashes, error) { return next, denied }); err != denied {
		t.Fatalf("a refusing callback: %v", err)
	}
	if got, _, _ := repo.Hashes(ctx, c.ID); got != was {
		t.Fatalf("a refused change moved the files: %+v", got)
	}
	var seen core.CampaignRulesState
	if err := repo.SetRulesets(ctx, c.ID, func(st core.CampaignRulesState) (core.RulesetHashes, error) { seen = st; return next, nil }); err != nil {
		t.Fatal(err)
	}
	if seen.OrganiserID != org.ID || seen.Hashes != was {
		t.Fatalf("the callback saw %+v", seen)
	}
	if got, _, _ := repo.Hashes(ctx, c.ID); got != next {
		t.Fatalf("the remembered hashes did not follow: %+v", got)
	}
	if err := repo.SetRulesets(ctx, c.ID, func(core.CampaignRulesState) (core.RulesetHashes, error) {
		return core.RulesetHashes{Vanilla: "nope", BattleCompanies: next.BattleCompanies, Glossary: next.Glossary}, nil
	}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a file that is not stored: %v", err)
	}
	if err := repo.SetRulesets(ctx, -1, func(st core.CampaignRulesState) (core.RulesetHashes, error) { return st.Hashes, nil }); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown campaign: %v", err)
	}
}
