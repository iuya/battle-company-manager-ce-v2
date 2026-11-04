package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Where the bytes of a small archive (four companies, their starting units) go: sizes per top-level section of each pruned file.
func TestArchiveWhereTheBytesGo(t *testing.T) {
	rs := shippedRules(t)
	files := RuleFiles{Vanilla: readDef(t, "vanilla/data2024.json"), BattleCompanies: readDef(t, shippedBCPath()), Glossary: readDef(t, "glossary/mesbg_glossary.json")}
	var states []CompanyState
	for i, p := range rs.BC.Profiles()[:4] {
		st := CompanyState{Company: CampaignCompany{ID: int64(i + 1), Name: p.Name + " Co", Faction: p.Name}}
		for j, s := range p.Start {
			st.Units = append(st.Units, UnitState{ID: int64(j + 1), Base: s.Unit, Role: RoleWarrior, Bought: s.Wargear})
		}
		states = append(states, st)
	}
	a, err := BuildArchive(rs, ArchiveSource{Name: "four", Raw: files, States: states})
	if err != nil {
		t.Fatal(err)
	}
	section := func(label string, raw, orig []byte) {
		got, _ := decode(raw)
		was, _ := decode(orig)
		var parts []string
		for k, v := range got {
			b, _ := json.Marshal(v)
			ob, _ := json.Marshal(was[k])
			if len(b)*40 > len(raw) { // 2.5% or more of the file
				parts = append(parts, fmt.Sprintf("%s %d (was %d)", k, len(b), len(ob)))
			}
		}
		slices.Sort(parts)
		t.Logf("%s %d bytes (was %d): %s", label, len(raw), len(orig), strings.Join(parts, ", "))
	}
	section("vanilla", a.Rules.Vanilla, files.Vanilla)
	section("battle companies", a.Rules.BattleCompanies, files.BattleCompanies)
	section("glossary", a.Rules.Glossary, files.Glossary)
	van, _ := decode(a.Rules.Vanilla)
	ov, _ := decode(files.Vanilla)
	d1, _ := van["data"].(obj)
	d2, _ := ov["data"].(obj)
	for k, v := range d1 {
		b, _ := json.Marshal(v)
		ob, _ := json.Marshal(d2[k])
		t.Logf("vanilla data.%s %d (was %d)", k, len(b), len(ob))
	}
}

// EXPERIMENT: every shipped company, one of every unit it names, pruned to what they use; the pruned triple must load and build the same
// rosters as the full rule set.
func TestArchivePrunedRulesLoadAndBuildTheSame(t *testing.T) {
	rs := shippedRules(t)
	files := RuleFiles{Vanilla: readDef(t, "vanilla/data2024.json"), BattleCompanies: readDef(t, shippedBCPath()), Glossary: readDef(t, "glossary/mesbg_glossary.json")}
	var states []CompanyState
	for i, p := range rs.BC.Profiles() {
		p = rs.finishPreviews(p.WithPreviews(rs.Vanilla))
		st := CompanyState{Company: CampaignCompany{ID: int64(i + 1), Name: p.Name + " Co", Faction: p.Name}, Influence: 5}
		id := int64(1)
		for key, pv := range p.Previews {
			var gear []string
			if _, g, ok := strings.Cut(key, "|"); ok && g != "" {
				gear = strings.Split(g, ",")
			}
			role := RoleWarrior
			if pv.HasHeroStats() {
				role = RoleHero
			}
			st.Units = append(st.Units, UnitState{ID: id, Base: pv.Name, Role: role, Bought: gear})
			id++
		}
		states = append(states, st)
	}
	a, err := BuildArchive(rs, ArchiveSource{Name: "all companies", Raw: files, States: states})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pruned sizes: vanilla %d, bc %d, glossary %d bytes", len(a.Rules.Vanilla), len(a.Rules.BattleCompanies), len(a.Rules.Glossary))
	mini, err := a.LoadPruned()
	if err != nil {
		var rep *RulesetReport
		if errors.As(err, &rep) {
			kinds := map[string]int{}
			for _, i := range rep.Issues() {
				path := regexp.MustCompile(`\[\d+\]`).ReplaceAllString(i.Path, "[]")
				msg := regexp.MustCompile(`"[^"]*"`).ReplaceAllString(i.Message, `"…"`)
				kinds[i.File+" "+path+": "+msg]++
			}
			for k, n := range kinds {
				t.Logf("%4d  %s", n, k)
			}
		}
		t.Fatalf("the pruned triple does not load: %v", err)
	}
	diffs := 0
	for _, st := range states {
		full, small := BuildDetail(rs, CampaignSettings{}, st), BuildDetail(mini, CampaignSettings{}, st)
		if len(full.Roster) != len(small.Roster) || full.Points != small.Points {
			t.Errorf("%s: roster %d/%d points %d/%d", st.Company.Faction, len(full.Roster), len(small.Roster), full.Points, small.Points)
			continue
		}
		for i := range full.Roster {
			if a, b := digestUnit(full.Roster[i]), digestUnit(small.Roster[i]); a != b {
				at := 0
				for at < len(a) && at < len(b) && a[at] == b[at] {
					at++
				}
				t.Errorf("%s: unit %s differs at %d\n full  …%s\n small …%s", st.Company.Faction, full.Roster[i].Type, at, a[max(0, at-60):min(len(a), at+120)], b[max(0, at-60):min(len(b), at+120)])
				if diffs++; diffs >= 6 {
					t.Fatal("more differences")
				}
			}
		}
	}
}
