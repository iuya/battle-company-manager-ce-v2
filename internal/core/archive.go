package core

// The archive of an ended campaign (docs/archive.md): ONE document that holds everything the read-only pages of the campaign need - its
// settings, the stored state of every company, the matches with their results and history, the grants - and the three rules files pruned
// to what the campaign used, so the campaign stops pinning the rules files and its relational rows can go. The pruned triple loads with the
// app's own loader (a "mini rule set"), so the pages and services that read a rule set are unchanged.
//
// The pruning works on generic JSON (maps and slices); the pruned Battle Companies file carries "archive": true, which lets its
// scenario selection tables stay empty (no match is rolled again).

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ArchiveFormat is the version of the Archive document. A reader refuses a newer one.
const ArchiveFormat = 1

// RuleFiles are the three stored rules files of a campaign, as bytes.
type RuleFiles struct{ Vanilla, BattleCompanies, Glossary []byte }

// Archive is the whole of an ended campaign in one document.
type Archive struct {
	Version     int
	CampaignID  int64
	Name        string
	Description string
	Round       int
	Rounds      int
	EndedAt     time.Time
	Settings    CampaignSettings
	// Files describes the three files the campaign was played with (what the Rules box of the Overview shows), whatever was pruned from them.
	Files []RulesetInfo
	// Companies are the stored states (the plain data the pages are built from), stripped of what only an open campaign needs.
	Companies []CompanyState
	Matches   []Match
	// Events is the history of ending and reopening of each match, by match id; Grants the history of each company, by company id.
	Events map[int64][]MatchEvent
	Grants map[int64][]Grant
	Rules  ArchiveRules
}

// ArchiveRules is the pruned rules triple; each file keeps the format of the original, so the same loader reads it.
type ArchiveRules struct {
	Vanilla, BattleCompanies, Glossary json.RawMessage
}

// File returns the pruned file of a kind.
func (r ArchiveRules) File(kind RulesetKind) []byte {
	switch kind {
	case KindVanilla:
		return r.Vanilla
	case KindBattleCompanies:
		return r.BattleCompanies
	case KindGlossary:
		return r.Glossary
	}
	return nil
}

// Pack is the document as it is stored: JSON, gzipped.
func (a Archive) Pack() ([]byte, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if _, err := w.Write(raw); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// UnpackArchive reads a stored document.
func UnpackArchive(packed []byte) (Archive, error) {
	r, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return Archive{}, fmt.Errorf("the archive is not gzip: %w", err)
	}
	raw, err := io.ReadAll(io.LimitReader(r, 256<<20))
	if err != nil {
		return Archive{}, err
	}
	var a Archive
	if err := json.Unmarshal(raw, &a); err != nil {
		return Archive{}, fmt.Errorf("the archive is not valid: %w", err)
	}
	if a.Version > ArchiveFormat {
		return Archive{}, fmt.Errorf("the archive is of format %d; this version reads up to %d", a.Version, ArchiveFormat)
	}
	return a, nil
}

// ArchiveUse is what the campaign used of the rules: the names that decide what is kept.
type ArchiveUse struct {
	Units     map[string]bool // vanilla/BC unit names (NameKey)
	Gear      map[string]bool
	Rules     map[string]bool // special rules, parameters stripped
	Paths     map[string]bool
	Powers    map[string]bool
	Companies map[string]bool // Battle Companies company names
	Scenarios map[string]bool
	Terms     map[string]bool // every name the glossary may be asked about
	Keywords  map[string]bool // vanilla keywords the kept Battle Companies rules name (auras, targets)
}

var parameterTail = regexp.MustCompile(`\s*\(.*\)\s*$`)

func baseName(s string) string { return NameKey(parameterTail.ReplaceAllString(s, "")) }

func newUse() *ArchiveUse {
	return &ArchiveUse{Units: map[string]bool{}, Gear: map[string]bool{}, Rules: map[string]bool{}, Paths: map[string]bool{},
		Powers: map[string]bool{}, Companies: map[string]bool{}, Scenarios: map[string]bool{}, Terms: map[string]bool{}, Keywords: map[string]bool{}}
}

func (u *ArchiveUse) add(set map[string]bool, names ...string) {
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			continue
		}
		set[baseName(n)] = true
		u.Terms[baseName(n)] = true
	}
}

