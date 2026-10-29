package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Several companies per player (docs/multi-company-spec.md, sections 2 and 3).

// seatedCampaign makes a campaign with the given cap (CompaniesPerPlayer) and the players as members.
func seatedCampaign(t *testing.T, s *Store, capacity int, players ...core.User) (core.CampaignSummary, core.User) {
	t.Helper()
	ctx := t.Context()
	org := testUser(t, s, ctx, "scorg")
	in := testNewCampaign(t, s, ctx, "Several")
	in.Settings.CompaniesPerPlayer = capacity
	camp, err := s.Campaigns().Create(ctx, org, in)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range players {
		if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	return camp, org
}

// scratchStore opens a database of its own: these tests leave a player with several companies in a campaign, which the shared test
// database must not keep (the migration tests of the package take it down to older versions, and 00024 cannot go down then).
func scratchStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := t.Context()
	s, err := Open(ctx, scratchDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, ctx
}

func newCo(name string, capacity int) core.NewCompany {
	return core.NewCompany{Name: name, Faction: "Rohan", Cap: capacity,
		Units: []core.UnitState{{Base: "Captain", Role: core.RoleLeader}, {Base: "Warrior", Role: core.RoleWarrior}}}
}

func TestSeveralCompaniesMigrationUpAndDown(t *testing.T) {
	dsn := scratchDatabase(t)
	ctx := t.Context()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname = 'companies_campaign_owner_key'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("the unique constraint is gone: %d %v", n, err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE indexname = 'companies_owner_idx'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("the owner index exists: %d %v", n, err)
	}
	owner := testUser(t, s, ctx, "mig")
	camp, _ := seatedCampaign(t, s, 2, owner)
	a, err := s.Companies().Create(ctx, camp.ID, owner, newCo("First", 2))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Companies().Create(ctx, camp.ID, owner, newCo("Second", 2))
	if err != nil {
		t.Fatal(err)
	}

	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	goose.SetBaseFS(migrations)
	// down refuses while a player has two companies: re-adding the unique constraint cannot work, and that is the right answer
	if err := goose.DownToContext(ctx, db, "migrations", 23); err == nil {
		t.Fatal("down must refuse while a player owns two companies in one campaign")
	}
	if v, err := goose.GetDBVersionContext(ctx, db); err != nil || v != 24 {
		t.Fatalf("the refused down changes nothing: version %d, %v", v, err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM companies WHERE id = $1`, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownToContext(ctx, db, "migrations", 23); err != nil {
		t.Fatalf("down with one company each: %v", err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname = 'companies_campaign_owner_key'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("down brings the constraint back: %d %v", n, err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Companies().Read(ctx, camp.ID, a.ID); err != nil || got.Company.Name != "First" {
		t.Fatalf("the data survives: %+v %v", got.Company, err)
	}
}

// Two simultaneous creates never exceed the cap: with cap 2 and three creates racing, exactly one is refused.
func TestCreateCompanyCapIsAtomic(t *testing.T) {
	s, ctx := scratchStore(t)
	owner := testUser(t, s, ctx, "race")
	camp, _ := seatedCampaign(t, s, 2, owner)
	errs := make([]error, 3)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.Companies().Create(ctx, camp.ID, owner, newCo([]string{"A", "B", "C"}[i], 2))
		}()
	}
	wg.Wait()
	refused := 0
	for _, err := range errs {
		switch {
		case err == nil:
		case errors.Is(err, core.ErrCompanyLimit):
			refused++
		default:
			t.Errorf("unexpected: %v", err)
		}
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM companies WHERE campaign_id = $1 AND owner_id = $2`, camp.ID, owner.ID).Scan(&n); err != nil || n != 2 || refused != 1 {
		t.Fatalf("%d companies stored, %d refused (want 2 and 1): %v", n, refused, errs)
	}
}

func TestCreateCompanyNamesAreUniquePerOwner(t *testing.T) {
	s, ctx := scratchStore(t)
	ana, bo := testUser(t, s, ctx, "ana"), testUser(t, s, ctx, "bo")
	camp, _ := seatedCampaign(t, s, 3, ana, bo)
	repo := s.Companies()
	if _, err := repo.Create(ctx, camp.ID, ana, newCo("Rangers", 3)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, camp.ID, ana, newCo("rANGERS", 3)); !errors.Is(err, core.ErrCompanyNameTaken) || !errors.Is(err, core.ErrInvalidCompany) {
		t.Errorf("same owner, same name ignoring case: %v", err)
	}
	if _, err := repo.Create(ctx, camp.ID, bo, newCo("Rangers", 3)); err != nil {
		t.Errorf("another owner may share the name: %v", err)
	}
	if _, err := repo.Create(ctx, camp.ID, ana, newCo("Riders", 3)); err != nil {
		t.Errorf("a different name: %v", err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM companies WHERE campaign_id = $1`, camp.ID).Scan(&n); err != nil || n != 3 {
		t.Errorf("refusals store nothing (units neither): %d companies, %v", n, err)
	}
}

// List, Get and Members keep one row per campaign / member however many companies there are, with the companies oldest first.
func TestQueriesReturnOneRowPerCampaignAndMember(t *testing.T) {
	s, ctx := scratchStore(t)
	ana, bo := testUser(t, s, ctx, "qa"), testUser(t, s, ctx, "qb")
	camp, org := seatedCampaign(t, s, 3, ana, bo)
	second, _ := seatedCampaign(t, s, 1, ana)
	repo, cos := s.Campaigns(), s.Companies()
	var made []core.CampaignCompany
	for _, name := range []string{"Zeta", "Alpha", "Mid"} {
		c, err := cos.Create(ctx, camp.ID, ana, newCo(name, 3))
		if err != nil {
			t.Fatal(err)
		}
		made = append(made, c)
	}
	one, err := cos.Create(ctx, camp.ID, bo, newCo("Solo", 3))
	if err != nil {
		t.Fatal(err)
	}

	list, err := repo.List(ctx, ana.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, g := range list {
		switch g.ID {
		case camp.ID:
			seen++
			if len(g.Companies) != 3 || g.Companies[0].ID != made[0].ID || g.Companies[1].ID != made[1].ID || g.Companies[2].ID != made[2].ID || g.Companies[0].Units != 2 {
				t.Errorf("List: ana's companies oldest first: %+v", g.Companies)
			}
		case second.ID:
			seen++
			if len(g.Companies) != 0 {
				t.Errorf("List: no company in the second campaign: %+v", g.Companies)
			}
		}
	}
	if seen != 2 {
		t.Errorf("List shows each campaign once: saw %d of the two in %d rows", seen, len(list))
	}
	ids := map[int64]int{}
	for _, g := range list {
		ids[g.ID]++
	}
	if ids[camp.ID] != 1 || ids[second.ID] != 1 {
		t.Errorf("one row per campaign: %v", ids)
	}

	d, err := repo.Get(ctx, ana.ID, camp.ID)
	if err != nil || len(d.Companies) != 3 || d.Companies[2].Name != "Mid" || d.Settings.CompanyCap() != 3 {
		t.Fatalf("Get: %+v, %v", d.Companies, err)
	}
	if d, err = repo.Get(ctx, bo.ID, camp.ID); err != nil || len(d.Companies) != 1 || d.Companies[0].ID != one.ID {
		t.Fatalf("Get for the other player: %+v, %v", d.Companies, err)
	}

	members, err := repo.Members(ctx, camp.ID)
	if err != nil || len(members) != 3 {
		t.Fatalf("Members: one row per member (organiser, ana, bo): %+v, %v", members, err)
	}
	if !members[0].Organiser || members[0].UserID != org.ID || len(members[0].Companies) != 0 {
		t.Errorf("the organiser has no company: %+v", members[0])
	}
	for _, m := range members {
		switch m.UserID {
		case ana.ID:
			if len(m.Companies) != 3 || m.Companies[0].Name != "Zeta" || m.Companies[1].Name != "Alpha" {
				t.Errorf("ana's companies oldest first: %+v", m.Companies)
			}
		case bo.ID:
			if len(m.Companies) != 1 || m.Companies[0].Name != "Solo" {
				t.Errorf("bo: %+v", m.Companies)
			}
		}
	}
	if all, err := repo.Companies(ctx, camp.ID); err != nil || len(all) != 4 {
		t.Errorf("Companies lists every company: %d, %v", len(all), err)
	}
}

// The rules through the services on PostgreSQL: cap 1 is the default, cap 2 allows a second company, lowering keeps the existing ones
// and blocks new ones, and the new limit is seen at once (the remembered settings are dropped).
func TestCompaniesPerPlayerThroughTheServices(t *testing.T) {
	s, ctx := scratchStore(t)
	rules := core.NewRules(s.Rulesets(), nil)
	org := testUser(t, s, ctx, "cap")
	org.CanCreateCampaigns = true
	camps := core.NewCampaigns(s.Campaigns(), s.Companies(), s.Rulesets(), rules, nil)
	camp, err := camps.Create(ctx, org, core.CampaignInput{Name: "Cap",
		Vanilla:         shippedFile(t, s, core.KindVanilla, "vanilla/data2024.json"),
		BattleCompanies: shippedFile(t, s, core.KindBattleCompanies, "battle_companies/battle_companies.json"),
		Glossary:        shippedFile(t, s, core.KindGlossary, "glossary/mesbg_glossary.json")})
	if err != nil {
		t.Fatal(err)
	}
	cos := core.NewCompanyService(s.Campaigns(), s.Companies(), rules)
	create := func(name string) error {
		_, err := cos.CreateCompany(ctx, org, camp.ID, core.CompanyInput{Name: name, Faction: "Minas Tirith"})
		return err
	}
	if err := create("One"); err != nil {
		t.Fatal(err)
	}
	if err := create("Two"); !errors.Is(err, core.ErrCompanyLimit) {
		t.Fatalf("the default cap is 1: %v", err)
	}
	if err := camps.SetCompaniesPerPlayer(ctx, org, camp.ID, 2); err != nil {
		t.Fatal(err)
	}
	if st, err := camps.Settings(ctx, camp.ID); err != nil || st.CompanyCap() != 2 {
		t.Fatalf("the new cap is read at once: %+v %v", st, err)
	}
	if err := create("Two"); err != nil {
		t.Fatalf("cap 2 allows a second company: %v", err)
	}
	if err := create("Three"); !errors.Is(err, core.ErrCompanyLimit) {
		t.Fatalf("cap 2 refuses a third: %v", err)
	}
	if err := camps.SetCompaniesPerPlayer(ctx, org, camp.ID, 4); err != nil {
		t.Fatal(err)
	}
	if err := create("Three"); err != nil {
		t.Fatal(err)
	}
	if err := camps.SetCompaniesPerPlayer(ctx, org, camp.ID, 1); err != nil { // lowered below the count of three
		t.Fatal(err)
	}
	d, err := camps.Get(ctx, org.ID, camp.ID)
	if err != nil || len(d.Companies) != 3 || d.CanCreateCompany() {
		t.Fatalf("lowering keeps the three companies and blocks new ones: %d, %v", len(d.Companies), err)
	}
	if err := create("Four"); !errors.Is(err, core.ErrCompanyLimit) {
		t.Fatalf("lowered cap: %v", err)
	}
	if err := create("two"); !errors.Is(err, core.ErrCompanyLimit) && !errors.Is(err, core.ErrInvalidCompany) {
		t.Fatalf("still refused: %v", err)
	}
	other := testUser(t, s, ctx, "capother")
	if err := camps.SetCompaniesPerPlayer(ctx, other, camp.ID, 3); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("only the organiser: %v", err)
	}
}

// Two companies of one owner fight each other: the match is allowed, and both companies get an aftermath that the owner finishes
// one by one, each locking only its own company.
func TestAnOwnerOfTwoCompaniesFightsThemselves(t *testing.T) {
	s, _ := scratchStore(t)
	w := newAftermathWorldOn(t, s)
	ctx := t.Context()
	id := w.camp.ID
	if err := w.s.Campaigns().UpdateSettings(ctx, id, func(_ int64, _ core.CampaignStatus, cur core.CampaignSettings) (core.CampaignSettings, error) {
		cur.CompaniesPerPlayer = 2
		return cur, nil
	}); err != nil {
		t.Fatal(err)
	}
	rules := core.NewRules(w.s.Rulesets(), nil)
	cos := core.NewCompanyService(w.s.Campaigns(), w.s.Companies(), rules)
	twin, err := cos.CreateCompany(ctx, w.p1, id, core.CompanyInput{Name: "One again", Faction: "Minas Tirith"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.s.Companies().Update(ctx, id, twin.ID, func(st *core.CompanyState) error {
		for i := range st.Units {
			if st.Units[i].Role != core.RoleWarrior && st.Units[i].Path == "" {
				st.Units[i].Path = "Path of the Soldier"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m, err := w.matches.CreateMatch(ctx, w.org, id, core.NewMatch{Sides: []core.NewSide{{Name: "A", Companies: []int64{w.co1.ID}}, {Name: "B", Companies: []int64{twin.ID}}}})
	if err != nil {
		t.Fatalf("a player's two companies on opposite sides: %v", err)
	}
	if sp := m.SamePlayers(); len(sp) != 1 || sp[0].OwnerID != w.p1.ID || !sp[0].AcrossSides {
		t.Errorf("the note for the web layer: %+v", sp)
	}
	if m, err = w.matches.SetScenario(ctx, w.org, id, m.ID, m.Revision, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Scavenge the Battlefield"}); err != nil {
		t.Fatal(err)
	}
	three, one := 3, 1
	if m, err = w.matches.SaveResult(ctx, w.org, id, m.ID, m.Revision, core.MatchResult{Decided: true, Winner: 1, Sides: []core.SideResult{{VictoryPoints: &three}, {VictoryPoints: &one}}}); err != nil {
		t.Fatal(err)
	}
	// one owner, two companies: proposing confirms for the proposer's companies, and the single owner's confirmation ends the match
	if m, err = w.matches.ProposeEnd(ctx, w.p1, id, m.ID, m.Revision); err != nil || !m.Ended {
		t.Fatalf("one owner ends it at once, both companies confirmed: ended=%v %v", m.Ended, err)
	}
	refs, err := w.am.AftermathStatuses(ctx, id, m.ID)
	if err != nil || len(refs) != 2 {
		t.Fatalf("both companies have an aftermath: %+v %v", refs, err)
	}
	for _, c := range []int64{w.co1.ID, twin.ID} {
		a, err := w.am.Aftermath(ctx, w.p1, id, m.ID, c)
		if err != nil || a.Access != core.AftermathEdit {
			t.Fatalf("company %d: the owner edits its aftermath: %+v %v", c, a, err)
		}
		if p, err := w.am.PendingAftermaths(ctx, id, c); err != nil || len(p) != 1 {
			t.Fatalf("company %d is locked by its own aftermath: %+v %v", c, p, err)
		}
	}
	if _, err := w.am.WaiveAftermath(ctx, w.org, id, m.ID, w.co1.ID, "test: waived"); err != nil {
		t.Fatal(err)
	}
	if p, _ := w.am.PendingAftermaths(ctx, id, w.co1.ID); len(p) != 0 {
		t.Errorf("the first company is free: %+v", p)
	}
	if p, _ := w.am.PendingAftermaths(ctx, id, twin.ID); len(p) != 1 {
		t.Errorf("the other still waits for its aftermath: %+v", p)
	}
}
