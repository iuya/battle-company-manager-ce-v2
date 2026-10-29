package core

import (
	"fmt"
	"slices"
	"strings"
)

// Consumables of the aftermath (Healing Herbs, Wondrous Cram): see docs/aftermath.md. Everything about them comes from
// campaign.aftermathEffects (which item activates which effect, who may elect it, the modifier, the clamp); nothing here
// names an item.

// LoanedItem is an item a unit carries for this match only, from a Healer's Kit (Against the Odds). It can be elected like a carried one and
// is simply spent; it never reaches the roster.
type LoanedItem struct {
	UnitID int64
	Item   string // the equipment name ("Healing Herbs")
	// The rest is for the live match: the kit it came from, the weapon of an Envenom Weapon, which copy of the item in the kit it is,
	// whether it acts only in the aftermath (Healing Herbs, Wondrous Cram) and whether it has been used (the fold fills Used).
	Kit       string
	Detail    string
	Index     int
	AfterGame bool
	Used      bool
}

// ItemEffect finds the consumable effect an equipment item activates.
func (r AftermathRules) ItemEffect(item string) (RewardEffect, bool) {
	key, ok := r.ItemEffects[NameKey(item)]
	if !ok {
		return RewardEffect{}, false
	}
	e, ok := r.Effects[key]
	return e, ok
}

// itemName is the equipment name that activates the effect.
func (r AftermathRules) itemName(effect string) string {
	best := ""
	for name, key := range r.ItemEffects {
		if key == effect && (best == "" || name < best) {
			best = name
		}
	}
	if best == "" {
		return effect
	}
	return titleCase(best)
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = Capitalise(w)
	}
	return strings.Join(words, " ")
}

// noRoll says whether the elected effect replaces the injury roll.
func (r AftermathRules) noRoll(effect string) bool {
	e, ok := r.Effects[effect]
	return ok && e.Kind == kindRollReplacement && e.NoRoll
}

// carries says whether the unit has the item in its equipment or loaned for this match.
func carries(u CompanyUnit, loaned []LoanedItem, item string) (has, isLoaned bool) {
	for _, e := range u.Equipment {
		if sameName(e.Name, item) {
			return true, false
		}
	}
	for _, l := range loaned {
		if l.UnitID == u.ID && sameName(l.Item, item) {
			return true, true
		}
	}
	return false, false
}

// itemRefusal says why the unit, with this sheet, may not elect the effect ("" when it may).
func itemRefusal(eff RewardEffect, rules AftermathRules, u CompanyUnit, m ModelSheet) string {
	name := rules.itemName(eff.Key)
	if eff.Carrier == "hero" && !u.IsHero() {
		return fmt.Sprintf("%s: only a Hero can use %s", u.Title(), name)
	}
	for _, req := range eff.Requires {
		switch req {
		case participatedKey:
			if !m.Took() {
				return fmt.Sprintf("%s did not take part, so it cannot use %s", u.Title(), name)
			}
		case "casualty":
			if m.Outcome != OutcomeCasualty {
				return fmt.Sprintf("%s can use %s only as a casualty", u.Title(), name)
			}
		case "notCasualty":
			if m.Outcome == OutcomeCasualty {
				return fmt.Sprintf("%s is a casualty and cannot use %s", u.Title(), name)
			}
		}
	}
	return ""
}

// ItemChoices lists, per unit of the sheet, the consumable effects it may elect right now: it carries (or is loaned) the
// item, and its outcome, role and participation satisfy the effect's activatedBy. Pure; the page shows the chips from it and
// Validate refuses what it does not list. Stacking is not considered here (the page greys out the second Healing Herbs).
func (in AftermathInput) ItemChoices(roster []CompanyUnit, loaned []LoanedItem, rules AftermathRules) map[int64][]string {
	out := map[int64][]string{}
	for _, m := range in.Models {
		u, ok := rosterUnit(roster, m.UnitID)
		if !ok {
			continue
		}
		var keys []string
		for _, e := range u.Equipment {
			keys = appendEffect(keys, rules, u, m, e.Name, true)
		}
		for _, l := range loaned {
			if l.UnitID == u.ID {
				keys = appendEffect(keys, rules, u, m, l.Item, true)
			}
		}
		if len(keys) > 0 {
			slices.Sort(keys)
			out[m.UnitID] = slices.Compact(keys)
		}
	}
	return out
}

