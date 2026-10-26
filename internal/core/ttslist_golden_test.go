package core

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

var updateTTSGolden = flag.Bool("update-tts", false, "rewrite testdata/tts_golden.txt")

// ttsEntry is one unit a company can have: where it comes from, the name the company gives it and the wargear it comes with.
type ttsEntry struct {
	source, unit string
	wargear      []string
}

// ttsEntries are the start units, the reinforcement and special-chart results, the units bought directly and the advancement targets of a company.
func ttsEntries(rs *RuleSet, bcCo BCCompany) []ttsEntry {
	var out []ttsEntry
	profile, _ := rs.BC.Profile(bcCo.Name)
	for _, s := range profile.Start {
		out = append(out, ttsEntry{"start", s.Unit, s.Wargear})
	}
	for _, c := range []struct {
		name string
		rows []ChartRow
	}{{"reinforcement", bcCo.Reinforcements}, {"special chart", bcCo.SpecialChart}} {
		for _, r := range c.rows {
			if r.Result.Unit != "" {
				out = append(out, ttsEntry{c.name, r.Result.Unit, r.Result.Wargear})
			}
			if r.Result.Alt != "" {
				out = append(out, ttsEntry{c.name + " (alt)", r.Result.Alt, nil})
			}
			for _, o := range r.Result.Options {
				res := optionResult(o)
				out = append(out, ttsEntry{c.name + " (choice)", res.Unit, res.Wargear})
			}
		}
	}
	for _, s := range bcCo.SpecialUnits {
		out = append(out, ttsEntry{"special unit", s.Unit, nil})
	}
	for _, a := range rs.BC.Advancements(bcCo) {
		for _, to := range a.To {
			name := to.Unit
			if to.Label != "" {
				name = to.Label
			}
			out = append(out, ttsEntry{"advancement", name, to.Wargear})
		}
	}
	return out
}

func ttsOne(rs *RuleSet, bcCo BCCompany, e ttsEntry) CompanyDetail {
	u := buildUnit(rs, bcCo, UnitState{ID: 1, Base: e.unit, Role: RoleWarrior, Gear: slices.Clone(e.wargear)}, CampaignSettings{})
	return CompanyDetail{CampaignCompany: CampaignCompany{Faction: bcCo.Name}, Roster: []CompanyUnit{u}}
}

// TestTTSListAllCompanies writes every start unit, reinforcement, special-chart, special and advancement entry of the 37 companies as a list, checks the
// rules that hold for all of them, and compares the lot with testdata/tts_golden.txt (go test -run TTSListAll -update-tts rewrites it).
func TestTTSListAllCompanies(t *testing.T) {
	rs := shippedRules(t)
	models := rs.BC.TTSModels()
	known := map[string]bool{} // model names that are not vanilla units: stand-ins and mount figures
	for _, s := range models.StandIns {
		n, _ := splitModel(s.Model)
		known[n] = true
	}
	for _, m := range models.Mounts {
		n, _ := splitModel(m.Model)
		known[n] = true
	}
	var golden strings.Builder
	companies := ttsCompanies(rs)
	if len(companies) != 37 {
		t.Fatalf("the shipped file has %d companies, want 37", len(companies))
	}
	for _, bcCo := range companies {
		fmt.Fprintf(&golden, "## %s\n", bcCo.Name)
		for _, e := range ttsEntries(rs, bcCo) {
			d := ttsOne(rs, bcCo, e)
			ex := rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
			label := fmt.Sprintf("%s: %s %v", e.source, e.unit, e.wargear)
			lines := ex.Lines()
			if len(lines) == 0 {
				if !slices.ContainsFunc(ex.Notes, func(n TTSNote) bool { return n.Kind == TTSNoModel }) {
					t.Errorf("%s: %s: no line and no note", bcCo.Name, label)
				}
			}
			for _, l := range lines {
				checkTTSLine(t, rs, models, known, bcCo.Name+" "+label, l)
			}
			if strings.Count(ex.Text, "(") != len(lines) || strings.Count(ex.Text, ")") != len(lines) {
				t.Errorf("%s: %s: more than one pair of brackets per line in %q", bcCo.Name, label, ex.Text)
			}
			fmt.Fprintf(&golden, "%s\n", label)
			for _, l := range lines {
				fmt.Fprintf(&golden, "    %s\n", l.Text(false))
			}
			for _, n := range ex.Notes {
				fmt.Fprintf(&golden, "    note %s: %s\n", n.Kind, n.Text)
			}
		}
	}
	// the armoury: wanderers are stand-ins, creatures follow their owner
	co := ttsCompanies(rs)[0]
	_, wanderers := rs.BC.Wanderers()
	_, creatures := rs.BC.Creatures()
	fmt.Fprintf(&golden, "## Armoury\n")
	for _, w := range wanderers {
		u := buildUnit(rs, co, UnitState{ID: 1, Base: w.Name, Wanderer: w.Name, Role: RoleWarrior}, CampaignSettings{})
		ex := rs.TTSList(CompanyDetail{CampaignCompany: CampaignCompany{Faction: co.Name}, Roster: []CompanyUnit{u}}, TTSOptions{})
		writeTTSGolden(t, &golden, "wanderer: "+w.Name, ex)
	}
	for _, c := range creatures {
		u := buildUnit(rs, co, UnitState{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleLeader}, CampaignSettings{})
		d := CompanyDetail{CampaignCompany: CampaignCompany{Faction: co.Name}, Roster: []CompanyUnit{u}, Creatures: []CompanyCreature{{Name: c.Name, OwnerID: 1}}}
		writeTTSGolden(t, &golden, "creature: "+c.Name, rs.TTSList(d, TTSOptions{}))
	}
	const path = "testdata/tts_golden.txt"
	if *updateTTSGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(golden.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -run TTSListAll -update-tts)", err)
	}
	if string(want) != golden.String() {
		t.Errorf("the lists changed; run go test -run TTSListAll -update-tts and review the diff of %s", path)
	}
}

