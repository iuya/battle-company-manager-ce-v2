package core

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// The Tabletop Simulator army list (docs/tabletop-simulator.md): a company, in the order of its My Company page, written as the
// Notebook text the MESBG FTC mod's Spawn Army button reads. Pure functions over a built company and the rules: no repository, no
// clock, no state.

// TTSOptions are the switches of the dialog.
type TTSOptions struct {
	FootVersions bool // add the on-foot line of a rider whose figure has no dismount state
	MountFigures bool // add a figure line for a mount that fights on its own (vanilla Attacks above 0)
	// Catalogue is the model keys of the mod's bag, when known (nil: every key is taken to exist). Set by the service, not the dialog.
	Catalogue TTSCatalogue
}

// TTSCatalogue is the set of model keys of the mod's bag (docs/tabletop-simulator.md, "Keys the bag does not have"). It is the mod's
// content and is never committed: a local file (sources/tts/models.json, or TTS_CATALOGUE) feeds it, and nil means unknown.
type TTSCatalogue interface {
	// Has says that the key (lower case, trimmed: "name: option a, option b") is in the bag.
	Has(key string) bool
	// Keys are the keys of the figures named name, as the mod spells them ("Mahûd Warrior: Spear"), bare ones included.
	Keys(name string) []string
}

// Note kinds.
const (
	TTSNoModel       = "noModel"       // the unit has no figure and is not in the list
	TTSOptionDropped = "optionDropped" // something the unit carries has no option on its figure and is left out
	TTSShownAs       = "shownAs"       // a stand-in figure
	TTSApproximate   = "approximate"   // the closest figure, not an exact one
	TTSHasStates     = "hasStates"     // the figure dismounts by changing its state
	TTSExtraFigure   = "extraFigure"   // a figure added by a switch
	TTSFallen        = "fallen"        // the second warband of a live match
)

// TTSNote is something the player should know about the list; UnitID is the company unit it is about (0: the whole list).
type TTSNote struct {
	UnitID int64
	Kind   string
	Text   string // one sentence for the player
}

// TTSLine is one line of the list.
type TTSLine struct {
	UnitIDs []int64  // the company units this line stands for (several when identical adjacent units are merged); an extra figure carries its rider's id
	Name    string   // as the mod names it, no brackets
	Count   int      // the number of figures; the first line of a warband is the hero line and is written without it
	Options []string // sorted, case-insensitively
	Key     string   // lower-cased "name: options", the mod's key for the figure
	Extra   string   // "" for a unit or creature of the company, "foot" for the rider on foot, "mount" for the mount alone
	// Creature says that the figure is a creature an officer owns (shown as a stand-in), not the unit itself; UnitIDs are the owners.
	Creature bool
	// Dropped are the marker options (Lorien Guard) the figure has no key for: the unit has them, the figure does not show them.
	Dropped []string
}

// Text is the line as the mod reads it: `(Name: Option A, Option B)`, or `(3x Name: )` for a follower. hero says it is the first line of
// its warband.
func (l TTSLine) Text(hero bool) string {
	s := l.Name + ": " + strings.Join(l.Options, ", ")
	if !hero {
		s = fmt.Sprintf("%dx %s", l.Count, s)
	}
	return "(" + s + ")"
}

// TTSWarband is a group of lines: the first one is the hero line, the others follow it.
type TTSWarband struct {
	Lines  []TTSLine
	Fallen bool // the second warband of a live match: the units that are wounded or out of the fight
}

// TTSExport is the list and what to tell the player about it.
type TTSExport struct {
	Text     string // paste-ready; warbands apart by a blank line
	Warbands []TTSWarband
	Notes    []TTSNote
	// Models are the figures of the company's units and creatures in the list; ExtraFigures the on-foot and mount figures the switches
	// added. The mod reports Models+ExtraFigures.
	Models, ExtraFigures int
	// BreakPercent and BreakPoint are the company's Break Point rule applied to Models ("6.5" for 13 models at 50%), as the live match
	// shows it.
	BreakPercent int
	BreakPoint   string
}

