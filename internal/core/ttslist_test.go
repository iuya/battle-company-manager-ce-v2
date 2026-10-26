package core

import (
	"slices"
	"strings"
	"testing"
)

// ttsUnit builds a company unit of the shipped rules.
func ttsUnit(t *testing.T, rs *RuleSet, faction string, id int64, base string, role UnitRole, gear, bought []string) CompanyUnit {
	t.Helper()
	co, ok := rs.BC.Company(faction)
	if !ok {
		t.Fatalf("no company %s", faction)
	}
	return buildUnit(rs, co, UnitState{ID: id, Base: base, Role: role, Gear: gear, Bought: bought}, CampaignSettings{})
}

// ttsWithModels changes data.ttsModels of the shared shipped rules for the rest of the test.
func ttsWithModels(t *testing.T, rs *RuleSet, change func(*TTSModels)) *RuleSet {
	t.Helper()
	saved := rs.BC.ttsModels
	m := saved
	change(&m)
	rs.BC.ttsModels = m
	t.Cleanup(func() { rs.BC.ttsModels = saved })
	return rs
}

func ttsDetail(faction string, units ...CompanyUnit) CompanyDetail {
	return CompanyDetail{CampaignCompany: CampaignCompany{Faction: faction}, Roster: units}
}

func ttsKinds(ex TTSExport) []string {
	var out []string
	for _, n := range ex.Notes {
		out = append(out, n.Kind)
	}
	return out
}

func TestTTSListFormatAndMerging(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	leader := ttsUnit(t, rs, f, 1, "Warrior of Minas Tirith", RoleLeader, []string{"Spear", "Shield"}, nil)
	var warriors []CompanyUnit
	for id := int64(2); id <= 4; id++ {
		warriors = append(warriors, ttsUnit(t, rs, f, id, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil))
	}
	other := ttsUnit(t, rs, f, 5, "Warrior of Minas Tirith", RoleWarrior, []string{"Spear", "Shield"}, nil)
	again := ttsUnit(t, rs, f, 6, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil)
	roster := append(append([]CompanyUnit{leader}, warriors...), other, again)
	ex := rs.TTSList(ttsDetail(f, roster...), TTSOptions{})
	want := "(Warrior of Minas Tirith: Shield and Spear)\n" +
		"    (3x Warrior of Minas Tirith: Bow)\n" +
		"    (1x Warrior of Minas Tirith: Shield and Spear)\n" +
		"    (1x Warrior of Minas Tirith: Bow)"
	if ex.Text != want {
		t.Fatalf("text:\n%s\nwant:\n%s", ex.Text, want)
	}
	if len(ex.Warbands) != 1 || ex.Models != 6 || ex.ExtraFigures != 0 {
		t.Errorf("warbands %d models %d extra %d", len(ex.Warbands), ex.Models, ex.ExtraFigures)
	}
	if got := ex.Lines()[1].UnitIDs; !slices.Equal(got, []int64{2, 3, 4}) {
		t.Errorf("merged ids %v", got)
	}
	// 6 models at the default 50%: Break Point 3.0
	if ex.BreakPercent != 50 || ex.BreakPoint != "3.0" {
		t.Errorf("break %d%% %s", ex.BreakPercent, ex.BreakPoint)
	}
}

