package core

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Building a company's pages from its state and the rules: a unit
// names a vanilla profile and the company adds what it bought, what its path gave it and its experience. Attributes, special
// rules, gear and spells are then looked up in the rules. Everything here is pure: no locks, no I/O.

var statSentence = regexp.MustCompile(`(\d+)"\s*Range,\s*(\d+)\s*Strength\.?`)

// grant is a special rule an item gives; mount is true when it belongs to the mount (Pony: Timid).
type grant struct {
	name  string
	mount bool
}

// equipmentFor builds an item from what the Battle Companies file says about it: its classes, missile profile and granted
// rules come from the gear annotations, its slot size, cost and text from the armoury, and, for items only vanilla knows,
// the text from the vanilla gear list. grants are the special rules the item gives.
func equipmentFor(rs *RuleSet, bcCo BCCompany, name string, bought, profile, hero bool) (e Equipment, grants []grant) {
	e = Equipment{Name: name, Bought: bought, Profile: profile}
	a, annotated := rs.BC.Annotation(name)
	offer, offered := rs.BC.Offer(bcCo, name)
	classes := []EquipmentClass{EquipOther}
	switch {
	case annotated:
		classes = a.Classes
	case offered:
		classes = offer.Classes
	}
	e.Class, e.Also = classes[0], classes[1:]
	if annotated {
		e.Spellings = append([]string{a.Name}, a.Aliases...)
		e.RequiresMount = a.RequiresMount
	}
	e.Exclusive = offered && offer.Exclusive
	e.Stats = a.Stats()
	if offered {
		e.Size, e.Description = offer.Size, offer.Description
	} else if g, found := rs.Vanilla.Gear(name); found {
		e.Description = g.Definition
	}
	e.Description = strings.TrimSpace(statSentence.ReplaceAllString(e.Description, "")) // the profile is shown as stats
	if a.Definition != "" {                                                             // what the Battle Companies file says the item does comes first
		e.Description = a.Definition
	}
	if e.Description == "" {
		e.Description = fallbackGearText(rs, name, annotated && len(a.Classes) == 1 && a.Classes[0] == EquipMelee && len(a.Grants) == 0)
	}
	if a.MountProfile != "" {
		e.MountStats = rs.Vanilla.MountProfile(a.MountProfile)
		if g, ok := rs.Vanilla.Gear(a.MountProfile); ok {
			e.MountRules = g.Rules
		}
	}
	e.Icon = IconFor(name, classes, e.Size)
	if annotated && IsUnitUpgrade(a, e.Size) {
		e.Upgrade, e.Profile = true, true // a package that came with the unit, not something to drop or count
	}
	rules := a.Grants
	if a.ChosenWeapon != nil { // a weapon modifier gives its rule to the weapon the player chose, and the unit's build adds it
		rules = nil
	}
	if !annotated && offered {
		rules = offer.Grants
	}
	for _, r := range rules {
		grants = append(grants, grant{name: r, mount: a.GrantedTo == "mount"})
	}
	return e, grants
}

// fallbackGearText is the tooltip of an item no file describes by itself: the vanilla special rule of the same name
// (Twin Blades), the glossary's wargear entry of the same name (Elven cloak), or, for plain melee weapons and the natural and plain weapons that are hand weapons ("Hand weapons", "Fangs (hand weapons)"),
// the rulebook's Hand Weapon entry. Mounts need none: their chip shows the mount's profile.
func fallbackGearText(rs *RuleSet, name string, plainMelee bool) string {
	if d, ok := rs.Vanilla.Keyword(name); ok {
		return d
	}
	if plainMelee || strings.Contains(strings.ToLower(name), "hand weapon") { // a plain melee weapon (an axe, a sword) is a hand weapon
		if t, ok := rs.Glossary.Find("Hand Weapon"); ok {
			return t.Summary(400)
		}
	}
	if t, ok := rs.Glossary.Find(name); ok && t.Category == "wargear" { // Elven cloak: the rulebook's wargear entry
		return t.Summary(500)
	}
	return ""
}

// ownPathLevels says whether units On Their Own Path earn experience in this campaign (the homebrew Path of Their Own Making):
// they are warriors that level up on the warrior table and become heroes on that path with a 6.
func ownPathLevels(rs *RuleSet, settings CampaignSettings, profile CompanyProfile, unit string) bool {
	return settings.CustomOnTheirOwnPath && rs.BC.Progression().CustomPath != "" && profile.OnOwnPath(unit)
}

