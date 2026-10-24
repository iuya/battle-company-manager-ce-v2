package core

import (
	"slices"
	"strings"
	"testing"
)

func names(ks []UnitKeyword) []string {
	var out []string
	for _, k := range ks {
		out = append(out, k.Name)
	}
	return out
}

func kwOf(t *testing.T, rs *RuleSet, company string, in UnitKeywordsInput) UnitKeywords {
	t.Helper()
	p, ok := rs.BC.Profile(company)
	if !ok {
		t.Fatalf("no company %s", company)
	}
	return rs.UnitKeywords(p, in)
}

func TestUnitKeywordsCases(t *testing.T) {
	rs := shippedRules(t)
	tests := []struct {
		name    string
		company string
		in      UnitKeywordsInput
		model   []string
		row     []string
		mount   string
	}{
		{"goblin hulk gets Goblin in the race group", "Goblin-Town", UnitKeywordsInput{Type: "Goblin Hulk"}, []string{"Ogre", "Goblin", "Gundabad", "Warrior", "Infantry", "Monster"}, []string{"Ogre", "Goblin", "Gundabad", "Monster"}, ""},
		{"warg marauder is cavalry, not infantry", "Mirkwood", UnitKeywordsInput{Type: "Warg Marauder"}, nil, nil, ""},
		{"hunter orc warrior takes the fill-in", "Dol Guldur", UnitKeywordsInput{Type: "Hunter Orc Warrior"}, []string{"Orc", "Gundabad", "Warrior", "Infantry"}, []string{"Orc", "Gundabad"}, ""},
		{"vault warden team is its first member", "Durin's Folk", UnitKeywordsInput{Type: "Vault Warden Team"}, nil, nil, ""},
		{"unknown unit is empty", "Rohan", UnitKeywordsInput{Type: "No Such Unit"}, nil, nil, ""},
		{"unknown mount keeps its name", "Rohan", UnitKeywordsInput{Type: "Warrior of Rohan", Mount: "Pegasus"}, []string{"Man", "Rohan", "Warrior", "Infantry", "Cavalry"}, []string{"Man", "Rohan", "Cavalry"}, "Pegasus"},
		{"wanderer has Man in the race group", "Rohan", UnitKeywordsInput{Wanderer: "Wandering Swordsman"}, []string{"Man", "Infantry", "Hero"}, []string{"Man"}, ""},
		{"creature has types only", "Rohan", UnitKeywordsInput{Creature: "Baggage Pony"}, []string{"Beast", "Infantry", "Warrior"}, []string{"Beast"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := kwOf(t, rs, tc.company, tc.in)
			switch tc.name {
			case "warg marauder is cavalry, not infantry":
				if !slices.Contains(names(k.Model), "Cavalry") || slices.Contains(names(k.Model), "Infantry") {
					t.Fatalf("got %v", names(k.Model))
				}
				return
			case "vault warden team is its first member":
				first, _ := rs.Vanilla.Unit("Iron Shield Bearer")
				if len(k.Model) == 0 || !slices.Contains(names(k.Model), normalizeKeywords(first.Race)[0]) {
					t.Fatalf("got %v", names(k.Model))
				}
				return
			}
			if !slices.Equal(names(k.Model), tc.model) {
				t.Errorf("model = %v, want %v", names(k.Model), tc.model)
			}
			row, _ := k.Row(5)
			if !slices.Equal(names(row), tc.row) {
				t.Errorf("row = %v, want %v", names(row), tc.row)
			}
			if k.Mount != tc.mount {
				t.Errorf("mount = %q, want %q", k.Mount, tc.mount)
			}
		})
	}
}

