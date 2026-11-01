package core

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The goldens of testdata/snapshot were written when who may take an item, a hero upgrade or a granted rule had a field of its own
// each. They stay byte-identical: these views turn the typed lean.Who back into the lines they have, so the goldens prove the one
// selector admits the same models and says the same words.

// legacyReinforcement is a chart result without the scope and picks of a choice, which the goldens show in its words.
type legacyReinforcement struct {
	Type    string
	Text    string
	Unit    string
	Count   int
	Wargear []string
	Choice  bool
	Blocks  bool
	Rare    int
	Options []ChoiceUnit
	Alt     string
}

type legacyKind struct {
	Race       string
	HeroesOnly bool
}

type legacyOffer struct {
	Name           string
	AlsoCalled     []string
	Classes        []EquipmentClass
	Cost           int
	Size           string
	Restrictions   []string
	Gold           Rating
	Rating         Rating
	Description    string
	Grants         []string
	GrantedTo      string
	Stats          string
	Exclusive      bool
	Only           string
	Limit          int
	WeaponModifier bool
	OnlyUnits      []string
	OnlyKind       *legacyKind
	ArmourStage    int
}

type legacyUpgrade struct {
	Name        string
	Flavour     string
	Rule        string
	Text        string
	Restriction string
	Units       []string
}

type legacyGranted struct {
	Rule      string
	AppliesTo string
	Cost      int
	Text      string
}

// legacyKeyword is a keyword limit as the goldens show it, with its share as a percent.
type legacyKeyword struct {
	Keyword string
	Percent float64
	Text    string
}

type legacyStart struct{ Allowed, Excluded []string }

// legacyView is the value a golden shows for v: the old shape of the types that carry a Who, v itself for the others.
func legacyView(v reflect.Value) reflect.Value {
	if !v.IsValid() || !v.CanInterface() {
		return v
	}
	switch x := v.Interface().(type) {
	case GearOffer:
		l := legacyOffer{Name: x.Name, AlsoCalled: x.AlsoCalled, Classes: x.Classes, Cost: x.Cost, Size: x.Size, Restrictions: x.Restrictions, Gold: x.Gold, Rating: x.Rating,
			Description: x.Description, Grants: x.Grants, GrantedTo: x.GrantedTo, Stats: x.Stats, Exclusive: x.Exclusive, Limit: x.Limit, WeaponModifier: x.WeaponModifier, ArmourStage: x.ArmourStage}
		if x.namesWho() {
			l.Only, l.OnlyUnits = x.Who.Label(), x.Who.Units
			if len(x.Who.Units) == 0 && len(x.Who.Race) > 0 {
				l.OnlyKind = &legacyKind{Race: x.Who.Race[0], HeroesOnly: x.Who.Hero != nil && *x.Who.Hero}
			}
		}
		return reflect.ValueOf(l)
	case Reinforcement:
		if x.Type == "choice" { // the words of the row said "above" and "any 2"; the data now says it: the two must agree
			words := strings.ToLower(x.Text)
			if above := strings.Contains(words, "above"); above != (x.Scope == "above") || (strings.Contains(words, "any 2") != (x.Picks == 2)) || (x.Picks != 1 && x.Picks != 2) {
				panic(fmt.Sprintf("the choice %q has scope %q and picks %d", x.Text, x.Scope, x.Picks))
			}
		}
		return reflect.ValueOf(legacyReinforcement{Type: x.Type, Text: x.Text, Unit: x.Unit, Count: x.Count, Wargear: x.Wargear, Choice: x.Choice, Blocks: x.Blocks, Rare: x.Rare, Options: x.Options, Alt: x.Alt})
	case HeroUpgrade:
		var units []string
		if x.Who != nil {
			units = x.Who.Units
		}
		return reflect.ValueOf(legacyUpgrade{Name: x.Name, Flavour: x.Flavour, Rule: x.Rule, Text: x.Text, Restriction: x.Restriction(), Units: units})
	case lean.Share: // the goldens were written when a share was a percent number (50, 66, 33)
		return reflect.ValueOf(x.Percent())
	case KeywordLimit:
		return reflect.ValueOf(legacyKeyword{Keyword: x.Keyword, Percent: x.Share.Percent(), Text: x.Text})
	case GrantedRule:
		return reflect.ValueOf(legacyGranted{Rule: x.Rule, AppliesTo: x.AppliesTo(), Cost: x.Cost, Text: x.Text})
	}
	return v
}

// legacyField is the value of a struct field a golden shows: the starting heroes were an allowed and an excluded list.
func legacyField(name string, v reflect.Value) reflect.Value {
	if w, ok := v.Interface().(*lean.Who); ok && name == "StartHeroes" {
		return reflect.ValueOf(&legacyStart{Allowed: w.Units, Excluded: w.Except})
	}
	return legacyView(v)
}

func legacyUnits(w *lean.Who) []string {
	if w == nil {
		return nil
	}
	return w.Units
}
