package core

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// ttsCodes are the rich text codes the descriptions may contain: bold, italic, a colour and its end.
var ttsCodes = regexp.MustCompile(`\[(/?[bi]|-|[0-9A-F]{6})\]`)

// checkTTSPayload holds for every payload: one figure per spawned model of the export in order, the keys and nicknames of the list, a
// JSON that reads back as the same payload, safe rich text and descriptions within the cap.
func checkTTSPayload(t *testing.T, label string, ex TTSExport, p TTSNamesPayload) {
	t.Helper()
	var want []TTSLine
	var ids []int64
	for _, l := range ex.Lines() {
		for _, id := range l.UnitIDs {
			want = append(want, l)
			ids = append(ids, id)
		}
	}
	if len(p.Figures) != len(want) {
		t.Fatalf("%s: %d figures for %d spawned models", label, len(p.Figures), len(want))
	}
	seen := map[string]int{}
	for i, f := range p.Figures {
		l := want[i]
		if f.Key != l.Key || f.Unit != ids[i] || f.Extra != l.Extra || f.Line != l.Name+": "+strings.Join(l.Options, ", ") {
			t.Errorf("%s: figure %d is %+v, line %+v", label, i, f, l)
		}
		if !strings.HasSuffix(l.Text(true), f.Line+")") {
			t.Errorf("%s: nickname %q is not the text of %q", label, f.Line, l.Text(true))
		}
		if f.Occurrence != seen[f.Key] {
			t.Errorf("%s: figure %d occurrence %d, want %d", label, i, f.Occurrence, seen[f.Key])
		}
		seen[f.Key]++
		if f.Name == "" || f.Desc == "" {
			t.Errorf("%s: figure %d has no name or description: %+v", label, i, f)
		}
		for what, s := range map[string]string{"name": f.Name, "description": f.Desc} {
			if rest := ttsCodes.ReplaceAllString(s, ""); strings.ContainsAny(rest, "[]") {
				t.Errorf("%s: stray bracket in %s %q", label, what, s)
			}
			if strings.ContainsFunc(s, func(r rune) bool { return (r < ' ' && r != '\n') || r == ' ' || r == ' ' }) {
				t.Errorf("%s: control character in %s %q", label, what, s)
			}
			for _, tag := range []string{"b", "i"} {
				if strings.Count(s, "["+tag+"]") != strings.Count(s, "[/"+tag+"]") {
					t.Errorf("%s: unbalanced [%s] in %q", label, tag, s)
				}
			}
		}
		if c := strings.Count(f.Desc, "[-]"); c != len(regexp.MustCompile(`\[[0-9A-F]{6}\]`).FindAllString(f.Desc, -1)) {
			t.Errorf("%s: unbalanced colours in %q", label, f.Desc)
		}
		if n := utf8.RuneCountInString(f.Desc); n > TTSDescriptionMax {
			t.Errorf("%s: description of %d characters", label, n)
		}
	}
	b, err := p.JSON()
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if len(b) != p.Size {
		t.Errorf("%s: size %d, JSON %d", label, p.Size, len(b))
	}
	var back TTSNamesPayload
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if !reflect.DeepEqual(back.Figures, p.Figures) || back.Version != 1 || back.Company != p.Company {
		t.Errorf("%s: the JSON does not read back as the payload", label)
	}
	if again, _ := p.JSON(); !slices.Equal(again, b) {
		t.Errorf("%s: JSON not deterministic", label)
	}
}

func TestTTSNamesAllCompanies(t *testing.T) {
	rs := shippedRules(t)
	companies := ttsCompanies(rs)
	if len(companies) != 37 {
		t.Fatalf("%d companies, want 37", len(companies))
	}
	opts := TTSOptions{FootVersions: true, MountFigures: true}
	for _, bcCo := range companies {
		entries := ttsEntries(rs, bcCo)
		// each entry alone, and the entries in rosters of 12 units as a company has
		var roster []CompanyUnit
		for i, e := range entries {
			d := ttsOne(rs, bcCo, e)
			ex := rs.TTSList(d, opts)
			if len(ex.Lines()) > 0 {
				checkTTSPayload(t, bcCo.Name+" "+e.unit, ex, rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort}))
			}
			u := d.Roster[0]
			u.ID = int64(len(roster) + 1)
			if len(roster)%12 == 0 {
				u.Role = RoleLeader
			}
			roster = append(roster, u)
			if len(roster) == 12 || i == len(entries)-1 {
				d := CompanyDetail{CampaignCompany: CampaignCompany{Name: "Test company", Faction: bcCo.Name}, Roster: roster}
				ex := rs.TTSList(d, opts)
				p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
				checkTTSPayload(t, bcCo.Name+" roster", ex, p)
				if p.Size > TTSNamesMaxBytes || p.Rules != TTSRulesShort || len(p.Notes) > 1 {
					t.Errorf("%s: roster of %d units: %d bytes, rules %s, notes %+v", bcCo.Name, len(roster), p.Size, p.Rules, p.Notes)
				}
				roster = nil
			}
		}
	}
}

