package postgres

import (
	"errors"
	"reflect"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestMatches(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Matches()
	org, p2, outsider := testUser(t, s, ctx, "morg"), testUser(t, s, ctx, "mtwo"), testUser(t, s, ctx, "mout")
	camp, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Matches"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Elsewhere"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, p2.ID); err != nil {
		t.Fatal(err)
	}
	units := []core.UnitState{{Base: "Captain", Role: core.RoleLeader}, {Base: "Warrior", Role: core.RoleWarrior}}
	a, err := s.Companies().Create(ctx, camp.ID, org, core.NewCompany{Name: "Alpha", Faction: "Rohan", Gold: 5, Units: units})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Companies().Create(ctx, camp.ID, p2, core.NewCompany{Name: "Beta", Faction: "Gondor", Units: units})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := s.Companies().Create(ctx, other.ID, org, core.NewCompany{Name: "Far", Faction: "Rohan", Units: units})
	if err != nil {
		t.Fatal(err)
	}
	sides := []core.NewSide{{Name: "Riders", Companies: []int64{a.ID}}, {Name: "Guards", Companies: []int64{b.ID}}}

	// Create: membership, companies of the campaign, no duplicates, numbering.
	if _, err := repo.Create(ctx, camp.ID, outsider, core.NewMatch{Sides: sides}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("non-member: %v", err)
	}
	if _, err := repo.Create(ctx, 0, org, core.NewMatch{Sides: sides}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown campaign: %v", err)
	}
	if _, err := repo.Create(ctx, camp.ID, org, core.NewMatch{Sides: []core.NewSide{sides[0], {Name: "X", Companies: []int64{foreign.ID}}}}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("company of another campaign: %v", err)
	}
	if _, err := repo.Create(ctx, camp.ID, org, core.NewMatch{Sides: []core.NewSide{sides[0], {Name: "X", Companies: []int64{a.ID}}}}); !errors.Is(err, core.ErrInvalidMatch) {
		t.Fatalf("company twice: %v", err)
	}
	if l, _ := repo.List(ctx, camp.ID); len(l) != 0 {
		t.Fatalf("failed creates leave nothing: %+v", l)
	}
	m1, err := repo.Create(ctx, camp.ID, org, core.NewMatch{Sides: sides})
	if err != nil || m1.Number != 1 || m1.Revision != 1 || m1.Ended || m1.Scenario != nil || len(m1.Sides) != 2 ||
		m1.Sides[0].Name != "Riders" || m1.Sides[1].Companies[0].ID != b.ID || m1.Sides[1].Companies[0].Owner != "mtwo" || m1.PlayedOn.IsZero() {
		t.Fatalf("create: %+v, %v", m1, err)
	}
	// A company is in one open match at a time: m1 is out of the way (raw SQL, so revisions stay) while m2 is created.
	var busy *core.CompanyBusyError
	if _, err := repo.Create(ctx, camp.ID, p2, core.NewMatch{Sides: sides}); !errors.As(err, &busy) || !errors.Is(err, core.ErrRefused) || busy.Number != 1 || busy.MatchID != m1.ID || busy.Company != "Alpha" {
		t.Fatalf("company in an open match: %v", err)
	}
	setEnded := func(id int64, ended bool) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, `UPDATE matches SET ended = $2, ended_on = CASE WHEN $2 THEN now() END WHERE id = $1`, id, ended); err != nil {
			t.Fatal(err)
		}
	}
	setEnded(m1.ID, true)
	m2, err := repo.Create(ctx, camp.ID, p2, core.NewMatch{Sides: []core.NewSide{{Name: "B", Companies: []int64{b.ID}}, {Name: "A", Companies: []int64{a.ID}}}})
	if err != nil || m2.Number != 2 {
		t.Fatalf("second: %+v, %v", m2, err)
	}
	setEnded(m1.ID, false)
	list, err := repo.List(ctx, camp.ID)
	if err != nil || len(list) != 2 || list[0].ID != m2.ID || list[1].ID != m1.ID {
		t.Fatalf("list newest first: %+v, %v", list, err)
	}
	if _, err := repo.Get(ctx, other.ID, m1.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("match of another campaign: %v", err)
	}
	if d, _ := s.Campaigns().Get(ctx, org.ID, camp.ID); d.Matches != 2 {
		t.Fatalf("campaign match count: %d", d.Matches)
	}

	// Update: scenario, result, objectives.
	vp := 12
	got, err := repo.Update(ctx, camp.ID, m1.ID, org, func(m *core.Match) error {
		m.Scenario = &core.MatchScenario{Kind: core.ScenarioCustom, Name: "Ambush", Description: "d", Victory: "v"}
		m.Result = core.MatchResult{Decided: true, Winner: 1, Note: "close",
			Sides:     []core.SideResult{{VictoryPoints: &vp}, {}},
			Companies: []core.CompanyResult{{CompanyID: b.ID, Fled: true}, {CompanyID: a.ID, Objectives: map[string]int{"allCargo": 1}}}}
		m.Winner = 1
		return nil
	})
	if err != nil || got.Revision != 2 || got.Winner != 1 || got.Scenario == nil || *got.Scenario != (core.MatchScenario{Kind: core.ScenarioCustom, Name: "Ambush", Description: "d", Victory: "v"}) {
		t.Fatalf("update: %+v, %v", got, err)
	}
	want := core.MatchResult{Decided: true, Winner: 1, Note: "close", Sides: []core.SideResult{{VictoryPoints: &vp}, {}},
		Companies: []core.CompanyResult{{CompanyID: a.ID, Objectives: map[string]int{"allCargo": 1}}, {CompanyID: b.ID, Fled: true}}}
	if !reflect.DeepEqual(got.Result, want) {
		t.Fatalf("result round trip: %+v want %+v", got.Result, want)
	}
	if re, _ := repo.Get(ctx, camp.ID, m1.ID); !reflect.DeepEqual(re, got) {
		t.Fatalf("get differs from the returned match: %+v vs %+v", re, got)
	}

	// An error from fn stores nothing; a company that is not in the match is rejected as a whole.
	boom := errors.New("boom")
	if _, err := repo.Update(ctx, camp.ID, m1.ID, org, func(m *core.Match) error { m.Result.Note = "lost"; return boom }); err != boom {
		t.Fatal(err)
	}
	if _, err := repo.Update(ctx, camp.ID, m1.ID, org, func(m *core.Match) error {
		m.Result.Companies = []core.CompanyResult{{CompanyID: foreign.ID, Fled: true}}
		return nil
	}); !errors.Is(err, core.ErrInvalidMatch) {
		t.Fatalf("foreign company in result: %v", err)
	}
	if re, _ := repo.Get(ctx, camp.ID, m1.ID); re.Revision != 2 || re.Result.Note != "close" {
		t.Fatalf("nothing must be stored: %+v", re)
	}
	if _, err := repo.Update(ctx, other.ID, m1.ID, org, func(*core.Match) error { return nil }); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("update in another campaign: %v", err)
	}

	// Changing the scenario to a rulebook one drops the one-off fields; clearing removes it.
	got, err = repo.Update(ctx, camp.ID, m1.ID, org, func(m *core.Match) error {
		m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Baggage Train"}
		return nil
	})
	if err != nil || got.Scenario.Kind != core.ScenarioRulebook || got.Scenario.Description != "" {
		t.Fatalf("rulebook: %+v, %v", got.Scenario, err)
	}
	got, err = repo.Update(ctx, camp.ID, m1.ID, org, func(m *core.Match) error { m.Scenario = nil; m.Result.Companies = nil; return nil })
	if err != nil || got.Scenario != nil || len(got.Result.Companies) != 0 {
		t.Fatalf("clear: %+v, %v", got, err)
	}

	// Ending: stats, history, reopening.
	got, err = repo.Update(ctx, camp.ID, m1.ID, org, func(m *core.Match) error { m.Ended = true; return nil })
	if err != nil || !got.Ended || got.EndedOn == nil {
		t.Fatalf("end: %+v, %v", got, err)
	}
	if _, err := repo.Update(ctx, camp.ID, m2.ID, p2, func(m *core.Match) error {
		m.Result = core.MatchResult{Decided: true, Winner: 0, Sides: []core.SideResult{{}, {}}}
		m.Ended = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cos, _ := s.Campaigns().Companies(ctx, camp.ID)
	if len(cos) != 2 || cos[0].Wins != 1 || cos[0].Draws != 1 || cos[0].Losses != 0 || cos[1].Wins != 0 || cos[1].Draws != 1 || cos[1].Losses != 1 {
		t.Fatalf("W/D/L derive from ended matches: %+v", cos)
	}
	got, err = repo.Update(ctx, camp.ID, m1.ID, p2, func(m *core.Match) error { m.Ended = false; return nil })
	if err != nil || got.Ended || got.EndedOn != nil {
		t.Fatalf("reopen: %+v, %v", got, err)
	}
	if cos, _ = s.Campaigns().Companies(ctx, camp.ID); cos[0].Wins != 0 || cos[1].Losses != 0 {
		t.Fatalf("an open match counts for nobody: %+v", cos)
	}
	ev, err := repo.Events(ctx, camp.ID, m1.ID)
	if err != nil || len(ev) != 2 || ev[0].Kind != "ended" || ev[0].By != "morg" || ev[1].Kind != "reopened" || ev[1].By != "mtwo" {
		t.Fatalf("events: %+v, %v", ev, err)
	}
	if _, err := repo.Events(ctx, other.ID, m1.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
	// Plain saves add no event and the revision counts every write.
	if re, _ := repo.Get(ctx, camp.ID, m1.ID); re.Revision != 6 {
		t.Fatalf("revision: %d", re.Revision)
	}
}

func TestGrants(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Matches()
	org := testUser(t, s, ctx, "gorg")
	camp, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Grants"))
	if err != nil {
		t.Fatal(err)
	}
	other, _ := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Grants2"))
	co, err := s.Companies().Create(ctx, camp.ID, org, core.NewCompany{Name: "G", Faction: "Rohan", Gold: 5, Influence: 1,
		Units: []core.UnitState{{Base: "Captain", Role: core.RoleLeader}}})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := s.Companies().Read(ctx, camp.ID, co.ID)
	unit := st.Units[0].ID

	g, err := repo.AddGrant(ctx, camp.ID, org, core.Grant{CompanyID: co.ID, Kind: core.GrantOrganiser, Gold: 3, Influence: -1, Experience: 2, UnitID: unit, Note: "Narrative scenario"})
	if err != nil || g.ID == 0 || g.By != "gorg" || g.On.IsZero() {
		t.Fatalf("grant: %+v, %v", g, err)
	}
	st, _ = s.Companies().Read(ctx, camp.ID, co.ID)
	if st.Gold != 8 || st.Influence != 0 || st.Units[0].XP != 2 {
		t.Fatalf("applied: gold %d influence %d xp %d", st.Gold, st.Influence, st.Units[0].XP)
	}
	g2, err := repo.AddGrant(ctx, camp.ID, org, core.Grant{CompanyID: co.ID, Kind: core.GrantScenario, Gold: 1})
	if err != nil {
		t.Fatal(err)
	}
	hist, err := repo.Grants(ctx, camp.ID, co.ID)
	if err != nil || len(hist) != 2 || hist[0].ID != g2.ID || hist[1].UnitID != unit || hist[1].Gold != 3 || hist[1].Kind != core.GrantOrganiser {
		t.Fatalf("history: %+v, %v", hist, err)
	}
	for name, bad := range map[string]core.Grant{
		"organiser without note":  {CompanyID: co.ID, Kind: core.GrantOrganiser, Gold: 1},
		"experience without unit": {CompanyID: co.ID, Kind: core.GrantScenario, Experience: 1},
		"negative xp":             {CompanyID: co.ID, Kind: core.GrantOrganiser, Experience: -9, UnitID: unit, Note: "x"},
	} {
		if _, err := repo.AddGrant(ctx, camp.ID, org, bad); !errors.Is(err, core.ErrInvalidCompany) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// Gold and Influence never go below zero (the company has 9 Gold and 0 Influence here); nothing is stored then.
	before, _ := repo.Grants(ctx, camp.ID, co.ID)
	for name, bad := range map[string]core.Grant{
		"gold":      {CompanyID: co.ID, Kind: core.GrantOrganiser, Gold: -10, Note: "x"},
		"influence": {CompanyID: co.ID, Kind: core.GrantOrganiser, Gold: 1, Influence: -1, Note: "x"},
	} {
		if _, err := repo.AddGrant(ctx, camp.ID, org, bad); !errors.Is(err, core.ErrNegativeBalance) {
			t.Fatalf("negative %s: %v", name, err)
		}
	}
	if _, err := repo.AddGrant(ctx, camp.ID, org, core.Grant{CompanyID: co.ID, Kind: core.GrantOrganiser, Gold: -9, Note: "all of it"}); err != nil {
		t.Fatalf("down to exactly zero: %v", err)
	}
	if after, _ := repo.Grants(ctx, camp.ID, co.ID); len(after) != len(before)+1 {
		t.Fatalf("only the valid grant is stored: %d -> %d", len(before), len(after))
	}
	if st, _ = s.Companies().Read(ctx, camp.ID, co.ID); st.Gold != 0 || st.Influence != 0 {
		t.Fatalf("balance: gold %d influence %d", st.Gold, st.Influence)
	}
	if _, err := repo.AddGrant(ctx, camp.ID, org, core.Grant{CompanyID: co.ID, Kind: core.GrantScenario, Experience: 1, UnitID: unit + 999}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unit not on the roster: %v", err)
	}
	if _, err := repo.AddGrant(ctx, other.ID, org, core.Grant{CompanyID: co.ID, Kind: core.GrantScenario, Gold: 1}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("company of another campaign: %v", err)
	}
	if _, err := repo.Grants(ctx, other.ID, co.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
	if st, _ = s.Companies().Read(ctx, camp.ID, co.ID); st.Gold != 0 || st.Units[0].XP != 2 {
		t.Fatalf("failed grants change nothing: %+v", st)
	}
}

func TestMatchSideRoles(t *testing.T) {
	w := newLiveWorld(t, "roles")
	repo := w.s.Matches()
	// the live world already holds an open match with both companies; end it and make a new one with tags
	if _, err := w.s.pool.Exec(w.ctx, `UPDATE matches SET ended = true, ended_on = now() WHERE id = $1`, w.match.ID); err != nil {
		t.Fatal(err)
	}
	m, err := repo.Create(w.ctx, w.camp, w.org, core.NewMatch{Sides: []core.NewSide{{Name: "A", Companies: []int64{w.a.ID}, Role: "attacker"}, {Name: "B", Companies: []int64{w.b.ID}, Role: "defender"}}})
	if err != nil || m.Sides[0].Role != "attacker" || m.Sides[1].Role != "defender" {
		t.Fatalf("create with roles: %+v %v", m.Sides, err)
	}
	if _, err := w.s.pool.Exec(w.ctx, `UPDATE match_sides SET role = 'attacker' WHERE match_id = $1 AND position = 1`, m.ID); err == nil {
		t.Error("the unique index allows one attacker per match")
	}
	swapped, err := repo.Update(w.ctx, w.camp, m.ID, w.org, func(x *core.Match) error {
		x.Sides = append([]core.MatchSide(nil), x.Sides...)
		x.Sides[0].Role, x.Sides[1].Role = x.Sides[1].Role, x.Sides[0].Role
		return nil
	})
	if err != nil || swapped.Sides[0].Role != "defender" || swapped.Sides[1].Role != "attacker" {
		t.Fatalf("swap: %+v %v", swapped.Sides, err)
	}
	cleared, err := repo.Update(w.ctx, w.camp, m.ID, w.org, func(x *core.Match) error {
		x.Sides = append([]core.MatchSide(nil), x.Sides...)
		x.Sides[0].Role, x.Sides[1].Role = "", ""
		return nil
	})
	if err != nil || cleared.Sides[0].Role != "" || cleared.Sides[1].Role != "" {
		t.Fatalf("clear: %+v %v", cleared.Sides, err)
	}
}
