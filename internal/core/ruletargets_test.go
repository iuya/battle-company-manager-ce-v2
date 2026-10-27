package core

import (
	"slices"
	"strings"
	"testing"
)

func testTargets() RuleTargets {
	return NewRuleTargets([]RuleTarget{
		{Rule: "Hatred (X)", Base: "Hatred", Direction: TargetEnemyKeyword, Target: "enemy models with the {X} keyword"},
		{Rule: "Ancient Enemies (X)", Base: "Ancient Enemies", Direction: TargetEnemyKeyword, Target: "enemy models with the {X} keyword"},
		{Rule: "Terror (X)", Base: "Terror", Direction: TargetEnemyKeyword, Target: "enemy models with the {X} keyword"},
		{Rule: "Fury (X)", Base: "Fury", Direction: TargetFriendlyKeyword, Target: "friendly models with the {X} keywords", All: true},
		{Rule: "Terror", Direction: TargetEnemyAny, Target: "enemy models in general"},
		{Rule: "Bodyguard", Direction: TargetOwn},
		{Rule: "\"Slay their Horses!\"", Direction: TargetEnemyKeyword, Target: "enemy {X}s", Keywords: []string{"Mount"}, All: true, Needs: []string{"Cavalry"}},
		{Rule: "Against all odds", Direction: TargetEnemyKeyword, Target: "enemy {X} models", Keywords: []string{"Hero", "Monster"}},
	})
}

func TestParseRuleName(t *testing.T) {
	for _, c := range []struct {
		in   string
		name string
		alts [][]string
	}{
		{"Hatred (Goblin)(Orc)(Uruk-Hai)", "Hatred", [][]string{{"Goblin"}, {"Orc"}, {"Uruk-Hai"}}},
		{"Ancient Enemies (Dwarf)(Elf)", "Ancient Enemies", [][]string{{"Dwarf"}, {"Elf"}}},
		{"Fury (Mordor, Orc)", "Fury", [][]string{{"Mordor", "Orc"}}},
		{"Hatred (Orc)", "Hatred", [][]string{{"Orc"}}},
		{"Terror", "Terror", nil},
		{"Terror (2)", "Terror", nil},
		{"Harbinger of Evil (12\")", "Harbinger of Evil", nil},
		{"Hatred (Lothlorien)", "Hatred", [][]string{{"Lothlórien"}}},
	} {
		name, alts := ParseRuleName(c.in)
		if name != c.name || !slices.EqualFunc(alts, c.alts, slices.Equal[[]string]) || (len(alts) == 0) != (len(c.alts) == 0) {
			t.Errorf("ParseRuleName(%q) = %q %v, want %q %v", c.in, name, alts, c.name, c.alts)
		}
	}
}

func TestDescribeRuleTargets(t *testing.T) {
	tt := testTargets()
	for _, c := range []struct {
		label, works string
		against      int
	}{
		{"Hatred (Orc)", "enemy models with the Orc keyword", 1},
		{"Hatred (Goblin)(Orc)(Uruk-Hai)", "enemy models with the Goblin, Orc or Uruk-Hai keyword", 3},
		{"Ancient Enemies (Dwarf)(Elf)", "enemy models with the Dwarf or Elf keyword", 2},
		{"Fury (Mordor, Orc)", "friendly models with the Mordor and Orc keywords", 0},
		{"Terror", "enemy models in general", 0},
		{"Terror (2)", "enemy models in general", 0}, // a number is no keyword: the plain rule
		{"Terror (Man)", "enemy models with the Man keyword", 1},
		{"Against all odds", "enemy Hero or Monster models", 2},
		{"\"Slay their Horses!\"", "enemy Mounts", 1},
		{"Bodyguard", "", 0},
		{"Unknown rule", "", 0},
	} {
		w := tt.Describe(c.label)
		if w.Works != c.works || len(w.Against) != c.against {
			t.Errorf("Describe(%q) = %q, %d alternatives; want %q, %d", c.label, w.Works, len(w.Against), c.works, c.against)
		}
	}
	if w := (RuleTargets{}).Describe("Hatred (Orc)"); w.Works != "" || w.Against != nil {
		t.Errorf("an empty table describes nothing, got %+v", w)
	}
}

func unitWith(id int64, name string, keywords []string, rules ...string) TargetUnit {
	u := TargetUnit{ID: id, Name: name}
	for _, k := range keywords {
		u.Keywords = append(u.Keywords, strings.ToLower(k))
	}
	tt := testTargets()
	for _, r := range rules {
		w := tt.Describe(r)
		u.Rules = append(u.Rules, UnitRule{Name: r, Works: w.Works, Against: w.Against, Needs: w.Needs})
	}
	return u
}