func TestUnitKeywordsMountAndOnFoot(t *testing.T) {
	rs := shippedRules(t)
	k := kwOf(t, rs, "Rohan", UnitKeywordsInput{Type: "Warrior of Rohan", Mount: "Horse"})
	var cav UnitKeyword
	for _, w := range k.Model {
		if w.Name == "Cavalry" {
			cav = w
		}
	}
	if cav.Source != KeywordFromMount || cav.Hidden || cav.Off {
		t.Fatalf("cavalry = %+v", cav)
	}
	if k.Mount != "Horse" || !slices.Equal(names(k.MountKeys), []string{"Horse", "Mount"}) {
		t.Fatalf("mount = %q %v", k.Mount, names(k.MountKeys))
	}
	foot := k.OnFoot()
	if foot.Mount != "" || foot.MountKeys != nil {
		t.Errorf("on foot keeps the mount: %+v", foot)
	}
	for _, w := range foot.Model {
		if w.Name == "Cavalry" && !w.Off {
			t.Error("Cavalry should be Off on foot")
		}
	}
	if row, _ := foot.Row(9); slices.Contains(names(row), "Cavalry") {
		t.Errorf("row has Cavalry on foot: %v", names(row))
	}
	if !cavInRow(k) {
		t.Error("mounted rider should show Cavalry in the row")
	}
	for _, w := range k.Model { // the original is not changed
		if w.Off {
			t.Error("OnFoot changed the receiver")
		}
	}
	// A See Rider faction takes the rider's.
	g := kwOf(t, rs, "Rohan", UnitKeywordsInput{Type: "Warrior of Rohan", Mount: "Shadowfax"})
	if !slices.Contains(names(g.MountKeys), "Rohan") || slices.Contains(names(g.MountKeys), "See Rider") {
		t.Errorf("shadowfax keys = %v", names(g.MountKeys))
	}
	// Mounts the vanilla file leaves empty are filled in by data.mountFillIns.
	for _, m := range []string{"Brego", "War boar", "Elk", "Cart", "Moria Goblin Drum (Drum)"} {
		if got := kwOf(t, rs, "Rohan", UnitKeywordsInput{Type: "Warrior of Rohan", Mount: m}); !slices.Contains(names(got.MountKeys), "Mount") {
			t.Errorf("%s keys = %v", m, names(got.MountKeys))
		}
	}
	if vw := kwOf(t, rs, "Durin's Folk", UnitKeywordsInput{Type: "Vault Warden Team"}); !slices.Equal(names(vw.Model), []string{"Dwarf", "Khazad-dum", "Warrior", "Infantry"}) {
		t.Errorf("vault warden team = %v", names(vw.Model))
	}
	// A profile that is itself Cavalry keeps it on foot.
	if own := kwOf(t, rs, "Rohan", UnitKeywordsInput{Type: "Rider of Rohan"}); !cavInRow(own) || cavOff(own.OnFoot()) {
		t.Errorf("rider of rohan: %v", names(own.Model))
	}
}

func cavInRow(k UnitKeywords) bool {
	row, _ := k.Row(20)
	return slices.Contains(names(row), "Cavalry")
}

func cavOff(k UnitKeywords) bool {
	for _, w := range k.Model {
		if w.Name == "Cavalry" && w.Off {
			return true
		}
	}
	return false
}

func TestUnitKeywordsRow(t *testing.T) {
	mk := func(kind KeywordKind, hidden bool, names ...string) (out []UnitKeyword) {
		for _, n := range names {
			out = append(out, UnitKeyword{Name: n, Kind: kind, Hidden: hidden})
		}
		return
	}
	k := UnitKeywords{Model: slices.Concat(mk(KeywordRace, false, "Elf"), mk(KeywordFaction, false, "A", "B", "C", "D"), mk(KeywordType, true, "Hero"), mk(KeywordType, false, "Elite"))}
	k.Model = append(k.Model, UnitKeyword{Name: "Cavalry", Kind: KeywordType, Off: true})
	tests := []struct {
		max, shown, more int
	}{{5, 5, 1}, {6, 6, 0}, {9, 6, 0}, {2, 2, 4}, {0, 6, 0}}
	for _, tc := range tests {
		got, more := k.Row(tc.max)
		if len(got) != tc.shown || more != tc.more {
			t.Errorf("Row(%d) = %d, +%d; want %d, +%d", tc.max, len(got), more, tc.shown, tc.more)
		}
		if got[0].Name != "Elf" {
			t.Errorf("race first, got %s", got[0].Name)
		}
	}
	if (UnitKeywords{}).OnFoot().Mount != "" {
		t.Error("empty set")
	}
}

