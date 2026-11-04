package glossaryref

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// GlossaryVersion is ours, not the rulebook's: bump it when the content changes.
const GlossaryVersion = "1.2"

// pageOffset: in the 2024 manual the printed page number is the PDF page index minus 1 (the PDF
// starts with a cover).
const pageOffset = 1

// introOnly are overviews whose sub-sections are separate terms.
var introOnly = map[string]bool{"Heroic Actions": true, "War Beasts": true, "Turn Sequence": true}

// ws is the set of characters that count as white space (Unicode spaces and the separators).
const ws = `[\s\x{1c}-\x{1f}\x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`

var (
	headRe       = regexp.MustCompile(`^(#{2,4}) (.*)$`)
	pageRe       = regexp.MustCompile(`<!-- page (\p{Nd}+) -->`)
	inchRe       = regexp.MustCompile(`(\p{Nd})` + ws + `*[”“]`)
	spacesRe     = regexp.MustCompile(`[ \t]+`)
	wsRunRe      = regexp.MustCompile(ws + `+`)
	parenRe      = regexp.MustCompile(ws + `*\(.*?\)` + ws + `*`)
	stepLabelRe  = regexp.MustCompile(`([.!?])` + ws + `+\p{Nd}$`)
	dropCapRe    = regexp.MustCompile(`^[A-Z]$`)
	normReplacer = strings.NewReplacer("\ufeff", "", "’", "'", "‘", "'", "“", `"`, "”", `"`, "…", "...")
)

func stripSpace(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) })
}

// norm normalises typography the way the vanilla json does (ASCII quotes, inches as "),
// collapses whitespace.
func norm(s string) string {
	s = strings.ReplaceAll(s, "\ufeff", "")
	s = inchRe.ReplaceAllString(s, `$1"`)
	s = normReplacer.Replace(s)
	return stripSpace(spacesRe.ReplaceAllString(s, " "))
}

func titleKey(t string) string {
	t = wsRunRe.ReplaceAllString(parenRe.ReplaceAllString(t, " "), " ")
	return strings.ToUpper(stripSpace(t))
}

// titleCase upper-cases the first letter of every word and lower-cases the rest.
func titleCase(s string) string {
	var b strings.Builder
	prevCased := false
	for _, r := range s {
		cased := unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r)
		switch {
		case !cased:
			b.WriteRune(r)
		case prevCased:
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(unicode.ToTitle(r))
		}
		prevCased = cased
	}
	return b.String()
}

type heading struct {
	line, level int
	title, key  string
	page, end   int
}

func loadSections(text string) ([]string, []*heading) {
	lines := strings.Split(text, "\n")
	var heads []*heading
	page := 0
	for i, l := range lines {
		if m := pageRe.FindStringSubmatch(l); m != nil {
			page, _ = strconv.Atoi(m[1])
		}
		if m := headRe.FindStringSubmatch(l); m != nil {
			t := stripSpace(m[2])
			heads = append(heads, &heading{line: i, level: len(m[1]), title: t, key: titleKey(m[2]), page: page})
		}
	}
	for k, h := range heads {
		h.end = len(lines)
		for _, n := range heads[k+1:] {
			if n.level <= h.level {
				h.end = n.line
				break
			}
		}
	}
	return lines, heads
}

func clamp(lines []string, start, end int) []string {
	if end > len(lines) {
		end = len(lines)
	}
	if start < 0 {
		start = 0
	}
	if start >= end {
		return nil
	}
	return lines[start:end]
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func endsWithAny(s string, sufs ...string) bool {
	for _, x := range sufs {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}

// paragraphs gives the plain (non-example, non-heading) paragraphs of lines[start:end].
func paragraphs(lines []string, start, end int) []string {
	var out []string
	dropcap := ""
	for _, l := range clamp(lines, start, end) {
		l = stripSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, ">") || strings.HasPrefix(l, "<!--") || isDigits(l) {
			continue // bare numbers are diagram step labels
		}
		if dropCapRe.MatchString(l) { // a drop cap is a paragraph of its own in the converted text
			dropcap = l
			continue
		}
		if utf8.RuneCountInString(l) < 40 && !endsWithAny(l, ".", "!", ":", ")", "”") && !strings.HasPrefix(l, "•") { // running heads
			continue
		}
		text := stepLabelRe.ReplaceAllString(norm(dropcap+l), "$1") // a step label can trail a paragraph
		if text == "" {
			dropcap = ""
			continue
		}
		if n := len(out); n > 0 && dropcap == "" && !strings.HasPrefix(text, "•") && utf8.RuneCountInString(out[n-1]) > 25 &&
			!endsWithAny(out[n-1], ".", "!", "?", ":", "”", ")", `"`, "’", "'") {
			out[n-1] += " " + text // a paragraph split by a page or column break
		} else {
			out = append(out, text)
		}
		dropcap = ""
	}
	return out
}

// BuildOptions are the inputs of BuildGlossary.
type BuildOptions struct {
	Src       string // the rulebook converted to Markdown (sources/mesbg_rules.md)
	Out       string // the glossary file to write
	Summaries string // tools/glossary_summaries.json
	Uncapped  string // when set, write the full verbatim text of every term here instead of Out
}

// BuildResult reports a build.
type BuildResult struct {
	Terms    int
	KB       int // thousands of characters of definition text
	Problems []string
}

// Entry is a term of the glossary file.
type Entry struct {
	Name       string    `json:"name"`
	Category   string    `json:"category"`
	Aliases    []string  `json:"aliases,omitempty"`
	Definition string    `json:"definition"`
	Summarized bool      `json:"summarized,omitempty"`
	Sources    []Source  `json:"sources"`
	See        *[]string `json:"see,omitempty"`
}

type referenceDoc struct {
	ToWound ToWoundTable `json:"toWound"`
}

type glossaryDoc struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Source      string       `json:"source"`
	Description string       `json:"description"`
	Terms       []Entry      `json:"terms"`
	Reference   referenceDoc `json:"reference"`
}