func buildUnit(rs *RuleSet, bcCo BCCompany, s UnitState, settings CampaignSettings) CompanyUnit {
	book := rs.Vanilla
	base, found := book.Unit(s.Base)
	var aliasRules, aliasOptions []string // a company's own name for a profile, with rules added (Ranger of Ithilien) and options selected
	if !found {
		if profile, ok := rs.BC.Profile(bcCo.Name); ok {
			for _, a := range profile.Aliases {
				if strings.EqualFold(a.Label, s.Base) {
					if v, ok := book.Unit(a.Unit); ok {
						base, found, aliasRules, aliasOptions = v, true, a.AddRules, a.Selected
					}
				}
			}
		}
	}
	var wanderer WandererOffer
	if s.Wanderer != "" {
		_, offers := rs.BC.Wanderers()
		for _, w := range offers {
			if w.Name == s.Wanderer {
				wanderer, base, found = w, w.AsUnit(), true
			}
		}
	}
	profile, _ := rs.BC.Profile(bcCo.Name)
	for _, name := range aliasOptions { // the options an alias has selected are part of the unit: they cost their points and are on its card
		if i := slices.IndexFunc(base.Options, func(o ChoiceOption) bool { return o.Name == name }); i >= 0 {
			base.Points += base.Options[i].Points
		}
		if !slices.Contains(s.Gear, name) {
			s.Gear = append(slices.Clone(s.Gear), name)
		}
	}
	if s.Wanderer == "" { // the whole option bundle of a row that names one piece of it (Iron Hills Warrior (Spear))
		if added, _ := completeBundle(rs, bcCo, base, s.Gear, nil); len(added) > 0 {
			s.Gear = append(slices.Clone(s.Gear), added...)
		}
	}
	base.UnitTypes = profile.UnitTypes(s.Base, base.UnitTypes) // the company's own changes to what the unit is (a Warg Marauder is Cavalry)
	if s.Points > 0 {
		base.Points = s.Points
	}
	ownPathLevels := ownPathLevels(rs, settings, profile, s.Base)
	hero := s.Role != RoleWarrior
	u := CompanyUnit{ID: s.ID, Name: s.Name, Type: s.Base, Role: s.Role, Level: s.Level, XP: s.XP, Injured: s.Injured, AtHealers: s.SentToHealers}
	u.Kind = s.Kind
	if u.Kind == "" {
		u.Kind = KindOf(base.UnitTypes)
	}
	if found {
		bonus := maps.Clone(s.Bonus)
		if bonus == nil {
			bonus = map[string]int{}
		}
		for k, v := range rs.BC.Injuries().StatBonus(s.Injuries) {
			bonus[k] += v
		}
		u.Stats = book.Profile(base, hero, bonus)
	}

	var grants []UnitRule // rules given by equipment, with the item that gives them
	addItem := func(name string, bought, profile bool) {
		e, g := equipmentFor(rs, bcCo, name, bought, profile, hero)
		u.Equipment = append(u.Equipment, e)
		for _, gr := range g {
			desc, ok := book.Keyword(gr.name)
			if !ok {
				desc, _ = rs.BC.Keyword(gr.name)
			}
			rule := UnitRule{Name: gr.name, Description: desc, Source: RuleFromEquipment, From: name}
			if gr.mount {
				rule.Mount = name
			}
			grants = append(grants, rule)
		}
	}
	drops := slices.Clone(s.Dropped) // each drop removes one item: starting ones first, then the bought
	dropped := func(name string) bool {
		if i := slices.Index(drops, name); i >= 0 {
			drops = slices.Delete(drops, i, i+1)
			return true
		}
		return false
	}
	for _, w := range base.Wargear {
		if !dropped(w) {
			addItem(w, false, true)
		}
	}
	for _, w := range s.Gear {
		if !dropped(w) {
			addItem(w, false, false)
		}
	}
	for _, b := range s.Bought {
		if !dropped(b) {
			addItem(b, true, false)
		}
	}
	u.Equipment = SortEquipment(u.Equipment)
	for i := range u.Equipment {
		u.Equipment[i].Drop = DropReason(u, u.Equipment[i], base.Options, settings.HeroesDropProfileGear)
	}

	for _, name := range base.SpecialRules {
		desc, _ := book.Keyword(name)
		u.Rules = append(u.Rules, UnitRule{Name: name, Description: desc, Source: RuleFromUnit})
	}
	for _, p := range base.MagicalPowers {
		power, _ := book.Power(p.Name)
		desc := power.Definition
		if power.Duration != "" {
			desc = "Duration: " + power.Duration + ". " + desc
		}
		u.Rules = append(u.Rules, UnitRule{Name: p.Name, Description: desc, Source: RuleFromUnit, Spell: true, Casting: p.Casting, Range: p.Range})
	}
	if hero && s.Path == "" && s.Wanderer == "" { // an officer that has not chosen its path: the card asks for it (a hired wanderer has none to choose)
		u.PathPending = true
		u.Path = &UnitPath{}
	} else if hero {
		u.Path = &UnitPath{Name: s.Path, Rules: rs.BC.PathRules(s.Path, s.PathRules)}
		if p := s.Progress; p != nil {
			for _, name := range p.Rules {
				u.Path.Rules = append(u.Path.Rules, rs.BC.pathRuleNamed(s.Path, name))
			}
			for _, name := range p.Upgrades { // company hero upgrades
				text := ""
				if profile, ok := rs.BC.Profile(bcCo.Name); ok {
					for _, up := range profile.HeroUpgrades {
						if up.Name == name {
							text = up.Text
						}
					}
				}
				u.Path.Rules = append(u.Path.Rules, UnitRule{Name: name, Description: text, Source: RuleFromCompany})
			}
			u.Path.MinorRules = len(p.Minor)
			for _, entry := range p.Minor {
				name, param, parameterised := strings.Cut(entry, "|")
				if parameterised { // a rule with a target: a weapon now, a hero once all the units are built (resolveTargets)
					if weapon, ok := strings.CutPrefix(param, "weapon:"); ok {
						def := rs.BC.Progression().MinorParams[name].Definition
						u.Path.Rules = append(u.Path.Rules, UnitRule{Name: name + " (" + weapon + ")", Description: def + " It applies to the " + weapon + ".", Source: RuleFromPath})
						u.Path.minorListed++
						for i := range u.Equipment {
							if strings.EqualFold(u.Equipment[i].Name, weapon) {
								u.Equipment[i].Poisoned = true
							}
						}
					}
					continue
				}
				desc, _ := book.Keyword(name)
				if desc == "" {
					desc, _ = rs.BC.Keyword(name)
				}
				u.Path.Rules = append(u.Path.Rules, UnitRule{Name: name, Description: desc, Source: RuleFromPath})
				u.Path.minorListed++
			}
		}
		for _, sp := range s.Spells {
			power, _ := book.Power(sp.Name)
			desc := power.Definition
			if power.Duration != "" {
				desc = "Duration: " + power.Duration + ". " + desc
			}
			u.Path.Powers = append(u.Path.Powers, UnitRule{Name: sp.Name, Description: desc, Source: RuleFromPath, Spell: true, Casting: sp.Casting, Range: sp.Range})
		}
		known := map[string]bool{}
		add := func(names []string, src ActionSource) { // a hero cannot have the same heroic action twice
			for _, a := range names {
				if !known[a] {
					known[a] = true
					u.Path.HeroicActions = append(u.Path.HeroicActions, HeroicAction{Name: a, Source: src})
				}
			}
		}
		add(base.HeroicActions, ActionFromUnit)
		add(s.Actions, ActionFromPath)
		add(s.BoughtActions, ActionBought)
		if s.Progress != nil {
			add(s.Progress.Actions, ActionFromPath)
		}
	}
	grants = applyWeaponModifiers(rs, book, &u, grants)
	have := map[string]bool{}
	for _, r := range u.Rules {
		have[strings.ToLower(r.Name)] = true
	}
	for _, g := range grants { // what the Battle Companies armoury says an item grants
		if !have[strings.ToLower(g.Name)] {
			u.Rules = append(u.Rules, g)
			have[strings.ToLower(g.Name)] = true
		}
	}
	for _, e := range u.Equipment { // vanilla items that are also special rules (Lance, Banner...) grant them too
		if desc, ok := book.Keyword(e.Name); ok && !have[strings.ToLower(e.Name)] {
			u.Rules = append(u.Rules, UnitRule{Name: e.Name, Description: desc, Source: RuleFromEquipment, From: e.Name})
			have[strings.ToLower(e.Name)] = true
		}
	}
	rs.BC.Injuries().ApplyToUnit(&u, s.Injuries, func(name string) string { d, _ := book.Keyword(name); return d })
	for _, name := range aliasRules {
		if !have[strings.ToLower(name)] {
			desc, _ := book.Keyword(name)
			u.Rules = append(u.Rules, UnitRule{Name: name, Description: desc, Source: RuleFromCompany})
			have[strings.ToLower(name)] = true
		}
	}
	if comp, ok := profile.Composites[s.Base]; ok { // one model on the table that counts as several
		u.Models = comp.Slots
		for _, part := range comp.Parts {
			u.Parts = append(u.Parts, UnitPart{Role: part.Role, Name: part.Name, Count: part.Count, Wargear: part.Wargear})
		}
		for _, name := range comp.Rules {
			if !have[strings.ToLower(name)] {
				desc, _ := book.Keyword(name)
				u.Rules = append(u.Rules, UnitRule{Name: name, Description: desc, Source: RuleFromUnit})
				have[strings.ToLower(name)] = true
			}
		}
	}
	if !hero && !ownPathLevels && profile.OwnPath != "" && profile.OnOwnPath(s.Base) { // it never earns experience: its path is the end of the path
		rule := rs.BC.Progression().EndOfPath[profile.OwnPath]
		desc, _ := rs.BC.Keyword(rule)
		u.OwnPath, u.XP, u.Level = true, 0, 0
		u.Path = &UnitPath{Name: profile.OwnPath}
		if rule != "" {
			u.Path.Rules = []UnitRule{{Name: rule, Description: desc, Source: RuleFromPath}}
		}
	}
	u.Rules = SortRules(u.Rules)
	label := func(rules []UnitRule) {
		for i := range rules {
			if l := rs.BC.RuleLabel(rules[i].Name); l != rules[i].Name {
				rules[i].Label = l
			}
		}
	}
	label(u.Rules)
	if u.Path != nil {
		label(u.Path.Rules)
	}
	if found {
		applyGearStats(rs, bcCo, base, &u, hero)
	}
	u.Points = pointsOf(rs, bcCo, base, &u)
	if s.Wanderer != "" { // a mercenary is worth its listed points and has no path, experience or wargear to buy
		u.Wanderer, u.Points = true, wanderer.Points
		u.Path = &UnitPath{Name: "Wanderer"}
		for _, a := range base.HeroicActions {
			u.Path.HeroicActions = append(u.Path.HeroicActions, HeroicAction{Name: a, Source: ActionFromUnit})
		}
	}
	u.Keywords = unitKeywordsOf(rs, profile, s, u) // last: a bought mount makes the rider Cavalry
	u.Auras = rs.UnitAuras(u)                      // after the gear, the rules and the path are in
	return u
}

