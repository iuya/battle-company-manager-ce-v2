// Package leancheck holds the checks of the lean Battle Companies file that the app's own validator (core.CheckBattleCompaniesV2, the
// code the upload runs) does not make. Used by tools/validate.
//
//   - company texts: no rule repeated inside a company's specialRules or notes, no parse artefact in the texts copied from the rulebook
//     (unbalanced quotes, an epigraph glued to a rule, a text starting or ending mid-sentence, a page header);
//   - aura rings (docs/aura-ranges.md): the vocabulary tables are the specified ones, a ring that names its group follows its "when" and
//     radius, and the radius is written in the text of the thing that draws it (or of its concept, merged labels or items);
//   - rule targets: the keywords a target names are keywords of the vanilla units (race, faction, unit type) or of Battle Companies' own
//     mounts;
//   - Tabletop Simulator: every model a piece of gear names is in the local catalogue sources/tts/models.json when that gitignored file
//     exists.
//
// Every function takes the decoded lean document (bc) and returns a list of error strings.
package leancheck

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/jsonx"
)

var nonWord = regexp.MustCompile(`[^\pL\pN_]+`)

// Norm lower-cases a text and reduces every run of non-word characters to one space.
func Norm(t string) string {
	return strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(t), " "))
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Fold lower-cases, drops accents and reduces everything but letters and digits to single spaces.
func Fold(s string) string {
	s = norm.NFKD.String(strings.ReplaceAll(s, "’", "'"))
	var b strings.Builder
	for _, r := range s {
		if !unicode.Is(unicode.Mn, r) && norm.NFKD.PropertiesString(string(r)).CCC() == 0 {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(nonAlnum.ReplaceAllString(strings.ToLower(b.String()), " "))
}

// ---- company texts ----

const quotes = `"“”`

var (
	fragmentEnd   = regexp.MustCompile(`(?i)(?:[,;:–—-]|\b(?:and|or|the|a|an|of|to|in|on|for|with|than|that|which|as|if|but|while|rather))$`)
	epigraph      = regexp.MustCompile(`^\s*[` + quotes + `].{3,200}?[` + quotes + `]\s*[–—-]`)
	pageFurniture = regexp.MustCompile(`(?i)Battle Companies:?\s*2nd|Community Edition\s*-|^\s*Page\s+\d+\s*$|^\s*\d+\s*/\s*\d+\s*$`)
	dashStart     = regexp.MustCompile(`^[–—-]\s`)
	lowerStart    = regexp.MustCompile(`^[a-z]`)
	punctStart    = regexp.MustCompile(`^[,;:.)]`)
)

// Balanced reports whether quotation marks and brackets pair up (typographic quotes pair in order, straight quotes in twos).
func Balanced(t string) bool {
	if strings.Count(t, "“") != strings.Count(t, "”") || strings.Count(t, "(") != strings.Count(t, ")") || strings.Count(t, "[") != strings.Count(t, "]") {
		return false
	}
	return strings.Count(t, `"`)%2 == 0
}

// Artefacts says what looks wrong with a rule or note text, as a list of short reasons.
func Artefacts(t string) []string {
	var out []string
	s := strings.TrimSpace(t)
	if s == "" {
		return []string{"empty"}
	}
	if !Balanced(s) {
		out = append(out, "unbalanced quotation marks or brackets")
	}
	if epigraph.MatchString(s) {
		out = append(out, "starts with a quoted epigraph and a dash")
	}
	if dashStart.MatchString(s) || lowerStart.MatchString(s) || punctStart.MatchString(s) {
		out = append(out, "starts mid-sentence")
	}
	if fragmentEnd.MatchString(s) {
		out = append(out, "ends mid-sentence")
	}
	if pageFurniture.MatchString(s) {
		out = append(out, "contains a page header or footer")
	}
	return out
}

func noteText(n any) string {
	if o := jsonx.O(n); o != nil {
		return jsonx.S(o["text"])
	}
	return jsonx.S(n)
}

type text struct{ where, text string }

// companyTexts is every sentence of a company that is copied from the rulebook as running text.
func companyTexts(c jsonx.Obj) []text {
	var out []text
	for i, r := range jsonx.A(c["specialRules"]) {
		out = append(out, text{fmt.Sprintf("specialRules[%d]", i), jsonx.S(r)})
	}
	for i, n := range jsonx.A(c["notes"]) {
		out = append(out, text{fmt.Sprintf("notes[%d]", i), noteText(n)})
	}
	if t := jsonx.S(jsonx.O(c["oath"])["text"]); t != "" {
		out = append(out, text{"oath.text", t})
	}
	for i, p := range strings.Split(jsonx.S(c["description"]), "\n\n") {
		if strings.TrimSpace(p) != "" {
			out = append(out, text{fmt.Sprintf("description[%d]", i), p})
		}
	}
	if ep := jsonx.O(c["epigraph"]); jsonx.Truthy(ep) {
		out = append(out, text{"epigraph.quote", jsonx.S(ep["quote"])}, text{"epigraph.text", jsonx.S(ep["text"])})
	}
	for i, h := range jsonx.A(c["heroUpgrades"]) {
		for _, k := range []string{"flavour", "text"} {
			if v := jsonx.S(jsonx.O(h)[k]); v != "" {
				out = append(out, text{fmt.Sprintf("heroUpgrades[%d].%s", i, k), v})
			}
		}
	}
	return out
}

// CompanyTextErrors checks the texts of every company.
func CompanyTextErrors(bc jsonx.Obj) []string {
	var errors []string
	for _, ci := range jsonx.A(bc["companies"]) {
		c := jsonx.O(ci)
		name := jsonx.ReprString(jsonx.S(c["name"]))
		for _, t := range companyTexts(c) {
			if t.where == "epigraph.quote" {
				continue
			}
			for _, why := range Artefacts(t.text) {
				errors = append(errors, fmt.Sprintf("company %s %s: %s: %s", name, t.where, why, jsonx.ReprString(jsonx.Slice(t.text, 80))))
			}
		}
		var rules, notes []text
		for i, r := range jsonx.A(c["specialRules"]) {
			rules = append(rules, text{fmt.Sprintf("specialRules[%d]", i), jsonx.S(r)})
		}
		for i, n := range jsonx.A(c["notes"]) {
			notes = append(notes, text{fmt.Sprintf("notes[%d]", i), noteText(n)})
		}
		for _, items := range [][]text{append(append([]text{}, rules...), notes...), notes} {
			for a := range items {
				for b := a + 1; b < len(items); b++ {
					ka, kb := Norm(items[a].text), Norm(items[b].text)
					if ka != "" && kb != "" && (ka == kb || strings.Contains(kb, ka) || strings.Contains(ka, kb)) {
						errors = append(errors, fmt.Sprintf("company %s: %s and %s repeat the same text: %s", name, items[a].where, items[b].where, jsonx.ReprString(jsonx.Slice(items[a].text, 80))))
					}
				}
			}
		}
		if ep := jsonx.O(c["epigraph"]); jsonx.Truthy(ep) {
			want := Norm(jsonx.S(ep["text"]))
			for _, r := range rules {
				if strings.Contains(Norm(r.text), want) {
					errors = append(errors, fmt.Sprintf("company %s: the epigraph is repeated in specialRules", name))
					break
				}
			}
		}
	}
	return errors
}

// ---- aura rings ----

// AuraKinds is the specified order of the kinds of ring.
var AuraKinds = []string{"banner", "command", "support", "enemy", "utility"}

// AuraGroupDefaults is the specified set of groups with their defaults.
var AuraGroupDefaults = map[string]bool{"always": true, "declared": true, "conditional": true, "cast": false, "target": false, "large": false, "optional": false}

const (
	auraKindsText         = "['banner', 'command', 'support', 'enemy', 'utility']"
	auraGroupDefaultsText = "{'always': True, 'declared': True, 'conditional': True, 'cast': False, 'target': False, 'large': False, 'optional': False}"
	// Large is the radius, in inches, above which a ring is in the "large" group.
	Large = 12
)

type radiusSum struct {
	parts []string
	also  []string
}

// radiusSums holds a ring whose radius is a sum: (owner, radius) -> the parts each written in the text, and the other terms whose
// text is read too (the war horn adds 3" to a Hero's Stand Fast of 6").
var radiusSums = map[string]radiusSum{"War Horn\x009": {parts: []string{"6", "3"}, also: []string{"Stand Fast"}}}

// Has reports whether the number n (inches) is written in the text as N" (not as part of a longer number).
func Has(n string, t string) bool {
	re := regexp.MustCompile(regexp.QuoteMeta(n) + `\s*["”]`)
	for _, m := range re.FindAllStringIndex(t, -1) {
		if m[0] == 0 {
			return true
		}
		prev := t[m[0]-1]
		if !(prev >= '0' && prev <= '9') && prev != '.' {
			return true
		}
	}
	return false
}

// GroupOf is the group a ring belongs to when it names none: its "when"; a spell in effect or a targeting range keeps its own group,
// a ring larger than Large inches is a large ring.
func GroupOf(ring jsonx.Obj) string {
	when := jsonx.S(ring["when"])
	if when == "target" || when == "cast" {
		return when
	}
	if jsonx.Float(ring["radius"]) > Large {
		return "large"
	}
	return when
}

type auraRing struct {
	kind, owner string
	ring        jsonx.Obj
}

func rings(bc jsonx.Obj) []auraRing {
	var out []auraRing
	for _, r := range jsonx.A(bc["rules"]) {
		for _, a := range jsonx.A(jsonx.O(r)["auras"]) {
			out = append(out, auraRing{"rule", jsonx.S(jsonx.O(r)["name"]), jsonx.O(a)})
		}
	}
	for _, g := range jsonx.A(bc["gear"]) {
		for _, a := range jsonx.A(jsonx.O(g)["auras"]) {
			out = append(out, auraRing{"item", jsonx.S(jsonx.O(g)["name"]), jsonx.O(a)})
		}
	}
	powers := jsonx.O(jsonx.O(bc["magic"])["powers"])
	names := make([]string, 0, len(powers))
	for n := range powers { // the document order is lost with a Go map; sorted keeps the report stable
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, a := range jsonx.A(jsonx.O(powers[n])["auras"]) {
			out = append(out, auraRing{"power", n, jsonx.O(a)})
		}
	}
	return out
}

// AuraErrors checks the aura vocabulary and every ring. textOf(name) is the text of the glossary terms of that name (ok false when
// there is none; the app's loader makes the glossary, conceptgraph.Index.Texts); van is the vanilla file, whose rule texts are read
// too (a glossary term of the same name may say something else, like the path rule Lead by Example).
func AuraErrors(bc jsonx.Obj, textOf func(string) (string, bool), van jsonx.Obj) []string {
	var errors []string
	a := jsonx.O(bc["auras"])
	if a == nil {
		return []string{"auras is missing"}
	}
	var kinds []string
	for _, k := range jsonx.A(a["kinds"]) {
		kinds = append(kinds, jsonx.S(jsonx.O(k)["id"]))
	}
	if strings.Join(kinds, "\x00") != strings.Join(AuraKinds, "\x00") {
		errors = append(errors, fmt.Sprintf("auras.kinds must be %s in this order", auraKindsText))
	}
	for _, k := range jsonx.A(a["kinds"]) {
		ko := jsonx.O(k)
		if d, ok := ko["default"]; ok && !jsonx.Truthy(d) {
			errors = append(errors, fmt.Sprintf("auras kind %s: every kind is on by default (docs/aura-ranges.md 4.4)", jsonx.ReprString(jsonx.S(ko["id"]))))
		}
	}
	groups := map[string]any{}
	for _, g := range jsonx.A(a["groups"]) {
		go_ := jsonx.O(g)
		groups[jsonx.S(go_["id"])] = go_["default"]
	}
	same := len(groups) == len(AuraGroupDefaults)
	for id, want := range AuraGroupDefaults {
		if got, ok := groups[id]; !ok || got != want {
			same = false
		}
	}
	if !same {
		errors = append(errors, fmt.Sprintf("auras.groups must be exactly %s (docs/aura-ranges.md section 9)", auraGroupDefaultsText))
	}
	vanillaRules := map[string]string{}
	for _, k := range jsonx.A(jsonx.O(van["data"])["keywords"]) {
		ko := jsonx.O(k)
		f := Fold(jsonx.S(ko["name"]))
		if _, ok := vanillaRules[f]; !ok {
			vanillaRules[f] = jsonx.S(ko["definition"])
		}
	}
	leanRules := map[string]string{}
	for _, r := range jsonx.A(bc["rules"]) {
		ro := jsonx.O(r)
		leanRules[Fold(jsonx.S(ro["name"]))] = jsonx.S(ro["definition"])
	}
	ownerText := func(n string) (string, bool) {
		extra := leanRules[Fold(n)]
		if extra == "" {
			extra = vanillaRules[Fold(n)]
		}
		t, ok := textOf(n)
		if !ok && extra == "" {
			return "", false
		}
		return t + " " + extra, true
	}
	for _, r := range rings(bc) {
		ring := r.ring
		w := fmt.Sprintf("aura ring of %s %s", r.kind, jsonx.ReprString(r.owner))
		if l := jsonx.Str(ring["label"]); jsonx.Truthy(ring["label"]) {
			w += " (" + l + ")"
		}
		want := GroupOf(ring)
		group := jsonx.S(ring["group"])
		when := jsonx.S(ring["when"])
		if group != "" && group != want && !(group == "optional" && when == "always" && jsonx.S(ring["kind"]) == "utility") {
			errors = append(errors, fmt.Sprintf("%s: group %s but when %s and radius %s make it %s", w, jsonx.ReprString(group), jsonx.ReprString(when), jsonx.Str(ring["radius"]), jsonx.ReprString(want)))
		}
		if jsonx.Len(jsonx.S(ring["tip"])) > 200 {
			errors = append(errors, w+": tip is longer than a sentence (200 characters)")
		}
		radius := jsonx.Str(ring["radius"])
		sums, hasSum := radiusSums[fmt.Sprintf("%s\x00%s", r.owner, radius)]
		var named []string
		if c := jsonx.S(ring["concept"]); c != "" {
			named = append(named, c)
		}
		for _, k := range []string{"items", "mergeLabels"} {
			for _, x := range jsonx.Strs(ring[k]) {
				if x != "" {
					named = append(named, x)
				}
			}
		}
		pool := []string{}
		if t, ok := ownerText(r.owner); ok {
			pool = append(pool, t)
		}
		all := append(append([]string{}, named...), sums.also...)
		for _, x := range all {
			if t, ok := ownerText(x); ok {
				pool = append(pool, t)
			}
		}
		for _, x := range named {
			if _, ok := ownerText(x); !ok {
				errors = append(errors, fmt.Sprintf("%s: %s is not a term of the glossary", w, jsonx.ReprString(x)))
			}
		}
		var nonEmpty []string
		for _, t := range pool {
			if t != "" {
				nonEmpty = append(nonEmpty, t)
			}
		}
		union := strings.Join(nonEmpty, " ")
		parts := []string{radius}
		if hasSum {
			parts = sums.parts
		}
		for _, part := range parts {
			if !Has(part, union) {
				errors = append(errors, fmt.Sprintf("%s: the radius %s\" is not written in the text of %s or of its concept", w, part, jsonx.ReprString(r.owner)))
			}
		}
	}
	return errors
}

// ---- rule targets ----

// KeywordVocabulary is every keyword a vanilla model can have: race, faction, unit type, and the profile's own kind (Hero, Warrior).
func KeywordVocabulary(van jsonx.Obj) map[string]bool {
	out := map[string]bool{"hero": true, "warrior": true}
	for _, k := range []string{"heroes", "warriors"} {
		for _, u := range jsonx.A(jsonx.O(van["data"])[k]) {
			for _, f := range []string{"race", "faction", "unitType"} {
				for _, x := range jsonx.Strs(jsonx.O(u)[f]) {
					out[Fold(x)] = true
				}
			}
		}
	}
	return out
}

// RuleTargetErrors checks the keywords of the rule targets against the vanilla vocabulary and Battle Companies' own mounts.
func RuleTargetErrors(bc, van jsonx.Obj) []string {
	var errors []string
	vocab := KeywordVocabulary(van)
	for _, m := range jsonx.O(jsonx.O(bc["vanilla"])["mounts"]) {
		for _, f := range []string{"race", "unitType"} {
			for _, x := range jsonx.Strs(jsonx.O(m)[f]) {
				vocab[Fold(x)] = true
			}
		}
	}
	for _, r := range jsonx.A(bc["rules"]) {
		ro := jsonx.O(r)
		for _, field := range []string{"target", "bareTarget"} {
			t := jsonx.O(ro[field])
			if !jsonx.Truthy(t) {
				continue
			}
			for _, kw := range jsonx.Strs(t["keywords"]) {
				if !vocab[Fold(kw)] {
					errors = append(errors, fmt.Sprintf("rule %s: %s keyword %s is not a keyword of any vanilla model", jsonx.ReprString(jsonx.S(ro["name"])), field, jsonx.ReprString(kw)))
				}
			}
		}
	}
	return errors
}

// ---- Tabletop Simulator ----

// Catalogue is the set of (name, options) pairs, folded, of the local Tabletop Simulator catalogue.
type Catalogue map[[2]string]bool

func split(model string) [2]string {
	name, opts, _ := strings.Cut(model, ":")
	return [2]string{Fold(name), Fold(opts)}
}

// TTSCatalogue reads the local catalogue's model keys ("Name: options"); nil when the file does not exist.
func TTSCatalogue(path string) (Catalogue, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	raw, err := jsonx.ReadFile(path)
	if err != nil {
		return nil, err
	}
	keys := raw
	if o := jsonx.O(raw); o != nil {
		keys = o["models"]
	}
	out := Catalogue{}
	for _, k := range jsonx.Strs(keys) {
		out[split(k)] = true
	}
	return out, nil
}

// TTSErrors checks that every model the file names is in the local catalogue (a no-op without one).
func TTSErrors(bc jsonx.Obj, cat Catalogue) []string {
	if cat == nil {
		return nil
	}
	var errors []string
	for _, g := range jsonx.A(bc["gear"]) {
		go_ := jsonx.O(g)
		t := jsonx.O(go_["tts"])
		if !jsonx.Truthy(t) {
			continue
		}
		models := append([]string{jsonx.S(t["model"])}, jsonx.Strs(t["alternatives"])...)
		for _, m := range models {
			if m == "" {
				continue
			}
			k := split(m)
			if !cat[k] && !cat[[2]string{k[0], ""}] {
				errors = append(errors, fmt.Sprintf("gear %s: model %s is not in the local Tabletop Simulator catalogue", jsonx.ReprString(jsonx.S(go_["name"])), jsonx.ReprString(m)))
			}
		}
	}
	return errors
}
