package postgres

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// shippedFile stores one of the rules files the app ships with and returns its hash.
func shippedFile(t *testing.T, s *Store, kind core.RulesetKind, path string) string {
	t.Helper()
	raw, err := os.ReadFile("../../definitions/" + path)
	if err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		t.Fatal(err)
	}
	hash, err := core.RulesetHash(raw)
	if err != nil {
		t.Fatal(err)
	}
	name, version, err := core.RulesetMeta(kind, raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := s.Rulesets().Insert(ctx, core.StoredRuleset{RulesetInfo: core.RulesetInfo{Kind: kind, Hash: hash, Name: name, Version: version}, Size: compact.Len(), Content: compact.Bytes()}); err != nil {
		t.Fatal(err)
	}
	return hash
}

// TestCoreServicesOnPostgres runs the company services of the core against the real repositories and the shipped rules: a
// company is created, a recruit waits between two service instances, and concurrent purchases cannot overspend.
func TestCoreServicesOnPostgres(t *testing.T) {
	s, ctx := testStore(t)
	rules := core.NewRules(s.Rulesets(), nil)
	owner := testUser(t, s, ctx, "svc")
	owner.CanCreateCampaigns = true

	campaigns := core.NewCampaigns(s.Campaigns(), s.Companies(), s.Rulesets(), rules, nil)
	camp, err := campaigns.Create(ctx, owner, core.CampaignInput{Name: "Services",
		Vanilla:         shippedFile(t, s, core.KindVanilla, "vanilla/data2024.json"),
		BattleCompanies: shippedFile(t, s, core.KindBattleCompanies, "battle_companies/battle_companies.json"),
		Glossary:        shippedFile(t, s, core.KindGlossary, "glossary/mesbg_glossary.json"),
		Settings:        core.CampaignSettings{ManualRolls: true}})
	if err != nil {
		t.Fatal(err)
	}

	companies := core.NewCompanyService(s.Campaigns(), s.Companies(), rules)
	created, err := companies.CreateCompany(ctx, owner, camp.ID, core.CompanyInput{Name: "Guard", Faction: "Minas Tirith"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Points == 0 || created.Units == 0 {
		t.Errorf("a new company is rated from its units: %+v", created)
	}
	if _, err := companies.CreateCompany(ctx, owner, camp.ID, core.CompanyInput{Name: "Again", Faction: "Minas Tirith"}); !errors.Is(err, core.ErrCompanyLimit) {
		t.Fatalf("one company per player: %v", err)
	}
	detail, err := companies.CompanyDetail(ctx, camp.ID, created.ID)
	if err != nil || len(detail.Roster) == 0 {
		t.Fatalf("detail: %+v %v", detail, err)
	}
	got, err := campaigns.Get(ctx, owner.ID, camp.ID)
	if err != nil || len(got.Companies) != 1 || got.Companies[0].Points != created.Points {
		t.Fatalf("the campaign shows the company rated: %+v %v", got.Companies, err)
	}
	var officers []int64
	for _, u := range detail.Roster {
		if u.IsHero() {
			officers = append(officers, u.ID)
		}
	}
	if len(officers) != 3 {
		t.Fatalf("a leader and two sergeants: %d", len(officers))
	}

	// A recruit in progress is stored with the company: another service instance (another request) continues it.
	if err := s.Companies().Update(ctx, camp.ID, created.ID, func(st *core.CompanyState) error { st.Influence = 3; return nil }); err != nil {
		t.Fatal(err)
	}
	dice := &core.FixedDice{Rolls: []int{6}}
	if err := core.NewRecruitService(s.Campaigns(), s.Companies(), rules, dice).RollRecruit(ctx, owner, camp.ID, created.ID, 0, false); err != nil {
		t.Fatal(err)
	}
	res, err := core.NewRecruitService(s.Campaigns(), s.Companies(), rules, dice).ConfirmRecruit(ctx, owner, camp.ID, created.ID, 0, "")
	if err != nil || !res.Next {
		t.Fatalf("a 6 leads on to the special chart: %+v %v", res, err)
	}
	if state, _ := core.NewRecruitService(s.Campaigns(), s.Companies(), rules, dice).RecruitState(ctx, owner, camp.ID, created.ID); state == nil || state.Stage != "special" {
		t.Fatalf("the special chart waits: %+v", state)
	}
	if err := s.Companies().Update(ctx, camp.ID, created.ID, func(st *core.CompanyState) error {
		st.Recruit = core.RecruitProgress{}
		st.Influence = 3 // a badge costs 2: only one of the officers can have one
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Concurrent purchases serialise on the company row: the influence cannot be spent twice.
	armoury := core.NewArmouryService(s.Campaigns(), s.Companies(), rules)
	var bought atomic.Int32
	var wg sync.WaitGroup
	for _, id := range officers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch err := armoury.BuyEquipment(ctx, owner, camp.ID, created.ID, id, "Badge of Courage", ""); {
			case err == nil:
				bought.Add(1)
			case !errors.Is(err, core.ErrRefused):
				t.Errorf("unit %d: %v", id, err)
			}
		}()
	}
	wg.Wait()
	st, err := s.Companies().Read(ctx, camp.ID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bought.Load() != 1 || st.Influence != 1 {
		t.Errorf("3 influence buys one badge at 2: %d bought, %d influence left", bought.Load(), st.Influence)
	}

	// A fallen hero rests in the cemetery with its snapshot.
	if err := core.NewCemeteryService(s.Campaigns(), s.Companies(), rules).Bury(ctx, camp.ID, created.ID, officers[1], "Dead", "Match 1"); err != nil {
		t.Fatal(err)
	}
	after, err := companies.CompanyDetail(ctx, camp.ID, created.ID)
	if err != nil || len(after.Cemetery) != 1 || after.Cemetery[0].Unit.ID != officers[1] || len(after.Roster) != len(detail.Roster)-1 {
		t.Fatalf("cemetery: %+v %v", after.Cemetery, err)
	}
}