func TestTTSNamesFormat(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	lead := ttsUnit(t, rs, f, 1, "Warrior of Minas Tirith", RoleLeader, []string{"Spear", "Shield"}, nil)
	lead.Name = "Boromir"
	lead.Stats = append(lead.Stats, Attribute{Abbr: "M", Name: "Might", Value: 3, HeroOnly: true})
	d := ttsDetail(f, lead)
	d.Name = "The [White] Company"
	ex := rs.TTSList(d, TTSOptions{})
	p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
	checkTTSPayload(t, "format", ex, p)
	if p.Version != 1 || p.Company != "The (White) Company" || p.Rules != TTSRulesShort || len(p.Notes) != 0 {
		t.Errorf("payload %+v", p)
	}
	fig := p.Figures[0]
	if fig.Name != fmt.Sprintf("[b]Boromir[/b] (%d)", lead.Points) || fig.Line != "Warrior of Minas Tirith: Shield and Spear" {
		t.Errorf("figure %+v", fig)
	}
	lines := strings.Split(fig.Desc, "\n")
	if lines[0] != "[i]Leader[/i]" && !strings.HasPrefix(lines[0], "[i]Leader · ") {
		t.Errorf("first line %q", lines[0])
	}
	if strings.Contains(fig.Desc, "level ") || strings.Contains(fig.Desc, "XP ") {
		t.Errorf("level or XP in the description:\n%s", fig.Desc)
	}
	if !strings.HasPrefix(lines[1], "[b]Mv[/b]") || strings.Contains(lines[2], "[b]") || !strings.HasPrefix(lines[2], "6\"") ||
		!strings.Contains(fig.Desc, "\n[00CCFF]Might 3[-]\n") || !strings.Contains(fig.Desc, "\n[b]Wargear:[/b] ") {
		t.Errorf("description:\n%s", fig.Desc)
	}
	b, _ := p.JSON()
	if !strings.HasPrefix(string(b), `{"v":1,"company":"The (White) Company","figures":[{"k":"warrior of minas tirith: shield and spear","l":"Warrior of Minas Tirith: Shield and Spear","u":1,"x":"","o":0,"n":`) {
		t.Errorf("json %s", b)
	}
	if strings.Contains(string(b), `<`) || strings.Contains(string(b), "\n}") {
		t.Errorf("escaped: %s", b)
	}
}

func TestTTSNamesStatColours(t *testing.T) {
	got := ttsStatLines([]Attribute{
		{Abbr: "Mv", Value: 6, Trail: `"`},
		{Abbr: "FV", Value: 5, Boost: 1},
		{Abbr: "S", Value: 3, Boost: -1},
		{Abbr: "D", Value: 5, Trail: "+", Boost: -1},
		{Abbr: "C", Value: 5, Trail: "+", Boost: 1},
		{Abbr: "A", Value: 2, Gear: 1},
	}, []Attribute{{Name: "Might", Value: 3}, {Name: "Will", Value: 2}, {Name: "Fate", Value: 1}})
	if len(got) != 3 || got[2][0] != `[00CCFF]Might 3  Will 2  Fate 1[-]` {
		t.Fatalf("got %q", got)
	}
	vals := got[1][0]
	for _, want := range []string{`[44BA26]5[-]`, `[E03C31]3[-]`, `[44BA26]5+[-]`, `[E03C31]5+[-]`, `[44BA26]2[-]`} {
		if !strings.Contains(vals, want) {
			t.Errorf("%q misses %q", vals, want)
		}
	}
	if strings.Contains(vals, `[44BA26]6`) || !strings.HasPrefix(vals, `6"`) {
		t.Errorf("unchanged Mv coloured: %q", vals)
	}
}

