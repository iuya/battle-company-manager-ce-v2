package core

import (
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// VanillaBook gives access to the rules data of the vanilla file: what a unit's profile is, what a special rule or a piece
// of gear says. It is the part of the future rules loader that the company pages need; names are looked up first
// occurrence wins, like everywhere else (docs/definitions.md).
type VanillaBook struct {
	stats      []StatDef
	units      map[string]VanillaUnit
	keywords   map[string]string
	gear       map[string]VanillaGear
	powers     map[string]VanillaPower
	powerOrder []string        // the names of powers, in the order of the file (the variants of a power: Fury (Orc))
	races      map[string]bool // lower-cased race words of every profile (a wanderer's unit type "Man" is one); see IsRace

	occurrences map[string]int // dupKey(kind, name) -> how often the file defines the name (the lean validator's vanilla.pins check)

	// heroes are the names of every hero profile and available the armies each option is limited to (the availableIn of the
	// winning occurrence of a unit), by lower-cased "unit|option": what ReservedOptions derives from.
	heroes    map[string]bool
	available map[string][]string
}

// Occurrences says how many times the vanilla file defines a name of a kind ("unit", "gear", "keyword", "power"); 0 when it does not.
func (b *VanillaBook) Occurrences(kind, name string) int { return b.occurrences[dupKey(kind, name)] }

// StatDef is one attribute of a profile as the vanilla file declares them, in display order.
type StatDef struct {
	Key      string // the property of a unit: "movement", "fight"...
	Name     string
	Abbr     string
	Trail    string // printed after the value: " for Movement, + for Shoot, Courage and Intelligence
	HeroOnly bool   // after the separator of the vanilla list (Might, Will and Fate)
}

type VanillaUnit struct {
	Name          string
	Points        int
	Stats         map[string]int // by StatDef.Key
	UnitTypes     []string
	Race          []string // Man, Elf, Dwarf, Orc... : the keywords company limits count
	Faction       []string // Gondor, Dol Amroth, Shire...: the factions of the unit
	SpecialRules  []string
	HeroicActions []string
	Wargear       []string
	Includes      []string       // the units a bundle brings (Vault Warden Team: Iron Shield Bearer, Foe Spear Wielder); it carries their wargear
	Options       []ChoiceOption // what the profile lets a model take, with the points it costs
	// OptionKeys is parallel to Options: the choose-one group of each option (chooseOneKey, requireChooseOneKey), "" when it has none.
	// At most one option of a group is taken; a group with a required option must have one taken.
	OptionKeys    []OptionKey
	MagicalPowers []PowerRef
	BaseSize      []string // "25mm", "120mm oval", "180x140mm oval": not applied (the helper measures the figure)
}

// OptionKey is the choose-one group an option belongs to.
type OptionKey struct {
	Key      string
	Required bool // requireChooseOneKey: the model must take one option of the group
}

// PowerRef is a magical power a unit knows, with its range and casting value.
type PowerRef struct {
	Name    string
	Range   int
	Casting int
}

type VanillaGear struct {
	Name       string
	Type       string // the vanilla type: mount, bow, throwingWeapon, shield, armour, banner, ...
	Definition string
	Stats      map[string]int // a mount's profile, by StatDef.Key
	Rules      []string       // a mount's special rules
	Modifies   []StatChange   // modifiesStats without a condition: what carrying it adds to a characteristic (Helmingas: Strength +1)
	Race       []string       // a mount's own keywords (Horse, Warg, Fell Beast...)
	Faction    []string       // Rohan, Gundabad...; "See Rider" means the rider's
	UnitTypes  []string       // Mount, Chariot, Hero, Monster...
}

type VanillaPower struct {
	Name       string
	Duration   string
	Definition string
}

// ParseVanilla reads the parts of the vanilla file the book needs: first occurrence wins, nothing reviewed, nothing logged.
func ParseVanilla(content []byte) (*VanillaBook, error) {
	return ParseVanillaWith(content, nil, nil)
}

// ParseVanillaWith is ParseVanilla with the reviewed duplicates of the Battle Companies file (ParseVanillaAmbiguities): a
// name the vanilla file defines more than once resolves to its first occurrence unless a "pinned" entry names another one,
// and a duplicate whose occurrences differ and that nobody reviewed is logged as a warning (log may be nil).
func ParseVanillaWith(content []byte, reviewed []VanillaAmbiguity, log *slog.Logger) (*VanillaBook, error) {
	return parseVanilla(content, reviewed, log, false)
}

// parseVanilla is ParseVanillaWith; pinsOnly is set for a lean document, which reviews duplicates by pinning (vanilla.pins) only.
func parseVanilla(content []byte, reviewed []VanillaAmbiguity, log *slog.Logger, pinsOnly bool) (*VanillaBook, error) {
	doc, err := decodeVanilla(content)
	if err != nil {
		return nil, err
	}
	return doc.book(reviewed, log, pinsOnly), nil
}

// vanillaDoc is the vanilla file as far as the book reads it. It is read-only once decoded, so one decoded file can build several books
// (loadV2 builds one to check a Battle Companies file against and one with the duplicates that file pins).
type vanillaDoc struct {
	Stats []struct {
		Name   string `json:"name"`
		Abbr   string `json:"abbreviation"`
		Trail  string `json:"trail"`
		ShowIf string `json:"showIf"`
	} `json:"stats"`
	Data struct {
		Heroes   []map[string]json.RawMessage `json:"heroes"`
		Warriors []map[string]json.RawMessage `json:"warriors"`
		Gear     []map[string]json.RawMessage `json:"gear"`
		Keywords []struct {
			Name       string `json:"name"`
			Definition string `json:"definition"`
		} `json:"keywords"`
		Powers []struct {
			Name       string `json:"name"`
			Duration   string `json:"duration"`
			Definition string `json:"definition"`
		} `json:"magicalPowers"`
	} `json:"data"`
}

func decodeVanilla(content []byte) (*vanillaDoc, error) {
	var doc vanillaDoc
	if err := json.Unmarshal(content, &doc); err != nil {
		return nil, errors.Join(ErrInvalidRuleset, errors.New("the vanilla file is not valid JSON"))
	}
	return &doc, nil
}

// book builds the vanilla book from the decoded file; the arguments are those of parseVanilla.
func (doc *vanillaDoc) book(reviewed []VanillaAmbiguity, log *slog.Logger, pinsOnly bool) *VanillaBook {
	dups := newDuplicates(reviewed, log)
	dups.pinsOnly = pinsOnly
	defer dups.report()
	b := &VanillaBook{units: map[string]VanillaUnit{}, keywords: map[string]string{}, gear: map[string]VanillaGear{}, powers: map[string]VanillaPower{},
		heroes: map[string]bool{}, available: map[string][]string{}}
	for _, raw := range doc.Data.Heroes {
		var name string
		_ = json.Unmarshal(raw["name"], &name)
		b.heroes[name] = true
	}
	heroOnly := false
	for _, s := range doc.Stats {
		if s.Name == "" { // the "|" separator between a profile and its hero attributes: what follows is for heroes only
			heroOnly = true
			continue
		}
		b.stats = append(b.stats, StatDef{Key: strings.ToLower(s.Name), Name: s.Name, Abbr: s.Abbr, Trail: s.Trail, HeroOnly: heroOnly})
	}
	for _, raw := range slices.Concat(doc.Data.Heroes, doc.Data.Warriors) {
		var name string
		_ = json.Unmarshal(raw["name"], &name)
		if name == "" || !dups.wins("unit", name, raw) {
			continue
		}
		b.addUnit(name, raw)
	}
	for _, raw := range doc.Data.Gear {
		g := VanillaGear{Stats: map[string]int{}}
		_ = json.Unmarshal(raw["name"], &g.Name)
		_ = json.Unmarshal(raw["type"], &g.Type)
		_ = json.Unmarshal(raw["definition"], &g.Definition)
		_ = json.Unmarshal(raw["race"], &g.Race)
		_ = json.Unmarshal(raw["faction"], &g.Faction)
		_ = json.Unmarshal(raw["unitType"], &g.UnitTypes)
		_ = json.Unmarshal(raw["specialRules"], &g.Rules)
		var mods []map[string]json.RawMessage
		_ = json.Unmarshal(raw["modifiesStats"], &mods)
		for _, m := range mods {
			var c StatChange
			_ = json.Unmarshal(m["stat"], &c.Stat)
			_ = json.Unmarshal(m["modifier"], &c.Delta)
			for k := range m { // any key besides the stat and its modifier is a condition (cancelledBy, whileEngaged...): a note, never a change
				if k != "stat" && k != "modifier" {
					c.Condition = "when " + k + " allows"
				}
			}
			g.Modifies = append(g.Modifies, c)
		}
		for _, s := range b.stats {
			var v int
			if raw[s.Key] != nil && json.Unmarshal(raw[s.Key], &v) == nil {
				g.Stats[s.Key] = v
			}
		}
		if dups.wins("gear", g.Name, raw) {
			b.gear[NameKey(g.Name)] = g
		}
	}
	for _, k := range doc.Data.Keywords {
		if dups.wins("keyword", k.Name, k) {
			b.keywords[k.Name] = k.Definition
		}
	}
	for _, p := range doc.Data.Powers {
		if dups.wins("power", p.Name, p) {
			b.powers[p.Name] = VanillaPower{p.Name, p.Duration, p.Definition}
			b.powerOrder = append(b.powerOrder, p.Name)
		}
	}
	b.collectRaces()
	b.occurrences = make(map[string]int, len(dups.seen))
	for key, st := range dups.seen {
		b.occurrences[key] = st.count
	}
	return b
}

// addUnit reads one unit of the vanilla shape (the flat characteristics, race, faction, unitType, specialRules, wargear, options...) into the book. The
// vanilla file and a brand-new unit of a Battle Companies file (RegisterLeanUnits) share this reader, so both give the same VanillaUnit.
func (b *VanillaBook) addUnit(name string, raw map[string]json.RawMessage) {
	b.units[name] = b.readUnit(name, raw)
}

// readUnit is addUnit without storing the unit: a company-local brand-new unit is kept by its company, not in the book.
func (b *VanillaBook) readUnit(name string, raw map[string]json.RawMessage) VanillaUnit {
	u := VanillaUnit{Name: name, Stats: map[string]int{}}
	_ = json.Unmarshal(raw["points"], &u.Points)
	_ = json.Unmarshal(raw["unitType"], &u.UnitTypes)
	_ = json.Unmarshal(raw["race"], &u.Race)
	_ = json.Unmarshal(raw["faction"], &u.Faction)
	_ = json.Unmarshal(raw["specialRules"], &u.SpecialRules)
	_ = json.Unmarshal(raw["heroicActions"], &u.HeroicActions)
	_ = json.Unmarshal(raw["wargear"], &u.Wargear)
	_ = json.Unmarshal(raw["baseSize"], &u.BaseSize)
	var includes []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw["includes"], &includes)
	for _, in := range includes {
		u.Includes = append(u.Includes, in.Name)
	}
	var options []struct {
		Name      string   `json:"name"`
		Points    int      `json:"points"`
		Choose    string   `json:"chooseOneKey"`
		Require   string   `json:"requireChooseOneKey"`
		Available []string `json:"availableIn"`
	}
	_ = json.Unmarshal(raw["options"], &options)
	for _, o := range options {
		u.Options = append(u.Options, ChoiceOption{Name: o.Name, Points: o.Points})
		if len(o.Available) > 0 {
			b.available[reservedKey(name, o.Name)] = o.Available
		}
		key := OptionKey{Key: o.Choose}
		if o.Require != "" {
			key = OptionKey{Key: o.Require, Required: true}
		}
		u.OptionKeys = append(u.OptionKeys, key)
	}
	var powers []struct {
		Name    string `json:"name"`
		Range   int    `json:"range"`
		Casting int    `json:"casting"`
	}
	_ = json.Unmarshal(raw["magicalPowers"], &powers)
	for _, p := range powers {
		u.MagicalPowers = append(u.MagicalPowers, PowerRef{p.Name, p.Range, p.Casting})
	}
	for _, s := range b.stats {
		var v int
		if json.Unmarshal(raw[s.Key], &v) == nil && raw[s.Key] != nil {
			u.Stats[s.Key] = v
		}
	}
	return u
}