func TestTTSListHeroLineIsAlwaysFirstAndNeverMerged(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	a := ttsUnit(t, rs, f, 1, "Warrior of Minas Tirith", RoleLeader, []string{"Bow"}, nil)
	b := ttsUnit(t, rs, f, 2, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil)
	ex := rs.TTSList(ttsDetail(f, a, b), TTSOptions{})
	if ex.Text != "(Warrior of Minas Tirith: Bow)\n    (1x Warrior of Minas Tirith: Bow)" {
		t.Errorf("text %q", ex.Text)
	}
	// a unit without a model cannot head the list: the next one does, and the first line never has a count
	ghost := CompanyUnit{ID: 9, Type: "Nobody At All", Role: RoleLeader}
	ex = rs.TTSList(ttsDetail(f, ghost, a, b), TTSOptions{})
	if !strings.HasPrefix(ex.Text, "(Warrior of Minas Tirith: Bow)\n") || !slices.Contains(ttsKinds(ex), TTSNoModel) || ex.Notes[0].UnitID != 9 {
		t.Errorf("text %q notes %+v", ex.Text, ex.Notes)
	}
	// an empty company has no list
	if ex := rs.TTSList(ttsDetail(f), TTSOptions{}); ex.Text != "" || len(ex.Warbands) != 0 || ex.Models != 0 {
		t.Errorf("empty: %+v", ex)
	}
}

func TestTTSListOptionsAreSortedAndMapped(t *testing.T) {
	rs := shippedRules(t)
	// Bow and Spear on a unit whose options are all in one group cannot both be taken: one is kept, the other noted
	u := ttsUnit(t, rs, "Muster of Isengard", 1, "Isengard Orc Warrior", RoleWarrior, []string{"Orc bow"}, nil)
	ex := rs.TTSList(ttsDetail("Muster of Isengard", u), TTSOptions{})
	if ex.Text != "(Isengard Orc Warrior: Bow)" { // the mod's spelling comes from data.ttsModels.names
		t.Errorf("text %q", ex.Text)
	}
	h := ttsUnit(t, rs, "Rohan", 2, "Warrior of Rohan", RoleWarrior, []string{"Shield", "Bow"}, nil)
	ex = rs.TTSList(ttsDetail("Rohan", h), TTSOptions{})
	if len(ex.Lines()) != 1 || len(ex.Lines()[0].Options) != 1 || !slices.Contains(ttsKinds(ex), TTSOptionDropped) {
		t.Errorf("two options of one group: %+v %+v", ex.Lines(), ex.Notes)
	}
	// Key is lower case and is what the helper looks up
	if l := ex.Lines()[0]; l.Key != strings.ToLower(l.Name+": "+strings.Join(l.Options, ", ")) {
		t.Errorf("key %q", l.Key)
	}
}

func TestTTSCleanStripsBracketsKeepsAccents(t *testing.T) {
	for in, want := range map[string]string{
		"Troll Brute (Troll)":    "Troll Brute Troll",
		"Watcher of Kârna":       "Watcher of Kârna",
		"  Aragorn   (Strider) ": "Aragorn Strider",
	} {
		if got := ttsClean(in); got != want {
			t.Errorf("ttsClean(%q) = %q, want %q", in, got, want)
		}
	}
	l := newTTSLine(1, "Beorn (Man)", []string{"Zeta", "alpha", "Beta (x)"}, "")
	if l.Name != "Beorn Man" || !slices.Equal(l.Options, []string{"alpha", "Beta x", "Zeta"}) || l.Key != "beorn man: alpha, beta x, zeta" {
		t.Errorf("%+v", l)
	}
}