func TestTTSNamesRidersAndMounts(t *testing.T) {
	rs := shippedRules(t)
	f := "Mordor"
	rider := ttsUnit(t, rs, f, 1, "Mordor Warg Rider", RoleLeader, nil, nil)
	rider.Name = "Grub"
	warg := ttsUnit(t, rs, f, 2, "Mordor Orc Warrior", RoleWarrior, []string{"Shield"}, []string{"Warg"})
	d := ttsDetail(f, rider, warg)
	ex := rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
	p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
	checkTTSPayload(t, "riders", ex, p)
	var kinds []string
	for _, fg := range p.Figures {
		kinds = append(kinds, fg.Extra)
	}
	if !slices.Equal(kinds, []string{"", "foot", "mount", "", "mount"}) {
		t.Fatalf("extras %v", kinds)
	}
	if !strings.HasSuffix(p.Figures[1].Name, "(on foot)") || !strings.Contains(p.Figures[1].Desc, " · on foot[/i]") || p.Figures[1].Unit != 1 {
		t.Errorf("foot figure %+v", p.Figures[1])
	}
	if p.Figures[2].Name != "[b]Grub[/b]'s Fell Warg" || !strings.HasPrefix(p.Figures[2].Desc, "[i]Mount of Grub[/i]\n") {
		t.Errorf("mount figure %+v", p.Figures[2])
	}
	if !strings.Contains(p.Figures[4].Desc, "[b]Mv[/b]") {
		t.Errorf("the bought warg has no stat line: %q", p.Figures[4].Desc)
	}
}

func TestTTSNamesStandInsAndCreatures(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	lead := ttsUnit(t, rs, f, 1, "Warrior of Minas Tirith", RoleLeader, []string{"Bow"}, nil)
	lead.Name = "Faramir"
	sage := buildUnit(rs, ttsBCCompany(t, rs, f), UnitState{ID: 2, Base: "Wandering Sage", Wanderer: "Wandering Sage", Role: RoleWarrior}, CampaignSettings{})
	d := ttsDetail(f, lead, sage)
	d.Creatures = []CompanyCreature{{Name: "Baggage Pony", OwnerID: 1, Points: 5, Rules: []string{"Timid"}}}
	ex := rs.TTSList(d, TTSOptions{})
	p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
	checkTTSPayload(t, "standins", ex, p)
	if len(p.Figures) != 3 {
		t.Fatalf("%+v", p.Figures)
	}
	pony, wander := p.Figures[1], p.Figures[2]
	if pony.Line != "Bill the Pony: " || pony.Name != "[b]Faramir[/b]'s Baggage Pony (5)" || !strings.HasPrefix(pony.Desc, "[i]Creature of Faramir, shown as Bill the Pony[/i]") || !strings.Contains(pony.Desc, "[b]Timid[/b]") {
		t.Errorf("creature figure %+v", pony)
	}
	if !strings.Contains(wander.Name, " ~ as Malbeth the Seer") || !strings.Contains(wander.Desc, "Wanderer") {
		t.Errorf("wanderer figure %+v", wander)
	}
}

func TestTTSNamesLiveFallenMarker(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	var units []CompanyUnit
	for id := int64(1); id <= 3; id++ {
		units = append(units, ttsUnit(t, rs, f, id, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil))
	}
	units[0].Role = RoleLeader
	d := ttsDetail(f, units...)
	ex := rs.TTSLiveList(d, TTSLive{Units: []LiveUnit{{UnitID: 3, Zone: ZoneCasualty}}}, TTSOptions{})
	p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
	checkTTSPayload(t, "live", ex, p)
	for _, fg := range p.Figures {
		fallen := strings.HasPrefix(fg.Desc, "[E03C31][b]Fallen[/b]")
		if fallen != (fg.Unit == 3) {
			t.Errorf("unit %d: fallen marker %v in %q", fg.Unit, fallen, fg.Desc)
		}
	}
	if plain := rs.TTSNames(d, rs.TTSList(d, TTSOptions{}), TTSNamesOptions{Rules: TTSRulesShort}); strings.Contains(plain.Figures[2].Desc, "Fallen") {
		t.Error("the plain list marks a unit fallen")
	}
}

