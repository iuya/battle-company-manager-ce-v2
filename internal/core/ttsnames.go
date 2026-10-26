package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The names and descriptions for the figures of a Tabletop Simulator army (docs/tabletop-simulator.md, Level B): the ArmyNames JSON
// version 1 that the helper object applies to the spawned figures. Pure functions over a built company, the list of TTSList and the
// rules: no repository, no clock, no state.

// Values of TTSNamesOptions.Rules.
const (
	TTSRulesShort = "short" // the first sentence of each rule
	TTSRulesNone  = "none"  // the names of the rules only (the default)
	TTSRulesFull  = "full"  // the whole text of each rule
)

// ttsHeaderSep separates the parts of the italic header line.
const ttsHeaderSep = " · "

// Limits of the payload.
const (
	TTSNamesVersion    = 1
	TTSDescriptionMax  = 900       // characters of one description
	TTSNamesMaxBytes   = 24 * 1024 // bytes of the JSON
	TTSRuleShortMax    = 160       // characters of a short rule text
	ttsColourUp        = "44BA26"  // a value the company raised
	ttsColourDown      = "E03C31"  // a value lowered
	ttsColourHeroStats = "00CCFF"  // Might, Will and Fate
)

// Note kinds of the names payload (TTSNote.Kind).
const (
	TTSRulesLowered   = "rulesLowered"   // the rule texts were shortened to fit the size limit
	TTSDescriptionCut = "descriptionCut" // some descriptions lost lines to fit 900 characters
	TTSTooLarge       = "tooLarge"       // the payload is over the size limit even without rule texts
)

// TTSNamesOptions are the switches of the dialog.
type TTSNamesOptions struct {
	Rules string // TTSRulesNone (also when empty or unknown), TTSRulesShort or TTSRulesFull: the highest level tried; the payload is lowered when it is too big
}

// TTSFigure is one figure of the army, as the helper reads it. The i-th figure with the same Key is the i-th (Occurrence) of that nickname.
type TTSFigure struct {
	Key        string `json:"k"` // TTSLine.Key: the line, lower case, single spaces
	Line       string `json:"l"` // the nickname the spawner gives the figure: the text of the list line without brackets and count, byte for byte
	Unit       int64  `json:"u"` // the company unit (a creature or mount carries its owner's id)
	Extra      string `json:"x"` // TTSLine.Extra: "", "foot" or "mount"
	Occurrence int    `json:"o"` // 0-based position among the figures with the same Key
	Name       string `json:"n"` // the name to give the figure: [b]Title[/b] (points), in the rich text of the game
	Desc       string `json:"d"` // the description: rich text, at most TTSDescriptionMax characters
}

// TTSNamesPayload is the ArmyNames JSON v1: one figure per spawned model in spawn order (the lines of the export in warband order, each
// expanded Count times).
type TTSNamesPayload struct {
	Version int         `json:"v"`
	Company string      `json:"company"`
	Figures []TTSFigure `json:"figures"`
	// Not in the JSON.
	Rules string    `json:"-"` // the level of rule texts that was used
	Size  int       `json:"-"` // bytes of the JSON
	Notes []TTSNote `json:"-"` // what was cut, for the player
}

// JSON is the payload as the text the player pastes: compact, deterministic, no HTML escaping.
func (p TTSNamesPayload) JSON() ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(p); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// TTSNames writes the names and descriptions of the figures of exp (the list TTSList or TTSLiveList made for d). The rule texts are
// opts.Rules at most: when the JSON passes TTSNamesMaxBytes they are shortened, then left out, and a note says so.
func (r *RuleSet) TTSNames(d CompanyDetail, exp TTSExport, opts TTSNamesOptions) TTSNamesPayload {
	want := opts.Rules
	if want != TTSRulesFull && want != TTSRulesShort {
		want = TTSRulesNone
	}
	levels := map[string][]string{
		TTSRulesFull:  {TTSRulesFull, TTSRulesShort, TTSRulesNone},
		TTSRulesShort: {TTSRulesShort, TTSRulesNone},
		TTSRulesNone:  {TTSRulesNone},
	}[want]
	var p TTSNamesPayload
	for _, level := range levels {
		p = r.ttsNamesAt(d, exp, level)
		if p.Size <= TTSNamesMaxBytes {
			break
		}
	}
	if p.Rules != want {
		p.Notes = append(p.Notes, TTSNote{Kind: TTSRulesLowered, Text: "Rule texts " + map[string]string{TTSRulesShort: "shortened", TTSRulesNone: "left out"}[p.Rules] + " to fit."})
	}
	if p.Size > TTSNamesMaxBytes {
		p.Notes = append(p.Notes, TTSNote{Kind: TTSTooLarge, Text: fmt.Sprintf("The names are %d KB, over the %d KB the Notebook is meant to take: send fewer figures.", (p.Size+1023)/1024, TTSNamesMaxBytes/1024)})
	}
	return p
}

