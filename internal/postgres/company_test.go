package postgres

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestCompanies(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Companies()
	owner, outsider := testUser(t, s, ctx, "own"), testUser(t, s, ctx, "non")
	camp, err := s.Campaigns().Create(ctx, owner, testNewCampaign(t, s, ctx, "Co"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Campaigns().Create(ctx, owner, testNewCampaign(t, s, ctx, "Other"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Create(ctx, camp.ID, outsider, core.NewCompany{Name: "X", Faction: "Y"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("non-member: %v", err)
	}
	if _, err := repo.Create(ctx, camp.ID, owner, core.NewCompany{Name: "X", Faction: "Y", Units: []core.UnitState{{ID: 5, Base: "B", Role: core.RoleHero}}}); !errors.Is(err, core.ErrInvalidCompany) {
		t.Fatalf("preset id: %v", err)
	}
	created, err := repo.Create(ctx, camp.ID, owner, core.NewCompany{Name: "Riders", Faction: "Rohan", Influence: 2, Gold: 7,
		Units: []core.UnitState{{Base: "Captain", Role: core.RoleLeader}, {Base: "Warrior", Role: core.RoleWarrior}, {Base: "Archer", Role: core.RoleWarrior}}})
	if err != nil || created.Units != 3 {
		t.Fatalf("create: %+v, %v", created, err)
	}
	if _, err := repo.Create(ctx, camp.ID, owner, core.NewCompany{Name: "Again", Faction: "Rohan"}); !errors.Is(err, core.ErrCompanyLimit) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := repo.Read(ctx, other.ID, created.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("wrong campaign: %v", err)
	}
	if err := repo.Update(ctx, camp.ID, created.ID+9999, func(*core.CompanyState) error { return nil }); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown company: %v", err)
	}

	st, err := repo.Read(ctx, camp.ID, created.ID)
	if err != nil || len(st.Units) != 3 || st.Gold != 7 || st.Influence != 2 || st.Units[0].Base != "Captain" || st.Units[2].Base != "Archer" {
		t.Fatalf("read: %+v, %v", st, err)
	}
	captain, warrior, archer := st.Units[0].ID, st.Units[1].ID, st.Units[2].ID

	// A full state round-trips: swap positions, change fields, add a unit, drop one into the cemetery.
	fallen := core.FallenUnit{Unit: core.CompanyUnit{ID: warrior, Name: "Eadric", Type: "Warrior", Role: core.RoleWarrior, Level: 1, XP: 6,
		Stats:     []core.Attribute{{Name: "Fight", Value: 4}},
		Equipment: []core.Equipment{{Name: "Spear"}}}, Fate: "Dead", Match: "Match 2"}
	want := core.UnitState{Name: "Hama", Base: "Captain", Role: core.RoleLeader, Kind: core.KindCavalry,
		Bonus: map[string]int{"might": 1}, Gear: []string{"Horse"}, Bought: []string{"Banner", "Shield"}, Dropped: []string{"Bow"},
		Path: "Leader", PathRules: 2, Actions: []string{"Heroic March"}, BoughtActions: []string{"Heroic Strike"},
		Spells: []core.PathSpell{{Name: "Blast", Casting: 4, Range: 12}}, Level: 2, XP: 12, Wanderer: "Wander", Points: 40,
		Progress: &core.Progress{Taken: 2, Bonus: map[string]int{"fight": 1}, Rules: []string{"Fell"}, Hero: true},
		Injuries: map[string]int{"arm wound": 1}, Injured: true, SentToHealers: true, Creature: "Warg",
		Pending: core.UnitPending{Level: &core.LevelRoll{Kind: "hero", Pairs: []core.HeroPair{{Sum: 7, Dice: []int{3, 4}, Usable: true}}, Offers: []core.LevelOffer{{Pair: 1, Roll: 7, Choices: []core.LevelChoice{{Key: "stat:fight:3", Label: "Fight"}}}}},
			Treat: &core.PendingTreat{Injury: "armWound", Result: core.TreatResult{Rolled: true, Roll: 3, Needed: 5, Pending: true, Boost: 2}}}}
	var newID int64
	err = repo.Update(ctx, camp.ID, created.ID, func(st *core.CompanyState) error {
		st.Company.Name = "Riders of the Mark"
		st.Influence, st.Gold, st.Rerolls = 1, 99, 3
		st.Pack = []string{"Rope", "Torch"}
		cap := want
		cap.ID = captain
		st.Units = []core.UnitState{{Base: "Scout", Role: core.RoleWarrior}, st.Units[2], cap} // new unit first; archer and captain swapped
		st.Cemetery = []core.FallenUnit{fallen}
		st.Order = []int64{archer, captain}
		st.Graveyard = []int64{warrior}
		st.Recruit = core.RecruitProgress{
			Roll:     &core.RecruitStep{Stage: "special", Roll: 4, Adjusted: 1, Paid: 4},
			Lower:    &core.LowerChoice{Prefix: "You rolled a 6: ", Paid: 3, Roll: 6, Stage: "chart", Entries: []core.LowerEntry{{Res: core.Reinforcement{Type: "unit", Unit: "Scout", Wargear: []string{"Bow"}}, Chart: "Reinforcements", Roll: "4–5"}}},
			Choice:   &core.OptionChoice{Unit: "Dwarf Warrior", Wargear: []string{"Axe"}},
			Officers: &core.OfficerStep{Unit: "Ranger of the North", Paid: 3, Roll: 4}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Read(ctx, camp.ID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Units) != 3 || got.Units[0].Base != "Scout" || got.Units[1].ID != archer || got.Units[2].ID != captain {
		t.Fatalf("units/order: %+v", got.Units)
	}
	newID = got.Units[0].ID
	if newID == 0 || newID == captain || newID == warrior || newID == archer {
		t.Fatalf("new unit id %d must be fresh", newID)
	}
	want.ID = captain
	if !reflect.DeepEqual(got.Units[2], want) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", got.Units[2], want)
	}
	if got.Company.Name != "Riders of the Mark" || got.Company.Owner != owner.DisplayName || got.Influence != 1 || got.Gold != 99 || got.Rerolls != 3 ||
		!reflect.DeepEqual(got.Pack, []string{"Rope", "Torch"}) || !reflect.DeepEqual(got.Order, []int64{archer, captain}) || !reflect.DeepEqual(got.Graveyard, []int64{warrior}) {
		t.Fatalf("company fields: %+v", got)
	}
	wantRecruit := core.RecruitProgress{
		Roll:     &core.RecruitStep{Stage: "special", Roll: 4, Adjusted: 1, Paid: 4},
		Lower:    &core.LowerChoice{Prefix: "You rolled a 6: ", Paid: 3, Roll: 6, Stage: "chart", Entries: []core.LowerEntry{{Res: core.Reinforcement{Type: "unit", Unit: "Scout", Wargear: []string{"Bow"}}, Chart: "Reinforcements", Roll: "4–5"}}},
		Choice:   &core.OptionChoice{Unit: "Dwarf Warrior", Wargear: []string{"Axe"}},
		Officers: &core.OfficerStep{Unit: "Ranger of the North", Paid: 3, Roll: 4}}
	if !reflect.DeepEqual(got.Recruit, wantRecruit) {
		t.Fatalf("recruit in progress:\n got %+v\nwant %+v", got.Recruit, wantRecruit)
	}
	if len(got.Cemetery) != 1 || !reflect.DeepEqual(got.Cemetery[0], fallen) {
		t.Fatalf("cemetery:\n got %+v\nwant %+v", got.Cemetery, fallen)
	}

	// Equipment is replaced, not accumulated, and the cemetery can be reordered and emptied.
	if err := repo.Update(ctx, camp.ID, created.ID, func(st *core.CompanyState) error {
		st.Units[2].Bought, st.Units[2].Dropped = nil, nil
		st.Cemetery[0].Fate = "Retired"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.Read(ctx, camp.ID, created.ID)
	if !reflect.DeepEqual(got.Units[2].Gear, []string{"Horse"}) || got.Units[2].Bought != nil || got.Cemetery[0].Fate != "Retired" {
		t.Fatalf("second update: %+v", got)
	}

	// A foreign unit id is refused and nothing is stored.
	if err := repo.Update(ctx, camp.ID, created.ID, func(st *core.CompanyState) error {
		st.Gold = 1
		st.Units = append(st.Units, core.UnitState{ID: warrior + 100000, Base: "X", Role: core.RoleWarrior})
		return nil
	}); err == nil {
		t.Fatal("foreign unit id accepted")
	}

	// fn errors roll back.
	boom := errors.New("boom")
	if err := repo.Update(ctx, camp.ID, created.ID, func(st *core.CompanyState) error {
		st.Gold = 1
		st.Units = nil
		st.Cemetery = nil
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("fn error: %v", err)
	}
	after, _ := repo.Read(ctx, camp.ID, created.ID)
	if after.Gold != 99 || len(after.Units) != 3 || len(after.Cemetery) != 1 {
		t.Fatalf("rolled back state changed: %+v", after)
	}

	// Two concurrent Updates serialise: no lost increment, and the second waits for the first.
	var wg sync.WaitGroup
	inside := make(chan struct{})
	release := make(chan struct{})
	errs := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- repo.Update(ctx, camp.ID, created.ID, func(st *core.CompanyState) error {
			close(inside)
			<-release
			st.Gold++
			return nil
		})
	}()
	<-inside
	secondStarted := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- repo.Update(ctx, camp.ID, created.ID, func(st *core.CompanyState) error {
			close(secondStarted)
			st.Gold++
			return nil
		})
	}()
	select {
	case <-secondStarted:
		t.Fatal("second Update ran while the first held the lock")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if final, _ := repo.Read(ctx, camp.ID, created.ID); final.Gold != 101 {
		t.Fatalf("lost update: gold %d", final.Gold)
	}
}

func TestCompanyPerksAreUsedInTheTransactionOfTheirChange(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Companies()
	owner := testUser(t, s, ctx, "perk")
	camp, err := s.Campaigns().Create(ctx, owner, testNewCampaign(t, s, ctx, "Perks"))
	if err != nil {
		t.Fatal(err)
	}
	co, err := repo.Create(ctx, camp.ID, owner, core.NewCompany{Name: "Riders", Faction: "Rohan", Units: []core.UnitState{{Base: "Captain", Role: core.RoleLeader}}})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.pool.QueryRow(ctx, `INSERT INTO company_perks (company_id, effect, name, applies) VALUES ($1, 'rendezvous', 'Rendezvous', 'firstReinforcementRollThisAftermath') RETURNING id`, co.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	st, err := repo.Read(ctx, camp.ID, co.ID)
	if err != nil || len(st.Perks) != 1 || st.Perks[0].ID != id || st.Perks[0].When != "firstReinforcementRollThisAftermath" || len(st.PerksUsed) != 0 {
		t.Fatalf("an unused perk is loaded: %+v %v", st.Perks, err)
	}
	boom := errors.New("later step fails")
	if err := repo.Update(ctx, camp.ID, co.ID, func(st *core.CompanyState) error { st.PerksUsed = []int64{id}; return boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if st, _ = repo.Read(ctx, camp.ID, co.ID); len(st.Perks) != 1 {
		t.Fatal("a failed change does not use the perk")
	}
	if err := repo.Update(ctx, camp.ID, co.ID, func(st *core.CompanyState) error { st.PerksUsed = []int64{id}; st.Influence++; return nil }); err != nil {
		t.Fatal(err)
	}
	if st, _ = repo.Read(ctx, camp.ID, co.ID); len(st.Perks) != 0 {
		t.Fatalf("used: %+v", st.Perks)
	}
	if err := repo.Update(ctx, camp.ID, co.ID, func(st *core.CompanyState) error { st.PerksUsed = []int64{id}; return nil }); !errors.Is(err, core.ErrRefused) {
		t.Errorf("a perk is used once: %v", err)
	}
}

// ReadRoster returns, for every company of a campaign, the same units, equipment and figures as Read, in a fixed number of queries.
func TestReadRosterMatchesRead(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Companies()
	camp, err := s.Campaigns().Create(ctx, testUser(t, s, ctx, "rr0"), testNewCampaign(t, s, ctx, "Roster"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i, name := range []string{"rr1", "rr2", "rr3"} {
		u := testUser(t, s, ctx, name)
		if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, u.ID); err != nil {
			t.Fatal(err)
		}
		c, err := repo.Create(ctx, camp.ID, u, core.NewCompany{Name: name, Faction: "Rohan", Gold: i,
			Units: []core.UnitState{{Base: "Captain", Role: core.RoleLeader, Gear: []string{"Horse", "Sword"}, Bought: []string{"Banner"}}, {Base: "Warrior", Role: core.RoleWarrior, Dropped: []string{"Bow"}}}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	all, err := repo.ReadRoster(ctx, camp.ID)
	if err != nil || len(all) != len(ids) {
		t.Fatalf("ReadRoster = %d companies, %v; want %d", len(all), err, len(ids))
	}
	for i, id := range ids {
		one, err := repo.Read(ctx, camp.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		if all[i].Company != one.Company || all[i].Gold != one.Gold || !reflect.DeepEqual(all[i].Units, one.Units) {
			t.Errorf("company %d: ReadRoster %+v differs from Read %+v", id, all[i], one)
		}
	}
}

// Read answers in one statement with JSON; load (used under the row lock by Update) in several. They must give the same state for every
// company the database holds, including pending aftermaths, perks, an open match and the cemetery.
func TestReadEqualsLoad(t *testing.T) {
	s, ctx := testStore(t)
	cs := s.Companies()
	rows, err := s.pool.Query(ctx, `SELECT campaign_id, id FROM companies ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var ids [][2]int64
	for rows.Next() {
		var p [2]int64
		if err := rows.Scan(&p[0], &p[1]); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p)
	}
	rows.Close()
	for _, p := range ids {
		got, err := cs.Read(ctx, p[0], p[1])
		if err != nil {
			t.Fatal(err)
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want, err := cs.load(ctx, tx, p[0], p[1], false)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("company %d: Read differs from load\n got %+v\nwant %+v", p[1], got, want)
		}
	}
	// the roster read gives the same companies and units, without what only Read has
	byCampaign := map[int64][]int64{}
	for _, p := range ids {
		byCampaign[p[0]] = append(byCampaign[p[0]], p[1])
	}
	for camp, list := range byCampaign {
		roster, err := cs.ReadRoster(ctx, camp)
		if err != nil || len(roster) != len(list) {
			t.Fatalf("roster of %d: %d companies, %v; want %d", camp, len(roster), err, len(list))
		}
		for i, id := range list {
			want, err := cs.Read(ctx, camp, id)
			if err != nil {
				t.Fatal(err)
			}
			want.Aftermaths, want.OpenMatch, want.Perks, want.Cemetery, want.Recruit = nil, nil, nil, nil, core.RecruitProgress{}
			if !reflect.DeepEqual(roster[i], want) {
				t.Fatalf("company %d: ReadRoster differs from Read\n got %+v\nwant %+v", id, roster[i], want)
			}
		}
	}
	t.Logf("%d companies compared", len(ids))
}

type writeTracer struct {
	mu  sync.Mutex
	sql []string
}

func (w *writeTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sql = append(w.sql, strings.Join(strings.Fields(d.SQL), " "))
	return ctx
}
func (w *writeTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// writes returns the statements since the last call that change a table, and forgets everything.
func (w *writeTracer) writes() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, q := range w.sql {
		if strings.HasPrefix(q, "UPDATE") || strings.HasPrefix(q, "DELETE") || strings.HasPrefix(q, "INSERT") {
			out = append(out, q)
		}
	}
	w.sql = nil
	return out
}

func (w *writeTracer) count(prefix string, writes []string) int {
	n := 0
	for _, q := range writes {
		if strings.HasPrefix(q, prefix) {
			n++
		}
	}
	return n
}

// Update writes back only the rows fn changed: a company of fourteen units used to cost an UPDATE, a DELETE of the equipment and an INSERT
// per item for every unit on every save. What is stored afterwards is exactly what fn left.
func TestUpdateWritesOnlyWhatChanged(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	tr := &writeTracer{}
	s, err := OpenWith(ctx, url, Options{Tracer: tr})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	repo := s.Companies()
	owner := testUser(t, s, ctx, "wr0")
	camp, err := s.Campaigns().Create(ctx, owner, testNewCampaign(t, s, ctx, "Writes"))
	if err != nil {
		t.Fatal(err)
	}
	co, err := repo.Create(ctx, camp.ID, owner, core.NewCompany{Name: "Writers", Faction: "Rohan", Gold: 3, Units: []core.UnitState{
		{Base: "Captain", Role: core.RoleLeader, Gear: []string{"Horse", "Sword"}, Bought: []string{"Banner"}},
		{Base: "Warrior", Role: core.RoleWarrior, Dropped: []string{"Bow"}},
		{Base: "Warrior", Role: core.RoleWarrior, Gear: []string{"Spear"}}}})
	if err != nil {
		t.Fatal(err)
	}
	tr.writes()
	update := func(fn func(*core.CompanyState)) []string {
		t.Helper()
		if err := repo.Update(ctx, camp.ID, co.ID, func(st *core.CompanyState) error { fn(st); return nil }); err != nil {
			t.Fatal(err)
		}
		return tr.writes()
	}
	read := func() core.CompanyState {
		t.Helper()
		st, err := repo.Read(ctx, camp.ID, co.ID)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	before := read()
	tr.writes()

	if w := update(func(*core.CompanyState) {}); len(w) != 0 {
		t.Errorf("an update that changes nothing writes nothing: %v", w)
	}
	if !reflect.DeepEqual(read().Units, before.Units) {
		t.Error("units changed by a no-op update")
	}

	w := update(func(st *core.CompanyState) { st.Gold++ })
	if tr.count("UPDATE companies", w) != 1 || len(w) != 1 {
		t.Errorf("a new purse is one UPDATE of the company: %v", w)
	}

	w = update(func(st *core.CompanyState) { st.Units[1].XP += 5; st.Units[1].Bought = []string{"Shield", "Helm"} })
	if tr.count("UPDATE company_units", w) != 1 || tr.count("DELETE FROM unit_equipment", w) != 1 || tr.count("INSERT INTO unit_equipment", w) != 3 || tr.count("UPDATE companies", w) != 0 {
		t.Errorf("one unit changed, one unit written (3 items of equipment): %v", w)
	}
	got := read()
	if got.Units[1].XP != before.Units[1].XP+5 || !slices.Equal(got.Units[1].Bought, []string{"Shield", "Helm"}) || !slices.Equal(got.Units[1].Dropped, []string{"Bow"}) {
		t.Errorf("the changed unit: %+v", got.Units[1])
	}
	if !reflect.DeepEqual(got.Units[0], before.Units[0]) || !reflect.DeepEqual(got.Units[2], before.Units[2]) {
		t.Error("the other units must be as they were")
	}

	// reorder: positions are part of a unit's row
	w = update(func(st *core.CompanyState) { st.Units[0], st.Units[2] = st.Units[2], st.Units[0] })
	if tr.count("UPDATE company_units", w) != 2 {
		t.Errorf("two units moved: %v", w)
	}
	if got := read(); got.Units[0].ID != before.Units[2].ID || got.Units[2].ID != before.Units[0].ID {
		t.Errorf("order after the swap: %d %d %d", got.Units[0].ID, got.Units[1].ID, got.Units[2].ID)
	}

	// a unit added, then one removed into the cemetery
	w = update(func(st *core.CompanyState) {
		st.Units = append(st.Units, core.UnitState{Base: "Rider", Role: core.RoleWarrior, Gear: []string{"Horse"}})
	})
	if tr.count("INSERT INTO company_units", w) != 1 || tr.count("UPDATE company_units", w) != 0 {
		t.Errorf("one unit inserted, none rewritten: %v", w)
	}
	cur := read()
	if len(cur.Units) != 4 || !slices.Equal(cur.Units[3].Gear, []string{"Horse"}) {
		t.Fatalf("units after the insert: %+v", cur.Units)
	}
	fallen := cur.Units[1]
	w = update(func(st *core.CompanyState) {
		st.Units = slices.Delete(slices.Clone(st.Units), 1, 2)
		st.Cemetery = append(st.Cemetery, core.FallenUnit{Unit: core.CompanyUnit{ID: fallen.ID, Name: fallen.Base}, Fate: "killed", Match: "Match 1"})
	})
	if tr.count("DELETE FROM company_units", w) != 1 || tr.count("INSERT INTO fallen_units", w) != 1 {
		t.Errorf("one unit removed, one grave written: %v", w)
	}
	cur = read()
	if len(cur.Units) != 3 || len(cur.Cemetery) != 1 || cur.Cemetery[0].Unit.ID != fallen.ID {
		t.Fatalf("after the burial: %d units, cemetery %+v", len(cur.Units), cur.Cemetery)
	}
	if w := update(func(*core.CompanyState) {}); len(w) != 0 {
		t.Errorf("nothing changed again, nothing written: %v", w)
	}
	w = update(func(st *core.CompanyState) { st.Cemetery[0].Fate = "lost" })
	if tr.count("INSERT INTO fallen_units", w) != 1 || len(w) != 1 || read().Cemetery[0].Fate != "lost" {
		t.Errorf("a changed grave is one upsert: %v", w)
	}
	// a company with no units reads as one company without units
	other := testUser(t, s, ctx, "wr1")
	if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	empty, err := repo.Create(ctx, camp.ID, other, core.NewCompany{Name: "Nobody", Faction: "Rohan"})
	if err != nil {
		t.Fatal(err)
	}
	roster, err := repo.ReadRoster(ctx, camp.ID)
	if err != nil || len(roster) != 2 {
		t.Fatalf("roster: %d companies, %v", len(roster), err)
	}
	for _, st := range roster {
		if st.Company.ID == empty.ID && len(st.Units) != 0 {
			t.Errorf("a company without units: %+v", st.Units)
		}
		if st.Company.ID == co.ID && len(st.Units) != 3 {
			t.Errorf("the company with units: %d", len(st.Units))
		}
	}
}
