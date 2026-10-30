package postgres

import (
	"errors"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestATOStore(t *testing.T) {
	w := newLiveWorld(t, "ato")
	ato := w.s.ATO()
	rec, err := ato.Get(w.ctx, w.camp, w.match.ID)
	if err != nil || rec.CompanyID != 0 || !rec.Selection.Empty() {
		t.Fatalf("nothing stored: %+v %v", rec, err)
	}
	if started, err := ato.GameStarted(w.ctx, w.camp, w.match.ID); err != nil || started {
		t.Fatalf("not started: %v %v", started, err)
	}
	if _, err := ato.Get(w.ctx, w.camp, w.match.ID+9999); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("another campaign's or unknown match: %v", err)
	}
	if _, err := ato.GameStarted(w.ctx, w.camp, w.match.ID+9999); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("GameStarted of an unknown match: %v", err)
	}
	if _, err := ato.GameStarted(w.ctx, w.camp+9999, w.match.ID); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("GameStarted of a match of another campaign: %v", err)
	}
	// the match has a rolled scenario and a reported objective
	if _, err := w.s.Matches().Update(w.ctx, w.camp, w.match.ID, w.org, func(m *core.Match) error {
		m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Rolled"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.s.pool.Exec(w.ctx, `INSERT INTO match_objectives (match_id, company_id, key, times) VALUES ($1, $2, 'x', 1)`, w.match.ID, w.a.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := w.s.Matches().Get(w.ctx, w.camp, w.match.ID)

	sel := core.ATOSelection{Bonuses: []string{"toolKit", "ambush"}, Kits: []string{"healer"}, Ambush: "Ambushed",
		Items: []core.KitAssignment{{Kit: "healer", Item: "Wondrous Cram", Index: 2, UnitID: w.unitsOf[w.a.ID][1]}, {Kit: "hunter", Item: "Envenom Weapon (X)", Detail: "Spear"}}}
	m, err := ato.Update(w.ctx, w.camp, w.match.ID, w.org, func(st *core.ATOState) error {
		if st.Started || !st.Record.Selection.Empty() || st.Match.Revision != before.Revision {
			t.Errorf("state: %+v", st)
		}
		st.Match.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Ambushed"}
		st.Record = core.ATORecord{MatchID: w.match.ID, CompanyID: w.a.ID, Selection: sel, Previous: &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Rolled"}, HadPrevious: true}
		return nil
	})
	if err != nil || m.Scenario == nil || m.Scenario.Name != "Ambushed" || m.Revision != before.Revision+1 {
		t.Fatalf("update: %+v %v", m, err)
	}
	rec, err = ato.Get(w.ctx, w.camp, w.match.ID)
	if err != nil || rec.CompanyID != w.a.ID || !rec.HadPrevious || rec.Previous == nil || rec.Previous.Name != "Rolled" ||
		len(rec.Selection.Items) != 2 || rec.Selection.Items[0].UnitID != w.unitsOf[w.a.ID][1] || rec.Selection.Items[1].Detail != "Spear" ||
		rec.Selection.Ambush != "Ambushed" || len(rec.Selection.Kits) != 1 || len(rec.Selection.Bonuses) != 2 {
		t.Fatalf("record: %+v %v", rec, err)
	}
	var objectives int
	if err := w.s.pool.QueryRow(w.ctx, `SELECT count(*) FROM match_objectives WHERE match_id = $1`, w.match.ID).Scan(&objectives); err != nil || objectives != 0 {
		t.Errorf("a new scenario clears the objectives: %d %v", objectives, err)
	}
	// a second save updates the row; a failing fn stores nothing
	if _, err := ato.Update(w.ctx, w.camp, w.match.ID, w.org, func(st *core.ATOState) error {
		st.Record.Selection.Wanderer = "ignored"
		return errors.New("no")
	}); err == nil {
		t.Fatal("fn failed")
	}
	if rec, _ = ato.Get(w.ctx, w.camp, w.match.ID); rec.Selection.Wanderer != "" {
		t.Error("a failed save stores nothing")
	}
	sel.Wanderer, sel.Bonuses = "Wandering Sage", []string{"toolKit", "ambush", "wanderer"}
	if _, err := ato.Update(w.ctx, w.camp, w.match.ID, w.org, func(st *core.ATOState) error { st.Record.Selection = sel; return nil }); err != nil {
		t.Fatal(err)
	}
	if rec, _ = ato.Get(w.ctx, w.camp, w.match.ID); rec.Selection.Wanderer != "Wandering Sage" || rec.CompanyID != w.a.ID {
		t.Errorf("second save: %+v", rec)
	}
	var revision int
	if err := w.s.pool.QueryRow(w.ctx, `SELECT revision FROM match_ato WHERE match_id = $1`, w.match.ID).Scan(&revision); err != nil || revision != 2 {
		t.Errorf("row revision %d %v", revision, err)
	}
	// the game starts with the first live entry and stops being started when it is undone
	if err := w.s.Live().Init(w.ctx, w.camp, w.match.ID, w.inits()); err != nil {
		t.Fatal(err)
	}
	e, err := w.append(t, w.org, stat(w.unitsOf[w.a.ID][0], w.a.ID, -1))
	if err != nil {
		t.Fatal(err)
	}
	if started, _ := ato.GameStarted(w.ctx, w.camp, w.match.ID); !started {
		t.Error("a live entry started the game")
	}
	if _, err := ato.Update(w.ctx, w.camp, w.match.ID, w.org, func(st *core.ATOState) error {
		if !st.Started {
			t.Error("Started under the lock")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.s.Live().Update(w.ctx, w.camp, w.match.ID, w.org, func(core.LiveSnapshot) (core.LiveChange, error) { return core.LiveChange{Undo: e.ID}, nil }); err != nil {
		t.Fatal(err)
	}
	if started, _ := ato.GameStarted(w.ctx, w.camp, w.match.ID); started {
		t.Error("undoing every entry unlocks it")
	}
	// an empty selection deletes the row and Ambush put the rolled scenario back
	m, err = ato.Update(w.ctx, w.camp, w.match.ID, w.org, func(st *core.ATOState) error {
		st.Match.Scenario = st.Record.Previous
		st.Record = core.ATORecord{}
		return nil
	})
	if err != nil || m.Scenario == nil || m.Scenario.Name != "Rolled" {
		t.Fatalf("clear: %+v %v", m, err)
	}
	if rec, _ = ato.Get(w.ctx, w.camp, w.match.ID); rec.CompanyID != 0 || !rec.Selection.Empty() {
		t.Errorf("deleted: %+v", rec)
	}
	// deleting the match's company rows (cascade) is not needed; but a record for a company outside the match is refused by the key
	if _, err := ato.Update(w.ctx, w.camp, w.match.ID, w.org, func(st *core.ATOState) error {
		st.Record = core.ATORecord{MatchID: w.match.ID, CompanyID: 987654, Selection: sel}
		return nil
	}); err == nil {
		t.Error("a company outside the match cannot own the record")
	}
}
