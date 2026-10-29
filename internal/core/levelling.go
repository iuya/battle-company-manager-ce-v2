package core

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// LevelService runs level-ups. A unit that earned LevelEvery experience rolls: a warrior on a D6 (nothing, a promotion, or a
// hero in the making), a hero twice on 2D6 on its path, choosing one result. The rules come from the Battle Companies file
// (ProgressionRules); the roll in progress is UnitState.Pending.Level, stored with the unit, until the player chooses. Every
// step is a companyRepository.Update.
type LevelService struct{ companyDeps }

// NewLevelService builds the service. rules is the loader (*Rules); dice rolls for the players who let the app roll.
func NewLevelService(campaigns campaignRepository, companies companyRepository, rules ruleSource, dice Dice) *LevelService {
	return &LevelService{companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: dice}}
}

// levelEnv is a company as a level-up sees it, with the unit rolling.
type levelEnv struct {
	rs       *RuleSet
	settings CampaignSettings
	d        CompanyDetail
	bcC      BCCompany
	unit     CompanyUnit
}

func newLevelEnv(rs *RuleSet, settings CampaignSettings, st CompanyState, unitID int64) (levelEnv, error) {
	bcCo, _ := rs.BC.Company(st.Company.Faction)
	env := levelEnv{rs: rs, settings: settings, d: BuildDetail(rs, settings, st), bcC: bcCo}
	i := slices.IndexFunc(env.d.Roster, func(u CompanyUnit) bool { return u.ID == unitID })
	if i < 0 {
		return env, ErrNotFound
	}
	env.unit = env.d.Roster[i]
	return env, nil
}

// refresh builds the detail again from the (changed) state.
func (e *levelEnv) refresh(st CompanyState) {
	e.d = BuildDetail(e.rs, e.settings, st)
	if i := slices.IndexFunc(e.d.Roster, func(u CompanyUnit) bool { return u.ID == e.unit.ID }); i >= 0 {
		e.unit = e.d.Roster[i]
	}
}

// mutate runs a step on the locked state of the user's company.
func (s *LevelService) mutate(ctx context.Context, user User, campaignID, unitID int64, fn func(env *levelEnv, st *CompanyState, us *UnitState) error) error {
	id, err := s.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	return s.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		if err := st.Locked(); err != nil { // a running match or an unfinished aftermath: no level-ups, no paths
			return err
		}
		env, err := newLevelEnv(rs, settings, *st, unitID)
		if err != nil {
			return err
		}
		us, _ := stateUnit(st, unitID)
		return fn(&env, st, us)
	})
}

// LevelUpState is the level-up roll a unit has in progress, nil when none.
func (s *LevelService) LevelUpState(ctx context.Context, user User, campaignID, unitID int64) (*LevelRoll, error) {
	id, err := s.myCompany(ctx, user, campaignID)
	if err != nil {
		return nil, err
	}
	st, err := s.companies.Read(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	us, ok := stateUnit(&st, unitID)
	if !ok {
		return nil, ErrNotFound
	}
	if us.Pending.Level == nil {
		return nil, nil
	}
	roll := cloneLevelRoll(*us.Pending.Level)
	return &roll, nil
}

// RollLevelUp makes a unit's level-up roll: its own D6 (or two 2D6 sums for a hero) or random ones.
func (s *LevelService) RollLevelUp(ctx context.Context, user User, campaignID, unitID int64, entered []int, thrown bool) (LevelRoll, error) {
	_, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return LevelRoll{}, err
	}
	if msg := settings.RollRefusal(slices.ContainsFunc(entered, func(n int) bool { return n > 0 }), thrown); msg != "" {
		return LevelRoll{}, Refuse("%s", msg)
	}
	var out LevelRoll
	err = s.mutate(ctx, user, campaignID, unitID, func(env *levelEnv, st *CompanyState, us *UnitState) error {
		u := env.unit
		switch {
		case u.Wanderer:
			return Refuse("A wanderer gains no experience.")
		case u.PathPending:
			return Refuse("%s has to choose a path before it can level up.", u.Title())
		case u.OwnPath:
			return Refuse("%s is on its own path: it earns no experience and never levels up.", u.Title())
		case u.PendingLevelUps() < 1:
			return Refuse("%s has no level-up roll waiting.", u.Title())
		case us.Pending.Level != nil:
			return Refuse("Finish the level-up roll in progress first.")
		}
		rules := env.rs.BC.Progression()
		if us.Progress == nil {
			us.Progress = &Progress{}
		}
		p := us.Progress
		var roll LevelRoll
		if u.IsHero() {
			roll = rules.RollHero(sheetOf(u, p, env.d.Roster, upgradesOf(env.rs, env.bcC, u)), levelLists(env.rs), entered, s.dice)
		} else {
			d6 := 0
			if len(entered) > 0 && entered[0] >= rules.WarriorDice.Min() && entered[0] <= rules.WarriorDice.Max() {
				d6 = entered[0]
			}
			if d6 == 0 {
				d6 = s.dice.Roll(rules.WarriorDice)
			}
			roll = env.warriorRoll(p, d6)
		}
		if roll.Kind == "warrior" {
			var err error
			out, err = env.settleWarrior(st, us, roll)
			return err
		}
		out = roll
		if roll.Done {
			p.Taken++
			return nil
		}
		us.Pending.Level = &roll
		return nil
	})
	return out, err
}

