package core

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/ttshelper"
	"github.com/iuya/battle-company-manager-ce-v2/internal/core/ttshelper/ttsfake"
)

// The glue between the app and the game: the real TTSList and TTSNames output for real companies goes through the Lua helper of
// internal/core/ttshelper, run in gopher-lua against a fake Tabletop Simulator, with the spawner's grid simulated for the list text.

// ttsGlue spawns the list of exp, pastes the payload, and checks Apply, a second Apply and Re-apply.
func ttsGlue(t *testing.T, label string, rs *RuleSet, d CompanyDetail, exp TTSExport) {
	t.Helper()
	p := rs.TTSNames(d, exp, TTSNamesOptions{Rules: TTSRulesShort})
	body, err := p.JSON()
	if err != nil {
		t.Fatal(err)
	}
	w := ttsfake.New(t, ttshelper.Script())
	figs := w.Spawn(exp.Text, 10, 20)
	if len(figs) != len(p.Figures) {
		t.Fatalf("%s: the spawner makes %d figures, the payload has %d", label, len(figs), len(p.Figures))
	}
	// the table lists the figures in no useful order
	slices.Reverse(w.Objs)
	w.SetNames(string(body), "White")
	n := len(figs)

	base := func(f TTSFigure) string {
		id := "bc-unit:" + glueItoa(f.Unit)
		if f.Extra != "" {
			id += ":" + f.Extra
		}
		return id
	}
	sharing, seen := map[string]int{}, map[string]int{}
	for _, f := range p.Figures {
		sharing[base(f)]++
	}
	w.Press("applyNames", "White")
	for i, f := range figs {
		want := p.Figures[i]
		id := base(want)
		if sharing[id] > 1 { // the parts of a composite unit share the id: the helper numbers them
			seen[id]++
			id += "/" + glueItoa(int64(seen[id]))
		}
		if f.Name != want.Name || f.Desc != want.Desc || f.Notes != id || !f.Tags["BCNamed"] {
			t.Fatalf("%s: figure %d (%s, occurrence %d) got name %q notes %q, want %q %q", label, i, want.Line, want.Occurrence, f.Name, f.Notes, want.Name, id)
		}
	}
	out := w.Texts()
	if strings.Contains(out, "Not found") || strings.Contains(out, "left alone") || !strings.Contains(out, "Named "+glueItoa(int64(n))+" of "+glueItoa(int64(n))+" figures.") {
		t.Fatalf("%s: messages after Apply:\n%s", label, out)
	}

	calls := make([]int, n)
	for i, f := range figs {
		calls[i] = f.Calls
	}
	w.Press("applyNames", "White")
	for i, f := range figs {
		if f.Calls != calls[i] {
			t.Fatalf("%s: figure %d touched by the second Apply", label, i)
		}
	}
	if out := w.Texts(); !strings.Contains(out, "0 new, "+glueItoa(int64(n))+" already named.") || strings.Contains(out, "Not found") {
		t.Fatalf("%s: messages after the second Apply:\n%s", label, out)
	}

	w.Press("reapplyNames", "White")
	for i, f := range figs {
		if f.Calls <= calls[i] || f.Name != p.Figures[i].Name {
			t.Fatalf("%s: figure %d not renamed by Re-apply (%d calls)", label, i, f.Calls)
		}
	}
	if out := w.Texts(); !strings.Contains(out, "Named "+glueItoa(int64(n))+" of "+glueItoa(int64(n))+" figures.") {
		t.Fatalf("%s: messages after Re-apply:\n%s", label, out)
	}
}

func glueItoa(n int64) string { return strconv.FormatInt(n, 10) }