// UseOf works out what the companies and matches use, by building every company with the full rule set.
func UseOf(rs *RuleSet, settings CampaignSettings, states []CompanyState, matches []Match) *ArchiveUse {
	use := newUse()
	addUnit := func(cu CompanyUnit) {
		use.add(use.Units, cu.Type, cu.PromotedFrom)
		for _, e := range cu.Equipment {
			use.add(use.Gear, e.Name)
			use.add(use.Gear, e.Spellings...)
			use.add(use.Rules, e.MountRules...)
		}
		for _, r := range cu.Rules {
			use.add(use.Rules, r.Name, r.From)
		}
		if cu.Path != nil {
			use.add(use.Paths, cu.Path.Name)
			for _, r := range cu.Path.Rules {
				use.add(use.Rules, r.Name)
			}
			for _, r := range cu.Path.Powers {
				use.add(use.Powers, r.Name)
			}
			for _, h := range cu.Path.HeroicActions {
				use.add(use.Rules, h.Name)
			}
		}
		for _, in := range cu.Injuries {
			use.add(use.Rules, in.Name)
		}
		if cu.Creature != nil {
			use.add(use.Gear, cu.Creature.Name)
			use.add(use.Rules, cu.Creature.Rules...)
		}
		for _, k := range cu.Keywords.Model {
			use.add(use.Terms, k.Name, k.Concept, k.Name+" keyword")
		}
	}
	for _, st := range states {
		use.add(use.Companies, st.Company.Faction)
		d := BuildDetail(rs, settings, st)
		for _, u := range d.Roster {
			addUnit(u)
		}
		for _, f := range d.Cemetery {
			addUnit(f.Unit)
		}
		for _, p := range st.Pack {
			use.add(use.Gear, p)
		}
		for _, u := range st.Units {
			use.add(use.Units, u.Base)
			use.add(use.Gear, u.Gear...)
			use.add(use.Gear, u.Bought...)
			use.add(use.Gear, u.Dropped...)
			use.add(use.Paths, u.Path)
			use.add(use.Rules, u.Actions...)
			use.add(use.Rules, u.BoughtActions...)
			for _, sp := range u.Spells {
				use.add(use.Powers, sp.Name)
			}
		}
	}
	for _, m := range matches {
		if m.Scenario != nil {
			use.add(use.Scenarios, m.Scenario.Name)
		}
	}
	return use
}

// ArchiveSource is what is read of a finished campaign to make its archive.
type ArchiveSource struct {
	CampaignID        int64
	Name, Description string
	Round, Rounds     int
	EndedAt           time.Time
	Settings          CampaignSettings
	Files             []RulesetInfo
	Raw               RuleFiles // the stored files, to prune
	States            []CompanyState
	Matches           []Match
	Events            map[int64][]MatchEvent
	Grants            map[int64][]Grant
}

// BuildArchive makes the archive of an ended campaign: the states, the matches and the pruned files. States are stripped of what only an
// open campaign needs (a recruit in progress, aftermaths and perks to finish). rs is the full rule set of the campaign.
func BuildArchive(rs *RuleSet, src ArchiveSource) (Archive, error) {
	use := UseOf(rs, src.Settings, src.States, src.Matches)
	if err := closeUse(rs, src.Raw.BattleCompanies, use); err != nil {
		return Archive{}, err
	}
	a := Archive{Version: ArchiveFormat, CampaignID: src.CampaignID, Name: src.Name, Description: src.Description, Round: src.Round, Rounds: src.Rounds,
		EndedAt: src.EndedAt, Settings: src.Settings, Files: src.Files, Matches: src.Matches, Events: src.Events, Grants: src.Grants}
	for _, st := range src.States {
		st = st.Clone()
		st.Recruit, st.Aftermaths, st.Perks, st.PerksUsed, st.OpenMatch = RecruitProgress{}, nil, nil, nil, nil
		a.Companies = append(a.Companies, st)
	}
	var err error
	if a.Rules.Vanilla, err = pruneVanilla(rs, src.Raw.Vanilla, use); err != nil {
		return a, fmt.Errorf("vanilla: %w", err)
	}
	if a.Rules.BattleCompanies, err = pruneBC(src.Raw.BattleCompanies, use); err != nil {
		return a, fmt.Errorf("battle companies: %w", err)
	}
	if a.Rules.Glossary, err = pruneGlossary(rs, src.Raw.Glossary, use); err != nil {
		return a, fmt.Errorf("glossary: %w", err)
	}
	return a, nil
}

