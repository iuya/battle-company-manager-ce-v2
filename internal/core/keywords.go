package core

import (
	"slices"
	"strings"
)

// Keywords of a model (docs/unit-keywords.md): the race, faction and unit-type words rules such as Hatred (Rohan) or "whilst this model
// has the Cavalry keyword" name. A model made of several parts (rider and Mount) has separate keywords for each part.

// KeywordKind is the group a keyword belongs to.
type KeywordKind string

const (
	KeywordRace    KeywordKind = "race"
	KeywordFaction KeywordKind = "faction"
	KeywordType    KeywordKind = "type"
)

// Where a keyword comes from.
const (
	KeywordFromProfile = "profile"
	KeywordFromCompany = "company" // an alias's addKeywords, a unitTypes effect
	KeywordFromFillIn  = "fillIn"
	KeywordFromMount   = "mount"
)

// hiddenTypes are the unit types the compact row leaves out: the kind icon, the insignia or the type line already say them.
var hiddenTypes = []string{"Hero", "Warrior", "Infantry", "Unique", "Mount"}

// seeRider is the faction a mount has when it belongs to its rider's.
const seeRider = "See Rider"

// UnitKeyword is one keyword of a model.
type UnitKeyword struct {
	Name    string // display spelling: "Elf", "Khazad-dum" (variants merged), "Cavalry"
	Kind    KeywordKind
	Hidden  bool   // left out of the compact row (Hero, Warrior, Infantry, Unique, Mount)
	Source  string // KeywordFromProfile, KeywordFromCompany, KeywordFromFillIn, KeywordFromMount
	Text    string // tooltip text, already resolved (glossary definition or the generated sentence)
	Concept string // data-concept value: the glossary term of the same name, else "Keywords"
	Off     bool   // set by OnFoot on Cavalry: shown struck through in the full list, omitted from the row
}

// UnitKeywords is a unit's keyword set: the rider's (or the model's own), and the mount's apart.
type UnitKeywords struct {
	Model     []UnitKeyword // race, faction, then unit types, each group in the order of the vanilla file, no duplicates
	Mount     string        // "Horse"; empty when there is none
	MountKeys []UnitKeyword // the mount's own keywords (a faction of "See Rider" replaced by the rider's)
}

// Empty says there is nothing to show.
func (k UnitKeywords) Empty() bool { return len(k.Model) == 0 && k.Mount == "" }

// Row is the compact row's chips: Model without Hidden and Off, at most max (max <= 0 means all), and how many more there are.
func (k UnitKeywords) Row(max int) (shown []UnitKeyword, more int) {
	for _, w := range k.Model {
		if w.Hidden || w.Off {
			continue
		}
		shown = append(shown, w)
	}
	if max > 0 && len(shown) > max {
		more = len(shown) - max
		shown = shown[:max]
	}
	return shown, more
}

// OnFoot is the set of a rider whose mount is lost: Cavalry marked Off, no mount block. A set without a mount is returned as it is.
func (k UnitKeywords) OnFoot() UnitKeywords {
	if k.Mount == "" {
		return k
	}
	out := UnitKeywords{Model: slices.Clone(k.Model)}
	for i := range out.Model {
		if out.Model[i].Kind == KeywordType && strings.EqualFold(out.Model[i].Name, "Cavalry") {
			out.Model[i].Off = true
		}
	}
	return out
}

// Group is the model's keywords of one kind (the full list's three groups).
func (k UnitKeywords) Group(kind KeywordKind) []UnitKeyword {
	return slices.DeleteFunc(slices.Clone(k.Model), func(w UnitKeyword) bool { return w.Kind != kind })
}

// UnitKeywordsInput says which model to describe.
type UnitKeywordsInput struct {
	Type     string // CompanyUnit.Type: the vanilla unit name or the company's label
	Role     UnitRole
	Mount    string // name of the mount the unit has (bought, or a composite's mount part); empty if none
	Wanderer string // wanderer offer name, empty otherwise
	Creature string // creature offer name, empty otherwise
}

// keywordSpellings merges the spellings of the same keyword the vanilla file uses (the JSON stays untouched): lower-cased spelling to
// the display one. A value of "" drops the word (a placeholder).
var keywordSpellings = map[string]string{
	"khazad-dum":          "Khazad-dum",
	"lothlorien":          "Lothlórien",
	"lothlórien":          "Lothlórien",
	"the iron hills":      "Iron Hills",
	"iron hills":          "Iron Hills",
	"the misty mountains": "Misty Mountains",
	"misty mountains":     "Misty Mountains",
	"misty mountain":      "Misty Mountains", // the Great Eagle's spelling
	"hero":                "Hero",
	"*":                   "",
	"none":                "",
}