// All the riders of the data: a rider with states is one line, the others get their on-foot line when the switch is on.
func TestTTSListRiders(t *testing.T) {
	rs := shippedRules(t)
	models := rs.BC.TTSModels()
	if len(models.Riders) != 18 {
		t.Fatalf("%d riders in the data, want 18", len(models.Riders))
	}
	for _, r := range models.Riders {
		t.Run(r.Unit, func(t *testing.T) {
			u := ttsUnit(t, rs, "Minas Tirith", 1, r.Unit, RoleWarrior, nil, nil)
			off := rs.TTSList(ttsDetail("Minas Tirith", u), TTSOptions{})
			on := rs.TTSList(ttsDetail("Minas Tirith", u), TTSOptions{FootVersions: true})
			for _, ex := range []TTSExport{off, on} {
				for _, l := range ex.Lines() {
					if l.Name == "" || strings.HasPrefix(l.Name, ":") {
						t.Fatalf("empty name: %+v", l)
					}
				}
			}
			if len(off.Lines()) != 1 || off.Lines()[0].Name != r.Unit || off.ExtraFigures != 0 {
				t.Fatalf("switch off: %+v", off.Lines())
			}
			switch {
			case r.States:
				if len(on.Lines()) != 1 || !slices.Contains(ttsKinds(on), TTSHasStates) {
					t.Errorf("states: lines %+v notes %v", on.Lines(), ttsKinds(on))
				}
			case r.FootModel != "":
				lines := on.Lines()
				if len(lines) < 2 || lines[1].Name != r.FootModel || lines[1].Extra != "foot" || lines[1].UnitIDs[0] != 1 || on.ExtraFigures != 1 {
					t.Errorf("foot: %+v", lines)
				}
			}
		})
	}
	// the Mirkwood Elf Knight (Halls of Thranduil) is a rider with states
	alias := ttsUnit(t, rs, "Halls of Thranduil", 1, "Mirkwood Elf Knight", RoleWarrior, nil, nil)
	ex := rs.TTSList(ttsDetail("Halls of Thranduil", alias), TTSOptions{FootVersions: true})
	if len(ex.Lines()) != 1 || ex.Lines()[0].Name != "Mirkwood Elf Knight" || !slices.Contains(ttsKinds(ex), TTSHasStates) {
		t.Errorf("Mirkwood Elf Knight: %+v %v", ex.Lines(), ex.Notes)
	}
}

// A rider the mod has no foot figure for is a note, never an empty or broken line.
func TestTTSListRiderWithoutFootModel(t *testing.T) {
	rs := shippedRules(t)
	rs2 := ttsWithModels(t, rs, func(m *TTSModels) {
		m.Riders = slices.Clone(m.Riders)
		for i, r := range m.Riders {
			if r.Unit == "Iron Hills Goat Rider" {
				m.Riders[i].FootModel = ""
			}
		}
	})
	u := ttsUnit(t, rs, "Minas Tirith", 7, "Iron Hills Goat Rider", RoleWarrior, nil, nil)
	ex := rs2.TTSList(ttsDetail("Minas Tirith", u), TTSOptions{FootVersions: true, MountFigures: true})
	if len(ex.Lines()) != 1 || ex.Lines()[0].Name != "Iron Hills Goat Rider" || ex.ExtraFigures != 0 {
		t.Errorf("lines %+v", ex.Lines())
	}
	if !slices.ContainsFunc(ex.Notes, func(n TTSNote) bool {
		return n.Kind == TTSNoModel && n.UnitID == 7 && strings.Contains(n.Text, "on-foot")
	}) {
		t.Errorf("notes %+v", ex.Notes)
	}
}

func TestTTSListMountFigures(t *testing.T) {
	rs := shippedRules(t)
	f := "Mordor"
	// a Mordor Warg Rider: states on some keys only, so the on-foot figure and the Fell Warg both come
	rider := ttsUnit(t, rs, f, 1, "Mordor Warg Rider", RoleWarrior, nil, nil)
	all := rs.TTSList(ttsDetail(f, rider), TTSOptions{FootVersions: true, MountFigures: true})
	if ls := all.Lines(); len(ls) != 3 || ls[1].Extra != "foot" || ls[2].Name != "Fell Warg" || ls[2].Extra != "mount" || all.ExtraFigures != 2 || all.Models != 1 {
		t.Errorf("all: %+v", ls)
	}
	if only := rs.TTSList(ttsDetail(f, rider), TTSOptions{MountFigures: true}); len(only.Lines()) != 2 || only.Lines()[1].Extra != "mount" {
		t.Errorf("mount only: %+v", only.Lines())
	}
	// Hunter Orc Warg Rider: states carry the Fell Warg, so nothing is added
	hunter := ttsUnit(t, rs, f, 2, "Hunter Orc Warg Rider", RoleWarrior, nil, nil)
	if ex := rs.TTSList(ttsDetail(f, hunter), TTSOptions{FootVersions: true, MountFigures: true}); len(ex.Lines()) != 1 {
		t.Errorf("hunter: %+v", ex.Lines())
	}
	// a foot unit that bought a Warg gets the figure; one that bought a Horse does not (Attacks 0)
	warg := ttsUnit(t, rs, f, 3, "Mordor Orc Warrior", RoleWarrior, []string{"Shield"}, []string{"Warg"})
	horse := ttsUnit(t, rs, f, 4, "Mordor Orc Warrior", RoleWarrior, []string{"Shield"}, []string{"Horse"})
	ex := rs.TTSList(ttsDetail(f, warg, horse), TTSOptions{MountFigures: true})
	var mounts int
	for _, l := range ex.Lines() {
		if l.Extra == "mount" {
			mounts++
			if l.Name != "Fell Warg" || l.UnitIDs[0] != 3 {
				t.Errorf("mount line %+v", l)
			}
		}
	}
	if mounts != 1 {
		t.Errorf("mount figures %d: %+v", mounts, ex.Lines())
	}
	if off := rs.TTSList(ttsDetail(f, warg, horse), TTSOptions{}); off.ExtraFigures != 0 {
		t.Errorf("switch off: %+v", off.Lines())
	}
}

