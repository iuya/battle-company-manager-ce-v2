package core

import (
	"encoding/json"
	"log/slog"
	"slices"
	"sort"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// DecodeBattleCompanies decodes and validates a schemaVersion 2 Battle Companies file against the vanilla file and the glossary it
// will be played with. file names the file in the report. The document is nil when the bytes are unusable or of another version.
func DecodeBattleCompanies(file string, bc []byte, vanilla *VanillaBook, glossary Glossary) (*lean.Document, lean.Report) {
	lower := newVanillaLower(vanilla, glossary)
	doc, ix, rep := lean.Decode(file, bc, lower)
	if doc != nil {
		checkNewUnits(file, doc, ix, lower, &rep)
	}
	return doc, rep
}

// loadV2 builds the rule set of a Battle Companies file: the whole report as a *RulesetReport when the file has errors (a file of the old
// format, with no schemaVersion or 1, is refused with the one issue that says so). The vanilla file
// is read twice: once to check the names of the document against, once with the duplicates the document pins.
func loadV2(vanilla, bc, glossary []byte, log *slog.Logger) (*RuleSet, error) {
	vb, err := ParseVanillaWith(vanilla, nil, nil)
	if err != nil {
		return nil, err
	}
	hash, err := RulesetHash(glossary)
	if err != nil {
		return nil, err
	}
	info := RulesetInfo{Kind: KindGlossary, Hash: hash}
	gl, err := BuildGlossary(info, glossary)
	if err != nil {
		return nil, err
	}
	if rep, old := oldFormatReport(KindBattleCompanies.fileName(), bc); old {
		return nil, rep
	}
	doc, rep := DecodeBattleCompanies(KindBattleCompanies.fileName(), bc, vb, gl)
	if !rep.OK() {
		return nil, newRulesetReport(rep)
	}
	if vb, err = parseVanilla(vanilla, LeanPins(doc), log, true); err != nil {
		return nil, err
	}
	ApplyLeanFillIns(vb, doc.Vanilla)
	book, err := LeanBook(doc, vb, gl)
	if err != nil {
		return nil, err
	}
	if gl, err = BuildGlossaryWith(info, glossary, LeanGlossaryExtras(vanilla, doc, vb, gl)); err != nil {
		return nil, err
	}
	return &RuleSet{Vanilla: vb, BC: book, Glossary: gl, Doc: doc}, nil
}

// LeanPins are the reviewed duplicates of the vanilla file a document pins: the occurrence that wins, where it is not the first.
func LeanPins(d *lean.Document) []VanillaAmbiguity {
	if d.Vanilla == nil {
		return nil
	}
	kinds := map[string]string{"hero": "unit", "warrior": "unit", "unit": "unit", "item": "gear", "mount": "gear", "rule": "keyword"}
	var out []VanillaAmbiguity
	for _, p := range d.Vanilla.Pins {
		out = append(out, VanillaAmbiguity{Kind: kinds[p.Kind], Name: p.Name, Status: "pinned", Occurrence: p.Occurrence})
	}
	return out
}

// vanillaLower implements lean.Lower over the vanilla book and the glossary. Names compare by lean.NameKey, never fuzzily.
type vanillaLower struct {
	book    *VanillaBook
	names   map[lean.Kind]map[string]string // kind -> NameKey -> spelling
	sorted  map[lean.Kind][]string
	options map[string][]string // NameKey of a unit -> option names
}

func newVanillaLower(book *VanillaBook, gl Glossary) *vanillaLower {
	l := &vanillaLower{book: book, names: map[lean.Kind]map[string]string{}, sorted: map[lean.Kind][]string{}, options: map[string][]string{}}
	add := func(kind lean.Kind, name string) {
		key := lean.NameKey(name)
		if key == "" {
			return
		}
		if l.names[kind] == nil {
			l.names[kind] = map[string]string{}
		}
		if cur, ok := l.names[kind][key]; !ok || name < cur {
			l.names[kind][key] = name
		}
	}
	if book != nil {
		for name, u := range book.units {
			add(lean.KindUnit, name)
			var opts []string
			for _, o := range u.Options {
				opts = append(opts, o.Name)
			}
			l.options[lean.NameKey(name)] = opts
			for _, w := range u.Race {
				add(lean.KindRace, w)
				add(lean.KindKeyword, w)
			}
			for _, w := range append(append([]string(nil), u.Faction...), u.UnitTypes...) {
				add(lean.KindKeyword, w)
			}
		}
		for _, g := range book.gear {
			add(lean.KindGear, g.Name)
			if g.Type == "mount" {
				add(lean.KindMount, g.Name)
			}
			for _, w := range g.Race {
				add(lean.KindRace, w)
			}
			for _, w := range slices.Concat(g.Race, g.Faction, g.UnitTypes) { // a mount's words are keywords of the model that rides it ("Mount")
				if w != "See Rider" { // the faction of a mount that is its rider's
					add(lean.KindKeyword, w)
				}
			}
		}
		for name := range book.keywords {
			add(lean.KindRule, name)
		}
		for name := range book.powers {
			add(lean.KindPower, name)
		}
	}
	for _, t := range gl.Terms {
		add(lean.KindRule, t.Name)
		for _, a := range t.Aliases {
			add(lean.KindRule, a)
		}
	}
	for kind, m := range l.names {
		for _, name := range m {
			l.sorted[kind] = append(l.sorted[kind], name)
		}
		sort.Strings(l.sorted[kind])
	}
	return l
}

func (l *vanillaLower) Has(kind lean.Kind, name string) bool {
	_, ok := l.names[kind][lean.NameKey(name)]
	return ok
}

func (l *vanillaLower) Names(kind lean.Kind) []string { return l.sorted[kind] }

func (l *vanillaLower) Duplicates(kind lean.Kind, name string) int {
	if l.book == nil {
		return 0
	}
	switch kind {
	case lean.KindUnit:
		return l.book.Occurrences("unit", name)
	case lean.KindGear, lean.KindMount:
		return l.book.Occurrences("gear", name)
	case lean.KindRule:
		return l.book.Occurrences("keyword", name)
	case lean.KindPower:
		return l.book.Occurrences("power", name)
	}
	return 0
}

func (l *vanillaLower) Options(unit string) []string { return l.options[lean.NameKey(unit)] }

// schemaVersionOf reads the schemaVersion of a Battle Companies file: 1 when it has none (the implicit old format, which is not read), -1 when it holds
// something that is not a whole number (the v2 decoder reports it), 0 when the bytes are not a JSON object.
func schemaVersionOf(bc []byte) int {
	var head map[string]json.RawMessage
	if err := json.Unmarshal(bc, &head); err != nil {
		return 0
	}
	raw, ok := head["schemaVersion"]
	if !ok {
		return 1
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		return -1
	}
	return v
}

// oldFormatReport is the one issue a Battle Companies file of the old format gets: no schemaVersion, or 1. The old format is not read.
func oldFormatReport(file string, bc []byte) (*RulesetReport, bool) {
	if schemaVersionOf(bc) != 1 { // 0 is bytes that are not an object: the lean decoder explains why
		return nil, false
	}
	msg := "schemaVersion 1 is not supported: this is a Battle Companies file of the old format (no schemaVersion, or 1) and this app reads schemaVersion 2; regenerate it with the definitions generator (docs/definitions.md)"
	return &RulesetReport{issues: []RulesetIssue{{File: file, Path: "schemaVersion", Message: msg}}, summary: msg}, true
}

// CheckBattleCompaniesV2 decodes a Battle Companies file against vanilla and glossary files given as bytes: the check behind
// `make check-ruleset` (cmd/rulecheck). The report holds every issue; the error is non-nil, wraps ErrInvalidRuleset and is the
// report when the file has errors. A file of the old format is an error that says so.
func CheckBattleCompaniesV2(file string, bc, vanilla, glossary []byte) (RulesetReport, error) {
	if rep, old := oldFormatReport(file, bc); old {
		return *rep, rep
	}
	book, err := ParseVanilla(vanilla)
	if err != nil {
		return RulesetReport{}, err
	}
	hash, err := RulesetHash(glossary)
	if err != nil {
		return RulesetReport{}, err
	}
	gl, err := BuildGlossary(RulesetInfo{Kind: KindGlossary, Hash: hash}, glossary)
	if err != nil {
		return RulesetReport{}, err
	}
	_, lrep := DecodeBattleCompanies(file, bc, book, gl)
	rep := newRulesetReport(lrep)
	if rep.Errors() > 0 {
		return *rep, rep
	}
	return *rep, nil
}
