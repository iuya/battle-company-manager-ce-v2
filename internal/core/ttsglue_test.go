package core

import (
	"math"
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

	ttsGlueRings(t, label, rs, d, exp)
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

// ttsGlueRings pastes the version 2 payload of the same list (every kind and group on, the base radius on) and checks the rings the helper
// draws: one line per ring on the right figure, the radius of the data plus the base, none on a mount or a fallen unit, none doubled by a
// second Apply or a Re-apply.
func ttsGlueRings(t *testing.T, label string, rs *RuleSet, d CompanyDetail, exp TTSExport) {
	t.Helper()
	sel := AuraSelection{Draw: true, Declared: true, Conditional: true, Cast: true, Target: true, Large: true, Optional: true, Base: true}
	p := rs.TTSNames(d, exp, TTSNamesOptions{Rules: TTSRulesNone, Auras: sel})
	body, err := p.JSON()
	if err != nil {
		t.Fatal(err)
	}
	rings := 0
	for _, f := range p.Figures {
		rings += len(f.Auras)
	}
	if (rings > 0) != (p.Version == 2) {
		t.Fatalf("%s: %d rings in a version %d payload", label, rings, p.Version)
	}
	w := ttsfake.New(t, ttshelper.Script())
	figs := w.Spawn(exp.Text, 10, 20)
	if len(figs) != len(p.Figures) {
		t.Fatalf("%s: the spawner makes %d figures, the payload has %d", label, len(figs), len(p.Figures))
	}
	slices.Reverse(w.Objs)
	w.SetNames(string(body), "White")
	w.Load("")
	w.Press("applyNames", "White")
	w.Advance(10)
	mod := 0.05
	lines := func(o *ttsfake.Obj) []ttsfake.Line {
		var out []ttsfake.Line
		for _, l := range o.Lines {
			if l.Thickness != mod {
				out = append(out, l)
			}
		}
		return out
	}
	check := func(when string) {
		total := 0
		for i, f := range figs {
			want := p.Figures[i]
			got := lines(f)
			total += len(got)
			if len(got) != len(want.Auras) {
				t.Fatalf("%s %s: figure %d (%s, %s) has %d ring lines, the payload says %d", label, when, i, want.Line, want.Extra, len(got), len(want.Auras))
			}
			if want.Extra == "mount" && len(got) > 0 {
				t.Fatalf("%s %s: a ring on the mount %d", label, when, i)
			}
			base := 0.49
			if want.Base > 0 {
				base = want.Base
			}
			var wantR, gotR []float64
			for _, r := range want.Auras {
				wantR = append(wantR, math.Round((r.R+base-0.0225)*1000)/1000)
			}
			for _, l := range got {
				if len(l.Points) != 49 {
					t.Fatalf("%s %s: a ring of %d points", label, when, len(l.Points))
				}
				gotR = append(gotR, math.Round(math.Hypot(l.Points[0][0], l.Points[0][2])*1000)/1000)
			}
			slices.Sort(wantR)
			slices.Sort(gotR)
			for k := range wantR { // two kinds of one radius are drawn 0.03 apart so that they do not flicker
				if diff := gotR[k] - wantR[k]; diff < -0.001 || diff > 0.091 {
					t.Fatalf("%s %s: figure %d radii %v, want %v", label, when, i, gotR, wantR)
				}
			}
		}
		if total != rings || total > AurasPerPayload {
			t.Fatalf("%s %s: %d ring lines, payload %d", label, when, total, rings)
		}
	}
	check("apply")
	w.Press("applyNames", "White")
	w.Advance(10)
	check("second apply")
	w.Press("reapplyNames", "White")
	w.Advance(10)
	check("re-apply")
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

// TestTTSGlueRings: a company with rings of every kind, a rider with its on-foot version and mount, a creature and a fallen unit.
func TestTTSGlueRings(t *testing.T) {
	rs := shippedRules(t)
	d, _ := rohanRingCompany(t, rs)
	sorc := auraHeroOn(t, rs, "Rohan", "Warrior of Rohan", "Path of the Sorcerer", func(s *UnitState) {
		s.Spells = []PathSpell{{Name: "Aura of Command", Casting: 4}, {Name: "Blinding Light", Casting: 4}}
	})
	sorc.ID = 5
	d.Roster = append(d.Roster, sorc)
	d.Creatures = []CompanyCreature{{Name: "Falcon", OwnerID: 1, Rules: []string{"Keen Sight (Falcon)"}}}
	exp := rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
	ttsGlueRings(t, "rings", rs, d, exp)
	live := rs.TTSLiveList(d, TTSLive{Units: []LiveUnit{{UnitID: 3, Zone: ZoneCasualty}, {UnitID: 5, Zone: ZoneRetreated}}}, TTSOptions{FootVersions: true, MountFigures: true})
	ttsGlueRings(t, "rings live", rs, d, live)
	p := rs.TTSNames(d, live, TTSNamesOptions{Rules: TTSRulesNone, Auras: rs.DefaultAuraSelection()})
	n := 0
	for _, f := range p.Figures {
		n += len(f.Auras)
		if (f.Unit == 3 || f.Unit == 5) && len(f.Auras) > 0 {
			t.Errorf("fallen unit %d has rings", f.Unit)
		}
	}
	if n == 0 {
		t.Error("no rings at all")
	}
}
