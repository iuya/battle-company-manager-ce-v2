package web

import (
	"fmt"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// keywordMax is how many chips the compact row shows before the "+N" button (docs/unit-keywords.md).
const keywordMax = 5

// notShownKeywords are the unit types no chip shows (owner decision, docs/unit-keywords.md): the card frame and the role insignia already say
// Hero and Warrior, the mount has its own display, and the kind icon says how the unit moves (Infantry, Cavalry, Beast and War Beast, Monster).
// Unique, Elite, Swarm, Chariot and the siege types have no other display and stay. core keeps computing every keyword (Hidden, Off).
var notShownKeywords = map[string]bool{"hero": true, "warrior": true, "mount": true, "infantry": true, "cavalry": true, "beast": true, "war beast": true, "monster": true}

// keywordShown says whether a keyword is drawn as a chip (row and full list alike).
func keywordShown(w core.UnitKeyword) bool {
	return w.Kind != core.KeywordType || !notShownKeywords[strings.ToLower(w.Name)]
}

// keywordRowView is what the keyword_row template draws: the compact row's chips, how many more there are, and the three groups of the
// full list. Built from core.UnitKeywords, never computed here.
type keywordRowView struct {
	Shown               []core.UnitKeyword
	More                int // chips past the row's limit: the number on the button
	Race, Faction, Type []core.UnitKeyword
}

// keywordRow splits a unit's keywords for the template. max <= 0 uses keywordMax.
func keywordRow(k core.UnitKeywords, max int) keywordRowView {
	if max <= 0 {
		max = keywordMax
	}
	k.Model = slices.DeleteFunc(slices.Clone(k.Model), func(w core.UnitKeyword) bool { return !keywordShown(w) })
	shown, more := k.Row(max)
	return keywordRowView{Shown: shown, More: more, Race: k.Group(core.KeywordRace), Faction: k.Group(core.KeywordFaction), Type: k.Group(core.KeywordType)}
}

// keywordMeta is the second line of a chip's tooltip: which group the keyword is in; part names the mount for a mount's keywords.
func keywordMeta(w core.UnitKeyword, part string) string {
	var m string
	switch w.Kind {
	case core.KeywordRace:
		m = "Race keyword"
	case core.KeywordFaction:
		m = "Faction keyword"
	default:
		m = "Unit type"
	}
	if part != "" {
		m += " · of the " + part
	}
	return m
}

// keywordLabel is the chip's accessible name: its group first ("Race keyword: Elf").
func keywordLabel(w core.UnitKeyword, part string) string {
	var g string
	switch w.Kind {
	case core.KeywordRace:
		g = "Race keyword"
	case core.KeywordFaction:
		g = "Faction keyword"
	default:
		g = "Unit type keyword"
	}
	if part != "" {
		g = fmt.Sprintf("%s of the %s", g, part)
	}
	s := g + ": " + w.Name
	return s
}

// joinNames writes names for a sentence: "Aldor", "Aldor and Hama", "Aldor, Hama and Eowyn".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