// ChooseLevelUp applies the choice the player made among the offers of the roll in progress. keys holds the choice (and, for a
// result that asks for the other roll too, the choice made on that one).
func (s *LevelService) ChooseLevelUp(ctx context.Context, user User, campaignID, unitID int64, keys []string) (LevelRoll, error) {
	if len(keys) == 0 || keys[0] == "" {
		return LevelRoll{}, Refuse("Choose one of the results offered.")
	}
	var out LevelRoll
	err := s.mutate(ctx, user, campaignID, unitID, func(env *levelEnv, st *CompanyState, us *UnitState) error {
		var err error
		out, err = env.choose(st, us, keys)
		return err
	})
	return out, err
}

func (env *levelEnv) choose(st *CompanyState, us *UnitState, keys []string) (LevelRoll, error) {
	key := keys[0]
	u, bcCo, rs := env.unit, env.bcC, env.rs
	roll := us.Pending.Level
	if roll == nil {
		return LevelRoll{}, Refuse("There is no level-up roll to choose from.")
	}
	var chosen LevelChoice
	chosenOffer := -1
	if roll.Upgrade != nil { // a hero upgrade replaces whichever result the hero would have taken
		for _, ch := range roll.Upgrade.Choices {
			if ch.Key == key {
				chosen, chosenOffer = ch, len(roll.Offers)
			}
		}
	}
	for i, o := range roll.Offers {
		for _, ch := range o.Choices {
			if ch.Key == key {
				chosen, chosenOffer = ch, i
			}
		}
	}
	if chosenOffer < 0 {
		return LevelRoll{}, Refuse("Choose one of the results offered.")
	}
	rules := rs.BC.Progression()
	if us.Progress == nil {
		us.Progress = &Progress{}
	}
	p := us.Progress
	kind, rest, _ := strings.Cut(key, ":")
	summary := ""
	switch kind {
	case "promo":
		var ai, ti int
		fmt.Sscanf(rest, "%d:%d", &ai, &ti)
		advs := rs.BC.Advancements(bcCo)
		if ai >= len(advs) || ti >= len(advs[ai].To) {
			return LevelRoll{}, ErrNotFound
		}
		to := advs[ai].To[ti]
		p.Base, p.Gear = to.Unit, to.Wargear
		name := promotedName(rs, bcCo, to)
		if name != to.Unit {
			p.Base = name // Ranger of Ithilien, Helminga: the company's name carries the rules and options the alias adds
		}
		if !env.unit.IsHero() { // a hero keeps what it carries; a warrior keeps only what its new profile lists
			dropUnlisted(env, us, p.Base, to.Wargear)
		}
		summary = fmt.Sprintf("%s is promoted to %s.", u.Title(), name)
	case "path":
		for _, pp := range rules.Paths { // the offer may be older than another hero's choice of a limited path
			if pp.Name == rest && pp.LimitPerCompany > 0 && heroesOnPath(env.d, rest) >= pp.LimitPerCompany {
				return LevelRoll{}, Refuse("The company already has a hero on the %s (at most %d).", rest, pp.LimitPerCompany)
			}
		}
		p.Hero, p.Path = true, rest
		profile, _ := rs.BC.Profile(bcCo.Name)
		promotion, promotes := profile.PromotionOnHero(u.Type)
		if path, ok := rules.Path(rest); ok && len(path.Start) > 0 { // the homebrew path starts on its own Might, Will and Fate
			p.Bonus = mergeBonus(p.Bonus, map[string]int{"might": path.Start["might"], "will": path.Start["will"], "fate": path.Start["fate"]})
		} else if profile.StartGains(u.Type) && !(promotes && !promotion.StatGain) { // a Ranger of the North already has its point of Might, Will and Fate
			p.Bonus = mergeBonus(p.Bonus, rs.BC.HeroBonus(nil))
		}
		summary = fmt.Sprintf("%s becomes a hero on the %s.", u.Title(), rest)
		if promotes { // it also changes profile: Ranger of Arnor to Ranger of the North, keeping some of its wargear
			p.Base = promotion.Becomes // what it had stays, except the wargear of its old profile (replacedWargear)
			summary = fmt.Sprintf("%s is promoted to %s and becomes a hero on the %s.", u.Title(), promotion.Becomes, rest)
		}
		if path, ok := rules.Path(rest); ok {
			p.Start = path.StartAction // the path's heroic action comes at once
			if path.StartAction != "" {
				summary += " It gains " + path.StartAction + "."
			}
			if path.StartPower { // a Sorcerer chooses its first magical power
				offer := LevelOffer{Text: "A Sorcerer adds a magical power of its choice to its spell list."}
				offer.Choices = rules.PowerChoices(HeroSheet{Powers: map[string]int{}})
				roll.Offers, roll.Stage, roll.Again = []LevelOffer{offer}, "startingPower", true
				roll.Notes = append(roll.Notes, summary)
				return cloneLevelRoll(*roll), nil
			}
		}
	default:
		if err := rules.Apply(p, key); err != nil {
			return LevelRoll{}, Refuse("%v", err)
		}
		summary = fmt.Sprintf("%s: %s.", u.Title(), chosen.Label)
	}
	if chosen.RollAgain && len(roll.Offers) > 1 && !roll.Again { // the hero also takes the other result, if it can still
		other := roll.Offers[1-chosenOffer]
		env.refresh(*st)
		if offer, ok := rules.OfferFor(sheetOf(env.unit, p, env.d.Roster, upgradesOf(rs, bcCo, u)), levelLists(rs), other.Roll); ok {
			if len(keys) > 1 { // chosen together with the first: apply it now
				i := slices.IndexFunc(offer.Choices, func(c LevelChoice) bool { return c.Key == keys[1] })
				if i < 0 {
					return LevelRoll{}, Refuse("The second result you chose is not available after the first.")
				}
				if err := rules.Apply(p, keys[1]); err != nil {
					return LevelRoll{}, Refuse("%v", err)
				}
				summary += " " + fmt.Sprintf("%s: %s.", u.Title(), offer.Choices[i].Label)
			} else { // chosen one at a time: the other result is offered next
				roll.Offers, roll.Again = []LevelOffer{offer}, true
				roll.Notes = append(roll.Notes, summary+" It also takes the other result.")
				return cloneLevelRoll(*roll), nil
			}
		}
	}
	p.Taken++
	us.Pending.Level = nil
	return LevelRoll{Kind: roll.Kind, Done: true, Summary: summary}, nil
}

