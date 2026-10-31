package core

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Unit references in texts. A campaign's glossary (RuleSet.GlossaryWithUnits) knows the base units of its rule set, so a text that
// names one ("Ranger of the North") gets a UnitID segment next to the TermID ones: the page links it to the unit's card in the
// campaign glossary and shows BaseUnit.TipText in a tooltip. Matching is the term linker's: whole words, the longest name first
// ("Erebor Dwarf Warrior" beats "Dwarf Warrior"), a name only as the file spells it when it is one word ("Ruffian"), each unit once
// per text. A name that is also a glossary term (Fell Warg, the wargear) stays the term.

// firstWord is the key a phrase is filed under in linker.byFirst: its first run of word characters, or "" when it starts with
// something else (such a phrase is never looked for).
func firstWord(p string) string {
	if p == "" {
		return ""
	}
	if r, _ := utf8.DecodeRuneInString(p); !isWordRune(r) {
		return ""
	}
	if k := strings.IndexFunc(p, func(r rune) bool { return !isWordRune(r) }); k >= 0 {
		return p[:k]
	}
	return p
}

// withUnits is a copy of the linker that also finds the names of units. A unit takes the anchor of its first card (the one a
// plain name gets: UnitAnchor(name)); a name shorter than minLinkLen, or one the linker already has as a term, is left out.
func (l *linker) withUnits(units []BaseUnit) *linker {
	out := &linker{byFirst: map[string][]string{}, byPhrase: map[string]string{}, exact: map[string]string{}, units: map[string]string{}}
	if l != nil {
		for k, v := range l.byFirst {
			out.byFirst[k] = slices.Clone(v)
		}
		for k, v := range l.byPhrase {
			out.byPhrase[k] = v
		}
		for k, v := range l.exact {
			out.exact[k] = v
		}
	}
	touched := map[string]bool{}
	for _, u := range units {
		spelled := strings.TrimSpace(u.Name)
		key := strings.ToLower(spelled)
		if _, term := out.byPhrase[key]; term || utf8.RuneCountInString(key) < minLinkLen || u.ID != UnitAnchor(u.Name) {
			continue // a term of that name, a name too short to trust, or a later card of a name two companies give to different profiles
		}
		if _, dup := out.units[key]; dup {
			continue
		}
		first := firstWord(key)
		if first == "" {
			continue
		}
		out.units[key] = u.ID
		if !strings.Contains(spelled, " ") {
			out.exact[key] = spelled
		}
		out.byFirst[first] = append(out.byFirst[first], key)
		touched[first] = true
	}
	for first := range touched {
		slices.SortStableFunc(out.byFirst[first], func(a, b string) int { return len(b) - len(a) }) // longest first
	}
	return out
}

// withUnits is the glossary with the base units of a rule set known to its links: the texts of its terms (definitions and path
// charts) are cut again so the unit names in them link too. The terms' own links stay as they were, apart from a name a longer unit
// name covers ("Erebor Dwarf Warrior" is one unit, not the term Dwarf and a Warrior). Built once per rule set.
func (g Glossary) withUnits(units []BaseUnit) Glossary {
	if len(units) == 0 {
		return g
	}
	g.Units = units
	g.unitByID = make(map[string]int, len(units))
	for i, u := range units {
		g.unitByID[u.ID] = i
	}
	g.link = g.link.withUnits(units)
	terms := make([]GlossaryTerm, len(g.Terms))
	for i, t := range g.Terms {
		linked := map[string]bool{t.ID: true}
		paras := make([][]Segment, len(t.Paragraphs))
		for j, para := range t.Paragraphs {
			paras[j] = g.link.segments(joinSegments(para), linked)
		}
		t.Paragraphs = paras
		if t.Path != nil {
			p := *t.Path
			p.Rows = make([]PathTableRow, len(t.Path.Rows))
			for j, row := range t.Path.Rows {
				p.Rows[j] = PathTableRow{Roll: row.Roll, Segments: g.link.segments(joinSegments(row.Segments), map[string]bool{t.ID: true})}
			}
			t.Path = &p
		}
		terms[i] = t
	}
	g.Terms = terms
	return g
}

func joinSegments(segs []Segment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

// UnitCard is the base unit with this anchor (UnitAnchor), nil when the glossary has no such card.
func (g Glossary) UnitCard(anchor string) *BaseUnit {
	if anchor == "" {
		return nil
	}
	if g.unitByID == nil { // units set by hand, not by RuleSet.GlossaryWithUnits
		if i := slices.IndexFunc(g.Units, func(u BaseUnit) bool { return u.ID == anchor }); i >= 0 {
			return &g.Units[i]
		}
		return nil
	}
	if i, ok := g.unitByID[anchor]; ok {
		return &g.Units[i]
	}
	return nil
}

// TipMeta is the second line of a unit's tooltip: its types and points ("Infantry · Hero · 60 pts").
func (u BaseUnit) TipMeta() string {
	parts := slices.Clone(u.Preview.Types)
	if u.Preview.Points > 0 {
		parts = append(parts, fmt.Sprintf("%d pts", u.Preview.Points))
	}
	return strings.Join(parts, " · ")
}

// TipText is a unit's tooltip: its profile on one line, then its wargear, heroic actions and special rules, a line each. The
// unit's card in the glossary has the rest (options, the companies that field it).
func (u BaseUnit) TipText() string {
	var lines []string
	var stats []string
	for _, a := range u.Preview.Stats {
		name := a.Abbr
		if a.HeroOnly {
			name = a.Name // Might, Will and Fate: their initials are the Move, Wounds and Fight of the line above
		}
		stats = append(stats, fmt.Sprintf("%s %d%s", name, a.Value, a.Trail))
	}
	if len(stats) > 0 {
		lines = append(lines, strings.Join(stats, " · "))
	}
	for _, l := range []struct {
		label string
		items []string
	}{{"Made of", u.Includes}, {"Wargear", u.Preview.Wargear}, {"Heroic actions", u.Preview.HeroicActions}, {"Special rules", u.Preview.Rules}} {
		if len(l.items) > 0 {
			lines = append(lines, l.label+": "+strings.Join(l.items, ", "))
		}
	}
	return strings.Join(lines, "\n")
}
