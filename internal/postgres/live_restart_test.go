package postgres

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// A live match survives a restart: all state is in Postgres, so a new service and hub on the same database give the same folded
// state and log, and a client that reconnects is told to refetch everything.
func TestLiveMatchSurvivesARestart(t *testing.T) {
	s, ctx := testStore(t)
	rules := core.NewRules(s.Rulesets(), nil)
	one, two := testUser(t, s, ctx, "rs1"), testUser(t, s, ctx, "rs2")
	one.CanCreateCampaigns = true
	campaigns := core.NewCampaigns(s.Campaigns(), s.Companies(), s.Rulesets(), rules, nil)
	camp, err := campaigns.Create(ctx, one, core.CampaignInput{Name: "Restart",
		Vanilla:         shippedFile(t, s, core.KindVanilla, "vanilla/data2024.json"),
		BattleCompanies: shippedFile(t, s, core.KindBattleCompanies, "battle_companies/battle_companies.json"),
		Glossary:        shippedFile(t, s, core.KindGlossary, "glossary/mesbg_glossary.json")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, two.ID); err != nil {
		t.Fatal(err)
	}
	companies := core.NewCompanyService(s.Campaigns(), s.Companies(), rules)
	a, err := companies.CreateCompany(ctx, one, camp.ID, core.CompanyInput{Name: "Guard", Faction: "Minas Tirith"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := companies.CreateCompany(ctx, two, camp.ID, core.CompanyInput{Name: "Horde", Faction: "Mordor"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a.ID, b.ID} { // the officers have chosen their paths
		if err := s.Companies().Update(ctx, camp.ID, id, func(st *core.CompanyState) error {
			for i := range st.Units {
				if st.Units[i].Role != core.RoleWarrior {
					st.Units[i].Path = "Path of the Warlord"
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	matches := core.NewMatches(s.Campaigns(), s.Companies(), s.Matches(), rules)
	m, err := matches.CreateMatch(ctx, one, camp.ID, []core.NewSide{{Name: "Light", Companies: []int64{a.ID}}, {Name: "Dark", Companies: []int64{b.ID}}})
	if err != nil {
		t.Fatal(err)
	}

	live := core.NewLiveService(s.Live(), s.Campaigns(), s.Companies(), s.Matches(), rules)
	mine, err := live.Live(ctx, one, camp.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := live.Live(ctx, two, camp.ID, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	attacker, target := mine.Units[0].UnitID, theirs.Units[0].UnitID
	if _, _, err := live.Apply(ctx, one, camp.ID, m.ID, a.ID, core.LiveAction{Kind: core.LiveWound, UnitID: attacker, Targets: []core.WoundTarget{{UnitID: target, Caused: 1}}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := live.Apply(ctx, one, camp.ID, m.ID, a.ID, core.LiveAction{Kind: core.LiveBroken, UnitID: attacker}); err != nil {
		t.Fatal(err)
	}
	logBefore, err := live.Log(ctx, one, camp.ID, m.ID, 0, 50)
	if err != nil || len(logBefore) != 2 {
		t.Fatalf("log: %+v %v", logBefore, err)
	}
	stateBefore, _ := live.Live(ctx, two, camp.ID, b.ID)

	// the process dies and another starts: new service, new hub, new listening connection
	hub := core.NewLiveHub(s.LiveFeed())
	restarted := core.NewLiveService(s.Live(), s.Campaigns(), s.Companies(), s.Matches(), rules).WithHub(hub)
	hctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = hub.Start(hctx) }()

	logAfter, err := restarted.Log(ctx, one, camp.ID, m.ID, 0, 50)
	if err != nil || len(logAfter) != 2 || logAfter[0].Text != logBefore[0].Text || logAfter[1].Seq != logBefore[1].Seq {
		t.Fatalf("log after restart: %+v %v", logAfter, err)
	}
	stateAfter, err := restarted.Live(ctx, two, camp.ID, b.ID)
	if err != nil || stateAfter.Seq != stateBefore.Seq || stateAfter.Units[0].Wounds != stateBefore.Units[0].Wounds {
		t.Fatalf("state after restart: %+v vs %+v (%v)", stateAfter.Units[0], stateBefore.Units[0], err)
	}
	stream, err := restarted.Subscribe(hctx, one, camp.ID, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	nctx, ncancel := context.WithTimeout(ctx, 5*time.Second)
	defer ncancel()
	batch, err := stream.Next(nctx)
	if err != nil || len(batch.Companies) != 2 || batch.Seq != stateAfter.Seq {
		t.Fatalf("a reconnecting client resyncs: %+v %v", batch, err)
	}
	// a write through the restarted service reaches the stream through NOTIFY
	time.Sleep(300 * time.Millisecond) // the listener is up after its first resync
	if _, _, err := restarted.Apply(ctx, two, camp.ID, m.ID, b.ID, core.LiveAction{Kind: core.LiveBroken, UnitID: theirs.Units[1].UnitID}); err != nil {
		t.Fatal(err)
	}
	for {
		batch, err = stream.Next(nctx)
		if err != nil {
			t.Fatalf("no event after the write: %v", err)
		}
		if slices.Contains(batch.Companies, b.ID) && batch.Seq > stateAfter.Seq {
			break
		}
	}
}