// Stats are the attributes the vanilla file declares, in display order.
func (b *VanillaBook) Stats() []StatDef { return b.stats }

// StatKeys are the keys of the characteristics, in display order: the vocabulary of every list of stats.
func (b *VanillaBook) StatKeys() []string {
	keys := make([]string, len(b.stats))
	for i, s := range b.stats {
		keys[i] = s.Key
	}
	return keys
}

// HeroStat says whether the characteristic belongs to heroes only (Might, Will and Fate).
func (b *VanillaBook) HeroStat(key string) bool {
	return slices.ContainsFunc(b.stats, func(s StatDef) bool { return s.Key == key && s.HeroOnly })
}

// HeroStats are the heroes-only characteristics by key.
func (b *VanillaBook) HeroStats() map[string]bool {
	out := map[string]bool{}
	for _, s := range b.stats {
		if s.HeroOnly {
			out[s.Key] = true
		}
	}
	return out
}

// HeroProfile says whether a profile (by stat key) has a hero characteristic: the vanilla heroes and the warriors alike are told apart by it.
func (b *VanillaBook) HeroProfile(stats map[string]int) bool {
	return slices.ContainsFunc(b.stats, func(s StatDef) bool { return s.HeroOnly && stats[s.Key] > 0 })
}

func (b *VanillaBook) Unit(name string) (VanillaUnit, bool) { u, ok := b.units[name]; return u, ok }