// closeUse adds what the loader insists on once the kept pieces are chosen: the units a kept company's charts and advancements name (a company
// stays whole), the rules a kept path's chart and a kept item's abilities name, and the vanilla keywords the kept rules name.
func closeUse(rs *RuleSet, rawBC []byte, use *ArchiveUse) error {
	for _, p := range rs.BC.Profiles() {
		if !use.Companies[NameKey(p.Name)] {
			continue
		}
		for _, m := range p.mentionedUnits() {
			use.add(use.Units, m.unit)
			if vu, alias, ok := rs.ResolveUnit(p, m.unit); ok {
				use.add(use.Units, vu.Name)
				if alias != nil {
					use.add(use.Units, alias.Unit)
				}
			}
		}
	}
	doc, err := decode(rawBC)
	if err != nil {
		return err
	}
	var collect func(v any, key string, into func(string))
	collect = func(v any, key string, into func(string)) {
		switch x := v.(type) {
		case obj:
			for k, e := range x {
				collect(e, k, into)
			}
		case []any:
			for _, e := range x {
				collect(e, key, into)
			}
		case string:
			if key == "rule" || key == "withRule" {
				into(x)
			}
		}
	}
	for _, p := range jlist(doc, "paths") {
		if use.Paths[baseName(nameOf(p))] {
			collect(p, "", func(r string) { use.add(use.Rules, r) })
		}
	}
	for _, g := range jlist(doc, "gear") {
		if use.Gear[baseName(nameOf(g))] {
			collect(g, "", func(r string) { use.add(use.Rules, r) })
		}
	}
	for _, r := range jlist(doc, "rules") {
		if !use.Rules[baseName(nameOf(r))] {
			continue
		}
		m := r.(obj)
		for _, a := range jlist(m, "auras") {
			for _, k := range jlist(a.(obj), "keywords") {
				use.Keywords[baseName(str(k))] = true
			}
		}
		if t, ok := m["target"].(obj); ok {
			for _, k := range jlist(t, "keywords") {
				use.Keywords[baseName(str(k))] = true
			}
		}
	}
	return nil
}

// LoadPruned loads the pruned triple with the app's loader: the "mini rule set" of the snapshot.
func (a Archive) LoadPruned() (*RuleSet, error) {
	return LoadRuleSet(a.Rules.Vanilla, a.Rules.BattleCompanies, a.Rules.Glossary, slog.New(slog.DiscardHandler))
}

// ---------------------------------------------------------------------------------------------------------------------
// Generic JSON helpers

type obj = map[string]any

func decode(raw []byte) (obj, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var o obj
	return o, d.Decode(&o)
}

func encode(o obj) (json.RawMessage, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	err := e.Encode(o)
	return bytes.TrimSpace(b.Bytes()), err
}

func jlist(o obj, key string) []any {
	l, _ := o[key].([]any)
	return l
}

func str(v any) string { s, _ := v.(string); return s }

func nameOf(v any) string {
	if m, ok := v.(obj); ok {
		return str(m["name"])
	}
	return ""
}

// keep filters o[key] (a list of objects with a name) to the entries whose name satisfies ok.
func keep(o obj, key string, ok func(name string) bool) {
	l := jlist(o, key)
	if l == nil {
		return
	}
	out := make([]any, 0, len(l))
	for _, e := range l {
		if ok(nameOf(e)) {
			out = append(out, e)
		}
	}
	o[key] = out
}

// ---------------------------------------------------------------------------------------------------------------------
// Vanilla

