package core

import (
	"maps"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// LeanBook builds the Battle Companies book from a lean document, the vanilla book (its fill-ins already applied, ApplyLeanFillIns) and
// the glossary. Every value comes from the typed document through the Lean* readers; nothing is read from the bytes of the file again.
func LeanBook(d *lean.Document, vb *VanillaBook, gl Glossary) (*BCBook, error) {
	b := &BCBook{companies: map[string]BCCompany{}, keywords: map[string]string{}, parameterised: map[string]string{}, annotations: map[string]GearAnnotation{},
		unsold: map[string]string{}, readAs: map[string]string{}, reserved: map[string]bool{}, xpGains: map[string]int{}}
	RegisterLeanUnits(vb, d) // the brand-new units are vanilla-shaped profiles from here on
	var err error
	var wargear WargearThreshold
	b.shape, wargear, b.limitRules = LeanCompanyRules(d)
	b.rating = LeanRating(d, wargear)
	b.rating.heroStats = vb.HeroStats()
	b.breakRules = LeanBreakRules(d)
	b.recruit = LeanRecruitRules(d)
	b.selection = LeanScenarioSelection(d)
	b.injuries = LeanInjuries(d)
	if b.ato, err = LeanATO(d, vb); err != nil {
		return nil, err
	}
	if b.aftermath, err = LeanAftermath(d, vb); err != nil {
		return nil, err
	}
	if v := d.Vanilla; v != nil { // what the file fills in for vanilla units and mounts (the audit tells these keywords from the profile's own)
		for _, name := range slices.Sorted(maps.Keys(v.Units)) {
			f := v.Units[name]
			b.fillIns = append(b.fillIns, unitFillIn{name, f.Race, f.Faction, f.UnitType})
		}
	}
	for _, g := range d.Campaign.Experience.Gains {
		b.xpGains[g.Key] = g.Amount
	}

	// the rules
	for _, r := range d.Rules {
		if r.Definition == "" {
			continue
		}
		if r.Parameter != nil {
			if def := strings.Join(strings.Fields(r.Definition), " "); def != "" {
				b.parameterised[NameKey(r.Name)] = def
			}
			continue
		}
		if _, seen := b.keywords[r.Name]; !seen {
			b.keywords[r.Name] = r.Definition
		}
	}
	b.labels = LeanRuleLabels(d.Rules)
	b.ruleTargets = LeanRuleTargets(d.Rules)
	b.auras = LeanAuraTable(d)
	b.ttsModels = LeanTTSModels(d, vb)
	if b.progression, err = LeanProgression(d, vb, gl); err != nil {
		return nil, err
	}
	b.pathRules = LeanPathRules(d, vb)

	// the gear and the companies
	gear := LeanGear(d, vb)
	spellVanillaWargear(vb, gear)
	b.armoury, b.annotations, b.armourUp = gear.Offers, gear.Annotations, gear.ArmourUp
	for _, g := range d.Gear { // the items that give a rule to a weapon the player chooses (Envenom Weapon (X))
		if a := LeanAnnotation(g, vb, true); a.ChosenWeapon != nil && !onlyTTS(g) {
			b.weaponRules = append(b.weaponRules, a)
		}
	}
	for _, c := range d.Companies {
		for _, g := range c.Gear {
			if a := LeanAnnotation(g, vb, false); a.ChosenWeapon != nil {
				b.weaponRules = append(b.weaponRules, a)
			}
		}
	}
	b.creatures, b.wanderers = gear.Creatures, gear.Wanderers
	b.creatureRules, b.wandererRules = LeanFollowerRules(d)
	set := LeanCompanies(d, vb, gear)
	for _, c := range set.Companies {
		b.companies[c.Name] = c
	}
	b.profiles, b.advancements = set.Profiles, set.Advancements
	for _, name := range slices.Sorted(mapKeys(b.companies)) {
		c := b.companies[name]
		if err := checkChart("company "+c.Name+" reinforcements", c.Reinforcements, b.recruit.Sides); err != nil {
			return nil, err
		}
		if len(c.SpecialChart) > 0 {
			if err := checkChart("company "+c.Name+" special chart", c.SpecialChart, b.recruit.Sides); err != nil {
				return nil, err
			}
		}
	}

	// the options of the vanilla units: sold as another item, not sold, or reserved to a named hero's army list
	sales := LeanOptionSales(d.Vanilla)
	for name, u := range vb.units {
		for _, o := range u.Options {
			key := reservedKey(name, o.Name)
			if item, ok := sales.ReadAs(name, o.Name); ok {
				b.readAs[key] = item
			} else if sales.Unsold(name, o.Name) {
				b.unsold[key] = sales.Reason(name, o.Name)
			}
		}
	}
	for _, r := range vb.ReservedOptions() {
		b.reserved[reservedKey(r.Unit, r.Option)] = true
	}

	if b.scenarios, err = LeanScenarios(d); err != nil {
		return nil, err
	}
	return b, nil
}

// spellVanillaWargear spells the wargear of the vanilla profiles as the armoury spells the same item ("Heavy armour" is the Heavy
// Armour), so that a chip reads alike whichever unit it comes from. The wargear the file writes is already in the armoury's spelling;
// vanilla's is not, and the vanilla book is shared, so its units are respelled once, when the book of a rule set is built.
func spellVanillaWargear(v *VanillaBook, gear LeanGearTable) {
	spell, byKey := map[string]string{}, map[string]string{}
	add := func(spelling, as string) {
		if k := strings.ToLower(strings.TrimSpace(spelling)); k != "" {
			if _, seen := spell[k]; !seen {
				spell[k] = as
			}
		}
	}
	offers := slices.Clone(gear.Offers)
	for _, local := range gear.Local {
		offers = append(offers, local...)
	}
	var names []string
	for _, o := range offers {
		add(o.Name, o.Name)
		names = append(names, o.Name)
	}
	for _, name := range slices.Sorted(maps.Keys(gear.Annotations)) { // sorted: the choice never depends on map order
		a := gear.Annotations[name]
		add(a.Name, a.Name)
		names = append(names, a.Name)
	}
	for _, o := range offers {
		for _, a := range o.AlsoCalled {
			add(a, o.Name)
		}
	}
	for _, n := range names { // the same item spelled another way (a plural, a missing bracket): vanilla's own typos
		if _, seen := byKey[ItemKey(n)]; !seen {
			byKey[ItemKey(n)] = n
		}
	}
	for name, u := range v.units {
		u.Wargear = slices.Clone(u.Wargear)
		for i, w := range u.Wargear {
			if c, ok := spell[strings.ToLower(strings.TrimSpace(w))]; ok {
				u.Wargear[i] = c
			} else if c, ok := byKey[ItemKey(w)]; ok {
				u.Wargear[i] = c
			}
		}
		v.units[name] = u
	}
}