func TestNormalizeKeywords(t *testing.T) {
	tests := []struct {
		in, want []string
	}{
		{[]string{"Khazad-Dum", "Khazad-dum"}, []string{"Khazad-dum"}},
		{[]string{"Lothlorien", "Lothlórien"}, []string{"Lothlórien"}},
		{[]string{"The Iron Hills", "Iron Hills"}, []string{"Iron Hills"}},
		{[]string{"Fellowship, Dunedain"}, []string{"Fellowship", "Dunedain"}},
		{[]string{"*", "None", "Rohan"}, []string{"Rohan"}},
		{[]string{"hero", "Hero"}, []string{"Hero"}},
		{nil, nil},
	}
	for _, tc := range tests {
		if got := normalizeKeywords(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("%v -> %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestUnitKeywordsTextAndConcept(t *testing.T) {
	rs := shippedRules(t)
	k := kwOf(t, rs, "Rohan", UnitKeywordsInput{Type: "Warrior of Rohan", Mount: "Horse"})
	for _, w := range k.Model {
		if w.Text == "" || w.Concept == "" {
			t.Errorf("%s has no text or concept: %+v", w.Name, w)
		}
	}
	for _, w := range k.Model {
		if w.Name == "Rohan" && (w.Concept != "Rohan keyword" || !strings.Contains(w.Text, "faction keyword")) {
			t.Errorf("rohan = %+v", w)
		}
	}
}

func TestUnitKeywordsNilSafe(t *testing.T) {
	var rs *RuleSet
	if k := rs.UnitKeywords(CompanyProfile{}, UnitKeywordsInput{Type: "x", Mount: "Horse"}); !k.Empty() {
		t.Error("nil rules must give the empty set")
	}
	if k := (&RuleSet{}).UnitKeywords(CompanyProfile{}, UnitKeywordsInput{}); !k.Empty() {
		t.Error("no book must give the empty set")
	}
}

func TestBuildUnitKeywords(t *testing.T) {
	rs := shippedRules(t)
	rohan := rs.mustCompany(t, "Rohan")
	horse := buildUnit(rs, rohan, UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleWarrior, Bought: []string{"Horse"}}, CampaignSettings{})
	if horse.Keywords.Mount == "" || !cavInRow(horse.Keywords) {
		t.Errorf("a bought horse should make the rider Cavalry with a mount block: %+v", horse.Keywords)
	}
	plain := buildUnit(rs, rohan, UnitState{ID: 2, Base: "Warrior of Rohan", Role: RoleWarrior}, CampaignSettings{})
	if plain.Keywords.Mount != "" || cavInRow(plain.Keywords) {
		t.Errorf("a footman has no mount: %+v", plain.Keywords)
	}
	gt := rs.mustCompany(t, "Goblin-Town")
	hulk := buildUnit(rs, gt, UnitState{ID: 3, Base: "Goblin Hulk", Role: RoleWarrior}, CampaignSettings{})
	if !slices.Contains(names(hulk.Keywords.Group(KeywordRace)), "Goblin") {
		t.Errorf("hulk = %v", names(hulk.Keywords.Model))
	}
	mirk := rs.mustCompany(t, "Mirkwood")
	marauder := buildUnit(rs, mirk, UnitState{ID: 4, Base: "Warg Marauder", Role: RoleWarrior}, CampaignSettings{})
	if !cavInRow(marauder.Keywords) {
		t.Errorf("marauder = %v", names(marauder.Keywords.Model))
	}
	t.Logf("marauder mount %q %v", marauder.Keywords.Mount, names(marauder.Keywords.MountKeys))
}

func TestAliasKeywordsCountForLimits(t *testing.T) {
	rs := shippedRules(t)
	p, _ := rs.BC.Profile("Goblin-Town")
	v, a, ok := resolveUnitKeywords(rs, p, "Goblin Hulk")
	if !ok || a == nil || !hasKeyword(v, "Goblin") {
		t.Fatalf("hulk should count as a Goblin: %+v", v)
	}
	if plain, _ := rs.Vanilla.Unit("Gundabad Ogre"); hasKeyword(plain, "Goblin") {
		t.Error("the vanilla ogre must not change")
	}
}

func TestUnitKeywordsOfEveryUnitDoNotPanic(t *testing.T) {
	rs := shippedRules(t)
	for name := range rs.BC.companies {
		p, _ := rs.BC.Profile(name)
		for key := range p.Previews {
			_ = rs.UnitKeywords(p, UnitKeywordsInput{Type: strings.SplitN(key, "|", 2)[0]})
		}
	}
}

func TestPreviewsOfferingsAndFallenCarryKeywords(t *testing.T) {
	rs := shippedRules(t)
	// previews
	p, _ := rs.BC.Profile("Goblin-Town")
	p = rs.withPreviewKeywords(p.WithPreviews(rs.Vanilla))
	found := false
	for _, pv := range p.Previews {
		if pv.Name == "Goblin Hulk" {
			found = true
			if !slices.Contains(names(pv.Keywords.Group(KeywordRace)), "Goblin") || pv.Keywords.Mount != "" {
				t.Errorf("hulk preview = %v", names(pv.Keywords.Model))
			}
		}
		if len(pv.Keywords.Model) == 0 && pv.Points > 0 {
			t.Errorf("preview %s has no keywords", pv.Name)
		}
	}
	if !found {
		t.Fatal("no hulk preview")
	}
	// armoury offers
	sh := shop{rs: rs, bcC: rs.mustCompany(t, "Rohan"), d: CompanyDetail{}}
	for _, w := range sh.wanderers() {
		if !slices.Contains(names(w.Keywords.Group(KeywordRace)), "Man") && w.Name == "Wandering Swordsman" {
			t.Errorf("wanderer = %v", names(w.Keywords.Model))
		}
		if len(w.Keywords.Model) == 0 {
			t.Errorf("wanderer %s has no keywords", w.Name)
		}
	}
	for _, c := range sh.creatures() {
		if len(c.Keywords.Model) == 0 || len(c.Keywords.Group(KeywordRace)) != 0 {
			t.Errorf("creature %s = %v", c.Name, names(c.Keywords.Model))
		}
	}
	// fallen units: a snapshot stored before keywords existed gets them; none carries a mount block
	bcCo := rs.mustCompany(t, "Rohan")
	old := buildUnit(rs, bcCo, UnitState{ID: 9, Base: "Warrior of Rohan", Role: RoleWarrior, Bought: []string{"Horse"}}, CampaignSettings{})
	old.Keywords = UnitKeywords{}
	d := BuildDetail(rs, CampaignSettings{}, CompanyState{Cemetery: []FallenUnit{{Unit: old}}, Company: CampaignCompany{Faction: "Rohan"}})
	if len(d.Cemetery) != 1 || len(d.Cemetery[0].Unit.Keywords.Model) == 0 {
		t.Fatalf("fallen = %+v", d.Cemetery)
	}
}