func appendEffect(keys []string, rules AftermathRules, u CompanyUnit, m ModelSheet, item string, _ bool) []string {
	eff, ok := rules.ItemEffect(item)
	if !ok || itemRefusal(eff, rules, u, m) != "" {
		return keys
	}
	return append(keys, eff.Key)
}

// validateItems checks every election of a cleaned sheet.
func validateItems(in AftermathInput, roster []CompanyUnit, loaned []LoanedItem, rules AftermathRules) []error {
	var errs []error
	count := map[string]int{}
	for _, m := range in.Models {
		if m.UsesItem == "" {
			continue
		}
		u, _ := rosterUnit(roster, m.UnitID)
		eff, ok := rules.Effects[m.UsesItem]
		if !ok || (eff.Kind != kindRollModifier && eff.Kind != kindRollReplacement) {
			errs = append(errs, fmt.Errorf("%s: %q is not an item that can be used", u.Title(), m.UsesItem))
			continue
		}
		name := rules.itemName(eff.Key)
		var has bool
		for item, key := range rules.ItemEffects {
			if key != eff.Key {
				continue
			}
			if h, _ := carries(u, loaned, item); h {
				has = true
			}
		}
		switch {
		case !has:
			errs = append(errs, fmt.Errorf("%s does not carry %s", u.Title(), name))
		default:
			if why := itemRefusal(eff, rules, u, m); why != "" {
				errs = append(errs, fmt.Errorf("%s", why))
			}
		}
		count[eff.Key]++
	}
	for key, n := range count {
		if n > 1 && !rules.Effects[key].Stackable {
			errs = append(errs, fmt.Errorf("%s do not stack: only one can be used", rules.itemName(key)))
		}
	}
	return errs
}

// RollModifier is what the elected consumables add to every injury roll and the range the modified total is held to; the zero
// value (no modifier) when none is elected.
type RollModifier struct {
	Effect   string
	Amount   int
	Min, Max int
}

// Apply returns the modified total (held to the clamp when the effect has one) and the modifier shown beside the dice.
func (m RollModifier) Apply(total int) (modified, shown int) {
	if m.Amount == 0 {
		return total, 0
	}
	modified = total + m.Amount
	if m.Max != 0 {
		modified = max(m.Min, min(m.Max, modified))
	}
	return modified, m.Amount
}

// RollModifierOf works out the modifier the sheet elects: the stackable effects add up, the others count once.
func RollModifierOf(rules AftermathRules, in AftermathInput) RollModifier {
	var out RollModifier
	seen := map[string]bool{}
	for _, m := range in.Models {
		eff, ok := rules.Effects[m.UsesItem]
		if m.UsesItem == "" || !ok || eff.Kind != kindRollModifier || (seen[eff.Key] && !eff.Stackable) {
			continue
		}
		seen[eff.Key] = true
		out.Effect = eff.Key
		out.Amount += eff.Modifier
		out.Min, out.Max = eff.ClampMin, eff.ClampMax
	}
	return out
}

// itemLines are the preview lines of the elected consumables.
func itemLines(ctx AftermathContext, in AftermathInput) []AmountLine {
	var out []AmountLine
	if mod := RollModifierOf(ctx.Rules, in); mod.Amount != 0 {
		out = append(out, AmountLine{Text: fmt.Sprintf("%s: %+d to every injury roll", ctx.Rules.itemName(mod.Effect), mod.Amount), Amount: mod.Amount, Key: mod.Effect})
	}
	for _, m := range in.Models {
		if !ctx.Rules.noRoll(m.UsesItem) {
			continue
		}
		u, _ := rosterUnit(ctx.Roster, m.UnitID)
		out = append(out, AmountLine{Text: fmt.Sprintf("%s on %s: no roll", ctx.Rules.itemName(m.UsesItem), u.Title()), UnitID: m.UnitID, Key: m.UsesItem})
	}
	return out
}
