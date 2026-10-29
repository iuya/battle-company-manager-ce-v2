package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// practiceWorld is the aftermath world with a second company for player one (companies per player 2): a practice match between a
// player's two companies is the same-player case of docs/practice-match-spec.md.
func practiceWorld(t *testing.T) (*aftermathWorld, core.CampaignCompany) {
	t.Helper()
	s, _ := scratchStore(t)
	w := newAftermathWorldOn(t, s)
	ctx := t.Context()
	if err := w.s.Campaigns().UpdateSettings(ctx, w.camp.ID, func(_ int64, _ core.CampaignStatus, cur core.CampaignSettings) (core.CampaignSettings, error) {
		cur.CompaniesPerPlayer = 2
		return cur, nil
	}); err != nil {
		t.Fatal(err)
	}
	cos := core.NewCompanyService(w.s.Campaigns(), w.s.Companies(), core.NewRules(w.s.Rulesets(), nil))
	twin, err := cos.CreateCompany(ctx, w.p1, w.camp.ID, core.CompanyInput{Name: "One again", Faction: "Minas Tirith"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.s.Companies().Update(ctx, w.camp.ID, twin.ID, func(st *core.CompanyState) error {
		for i := range st.Units {
			if st.Units[i].Role != core.RoleWarrior && st.Units[i].Path == "" {
				st.Units[i].Path = "Path of the Soldier"
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return w, twin
}

// finish plays a match to its end: a scenario, a result won by side 1 and the confirmed end.
func (w *aftermathWorld) finish(t *testing.T, m core.Match) core.Match {
	t.Helper()
	ctx := t.Context()
	var err error
	if m, err = w.matches.SetScenario(ctx, w.org, w.camp.ID, m.ID, m.Revision, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Scavenge the Battlefield"}); err != nil {
		t.Fatal(err)
	}
	three, one := 3, 1
	res := core.MatchResult{Decided: true, Winner: 1, Sides: []core.SideResult{{VictoryPoints: &three}, {VictoryPoints: &one}}}
	if m, err = w.matches.SaveResult(ctx, w.org, w.camp.ID, m.ID, m.Revision, res); err != nil {
		t.Fatal(err)
	}
	if m, err = endNow(ctx, w.matches, w.org, w.camp.ID, m.ID, m.Revision); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPracticeMatchOnPostgres(t *testing.T) {
	w, twin := practiceWorld(t)
	ctx := t.Context()
	id := w.camp.ID

	// Created as a practice match: the flag is stored, read back by Get and List, and locks the companies like any open match.
	m, err := w.matches.CreateMatch(ctx, w.org, id, core.NewMatch{Practice: true,
		Sides: []core.NewSide{{Name: "A", Companies: []int64{w.co1.ID}}, {Name: "B", Companies: []int64{twin.ID}}}})
	if err != nil || !m.Practice {
		t.Fatalf("CreateMatch = %+v, %v", m, err)
	}
	if got, err := w.matches.Match(ctx, id, m.ID); err != nil || !got.Practice {
		t.Fatalf("Match = %+v, %v", got, err)
	}
	_, err = w.matches.CreateMatch(ctx, w.org, id, core.NewMatch{Sides: []core.NewSide{{Companies: []int64{w.co1.ID}}, {Companies: []int64{w.co2.ID}}}})
	var busy *core.CompanyBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("a company in an open practice match cannot join another: %v", err)
	}
	if d, err := w.companies.CompanyDetail(ctx, id, w.co1.ID); err != nil || d.OpenMatch == nil {
		t.Fatalf("the company page shows the open match: %+v %v", d.OpenMatch, err)
	}
	before1, before2 := w.state(t, w.co1.ID), w.state(t, twin.ID)

	m = w.finish(t, m)
	if !m.Ended || !m.Practice {
		t.Fatalf("ended: %+v", m)
	}

	// Nothing is pending, nothing is locked, nothing is stored.
	for _, cid := range []int64{w.co1.ID, twin.ID} {
		if p, err := w.am.PendingAftermaths(ctx, id, cid); err != nil || len(p) != 0 {
			t.Fatalf("company %d pending: %+v %v", cid, p, err)
		}
		a, err := w.am.Aftermath(ctx, w.p1, id, m.ID, cid)
		if err != nil || !a.Practice || a.Status != core.AftermathDone {
			t.Fatalf("company %d aftermath: %+v %v", cid, a, err)
		}
		if d, err := w.companies.CompanyDetail(ctx, id, cid); err != nil || len(d.Aftermaths) != 0 {
			t.Fatalf("company %d detail: %+v %v", cid, d.Aftermaths, err)
		}
	}
	refs, err := w.am.AftermathStatuses(ctx, id, m.ID)
	if err != nil || len(refs) != 2 || refs[0].Status != core.AftermathDone || refs[1].Status != core.AftermathDone {
		t.Fatalf("statuses: %+v %v", refs, err)
	}
	rec, err := w.s.Aftermaths().Get(ctx, id, m.ID, w.co1.ID)
	if err != nil || rec.Status != core.AftermathDone || rec.Revision != 1 || !rec.MatchEnded || rec.FinishedAt == nil || len(rec.Lines) != 1 || rec.Lines[0].Text != core.PracticeNothingToRoll {
		t.Fatalf("record: %+v %v", rec, err)
	}
	if err := w.s.Aftermaths().Update(ctx, id, m.ID, w.co1.ID, func(*core.CompanyState, *core.AftermathRecord) error { return nil }); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("the repository refuses to store one: %v", err)
	}
	if _, err := w.am.StartAftermath(ctx, w.p1, id, m.ID, w.co1.ID, 1, core.AftermathInput{}); !errors.Is(err, core.ErrRefused) || !strings.Contains(err.Error(), core.PracticeNothingToRoll) {
		t.Fatalf("write: %v", err)
	}
	var rows int
	if err := w.s.pool.QueryRow(ctx, `SELECT count(*) FROM match_aftermaths WHERE match_id = $1`, m.ID).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("match_aftermaths rows = %d, %v", rows, err)
	}
	if after := w.state(t, w.co1.ID); !sameState(before1, after) {
		t.Errorf("company one changed: %+v -> %+v", before1, after)
	}
	if after := w.state(t, twin.ID); !sameState(before2, after) {
		t.Errorf("company two changed: %+v -> %+v", before2, after)
	}

	// Counts: the company has no win, the campaign has no played match and one practice match.
	cos, err := w.s.Campaigns().Companies(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cos {
		if c.Wins+c.Draws+c.Losses != 0 {
			t.Errorf("%s: W/D/L %d/%d/%d", c.Name, c.Wins, c.Draws, c.Losses)
		}
	}
	d, err := w.s.Campaigns().Get(ctx, w.p1.ID, id)
	if err != nil || d.Matches != 0 || d.PracticeMatches != 1 || d.PendingAftermaths != 0 {
		t.Fatalf("detail: matches %d practice %d pending %d, %v", d.Matches, d.PracticeMatches, d.PendingAftermaths, err)
	}
	if list, err := w.s.Campaigns().List(ctx, w.p1.ID, false); err != nil || len(list) != 1 || list[0].Matches != 0 || list[0].PracticeMatches != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}

	// The company is free at once: a real match opens, and next to it the practice one stays derived while the real one is pending.
	real := w.play(t, "Scavenge the Battlefield", nil)
	for _, c := range []struct {
		id      int64
		pending int
	}{{w.co1.ID, 1}, {twin.ID, 0}, {w.co2.ID, 1}} {
		p, err := w.am.PendingAftermaths(ctx, id, c.id)
		if err != nil || len(p) != c.pending || (c.pending == 1 && p[0].MatchID != real.ID) {
			t.Fatalf("company %d pending: %+v %v", c.id, p, err)
		}
	}
	refs, err = w.am.AftermathStatuses(ctx, id, real.ID)
	if err != nil || refs[0].Status != core.AftermathPending {
		t.Fatalf("a real match is unchanged: %+v %v", refs, err)
	}
	if a, err := w.am.Aftermath(ctx, w.p1, id, real.ID, w.co1.ID); err != nil || a.Practice || a.Status != core.AftermathPending {
		t.Fatalf("real aftermath: %+v %v", a, err)
	}
	cos, _ = w.s.Campaigns().Companies(ctx, id)
	for _, c := range cos {
		if c.ID == w.co1.ID && (c.Wins != 1 || c.Losses != 0) {
			t.Errorf("only the real match counts: %d/%d/%d", c.Wins, c.Draws, c.Losses)
		}
	}
	if d, _ = w.s.Campaigns().Get(ctx, w.p1.ID, id); d.Matches != 1 || d.PracticeMatches != 1 || d.PendingAftermaths != 2 {
		t.Errorf("detail: matches %d practice %d pending %d", d.Matches, d.PracticeMatches, d.PendingAftermaths)
	}
}

// An administrator can reopen an ended practice match and end it again.
func TestReopenAPracticeMatchOnPostgres(t *testing.T) {
	w, twin := practiceWorld(t)
	ctx := t.Context()
	m, err := w.matches.CreateMatch(ctx, w.org, w.camp.ID, core.NewMatch{Practice: true, Sides: []core.NewSide{{Companies: []int64{w.co1.ID}}, {Companies: []int64{twin.ID}}}})
	if err != nil {
		t.Fatal(err)
	}
	m = w.finish(t, m)
	if m, err = w.matches.ReopenMatch(ctx, w.admin, w.camp.ID, m.ID, m.Revision); err != nil || m.Ended || !m.Practice {
		t.Fatalf("reopen: %+v %v", m, err)
	}
	if m, err = endNow(ctx, w.matches, w.org, w.camp.ID, m.ID, m.Revision); err != nil || !m.Ended {
		t.Fatalf("end again: %+v %v", m, err)
	}
}

func TestPracticeMigrationDownUp(t *testing.T) {
	s, ctx := testStore(t)
	db := stdlib.OpenDBFromPool(s.pool)
	defer db.Close()
	goose.SetBaseFS(migrations)
	column := func() int {
		var n int
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'matches' AND column_name = 'practice'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := goose.DownToContext(ctx, db, "migrations", 24); err != nil {
		t.Fatal(err)
	}
	if column() != 0 {
		t.Fatal("down drops the column")
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		t.Fatal(err)
	}
	if column() != 1 {
		t.Fatal("up adds it")
	}
}

func (w *aftermathWorld) state(t *testing.T, companyID int64) core.CompanyState {
	t.Helper()
	st, err := w.s.Companies().Read(t.Context(), w.camp.ID, companyID)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// sameState compares what an aftermath could change: resources, rerolls and the units' experience, injuries and presence.
func sameState(a, b core.CompanyState) bool {
	if a.Influence != b.Influence || a.Gold != b.Gold || a.Rerolls != b.Rerolls || len(a.Units) != len(b.Units) {
		return false
	}
	for i := range a.Units {
		if a.Units[i].XP != b.Units[i].XP || a.Units[i].Injured != b.Units[i].Injured || a.Units[i].ID != b.Units[i].ID {
			return false
		}
	}
	return true
}