// checkTTSLine: the name is a vanilla unit (or a known stand-in or mount figure) and every option is one of that unit's, in the mod's spelling.
func checkTTSLine(t *testing.T, rs *RuleSet, models TTSModels, known map[string]bool, label string, l TTSLine) {
	t.Helper()
	if strings.ContainsAny(l.Name, "()") || slices.ContainsFunc(l.Options, func(o string) bool { return strings.ContainsAny(o, "()") }) {
		t.Errorf("%s: brackets in %q %v", label, l.Name, l.Options)
	}
	if l.Key != strings.ToLower(l.Name+": "+strings.Join(l.Options, ", ")) || l.Count < 1 || len(l.UnitIDs) == 0 {
		t.Errorf("%s: bad line %+v", label, l)
	}
	if !slices.IsSortedFunc(l.Options, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }) {
		t.Errorf("%s: options not sorted: %v", label, l.Options)
	}
	v, ok := rs.Vanilla.Unit(l.Name)
	if !ok {
		if !known[l.Name] {
			t.Errorf("%s: %q is neither a vanilla unit nor a known stand-in", label, l.Name)
		}
		return
	}
	for _, o := range l.Options {
		name := o
		for _, n := range models.Names {
			if n.Unit == v.Name && n.To == o {
				name = n.From
			}
		}
		if !slices.ContainsFunc(v.Options, func(vo ChoiceOption) bool { return vo.Name == name }) && !strings.Contains(strings.Join(known2(models, l.Name), "|"), o) {
			t.Errorf("%s: %q is not an option of %s", label, o, l.Name)
		}
	}
}

// known2 are the options of the stand-in and mount models named name ("Ruffian: Bow").
func known2(models TTSModels, name string) []string {
	var out []string
	for _, s := range models.StandIns {
		if n, o := splitModel(s.Model); n == name {
			out = append(out, o...)
		}
	}
	for _, m := range models.Mounts {
		if n, o := splitModel(m.Model); n == name {
			out = append(out, o...)
		}
	}
	return out
}

// TestTTSListModelsFile: when the local catalogue sources/tts/models.json exists (gitignored), every name the lists emit is a key of it.
func TestTTSListCatalogue(t *testing.T) {
	raw, err := os.ReadFile("../../sources/tts/models.json")
	if err != nil {
		t.Skip("no local catalogue (sources/tts/models.json)")
	}
	keys := map[string]bool{}
	var asList []string
	var asMap map[string]json.RawMessage
	switch {
	case json.Unmarshal(raw, &asList) == nil:
		for _, k := range asList {
			keys[strings.ToLower(strings.TrimSpace(k))] = true
		}
	case json.Unmarshal(raw, &asMap) == nil:
		for k := range asMap {
			keys[strings.ToLower(strings.TrimSpace(k))] = true
		}
	default:
		t.Skip("sources/tts/models.json has an unknown shape")
	}
	rs := shippedRules(t)
	missing := map[string]bool{}
	for _, bcCo := range ttsCompanies(rs) {
		for _, e := range ttsEntries(rs, bcCo) {
			for _, l := range rs.TTSList(ttsOne(rs, bcCo, e), TTSOptions{FootVersions: true, MountFigures: true}).Lines() {
				if !keys[l.Key] {
					missing[l.Key] = true
				}
			}
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d keys are not in the catalogue: %v", len(missing), slices.Sorted(mapKeys(missing)))
	}
}

// ttsCompanies are the companies of the rules in name order.
func ttsCompanies(rs *RuleSet) []BCCompany {
	var out []BCCompany
	for _, c := range rs.BC.companies {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b BCCompany) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func writeTTSGolden(t *testing.T, b *strings.Builder, label string, ex TTSExport) {
	t.Helper()
	if len(ex.Lines()) == 0 {
		t.Errorf("%s: no line", label)
	}
	fmt.Fprintf(b, "%s\n", label)
	for i, l := range ex.Lines() {
		fmt.Fprintf(b, "    %s\n", l.Text(i == 0))
	}
	for _, n := range ex.Notes {
		fmt.Fprintf(b, "    note %s: %s\n", n.Kind, n.Text)
	}
}
