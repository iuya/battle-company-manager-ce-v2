package core

import (
	"encoding/json"
	"log/slog"
	"sort"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// ErrRulesetNotPlayable says a Battle Companies file of schemaVersion 2 is valid but this build cannot play it yet: the consumers
// still read the old format (docs/definitions-lean-design.md, slices 2 to 9). It is returned instead of storing or loading the file.
var ErrRulesetNotPlayable error = notPlayableError{}

// notPlayableError is an invalid-ruleset error (the upload pages answer it with 422 and its text) with its own identity.
type notPlayableError struct{}

func (notPlayableError) Error() string {
	return "schemaVersion 2 files cannot be played by this build yet"
}

func (notPlayableError) Is(target error) bool { return target == ErrInvalidRuleset }

// acceptV2 is the one gate slice 9 flips: while false, a v2 file that validates is refused with ErrRulesetNotPlayable.
const acceptV2 = false

// DecodeBattleCompanies decodes and validates a schemaVersion 2 Battle Companies file against the vanilla file and the glossary it
// will be played with. file names the file in the report. The document is nil when the bytes are unusable or of another version.
func DecodeBattleCompanies(file string, bc []byte, vanilla *VanillaBook, glossary Glossary) (*lean.Document, lean.Report) {
	doc, _, rep := lean.Decode(file, bc, newVanillaLower(vanilla, glossary))
	return doc, rep
}

// loadV2 builds the rule set of a schemaVersion 2 file: the whole report as a *RulesetReport when the file has errors, else
// ErrRulesetNotPlayable until acceptV2 is flipped (slice 9). The consumers do not read Doc yet, so a playable set is never built.
func loadV2(vanilla, bc, glossary []byte, log *slog.Logger) (*RuleSet, error) {
	vb, err := ParseVanillaWith(vanilla, nil, log)
	if err != nil {
		return nil, err
	}
	hash, err := RulesetHash(glossary)
	if err != nil {
		return nil, err
	}
	gl, err := BuildGlossary(RulesetInfo{Kind: KindGlossary, Hash: hash}, glossary)
	if err != nil {
		return nil, err
	}
	doc, rep := DecodeBattleCompanies(KindBattleCompanies.fileName(), bc, vb, gl)
	if !rep.OK() {
		return nil, newRulesetReport(rep)
	}
	if !acceptV2 {
		return nil, ErrRulesetNotPlayable
	}
	return &RuleSet{Vanilla: vb, Glossary: gl, Doc: doc}, nil
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

// schemaVersionOf reads the schemaVersion of a Battle Companies file: 1 when it has none (the implicit old format), -1 when it holds
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

// isLeanFile reports a Battle Companies file that is not the old format (schemaVersion 2, or something the v2 decoder will explain).
func isLeanFile(bc []byte) bool {
	v := schemaVersionOf(bc)
	return v != 0 && v != 1
}

// CheckBattleCompaniesV2 decodes a schemaVersion 2 file against vanilla and glossary files given as bytes: the check behind
// `make check-ruleset` (cmd/rulecheck). The report holds every issue; the error is non-nil, wraps ErrInvalidRuleset and is the
// report when the file has errors. A file of the old format is an error naming the old validator.
func CheckBattleCompaniesV2(file string, bc, vanilla, glossary []byte) (RulesetReport, error) {
	if schemaVersionOf(bc) == 1 { // 0 is bytes that are not an object: the lean decoder explains why
		msg := "this is a file of the old format (no schemaVersion, or 1); check it with python3 tools/validate_definitions.py"
		rep := &RulesetReport{issues: []RulesetIssue{{File: file, Path: "schemaVersion", Message: msg}}, summary: msg}
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