// pointsOf is the unit's rating by the Battle Companies rules (its points):
//   - a warrior is worth its MESBG points (with the option its wargear amounts to) plus the rating of its equipment;
//   - a hero built from a warrior profile is worth that profile's points plus the rating of what it gained (HeroRating), plus
//     the options of its profile it carries at their MESBG points ("make no adjustment: it is as it is in their book"), plus
//     the rating of wargear and equipment that is neither part of the profile nor of its options, at the hero rating when its
//     Attacks and Wounds add up to 3 or more and at the warrior rating below that;
//   - any other hero (a named character, a monster) is its vanilla points plus its options plus the rating of its gear.
func pointsOf(rs *RuleSet, bcCo BCCompany, base VanillaUnit, u *CompanyUnit) int {
	stats := map[string]int{}
	for _, a := range u.Stats {
		stats[strings.ToLower(a.Name)] = a.Value - a.Gear // the rating counts the earned profile, not what gear adds
	}
	hero := u.IsHero()
	heroColumn := hero && stats["attack"]+stats["wounds"] >= 3
	ratingOf := func(name string) Rating {
		offer, _ := rs.BC.Offer(bcCo, name)
		return offer.Rating
	}
	total := 0
	var wargear []Equipment // wargear that is neither part of the profile
	for i := range u.Equipment {
		e := &u.Equipment[i]
		e.Rating = 0
		switch {
		case e.Profile:
		case e.Size != "": // equipment always counts
			e.Rating = ratingOf(e.Name).For(heroColumn)
		default:
			wargear = append(wargear, *e)
			if stage, isArmour := armourRating(rs, base, *e); isArmour { // an armour upgrade adds rating per stage above the profile's own armour
				if hero {
					e.Rating = stage * rs.BC.ArmourUpgrade().Rating.For(heroColumn) // +1 per stage until Attacks + Wounds reach 3, then +5
				}
			} else if hero && !slices.ContainsFunc(base.Options, func(o ChoiceOption) bool {
				return GearMatchesOption(GearOffer{Name: e.Name, AlsoCalled: e.Spellings, Classes: e.Classes()}, o.Name)
			}) {
				e.Rating = ratingOf(e.Name).For(heroColumn)
			}
		}
		total += e.Rating
	}
	covered, rest := OptionsCover(wargear, base.Options) // the options of the profile cost what the MESBG book says, for a hero as for a warrior
	switch {
	case !hero:
		for _, e := range rest { // wargear that is no part of any option: charge its rating
			total += ratingOf(e.Name).Warrior
		}
		return base.Points + covered + total
	case slices.Contains(base.UnitTypes, "Warrior"):
		pathRules, minor := 0, 0
		if u.Path != nil {
			pathRules, minor = len(u.Path.Rules)-u.Path.minorListed, u.Path.MinorRules
		}
		return rs.BC.HeroRating(base, stats, pathRules, minor) + covered + total
	}
	return base.Points + covered + total
}