type ttsNamer struct {
	rs     *RuleSet
	d      CompanyDetail
	models TTSModels
	level  string
	units  map[int64]CompanyUnit
	cut    int
	named  map[string]bool // lower-case names of the named unit upgrades (Hearthguard, Helmingas...)
}

// namedUpgrades are the names of the unit upgrades of the data: the options reserved to a named hero's list (data.reservedOptions) and the
// options a company's unit alias selects (Helminga is a Warrior of Rohan with the Helmingas option).
func (r *RuleSet) namedUpgrades() map[string]bool {
	out := map[string]bool{}
	for k := range r.BC.reserved {
		if _, opt, ok := strings.Cut(k, "|"); ok {
			out[opt] = true
		}
	}
	for name := range r.BC.companies {
		if p, ok := r.BC.Profile(name); ok {
			for _, a := range p.Aliases {
				for _, o := range a.Selected {
					out[strings.ToLower(o)] = true
				}
			}
		}
	}
	return out
}

// upgrade is the named unit upgrade the unit carries, if any.
func (n *ttsNamer) upgrade(u CompanyUnit) (Equipment, bool) {
	for _, e := range u.Equipment {
		if e.Upgrade && n.named[strings.ToLower(e.Name)] {
			return e, true
		}
	}
	return Equipment{}, false
}

// title is the name the figures of a unit carry: the unit's own name, else the upgrade that renames its type (a Khazad Guard with the
// Hearthguard upgrade is a Hearthguard), else its type. A company's alias already is the type (Helminga).
func (n *ttsNamer) title(u CompanyUnit) string {
	if strings.TrimSpace(u.Name) == "" {
		if up, ok := n.upgrade(u); ok && n.rs.BC.Reserved(u.Type, up.Name) {
			return up.Name
		}
	}
	return u.Title()
}

func (r *RuleSet) ttsNamesAt(d CompanyDetail, exp TTSExport, level string) TTSNamesPayload {
	n := &ttsNamer{rs: r, d: d, models: r.BC.TTSModels(), level: level, units: map[int64]CompanyUnit{}, named: r.namedUpgrades()}
	for _, u := range d.Roster {
		n.units[u.ID] = u
	}
	p := TTSNamesPayload{Version: TTSNamesVersion, Company: ttsSafe(d.Name), Figures: []TTSFigure{}, Rules: level}
	seen := map[string]int{}
	for _, w := range exp.Warbands {
		for _, l := range w.Lines {
			for _, id := range l.UnitIDs {
				f := n.figure(l, id, w.Fallen)
				f.Key, f.Line, f.Unit, f.Extra = l.Key, ttsLineText(l), id, l.Extra
				f.Occurrence = seen[l.Key]
				seen[l.Key]++
				p.Figures = append(p.Figures, f)
			}
		}
	}
	if n.cut > 0 {
		p.Notes = append(p.Notes, TTSNote{Kind: TTSDescriptionCut, Text: fmt.Sprintf("%d descriptions were cut to %d characters.", n.cut, TTSDescriptionMax)})
	}
	if b, err := p.JSON(); err == nil {
		p.Size = len(b)
	}
	return p
}

// ttsLineText is the nickname the spawner gives: the line without its brackets and count.
func ttsLineText(l TTSLine) string { return l.Name + ": " + strings.Join(l.Options, ", ") }