func TestTTSListRidersMergeWithTheirExtraFigures(t *testing.T) {
	rs := shippedRules(t)
	f := "Mordor"
	lead := ttsUnit(t, rs, f, 1, "Mordor Orc Warrior", RoleLeader, []string{"Shield"}, nil)
	var units []CompanyUnit
	units = append(units, lead)
	for id := int64(2); id <= 4; id++ {
		units = append(units, ttsUnit(t, rs, f, id, "Mordor Warg Rider", RoleWarrior, nil, nil))
	}
	ex := rs.TTSList(ttsDetail(f, units...), TTSOptions{FootVersions: true, MountFigures: true})
	ls := ex.Lines()
	if len(ls) != 4 || ls[1].Count != 3 || ls[2].Count != 3 || ls[3].Count != 3 || ex.Models != 4 || ex.ExtraFigures != 6 {
		t.Fatalf("%+v", ls)
	}
}

func TestTTSListStandInsAndCreatures(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	lead := ttsUnit(t, rs, f, 1, "Warrior of Minas Tirith", RoleLeader, []string{"Bow"}, nil)
	sage := buildUnit(rs, ttsBCCompany(t, rs, f), UnitState{ID: 2, Base: "Wandering Sage", Wanderer: "Wandering Sage", Role: RoleWarrior}, CampaignSettings{})
	d := ttsDetail(f, lead, sage)
	d.Creatures = []CompanyCreature{{Name: "Baggage Pony", OwnerID: 1}, {Name: "Mystery Beast", OwnerID: 1}}
	ex := rs.TTSList(d, TTSOptions{})
	want := "(Warrior of Minas Tirith: Bow)\n    (1x Bill the Pony: )\n    (1x Malbeth the Seer: )"
	if ex.Text != want {
		t.Errorf("text:\n%s", ex.Text)
	}
	kinds := ttsKinds(ex)
	if strings.Count(strings.Join(kinds, ","), TTSShownAs) != 2 || !slices.Contains(kinds, TTSNoModel) {
		t.Errorf("notes %+v", ex.Notes)
	}
	// a wanderer the data has no stand-in for is a note, not a line
	rs2 := ttsWithModels(t, rs, func(m *TTSModels) { m.StandIns = nil })
	if ex := rs2.TTSList(ttsDetail(f, lead, sage), TTSOptions{}); len(ex.Lines()) != 1 || !slices.Contains(ttsKinds(ex), TTSNoModel) {
		t.Errorf("no stand-in: %+v %+v", ex.Lines(), ex.Notes)
	}
}

func ttsBCCompany(t *testing.T, rs *RuleSet, name string) BCCompany {
	t.Helper()
	co, ok := rs.BC.Company(name)
	if !ok {
		t.Fatalf("no company %s", name)
	}
	return co
}