// armourRating is the number of stages an armour the unit bought is above the armour its profile comes with and the armour the
// profile's Options give (BC "Calculate Rating": wargear in the base profile or the profile's Options makes no adjustment).
func armourRating(rs *RuleSet, base VanillaUnit, e Equipment) (stages int, isArmour bool) {
	if !slices.Contains(e.Classes(), EquipArmour) {
		return 0, false
	}
	up := rs.BC.ArmourUpgrade()
	now, ok := up.StageOf(e.Name)
	if !ok {
		return 0, false
	}
	free := 0 // the highest stage the profile or its Options have
	for _, w := range base.Wargear {
		if s, ok := up.StageOf(w); ok {
			free = max(free, s)
		}
	}
	for _, o := range base.Options {
		for _, part := range OptionItems(o.Name) {
			if s, ok := up.StageOf(part); ok && !strings.HasPrefix(strings.ToLower(o.Name), "exchange") {
				free = max(free, s)
			}
		}
	}
	return max(0, now-free), true
}

// StartingUnits makes the units of a company's starting company: the leader and the sergeants the player chose are heroes (with a
// point of Might, Will and Fate) that have yet to choose their path, the rest warriors.
func StartingUnits(p CompanyProfile, heroes []StartHero) []UnitState {
	byIndex := map[int]StartHero{}
	for _, h := range heroes {
		byIndex[h.Index] = h
	}
	var out []UnitState
	for i, s := range p.StartList() {
		spec := UnitState{ID: int64(i + 1), Base: s.Unit, Role: RoleWarrior, Gear: slices.Clone(s.Wargear)}
		if s.Role == RoleHeroInTheMaking { // the file starts it as a hero that has yet to choose its path
			spec.Role = RoleHero
			if p.StartGains(s.Unit) {
				spec.Bonus = HeroBonus(nil)
			}
		}
		if h, ok := byIndex[i]; ok {
			spec.Role = h.Role
			if p.StartGains(s.Unit) { // an officer keeps its profile (no hero promotion: that is a level-up result) and gains a point of Might, Will and Fate
				spec.Bonus = HeroBonus(nil)
			}
		}
		out = append(out, spec)
	}
	return out
}

// WithPath gives an officer the path it chose: the path, its heroic action and, for a Sorcerer, its first power.
func WithPath(spec UnitState, path, power string, paths ProgressionRules) UnitState {
	spec.Path = path
	if p, ok := paths.Path(path); ok && p.StartAction != "" {
		spec.Actions = []string{p.StartAction}
	}
	if offer, ok := paths.Power(power); ok {
		spec.Spells = []PathSpell{{Name: offer.Name, Casting: offer.Casting, Range: offer.Range}}
	}
	return spec
}

func describe(r Reinforcement) string {
	switch r.Type {
	case "nothing":
		return "Nothing"
	case "specialChart":
		return "Roll on the special chart"
	case "choice":
		return r.Text
	case "lostStartingUnit":
		return "Replace a starting unit that was lost"
	case "unitChoice":
		var names []string
		for _, o := range r.Options {
			names = append(names, choiceName(o))
		}
		return "One of: " + strings.Join(names, ", ") + " (one of each at most)"
	}
	text := r.Unit
	if r.Count > 1 {
		text = fmt.Sprintf("%d × %s", r.Count, text)
	}
	if len(r.Wargear) > 0 {
		text += " (" + strings.Join(r.Wargear, ", ") + ")"
	}
	if r.Choice {
		text += ", you choose its options"
	}
	return text
}