func pruneVanilla(rs *RuleSet, raw []byte, use *ArchiveUse) (json.RawMessage, error) {
	o, err := decode(raw)
	if err != nil {
		return nil, err
	}
	data, _ := o["data"].(obj)
	// units: the ones the companies are made of, and the members of a bundle
	units := map[string]bool{}
	for n := range use.Units {
		units[n] = true
		if vu, ok := rs.Vanilla.Unit(n); ok {
			for _, m := range vu.Includes {
				units[NameKey(m)] = true
			}
		}
	}
	// the vanilla unit behind a company's own name for it (an alias) is used too
	for _, p := range rs.BC.Profiles() {
		if !use.Companies[NameKey(p.Name)] {
			continue
		}
		for _, a := range p.Aliases {
			if units[NameKey(a.Label)] {
				units[NameKey(a.Unit)] = true
			}
		}
	}
	// a keyword a kept rule names (an aura against Orcs) must exist in the vanilla file, and the loader finds keywords on the units: one unit
	// that has it stays for each keyword no kept unit has
	has := func(m obj, kw string) bool {
		for _, k := range []string{"race", "faction", "unitType"} {
			for _, v := range jlist(m, k) {
				if baseName(str(v)) == kw {
					return true
				}
			}
		}
		return false
	}
	for kw := range use.Keywords {
		covered := false
		for _, key := range []string{"heroes", "warriors"} {
			for _, e := range jlist(data, key) {
				if m := e.(obj); units[NameKey(nameOf(m))] && has(m, kw) {
					covered = true
				}
			}
		}
		for _, key := range []string{"warriors", "heroes"} {
			for _, e := range jlist(data, key) {
				if m := e.(obj); !covered && has(m, kw) {
					units[NameKey(nameOf(m))] = true
					covered = true
				}
			}
		}
	}
	factions := map[string]bool{}
	gear := map[string]bool{}
	for n := range use.Gear {
		gear[n] = true
	}
	rules := map[string]bool{}
	for n := range use.Rules {
		rules[n] = true
	}
	powers := map[string]bool{}
	for n := range use.Powers {
		powers[n] = true
	}
	for _, key := range []string{"heroes", "warriors"} {
		keep(data, key, func(n string) bool { return units[NameKey(n)] })
		for _, e := range jlist(data, key) {
			m := e.(obj)
			for _, f := range jlist(m, "factions") {
				factions[NameKey(str(f))+NameKey(nameOf(f))] = true
				factions[NameKey(nameOf(f))] = true
				factions[NameKey(str(f))] = true
			}
			for _, k := range []string{"wargear", "options", "mountOptions"} {
				for _, g := range jlist(m, k) {
					gear[baseName(nameOf(g)+str(g))] = true
				}
			}
			for _, r := range jlist(m, "specialRules") {
				rules[baseName(str(r)+nameOf(r))] = true
			}
			for _, r := range jlist(m, "heroicActions") {
				rules[baseName(str(r)+nameOf(r))] = true
			}
			for _, p := range jlist(m, "magicalPowers") {
				powers[baseName(nameOf(p)+str(p))] = true
			}
		}
	}
	keep(data, "gear", func(n string) bool { return gear[baseName(n)] })
	keep(data, "keywords", func(n string) bool { return rules[baseName(n)] || use.Keywords[baseName(n)] })
	keep(data, "magicalPowers", func(n string) bool { return powers[baseName(n)] })
	keep(data, "factions", func(n string) bool { return factions[NameKey(n)] })
	for _, f := range jlist(data, "factions") {
		delete(f.(obj), "armyBonuses")
		delete(f.(obj), "additionalRules")
	}
	delete(data, "armyBonuses")
	// the vanilla scenarios and their scoring are the biggest part of the rest and the campaign's own scenarios are in the BC file
	delete(o, "scenarios")
	delete(o, "scenarioScoring")
	return encode(o)
}

// ---------------------------------------------------------------------------------------------------------------------
// Battle Companies

func pruneBC(raw []byte, use *ArchiveUse) (json.RawMessage, error) {
	o, err := decode(raw)
	if err != nil {
		return nil, err
	}
	keep(o, "rules", func(n string) bool { return use.Rules[baseName(n)] })
	keep(o, "gear", func(n string) bool { return use.Gear[baseName(n)] })
	keep(o, "paths", func(n string) bool { return use.Paths[baseName(n)] })
	// no match is rolled again: the selection tables go (the loader allows that for an "archive" file) and only the scenarios played stay
	scenarios := map[string]bool{}
	for n := range use.Scenarios {
		scenarios[n] = true
	}
	o["archive"] = true
	if c, ok := o["campaign"].(obj); ok {
		if sel, ok := c["scenarioSelection"].(obj); ok {
			sel["tables"] = []any{}
		}
	}
	keep(o, "scenarios", func(n string) bool { return scenarios[baseName(n)] })
	keep(o, "units", func(n string) bool { return use.Units[baseName(n)] })
	keep(o, "companies", func(n string) bool { return use.Companies[baseName(n)] })
	pruneBCReferences(o, use, scenarios)
	return encode(o)
}