func TestRuleMatches(t *testing.T) {
	mine := []TargetUnit{
		unitWith(1, "Grimbold", []string{"Man", "Rohan"}, "Hatred (Orc)", "Terror"),
		unitWith(2, "Hama", []string{"Man", "Rohan", "Cavalry"}, "Hatred (Goblin)(Orc)", "\"Slay their Horses!\""),
		unitWith(3, "Eowyn", []string{"Man", "Rohan"}),
	}
	theirs := []TargetUnit{
		unitWith(10, "Orc Warrior", []string{"Orc", "Mordor"}),
		unitWith(11, "Goblin", []string{"Goblin"}),
		unitWith(12, "Haradrim", []string{"Man", "Harad"}),
	}
	got := RuleMatches(mine, theirs)
	if len(got) != 2 {
		t.Fatalf("matches = %+v, want units 10 and 11 only (Terror names no keyword, Man is nobody's hatred)", got)
	}
	if m := got[10]; len(m) != 2 || m[0].Rule != "Hatred (Orc)" || !slices.Equal(m[0].Holders, []string{"Grimbold"}) ||
		m[1].Rule != "Hatred (Goblin)(Orc)" || !slices.Equal(m[1].Holders, []string{"Hama"}) {
		t.Errorf("Orc Warrior matches = %+v", m)
	}
	if m := got[11]; len(m) != 1 || m[0].Rule != "Hatred (Goblin)(Orc)" || m[0].Label() != "Hatred (Goblin)(Orc) applies" {
		t.Errorf("Goblin matches = %+v", m)
	}
	// the holders of one rule are all listed
	mine = append(mine, unitWith(4, "Erkenbrand", []string{"Man"}, "Hatred (Orc)"))
	if m := RuleMatches(mine, theirs[:1])[10][0]; !slices.Equal(m.Holders, []string{"Grimbold", "Erkenbrand"}) {
		t.Errorf("holders = %v", m.Holders)
	}
	// a rule that needs a keyword of its bearer does not work without it
	mounted := unitWith(5, "Rider", []string{"Man", "Cavalry"}, "Against all odds")
	hero := unitWith(20, "Enemy hero", []string{"Hero", "Orc"})
	if len(RuleMatches([]TargetUnit{mounted}, []TargetUnit{hero})[20]) != 1 {
		t.Error("Against all odds names an enemy Hero")
	}
	horses := unitWith(6, "Foot soldier", []string{"Man"}, "\"Slay their Horses!\"")
	mount := unitWith(21, "Mount", []string{"Mount"})
	if len(RuleMatches([]TargetUnit{horses}, []TargetUnit{mount})) != 0 {
		t.Error("Slay their Horses! needs a Cavalry bearer")
	}
	if len(RuleMatches([]TargetUnit{unitWith(7, "Rider", []string{"Cavalry"}, "\"Slay their Horses!\"")}, []TargetUnit{mount})[21]) != 1 {
		t.Error("a Cavalry bearer of Slay their Horses! has the rule work against a Mount")
	}
	if len(RuleMatches(nil, theirs)) != 0 || len(RuleMatches(mine, nil)) != 0 {
		t.Error("nobody on one side, no matches")
	}
}

func TestNewTargetUnitOnlyTheFieldCountsAndDismountedLosesCavalry(t *testing.T) {
	u := CompanyUnit{ID: 1, Name: "Hama", Type: "Rider of Rohan", Rules: []UnitRule{{Name: "Hatred (Orc)", Against: [][]string{{"Orc"}}}, {Name: "Timid", Mount: "Pony"}},
		Keywords: UnitKeywords{Mount: "Horse", Model: []UnitKeyword{{Name: "Man", Kind: KeywordRace}, {Name: "Cavalry", Kind: KeywordType}}}}
	if _, ok := NewTargetUnit(u, LiveUnit{UnitID: 1, Zone: ZoneCasualty}); ok {
		t.Error("a casualty takes no part")
	}
	if _, ok := NewTargetUnit(u, LiveUnit{UnitID: 1, Zone: ZoneRetreated}); ok {
		t.Error("a retreated unit takes no part")
	}
	if _, ok := NewTargetUnit(u, LiveUnit{}); ok {
		t.Error("a unit with no live state takes no part")
	}
	on, ok := NewTargetUnit(u, LiveUnit{UnitID: 1, Zone: ZoneField, HasMount: true, Mounted: true})
	if !ok || !slices.Contains(on.Keywords, "cavalry") || len(on.Rules) != 1 {
		t.Errorf("mounted = %+v, %v (the mount's rule is not the rider's)", on, ok)
	}
	off, _ := NewTargetUnit(u, LiveUnit{UnitID: 1, Zone: ZoneField, HasMount: true, Mounted: false})
	if slices.Contains(off.Keywords, "cavalry") || !slices.Contains(off.Keywords, "man") {
		t.Errorf("dismounted keywords = %v", off.Keywords)
	}
}