// optionResult is what picking one of the units of a "unitChoice" result gives: that unit, as the company's label for it when it
// has one, with the option it selects.
func optionResult(o ChoiceUnit) Reinforcement {
	r := Reinforcement{Type: "unit", Unit: o.Unit, Count: 1, Rare: o.Rare}
	if o.Label != "" {
		r.Unit, r.Wargear = o.Label, o.Selected
	}
	return r
}

// choiceName is how a unit of a "unitChoice" result is called: the company's label for it, or the unit.
func choiceName(o ChoiceUnit) string {
	if o.Label != "" {
		return o.Label
	}
	return o.Unit
}

func chartRows(chart []ChartRow) []RecruitRow {
	var rows []RecruitRow
	for _, r := range chart {
		rows = append(rows, RecruitRow{Min: r.Min, Max: r.Max, Text: describe(r.Result), Alt: r.Result.Alt, Rare: r.Result.Rare})
	}
	return rows
}

func recruitInfo(bc BCCompany) RecruitInfo {
	info := RecruitInfo{Cost: bc.ReinforcementCost, Chart: chartRows(bc.Reinforcements), SpecialChart: chartRows(bc.SpecialChart), Notes: bc.RecruitNotes}
	for _, s := range bc.SpecialUnits {
		info.Special = append(info.Special, SpecialRecruit{Unit: s.Unit, Cost: s.Cost, Rare: s.Rare})
	}
	return info
}

// applyProgress puts what level-ups gave a unit onto its spec.
func applyProgress(rs *RuleSet, s *UnitState, p *Progress) {
	if p == nil {
		return
	}
	s.Progress = p
	s.Level += p.Taken
	s.Bonus = mergeBonus(s.Bonus, p.Bonus)
	if p.Base != "" {
		s.Base = p.Base
		s.Gear = append(slices.Clone(s.Gear), p.Gear...)
		for _, lost := range p.Drop {
			if i := slices.IndexFunc(s.Gear, func(g string) bool { return strings.EqualFold(g, lost) }); i >= 0 {
				s.Gear = slices.Delete(s.Gear, i, i+1)
			}
		}
	}
	if p.Hero {
		if s.Role != RoleLeader && s.Role != RoleSergeant { // a Hero named Sergeant or Leader keeps the officer's place
			s.Role = RoleHero
		}
		s.Path = p.Path
		if p.Start != "" {
			s.Actions = append(slices.Clone(s.Actions), p.Start)
		}
	}
	// magical powers learned, and casting values improved (a lower number is better: 5+ becomes 4+)
	s.Spells = slices.Clone(s.Spells)
	for _, name := range p.Powers {
		if offer, ok := rs.BC.Progression().Power(name); ok {
			s.Spells = append(s.Spells, PathSpell{Name: offer.Name, Casting: offer.Casting, Range: offer.Range})
		}
	}
	for i := range s.Spells {
		s.Spells[i].Casting -= p.Cast[strings.ToLower(s.Spells[i].Name)]
	}
}

func mergeBonus(a, b map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range a {
		out[k] += v
	}
	for k, v := range b {
		out[k] += v
	}
	return out
}

// resolveTargets gives the heroes the rules they aimed at another unit (Combat Synergy). The target is kept by id, so the rule
// follows the unit when it is renamed: "Combat Synergy - Hador" becomes "Combat Synergy - Hador the Bold" by itself.
func resolveTargets(rs *RuleSet, progress map[int64]*Progress, units []CompanyUnit) {
	byID := map[int64]int{}
	for i, u := range units {
		byID[u.ID] = i
	}
	for i := range units {
		p := progress[units[i].ID]
		if p == nil || units[i].Path == nil {
			continue
		}
		for _, entry := range p.Minor {
			name, param, ok := strings.Cut(entry, "|unit:")
			if !ok {
				continue
			}
			id, _ := strconv.ParseInt(param, 10, 64)
			t, found := byID[id]
			if !found {
				continue // the unit is gone
			}
			target := units[t].Title()
			def := strings.ReplaceAll(rs.BC.Progression().MinorParams[name].Definition, " X ", " "+target+" ")
			units[i].Path.Rules = append(units[i].Path.Rules, UnitRule{Name: name + " - " + target, Description: def, Source: RuleFromPath})
		}
	}
}