func TestTTSListBreakPoint(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	var units []CompanyUnit
	for id := int64(1); id <= 13; id++ {
		units = append(units, ttsUnit(t, rs, f, id, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil))
	}
	d := ttsDetail(f, units...)
	if ex := rs.TTSList(d, TTSOptions{}); ex.Models != 13 || ex.BreakPercent != 50 || ex.BreakPoint != "6.5" {
		t.Errorf("default: %d %d %s", ex.Models, ex.BreakPercent, ex.BreakPoint)
	}
	d.BreakPointPercent = 66 // Muster of Isengard
	if ex := rs.TTSList(d, TTSOptions{}); ex.BreakPoint != "8.5" {
		t.Errorf("66%%: %s", ex.BreakPoint)
	}
	// the extra figures are not company models
	rider := ttsUnit(t, rs, "Mordor", 1, "Mordor Warg Rider", RoleWarrior, nil, nil)
	ex := rs.TTSList(ttsDetail("Mordor", rider), TTSOptions{FootVersions: true, MountFigures: true})
	if ex.Models != 1 || ex.ExtraFigures != 2 || ex.BreakPoint != "0.5" {
		t.Errorf("rider: %d %d %s", ex.Models, ex.ExtraFigures, ex.BreakPoint)
	}
}

func TestTTSLiveListSecondWarband(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	var units []CompanyUnit
	for id := int64(1); id <= 5; id++ {
		units = append(units, ttsUnit(t, rs, f, id, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil))
	}
	units[0].Role = RoleLeader
	d := ttsDetail(f, units...)
	plain := rs.TTSList(d, TTSOptions{})
	live := TTSLive{
		Units:  []LiveUnit{{UnitID: 1, Zone: ZoneField}, {UnitID: 2, Zone: ZoneCasualty}, {UnitID: 3, Zone: ZoneRetreated}, {UnitID: 4, Zone: ZoneField}},
		Absent: []int64{5},
	}
	ex := rs.TTSLiveList(d, live, TTSOptions{})
	if len(ex.Warbands) != 2 || ex.Warbands[0].Fallen || !ex.Warbands[1].Fallen {
		t.Fatalf("warbands %+v", ex.Warbands)
	}
	want := "(Warrior of Minas Tirith: Bow)\n    (1x Warrior of Minas Tirith: Bow)\n\n" +
		"(Warrior of Minas Tirith: Bow)\n    (2x Warrior of Minas Tirith: Bow)"
	if ex.Text != want {
		t.Errorf("text:\n%s", ex.Text)
	}
	if got := ex.Warbands[1].Lines[0].UnitIDs; !slices.Equal(got, []int64{2}) {
		t.Errorf("second hero line %v", got)
	}
	if ex.Models != 5 || !slices.Contains(ttsKinds(ex), TTSFallen) {
		t.Errorf("models %d notes %v", ex.Models, ttsKinds(ex))
	}
	// the plain export knows nothing of the match
	if len(plain.Warbands) != 1 || strings.Contains(plain.Text, "\n\n") || slices.Contains(ttsKinds(plain), TTSFallen) {
		t.Errorf("plain: %q", plain.Text)
	}
	// nobody out: one warband, no note; everybody out: only the second warband
	if ex := rs.TTSLiveList(d, TTSLive{}, TTSOptions{}); ex.Text != plain.Text || slices.Contains(ttsKinds(ex), TTSFallen) {
		t.Errorf("all in the field: %q", ex.Text)
	}
	all := rs.TTSLiveList(d, TTSLive{Absent: []int64{1, 2, 3, 4, 5}}, TTSOptions{})
	if len(all.Warbands) != 1 || !all.Warbands[0].Fallen || all.Models != 5 || !slices.Contains(ttsKinds(all), TTSFallen) {
		t.Errorf("all out: %+v", all.Warbands)
	}
}