// Over the real data: every keyword a rule of a shipped company names is a keyword some model of the data has, a unit with the rule matches
// an enemy that has the keyword and not one that has none of them, and a ruleset without the table has no targets.
func TestRuleTargetsOverAllCompanies(t *testing.T) {
	rs := shippedRules(t)
	tt := rs.BC.RuleTargets()
	if tt.Empty() {
		t.Fatal("the shipped Battle Companies file has no data.ruleTargets")
	}
	known := map[string]bool{}
	var rules = map[string]RuleWorks{}
	for _, p := range rs.BC.Profiles() {
		p = rs.withPreviewKeywords(p.WithPreviews(rs.Vanilla))
		for _, pv := range p.Previews {
			for _, w := range pv.Keywords.Model {
				known[strings.ToLower(w.Name)] = true
			}
			for _, r := range pv.Rules {
				if w := tt.Describe(r); w.Works != "" {
					rules[r] = w
				}
			}
		}
	}
	for _, w := range []string{"hero", "warrior", "infantry", "cavalry", "monster"} {
		known[w] = true
	}
	hated := 0
	for label, w := range rules {
		if strings.Contains(w.Works, "{X}") {
			t.Errorf("%s: the text %q still has its placeholder", label, w.Works)
		}
		for _, alt := range w.Against {
			if strings.Contains(strings.ToLower(label), "slay their horses") {
				continue // a Mount is the mount's keyword, not a rider's
			}
			enemy := TargetUnit{ID: 9, Name: "enemy"}
			for _, k := range alt {
				enemy.Keywords = append(enemy.Keywords, strings.ToLower(k))
				if !known[strings.ToLower(k)] && !rs.Vanilla.IsRace(k) {
					t.Errorf("%s names %q, which no unit of the data has", label, k)
				}
			}
			holder := TargetUnit{ID: 1, Name: "holder", Rules: []UnitRule{{Name: label, Against: w.Against}}, Keywords: []string{"cavalry", "infantry"}}
			if len(RuleMatches([]TargetUnit{holder}, []TargetUnit{enemy})[9]) != 1 {
				t.Errorf("%s: an enemy with %v is not matched", label, alt)
			}
			if len(RuleMatches([]TargetUnit{holder}, []TargetUnit{{ID: 8, Keywords: []string{"nothing"}}})) != 0 {
				t.Errorf("%s: matched an enemy with no keyword of it", label)
			}
			hated++
		}
	}
	if hated < 10 {
		t.Errorf("only %d enemy-keyword alternatives found over the companies' units", hated)
	}
	t.Logf("%d rules with targets, %d keyword alternatives", len(rules), hated)
	for _, label := range []string{"Hatred (Orc)", "Ancient Enemies (Dwarf)(Elf)", "Terror"} {
		if tt.Describe(label).Works == "" {
			t.Errorf("%s has no works-against text", label)
		}
	}
}

// A unit built by the loader carries the text of its rules; a custom ruleset without data.ruleTargets has none.
func TestBuiltUnitsCarryWorksAgainst(t *testing.T) {
	rs := shippedRules(t)
	found := false
	for _, p := range rs.BC.Profiles() {
		p = p.WithPreviews(rs.Vanilla)
		st := CompanyState{Company: CampaignCompany{ID: 1, Name: "x", Faction: p.Name}}
		for i, pv := range p.Previews {
			if !slices.ContainsFunc(pv.Rules, func(r string) bool { return strings.HasPrefix(r, "Hatred (") }) {
				continue
			}
			st.Units = []UnitState{{ID: 1, Base: pv.Name, Role: RoleWarrior}}
			d := BuildDetail(rs, CampaignSettings{}, st)
			for _, r := range unitByID(t, d, 1).Rules {
				if strings.HasPrefix(r.Name, "Hatred (") {
					if r.Works == "" || len(r.Against) == 0 {
						t.Errorf("%s / %s: %+v has no target", p.Name, i, r)
					}
					found = true
				}
			}
			break
		}
	}
	if !found {
		t.Error("no built unit with a Hatred rule found")
	}
	var none RuleTargets
	u := CompanyUnit{Rules: []UnitRule{{Name: "Hatred (Orc)"}}}
	none.annotateUnit(&u)
	if u.Rules[0].Works != "" || u.Rules[0].Against != nil {
		t.Error("without the table nothing is annotated")
	}
}