// applyCompanyRules applies the rules of the company that change its models: the special rules it gives some of them for free
// (Rohan's heroes), the rules it swaps for others (Sharkey's Rogues), and the share of the company a keyword may have (Helm's Deep: at most a third Elves), which it returns as
// limits to show. The rules that only matter in a match (substitutions, auras) are left to the cards' text.
func applyCompanyRules(rs *RuleSet, bcCo BCCompany, units []CompanyUnit) []CompanyLimit {
	profile, ok := rs.BC.Profile(bcCo.Name)
	if !ok {
		return nil
	}
	for _, g := range profile.Granted {
		heroesOnly := strings.Contains(strings.ToLower(g.AppliesTo), "hero")
		for i := range units {
			if heroesOnly && !units[i].IsHero() {
				continue
			}
			if slices.ContainsFunc(units[i].Rules, func(r UnitRule) bool { return strings.EqualFold(r.Name, g.Rule) }) {
				continue
			}
			desc, _ := rs.Vanilla.Keyword(g.Rule)
			if desc == "" {
				desc, _ = rs.BC.Keyword(g.Rule)
			}
			if desc == "" {
				desc = g.Text
			}
			units[i].Rules = SortRules(append(units[i].Rules, UnitRule{Name: g.Rule, Description: desc, Source: RuleFromCompany}))
		}
	}
	for _, e := range profile.Exchanges { // Sharkey's Rogues: Hatred (Rohan) becomes Hatred (Hobbit)
		for i := range units {
			j := slices.IndexFunc(units[i].Rules, func(r UnitRule) bool { return strings.EqualFold(r.Name, e.Remove) })
			if j < 0 {
				continue
			}
			if slices.ContainsFunc(units[i].Rules, func(r UnitRule) bool { return strings.EqualFold(r.Name, e.Add) }) {
				units[i].Rules = slices.Delete(units[i].Rules, j, j+1)
				continue
			}
			desc, _ := rs.Vanilla.Keyword(e.Add)
			if desc == "" {
				desc, _ = rs.BC.Keyword(e.Add)
			}
			if desc == "" {
				desc = e.Text
			}
			units[i].Rules[j] = UnitRule{Name: e.Add, Description: desc, Source: RuleFromCompany}
			units[i].Rules = SortRules(units[i].Rules)
		}
	}
	var limits []CompanyLimit
	for _, l := range profile.KeywordLimits {
		used, models := 0, 0
		for _, u := range units {
			models += u.ModelCount()
			if v, _, ok := resolveUnitKeywords(rs, profile, u.Type); ok && slices.ContainsFunc(v.Race, func(r string) bool { return strings.EqualFold(r, l.Keyword) }) {
				used++
			}
		}
		limits = append(limits, CompanyLimit{ID: "keyword-" + strings.ToLower(l.Keyword), Label: l.Keyword + " models", Used: used,
			Max: LimitMax(l.Percent, models), Next: LimitMax(l.Percent, models+1)})
	}
	return limits
}

// hasKeyword says whether a unit counts as the keyword: its race or one of its factions.
func hasKeyword(v VanillaUnit, keyword string) bool {
	match := func(k string) bool { return strings.EqualFold(k, keyword) }
	return slices.ContainsFunc(v.Race, match) || slices.ContainsFunc(v.Faction, match)
}

// countKeyword counts the models of a roster with a keyword.
func countKeyword(rs *RuleSet, profile CompanyProfile, roster []CompanyUnit, keyword string) int {
	n := 0
	for _, u := range roster {
		if v, _, ok := resolveUnitKeywords(rs, profile, u.Type); ok && hasKeyword(v, keyword) { // a company's own name for a profile counts as that profile
			n++
		}
	}
	return n
}

// Why a unit cannot join the company now.
const (
	blockLimit   = "limit"   // it would go over the share of the company a keyword may have (Helm's Deep: a third Elves)
	blockBalance = "balance" // it would outnumber the race the company balances it against (Defenders: Dwarves and Dale)
	blockRare    = "rare"    // the company already has as many of this rare unit as it may
	blockModels  = "models"  // it would take the company over its maximum number of models
)

// companyBlock says why a unit cannot join the company now, with the kind of reason (empty when it can join).
func companyBlock(rs *RuleSet, bcCo BCCompany, roster []CompanyUnit, unit string) (why, kind string) {
	profile, _ := rs.BC.Profile(bcCo.Name)
	v, _, known := resolveUnitKeywords(rs, profile, unit)
	if !known {
		return "", ""
	}
	return companyBlockFor(rs, bcCo, roster, v)
}

// companyBlockFor is companyBlock for a profile that need not be in the vanilla book (a wanderer).
func companyBlockFor(rs *RuleSet, bcCo BCCompany, roster []CompanyUnit, v VanillaUnit) (why, kind string) {
	profile, ok := rs.BC.Profile(bcCo.Name)
	if !ok {
		return "", ""
	}
	models := 0
	for _, u := range roster {
		models += u.ModelCount()
	}
	n := profile.Models(v.Name)
	if models+n > bcCo.MaxModels {
		return fmt.Sprintf("it counts as %d model(s) and the company has %d of its %d", n, models, bcCo.MaxModels), blockModels
	}
	for _, l := range profile.KeywordLimits {
		if !hasKeyword(v, l.Keyword) {
			continue
		}
		have := countKeyword(rs, profile, roster, l.Keyword)
		if max := LimitMax(l.Percent, models+n); have+1 > max { // the new model counts in both numerator and denominator
			return fmt.Sprintf("the company may have at most %d %s models (%v%% of %d, rounded up) and has %d", max, l.Keyword, l.Percent, models+n, have), blockLimit
		}
	}
	for _, b := range profile.Balance {
		if hasKeyword(v, b.Keyword) && countKeyword(rs, profile, roster, b.Keyword)+1 > countKeyword(rs, profile, roster, b.NotMoreThan) {
			return fmt.Sprintf("the %s models may not outnumber the %s models", b.Keyword, b.NotMoreThan), blockBalance
		}
	}
	return "", ""
}

// rareBlock says why a rare unit cannot join: the company may have only so many of it at once. A unit that is no longer that
// unit (promoted) or is gone does not count.
func rareBlock(roster []CompanyUnit, unit string, rare int) (why, kind string) {
	if rare <= 0 || unit == "" {
		return "", ""
	}
	have := 0
	for _, u := range roster {
		if u.Type == unit {
			have++
		}
	}
	if have >= rare {
		return fmt.Sprintf("%s is rare: the company may have %d at once and has %d", unit, rare, have), blockRare
	}
	return "", ""
}