// RerollLevelUp spends an Against the Odds re-roll (the company's Rerolls) on one result of the roll in progress. A warrior's D6
// is rolled again; a hero's pair is replaced by a new one (which: 0 or 1, the result to replace).
func (s *LevelService) RerollLevelUp(ctx context.Context, user User, campaignID, unitID int64, which int) (LevelRoll, error) {
	var out LevelRoll
	err := s.mutate(ctx, user, campaignID, unitID, func(env *levelEnv, st *CompanyState, us *UnitState) error {
		roll := us.Pending.Level
		switch {
		case roll == nil:
			return Refuse("There is no level-up roll to re-roll.")
		case st.Rerolls < 1:
			return Refuse("The company has no re-roll left.")
		case roll.Again:
			return Refuse("The result you chose is already being applied.")
		}
		if us.Progress == nil {
			us.Progress = &Progress{}
		}
		p := us.Progress
		rules := env.rs.BC.Progression()
		if roll.Kind == "warrior" {
			st.Rerolls--
			fresh := env.warriorRoll(p, s.dice.Roll(rules.WarriorDice))
			fresh.Notes = append(fresh.Notes, fmt.Sprintf("Re-rolled the %d (Against the Odds).", roll.D6))
			var err error
			out, err = env.settleWarrior(st, us, fresh)
			return err
		}
		if which < 0 || which >= len(roll.Offers) {
			return Refuse("Choose which result to re-roll.")
		}
		st.Rerolls--
		sheet, lists := sheetOf(env.unit, p, env.d.Roster, upgradesOf(env.rs, env.bcC, env.unit)), levelLists(env.rs)
		spec := rules.HeroDice
		if hp, ok := rules.Path(sheet.Path); ok {
			spec = hp.PathDice
		}
		for try := 0; try < 60; try++ {
			dice := RollDice(s.dice, spec)
			total := sumOf(dice)
			if offer, ok := rules.OfferFor(sheet, lists, total); ok {
				old := roll.Offers[which]
				offer.Pair = old.Pair
				roll.Offers[which] = offer
				if pi := old.Pair - 1; pi >= 0 && pi < len(roll.Pairs) {
					roll.Pairs[pi] = HeroPair{Dice: dice, Sum: total, Usable: true}
				}
				roll.Notes = append(roll.Notes, fmt.Sprintf("Re-rolled the %d: now a %d (Against the Odds).", old.Roll, total))
				break
			}
		}
		out = cloneLevelRoll(*roll)
		return nil
	})
	return out, err
}

