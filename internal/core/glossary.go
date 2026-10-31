package core

import (
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Glossary is a rules glossary ready to display: its terms with the links between them already made. The JSON file is
// never changed; BuildGlossary derives the links when the file is loaded.
type Glossary struct {
	Info  RulesetInfo
	Terms []GlossaryTerm
	link  *linker
	byID  map[string]int

	// Units are the base unit cards of a campaign's glossary (RuleSet.GlossaryWithUnits); empty for a glossary file on its own.
	Units []BaseUnit

	// ToWound is the To Wound Chart of the file (reference.toWound); nil when the file has none or a broken one (the chart is then not offered).
	ToWound *ToWoundChart
}

// Term finds a term by its id.
func (g Glossary) Term(id string) (GlossaryTerm, bool) {
	if i, ok := g.byID[id]; ok {
		return g.Terms[i], true
	}
	return GlossaryTerm{}, false
}

// Categories lists the categories of the terms, sorted.
func (g Glossary) Categories() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range g.Terms {
		if t.Category != "" && !seen[t.Category] {
			seen[t.Category] = true
			out = append(out, t.Category)
		}
	}
	slices.Sort(out)
	return out
}

// Find looks a term up by its name or one of its aliases, ignoring case (the way a rule names it: "Heroic Strike").
func (g Glossary) Find(name string) (GlossaryTerm, bool) {
	if g.link == nil {
		return GlossaryTerm{}, false
	}
	id, ok := g.link.byPhrase[strings.ToLower(strings.TrimSpace(name))]
	if !ok { // a rule with its parameter ("Hatred (Spirit)") is the term of the rule
		id, ok = g.link.byPhrase[strings.ToLower(strings.TrimSpace(parameter.ReplaceAllString(name, "")))]
	}
	if !ok {
		return GlossaryTerm{}, false
	}
	return g.Term(id)
}

// Link cuts any text (a special rule's description, say) into plain pieces and mentions of glossary terms, with the same
// rules as the definitions themselves: whole words, a single word only when written as the file spells it, each term once.
func (g Glossary) Link(text string) []Segment {
	if g.link == nil {
		return []Segment{{Text: text}}
	}
	return g.link.segments(text, map[string]bool{})
}

// Text is the term's definition as plain text, one paragraph per line break pair.
func (t GlossaryTerm) Text() string {
	var paras []string
	for _, para := range t.Paragraphs {
		var b strings.Builder
		for _, seg := range para {
			b.WriteString(seg.Text)
		}
		paras = append(paras, b.String())
	}
	return strings.Join(paras, "\n\n")
}

// Summary is the term's definition as plain text, cut at a word boundary after about max characters.
//
// A term of a built glossary remembers the answer per length (the glossary never changes), so the tooltips of a page cost one lookup each.
func (t GlossaryTerm) Summary(max int) string {
	if t.summaries == nil {
		return t.summary(max)
	}
	if v, ok := t.summaries.Load(max); ok {
		return v.(string)
	}
	v, _ := t.summaries.LoadOrStore(max, t.summary(max))
	return v.(string)
}

func (t GlossaryTerm) summary(max int) string {
	var b strings.Builder
	for _, para := range t.Paragraphs {
		for _, seg := range para {
			b.WriteString(seg.Text)
		}
		b.WriteByte(' ')
	}
	text := strings.Join(strings.Fields(b.String()), " ")
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	cut := []rune(text)[:max]
	for len(cut) > 0 && cut[len(cut)-1] != ' ' {
		cut = cut[:len(cut)-1]
	}
	return strings.TrimSpace(string(cut)) + "…"
}

// linker finds mentions of glossary terms in text. Phrases are looked up by their first word (longest first), which is much
// faster than one regular expression over hundreds of names.
type linker struct {
	byFirst  map[string][]string // first word, lower-cased -> phrases starting with it, longest first
	byPhrase map[string]string   // lower-cased name or alias -> term id
	// exact holds, for single-word names and aliases, the spelling in the file: a single word is linked only when written
	// exactly so ("Will" is the Will term, "will" is the ordinary word). Multi-word phrases are case insensitive.
	exact map[string]string
}

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// segments cuts para into plain text and links. linked holds the targets already linked in this text (each is linked
// once) and is updated.
func (l *linker) segments(para string, linked map[string]bool) []Segment {
	var segs []Segment
	last := 0
	for i := 0; i < len(para); {
		r, size := utf8.DecodeRuneInString(para[i:])
		prev := rune(0)
		if i > 0 {
			prev, _ = utf8.DecodeLastRuneInString(para[:i])
		}
		if !isWordRune(r) || (i > 0 && isWordRune(prev)) {
			i += size
			continue
		}
		j := i
		for j < len(para) {
			rr, sz := utf8.DecodeRuneInString(para[j:])
			if !isWordRune(rr) {
				break
			}
			j += sz
		}
		matched := 0
		for _, phrase := range l.byFirst[strings.ToLower(para[i:j])] {
			end := i + len(phrase)
			if end > len(para) || !strings.EqualFold(para[i:end], phrase) {
				continue
			}
			if end < len(para) {
				lastR, _ := utf8.DecodeLastRuneInString(para[:end])
				nextR, _ := utf8.DecodeRuneInString(para[end:])
				if isWordRune(lastR) == isWordRune(nextR) {
					continue // not a whole word
				}
			}
			matched = end
			target := l.byPhrase[phrase]
			mention := para[i:end]
			if linked[target] || (!strings.Contains(mention, " ") && l.exact[phrase] != mention) {
				break // a single word is linked only as the file spells it: "will" is most likely just the ordinary word
			}
			linked[target] = true
			if i > last {
				segs = append(segs, Segment{Text: para[last:i]})
			}
			segs = append(segs, Segment{Text: mention, TermID: target})
			last = end
			break
		}
		if matched > 0 {
			i = matched
		} else {
			i = j
		}
	}
	if last < len(para) {
		segs = append(segs, Segment{Text: para[last:]})
	}
	return segs
}