// resultBlock says why a result cannot join the company (empty when it can), with the kind of reason.
func resultBlock(rs *RuleSet, bcCo BCCompany, roster []CompanyUnit, res Reinforcement) (why, kind string) {
	if res.Type == "unitChoice" { // it can join when at least one of its units can
		for _, o := range res.Options {
			if why, _ := resultBlock(rs, bcCo, roster, optionResult(o)); why == "" {
				return "", ""
			}
		}
		return "the company already has every one of them", blockRare
	}
	if res.Type != "unit" {
		return "", ""
	}
	if profile, ok := rs.BC.Profile(bcCo.Name); ok && res.Rare > 0 {
		if members, isTeam := profile.IncludedPoints[res.Unit]; isTeam { // a team (Vault Warden Team) is in the roster as its members: it counts as many times as its most numerous member
			teams := 0
			for _, m := range members {
				n := 0
				for _, u := range roster {
					if u.Type == m.Name {
						n++
					}
				}
				teams = max(teams, n)
			}
			if teams >= res.Rare {
				return fmt.Sprintf("%s is rare: the company may have %d at once and has %d", res.Unit, res.Rare, teams), blockRare
			}
			return companyBlock(rs, bcCo, roster, res.Unit)
		}
	}
	if why, kind := rareBlock(roster, res.Unit, res.Rare); why != "" {
		return why, kind
	}
	return companyBlock(rs, bcCo, roster, res.Unit)
}

func creatureOffer(rs *RuleSet, name string) (CreatureOffer, bool) {
	_, offers := rs.BC.Creatures()
	for _, o := range offers {
		if o.Name == name {
			return o, true
		}
	}
	return CreatureOffer{}, false
}

func creatureStats(rs *RuleSet, o CreatureOffer) []Attribute {
	return rs.Vanilla.Profile(VanillaUnit{Stats: o.Stats}, false, nil)
}

// creaturesOf lists the creatures the units own, marks their owners' cards and works out the Baggage Pony's pack.
func creaturesOf(rs *RuleSet, owned map[int64]string, pack []string, units []CompanyUnit) ([]CompanyCreature, *Pack) {
	var out []CompanyCreature
	var carried *Pack
	for i := range units {
		name, ok := owned[units[i].ID]
		if !ok {
			continue
		}
		o, found := creatureOffer(rs, name)
		if !found {
			continue
		}
		stats := creatureStats(rs, o)
		notes := slices.Clone(o.Notes)
		if o.CourageOfOwner {
			notes = append(notes, "Its Courage is the same as its owner's.")
		}
		units[i].Creature = &UnitCreatureRef{Name: o.Name, Stats: stats, Rules: o.Rules, Notes: notes}
		out = append(out, CompanyCreature{Name: o.Name, OwnerID: units[i].ID, OwnerName: units[i].Title(), Points: o.Points, Stats: stats, Rules: o.Rules, Notes: notes, Carry: o.Carry})
		if o.Carry != nil && carried == nil {
			carried = &Pack{Capacity: o.Carry.Capacity}
			for _, n := range pack {
				e, _ := equipmentFor(rs, BCCompany{}, n, false, false, false)
				carried.Items = append(carried.Items, PackItem{Name: n, Class: e.Class, Icon: e.Icon, Size: e.Size})
			}
		}
	}
	return out, carried
}

// parsePathRules reads the special rules each Battle Companies path unlocks, with their descriptions ("Spy (Active) - text").
func parsePathRules(bc []byte) map[string][]UnitRule {
	var doc struct {
		Data struct {
			Paths []struct {
				Name        string `json:"name"`
				Progression []struct {
					Text    string `json:"text"`
					Options []struct {
						Type        string `json:"type"`
						SpecialRule string `json:"specialRule"`
					} `json:"options"`
				} `json:"progression"`
			} `json:"paths"`
		} `json:"data"`
	}
	out := map[string][]UnitRule{}
	if json.Unmarshal(bc, &doc) != nil {
		return out
	}
	for _, p := range doc.Data.Paths {
		for _, step := range p.Progression {
			for _, o := range step.Options {
				if o.Type != "specialRule" {
					continue
				}
				text := step.Text
				if _, after, ok := strings.Cut(text, " - "); ok {
					text = after
				}
				out[p.Name] = append(out[p.Name], UnitRule{Name: o.SpecialRule, Description: text, Source: RuleFromPath})
			}
		}
	}
	return out
}

// PathRules returns the first n special rules a path unlocks, with their descriptions: the rules a hero of that path has gained
// so far.
func (b *BCBook) PathRules(path string, n int) []UnitRule {
	all := b.pathRules[path]
	return slices.Clone(all[:max(0, min(n, len(all)))])
}

// pathRuleNamed is the special rule of a path with that name and its description.
func (b *BCBook) pathRuleNamed(path, name string) UnitRule {
	for _, r := range b.pathRules[path] {
		if r.Name == name {
			return r
		}
	}
	return UnitRule{Name: name, Source: RuleFromPath}
}

// HeroBonus is what a hero starts with: one point each of Might, Will and Fate, plus extra.
func HeroBonus(extra map[string]int) map[string]int {
	b := map[string]int{"might": 1, "will": 1, "fate": 1}
	for k, v := range extra {
		b[k] += v
	}
	return b
}