// ownPath is the homebrew path the rolling unit follows when it is On Its Own Path in a campaign with the option.
func (env *levelEnv) ownPath() (string, bool) {
	profile, _ := env.rs.BC.Profile(env.bcC.Name)
	if !ownPathLevels(env.rs, env.settings, profile, env.unit.Type) {
		return "", false
	}
	return env.rs.BC.Progression().CustomPath, true
}

// settleWarrior stores a warrior's roll, or finishes it: a roll that needs no choice ends the level-up, and one of a unit on its own
// path that came up Hero in the Making takes that path at once.
func (env *levelEnv) settleWarrior(st *CompanyState, us *UnitState, roll LevelRoll) (LevelRoll, error) {
	p := us.Progress
	switch {
	case roll.Done:
		p.Taken++
		us.Pending.Level = nil
		return roll, nil
	case roll.Auto:
		us.Pending.Level = &roll
		done, err := env.choose(st, us, []string{roll.Offers[0].Choices[0].Key})
		if err != nil {
			return roll, err
		}
		done.D6, done.Result, done.Notes = roll.D6, roll.Result, roll.Notes
		return done, nil
	}
	us.Pending.Level = &roll
	return roll, nil
}

// warriorRoll resolves a warrior's D6: nothing, a promotion to choose, or a hero in the making.
func (env *levelEnv) warriorRoll(p *Progress, d6 int) LevelRoll {
	rules := env.rs.BC.Progression()
	row, _ := rules.WarriorFor(d6)
	roll := LevelRoll{Kind: "warrior", D6: d6, Result: row.Key}
	u := env.unit
	switch row.Key {
	case "promoted":
		choices := promotionChoices(env.rs, env.bcC, u)
		if len(choices) > 0 {
			roll.Offers = []LevelOffer{{Roll: d6, Text: row.Text, Choices: choices}}
		} else {
			roll.Done, roll.Summary = true, noPromotion(u, p, rules)
		}
	case "heroInTheMaking":
		offer := LevelOffer{Roll: d6, Text: row.Text}
		if custom, ok := env.ownPath(); ok { // a unit on its own path takes its path at once: nothing to choose, no promotion to forgo for
			offer.Choices = []LevelChoice{{Key: "path:" + custom, Label: custom, Group: "Its own path"}}
			roll.Offers, roll.Auto = []LevelOffer{offer}, true
			return roll
		}
		for _, path := range rules.Paths {
			if path.LimitPerCompany > 0 && heroesOnPath(env.d, path.Name) >= path.LimitPerCompany {
				roll.Notes = append(roll.Notes, fmt.Sprintf("The company already has a hero on the %s (at most %d): it is not offered.", path.Name, path.LimitPerCompany))
				continue
			}
			offer.Choices = append(offer.Choices, LevelChoice{Key: "path:" + path.Name, Label: path.Name, Group: "Choose its path"})
		}
		for _, ch := range promotionChoices(env.rs, env.bcC, u) {
			ch.Label = "Forgo the Hero's Journey: " + ch.Label
			ch.Group = "Take the promotion instead"
			offer.Choices = append(offer.Choices, ch)
		}
		roll.Offers = []LevelOffer{offer}
	default:
		roll.Done, roll.Summary = true, fmt.Sprintf("%s rolled a %d: no change.", u.Title(), d6)
	}
	return roll
}