type GlossaryTerm struct {
	ID         string // stable within a glossary, safe in a URL fragment
	Name       string
	Category   string
	Aliases    []string
	Paragraphs [][]Segment // the definition, paragraph by paragraph
	See        []TermRef   // related terms
	Pages      []int       // rulebook pages it comes from
	Path       *PathTable  // for a path: its limits and chart, as tables
	Source     string      // for terms made from a campaign's rules files: the file they come from

	summaries *sync.Map // Summary(max) by max, shared by the copies of the term; nil on a term not made by BuildGlossaryWith
}

// Segment is a piece of a definition: plain text, or (TermID set) a mention of another term to link to.
type Segment struct {
	Text   string
	TermID string
}

type TermRef struct{ ID, Name string }

// PathTable is a path's limits and level-up chart; each result is cut into text and links to other terms.
type PathTable struct {
	Limits   []PathTableLimit
	Rows     []PathTableRow
	PathDice DiceSpec // the dice the chart is rolled with (zero when the file names none)
}

// PathTableLimit is the limit of one characteristic, with the abbreviation of its name linked to its glossary term.
type PathTableLimit struct {
	Stat, Abbr, Limit string
	TermID            string // empty when the glossary has no term for it
}

// PathTableRow is one result of a path's chart.
type PathTableRow struct {
	Roll     int
	Segments []Segment
}

const minLinkLen = 3 // shorter names are too likely to be ordinary words

func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r < utf8.RuneSelf && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// BuildGlossary parses a glossary file and links its terms: every mention of another term (by name or alias, whole
// words, case-insensitive) becomes a link, the first time it appears in a definition; terms never link to themselves.
// The related terms listed in "see" are resolved to references.
func BuildGlossary(info RulesetInfo, content []byte) (Glossary, error) {
	return BuildGlossaryWith(info, content, nil)
}