// normalizeKeywords is the keyword words of one list: split where one string names several factions ("Fellowship, Dunedain"), the
// spelling variants merged, placeholders dropped, duplicates (ignoring case) removed, the order kept.
func normalizeKeywords(in []string) []string {
	var out []string
	for _, raw := range in {
		for _, w := range strings.Split(raw, ",") {
			w = strings.TrimSpace(w)
			if to, ok := keywordSpellings[strings.ToLower(w)]; ok {
				w = to
			}
			if w == "" || slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, w) }) {
				continue
			}
			out = append(out, w)
		}
	}
	return out
}

// collectRaces records the race words of every profile, lower-cased.
func (b *VanillaBook) collectRaces() {
	b.races = map[string]bool{}
	for _, u := range b.units {
		for _, r := range normalizeKeywords(u.Race) {
			b.races[strings.ToLower(r)] = true
		}
	}
}

// IsRace says whether the word is the race of some vanilla profile (Man, Goblin, Elf).
func (b *VanillaBook) IsRace(word string) bool { return b != nil && b.races[strings.ToLower(word)] }

// keywordSet collects keywords, dropping repeats within a kind.
type keywordSet struct {
	race, faction, types []UnitKeyword
}

func (s *keywordSet) add(kind KeywordKind, name, source string) {
	list := &s.types
	switch kind {
	case KeywordRace:
		list = &s.race
	case KeywordFaction:
		list = &s.faction
	}
	for _, n := range normalizeKeywords([]string{name}) {
		if !slices.ContainsFunc(*list, func(k UnitKeyword) bool { return strings.EqualFold(k.Name, n) }) {
			*list = append(*list, UnitKeyword{Name: n, Kind: kind, Source: source})
		}
	}
}

func (s *keywordSet) all() []UnitKeyword {
	out := slices.Concat(s.race, s.faction, s.types)
	if len(out) == 0 {
		return nil
	}
	return out
}

// keywordsOfProfile adds a vanilla profile's race, faction and unit types.
func (s *keywordSet) addProfile(v VanillaUnit, types []string, source string) {
	for _, r := range v.Race {
		s.add(KeywordRace, r, source)
	}
	for _, f := range v.Faction {
		s.add(KeywordFaction, f, source)
	}
	for _, t := range types {
		s.add(KeywordType, t, source)
	}
}

// UnitKeywords computes the keyword set of a model of a company. It cannot fail: an unknown unit gives the empty value.
func (r *RuleSet) UnitKeywords(p CompanyProfile, in UnitKeywordsInput) UnitKeywords {
	if r == nil || r.Vanilla == nil {
		return UnitKeywords{}
	}
	var set keywordSet
	switch {
	case in.Wanderer != "":
		set = r.wandererKeywords(in.Wanderer)
	case in.Creature != "":
		if o, ok := creatureOffer(r, in.Creature); ok {
			for _, t := range o.UnitTypes {
				set.add(KeywordType, t, KeywordFromProfile)
			}
		}
	default:
		v, alias, ok := resolveUnit(r, p, in.Type)
		if !ok {
			return UnitKeywords{}
		}
		if len(v.Race)+len(v.Faction)+len(v.UnitTypes) == 0 && len(v.Includes) > 0 { // a bundle (Vault Warden Team) is its first member
			if m, ok := r.Vanilla.Unit(v.Includes[0]); ok {
				v = VanillaUnit{Name: v.Name, Race: m.Race, Faction: m.Faction, UnitTypes: m.UnitTypes, Includes: v.Includes}
			}
		}
		source := KeywordFromProfile
		if slices.ContainsFunc(r.BC.fillIns, func(f unitFillIn) bool { return f.unit == v.Name }) {
			source = KeywordFromFillIn // the Battle Companies file gave this profile its race, faction and types
		}
		set.addProfile(v, nil, source)
		for _, t := range p.UnitTypes(in.Type, v.UnitTypes) {
			src := source
			if !slices.Contains(v.UnitTypes, t) {
				src = KeywordFromCompany
			}
			set.add(KeywordType, t, src)
		}
		if alias != nil {
			for _, w := range alias.Keywords {
				if r.Vanilla.IsRace(w) {
					set.add(KeywordRace, w, KeywordFromCompany)
				} else {
					set.add(KeywordFaction, w, KeywordFromCompany)
				}
			}
		}
	}
	out := UnitKeywords{}
	if in.Mount != "" {
		set.add(KeywordType, "Cavalry", KeywordFromMount)
		out.Mount = in.Mount
		if g, ok := r.Vanilla.Gear(in.Mount); ok {
			var mount keywordSet
			for _, race := range g.Race {
				mount.add(KeywordRace, race, KeywordFromMount)
			}
			for _, f := range g.Faction {
				if strings.EqualFold(strings.TrimSpace(f), seeRider) {
					for _, rf := range set.faction {
						mount.add(KeywordFaction, rf.Name, KeywordFromMount)
					}
					continue
				}
				mount.add(KeywordFaction, f, KeywordFromMount)
			}
			for _, t := range g.UnitTypes {
				mount.add(KeywordType, t, KeywordFromMount)
			}
			out.MountKeys = r.describeKeywords(mount.all())
		}
	}
	out.Model = r.describeKeywords(set.all())
	return out
}