// figure is the name and description of one figure of line l, whose company unit is id.
func (n *ttsNamer) figure(l TTSLine, id int64, fallen bool) TTSFigure {
	u, ok := n.units[id]
	if !ok {
		return TTSFigure{Name: "[b]" + ttsSafe(l.Name) + "[/b]"}
	}
	owner := ttsSafe(n.title(u))
	var f TTSFigure
	var lines []ttsDLine
	switch {
	case l.Extra == "mount":
		f.Name = "[b]" + owner + "[/b]'s " + ttsSafe(l.Name)
		lines = n.mountLines(u, l)
	case l.Creature:
		c, ok := n.creature(u, l)
		if !ok {
			f.Name = "[b]" + owner + "[/b]'s " + ttsSafe(l.Name)
			break
		}
		f.Name = "[b]" + owner + "[/b]'s " + ttsSafe(c.Name)
		if c.Points > 0 {
			f.Name += fmt.Sprintf(" (%d)", c.Points)
		}
		lines = n.creatureLines(u, c, l)
	default:
		f.Name = fmt.Sprintf("[b]%s[/b] (%d)", owner, u.Points)
		if u.Wanderer {
			f.Name += " ~ as " + ttsSafe(l.Name)
		}
		if l.Extra == "foot" {
			f.Name += " (on foot)"
		}
		lines = n.unitLines(u, l)
	}
	if fallen {
		lines = append([]ttsDLine{{"[" + ttsColourDown + "][b]Fallen[/b]: wounded or out of the fight[-]"}}, lines...)
	}
	var cut bool
	f.Desc, cut = fitDescription(lines, TTSDescriptionMax)
	if cut {
		n.cut++
	}
	return f
}

// ttsDLine is a line of a description with the forms it can take, the longest first.
type ttsDLine []string

func (n *ttsNamer) creature(u CompanyUnit, l TTSLine) (CompanyCreature, bool) {
	for _, c := range n.d.Creatures {
		if c.OwnerID != u.ID {
			continue
		}
		for _, s := range n.models.StandIns {
			if s.Kind == "creature" && strings.EqualFold(s.Name, c.Name) {
				name, opts := splitModel(s.Model)
				if newTTSLine(u.ID, name, opts, "").Key == l.Key {
					return c, true
				}
			}
		}
	}
	return CompanyCreature{}, false
}

var ttsRoleLabel = map[UnitRole]string{RoleLeader: "Leader", RoleSergeant: "Sergeant", RoleHero: "Hero", RoleWarrior: "Warrior"}

func (n *ttsNamer) unitLines(u CompanyUnit, l TTSLine) []ttsDLine {
	head := []string{ttsRoleLabel[u.Role]}
	if u.Path != nil && u.Path.Name != "" {
		head = append(head, ttsSafe(u.Path.Name))
	}
	if l.Extra == "foot" {
		head = append(head, "on foot")
	}
	lines := []ttsDLine{{"[i]" + strings.Join(head, ttsHeaderSep) + "[/i]"}}
	var plain, hero []Attribute
	for _, a := range u.Stats {
		if a.HeroOnly {
			hero = append(hero, a)
		} else {
			plain = append(plain, a)
		}
	}
	lines = append(lines, ttsStatLines(plain, hero)...)
	// The same split as the unit card: a mount, wargear (no slot) and equipment (takes a slot); named unit upgrades are neither.
	var mounts, gear, equip []string
	for _, e := range u.Equipment {
		switch {
		case e.Upgrade && n.named[strings.ToLower(e.Name)]: // a named unit upgrade names the unit and adds its rules and stats; no line of its own
		case slices.Contains(e.Classes(), EquipMount):
			if l.Extra == "foot" {
				continue // the rider on foot has no mount
			}
			mounts = append(mounts, ttsSafe(e.Name))
		case e.Size != "":
			equip = append(equip, ttsSafe(e.Name))
		default:
			gear = append(gear, ttsSafe(e.Name))
		}
	}
	for _, sec := range []struct {
		title string
		items []string
	}{{"Mount", mounts}, {"Wargear", gear}, {"Equipment", equip}} {
		if len(sec.items) > 0 {
			lines = append(lines, ttsDLine{"[b]" + sec.title + ":[/b] " + strings.Join(sec.items, ", ")})
		}
	}
	// Heroic actions: the path's own, never the default ones every hero has.
	if u.Path != nil {
		var acts []string
		for _, a := range u.Path.HeroicActions {
			if a.Source == ActionFromPath {
				acts = append(acts, ttsSafe(a.Name))
			}
		}
		if len(acts) > 0 {
			lines = append(lines, ttsDLine{"[b]Heroic actions:[/b] " + strings.Join(acts, ", ")})
		}
	}
	var rules []UnitRule
	rules = append(rules, u.Rules...)
	if up, ok := n.upgrade(u); ok && !n.upgradeCovered(up) {
		rules = append(rules, UnitRule{Name: up.Name, Description: up.Description})
	}
	if u.Path != nil {
		rules = append(rules, u.Path.Rules...)
		rules = append(rules, u.Path.Powers...)
	}
	lines = append(lines, n.ruleLines(rules)...)
	var inj []string
	for _, in := range u.Injuries {
		s := ttsSafe(in.Name)
		if in.Count > 1 {
			s += fmt.Sprintf(" x%d", in.Count)
		}
		inj = append(inj, s)
	}
	for _, a := range u.LostAbilities {
		inj = append(inj, "lost "+ttsSafe(a.Ability))
	}
	if u.Injured {
		inj = append(inj, "misses the next game")
	}
	if len(inj) > 0 {
		lines = append(lines, ttsDLine{"[" + ttsColourDown + "][b]Injuries:[/b] " + strings.Join(inj, ", ") + "[-]"})
	}
	return lines
}

