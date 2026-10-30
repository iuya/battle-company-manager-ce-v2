package core

import (
	"slices"
	"strconv"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// InjuryDef is a hero injury of the Battle Companies injury chart, as the rules file describes its effects. Only the
// injuries that stay with a hero (arm wound, leg wound, broken honour) are counted per hero; the others happen at once.
type InjuryDef struct {
	Key, Name, Text  string
	Persistent       bool
	RetireAt         int              // forced retirement at this count (0: never)
	StatPerCount     map[string]int   // stat key -> change per count (movement: -1)
	DisableClasses   []EquipmentClass // gear of these classes gives the hero no benefit
	DisableItems     []string         // and these named items
	DisableAbilities []string         // abilities lost ("standFast", "heroicActionsOnCompany")
	GrantsAt         map[int]string   // count -> special rule granted from that count on (Fearful)
	HealableBy       []string         // treatment methods that can remove it, for statuses like T'is Just a Scratch
}

// TreatMethod is one way of paying influence to treat a hero's injuries (Battle Companies, Step 3).
type TreatMethod struct {
	Key          string
	Cost         int
	TreatDice    DiceSpec // the dice of the roll (roll.dice); Target is a total of them
	Target       int      // the roll must reach it (0: no roll)
	Ceiling      int      // the modified roll never goes above it
	PerPoint     int      // roll gained per extra influence point
	Healable     []string
	MissNextGame bool // the hero misses the next game
	Text         string
}

// Rolls is true for a treatment that needs a roll.
func (m TreatMethod) Rolls() bool { return m.Target > 0 }

// InjuryRules are the injury and treatment rules of the Battle Companies file.
type InjuryRules struct {
	WarriorCost int
	Hero        []InjuryDef
	Attempt     TreatMethod
	Healers     TreatMethod
}

// Def finds an injury by key.
func (r InjuryRules) Def(key string) (InjuryDef, bool) {
	i := slices.IndexFunc(r.Hero, func(d InjuryDef) bool { return d.Key == key })
	if i < 0 {
		return InjuryDef{}, false
	}
	return r.Hero[i], true
}

// Method finds a hero treatment by key ("attempt" or "healers").
func (r InjuryRules) Method(key string) (TreatMethod, bool) {
	switch key {
	case r.Attempt.Key:
		return r.Attempt, r.Attempt.Key != ""
	case r.Healers.Key:
		return r.Healers, r.Healers.Key != ""
	}
	return TreatMethod{}, false
}

// UnitInjury is a persistent injury a hero carries and how many times it suffered it.
type UnitInjury struct {
	Key, Name, Text string
	Count           int
}

// Treatment is what the player asks for when treating a unit's injuries.
type Treatment struct {
	Method  string // "warrior", "attempt" or "healers"
	Injury  string // the injury key to remove (heroes)
	Roll    int    // the total the player rolled with the method's dice; 0 for the server to roll (attempt)
	Extra   int    // extra influence spent to raise the roll (attempt)
	Thrown  bool   // the browser threw the dice for Roll (Shake the table)
	Resolve bool   // answer a roll that waits: Extra is the influence the player spends to raise it (0: accept the result)
}

// TreatResult says what a treatment did.
type TreatResult struct {
	Rolled  bool
	Roll    int   // the total of the dice
	Dice    []int // each die the server rolled (nil for a roll the player typed)
	Total   int   // with the extra influence
	Needed  int
	Healed  bool
	Pending bool // the roll failed but extra influence could make it succeed: the player decides
	Boost   int  // the extra influence that would do it
	Summary string
}

// readInjuries reads the injury chart and the treatments of the campaign section of a lean document. The document is validated
// (lean.Decode): the treatment rolls are checked against their dice there.
func readInjuries(inj lean.Injuries) InjuryRules {
	rules := InjuryRules{WarriorCost: inj.Treatment.Warrior.Cost}
	for _, h := range inj.Hero {
		d := InjuryDef{Key: h.Key, Name: h.Name, Text: h.Text, Persistent: h.Persistent, RetireAt: h.RetireAtCount, HealableBy: h.HealableBy,
			StatPerCount: map[string]int{}, GrantsAt: map[int]string{}}
		for _, c := range h.StatChanges {
			if c.Per == "count" {
				d.StatPerCount[c.Stat] += c.Amount
			}
		}
		for _, x := range h.Disables {
			switch x.Kind {
			case "gearClass":
				for _, c := range x.Classes {
					d.DisableClasses = append(d.DisableClasses, EquipmentClass(c))
				}
			case "gearItem":
				d.DisableItems = append(d.DisableItems, x.Names...)
			case "ability":
				d.DisableAbilities = append(d.DisableAbilities, x.Key)
			}
		}
		for n, g := range h.GrantsAtCount {
			if count, err := strconv.Atoi(n); err == nil {
				d.GrantsAt[count] = g.SpecialRule
			}
		}
		rules.Hero = append(rules.Hero, d)
	}
	for _, m := range inj.Treatment.Hero {
		tm := TreatMethod{Key: m.Key, Cost: m.Cost, Healable: m.Healable, MissNextGame: m.MissNextGame, Text: m.Text}
		if r := m.Roll; r != nil {
			tm.Target, tm.PerPoint, tm.Ceiling, tm.TreatDice = r.Target, r.Modifier.PerPoint, r.Modifier.Ceiling, r.Dice
		}
		switch m.Key {
		case "attempt":
			rules.Attempt = tm
		case "healers":
			rules.Healers = tm
		}
	}
	return rules
}

// ApplyInjuries is what a hero's injury counts do to it: its stats change, gear and rules are lost or granted. It returns the
// stat bonus to add to the profile (movement -1 per leg wound), and later (ApplyToUnit) the gear and rules.
func (r InjuryRules) StatBonus(counts map[string]int) map[string]int {
	out := map[string]int{}
	for _, d := range r.Hero {
		for stat, delta := range d.StatPerCount {
			out[stat] += delta * counts[d.Key]
		}
	}
	return out
}

// ApplyToUnit marks the gear a hero's injuries disable, lists the injuries on the unit and grants the rules they give.
func (r InjuryRules) ApplyToUnit(u *CompanyUnit, counts map[string]int, keyword func(string) string) {
	for _, d := range r.Hero {
		n := counts[d.Key]
		if !d.Persistent || n == 0 {
			continue
		}
		u.Injuries = append(u.Injuries, UnitInjury{Key: d.Key, Name: d.Name, Text: d.Text, Count: n})
		for i := range u.Equipment {
			e := &u.Equipment[i]
			if e.Disabled != "" {
				continue
			}
			byClass := slices.ContainsFunc(e.Classes(), func(c EquipmentClass) bool { return slices.Contains(d.DisableClasses, c) })
			byName := slices.ContainsFunc(d.DisableItems, func(name string) bool { return sameName(name, e.Name) })
			if byClass || byName {
				e.Disabled = d.Name
			}
		}
		for at, rule := range d.GrantsAt {
			if n >= at && !slices.ContainsFunc(u.Rules, func(x UnitRule) bool { return sameName(x.Name, rule) }) {
				u.Rules = append(u.Rules, UnitRule{Name: rule, Description: keyword(rule), Source: RuleFromEquipment, From: d.Name})
			}
		}
		for _, a := range d.DisableAbilities {
			u.LostAbilities = append(u.LostAbilities, InjuryLoss{Ability: a, Injury: d.Name})
		}
	}
}

// The keys of the abilities an injury can take away (the `ability` keys of a disables entry in the data).
const (
	AbilityStandFast              = "standFast"
	AbilityHeroicActionsOnCompany = "heroicActionsOnCompany"
)

// Lost says whether an injury took the ability (by its data key) away from the unit.
func (u CompanyUnit) Lost(key string) bool {
	return slices.ContainsFunc(u.LostAbilities, func(l InjuryLoss) bool { return l.Ability == key })
}

// InjuryLoss is an ability (by its data key) a hero lost to an injury.
type InjuryLoss struct{ Ability, Injury string }

// Healable lists the injuries (keys) of a unit a method can remove: the persistent ones it carries, and the missed game of
// T'is Just a Scratch where the method allows it.
func (r InjuryRules) Healable(u CompanyUnit, m TreatMethod) []UnitInjury {
	var out []UnitInjury
	for _, in := range u.Injuries {
		if slices.Contains(m.Healable, in.Key) {
			out = append(out, in)
		}
	}
	if u.Injured && !u.AtHealers && slices.Contains(m.Healable, "justAScratch") {
		d, _ := r.Def("justAScratch")
		out = append(out, UnitInjury{Key: "justAScratch", Name: d.Name, Text: d.Text, Count: 1})
	}
	return out
}
