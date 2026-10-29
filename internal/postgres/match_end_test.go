package postgres

import (
	"testing"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestMatchEndProposalRoundTrip(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Matches()
	org, p2 := testUser(t, s, ctx, "eorg"), testUser(t, s, ctx, "etwo")
	camp, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Ending"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, p2.ID); err != nil {
		t.Fatal(err)
	}
	units := []core.UnitState{{Base: "Captain", Role: core.RoleLeader}, {Base: "Warrior", Role: core.RoleWarrior}}
	a, _ := s.Companies().Create(ctx, camp.ID, org, core.NewCompany{Name: "Alpha", Faction: "Rohan", Units: units})
	b, _ := s.Companies().Create(ctx, camp.ID, p2, core.NewCompany{Name: "Beta", Faction: "Gondor", Units: units})
	m, err := repo.Create(ctx, camp.ID, org, core.NewMatch{Sides: []core.NewSide{{Name: "Riders", Companies: []int64{a.ID}}, {Name: "Guards", Companies: []int64{b.ID}}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)

	// a pending end: Alpha confirmed by its owner, Beta waiting
	got, err := repo.Update(ctx, camp.ID, m.ID, org, func(x *core.Match) error {
		x.Scenario = &core.MatchScenario{Kind: core.ScenarioCustom, Name: "x"}
		x.Result = core.MatchResult{Decided: true, Winner: 1, Sides: make([]core.SideResult, 2)}
		x.EndProposal = &core.EndProposal{ProposedBy: org.DisplayName, ProposedByID: org.ID, ProposedOn: now, Notice: "Someone changed the result; confirm again",
			Companies: []core.EndConfirmation{{CompanyID: a.ID, OwnerID: org.ID, CompanyName: "Alpha", Confirmed: true, By: org.DisplayName, On: now}, {CompanyID: b.ID, OwnerID: p2.ID, CompanyName: "Beta"}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	p := got.EndProposal
	if p == nil || p.ProposedByID != org.ID || p.ProposedBy != "eorg" || p.Notice == "" || len(p.Companies) != 2 || !p.Companies[0].Confirmed || p.Companies[0].By != "eorg" ||
		p.Companies[1].Confirmed || p.Companies[1].OwnerID != p2.ID || p.Companies[1].CompanyName != "Beta" || p.Companies[0].On.IsZero() {
		t.Fatalf("stored proposal: %+v", p)
	}
	read, err := repo.Get(ctx, camp.ID, m.ID)
	if err != nil || read.EndProposal == nil || len(read.EndProposal.Waiting()) != 1 {
		t.Fatalf("Get: %+v %v", read.EndProposal, err)
	}
	list, _ := repo.List(ctx, camp.ID)
	if len(list) != 1 || list[0].EndProposal == nil {
		t.Errorf("List carries the proposal: %+v", list)
	}

	// the second owner confirms, an override-free end: the proposal goes and the history keeps who confirmed
	got, err = repo.Update(ctx, camp.ID, m.ID, p2, func(x *core.Match) error {
		x.Ended, x.EndProposal, x.EndDetail = true, nil, "confirmed by Alpha, Beta"
		return nil
	})
	if err != nil || got.EndProposal != nil || !got.Ended {
		t.Fatalf("end: %+v %v", got, err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM match_end_proposal WHERE match_id = $1) + (SELECT count(*) FROM match_end_confirmation WHERE match_id = $1)`, m.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows left behind: %d %v", n, err)
	}
	ev, err := repo.Events(ctx, camp.ID, m.ID)
	if err != nil || len(ev) != 1 || ev[0].Kind != "ended" || ev[0].Detail != "confirmed by Alpha, Beta" || ev[0].By != "etwo" {
		t.Fatalf("events: %+v %v", ev, err)
	}
}
