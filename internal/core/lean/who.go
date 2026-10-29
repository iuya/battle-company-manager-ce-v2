package lean

import (
	"slices"
	"strings"
)

// Who is the one selector of the data for "which models": the models that may buy an item, take a hero upgrade, receive a granted rule or
// be made starting heroes. Every field narrows it; all that are set must hold, and an empty Who admits every model. Who.Admits is the only
// place that reads it.
type Who struct {
	Hero      *bool    `json:"hero,omitempty"`                       // true: heroes only; false: warriors only
	Units     []string `json:"units,omitempty"`                      // exact unit names (the name of the model or of its profile), never part of a name
	Except    []string `json:"except,omitempty"`                     // exact unit names that are left out
	Race      []string `json:"race,omitempty"`                       // any of these races (Elf, Hobbit)
	Keywords  []string `json:"keywords,omitempty"`                   // any of these keywords of the model: its races, factions and unit types (Dol Amroth, Spider, Cavalry)
	Alignment string   `json:"alignment,omitempty" enum:"good|evil"` // the alignment of the company
	Text      string   `json:"text,omitempty"`                       // how the page names the models ("Hobbit Heroes", "All Rohan Hero models"); made from the fields when empty
	Refusal   string   `json:"refusal,omitempty"`                    // what the page says to a model that is not admitted; made from the fields when empty
}

// Subject is a model as Who sees it: the facts Admits needs, filled by the caller from the company and the rules.
type Subject struct {
	Names     []string // what the model is called: its own name and the name of its profile
	Hero      bool
	Known     bool     // the profile of the model is known: without it, races are not tested (a model whose profile is missing is not refused)
	Race      []string // the races of the profile
	Keywords  []string // the races, factions and unit types of the profile, and Cavalry for a model that rides
	Alignment string   // of the company: good or evil
}

// Admits says why a model is not admitted (the refusal, in the words the page shows); empty when it is. A nil Who admits every model.
func (w *Who) Admits(s Subject) string {
	if w == nil {
		return ""
	}
	refuse := func(why string) string {
		if w.Refusal != "" {
			return w.Refusal
		}
		return why
	}
	if w.Hero != nil && *w.Hero != s.Hero {
		if *w.Hero {
			return refuse("Heroes only")
		}
		return refuse("Warriors only")
	}
	if len(w.Keywords) > 0 && !overlaps(w.Keywords, s.Keywords) {
		return refuse(strings.Join(w.Keywords, " or ") + " only")
	}
	if w.Alignment != "" && w.Alignment != s.Alignment {
		return refuse(strings.ToUpper(w.Alignment[:1]) + w.Alignment[1:] + " companies only")
	}
	if s.Known && len(w.Race) > 0 && !overlaps(w.Race, s.Race) {
		return refuse(strings.Join(w.Race, " or ") + " only")
	}
	if len(w.Units) > 0 && !overlaps(w.Units, s.Names) {
		return refuse(strings.Join(w.Units, ", ") + " only")
	}
	if overlaps(w.Except, s.Names) {
		return refuse("Not for " + strings.Join(w.Except, ", "))
	}
	return ""
}

// Label is how the page names the models the selector admits: its text, else the races, keywords and units it names, with Heroes or
// Warriors when it says so ("Hobbit Heroes"); empty for a selector that admits everyone.
func (w *Who) Label() string {
	if w == nil {
		return ""
	}
	if w.Text != "" {
		return w.Text
	}
	var parts []string
	parts = append(parts, w.Race...)
	parts = append(parts, w.Keywords...)
	parts = append(parts, w.Units...)
	if w.Hero != nil {
		parts = append(parts, map[bool]string{true: "Heroes", false: "Warriors"}[*w.Hero])
	}
	return strings.Join(parts, " ")
}

// Badges are the short words the armoury shows next to an item for who may take it: hero, a keyword (cavalry), the alignment.
func (w *Who) Badges() []string {
	if w == nil {
		return nil
	}
	var out []string
	if w.Hero != nil && *w.Hero {
		out = append(out, "hero")
	}
	for _, k := range w.Keywords {
		out = append(out, strings.ToLower(k))
	}
	if w.Alignment != "" {
		out = append(out, w.Alignment)
	}
	return out
}

// WithoutRace is the selector with its race left out: what remains when something else admits the race (a company that lets its models
// wear Elven-made weaponry).
func (w *Who) WithoutRace() *Who {
	if w == nil {
		return nil
	}
	c := *w
	c.Race = nil
	return &c
}

// overlaps says whether the two lists share a name, compared as names are everywhere (case, accents and punctuation do not matter).
func overlaps(want, have []string) bool {
	return slices.ContainsFunc(want, func(a string) bool {
		return slices.ContainsFunc(have, func(b string) bool { return NameKey(a) == NameKey(b) })
	})
}

// Models is the selector as a noun phrase for a sentence ("Dol Amroth Hero models", "Corsair Reaver and Corsair of Umbar models", "models
// except Hobbit Archer"); "models" alone for a selector that admits everyone.
func (w *Who) Models() string {
	if w == nil {
		return "models"
	}
	list := func(l []string, last string) string {
		if len(l) < 2 {
			return strings.Join(l, "")
		}
		return strings.Join(l[:len(l)-1], ", ") + " " + last + " " + l[len(l)-1]
	}
	var words []string
	switch {
	case len(w.Units) > 0:
		words = append(words, list(w.Units, "and"))
	default:
		words = append(words, list(append(slices.Clone(w.Race), w.Keywords...), "or"))
	}
	if w.Hero != nil {
		words = append(words, map[bool]string{true: "Hero", false: "Warrior"}[*w.Hero])
	}
	words = append(words, "models")
	if len(w.Except) > 0 {
		words = append(words, "except", list(w.Except, "and"))
	}
	if w.Alignment != "" {
		words = append(words, "of "+w.Alignment+" companies")
	}
	return strings.Join(slices.DeleteFunc(words, func(s string) bool { return s == "" }), " ")
}