// TestTTSGlueAllCompanies runs every unit the 37 companies can have, in rosters of 12 (the first unit leads), twice: once as they are,
// once with every unit doubled so that identical neighbours merge into Nx lines.
func TestTTSGlueAllCompanies(t *testing.T) {
	rs := shippedRules(t)
	opts := TTSOptions{FootVersions: true, MountFigures: true}
	for _, bcCo := range ttsCompanies(rs) {
		var units []CompanyUnit
		for _, e := range ttsEntries(rs, bcCo) {
			units = append(units, ttsOne(rs, bcCo, e).Roster[0])
		}
		for _, double := range []bool{false, true} {
			chunk := 12
			if double {
				chunk = 6
			}
			for start := 0; start < len(units); start += chunk {
				var roster []CompanyUnit
				for _, u := range units[start:min(start+chunk, len(units))] {
					for k := 0; k < map[bool]int{false: 1, true: 2}[double]; k++ {
						u.ID = int64(len(roster) + 1)
						if len(roster) == 0 {
							u.Role = RoleLeader
						}
						roster = append(roster, u)
					}
				}
				d := CompanyDetail{CampaignCompany: CampaignCompany{Name: "Glue " + bcCo.Name, Faction: bcCo.Name}, Roster: roster}
				exp := rs.TTSList(d, opts)
				if len(exp.Lines()) == 0 {
					continue
				}
				label := bcCo.Name
				if double {
					label += " (doubled)"
				}
				ttsGlue(t, label, rs, d, exp)
			}
		}
	}
}

// TestTTSGlueMergedLines makes sure the merged-line path is really exercised, and the identical-nickname groups are real.
func TestTTSGlueMergedLines(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	var units []CompanyUnit
	for id := int64(1); id <= 9; id++ {
		units = append(units, ttsUnit(t, rs, f, id, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil))
	}
	units[0].Role = RoleLeader
	d := ttsDetail(f, units...)
	exp := rs.TTSList(d, TTSOptions{})
	if !strings.Contains(exp.Text, "(8x Warrior of Minas Tirith: Bow)") {
		t.Fatalf("not merged:\n%s", exp.Text)
	}
	ttsGlue(t, "merged", rs, d, exp)
}

// TestTTSGlueLiveFallenWarband: the live list's second warband (fallen units) carries the same nicknames as the first.
func TestTTSGlueLiveFallenWarband(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	var units []CompanyUnit
	for id := int64(1); id <= 7; id++ {
		units = append(units, ttsUnit(t, rs, f, id, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil))
	}
	units[0].Role = RoleLeader
	d := ttsDetail(f, units...)
	live := TTSLive{Units: []LiveUnit{{UnitID: 2, Zone: ZoneCasualty}, {UnitID: 3, Zone: ZoneRetreated}}, Absent: []int64{7}}
	exp := rs.TTSLiveList(d, live, TTSOptions{})
	if len(exp.Warbands) != 2 {
		t.Fatalf("warbands %d", len(exp.Warbands))
	}
	ttsGlue(t, "live", rs, d, exp)
}

// TestTTSGlueNamedCompanies: the cavalry of Rohan and the warg riders of Angmar, whatever the other tests cover.
func TestTTSGlueNamedCompanies(t *testing.T) {
	rs := shippedRules(t)
	seen := map[string]bool{}
	for _, bcCo := range ttsCompanies(rs) {
		for _, key := range []string{"Rohan", "Angmar"} {
			if !strings.Contains(bcCo.Name, key) || seen[key] {
				continue
			}
			seen[key] = true
			var roster []CompanyUnit
			for _, e := range ttsEntries(rs, bcCo) {
				u := ttsOne(rs, bcCo, e).Roster[0]
				if strings.Contains(u.Type, "Rider") || strings.Contains(u.Type, "Warg") || strings.Contains(u.Type, "Knight") || len(roster) == 0 {
					for k := 0; k < 3; k++ {
						v := u
						v.ID = int64(len(roster) + 1)
						roster = append(roster, v)
					}
				}
			}
			d := CompanyDetail{CampaignCompany: CampaignCompany{Name: key, Faction: bcCo.Name}, Roster: roster}
			exp := rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
			ttsGlue(t, bcCo.Name, rs, d, exp)
			extras := 0
			for _, l := range exp.Lines() {
				if l.Extra != "" {
					extras++
				}
			}
			t.Logf("%s: %d lines, %d extra-figure lines", bcCo.Name, len(exp.Lines()), extras)
		}
	}
	if len(seen) != 2 {
		t.Errorf("companies found: %v", seen)
	}
}
