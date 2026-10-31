package core

import (
	"slices"
	"strings"
)

// BaseUnit is the card of a unit of the ruleset as the campaign glossary shows it: the profile a company recruits, before any
// wargear is bought or experience earned. One per distinct unit name the companies use.
type BaseUnit struct {
	// ID is the anchor of the card in the glossary (UnitAnchor): stable for a unit name, so a link can name it.
	ID   string
	Name string
	// Base is the vanilla profile the unit is when a company gives it a name of its own (Helminga: Rohan Royal Guard); empty otherwise.
	Base string
	// Text is the company's own words about the unit (an alias with a text), empty when it has none.
	Text    string
	Preview UnitPreview
	// Includes are the units a bundle brings (the Vault Warden Team has no profile of its own); empty for a single model.
	Includes []string
	// Options is what the profile may take, with the points each costs (the options a company's own version already has are in the wargear).
	Options []ChoiceOption
	// Companies are the display names of the companies that name the unit, in the order of the file.
	Companies []string
}

// UnitAnchor is the fragment of a unit's card in the campaign glossary: "u-" and the NameKey of the name with dashes, so
// "Warrior of Minas Tirith" is "u-warrior-of-minas-tirith". Unit references link to /campaigns/{id}/glossary#<UnitAnchor>.
func UnitAnchor(name string) string {
	return "u-" + strings.ReplaceAll(NameKey(name), " ", "-")
}

// BaseUnits lists the card of every distinct unit the companies of the rule set name (starting company, charts, special units and
// promotions), sorted by name. Built once per rule set. A name two companies give to different profiles gets one card per profile, the
// later ones anchored with the company's name.
func (r *RuleSet) BaseUnits() []BaseUnit {
	r.baseOnce.Do(func() { r.baseUnits = r.buildBaseUnits() })
	return r.baseUnits
}

// GlossaryWithUnits is the rule set's glossary with the unit cards of its companies: what a campaign's Glossary tab shows.
func (r *RuleSet) GlossaryWithUnits() Glossary {
	g := r.Glossary
	g.Units = r.BaseUnits()
	return g
}

func (r *RuleSet) buildBaseUnits() []BaseUnit {
	if r == nil || r.BC == nil || r.Vanilla == nil {
		return nil
	}
	type seen struct {
		at   int
		sign string
	}
	var out []BaseUnit
	byKey := map[string][]seen{}
	for _, p := range r.BC.Profiles() {
		done := map[string]bool{}
		for _, m := range p.mentionedUnits() {
			if done[NameKey(m.unit)] {
				continue
			}
			done[NameKey(m.unit)] = true
			vu, alias, ok := r.ResolveUnit(p, m.unit)
			if !ok {
				continue
			}
			u := r.baseUnit(p, m.unit, vu, alias)
			sign := unitSignature(vu, alias)
			key := NameKey(m.unit)
			if i := slices.IndexFunc(byKey[key], func(s seen) bool { return s.sign == sign }); i >= 0 {
				bu := &out[byKey[key][i].at]
				bu.Companies = append(bu.Companies, p.DisplayName())
				continue
			}
			if len(byKey[key]) > 0 { // the same name, another profile: tell the cards apart by the company
				u.ID = UnitAnchor(m.unit + " " + p.Name)
			}
			byKey[key] = append(byKey[key], seen{len(out), sign})
			out = append(out, u)
		}
	}
	slices.SortStableFunc(out, func(a, b BaseUnit) int { return strings.Compare(NameKey(a.Name), NameKey(b.Name)) })
	return out
}

// unitSignature tells two readings of one name apart: the vanilla profile and what the company adds.
func unitSignature(vu VanillaUnit, a *UnitAlias) string {
	s := vu.Name
	if a != nil {
		s += "|" + strings.Join(a.AddRules, ",") + "|" + strings.Join(a.Selected, ",") + "|" + a.Text
	}
	return s
}

func (r *RuleSet) baseUnit(p CompanyProfile, name string, vu VanillaUnit, alias *UnitAlias) BaseUnit {
	u := BaseUnit{ID: UnitAnchor(name), Name: name, Includes: slices.Clone(vu.Includes), Companies: []string{p.DisplayName()}}
	pv := UnitPreview{Key: PreviewKey(name, nil), Name: name, Points: vu.Points, Types: vu.UnitTypes, Kind: KindOf(vu.UnitTypes),
		Wargear: slices.Clone(vu.Wargear), Rules: slices.Clone(vu.SpecialRules), HeroicActions: vu.HeroicActions}
	pv.Stats = r.Vanilla.Profile(vu, r.Vanilla.HeroProfile(vu.Stats), nil)
	var selected []string
	if alias != nil {
		if !sameName(alias.Unit, name) {
			u.Base = alias.Unit
		}
		u.Text = alias.Text
		pv.Rules = append(pv.Rules, alias.AddRules...)
		selected = alias.Selected
	}
	for _, o := range vu.Options {
		if slices.ContainsFunc(selected, func(s string) bool { return sameName(s, o.Name) }) { // already part of the company's unit
			pv.Points += o.Points
			pv.Wargear = append(pv.Wargear, o.Name)
			continue
		}
		u.Options = append(u.Options, o)
	}
	pv.Keywords = r.UnitKeywords(p, UnitKeywordsInput{Type: name})
	u.Preview = pv
	return u
}