// BuildGlossaryWith is BuildGlossary plus the terms of a campaign's rules files (GlossaryExtras): they come after the file's
// own terms and are left out when a term of the same name (or alias) exists.
func BuildGlossaryWith(info RulesetInfo, content []byte, extras []ExtraTerm) (Glossary, error) {
	type rawTerm struct {
		Name       string   `json:"name"`
		Category   string   `json:"category"`
		Aliases    []string `json:"aliases"`
		Definition string   `json:"definition"`
		See        []string `json:"see"`
		Sources    []struct {
			RulebookPage int `json:"rulebookPage"`
		} `json:"sources"`
		From string    `json:"-"`
		Path *PathData `json:"-"`
	}
	var doc struct {
		Terms []rawTerm `json:"terms"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return Glossary{}, errors.Join(ErrInvalidRuleset, errors.New("the glossary is not valid JSON"))
	}
	// A name may be defined in several places. Battle Companies wins over everything (it is a game mode built on top of the
	// base rules), then the glossary file, then the vanilla file: a Battle Companies term replaces the file's term of the same
	// name or alias, a vanilla term is left out when the name is taken.
	owner := map[string]int{} // lower-cased name or alias -> index in doc.Terms
	fromFile := len(doc.Terms)
	for i, t := range doc.Terms {
		for _, n := range append([]string{t.Name}, t.Aliases...) {
			owner[strings.ToLower(strings.TrimSpace(n))] = i
		}
	}
	for _, x := range extras {
		names := append([]string{x.Name}, x.Aliases...)
		at := -1
		for _, n := range names {
			if i, ok := owner[strings.ToLower(strings.TrimSpace(n))]; ok {
				at = i
				break
			}
		}
		switch {
		case at >= 0 && at < fromFile && x.Weak:
			// the glossary file already explains it
		case at >= 0 && at < fromFile && x.Source == "Battle Companies":
			t := &doc.Terms[at] // replaces the file's term, keeping its place, its id and its aliases
			t.Category, t.Definition, t.From, t.Sources, t.Path = x.Category, x.Definition, x.Source, nil, x.Path
			for _, n := range x.Aliases {
				if !slices.Contains(t.Aliases, n) {
					t.Aliases = append(t.Aliases, n)
				}
				owner[strings.ToLower(strings.TrimSpace(n))] = at
			}
		case at >= 0:
			// the name is taken
		default:
			doc.Terms = append(doc.Terms, rawTerm{Name: x.Name, Category: x.Category, Aliases: x.Aliases, Definition: x.Definition, From: x.Source, Path: x.Path})
			for _, n := range names {
				owner[strings.ToLower(strings.TrimSpace(n))] = len(doc.Terms) - 1
			}
		}
	}
	// Sorted by category, then by name, whatever order the files list them in. The keys are worked out once per term, not once per comparison.
	sortKey := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	type termKey struct{ category, name string }
	keys := make([]termKey, len(doc.Terms))
	order := make([]int, len(doc.Terms))
	for i, t := range doc.Terms {
		keys[i], order[i] = termKey{sortKey(t.Category), sortKey(t.Name)}, i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		if c := strings.Compare(keys[a].category, keys[b].category); c != 0 {
			return c
		}
		return strings.Compare(keys[a].name, keys[b].name)
	})
	sorted := make([]rawTerm, len(doc.Terms))
	for i, at := range order {
		sorted[i] = doc.Terms[at]
	}
	doc.Terms = sorted
	g := Glossary{Info: info, Terms: make([]GlossaryTerm, len(doc.Terms))}
	if chart, ok, err := ParseToWound(content); ok && err == nil {
		g.ToWound = &chart
	}

	byPhrase := map[string]string{} // lower-cased name or alias -> term id
	used := map[string]bool{}
	exact := map[string]string{}
	var phrases []string
	for i, t := range doc.Terms {
		id := slugify(t.Name)
		for n := 2; used[id] || id == ""; n++ {
			id = slugify(t.Name) + "-" + strconv.Itoa(n)
		}
		used[id] = true
		g.Terms[i] = GlossaryTerm{ID: id, Name: t.Name, Category: t.Category, Aliases: t.Aliases, Source: t.From, summaries: new(sync.Map)}
		for _, p := range slices.Concat([]string{t.Name}, t.Aliases) {
			key := strings.ToLower(strings.TrimSpace(p))
			if _, dup := byPhrase[key]; !dup && utf8.RuneCountInString(key) >= minLinkLen {
				byPhrase[key] = id
				phrases = append(phrases, key)
				if spelled := strings.TrimSpace(p); !strings.Contains(spelled, " ") {
					exact[key] = spelled
				}
			}
		}
	}
	if len(phrases) > 0 {
		slices.SortFunc(phrases, func(a, b string) int { return len(b) - len(a) }) // longest first, so "Stand Fast" beats "Stand"
		l := &linker{byFirst: map[string][]string{}, byPhrase: byPhrase, exact: exact}
		for _, p := range phrases {
			first := p
			if k := strings.IndexFunc(p, func(r rune) bool { return !isWordRune(r) }); k >= 0 {
				first = p[:k]
			}
			if first != "" && isWordRune([]rune(p)[0]) {
				l.byFirst[first] = append(l.byFirst[first], p)
			}
		}
		g.link = l
	}
	g.byID = make(map[string]int, len(g.Terms))
	for i, t := range g.Terms {
		g.byID[t.ID] = i
	}

	for i, t := range doc.Terms {
		term := &g.Terms[i]
		linked := map[string]bool{term.ID: true} // never to itself, and each target once per definition
		for _, para := range strings.Split(t.Definition, "\n\n") {
			para = strings.TrimSpace(para)
			if para == "" {
				continue
			}
			var segs []Segment
			if g.link != nil {
				segs = g.link.segments(para, linked)
			} else {
				segs = []Segment{{Text: para}}
			}
			term.Paragraphs = append(term.Paragraphs, segs)
		}
		for _, s := range t.See {
			if id, ok := byPhrase[strings.ToLower(strings.TrimSpace(s))]; ok && id != term.ID {
				name := s
				for _, o := range g.Terms {
					if o.ID == id {
						name = o.Name
					}
				}
				term.See = append(term.See, TermRef{ID: id, Name: name})
			}
		}
		for _, s := range t.Sources {
			term.Pages = append(term.Pages, s.RulebookPage)
		}
		if t.Path != nil {
			table := &PathTable{PathDice: t.Path.PathDice}
			for _, l := range t.Path.Limits {
				limit := PathTableLimit{Stat: l.Stat, Abbr: l.Abbr, Limit: l.Limit}
				if term, ok := g.Find(l.Term); ok {
					limit.TermID = term.ID
				}
				table.Limits = append(table.Limits, limit)
			}
			for _, row := range t.Path.Rows {
				segs := []Segment{{Text: row.Text}}
				if g.link != nil {
					segs = g.link.segments(row.Text, map[string]bool{term.ID: true})
				}
				table.Rows = append(table.Rows, PathTableRow{Roll: row.Roll, Segments: segs})
			}
			term.Path = table
		}
	}
	return g, nil
}