// PathOptions says which paths a hero without one can take now.
func (s *LevelService) PathOptions(ctx context.Context, user User, campaignID, unitID int64) (PathOffer, error) {
	id, err := s.myCompany(ctx, user, campaignID)
	if err != nil {
		return PathOffer{}, err
	}
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return PathOffer{}, err
	}
	st, err := s.companies.Read(ctx, campaignID, id)
	if err != nil {
		return PathOffer{}, err
	}
	env, err := newLevelEnv(rs, settings, st, unitID)
	if err != nil {
		return PathOffer{}, err
	}
	return env.pathOffer()
}

func (env *levelEnv) pathOffer() (PathOffer, error) {
	u := env.unit
	if !u.PathPending {
		return PathOffer{}, Refuse("%s has a path already.", u.Title())
	}
	offer := PathOffer{Unit: u}
	for _, path := range env.rs.BC.Progression().Paths {
		opt := PathOption{Name: path.Name, StartAction: path.StartAction, StartPower: path.StartPower, Description: path.Description, Summary: path.Summary, Mastery: path.Mastery, Rules: path.SpecialRules}
		if path.LimitPerCompany > 0 && heroesOnPath(env.d, path.Name) >= path.LimitPerCompany {
			opt.Blocked = fmt.Sprintf("the company already has a hero on it (at most %d)", path.LimitPerCompany)
		}
		offer.Paths = append(offer.Paths, opt)
	}
	offer.Powers = env.rs.BC.Progression().PowerChoices(HeroSheet{Powers: map[string]int{}})
	return offer, nil
}

// ChoosePath gives an officer the path the player chose (and a Sorcerer its first power): its heroic action comes with it.
func (s *LevelService) ChoosePath(ctx context.Context, user User, campaignID, unitID int64, path, power string) error {
	return s.mutate(ctx, user, campaignID, unitID, func(env *levelEnv, st *CompanyState, us *UnitState) error {
		offer, err := env.pathOffer()
		if err != nil {
			return err
		}
		i := slices.IndexFunc(offer.Paths, func(o PathOption) bool { return o.Name == path })
		switch {
		case i < 0:
			return Refuse("Choose one of the paths.")
		case offer.Paths[i].Blocked != "":
			return Refuse("%s: %s.", path, offer.Paths[i].Blocked)
		case offer.Paths[i].StartPower && !slices.ContainsFunc(offer.Powers, func(p LevelChoice) bool { return p.Key == "power:"+power }):
			return Refuse("Choose the first magical power of the Sorcerer.")
		}
		if !offer.Paths[i].StartPower {
			power = ""
		}
		*us = WithPath(*us, path, power, env.rs.BC.Progression())
		return nil
	})
}

// promotedName is what the company calls the unit a promotion makes: its own name where that changes the unit, else the unit.
func promotedName(rs *RuleSet, bcCo BCCompany, to AdvancementTarget) string {
	if profile, ok := rs.BC.Profile(bcCo.Name); ok && to.Label != "" && profile.ChangesUnit(to.Label) {
		return to.Label
	}
	return to.Unit
}

// promotionChoices lists the promotions of the company that the unit can take.
func promotionChoices(rs *RuleSet, bcCo BCCompany, u CompanyUnit) []LevelChoice {
	var out []LevelChoice
	for i, adv := range rs.BC.Advancements(bcCo) {
		if !adv.Matches(u) {
			continue
		}
		for j, to := range adv.To {
			if to.AlreadyIs(u) {
				continue
			}
			label := promotedName(rs, bcCo, to)
			if len(to.Wargear) > 0 {
				label += " with " + strings.Join(to.Wargear, ", ")
			}
			out = append(out, LevelChoice{Key: fmt.Sprintf("promo:%d:%d", i, j), Label: label, Group: "Promotions"})
		}
	}
	return out
}