// Gear looks an item up by name, ignoring case ("Heavy Armour" and "Heavy armour" are the same item).
func (b *VanillaBook) Gear(name string) (VanillaGear, bool) {
	g, ok := b.gear[NameKey(name)]
	return g, ok
}

// HeroicActionNames lists every heroic action the vanilla profiles have, sorted: what a hero may learn.
func (b *VanillaBook) HeroicActionNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, u := range b.units {
		for _, a := range u.HeroicActions {
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	sort.Strings(out)
	return out
}

// MountProfile is the stat block of a mount from its vanilla entry (Shoot is only a placeholder for a mount, so it is left out).
func (b *VanillaBook) MountProfile(name string) []Attribute {
	g, ok := b.Gear(name)
	if !ok || len(g.Stats) == 0 {
		return nil
	}
	var out []Attribute
	for _, s := range b.stats {
		v, has := g.Stats[s.Key]
		if !has || s.HeroOnly || s.Key == "shoot" {
			continue
		}
		out = append(out, Attribute{Abbr: s.Abbr, Name: s.Name, Value: v, Trail: s.Trail})
	}
	return out
}

func (b *VanillaBook) Power(name string) (VanillaPower, bool) {
	p, ok := b.powers[name]
	return p, ok
}

var parameter = regexp.MustCompile(`\s*\(.*\)\s*$`)

// Keyword returns the definition of a special rule; a parameter in brackets ("Terror (2)") is ignored for the lookup.
func (b *VanillaBook) Keyword(name string) (string, bool) {
	if d, ok := b.keywords[name]; ok {
		return d, true
	}
	d, ok := b.keywords[parameter.ReplaceAllString(name, "")]
	return d, ok
}

// Profile builds the attributes of a unit from its vanilla profile and the company's bonuses (by StatDef.Key). The hero
// attributes (Might, Will, Fate) are only included for heroes.
func (b *VanillaBook) Profile(u VanillaUnit, hero bool, bonus map[string]int) []Attribute {
	var out []Attribute
	var heroStats []Attribute // Might, Will and Fate: shown only when a hero has some
	for _, s := range b.stats {
		if s.HeroOnly && !hero {
			continue
		}
		base, has := u.Stats[s.Key]
		extra := bonus[s.Key]
		if !has && extra == 0 && !s.HeroOnly {
			continue
		}
		a := Attribute{Abbr: s.Abbr, Name: s.Name, Value: base + extra, Trail: s.Trail, Boost: extra, HeroOnly: s.HeroOnly}
		if s.HeroOnly {
			heroStats = append(heroStats, a)
		} else {
			out = append(out, a)
		}
	}
	for _, a := range heroStats {
		if a.Value != 0 {
			return append(out, heroStats...)
		}
	}
	return out
}
