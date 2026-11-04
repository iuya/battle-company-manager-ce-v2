package core

import "testing"

// A campaign with a single company of any of the shipped ones, with nothing but its first unit, still prunes to a triple that loads: a company
// stays whole, so everything its charts name must stay with it.
func TestArchiveOfEachShippedCompanyAloneLoads(t *testing.T) {
	rs := shippedRules(t)
	files := RuleFiles{Vanilla: readDef(t, "vanilla/data2024.json"), BattleCompanies: readDef(t, shippedBCPath()), Glossary: readDef(t, "glossary/mesbg_glossary.json")}
	for _, p := range rs.BC.Profiles() {
		st := CompanyState{Company: CampaignCompany{ID: 1, Name: "Co", Faction: p.Name}}
		if len(p.Start) > 0 {
			st.Units = []UnitState{{ID: 1, Base: p.Start[0].Unit, Role: RoleWarrior}}
		}
		a, err := BuildArchive(rs, ArchiveSource{Raw: files, States: []CompanyState{st}})
		if err != nil {
			t.Fatalf("%s: %v", p.Name, err)
		}
		if _, err := a.LoadPruned(); err != nil {
			t.Errorf("%s: the pruned triple does not load: %v", p.Name, err)
		}
	}
}

// The pruned Battle Companies file of an archive says so and keeps no scenario selection tables; the same file without the mark is refused like
// any upload with empty tables.
func TestArchiveRelaxesTheScenarioSelectionOnlyForAnArchive(t *testing.T) {
	rs := shippedRules(t)
	files := RuleFiles{Vanilla: readDef(t, "vanilla/data2024.json"), BattleCompanies: readDef(t, shippedBCPath()), Glossary: readDef(t, "glossary/mesbg_glossary.json")}
	st := CompanyState{Company: CampaignCompany{ID: 1, Name: "Co", Faction: rs.BC.Profiles()[0].Name}}
	a, err := BuildArchive(rs, ArchiveSource{Raw: files, States: []CompanyState{st}})
	if err != nil {
		t.Fatal(err)
	}
	o, _ := decode(a.Rules.BattleCompanies)
	if o["archive"] != true {
		t.Fatal("the pruned file is not marked as an archive")
	}
	if _, err := a.LoadPruned(); err != nil {
		t.Fatalf("the archive does not load: %v", err)
	}
	delete(o, "archive")
	plain, _ := encode(o)
	a.Rules.BattleCompanies = plain
	if _, err := a.LoadPruned(); err == nil {
		t.Fatal("a file without the archive mark and with no selection tables must be refused")
	}
}
