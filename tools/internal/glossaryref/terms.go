package glossaryref

import "fmt"

// ParagraphRule asks for a paragraph the manual only defines inline: the first line matching
// Regex, then Following more paragraphs.
type ParagraphRule struct {
	Regex     string
	Following int
}

// Term is a term of the manual that the definitions refer to but do not define.
//
// Headings are section titles in the manual (every occurrence is included, parentheses and
// rule-example numbers ignored). Aliases are other spellings used by the definitions files
// (synonyms only: a related concept is a separate term, linked with See). First keeps only the
// first N paragraphs of the headings' text (0 keeps all); it only stops the text of the next
// chapter from leaking in. Synth marks long rule chapters whose brief entry is written by the
// curating agent in tools/glossary_summaries.json.
type Term struct {
	Name       string
	Category   string
	Headings   []string
	Aliases    []string
	Paragraphs []ParagraphRule
	See        []string
	First      int
	Synth      bool
}

// Source is a place in the rulebook.
type Source struct {
	Section      string `json:"section"`
	RulebookPage int    `json:"rulebookPage"`
}

// KeywordSource is where the manual explains the idea of a keyword.
var KeywordSource = Source{Section: "Keywords", RulebookPage: 8}

// RaceKeywords and FactionKeywords appear on unit cards and mounts. The manual only explains the
// idea (term "Keywords"); the brief entries are written by the curator, never quoted, and are
// marked summarized. The name carries " keyword" so that the glossary linker does not turn every
// "man", "elf" or "shire" of the rules texts into a link.
var (
	RaceKeywords = []string{"Bat", "Bird", "Boar", "Camel", "Drake", "Dwarf", "Eagle", "Elf", "Elk", "Goat", "Goblin", "Hobbit", "Horse", "Man", "Ogre",
		"Orc", "Spider", "Spirit", "Troll", "Uruk-hai", "Warg"}
	FactionKeywords = []string{"Angmar", "Arnor", "Black Dragon", "Blackroot Vale", "Carn Dum", "Corsair", "Dale", "Dol Amroth", "Dol Guldur", "Druadan",
		"Dunedain", "Dunland", "Easterling", "Erebor", "Goblin-town", "Gondor", "Gundabad", "Harad", "Iron Hills", "Isengard",
		"Khand", "Khazad-dum", "Lake-town", "Lamedon", "Lossarnach", "Lothlórien", "Mahûd", "Mirkwood", "Misty Mountains",
		"Mordor", "Moria", "Numenor", "Rivendell", "Rohan", "Ruffian", "Shire", "Umbar"}
)

// KeywordTerm is an authored race or faction entry.
type KeywordTerm struct {
	Name       string
	Kind       string // "race" or "faction"
	Definition string
}

// KeywordTerms returns the authored race and faction terms, races first.
func KeywordTerms() []KeywordTerm {
	var out []KeywordTerm
	for _, k := range []struct {
		kind  string
		words []string
	}{{"race", RaceKeywords}, {"faction", FactionKeywords}} {
		for _, w := range k.words {
			out = append(out, KeywordTerm{
				Name: w + " keyword",
				Kind: k.kind,
				Definition: fmt.Sprintf("%s is a %s keyword. Special rules that name a keyword, such as Hatred (%s), apply to models that have it.",
					w, k.kind, w),
			})
		}
	}
	return out
}
