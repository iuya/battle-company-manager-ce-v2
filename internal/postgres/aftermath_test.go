package postgres

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

type aftermathWorld struct {
	s                      *Store
	camp                   core.CampaignSummary
	org, p1, p2, p3, admin core.User
	co1, co2               core.CampaignCompany
	matches                *core.Matches
	am                     *core.AftermathService
	companies              *core.CompanyService
	armoury                *core.ArmouryService
}

func newAftermathWorld(t *testing.T) *aftermathWorld {
	t.Helper()
	s, _ := testStore(t)
	return newAftermathWorldOn(t, s)
}

// newAftermathWorldOn is newAftermathWorld on the store given (a scratch database, for the tests that leave a player with several companies).
func newAftermathWorldOn(t *testing.T, s *Store) *aftermathWorld {
	t.Helper()
	ctx := t.Context()
	w := &aftermathWorld{s: s}
	w.org, w.p1, w.p2, w.p3 = testUser(t, s, ctx, "aorg"), testUser(t, s, ctx, "aone"), testUser(t, s, ctx, "atwo"), testUser(t, s, ctx, "athree")
	w.admin = testUser(t, s, ctx, "aadm")
	w.admin.Role = core.RoleAdmin
	w.org.CanCreateCampaigns = true
	rules := core.NewRules(s.Rulesets(), nil)
	camps := core.NewCampaigns(s.Campaigns(), s.Companies(), s.Rulesets(), rules, nil)
	var err error
	w.camp, err = camps.Create(ctx, w.org, core.CampaignInput{Name: "Aftermath",
		Vanilla:         shippedFile(t, s, core.KindVanilla, "vanilla/data2024.json"),
		BattleCompanies: shippedFile(t, s, core.KindBattleCompanies, "battle_companies/battle_companies.json"),
		Glossary:        shippedFile(t, s, core.KindGlossary, "glossary/mesbg_glossary.json"),
		Settings:        core.CampaignSettings{ManualRolls: true}})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []core.User{w.p1, w.p2, w.p3, w.admin} {
		if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, w.camp.ID, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	w.companies = core.NewCompanyService(s.Campaigns(), s.Companies(), rules).WithAftermaths(s.Aftermaths())
	w.armoury = core.NewArmouryService(s.Campaigns(), s.Companies(), rules)
	if w.co1, err = w.companies.CreateCompany(ctx, w.p1, w.camp.ID, core.CompanyInput{Name: "One", Faction: "Minas Tirith"}); err != nil {
		t.Fatal(err)
	}
	if w.co2, err = w.companies.CreateCompany(ctx, w.p2, w.camp.ID, core.CompanyInput{Name: "Two", Faction: "Minas Tirith"}); err != nil {
		t.Fatal(err)
	}
	for _, co := range []int64{w.co1.ID, w.co2.ID} { // the officers choose their path, or the company cannot play
		if err := s.Companies().Update(ctx, w.camp.ID, co, func(st *core.CompanyState) error {
			for i := range st.Units {
				if st.Units[i].Role != core.RoleWarrior && st.Units[i].Path == "" {
					st.Units[i].Path = "Path of the Soldier"
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	w.matches = core.NewMatches(s.Campaigns(), s.Companies(), s.Matches(), rules)
	w.am = core.NewAftermathService(s.Campaigns(), s.Companies(), s.Matches(), s.Aftermaths(), rules, core.RandomDice{})
	w.matches.AftermathStarted = w.am.Started
	return w
}

// play creates and ends a match won by side 1 in the rulebook scenario.
func (w *aftermathWorld) play(t *testing.T, scenario string, objectives map[string]int) core.Match {
	t.Helper()
	ctx := t.Context()
	m, err := w.matches.CreateMatch(ctx, w.org, w.camp.ID, []core.NewSide{{Name: "A", Companies: []int64{w.co1.ID}}, {Name: "B", Companies: []int64{w.co2.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if m, err = w.matches.SetScenario(ctx, w.org, w.camp.ID, m.ID, m.Revision, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: scenario}); err != nil {
		t.Fatal(err)
	}
	three, one := 3, 1
	res := core.MatchResult{Decided: true, Winner: 1, Sides: []core.SideResult{{VictoryPoints: &three}, {VictoryPoints: &one}},
		Companies: []core.CompanyResult{{CompanyID: w.co1.ID, Objectives: objectives}}}
	if m, err = w.matches.SaveResult(ctx, w.org, w.camp.ID, m.ID, m.Revision, res); err != nil {
		t.Fatal(err)
	}
	if m, err = endNow(ctx, w.matches, w.org, w.camp.ID, m.ID, m.Revision); err != nil {
		t.Fatal(err)
	}
	return m
}

func (w *aftermathWorld) roster(t *testing.T, id int64) []core.CompanyUnit {
	t.Helper()
	d, err := w.companies.CompanyDetail(t.Context(), w.camp.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	return d.Roster
}

func first(roster []core.CompanyUnit, role core.UnitRole) core.CompanyUnit {
	return roster[slices.IndexFunc(roster, func(u core.CompanyUnit) bool { return u.Role == role })]
}

func TestAftermathOnPostgres(t *testing.T) {
	w := newAftermathWorld(t)
	ctx := t.Context()
	id := w.camp.ID
	if err := w.s.Companies().Update(ctx, id, w.co1.ID, func(st *core.CompanyState) error { st.Influence = 10; return nil }); err != nil {
		t.Fatal(err)
	}

	// An open match has no aftermath and locks nothing.
	open, err := w.matches.CreateMatch(ctx, w.org, id, []core.NewSide{{Name: "A", Companies: []int64{w.co1.ID}}, {Name: "B", Companies: []int64{w.co2.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.am.Aftermath(ctx, w.p1, id, open.ID, w.co1.ID); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("open match: %v", err)
	}
	if p, err := w.am.PendingAftermaths(ctx, id, w.co1.ID); err != nil || len(p) != 0 {
		t.Fatalf("nothing pending yet: %v %v", p, err)
	}
	if refs, err := w.am.AftermathStatuses(ctx, id, open.ID); err != nil || len(refs) != 2 || refs[0].Status != "" {
		t.Fatalf("statuses of an open match: %+v %v", refs, err)
	}

	// A company is in one open match at a time: match 1 is waived and ended (raw SQL) to make room for the played one.
	if _, err := w.s.pool.Exec(ctx, `UPDATE matches SET ended = true, ended_on = now() WHERE id = $1`, open.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.s.pool.Exec(ctx, `INSERT INTO match_aftermaths (match_id, company_id, status) SELECT match_id, company_id, 'waived' FROM match_side_companies WHERE match_id = $1`, open.ID); err != nil {
		t.Fatal(err)
	}
	m := w.play(t, "Scavenge the Battlefield", map[string]int{"allObjectiveMarkers": 1})
	// (the open match above is match 1, the played one match 2)
	if m.Number != 2 {
		t.Fatalf("match number %d", m.Number)
	}

	// The lock: both companies must finish; roster changes and spending are refused, renaming is not.
	pend, err := w.am.PendingAftermaths(ctx, id, w.co1.ID)
	if err != nil || len(pend) != 1 || pend[0].MatchID != m.ID || pend[0].MatchNumber != 2 || pend[0].Status != core.AftermathPending {
		t.Fatalf("pending: %+v %v", pend, err)
	}
	leader := first(w.roster(t, w.co1.ID), core.RoleLeader)
	if err := w.armoury.BuyEquipment(ctx, w.p1, id, w.co1.ID, leader.ID, "Banner", ""); !errors.Is(err, core.ErrRefused) || !strings.Contains(err.Error(), "Finish the aftermath of Match 2") {
		t.Fatalf("locked: %v", err)
	}
	if err := w.companies.RenameCompany(ctx, w.p1, id, w.co1.ID, "One Renamed"); err != nil {
		t.Fatalf("renaming stays allowed: %v", err)
	}
	if d, _ := w.companies.CompanyDetail(ctx, id, w.co1.ID); len(d.Aftermaths) != 1 {
		t.Fatalf("the detail shows the aftermath: %+v", d.Aftermaths)
	}
	if _, err := w.matches.ReopenMatch(ctx, w.admin, id, m.ID, m.Revision); err != nil {
		t.Fatalf("reopen before anyone started: %v", err)
	}
	m, _ = endNow(ctx, w.matches, w.org, id, m.ID, m.Revision+1)

	// Permissions: the owner, the organiser and an administrator edit; another player only reads.
	a, err := w.am.Aftermath(ctx, w.p3, id, m.ID, w.co1.ID)
	if err != nil || a.Access != core.AftermathNone || a.Status != core.AftermathPending || len(a.Input.Models) == 0 {
		t.Fatalf("another player reads the default sheet: %+v %v", a, err)
	}
	if _, err := w.am.SaveAftermath(ctx, w.p3, id, m.ID, w.co1.ID, a.Revision, a.Input); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("another player writes: %v", err)
	}
	if _, err := w.am.Aftermath(ctx, core.User{ID: 999999}, id, m.ID, w.co1.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a stranger: %v", err)
	}
	if _, err := w.am.SaveAftermath(ctx, w.org, id, m.ID, w.co2.ID, 0, core.AftermathInput{}); err != nil {
		t.Fatalf("the organiser saves for the absent player: %v", err)
	}
	if _, err := w.am.SaveAftermath(ctx, w.admin, id, m.ID, w.co2.ID, 1, core.AftermathInput{}); err != nil {
		t.Fatalf("an administrator saves: %v", err)
	}
	if _, err := w.am.SaveAftermath(ctx, w.p2, id, m.ID, w.co2.ID, 1, core.AftermathInput{}); !errors.Is(err, core.ErrAftermathChanged) {
		t.Fatalf("stale revision: %v", err)
	}

	// Company one: a sergeant (hero) and a warrior are casualties.
	roster := w.roster(t, w.co1.ID)
	sergeant, warrior := first(roster, core.RoleSergeant), first(roster, core.RoleWarrior)
	in := core.AftermathInput{Models: []core.ModelSheet{{UnitID: sergeant.ID, Outcome: core.OutcomeCasualty}, {UnitID: warrior.ID, Outcome: core.OutcomeCasualty}}}
	a, _ = w.am.Aftermath(ctx, w.p1, id, m.ID, w.co1.ID)
	if _, err := w.am.StartAftermath(ctx, w.p1, id, m.ID, w.co1.ID, a.Revision+5, in); !errors.Is(err, core.ErrAftermathChanged) {
		t.Fatalf("stale start: %v", err)
	}
	a, err = w.am.StartAftermath(ctx, w.p1, id, m.ID, w.co1.ID, a.Revision, in)
	if err != nil || a.Status != core.AftermathInjuries || len(a.Casualties) != 2 {
		t.Fatalf("start: %+v %v", a, err)
	}
	if ok, _ := w.am.Started(ctx, id, m.ID); !ok {
		t.Error("started")
	}
	if _, err := w.matches.ReopenMatch(ctx, w.admin, id, m.ID, m.Revision); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("reopen after a start: %v", err)
	}
	if _, err := w.am.RollCasualty(ctx, w.p1, id, m.ID, w.co1.ID, warrior.ID, nil, true); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("thrown dice are refused by the campaign: %v", err)
	}
	if _, err := w.am.RollCasualty(ctx, w.p1, id, m.ID, w.co1.ID, warrior.ID, []int{1, 2}, false); err != nil { // 3: Dead
		t.Fatal(err)
	}
	a, err = w.am.RollCasualty(ctx, w.p1, id, m.ID, w.co1.ID, sergeant.ID, []int{1, 2}, false) // 3: Arm Wound
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != core.AftermathLevelUps {
		t.Fatalf("every casualty rolled: %+v", a)
	}
	d, _ := w.companies.CompanyDetail(ctx, id, w.co1.ID)
	if len(d.Cemetery) != 1 || d.Cemetery[0].Unit.ID != warrior.ID || d.Cemetery[0].Fate != "Dead" {
		t.Fatalf("the warrior is buried: %+v", d.Cemetery)
	}
	s1 := mustUnit(t, d.Roster, sergeant.ID)
	if len(s1.Injuries) != 1 || s1.Injuries[0].Key != "armWound" {
		t.Fatalf("the sergeant keeps the injury: %+v", s1.Injuries)
	}
	if ld := mustUnit(t, d.Roster, leader.ID); ld.XP == 0 {
		t.Errorf("the leader took part and earns experience: %d", ld.XP)
	}

	// Finish needs the free equipment decision.
	if _, err := w.am.FinishAftermath(ctx, w.p1, id, m.ID, w.co1.ID, a.Revision); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("finish without the choice: %v", err)
	}
	if len(a.Pending) != 1 || a.Pending[0].Key != "toTheVictorTheSpoils" {
		t.Fatalf("pending: %+v", a.Pending)
	}
	offers, err := w.am.FreeEquipmentOffers(ctx, id, w.co1.ID, a.Pending[0].MaxValue)
	if err != nil || len(offers) == 0 {
		t.Fatalf("offers: %v %v", offers, err)
	}
	if _, err := w.am.Decide(ctx, w.p1, id, m.ID, w.co1.ID, core.AftermathAnswer{Kind: "reward", Key: "toTheVictorTheSpoils", Answer: "Nonexistent|1"}); !errors.Is(err, core.ErrInvalidAftermath) {
		t.Fatalf("bad item: %v", err)
	}
	item := offers[0]
	a, err = w.am.Decide(ctx, w.p1, id, m.ID, w.co1.ID, core.AftermathAnswer{Kind: "reward", Key: "toTheVictorTheSpoils",
		Answer: item.Name + "|" + itoa64(item.Units[0].ID)})
	if err != nil || len(a.Pending) != 0 {
		t.Fatalf("decide: %+v %v", a, err)
	}
	before := d.Influence
	a, err = w.am.FinishAftermath(ctx, w.p1, id, m.ID, w.co1.ID, a.Revision)
	if err != nil || a.Status != core.AftermathDone {
		t.Fatalf("finish: %+v %v", a, err)
	}
	d, _ = w.companies.CompanyDetail(ctx, id, w.co1.ID)
	if d.Influence != before+4 || len(d.Aftermaths) != 0 || len(d.History) == 0 || d.Rerolls != 0 {
		t.Errorf("participating 2 + win 2: influence %d -> %d, aftermaths %v, history %d", before, d.Influence, d.Aftermaths, len(d.History))
	}
	if l := d.History[0]; l.Match != m.Number || l.MatchID != m.ID || l.On.IsZero() {
		t.Errorf("a history line carries the match and a date: %+v", l)
	}
	if p, _ := w.am.PendingAftermaths(ctx, id, w.co1.ID); len(p) != 0 {
		t.Errorf("unlocked: %+v", p)
	}
	if _, err := w.am.FinishAftermath(ctx, w.p1, id, m.ID, w.co1.ID, a.Revision); !errors.Is(err, core.ErrRefused) {
		t.Errorf("finish twice: %v", err)
	}

	// Waive: only the organiser and administrators; the company is unlocked without effects.
	if _, err := w.am.WaiveAftermath(ctx, w.p2, id, m.ID, w.co2.ID, "gone"); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("a player cannot waive: %v", err)
	}
	if _, err := w.am.WaiveAftermath(ctx, w.org, id, m.ID, w.co2.ID, " "); !errors.Is(err, core.ErrInvalidAftermath) {
		t.Fatalf("a note is needed: %v", err)
	}
	a, err = w.am.WaiveAftermath(ctx, w.org, id, m.ID, w.co2.ID, "player is away")
	if err != nil || a.Status != core.AftermathWaived || a.WaivedNote != "player is away" {
		t.Fatalf("waive: %+v %v", a, err)
	}
	if p, _ := w.am.PendingAftermaths(ctx, id, w.co2.ID); len(p) != 0 {
		t.Errorf("waived unlocks: %+v", p)
	}
	statuses, _ := w.am.AftermathStatuses(ctx, id, m.ID)
	if len(statuses) != 2 || statuses[0].Status != core.AftermathDone || statuses[1].Status != core.AftermathWaived {
		t.Errorf("statuses: %+v", statuses)
	}

	// Perks: a victory in Chance Encounter leaves one, bound to the owner; only the owner uses it.
	m2 := w.play(t, "Chance Encounter", nil)
	a, _ = w.am.Aftermath(ctx, w.p1, id, m2.ID, w.co1.ID)
	if a, err = w.am.StartAftermath(ctx, w.p1, id, m2.ID, w.co1.ID, a.Revision, a.Input); err != nil || a.Status != core.AftermathLevelUps {
		t.Fatalf("no casualties goes straight on: %+v %v", a, err)
	}
	if _, err = w.am.FinishAftermath(ctx, w.p1, id, m2.ID, w.co1.ID, a.Revision); err != nil {
		t.Fatal(err)
	}
	perks, err := w.am.Perks(ctx, id, w.co1.ID)
	if err != nil || len(perks) != 1 || perks[0].Effect != "rendezvous" || perks[0].UsedOn != nil || perks[0].MatchID != m2.ID {
		t.Fatalf("perks: %+v %v", perks, err)
	}
	if _, err := w.am.WaiveAftermath(ctx, w.org, id, m2.ID, w.co2.ID, "away"); err != nil {
		t.Fatal(err)
	}
	if err := w.am.UsePerk(ctx, w.p2, id, w.co1.ID, perks[0].ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("another player's company is not found: %v", err)
	}
	if err := w.am.UsePerk(ctx, w.p1, id, w.co1.ID, 424242); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown perk: %v", err)
	}
	if err := w.am.UsePerk(ctx, w.p1, id, w.co1.ID, perks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := w.am.UsePerk(ctx, w.p1, id, w.co1.ID, perks[0].ID); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("used twice: %v", err)
	}
	if d, _ = w.companies.CompanyDetail(ctx, id, w.co1.ID); len(d.Perks) != 1 || d.Perks[0].UsedOn == nil {
		t.Errorf("the detail lists the used perk: %+v", d.Perks)
	}
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func mustUnit(t *testing.T, roster []core.CompanyUnit, id int64) core.CompanyUnit {
	t.Helper()
	i := slices.IndexFunc(roster, func(u core.CompanyUnit) bool { return u.ID == id })
	if i < 0 {
		t.Fatalf("unit %d is not on the roster", id)
	}
	return roster[i]
}

func TestAftermathMigrationDownUp(t *testing.T) {
	s, ctx := testStore(t)
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	goose.SetBaseFS(migrations)
	if err := goose.DownToContext(ctx, db, "migrations", 12); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name IN ('match_aftermaths', 'company_perks')`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("down removes the tables: %d %v", n, err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name IN ('match_aftermaths', 'company_perks')`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("up creates them: %d %v", n, err)
	}
}

// endNow ends a match through the confirmation flow: the organiser proposes and overrides whoever has not confirmed.
func endNow(ctx context.Context, ms *core.Matches, org core.User, campaignID, matchID int64, revision int) (core.Match, error) {
	m, err := ms.ProposeEnd(ctx, org, campaignID, matchID, revision)
	if err != nil || m.Ended {
		return m, err
	}
	return ms.OverrideEnd(ctx, org, campaignID, matchID, m.Revision, "test: ended without confirmation")
}