// BuildDetail turns a company's stored state into what the pages show: the units built from their profiles, equipment, path
// and level-ups, the company's limits and rating, the recruit charts with the reasons a result cannot join, and the cemetery.
// Units are in the company's saved order (OrderUnits). An injured unit (or a hero sent to the healers) misses the next game:
// its points are left out of Points and counted in FullPoints. It never fails: a name the rules do not know shows as is.
func BuildDetail(rs *RuleSet, settings CampaignSettings, st CompanyState) CompanyDetail {
	co := st.Company
	bcCo, _ := rs.BC.Company(co.Faction)
	var units []CompanyUnit
	progress := map[int64]*Progress{}
	owned := map[int64]string{}
	for _, us := range st.Units {
		s := us.Clone()
		applyProgress(rs, &s, s.Progress)
		s.Injured = s.Injured || s.SentToHealers
		if s.Injuries == nil {
			s.Injuries = map[string]int{}
		}
		if s.Progress != nil {
			progress[s.ID] = s.Progress
		}
		if s.Creature != "" {
			owned[s.ID] = s.Creature
		}
		units = append(units, buildUnit(rs, bcCo, s, settings))
	}
	units = OrderUnits(units, st.Order)
	resolveTargets(rs, progress, units)
	companyLimits := applyCompanyRules(rs, bcCo, units)
	targets := rs.BC.RuleTargets()
	for i := range units {
		targets.annotateUnit(&units[i]) // last: the rules a company grants or swaps are in
	}
	creatures, pack := creaturesOf(rs, owned, st.Pack, units)
	var stored []Equipment // the pack's gear still counts towards the bow and throwing limits
	for _, n := range st.Pack {
		e, _ := equipmentFor(rs, bcCo, n, false, false, false)
		stored = append(stored, e)
	}
	co.Units, co.Points, co.FullPoints = 0, 0, 0
	for _, u := range units {
		co.Units += u.ModelCount()
	}
	injured := 0
	for _, u := range units {
		co.FullPoints += u.Points
		if u.Injured { // an injured model misses the next game: it does not count towards the company's rating
			injured++
			continue
		}
		co.Points += u.Points
	}
	for _, cr := range creatures {
		co.Points += cr.Points // a creature counts for its points in the rating
		co.FullPoints += cr.Points
	}
	detail := CompanyDetail{CampaignCompany: co, Influence: st.Influence, Gold: st.Gold, Limits: append(rs.BC.Limits(bcCo, units, stored...), companyLimits...),
		MaxUnits: bcCo.MaxModels, Roster: units, Recruit: recruitInfo(bcCo), InjuredUnits: injured, Settings: settings, Injuries: rs.BC.Injuries(), Creatures: creatures, Pack: pack, Rerolls: st.Rerolls, Aftermaths: st.Aftermaths, OpenMatch: st.OpenMatch, OfficersPending: st.Recruit.Officers != nil, BreakPointPercent: breakPointPercent(rs, co.Faction)}
	for _, chart := range []struct {
		rows []RecruitRow
		from []ChartRow
	}{{detail.Recruit.Chart, bcCo.Reinforcements}, {detail.Recruit.SpecialChart, bcCo.SpecialChart}} {
		for i, row := range chart.from {
			for j, o := range row.Result.Options { // the units a row lets the player pick, and which of them the company has already
				blocked, _ := resultBlock(rs, bcCo, units, optionResult(o))
				chart.rows[i].Options = append(chart.rows[i].Options, RowOption{Index: j, Text: choiceName(o), Blocked: blocked})
			}
		}
	}
	detail.Cemetery = OrderFallen(slices.Clone(st.Cemetery), st.Graveyard)
	if profile, ok := rs.BC.Profile(bcCo.Name); ok {
		for i := range detail.Cemetery { // a stored snapshot may predate the keywords: the grave shows the unit's own, without a mount
			u := &detail.Cemetery[i].Unit
			if !u.Keywords.Empty() {
				continue
			}
			in := UnitKeywordsInput{Type: u.Type, Role: u.Role}
			if u.Wanderer {
				in.Wanderer = u.Type
			}
			u.Keywords = rs.UnitKeywords(profile, in)
		}
	}
	detail.RangerUnit, detail.RangerMayReturn = rangerMayReturn(rs, bcCo.Name, units, detail.Cemetery)
	for i, sp := range detail.Recruit.Special {
		detail.Recruit.Special[i].Blocked, _ = resultBlock(rs, bcCo, units, Reinforcement{Type: "unit", Unit: sp.Unit, Rare: sp.Rare})
	}
	return detail
}

// breakPointPercent is the company's Break Point percentage: its own rule (Muster of Isengard 66) or the file's default.
func breakPointPercent(rs *RuleSet, faction string) int {
	if p, ok := rs.BC.Profile(faction); ok && p.BreakPoint > 0 {
		return p.BreakPoint
	}
	return rs.BC.BreakRules().DefaultPercent
}

// unitKeywordsOf is the keyword set of a built unit: its profile's, plus the mount it carries (a bought one, or a composite's).
func unitKeywordsOf(rs *RuleSet, profile CompanyProfile, s UnitState, u CompanyUnit) UnitKeywords {
	in := UnitKeywordsInput{Type: s.Base, Role: s.Role, Wanderer: s.Wanderer}
	if i := slices.IndexFunc(u.Equipment, func(e Equipment) bool { return slices.Contains(e.Classes(), EquipMount) }); i >= 0 {
		in.Mount = u.Equipment[i].Name
	}
	if in.Mount == "" {
		for _, part := range u.Parts {
			if part.Role == "mount" {
				in.Mount = part.Name
			}
		}
	}
	return rs.UnitKeywords(profile, in)
}