// longRules gives a unit rules whose texts are long and bracketed.
func longRules(u CompanyUnit, n int) CompanyUnit {
	u.Rules = nil
	for i := 0; i < n; i++ {
		u.Rules = append(u.Rules, UnitRule{Name: fmt.Sprintf("Rule %d", i), Description: fmt.Sprintf("First sentence of rule %d, e.g. a [bracket] test. %s", i, strings.Repeat("A long second sentence with many words. ", 12))})
	}
	return u
}

func TestTTSNamesRuleTextLevels(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	u := longRules(ttsUnit(t, rs, f, 1, "Warrior of Minas Tirith", RoleLeader, []string{"Bow"}, nil), 3)
	d := ttsDetail(f, u)
	ex := rs.TTSList(d, TTSOptions{})
	short := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
	full := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesFull})
	none := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesNone})
	for label, p := range map[string]TTSNamesPayload{"short": short, "full": full, "none": none} {
		checkTTSPayload(t, label, ex, p)
	}
	if !strings.Contains(short.Figures[0].Desc, "[b]Rule 0[/b]: First sentence of rule 0, e.g. a (bracket) test.\n") {
		t.Errorf("short:\n%s", short.Figures[0].Desc)
	}
	if strings.Contains(none.Figures[0].Desc, "First sentence") || !strings.Contains(none.Figures[0].Desc, "[b]Rules:[/b] Rule 0, Rule 1, Rule 2") {
		t.Errorf("none:\n%s", none.Figures[0].Desc)
	}
	if !strings.Contains(full.Figures[0].Desc, "A long second sentence") && !strings.Contains(full.Figures[0].Desc, "...") {
		t.Errorf("full:\n%s", full.Figures[0].Desc)
	}
	if !(short.Size < full.Size+1 && none.Size < short.Size) {
		t.Errorf("sizes none %d short %d full %d", none.Size, short.Size, full.Size)
	}
	if len(short.Notes) != 0 || short.Rules != TTSRulesShort {
		t.Errorf("short notes %+v", short.Notes)
	}
	// a description over the cap loses whole lines, not half a tag
	big := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesFull})
	if big.Rules != TTSRulesFull || !slices.ContainsFunc(big.Notes, func(n TTSNote) bool { return n.Kind == TTSDescriptionCut }) {
		t.Errorf("full: rules %s notes %+v", big.Rules, big.Notes)
	}
	if strings.Contains(big.Figures[0].Desc, "[b]Rule 1[/b]:") {
		t.Errorf("rule 1 kept its text: %q", big.Figures[0].Desc)
	}
}

func TestTTSNamesLowersRulesToFitTheSize(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	var units []CompanyUnit
	for id := int64(1); id <= 30; id++ {
		u := longRules(ttsUnit(t, rs, f, id, "Warrior of Minas Tirith", RoleWarrior, []string{"Bow"}, nil), 6)
		u.Name = fmt.Sprintf("Unit number %d", id)
		units = append(units, u)
	}
	units[0].Role = RoleLeader
	d := ttsDetail(f, units...)
	ex := rs.TTSList(d, TTSOptions{})
	full := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesFull})
	checkTTSPayload(t, "lowered", ex, full)
	if full.Size > TTSNamesMaxBytes || full.Rules == TTSRulesFull || !slices.ContainsFunc(full.Notes, func(n TTSNote) bool { return n.Kind == TTSRulesLowered }) {
		t.Errorf("full: %d bytes, rules %s, notes %+v", full.Size, full.Rules, full.Notes)
	}
	// 200 figures cannot fit even with the names of the rules only
	for len(units) < 200 {
		u := units[len(units)-1]
		u.ID++
		units = append(units, u)
	}
	d = ttsDetail(f, units...)
	ex = rs.TTSList(d, TTSOptions{})
	huge := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
	checkTTSPayload(t, "huge", ex, huge)
	if huge.Rules != TTSRulesNone || !slices.ContainsFunc(huge.Notes, func(n TTSNote) bool { return n.Kind == TTSTooLarge }) {
		t.Errorf("huge: %d bytes, rules %s, notes %+v", huge.Size, huge.Rules, huge.Notes)
	}
}