// wandererKeywords is a wanderer's set: the race word its unit types hold (Man) goes to the race group.
func (r *RuleSet) wandererKeywords(name string) keywordSet {
	var set keywordSet
	_, offers := r.BC.Wanderers()
	for _, w := range offers {
		if w.Name != name {
			continue
		}
		for _, t := range w.UnitTypes {
			if r.Vanilla.IsRace(t) {
				set.add(KeywordRace, t, KeywordFromProfile)
			}
		}
		for _, t := range w.UnitTypes {
			if !r.Vanilla.IsRace(t) {
				set.add(KeywordType, t, KeywordFromProfile)
			}
		}
	}
	return set
}

// describeKeywords sets the hidden flag, the tooltip text and the concept of each keyword.
func (r *RuleSet) describeKeywords(in []UnitKeyword) []UnitKeyword {
	for i := range in {
		k := &in[i]
		k.Hidden = k.Kind == KeywordType && slices.ContainsFunc(hiddenTypes, func(h string) bool { return strings.EqualFold(h, k.Name) })
		k.Concept = "Keywords"
		term, ok := r.Glossary.Find(k.Name)
		if k.Kind != KeywordType { // a race or faction word has its own glossary term, named "Elf keyword" so that the linker leaves "elf" alone
			if t, found := r.Glossary.Find(k.Name + " keyword"); found {
				term, ok = t, true
			}
		}
		if ok {
			k.Concept, k.Text = term.Name, term.Summary(300)
		}
		if k.Text == "" {
			k.Text = generatedKeywordText(k.Name, k.Kind)
		}
	}
	return in
}

func generatedKeywordText(name string, kind KeywordKind) string {
	what := map[KeywordKind]string{KeywordRace: "a race keyword", KeywordFaction: "a faction keyword", KeywordType: "a unit type keyword"}[kind]
	return name + " is " + what + ". Rules that name it (Hatred (" + name + "), Ancient Enemies (" + name + ")) apply to models that have it."
}

// withAliasKeywords is the profile a company's alias stands for with the keywords it adds (Goblin Hulk: Goblin), as company limits
// count them.
func withAliasKeywords(rs *RuleSet, v VanillaUnit, a *UnitAlias) VanillaUnit {
	if a == nil || len(a.Keywords) == 0 {
		return v
	}
	v.Race, v.Faction = slices.Clone(v.Race), slices.Clone(v.Faction)
	for _, w := range a.Keywords {
		if rs.Vanilla.IsRace(w) {
			v.Race = append(v.Race, w)
		} else {
			v.Faction = append(v.Faction, w)
		}
	}
	return v
}

// resolveUnitKeywords is resolveUnit with the keywords the alias adds, for the company limits.
func resolveUnitKeywords(rs *RuleSet, p CompanyProfile, name string) (VanillaUnit, *UnitAlias, bool) {
	v, a, ok := resolveUnit(rs, p, name)
	return withAliasKeywords(rs, v, a), a, ok
}

// withPreviewKeywords gives each unit preview of a profile its keyword set (no mount, no state: a preview is the unit as it is
// recruited). p.Previews must be its own map, as WithPreviews makes it.
func (r *RuleSet) withPreviewKeywords(p CompanyProfile) CompanyProfile {
	for key, pv := range p.Previews {
		pv.Keywords = r.UnitKeywords(p, UnitKeywordsInput{Type: pv.Name})
		p.Previews[key] = pv
	}
	return p
}