// Lines are the lines of every warband.
func (e TTSExport) Lines() []TTSLine {
	var out []TTSLine
	for _, w := range e.Warbands {
		out = append(out, w.Lines...)
	}
	return out
}

// TTSLive is what a live match knows about a company's units: the plain export ignores it.
type TTSLive struct {
	Units  []LiveUnit // LiveCompany.Units
	Absent []int64    // LiveCompany.Absent: units that miss the match (injured, at the healers)
}

// out lists the units that are not on the table: retreated, casualties and absent ones.
func (l TTSLive) out() map[int64]bool {
	m := map[int64]bool{}
	for _, u := range l.Units {
		if u.Zone == ZoneRetreated || u.Zone == ZoneCasualty {
			m[u.UnitID] = true
		}
	}
	for _, id := range l.Absent {
		m[id] = true
	}
	return m
}

// TTSList writes the company (d.Roster, in the order of the page) as a Tabletop Simulator army list.
func (r *RuleSet) TTSList(d CompanyDetail, opts TTSOptions) TTSExport {
	x := r.newTTS(d, opts)
	return x.export([]ttsGroup{{units: d.Roster}})
}

// TTSLiveList is TTSList for a live match: every unit is listed, but the ones that are wounded or out of the fight are in a second
// warband after the main one, with a note.
func (r *RuleSet) TTSLiveList(d CompanyDetail, live TTSLive, opts TTSOptions) TTSExport {
	x := r.newTTS(d, opts)
	out := live.out()
	var field, fallen []CompanyUnit
	for _, u := range d.Roster {
		if out[u.ID] {
			fallen = append(fallen, u)
		} else {
			field = append(field, u)
		}
	}
	return x.export([]ttsGroup{{units: field}, {units: fallen, fallen: true}})
}

type ttsGroup struct {
	units  []CompanyUnit
	fallen bool
}

type ttsBuild struct {
	rs      *RuleSet
	profile CompanyProfile
	d       CompanyDetail
	models  TTSModels
	opts    TTSOptions
	notes   []TTSNote
	silent  bool // build without notes (the foot version of a rider)
}

func (r *RuleSet) newTTS(d CompanyDetail, opts TTSOptions) *ttsBuild {
	x := &ttsBuild{rs: r, d: d, opts: opts}
	x.profile, _ = r.BC.Profile(d.Faction)
	x.models = r.BC.TTSModels()
	return x
}

func (x *ttsBuild) note(id int64, kind, format string, args ...any) {
	if !x.silent {
		x.notes = append(x.notes, TTSNote{UnitID: id, Kind: kind, Text: fmt.Sprintf(format, args...)})
	}
}

func (x *ttsBuild) export(groups []ttsGroup) TTSExport {
	var e TTSExport
	var blocks []string
	for _, g := range groups {
		lines := x.warband(g.units)
		if len(lines) == 0 {
			continue
		}
		e.Warbands = append(e.Warbands, TTSWarband{Lines: lines, Fallen: g.fallen})
		var b strings.Builder
		for i, l := range lines {
			if i > 0 {
				b.WriteString("\n    ")
			}
			b.WriteString(l.Text(i == 0))
		}
		blocks = append(blocks, b.String())
		for _, l := range lines {
			if l.Extra == "foot" || l.Extra == "mount" {
				e.ExtraFigures += l.Count
			} else {
				e.Models += l.Count
			}
		}
		if g.fallen {
			var names []string
			for _, u := range g.units {
				names = append(names, u.Title())
			}
			x.note(0, TTSFallen, "The units that are wounded or out of the fight (%s) are in a second warband after the main one.", strings.Join(names, ", "))
		}
	}
	e.Text = strings.Join(blocks, "\n\n")
	e.Notes = x.notes
	e.BreakPercent = x.d.BreakPointPercent
	if e.BreakPercent == 0 {
		e.BreakPercent = x.rs.BC.BreakRules().DefaultPercent
	}
	t := e.BreakPercent * e.Models
	e.BreakPoint = fmt.Sprintf("%d.%d", t/100, (t%100)/10)
	return e
}