func (n *ttsNamer) creatureLines(u CompanyUnit, c CompanyCreature, l TTSLine) []ttsDLine {
	head := "Creature of " + ttsSafe(n.title(u))
	if !strings.EqualFold(c.Name, l.Name) {
		head += ", shown as " + ttsSafe(l.Name)
	}
	lines := []ttsDLine{{"[i]" + head + "[/i]"}}
	lines = append(lines, ttsStatLines(c.Stats, nil)...)
	lines = append(lines, n.keywordLines(c.Rules)...)
	for _, note := range c.Notes {
		lines = append(lines, ttsDLine{ttsSafe(note)})
	}
	return lines
}

func (n *ttsNamer) mountLines(u CompanyUnit, l TTSLine) []ttsDLine {
	lines := []ttsDLine{{"[i]Mount of " + ttsSafe(n.title(u)) + "[/i]"}}
	for _, m := range n.models.Mounts {
		if name, _ := splitModel(m.Model); !strings.EqualFold(name, l.Name) {
			continue
		}
		for _, e := range u.Equipment {
			if namesItem(e.Name, e.Spellings, m.Gear) {
				lines = append(lines, ttsStatLines(e.MountStats, nil)...)
				lines = append(lines, n.keywordLines(e.MountRules)...)
				return lines
			}
		}
	}
	return lines
}

// upgradeCovered says that everything a named upgrade adds is already in the profile: its stat changes are in the table (coloured), the
// rules it grants are in the Rules section. What is left is a rule with no name of its own (Lorien Guard's re-roll), which then shows
// under the upgrade's name.
func (n *ttsNamer) upgradeCovered(e Equipment) bool {
	if a, ok := n.rs.BC.Annotation(e.Name); ok && len(a.Grants) > 0 {
		return true
	}
	if g, ok := n.rs.Vanilla.Gear(e.Name); ok && len(g.Modifies) > 0 {
		return true
	}
	return strings.TrimSpace(e.Description) == ""
}

// keywordLines are rules known by name only; their texts are the glossary's.
func (n *ttsNamer) keywordLines(names []string) []ttsDLine {
	var rules []UnitRule
	for _, name := range names {
		text, _ := n.rs.Vanilla.Keyword(name)
		rules = append(rules, UnitRule{Name: name, Description: text})
	}
	return n.ruleLines(rules)
}

// ruleLines are the special rules at the level of the payload: a line per rule with its text, or one line of names.
func (n *ttsNamer) ruleLines(rules []UnitRule) []ttsDLine {
	var out []ttsDLine
	var names []string
	seen := map[string]bool{}
	for _, r := range rules {
		name := ttsSafe(r.Shown())
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		names = append(names, name)
		var text string
		switch n.level {
		case TTSRulesFull:
			text = ttsSafe(r.Description)
		case TTSRulesShort:
			text = ttsFirstSentence(ttsSafe(r.Description), TTSRuleShortMax)
		}
		if text != "" {
			out = append(out, ttsDLine{"[b]" + name + "[/b]: " + text, "[b]" + name + "[/b]"})
		} else if n.level != TTSRulesNone {
			out = append(out, ttsDLine{"[b]" + name + "[/b]"})
		}
	}
	if n.level == TTSRulesNone && len(names) > 0 {
		out = append(out, ttsDLine{"[b]Rules:[/b] " + strings.Join(names, ", ")})
	}
	return out
}