func TestTTSNamesSpecialCharacters(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	u := ttsUnit(t, rs, f, 1, "Warrior of Minas Tirith", RoleLeader, []string{"Bow"}, nil)
	u.Name = "Ar[b]athorn \"Strider\" <the> Éowyn\n\t &"
	u.Rules = []UnitRule{{Name: "Odd [rule]", Description: "Move 6\" \\ and\ttab\x00 [FF0000]red[-]. Second."}}
	u.Injuries = []UnitInjury{{Key: "arm", Name: "Arm [wound]", Count: 2}}
	d := ttsDetail(f, u)
	d.Name = "O'Neil & \"Sons\" \x01"
	ex := rs.TTSList(d, TTSOptions{})
	p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort})
	checkTTSPayload(t, "special", ex, p)
	fg := p.Figures[0]
	if !strings.HasPrefix(fg.Name, `[b]Ar(b)athorn "Strider" <the> `+"É"+`owyn &[/b] (`) {
		t.Errorf("name %q", fg.Name)
	}
	if !strings.Contains(fg.Desc, `[b]Odd (rule)[/b]: Move 6" \ and tab (FF0000)red(-).`) || !strings.Contains(fg.Desc, "Arm (wound) x2") {
		t.Errorf("description %q", fg.Desc)
	}
	b, _ := p.JSON()
	if !strings.Contains(string(b), `<the>`) || !strings.Contains(string(b), `Move 6\" \\ and`) || !strings.Contains(string(b), `"company":"O'Neil & \"Sons\""`) {
		t.Errorf("json %s", b)
	}
	// the nickname is the list line byte for byte, whatever the unit is called
	if fg.Line != "Warrior of Minas Tirith: Bow" {
		t.Errorf("line %q", fg.Line)
	}
}