// warband is the lines of the units: the first line is the hero line, then followers, identical adjacent units merged.
func (x *ttsBuild) warband(units []CompanyUnit) []TTSLine {
	type group struct {
		sig   string
		lines []TTSLine
	}
	var head []TTSLine
	var groups []*group
	for _, u := range units {
		ul := x.unitLines(u)
		if len(ul) == 0 {
			continue
		}
		if head == nil {
			head = ul
			continue
		}
		var sig []string
		for _, l := range ul {
			sig = append(sig, l.Extra+"|"+l.Key+"|"+strings.Join(l.Dropped, ","))
		}
		s := strings.Join(sig, "\n")
		if n := len(groups); n > 0 && groups[n-1].sig == s {
			for i := range ul {
				groups[n-1].lines[i].Count++
				groups[n-1].lines[i].UnitIDs = append(groups[n-1].lines[i].UnitIDs, ul[i].UnitIDs...)
			}
			continue
		}
		groups = append(groups, &group{s, ul})
	}
	lines := head
	for _, g := range groups {
		lines = append(lines, g.lines...)
	}
	return lines
}

// unitLines are the lines of one unit with Count 1: its figure, its creature, then the extra figures.
func (x *ttsBuild) unitLines(u CompanyUnit) []TTSLine {
	var lines []TTSLine
	switch {
	case u.Wanderer:
		s, ok := x.standIn("wanderer", u.Type)
		if !ok {
			x.note(u.ID, TTSNoModel, "%s has no figure in the mod and is not in the list.", u.Title())
			return nil
		}
		lines = append(lines, x.standInLine(u, s))
	default:
		v, _, ok := resolveUnit(x.rs, x.profile, u.Type)
		if !ok {
			x.note(u.ID, TTSNoModel, "%s (%s) is not a standard unit and is not in the list.", u.Title(), u.Type)
			return nil
		}
		lines = append(lines, x.vanillaLines(u, v)...)
		if len(lines) == 0 {
			return nil
		}
	}
	for _, c := range x.d.Creatures {
		if c.OwnerID != u.ID {
			continue
		}
		if s, ok := x.standIn("creature", c.Name); ok {
			lines = append(lines, x.standInLine(u, s))
			x.note(u.ID, TTSShownAs, "%s's %s is shown as %s.", u.Title(), c.Name, s.Model)
		} else {
			x.note(u.ID, TTSNoModel, "%s's %s has no figure in the mod and is not in the list.", u.Title(), c.Name)
		}
	}
	return lines
}