// ttsStatLines are the stat table (two rows, names over values, changed values green or red, columns padded for the font of the game; see
// ttsfont.go) and the line of Might, Will and Fate.
func ttsStatLines(plain, hero []Attribute) []ttsDLine {
	var out []ttsDLine
	var names, vals []string
	for _, a := range plain {
		v := fmt.Sprintf("%d%s", a.Value, a.Trail)
		switch delta := a.Boost + a.Gear; {
		case delta == 0:
		case (delta > 0) != (a.Trail == "+"):
			v = "[" + ttsColourUp + "]" + v + "[-]"
		default:
			v = "[" + ttsColourDown + "]" + v + "[-]"
		}
		names, vals = append(names, ttsSafe(a.Abbr)), append(vals, v)
	}
	if len(names) > 0 {
		r1, r2 := ttsTable(names, vals)
		out = append(out, ttsDLine{r1}, ttsDLine{r2})
	}
	var h []string
	for _, a := range hero {
		h = append(h, fmt.Sprintf("%s %d", a.Name, a.Value))
	}
	if len(h) > 0 {
		out = append(out, ttsDLine{"[" + ttsColourHeroStats + "]" + strings.Join(h, "  ") + "[-]"})
	}
	return out
}

// fitDescription joins the lines with newlines, using for each the longest form that leaves room; a line that fits in no form is left
// out, and then the text ends with "...". cut says that something was shortened or left out.
func fitDescription(lines []ttsDLine, limit int) (string, bool) {
	size := func(forms []string) int {
		n := 0
		for _, s := range forms {
			n += utf8.RuneCountInString(s) + 1
		}
		return n - 1
	}
	first := make([]string, len(lines))
	for i, l := range lines {
		first[i] = l[0]
	}
	if size(first) <= limit {
		return strings.Join(first, "\n"), false
	}
	const tail = "\n..."
	budget := limit - len(tail)
	var out []string
	used := 0
	cut, skipped := false, false
	for _, l := range lines {
		placed := false
		for k, form := range l {
			w := utf8.RuneCountInString(form)
			if len(out) > 0 {
				w++
			}
			if used+w <= budget {
				out = append(out, form)
				used += w
				placed = true
				cut = cut || k > 0
				break
			}
		}
		skipped = skipped || !placed
	}
	s := strings.Join(out, "\n")
	if skipped {
		s += tail
	}
	return s, cut || skipped
}

// ttsSafe makes a text safe for the rich text of the game and for JSON: brackets (which would start a colour code) become round ones,
// control characters and line separators become spaces, and spaces are single.
func ttsSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '[':
			return '('
		case r == ']':
			return ')'
		case r == ' ', r == ' ', unicode.IsControl(r), r == utf8.RuneError:
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// ttsFirstSentence is the first sentence of s, at most max characters (cut at a word with "..." when longer).
func ttsFirstSentence(s string, max int) string {
	rs := []rune(s)
	end := len(rs)
	for i, r := range rs {
		if (r == '.' || r == '!' || r == '?') && (i+1 == len(rs) || rs[i+1] == ' ') {
			w := ttsLastWord(rs[:i])
			if r == '.' && (len(w) <= 1 || slices.Contains([]string{"e.g", "i.e", "etc", "vs", "no"}, strings.ToLower(w))) {
				continue
			}
			end = i + 1
			break
		}
	}
	rs = rs[:end]
	if len(rs) <= max {
		return string(rs)
	}
	cut := rs[:max-3]
	if rs[max-3] != ' ' { // back up to a word boundary
		for j := len(cut) - 1; j > 0; j-- {
			if cut[j] == ' ' {
				cut = cut[:j]
				break
			}
		}
	}
	return strings.TrimRight(string(cut), " ,;:") + "..."
}

func ttsLastWord(rs []rune) string {
	i := len(rs)
	for i > 0 && rs[i-1] != ' ' {
		i--
	}
	return string(rs[i:])
}