func dedupe(parts [][]string) []string {
	seen := map[string]bool{}
	var flat []string
	for _, ps := range parts {
		for _, p := range ps {
			if !seen[p] {
				seen[p] = true
				flat = append(flat, p)
			}
		}
	}
	return flat
}

func lastLine(lines []string, i int) string {
	if i < 0 {
		i += len(lines) // counts from the end
	}
	return lines[i]
}

// BuildGlossary extracts the terms from the rulebook text and writes the glossary file.
func BuildGlossary(o BuildOptions) (BuildResult, error) {
	var res BuildResult
	summaries := map[string]string{}
	if b, err := os.ReadFile(o.Summaries); err == nil {
		if err := json.Unmarshal(b, &summaries); err != nil {
			return res, fmt.Errorf("%s: %w", o.Summaries, err)
		}
	}
	raw, err := os.ReadFile(o.Src)
	if err != nil {
		return res, err
	}
	lines, heads := loadSections(string(raw))
	pageOfLine := make([]int, len(lines))
	page := 0
	for i, l := range lines {
		if m := pageRe.FindStringSubmatch(l); m != nil {
			page, _ = strconv.Atoi(m[1])
		}
		pageOfLine[i] = page
	}
	byKey := map[string][]*heading{}
	for _, h := range heads {
		byKey[h.key] = append(byKey[h.key], h)
	}

	terms := Terms()
	var entries []Entry
	var problems []string
	uncappedNames := []string{}
	uncapped := map[string]string{}
	for _, t := range terms {
		var picked []*heading
		for _, title := range t.Headings {
			hs := byKey[titleKey(title)]
			if len(hs) == 0 {
				problems = append(problems, fmt.Sprintf("%s: heading %s not found", t.Name, quoteRepr(title)))
			}
			picked = append(picked, hs...)
		}
		sort.SliceStable(picked, func(a, b int) bool { return picked[a].line < picked[b].line })
		// drop sections nested in another picked section so text is not repeated
		var top []*heading
		for _, h := range picked {
			nested := false
			for _, o := range picked {
				if o != h && o.line < h.line && h.line < o.end {
					nested = true
				}
			}
			if !nested {
				top = append(top, h)
			}
		}
		var parts [][]string
		sources := []Source{}
		for _, h := range top {
			end := h.end
			if introOnly[t.Name] { // overview sections whose sub-sections are separate terms
				for _, n := range heads {
					if n.line > h.line {
						end = n.line
						break
					}
				}
			}
			if ps := paragraphs(lines, h.line+1, end); len(ps) > 0 {
				parts = append(parts, ps)
				sources = append(sources, Source{Section: strings.ReplaceAll(titleCase(wsRunRe.ReplaceAllString(h.title, " ")), "'S", "'s"), RulebookPage: h.page - pageOffset})
			}
		}
		if t.First > 0 { // brief entry: keep the first paragraphs of the headings' text (not of each section)
			flat := dedupe(parts)
			if len(flat) > t.First {
				flat = flat[:t.First]
			}
			parts = [][]string{flat}
		}
		for _, pr := range t.Paragraphs {
			rx, err := regexp.Compile(pr.Regex)
			if err != nil {
				return res, fmt.Errorf("%s: %w", t.Name, err)
			}
			found := false
			for i, l := range lines {
				if !rx.MatchString(stripSpace(l)) {
					continue
				}
				i0 := i
				j0 := i - 1
				for j0 > 0 && stripSpace(lines[j0]) == "" {
					j0--
				}
				if dropCapRe.MatchString(stripSpace(lastLine(lines, j0))) {
					i = j0 // include the drop cap
				}
				end := i + 1
				got := 0
				j := i + 1
				if i != i0 { // drop cap line: the paragraph itself follows
					for j < len(lines) && stripSpace(lines[j]) == "" {
						j++
					}
					end = j + 1
					j++
				}
				for got < pr.Following && j < len(lines) && !headRe.MatchString(lines[j]) {
					if stripSpace(lines[j]) != "" && !strings.HasPrefix(lines[j], ">") && !strings.HasPrefix(lines[j], "<!--") {
						got++
						end = j + 1
					}
					j++
				}
				parts = append(parts, paragraphs(lines, i, end))
				sources = append(sources, Source{Section: "(inline)", RulebookPage: pageOfLine[i] - pageOffset})
				found = true
				break
			}
			if !found {
				problems = append(problems, fmt.Sprintf("%s: paragraph %s not found", t.Name, quoteRepr(pr.Regex)))
			}
		}
		if len(parts) == 0 {
			problems = append(problems, t.Name+": nothing extracted")
			continue
		}
		e := Entry{Name: t.Name, Category: t.Category, Aliases: t.Aliases}
		e.Definition = strings.Join(dedupe(parts), "\n\n") // the manual repeats some blocks (e.g. on spreads)
		if _, dup := uncapped[t.Name]; !dup {
			uncappedNames = append(uncappedNames, t.Name)
		}
		uncapped[t.Name] = e.Definition
		if t.Synth { // a long chapter: the curating agent writes the brief reminder
			if s, ok := summaries[t.Name]; ok {
				e.Definition = s
				e.Summarized = true
			} else {
				problems = append(problems, t.Name+": needs a summary in tools/glossary_summaries.json (verbatim text kept for now)")
			}
		}
		e.Sources = sources
		entries = append(entries, e)
	}

	see := []string{"Keywords"}
	for _, k := range KeywordTerms() {
		entries = append(entries, Entry{Name: k.Name, Category: "keywords", Definition: k.Definition, Summarized: true, Sources: []Source{KeywordSource}, See: &see})
		terms = append(terms, Term{Name: k.Name, Category: "keywords", See: []string{"Keywords"}})
	}

	// `see` links point to other terms of this file (a small concept graph); they must resolve
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	byName := map[string]Term{}
	for _, t := range terms {
		byName[t.Name] = t
	}
	for i := range entries {
		e := &entries[i]
		if e.See != nil { // the keyword entries
			continue
		}
		sees := byName[e.Name].See
		var kept []string
		for _, target := range sees {
			if !names[target] {
				problems = append(problems, fmt.Sprintf("%s: see -> %s is not a term", e.Name, quoteRepr(target)))
			} else {
				kept = append(kept, target)
			}
		}
		if len(sees) > 0 {
			if kept == nil {
				kept = []string{}
			}
			e.See = &kept
		}
	}
	// keyword entries link to "Keywords", which always resolves, so it is not checked
	if !names["Keywords"] {
		for _, k := range KeywordTerms() {
			problems = append(problems, fmt.Sprintf("%s: see -> 'Keywords' is not a term", k.Name))
		}
	}

	synth := map[string]bool{}
	for _, t := range terms {
		if t.Synth {
			synth[t.Name] = true
		}
	}
	var stray []string
	for n := range summaries {
		if !synth[n] {
			stray = append(stray, n)
		}
	}
	sort.Strings(stray)
	for _, n := range stray {
		problems = append(problems, fmt.Sprintf("summary for %s has no synth term", quoteRepr(n)))
	}

	if o.Uncapped != "" {
		var b bytes.Buffer
		b.WriteString("{")
		for i, n := range uncappedNames {
			if i > 0 {
				b.WriteString(",")
			}
			k, _ := marshalNoEscape(n, "")
			v, _ := marshalNoEscape(uncapped[n], "")
			b.WriteString("\n " + string(k) + ": " + string(v))
		}
		b.WriteString("\n}")
		res.Problems = problems
		return res, os.WriteFile(o.Uncapped, b.Bytes(), 0o644)
	}

	doc := glossaryDoc{
		Name:        "MESBG Rules Glossary (2024 manual)",
		Version:     GlossaryVersion,
		Source:      "Middle-earth Strategy Battle Game Rules Manual (2024)",
		Description: "Concepts of the MESBG rules manual that the vanilla definitions leave out. Complements the vanilla file; text extracted verbatim from the rules manual (`rulebookPage` is the page number printed in the rulebook). This file refers to nothing else: other files refer to it.",
		Terms:       entries,
		Reference:   referenceDoc{ToWound: ToWound()},
	}
	out, err := marshalNoEscape(doc, "  ")
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(filepath.Dir(o.Out), 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(o.Out, append(out, '\n'), 0o644); err != nil {
		return res, err
	}
	res.Terms = len(entries)
	chars := 0
	for _, e := range entries {
		chars += utf8.RuneCountInString(e.Definition)
	}
	res.KB = chars / 1000
	res.Problems = problems
	return res, nil
}

// marshalNoEscape is json.MarshalIndent without HTML escaping (non-ASCII is kept).
func marshalNoEscape(v any, indent string) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// quoteRepr quotes a text (single quotes unless the text has one and no double).
func quoteRepr(s string) string {
	q := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = `"`
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	if q == "'" {
		s = strings.ReplaceAll(s, "'", `\'`)
	}
	return q + s + q
}
