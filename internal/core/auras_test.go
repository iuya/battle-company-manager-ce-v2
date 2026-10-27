package core

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The auras against the shipped rules (docs/aura-ranges.md, data.auras).

// withoutAuras empties data.auras of the shared shipped rules for the rest of the test.
func withoutAuras(t *testing.T, rs *RuleSet) {
	t.Helper()
	saved := rs.BC.auras
	rs.BC.auras = AuraTable{}
	t.Cleanup(func() { rs.BC.auras = saved })
}

func auraCompany(t *testing.T, rs *RuleSet, name string) BCCompany {
	t.Helper()
	co, ok := rs.BC.Company(name)
	if !ok {
		t.Fatalf("no company %s", name)
	}
	return co
}

func auraHeroOn(t *testing.T, rs *RuleSet, company, base, path string, mod func(*UnitState)) CompanyUnit {
	t.Helper()
	co := auraCompany(t, rs, company)
	var p PathProgression
	for _, x := range rs.BC.Progression().Paths {
		if x.Name == path {
			p = x
		}
	}
	st := UnitState{ID: 1, Base: base, Role: RoleHero, Path: path, PathRules: 99, Bonus: HeroBonus(nil)}
	if p.StartAction != "" {
		st.Actions = []string{p.StartAction}
	}
	if mod != nil {
		mod(&st)
	}
	return buildUnit(rs, co, st, CampaignSettings{})
}

func auraSig(as []Aura) []string {
	var out []string
	for _, a := range as {
		out = append(out, fmt.Sprintf("%g %s %s", a.Radius, a.Kind, a.Label))
	}
	return out
}

func wantAuras(t *testing.T, label string, got []Aura, want ...string) {
	t.Helper()
	if g := auraSig(got); !slices.Equal(g, want) {
		t.Errorf("%s:\n got  %q\n want %q", label, g, want)
	}
}

// TestUnitAurasItems: every match type of an item, with the bearers that may (or may not) have it.
func TestUnitAurasItems(t *testing.T) {
	rs := shippedRules(t)
	co := auraCompany(t, rs, "Rohan")
	bought := func(base string, role UnitRole, bought ...string) CompanyUnit {
		return buildUnit(rs, co, UnitState{ID: 1, Base: base, Role: role, Bought: bought}, CampaignSettings{})
	}
	gear := func(base string, role UnitRole, gear ...string) CompanyUnit {
		return buildUnit(rs, co, UnitState{ID: 1, Base: base, Role: role, Gear: gear}, CampaignSettings{})
	}
	tests := []struct {
		name string
		u    CompanyUnit
		want []string
	}{
		{"no item, no aura", gear("Warrior of Rohan", RoleWarrior), nil},
		{"a vanilla Banner option", gear("Warrior of Rohan", RoleWarrior, "Banner"), []string{"3 banner Banner"}},
		{"a banner bought in the armoury", bought("Warrior of Rohan", RoleWarrior, "Banner"), []string{"3 banner Banner"}},
		{"an exchange option names the banner", gear("Rider of Rohan", RoleWarrior, "Banner"), []string{"3 banner Banner"}},
		{"Company Standard is a banner ring: one ring with the banner", bought("Warrior of Rohan", RoleWarrior, "Company Standard"), []string{"3 banner Company Standard"}},
		{"banner and standard share one ring", gear("Warrior of Rohan", RoleWarrior, "Banner"), []string{"3 banner Banner"}},
		{"Company Instrument", bought("Warrior of Rohan", RoleWarrior, "Company Instrument"), []string{"3 support Company Instrument"}},
		{"Company Drum", bought("Warrior of Rohan", RoleWarrior, "Company Drum"), []string{"12 command War Drum"}},
		{"the war horn of a Warrior is the 24 inch ring only", gear("Warrior of Rohan", RoleWarrior, "War horn, shield, and throwing spears"), []string{"24 command War Horn"}},
		{"the war horn of a Hero adds the 9 inch Stand Fast extension", gear("Warrior of Rohan", RoleHero, "War horn, shield, and throwing spears"),
			[]string{"6 command Stand Fast, With Me!, Loose", "9 command War Horn: Stand Fast +3\"", "24 command War Horn"}},
		{"a Hero alone has Stand Fast", gear("Warrior of Rohan", RoleHero), []string{"6 command Stand Fast, With Me!, Loose"}},
		{"a Warrior has no Stand Fast", gear("Warrior of Rohan", RoleWarrior), nil},
	}
	for _, tc := range tests {
		wantAuras(t, tc.name, rs.UnitAuras(tc.u), tc.want...)
	}
	// the Company Standard and a Banner on one unit are one ring with both labels
	both := bought("Warrior of Rohan", RoleWarrior, "Company Standard")
	both.Equipment = append(both.Equipment, Equipment{Name: "Banner"})
	wantAuras(t, "banner and standard", rs.UnitAuras(both), "3 banner Banner, Company Standard")
	if as := rs.UnitAuras(both); len(as) == 1 && !slices.Equal(as[0].Rules, []string{"Banner", "Company Standard"}) {
		t.Errorf("rules %q", as[0].Rules)
	}
	// an item an injury disables is not an aura
	dis := gear("Warrior of Rohan", RoleWarrior, "Banner")
	for i := range dis.Equipment {
		if dis.Equipment[i].Name == "Banner" {
			dis.Equipment[i].Disabled = "Arm Wound"
		}
	}
	wantAuras(t, "disabled banner", rs.UnitAuras(dis))
	// the Torch, if the armoury sells it to this company
	torch := bought("Warrior of Rohan", RoleWarrior, "Torching Brand")
	if slices.ContainsFunc(torch.Equipment, func(e Equipment) bool { return e.Name == "Torching Brand" }) {
		wantAuras(t, "torch", rs.UnitAuras(torch), "6 utility Torching Brand")
	}
}