func TestTTSFirstSentence(t *testing.T) {
	for _, c := range []struct {
		in   string
		max  int
		want string
	}{
		{"One. Two.", 160, "One."},
		{"No stop", 160, "No stop"},
		{"Use e.g. a bow. More.", 160, "Use e.g. a bow."},
		{"Range 6.5 inches. Next.", 160, "Range 6.5 inches."},
		{"Is it? Yes.", 160, "Is it?"},
		{"A very long sentence without any stop that goes on and on", 30, "A very long sentence..."},
		{"", 160, ""},
	} {
		if got := ttsFirstSentence(c.in, c.max); got != c.want || utf8.RuneCountInString(got) > c.max {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTTSFitDescription(t *testing.T) {
	s, cut := fitDescription([]ttsDLine{{"head"}, {"[b]a[/b]: " + strings.Repeat("x", 100), "[b]a[/b]"}, {"tail"}}, 40)
	if s != "head\n[b]a[/b]\ntail" || !cut {
		t.Errorf("shorter form: %q %v", s, cut)
	}
	s, cut = fitDescription([]ttsDLine{{"head"}, {strings.Repeat("y", 100)}, {"tail"}}, 40)
	if s != "head\ntail\n..." || !cut {
		t.Errorf("skipped line: %q %v", s, cut)
	}
}

// TestTTSNamesSectionsAndHeader: the header has the role and the path only (no level, no XP); the Mount, Wargear, Equipment and Heroic
// actions sections appear only when not empty, and a heroic action the path did not give is left out.
func TestTTSNamesSectionsAndHeader(t *testing.T) {
	rs := shippedRules(t)
	f := "Minas Tirith"
	u := ttsUnit(t, rs, f, 1, "Warrior of Minas Tirith", RoleSergeant, []string{"Spear"}, nil)
	u.Level, u.Equipment = 3, []Equipment{
		{Name: "Horse", Class: EquipMount},
		{Name: "Shield", Class: EquipShield},
		{Name: "Banner", Class: EquipOther, Size: "large"},
		{Name: "Healing Herbs", Class: EquipOther, Size: "small"},
		{Name: "Hearthguard", Upgrade: true},
	}
	u.Path = &UnitPath{Name: "Path of the Sharpshooter", HeroicActions: []HeroicAction{
		{Name: "Heroic Strike", Source: ActionFromUnit}, {Name: "Deadly Aim", Source: ActionFromPath}, {Name: "Bought One", Source: ActionBought},
	}}
	d := ttsDetail(f, u)
	ex := rs.TTSList(d, TTSOptions{})
	for _, lvl := range []string{TTSRulesNone, TTSRulesShort, TTSRulesFull} {
		desc := rs.TTSNames(d, ex, TTSNamesOptions{Rules: lvl}).Figures[0].Desc
		lines := strings.Split(desc, "\n")
		if lines[0] != "[i]Sergeant · Path of the Sharpshooter[/i]" || strings.Contains(desc, "level") || strings.Contains(desc, "XP") {
			t.Errorf("%s: header %q", lvl, lines[0])
		}
		for _, want := range []string{"[b]Mount:[/b] Horse", "[b]Wargear:[/b] Shield", "[b]Equipment:[/b] Banner, Healing Herbs", "[b]Heroic actions:[/b] Deadly Aim"} {
			if !slices.Contains(lines, want) {
				t.Errorf("%s: no line %q in\n%s", lvl, want, desc)
			}
		}
		if strings.Contains(desc, "Heroic Strike") || strings.Contains(desc, "Bought One") || strings.Contains(desc, "Hearthguard") {
			t.Errorf("%s: default action or upgrade shown:\n%s", lvl, desc)
		}
	}
	// empty sections are left out
	w := ttsUnit(t, rs, f, 2, "Warrior of Minas Tirith", RoleWarrior, nil, nil)
	w.Equipment, w.Path = nil, nil
	dw := ttsDetail(f, w)
	desc := rs.TTSNames(dw, rs.TTSList(dw, TTSOptions{}), TTSNamesOptions{Rules: TTSRulesNone}).Figures[0].Desc
	for _, no := range []string{"Mount:", "Wargear:", "Equipment:", "Heroic actions:"} {
		if strings.Contains(desc, no) {
			t.Errorf("empty section %s shown:\n%s", no, desc)
		}
	}
	if !strings.HasPrefix(desc, "[i]Warrior[/i]\n[b]Mv[/b]") {
		t.Errorf("warrior description:\n%s", desc)
	}
}

// TestTTSNamesUnitUpgrades: for every named unit upgrade the data defines (the options reserved to a named hero's list, built on their unit,
// and every unit alias of the 37 companies) the figure carries the upgrade's title and everything the upgrade adds: changed stats are in
// the table in colour, granted and added rules are in the Rules section, gear is in its section; the upgrade has no line of its own.
func TestTTSNamesUnitUpgrades(t *testing.T) {
	rs := shippedRules(t)
	var doc struct {
		Data struct {
			ReservedOptions struct {
				Items []struct{ Unit, Option string }
			}
		}
	}
	if err := json.Unmarshal(readDef(t, "battle_companies/battle_companies.json"), &doc); err != nil {
		t.Fatal(err)
	}
	type tc struct {
		label, title string
		bcCo         BCCompany
		state        UnitState
		upgrade      string // the upgrade item, empty for an alias without one
		rules        []string
	}
	var cases []tc
	first := ttsCompanies(rs)[0]
	for _, r := range doc.Data.ReservedOptions.Items {
		cases = append(cases, tc{label: r.Unit + " with " + r.Option, title: r.Option, bcCo: first, state: UnitState{Base: r.Unit, Gear: []string{r.Option}}, upgrade: r.Option})
	}
	for _, bcCo := range ttsCompanies(rs) {
		p, _ := rs.BC.Profile(bcCo.Name)
		for _, a := range p.Aliases {
			c := tc{label: bcCo.Name + " " + a.Label, title: a.Label, bcCo: bcCo, state: UnitState{Base: a.Label}, rules: a.AddRules}
			if len(a.Selected) > 0 {
				c.upgrade = a.Selected[0]
			}
			cases = append(cases, c)
		}
	}
	if len(cases) < 17 {
		t.Fatalf("only %d upgrade cases", len(cases))
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			c.state.ID, c.state.Role = 1, RoleWarrior
			u := buildUnit(rs, c.bcCo, c.state, CampaignSettings{})
			d := CompanyDetail{CampaignCompany: CampaignCompany{Faction: c.bcCo.Name}, Roster: []CompanyUnit{u}}
			ex := rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
			if len(ex.Lines()) == 0 {
				t.Skip("no model")
			}
			for _, level := range []string{TTSRulesNone, TTSRulesShort, TTSRulesFull} {
				p := rs.TTSNames(d, ex, TTSNamesOptions{Rules: level})
				checkTTSPayload(t, c.label, ex, p)
				f := p.Figures[0]
				if level != TTSRulesNone && strings.HasSuffix(f.Desc, "\n...") {
					continue // the 900 characters cap cut lines: only the names-only level has to hold everything
				}
				if f.Line != ttsLineText(ex.Lines()[0]) {
					t.Errorf("%s: the nickname %q is not the list line", level, f.Line)
				}
				if !strings.HasPrefix(f.Name, "[b]"+c.title+"[/b] (") {
					t.Errorf("%s: name %q lacks the title %q", level, f.Name, c.title)
				}
				var rule []string
				for _, r := range u.Rules {
					rule = append(rule, r.Shown())
				}
				rule = append(rule, c.rules...)
				if c.upgrade != "" {
					if a, ok := rs.BC.Annotation(c.upgrade); ok {
						rule = append(rule, a.Grants...)
					}
				}
				for _, r := range rule {
					if !strings.Contains(f.Desc, ttsSafe(r)) {
						t.Errorf("%s: the rule %q is missing:\n%s", level, r, f.Desc)
					}
				}
				for _, e := range u.Equipment {
					if (!e.Upgrade || !rs.namedUpgrades()[strings.ToLower(e.Name)]) && !slices.Contains(e.Classes(), EquipMount) && !strings.Contains(f.Desc, ttsSafe(e.Name)) {
						t.Errorf("%s: the gear %q is missing:\n%s", level, e.Name, f.Desc)
					}
				}
				if c.upgrade != "" {
					for _, e := range u.Equipment {
						if e.Name == c.upgrade && e.Upgrade {
							if g, ok := rs.Vanilla.Gear(e.Name); ok {
								for _, m := range g.Modifies {
									if m.Condition == "" && !strings.Contains(f.Desc, ttsColourUp+"]") && !strings.Contains(f.Desc, ttsColourDown+"]") {
										t.Errorf("%s: the %+d %s of the upgrade is not coloured in the table:\n%s", level, m.Delta, m.Stat, f.Desc)
									}
								}
							}
							if e.Description != "" && level == TTSRulesFull {
								if a, _ := rs.BC.Annotation(e.Name); len(a.Grants) == 0 && !strings.Contains(f.Desc, "[b]"+ttsSafe(e.Name)+"[/b]") {
									if g, ok := rs.Vanilla.Gear(e.Name); !ok || len(g.Modifies) == 0 {
										t.Errorf("%s: the text of %q is missing:\n%s", level, e.Name, f.Desc)
									}
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestTTSNamesDefaultIsNamesOnly(t *testing.T) {
	rs := shippedRules(t)
	u := ttsUnit(t, rs, "Minas Tirith", 1, "Warrior of Minas Tirith", RoleWarrior, nil, nil)
	d := ttsDetail("Minas Tirith", u)
	ex := rs.TTSList(d, TTSOptions{})
	def := rs.TTSNames(d, ex, TTSNamesOptions{})
	none := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesNone})
	if def.Rules != TTSRulesNone || !reflect.DeepEqual(def.Figures, none.Figures) {
		t.Errorf("the default is not the names-only level: %q", def.Rules)
	}
	if bogus := rs.TTSNames(d, ex, TTSNamesOptions{Rules: "bogus"}); bogus.Rules != TTSRulesNone {
		t.Errorf("an unknown level is %q", bogus.Rules)
	}
}

// TestTTSNamesUpgradeRenamesTheUnit: a Khazad Guard with the Hearthguard upgrade is a Hearthguard, a named unit keeps its own name, and the
// nickname stays the line of the list.
func TestTTSNamesUpgradeRenamesTheUnit(t *testing.T) {
	rs := shippedRules(t)
	co := ttsCompanies(rs)[0]
	u := buildUnit(rs, co, UnitState{ID: 1, Base: "Khazad Guard", Role: RoleWarrior, Gear: []string{"Hearthguard"}}, CampaignSettings{})
	d := CompanyDetail{CampaignCompany: CampaignCompany{Faction: co.Name}, Roster: []CompanyUnit{u}}
	ex := rs.TTSList(d, TTSOptions{})
	f := rs.TTSNames(d, ex, TTSNamesOptions{}).Figures[0]
	if !strings.HasPrefix(f.Name, "[b]Hearthguard[/b] (") || f.Line != "Khazad Guard: Hearthguard" || !strings.Contains(f.Desc, "Burly") || strings.Contains(f.Desc, "[b]Hearthguard") {
		t.Errorf("Hearthguard: %q / %q\n%s", f.Name, f.Line, f.Desc)
	}
	d.Roster[0].Name = "Dwalin's Own"
	if f := rs.TTSNames(d, ex, TTSNamesOptions{}).Figures[0]; !strings.HasPrefix(f.Name, "[b]Dwalin's Own[/b] (") {
		t.Errorf("a named unit keeps its name: %q", f.Name)
	}
}