// pruneBCReferences drops, from what is left of the file, the references to what was pruned: a scenario table row, an aftermath effect of a
// scenario no match played, a hero upgrade whose rule nobody has. The loader checks every name, so a dangling one makes the file invalid.
func pruneBCReferences(o obj, use *ArchiveUse, scenarios map[string]bool) {
	scenario := func(n string) bool { return scenarios[baseName(n)] }
	rule := func(n string) bool { return use.Rules[baseName(n)] }
	if c, ok := o["campaign"].(obj); ok {
		dropWhere(c, "aftermathEffects", func(e obj) bool { return str(e["scenario"]) != "" && !scenario(str(e["scenario"])) })
		if bp, ok := c["breakPoint"].(obj); ok {
			dropWhere(bp, "scenarioExceptions", func(e obj) bool {
				return !scenario(str(e["scenario"])) || (str(e["rule"]) != "" && !rule(str(e["rule"])))
			})
		}
		if cl, ok := c["characteristicLimits"].(obj); ok {
			if peak, ok := cl["peak"].(obj); ok {
				if w, ok := peak["willMaxOverrides"].(obj); ok {
					for k := range w {
						if !use.Paths[baseName(k)] {
							delete(w, k)
						}
					}
				}
			}
		}
	}
	if v, ok := o["vanilla"].(obj); ok {
		delete(v, "pins")
	}
	for _, g := range jlist(o, "gear") {
		gm := g.(obj)
		filterStrings(gm, "related", func(n string) bool { return use.Gear[baseName(n)] })
	}
	for _, c := range jlist(o, "companies") {
		cm := c.(obj)
		dropWhere(cm, "heroUpgrades", func(e obj) bool { return !rule(str(e["name"])) })
		dropWhere(cm, "substitutions", func(e obj) bool {
			t, _ := e["target"].(obj)
			return (str(t["withRule"]) != "" && !rule(str(t["withRule"]))) || (str(e["rule"]) != "" && !rule(str(e["rule"])))
		})
		for _, g := range jlist(cm, "gear") {
			filterStrings(g.(obj), "grants", rule)
		}
		for _, u := range jlist(cm, "units") {
			um := u.(obj)
			dropWhere(um, "includes", func(e obj) bool { return !use.Units[baseName(str(e["name"]))] })
		}
	}
}

func dropWhere(o obj, key string, drop func(obj) bool) {
	l := jlist(o, key)
	if l == nil {
		return
	}
	out := make([]any, 0, len(l))
	for _, e := range l {
		if m, ok := e.(obj); ok && drop(m) {
			continue
		}
		out = append(out, e)
	}
	o[key] = out
}

func filterStrings(o obj, key string, ok func(string) bool) {
	l := jlist(o, key)
	if l == nil {
		return
	}
	out := make([]any, 0, len(l))
	for _, e := range l {
		if ok(str(e)) {
			out = append(out, e)
		}
	}
	o[key] = out
}

// ---------------------------------------------------------------------------------------------------------------------
// Glossary

// pruneGlossary keeps the terms the campaign named and, of each, the terms its definition links to or lists under "see also": one level,
// never the terms of those.
func pruneGlossary(rs *RuleSet, raw []byte, use *ArchiveUse) (json.RawMessage, error) {
	o, err := decode(raw)
	if err != nil {
		return nil, err
	}
	g := rs.Glossary
	first := map[string]bool{} // term ids named directly
	for n := range use.Terms {
		if t, ok := g.Find(n); ok {
			first[t.ID] = true
		}
	}
	for _, t := range g.Terms { // "Lothlórien keyword": the text of a keyword chip, found by the keyword's own spelling
		if n, ok := strings.CutSuffix(t.Name, " keyword"); ok && use.Terms[baseName(n)] {
			first[t.ID] = true
		}
	}
	keepID := map[string]bool{}
	for id := range first {
		keepID[id] = true
		t, _ := g.Term(id)
		for _, para := range t.Paragraphs {
			for _, s := range para {
				if s.TermID != "" {
					keepID[s.TermID] = true
				}
			}
		}
		for _, r := range t.See {
			keepID[r.ID] = true
		}
	}
	names := map[string]bool{}
	for id := range keepID {
		if t, ok := g.Term(id); ok {
			names[t.Name] = true
		}
	}
	keep(o, "terms", func(n string) bool { return names[n] })
	return encode(o)
}

// Compact is the document as JSON.
func (a Archive) Compact() ([]byte, error) { return json.Marshal(a) }

var _ = slices.Contains[[]string]