// TestUnitAurasRulesPathsPowers: rules, path rules (unlocked only), heroic actions, powers.
func TestUnitAurasRulesPathsPowers(t *testing.T) {
	rs := shippedRules(t)
	start := "Warrior of Minas Tirith"
	hero := func(path string, mod func(*UnitState)) []Aura {
		return rs.UnitAuras(auraHeroOn(t, rs, "Minas Tirith", start, path, mod))
	}
	tests := []struct {
		name string
		path string
		mod  func(*UnitState)
		want []string
	}{
		{"Warlord: Voice of Command replaces the four 6 inch command rings", "Path of the Warlord", nil,
			[]string{"3 banner Commanding Bellow", "12 command Voice of Command", "6 enemy Furious Tirade, Master of Battle"}},
		{"Warlord with no rule unlocked has the plain Hero rings and Heroic Resolve", "Path of the Warlord", func(s *UnitState) { s.PathRules = 0 },
			[]string{"6 command Stand Fast, With Me!, Loose, Heroic Resolve"}},
		{"Tactician", "Path of the Tactician", nil, []string{"6 command Stand Fast, With Me!, Loose", "6 support Heroic Move Mastery, Reading the Portents", "6 enemy Gatekeeper", "12 enemy Spy"}},
		{"Tactician with no rule unlocked has only the Hero rings", "Path of the Tactician", func(s *UnitState) { s.PathRules = 0 }, []string{"6 command Stand Fast, With Me!, Loose"}},
		{"Marksman: Take Aim joins Stand Fast", "Path of the Marksman", nil, []string{"6 command Stand Fast, With Me!, Loose, Take Aim", "3 support \"Hold positions, loose the arrows!\""}},
		{"Ranger: At the Double", "Path of the Ranger", nil, []string{"6 command Stand Fast, With Me!, Loose, At the Double", "6 support Chief Advisor"}},
		{"Sorcerer: the powers in a separate tier", "Path of the Sorcerer", func(s *UnitState) {
			s.Spells = []PathSpell{{Name: "Aura of Command", Casting: 4, Range: 6}, {Name: "Fury (Beast)", Casting: 4}, {Name: "Aura of Dismay", Casting: 4}, {Name: "Blinding Light", Casting: 4}, {Name: "Writhing Vines", Casting: 4}}
		}, nil},
	}
	for _, tc := range tests {
		got := hero(tc.path, tc.mod)
		switch {
		case strings.HasPrefix(tc.name, "Sorcerer"):
			var cast []string
			for _, a := range got {
				if a.When == AuraCast {
					cast = append(cast, fmt.Sprintf("%g %s %s", a.Radius, a.Kind, a.Label))
				}
			}
			want := []string{"6 command Aura of Command", "6 support Fury", "6 enemy Aura of Dismay", "3 utility Writhing Vines", "6 utility Blinding Light"}
			if !slices.Equal(cast, want) {
				t.Errorf("%s:\n got  %q\n want %q", tc.name, cast, want)
			}
		default:
			wantAuras(t, tc.name, got, tc.want...)
		}
	}
}