// noPromotion is what a warrior gets when its company has no promotion for it: the stats of campaign.progression.warrior.noPromotion improve by its amounts, up to its maximum.
func noPromotion(u CompanyUnit, p *Progress, rules ProgressionRules) string {
	var gained []string
	for _, stat := range rules.NoPromotion {
		for _, a := range u.Stats {
			if strings.ToLower(a.Name) == stat && a.Value > rules.NoPromotionTo {
				// a lower Courage or Intelligence is better: it improves by the amount of the rules, down to the maximum
				step := min(rules.NoPromotionBy[stat], a.Value-rules.NoPromotionTo)
				if p.Bonus == nil {
					p.Bonus = map[string]int{}
				}
				p.Bonus[stat] -= step
				gained = append(gained, fmt.Sprintf("%s %d+", a.Name, a.Value-step))
			}
		}
	}
	if len(gained) == 0 {
		return u.Title() + " has no promotion to take and its Courage and Intelligence are as good as they go."
	}
	return u.Title() + " has no promotion in the company: " + strings.Join(gained, " and ") + "."
}

// heroesOnPath counts the company's heroes on a path.
func heroesOnPath(d CompanyDetail, path string) int {
	n := 0
	for _, u := range d.Roster {
		if u.Path != nil && u.Path.Name == path {
			n++
		}
	}
	return n
}

// sheetOf is what the rules need to know about a hero.
func sheetOf(u CompanyUnit, p *Progress, roster []CompanyUnit, upgrades []HeroUpgrade) HeroSheet {
	sheet := HeroSheet{Stats: map[string]SheetStat{}, Gear: map[string]int{}, Rules: map[string]bool{}, Actions: map[string]bool{}, Powers: map[string]int{}, Casting: map[string]int{}, PowerNames: map[string]string{}}
	for _, a := range u.Stats {
		sheet.Stats[strings.ToLower(a.Name)] = SheetStat{Value: a.Value, Boost: a.Boost}
		sheet.Gear[strings.ToLower(a.Name)] = a.Gear
	}
	for _, r := range u.Rules {
		sheet.Rules[strings.ToLower(r.Name)] = true
	}
	if u.Path != nil {
		sheet.Path = u.Path.Name
		for _, r := range u.Path.Rules {
			sheet.Rules[strings.ToLower(r.Name)] = true
		}
		for _, pw := range u.Path.Powers { // spells learned through the path
			k := strings.ToLower(pw.Name)
			sheet.Powers[k], sheet.Casting[k], sheet.PowerNames[k] = 0, pw.Casting, pw.Name
			if p != nil {
				sheet.Powers[k] = p.Cast[k]
			}
		}
		for _, a := range u.Path.HeroicActions {
			sheet.Actions[strings.ToLower(a.Name)] = true
		}
	}
	if p != nil {
		sheet.MinorGained, sheet.ActionsGained = len(p.Minor), len(p.Actions)
	}
	for _, up := range upgrades { // company hero upgrades the hero does not have yet
		have := false
		if p != nil {
			have = slices.Contains(p.Upgrades, up.Name)
		}
		if !have {
			sheet.Upgrades = append(sheet.Upgrades, up)
		}
	}
	sheet.Weapons = slices.Clone(u.PoisonableWeapons) // weapons that are not poisoned yet (by a rule, a level-up or Envenom Weapon)
	// other heroes of the company, not already the target of this unit's Combat Synergy
	aimed := map[int64]bool{}
	if p != nil {
		for _, m := range p.Minor {
			if _, id, ok := strings.Cut(m, "|unit:"); ok {
				n, _ := strconv.ParseInt(id, 10, 64)
				aimed[n] = true
			}
		}
	}
	for _, h := range roster {
		if h.ID != u.ID && h.IsHero() && !aimed[h.ID] {
			sheet.Heroes = append(sheet.Heroes, SheetTarget{ID: h.ID, Name: h.Title()})
		}
	}
	return sheet
}

func levelLists(rs *RuleSet) LevelLists {
	return LevelLists{MinorRules: rs.BC.Progression().MinorRules, HeroicActions: rs.HeroicActionNames(), ActionName: rs.HeroicActionName}
}

// upgradesOf are the hero upgrades of a company that a unit can take.
func upgradesOf(rs *RuleSet, bcCo BCCompany, u CompanyUnit) []HeroUpgrade {
	p, ok := rs.BC.Profile(bcCo.Name)
	if !ok {
		return nil
	}
	vanilla, _, _ := rs.ResolveUnit(p, u.Type) // a company's own name for a profile (Tookish Hunter) is that profile's race too
	var out []HeroUpgrade
	for _, h := range p.HeroUpgrades {
		if h.Allows(vanilla) { // some upgrades belong to one race (a Hobbit may take both of the Wanderers' upgrades, the others one)
			out = append(out, h)
		}
	}
	return out
}