func (x *ttsBuild) standIn(kind, name string) (TTSStandIn, bool) {
	for _, s := range x.models.StandIns {
		if s.Kind == kind && strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return TTSStandIn{}, false
}

func (x *ttsBuild) standInLine(u CompanyUnit, s TTSStandIn) TTSLine {
	name, options := splitModel(s.Model)
	if u.Wanderer {
		x.note(u.ID, TTSShownAs, "%s is shown as %s.", u.Title(), s.Model)
	}
	l := newTTSLine(u.ID, name, options, "")
	l.Creature = s.Kind == "creature"
	return l
}

// vanillaLines are the lines of a unit that is a vanilla profile.
func (x *ttsBuild) vanillaLines(u CompanyUnit, v VanillaUnit) []TTSLine {
	var lines []TTSLine
	if len(v.Includes) > 0 { // a bundle is its members
		for _, in := range v.Includes {
			m, ok := x.rs.Vanilla.Unit(in)
			if !ok {
				x.note(u.ID, TTSNoModel, "%s of %s is not a standard unit and is not in the list.", in, u.Title())
				continue
			}
			lines = append(lines, x.unitLine(u, m))
		}
		return lines
	}
	lines = append(lines, x.unitLine(u, v))
	rider, isRider := x.rider(v.Name)
	switch {
	case isRider && rider.States:
		x.note(u.ID, TTSHasStates, "%s: change the figure's state to dismount it.", u.Title())
	case isRider && x.opts.FootVersions:
		if rider.FootModel == "" {
			x.note(u.ID, TTSNoModel, "%s has no on-foot figure in the mod; only the mounted one is in the list.", u.Title())
		} else if foot, ok := x.rs.Vanilla.Unit(rider.FootModel); !ok {
			x.note(u.ID, TTSNoModel, "%s has no on-foot figure in the mod; only the mounted one is in the list.", u.Title())
		} else {
			x.silent = true
			l := x.unitLine(u, foot)
			x.silent = false
			l.Extra = "foot"
			lines = append(lines, l)
			x.note(u.ID, TTSExtraFigure, "%s also has an on-foot figure (%s), for when it dismounts.", u.Title(), l.Name)
		}
	}
	if x.opts.MountFigures && !(isRider && rider.States) {
		for _, m := range x.models.Mounts {
			if m.Attacks <= 0 || !ttsCarries(u.Equipment, m.Gear) {
				continue
			}
			name, options := splitModel(m.Model)
			l := newTTSLine(u.ID, name, options, "mount")
			lines = append(lines, l)
			x.note(u.ID, TTSExtraFigure, "%s also has its %s as a figure of its own (%s).", u.Title(), m.Gear, m.Model)
			break
		}
	}
	return lines
}

func (x *ttsBuild) rider(unit string) (TTSRider, bool) {
	for _, r := range x.models.Riders {
		if strings.EqualFold(r.Unit, unit) {
			return r, true
		}
	}
	return TTSRider{}, false
}

func ttsCarries(eq []Equipment, gear string) bool {
	return slices.ContainsFunc(eq, func(e Equipment) bool { return namesItem(e.Name, e.Spellings, gear) })
}

// unitLine is the figure of unit v for what u carries.
func (x *ttsBuild) unitLine(u CompanyUnit, v VanillaUnit) TTSLine {
	options := x.optionsFor(u, v)
	for i, o := range options {
		for _, n := range x.models.Names {
			if strings.EqualFold(n.Unit, v.Name) && strings.EqualFold(n.From, o) {
				options[i] = n.To
			}
		}
	}
	var dropped []string
	if len(options) == 1 {
		for _, m := range x.models.MarkerOptions {
			if strings.EqualFold(m.Unit, v.Name) && strings.EqualFold(m.Option, options[0]) {
				dropped, options = []string{options[0]}, nil
				x.note(u.ID, TTSApproximate, "%s: the mod has no figure with %s alone, so %s is shown; the name and description still say so.", u.Title(), dropped[0], figureName(newTTSLine(u.ID, v.Name, nil, "")))
			}
		}
	}
	if len(options) == 0 && dropped == nil {
		for _, d := range x.models.DefaultOptions {
			if strings.EqualFold(d.Unit, v.Name) {
				options = slices.Clone(d.Options)
				x.note(u.ID, TTSApproximate, "%s: the mod has no figure for %s, so %s is shown (the closest one the mod has).", u.Title(), figureName(newTTSLine(u.ID, v.Name, nil, "")), figureName(newTTSLine(u.ID, v.Name, options, "")))
			}
		}
	}
	l := newTTSLine(u.ID, v.Name, options, "")
	l.Dropped = dropped
	if x.opts.Catalogue != nil && !x.opts.Catalogue.Has(strings.TrimSpace(l.Key)) {
		l = x.fallback(u, v, l)
	}
	return l
}

func newTTSLine(id int64, name string, options []string, extra string) TTSLine {
	name = ttsClean(name)
	var opts []string
	for _, o := range options {
		if o = ttsClean(o); o != "" {
			opts = append(opts, o)
		}
	}
	slices.SortFunc(opts, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return TTSLine{UnitIDs: []int64{id}, Name: name, Count: 1, Options: opts, Key: strings.ToLower(name + ": " + strings.Join(opts, ", ")), Extra: extra}
}

// ttsClean takes the brackets out of a name: the mod reads the text between the last pair of brackets of a line.
func ttsClean(s string) string {
	s = strings.NewReplacer("(", "", ")", "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// splitModel reads "Name: Option A, Option B".
func splitModel(model string) (string, []string) {
	name, rest, ok := strings.Cut(model, ":")
	if !ok {
		return strings.TrimSpace(model), nil
	}
	var out []string
	for _, o := range strings.Split(rest, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return strings.TrimSpace(name), out
}

// optionParts are the items an option has to be carried to be taken: all of them, or for an exchange ("Exchange shield for banner")
// the item it gives.
func optionParts(name string) []string {
	if optionVerb.MatchString(name) {
		stripped := optionVerb.ReplaceAllString(name, "")
		for _, sep := range []string{" for ", " with "} {
			if _, after, ok := strings.Cut(strings.ToLower(stripped), sep); ok {
				return OptionItems(stripped[len(stripped)-len(after):])
			}
		}
	}
	return OptionItems(name)
}

func isExchange(name string) bool { return optionVerb.MatchString(name) }

// optionsFor are the options of v that u's wargear stands for: whole options first (the one that covers most items), at most one
// per choose-one group; what is left over goes to the option that holds it, as the closest figure, or is dropped with a note.
func (x *ttsBuild) optionsFor(u CompanyUnit, v VanillaUnit) []string {
	var items []Equipment
	for _, e := range u.Equipment {
		if (e.Profile && !e.Upgrade) || e.Size != "" || e.Modifies != "" {
			continue
		}
		items = append(items, e)
	}
	type cand struct {
		i       int
		covers  []int
		partial bool
	}
	parts := make([][]string, len(v.Options))
	var cands []cand
	for i, o := range v.Options {
		parts[i] = optionParts(o.Name)
		if len(parts[i]) == 0 {
			continue
		}
		var covers []int
		full := true
		for _, p := range parts[i] {
			j := slices.IndexFunc(items, func(e Equipment) bool { return namesItem(e.Name, e.Spellings, p) })
			if j < 0 {
				full = false
				continue
			}
			covers = append(covers, j)
		}
		if len(covers) > 0 {
			cands = append(cands, cand{i, covers, !full})
		}
	}
	key := func(i int) OptionKey {
		if i < len(v.OptionKeys) {
			return v.OptionKeys[i]
		}
		return OptionKey{}
	}
	var chosen []int
	used := make([]bool, len(items))
	groups := map[string]bool{}
	take := func(c cand) bool {
		if k := key(c.i).Key; k != "" && groups[k] {
			return false
		}
		if !slices.ContainsFunc(c.covers, func(j int) bool { return !used[j] }) {
			return false
		}
		for _, j := range c.covers {
			used[j] = true
		}
		if k := key(c.i).Key; k != "" {
			groups[k] = true
		}
		chosen = append(chosen, c.i)
		return true
	}
	order := func(partial bool) []cand {
		var out []cand
		for _, c := range cands {
			if c.partial == partial {
				out = append(out, c)
			}
		}
		slices.SortStableFunc(out, func(a, b cand) int {
			if d := len(b.covers) - len(a.covers); d != 0 {
				return d
			}
			if partial { // the option with the fewest extras
				if d := len(parts[a.i]) - len(parts[b.i]); d != 0 {
					return d
				}
			}
			if d := btoi(isExchange(v.Options[a.i].Name)) - btoi(isExchange(v.Options[b.i].Name)); d != 0 {
				return d
			}
			return a.i - b.i
		})
		return out
	}
	for _, c := range order(false) {
		take(c)
	}
	for _, c := range order(true) {
		anyNew := false
		for _, j := range c.covers {
			anyNew = anyNew || !used[j]
		}
		if anyNew && take(c) {
			x.note(u.ID, TTSApproximate, "%s: %q is the closest figure to what it carries (%s).", u.Title(), v.Options[c.i].Name, itemNames(items, c.covers))
		}
	}
	for j, e := range items {
		if used[j] || e.Class == EquipArmour || isSteedArmour(e.Name) { // armour upgrades are not options of a figure
			continue
		}
		x.note(u.ID, TTSOptionDropped, "%s: %s has no matching option on the figure and is left out.", u.Title(), e.Name)
	}
	required := map[string]int{} // a group the model must take one of: the cheapest option if none was taken
	for i := range v.Options {
		if k := key(i); k.Key != "" && k.Required && !groups[k.Key] {
			if j, seen := required[k.Key]; !seen || v.Options[i].Points < v.Options[j].Points {
				required[k.Key] = i
			}
		}
	}
	for _, k := range slices.Sorted(mapKeys(required)) {
		i := required[k]
		chosen = append(chosen, i)
		x.note(u.ID, TTSApproximate, "%s: the mod has no figure without %q, so that is the one shown.", u.Title(), v.Options[i].Name)
	}
	slices.Sort(chosen)
	var out []string
	for _, i := range chosen {
		out = append(out, v.Options[i].Name)
	}
	return out
}

func mapKeys[K comparable, V any](m map[K]V) func(func(K) bool) {
	return func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func itemNames(items []Equipment, idx []int) string {
	var names []string
	for _, j := range idx {
		names = append(names, items[j].Name)
	}
	return strings.Join(names, ", ")
}

// markers are the options a company's unit alias selects (Lorien Guard): they only name the unit, the figure needs none of them.
func (x *ttsBuild) markers() map[string]bool {
	m := map[string]bool{}
	for _, a := range x.profile.Aliases {
		for _, o := range a.Selected {
			m[strings.ToLower(o)] = true
		}
	}
	return m
}

// fallback is the closest figure the bag has for line l, whose exact key it does not have, with an approximate note. First the marker
// options (Lorien Guard) are dropped, if the rest is a key; else the closest key of the unit is taken (see closestKey).
func (x *ttsBuild) fallback(u CompanyUnit, v VanillaUnit, l TTSLine) TTSLine {
	cat := x.opts.Catalogue
	markers := x.markers()
	var kept, dropped []string
	for _, o := range l.Options {
		if markers[strings.ToLower(o)] {
			dropped = append(dropped, o)
		} else {
			kept = append(kept, o)
		}
	}
	if len(dropped) > 0 {
		if nl := newTTSLine(u.ID, l.Name, kept, l.Extra); cat.Has(strings.TrimSpace(nl.Key)) {
			nl.Dropped = dropped
			x.note(u.ID, TTSApproximate, "%s: the mod has no figure with %s, so %s is shown; the name and description still say so.", u.Title(), strings.Join(dropped, ", "), figureName(nl))
			return nl
		}
	}
	best, ok := x.closestKey(v, l.Name, kept, cat)
	if !ok {
		return l
	}
	name, opts := splitModel(best)
	nl := newTTSLine(u.ID, name, opts, l.Extra)
	nl.Dropped = dropped
	x.note(u.ID, TTSApproximate, "%s: the mod has no figure for %s, so %s is shown (the closest one the mod has).", u.Title(), figureName(l), figureName(nl))
	return nl
}

func figureName(l TTSLine) string {
	if len(l.Options) == 0 {
		return l.Name + " with no options"
	}
	return l.Name + " with " + strings.Join(l.Options, ", ")
}

// Words of an option that decide how close it is to a plain melee figure.
var (
	ttsAvoidWords = []string{"bow", "crossbow", "blowpipe", "throwing", "banner", "horn", "sling"}
	ttsMeleeWords = []string{"spear", "sword", "axe", "mace", "hammer", "lance", "pike", "halberd", "glaive", "flail", "club", "scimitar", "weapon", "pick"}
)

func hasAnyWord(opt string, words []string) bool {
	o := strings.ToLower(opt)
	return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(o, w) })
}

// closestKey picks among the keys of the unit the bag has: the one that keeps most of the wanted options, then the one without ranged
// weapons, banner or war horn, then the one with a melee weapon, then the cheapest (the points of the unit's options), then by key.
func (x *ttsBuild) closestKey(v VanillaUnit, name string, wanted []string, cat TTSCatalogue) (string, bool) {
	type cand struct {
		key                   string
		matched, avoid, melee int
		points                int
	}
	points := func(opt string) int {
		for _, n := range x.models.Names {
			if strings.EqualFold(n.Unit, v.Name) && strings.EqualFold(n.To, opt) {
				opt = n.From
			}
		}
		for _, o := range v.Options {
			if strings.EqualFold(o.Name, opt) {
				return o.Points
			}
		}
		return 1000
	}
	var cands []cand
	for _, k := range cat.Keys(name) {
		_, opts := splitModel(k)
		if len(opts) == 0 {
			continue
		}
		c := cand{key: k}
		for _, o := range opts {
			if slices.ContainsFunc(wanted, func(w string) bool { return strings.EqualFold(w, o) }) {
				c.matched++
				continue
			}
			c.avoid += btoi(hasAnyWord(o, ttsAvoidWords))
			c.melee += btoi(hasAnyWord(o, ttsMeleeWords))
			c.points += points(o)
		}
		cands = append(cands, c)
	}
	if len(cands) == 0 {
		return "", false
	}
	slices.SortFunc(cands, func(a, b cand) int {
		switch {
		case a.matched != b.matched:
			return b.matched - a.matched
		case a.avoid != b.avoid:
			return a.avoid - b.avoid
		case (a.melee > 0) != (b.melee > 0):
			return btoi(b.melee > 0) - btoi(a.melee > 0)
		case a.points != b.points:
			return a.points - b.points
		}
		return strings.Compare(strings.ToLower(a.key), strings.ToLower(b.key))
	})
	return cands[0].key, true
}

// MapCatalogue is a TTSCatalogue over a list of keys.
type MapCatalogue struct {
	keys   map[string]bool
	byName map[string][]string
}

// NewMapCatalogue indexes keys such as "Mahûd Warrior: Spear" (case and surrounding spaces do not matter).
func NewMapCatalogue(keys []string) *MapCatalogue {
	c := &MapCatalogue{keys: map[string]bool{}, byName: map[string][]string{}}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		lk := strings.ToLower(k)
		if c.keys[lk] {
			continue
		}
		c.keys[lk] = true
		name, _ := splitModel(k)
		c.byName[strings.ToLower(name)] = append(c.byName[strings.ToLower(name)], k)
	}
	return c
}

func (c *MapCatalogue) Has(key string) bool { return c.keys[strings.ToLower(strings.TrimSpace(key))] }

func (c *MapCatalogue) Keys(name string) []string {
	return c.byName[strings.ToLower(strings.TrimSpace(name))]
}

// LoadTTSCatalogue reads a catalogue file (tools/extract_tts_models.py: a JSON list of keys, or an object keyed by them).
func LoadTTSCatalogue(path string) (*MapCatalogue, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return NewMapCatalogue(list), nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("catalogue %s: neither a list nor an object of keys: %w", path, err)
	}
	return NewMapCatalogue(slices.Collect(mapKeys(obj))), nil
}