// TestUnitAurasCompanyRules: the rules a company hands its heroes, and the receiver-side and creature cases.
func TestUnitAurasCompanyRules(t *testing.T) {
	rs := shippedRules(t)
	tests := []struct {
		company, base, upgrade string
		want                   []string
	}{
		{"Fiefdoms", "Knight of Dol Amroth", "Lord of Dol Amroth", []string{"6 banner Lord of Dol Amroth", "6 command Stand Fast, With Me!, Loose"}},
		{"Fiefdoms", "Axeman of Lossarnach", "Lord of Lossarnach", []string{"6 command Stand Fast, With Me!, Loose", "3 support Lord of Lossarnach"}},
		{"Lake-town", "Lake-town Guard", "Moneybags", []string{"6 command Stand Fast, With Me!, Loose", "6 support Moneybags"}},
		{"Moria", "Moria Goblin Warrior", "Callous Leadership", []string{"6 command Stand Fast, With Me!, Loose", "3 support Callous Leadership"}},
		{"City of Umbar", "Black Numenorean Warrior", "Seize Command", []string{"6 command Stand Fast, With Me!, Loose", "3 support Seize Command"}},
		{"Osgiliath & Ithilien", "Osgiliath Veteran", "Remember Today", []string{"6 command Stand Fast, With Me!, Loose", "3 support Remember Today"}},
	}
	for _, tc := range tests {
		u := auraHeroOn(t, rs, tc.company, tc.base, "Path of the Soldier", func(s *UnitState) {
			s.PathRules, s.Progress = 0, &Progress{Upgrades: []string{tc.upgrade}}
		})
		wantAuras(t, tc.upgrade, rs.UnitAuras(u), tc.want...)
	}
	// a Falcon is a creature figure with its own ring; the rider of the Falcon gets none from it
	co := auraCompany(t, rs, "Minas Tirith")
	owner := buildUnit(rs, co, UnitState{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleLeader}, CampaignSettings{})
	for _, c := range []CompanyCreature{{Name: "Falcon", OwnerID: 1, Rules: []string{"Keen Sight (Falcon)"}}} {
		wantAuras(t, "falcon", rs.CreatureAuras(c), "12 enemy Keen Sight")
	}
	wantAuras(t, "owner of a falcon", rs.UnitAuras(owner), "6 command Stand Fast, With Me!, Loose")
}

func TestUnitAurasLostStandFastAndWanderers(t *testing.T) {
	rs := shippedRules(t)
	u := auraHeroOn(t, rs, "Rohan", "Warrior of Rohan", "Path of the Soldier", func(s *UnitState) { s.PathRules = 0 })
	wantAuras(t, "hero", rs.UnitAuras(u), "6 command Stand Fast, With Me!, Loose")
	u.LostAbilities = []InjuryLoss{{Ability: "Stand Fast", Injury: "Broken Honour"}}
	wantAuras(t, "a hero who lost Stand Fast", rs.UnitAuras(u))
	u.LostAbilities = nil
	u.Wanderer = true
	wantAuras(t, "a wanderer", rs.UnitAuras(u))
}

// TestUnitAurasNoData: a ruleset without data.auras has no rings and no error.
func TestUnitAurasNoData(t *testing.T) {
	tab, err := ParseAuras([]byte(`{"data":{}}`))
	if err != nil || !tab.Empty() {
		t.Fatalf("empty file: %v %v", tab, err)
	}
	if _, err := ParseAuras([]byte(`{"data":{"auras":{"entries":[{"id":"x","radius":0}]}}}`)); err == nil {
		t.Error("an entry with no radius is an error")
	}
	rs := shippedRules(t)
	withoutAuras(t, rs)
	u := auraHeroOn(t, rs, "Rohan", "Warrior of Rohan", "Path of the Warlord", nil)
	if as := rs.UnitAuras(u); len(as) != 0 {
		t.Errorf("auras without data: %v", auraSig(as))
	}
	d := CompanyDetail{CampaignCompany: CampaignCompany{Faction: "Rohan"}, Roster: []CompanyUnit{u}}
	sel := rs.DefaultAuraSelection()
	if s := rs.CompanyAuraSummary(d, sel); s.Rings != 0 || len(s.Kinds) != 0 {
		t.Errorf("summary without data: %+v", s)
	}
}

// TestAuraMergeKeywords: a keyword filter survives a merge only when every merged entry has one.
func TestAuraMergeKeywords(t *testing.T) {
	rs := shippedRules(t)
	mk := func(id string, kw ...string) auraMatch {
		return auraMatch{e: AuraEntry{ID: id, Label: id, Radius: 3, Kind: "support", Affects: "friendly", When: AuraAlways, Group: AuraGroupAlways, Keywords: kw}, from: id}
	}
	got := rs.mergeAuras([]auraMatch{mk("a", "Lamedon"), mk("b", "Goblin")})
	if len(got) != 1 || !slices.Equal(got[0].Keywords, []string{"Lamedon", "Goblin"}) || got[0].Label != "a, b" {
		t.Errorf("both filtered: %+v", got)
	}
	got = rs.mergeAuras([]auraMatch{mk("a", "Lamedon"), mk("b")})
	if len(got) != 1 || got[0].Keywords != nil {
		t.Errorf("one open entry opens the ring: %+v", got)
	}
}

// TestAuraSelection: the switches keep and remove rings; the merge follows the selection (a switched-off entry lends no label).
func TestAuraSelection(t *testing.T) {
	rs := shippedRules(t)
	u := auraHeroOn(t, rs, "Rohan", "Warrior of Rohan", "Path of the Marksman", func(s *UnitState) {
		s.Gear = []string{"War horn, shield, and throwing spears"}
	})
	ms := rs.auraMatches(u)
	labels := func(sel AuraSelection) []string { return auraSig(rs.selectedAuras(ms, sel)) }
	def := rs.DefaultAuraSelection()
	if !def.Draw || !def.Declared || !def.Conditional || def.Cast || def.Target || def.Large || def.Optional || def.Kinds != nil {
		t.Errorf("defaults %+v", def)
	}
	if got := labels(AuraSelection{}); got != nil {
		t.Errorf("the zero selection draws %q", got)
	}
	if got := labels(def); !slices.Equal(got, []string{"6 command Stand Fast, With Me!, Loose, Take Aim", "9 command War Horn: Stand Fast +3\"", "3 support \"Hold positions, loose the arrows!\""}) {
		t.Errorf("default: %q", got)
	}
	noDecl := def
	noDecl.Declared = false
	if got := labels(noDecl); !slices.Equal(got, []string{"6 command Stand Fast, With Me!, Loose", "3 support \"Hold positions, loose the arrows!\""}) {
		t.Errorf("without declared rules: %q", got) // Take Aim is a declared entry: it lends no label
	}
	large := def
	large.Large = true
	if got := labels(large); !slices.Contains(got, "24 command War Horn") {
		t.Errorf("large rings: %q", got)
	}
	noCmd := def
	noCmd.Kinds = map[AuraKind]bool{"support": true}
	if got := labels(noCmd); !slices.Equal(got, []string{"3 support \"Hold positions, loose the arrows!\""}) {
		t.Errorf("kinds: %q", got)
	}
}

func TestCompanyAuraSummary(t *testing.T) {
	rs := shippedRules(t)
	co := auraCompany(t, rs, "Rohan")
	lead := auraHeroOn(t, rs, "Rohan", "Warrior of Rohan", "Path of the Sorcerer", func(s *UnitState) {
		s.Spells = []PathSpell{{Name: "Aura of Command", Casting: 4, Range: 6}}
	})
	lead.Role = RoleLeader
	banner := buildUnit(rs, co, UnitState{ID: 2, Base: "Warrior of Rohan", Role: RoleWarrior, Gear: []string{"Banner"}}, CampaignSettings{})
	plain := buildUnit(rs, co, UnitState{ID: 3, Base: "Warrior of Rohan", Role: RoleWarrior}, CampaignSettings{})
	d := CompanyDetail{CampaignCompany: CampaignCompany{Faction: "Rohan"}, Roster: []CompanyUnit{lead, banner, plain}}
	s := rs.CompanyAuraSummary(d, rs.DefaultAuraSelection())
	if s.Figures != 2 || s.Rings != 2 || s.LeftOut != 0 {
		t.Errorf("default summary: %+v", s)
	}
	var kinds []string
	for _, k := range s.Kinds {
		kinds = append(kinds, fmt.Sprintf("%s:%d:%v:%v", k.Kind, k.Rings, k.Radii, k.On))
	}
	if want := []string{"banner:1:[3]:true", "command:1:[6]:true"}; !slices.Equal(kinds, want) {
		t.Errorf("kinds %q, want %q", kinds, want)
	}
	if s.Counts["cast"] != 1 || s.Counts["large"] != 0 {
		t.Errorf("counts %v", s.Counts)
	}
	all := rs.DefaultAuraSelection()
	all.Cast = true
	if s := rs.CompanyAuraSummary(d, all); s.Rings != 3 || s.Counts["cast"] != 1 {
		t.Errorf("with spells: rings %d counts %v", s.Rings, s.Counts)
	}
	if s := rs.CompanyAuraSummary(d, AuraSelection{}); s.Rings != 0 || len(s.Counts) != 0 {
		t.Errorf("not drawing: %+v", s)
	}
}

// TestAuraRadiiInRuleText: the radius of a curated entry is written in the text of its source (a typo in the table, or a rules change, fails).
func TestAuraRadiiInRuleText(t *testing.T) {
	rs := shippedRules(t)
	text := func(e AuraEntry) string {
		var b strings.Builder
		for _, n := range e.Source.Names {
			if s, ok := rs.BC.Keyword(n); ok {
				b.WriteString(s + " ")
			}
			if s, ok := rs.Vanilla.Keyword(n); ok {
				b.WriteString(s + " ")
			}
			if p, ok := rs.Vanilla.Power(n); ok {
				b.WriteString(p.Definition + " ")
			}
			if a, ok := rs.BC.Annotation(n); ok {
				b.WriteString(a.Definition + " ")
			}
			for _, rules := range rs.BC.pathRules {
				for _, r := range rules {
					if strings.EqualFold(r.Name, n) {
						b.WriteString(r.Description + " ")
					}
				}
			}
		}
		for _, pw := range rs.BC.Progression().Powers {
			for _, n := range e.Source.Names {
				if strings.HasPrefix(strings.ToLower(n), strings.ToLower(pw.Name)) {
					b.WriteString(pw.Definition + " ")
				}
			}
		}
		return b.String()
	}
	checked := 0
	for _, e := range rs.BC.Auras().Entries {
		if e.Source.Type == "hero" || e.Source.Type == "heroicAction" {
			continue // the glossary and the path's Heroic Action text; checked by tools/validate_definitions.py
		}
		s := text(e)
		if s == "" {
			continue // an item or a rule the Go side does not hold the text of
		}
		checked++
		radius := fmt.Sprintf("%g", e.Radius)
		if !regexp.MustCompile(`\b`+radius+`\s*(?:"|”|inch)`).MatchString(s) && len(e.Source.Names) > 0 {
			// a sum (the war horn's 6+3) is written in parts
			if !strings.Contains(e.ID, "standfast") {
				t.Errorf("%s: %s\" is not written in the text of %v", e.ID, radius, e.Source.Names)
			}
		}
	}
	if checked < 25 {
		t.Errorf("only %d entries had a text to check", checked)
	}
}

func TestBaseRadius(t *testing.T) {
	tests := []struct {
		in   []string
		want float64
	}{
		{[]string{"25mm"}, 0.49}, {[]string{"40mm"}, 0.79}, {[]string{"60mm"}, 1.18}, {[]string{"120mm oval"}, 2.36},
		{[]string{"180x140mm oval"}, 3.54}, {[]string{"231x133mm Oval"}, 4.55}, {[]string{"170mmm oval"}, 3.35}, {nil, 0}, {[]string{"large"}, 0},
	}
	for _, tc := range tests {
		if got := BaseRadius(tc.in); got != tc.want {
			t.Errorf("BaseRadius(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
