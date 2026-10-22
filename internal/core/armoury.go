package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// ArmouryService is the armoury of a company: gear, creatures, wanderers and the Baggage Pony's pack. Purchases are
// companyRepository.Update calls, so they are atomic and serialised per company: two concurrent purchases cannot spend the
// same influence twice. What can be bought is worked out from BuildDetail of the state the purchase is made on.
type ArmouryService struct{ companyDeps }

// NewArmouryService builds the service. rules is the loader (*Rules).
func NewArmouryService(campaigns campaignRepository, companies companyRepository, rules ruleSource) *ArmouryService {
	return &ArmouryService{companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: RandomDice{}}}
}

// shop is a company as the armoury sees it: its rules, its page data and its Battle Companies profile.
type shop struct {
	rs  *RuleSet
	d   CompanyDetail
	bcC BCCompany
}

func newShop(rs *RuleSet, settings CampaignSettings, st CompanyState) shop {
	d := BuildDetail(rs, settings, st)
	bcCo, _ := rs.BC.Company(st.Company.Faction)
	return shop{rs: rs, d: d, bcC: bcCo}
}

func (sh shop) unit(id int64) (CompanyUnit, bool) {
	i := slices.IndexFunc(sh.d.Roster, func(u CompanyUnit) bool { return u.ID == id })
	if i < 0 {
		return CompanyUnit{}, false
	}
	return sh.d.Roster[i], true
}

// readShop builds the shop of a company without locking it.
func (a *ArmouryService) readShop(ctx context.Context, campaignID, companyID int64) (shop, error) {
	rs, settings, err := a.ruleset(ctx, campaignID)
	if err != nil {
		return shop{}, err
	}
	st, err := a.companies.Read(ctx, campaignID, companyID)
	if err != nil {
		return shop{}, err
	}
	return newShop(rs, settings, st), nil
}

// stateUnit finds a unit of the state by id.
func stateUnit(st *CompanyState, id int64) (*UnitState, bool) {
	i := slices.IndexFunc(st.Units, func(u UnitState) bool { return u.ID == id })
	if i < 0 {
		return nil, false
	}
	return &st.Units[i], true
}

// lowerFirst makes a reason fit into a sentence.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// canPay says how the company could pay a price: in influence, or in Gold where the thing has a Gold price (0: none).
func canPay(d CompanyDetail, influence, gold int) (inf, goldOK bool) {
	return influence <= d.Influence, gold > 0 && gold <= d.Gold
}

// costReason says why the company cannot afford something.
func costReason(d CompanyDetail, influence, gold int) string {
	if gold > 0 {
		return fmt.Sprintf("Costs %d influence or %d gold, the company has %d and %d", influence, gold, d.Influence, d.Gold)
	}
	return fmt.Sprintf("Costs %d influence, the company has %d", influence, d.Influence)
}

// pay takes a price from the state in influence or, for pay == "gold", in Gold; the caller says whether each way is open.
func pay(st *CompanyState, how string, name string, influence, gold int, inf, goldOK bool) error {
	if how == "gold" {
		if !goldOK {
			return Refuse("%s cannot be paid in gold now.", name)
		}
		st.Gold -= gold
		return nil
	}
	if !inf {
		return Refuse("%s cannot be paid in influence now.", name)
	}
	st.Influence -= influence
	return nil
}

// Armoury lists what the unit could buy. An item a rule forbids this unit (not a hero, not cavalry, not an option of its
// profile...) is left out; one that only lacks influence or a free slot is listed with that reason.
func (a *ArmouryService) Armoury(ctx context.Context, campaignID, companyID, unitID int64) ([]ArmouryItem, error) {
	sh, err := a.readShop(ctx, campaignID, companyID)
	if err != nil {
		return nil, err
	}
	return sh.items(unitID)
}

func (sh shop) items(unitID int64) ([]ArmouryItem, error) {
	unit, ok := sh.unit(unitID)
	if !ok {
		return nil, ErrNotFound
	}
	var out []ArmouryItem
	for _, offer := range sh.offers() {
		reason, blocker := sh.whyNot(unit, offer)
		item := ArmouryItem{GearOffer: offer, Icon: IconFor(offer.Name, offer.Classes, offer.Size), Reason: reason, Blocker: blocker, GoldPrice: sh.goldPrice(unit, offer)}
		item.PayInfluence, item.PayGold = canPay(sh.d, offer.Cost, item.GoldPrice)
		if item.Offered() {
			out = append(out, item)
		}
	}
	return out, nil
}

// ArmouryEntries lists every item of the company's armoury, whether or not any unit can take it now, with for each unit
// whether it can buy it and, if not, why.
func (a *ArmouryService) ArmouryEntries(ctx context.Context, campaignID, companyID int64) ([]ArmouryEntry, error) {
	sh, err := a.readShop(ctx, campaignID, companyID)
	if err != nil {
		return nil, err
	}
	var out []ArmouryEntry
	for _, offer := range sh.offers() {
		entry := ArmouryEntry{GearOffer: offer, Icon: IconFor(offer.Name, offer.Classes, offer.Size)}
		for _, u := range sh.d.Roster {
			reason, blocker := sh.whyNot(u, offer)
			opt := UnitOption{ID: u.ID, Title: u.Title(), Type: u.Type, Role: u.Role, Kind: u.Kind, Reason: reason, Blocker: blocker, GoldPrice: sh.goldPrice(u, offer)}
			opt.PayInfluence, opt.PayGold = canPay(sh.d, offer.Cost, opt.GoldPrice)
			entry.Units = append(entry.Units, opt)
		}
		out = append(out, entry)
	}
	return out, nil
}

// goldPrice is what the unit pays in Gold for the item: its rating. A warrior is worth its MESBG points, so for wargear that is
// one of its profile's options on its own the price is that option's points (a Dwarf Ranger pays 1 for a Dwarf Longbow, the
// armoury says 2); every other case pays the armoury's price. 0 stays 0 (not for Gold).
func (sh shop) goldPrice(u CompanyUnit, o GearOffer) int {
	price := o.Gold.For(u.IsHero())
	if price == 0 || u.IsHero() || o.Size != "" {
		return price
	}
	if v, ok := sh.profileOf(u.Type); ok {
		for _, opt := range v.Options {
			if items := OptionItems(opt.Name); len(items) == 1 && !strings.HasPrefix(strings.ToLower(opt.Name), "exchange") && GearMatchesOption(o, opt.Name) && opt.Points > 0 {
				return opt.Points
			}
		}
	}
	return price
}

// profileOf is the vanilla profile a unit type stands for: a vanilla name, or the company's own label for one (Battlin'
// Brandybuck is a Hobbit Militia), whose options are those of the profile it labels.
func (sh shop) profileOf(unit string) (VanillaUnit, bool) {
	profile, _ := sh.rs.BC.Profile(sh.bcC.Name)
	v, _, ok := resolveUnit(sh.rs, profile, unit)
	return v, ok
}

// legalWargear applies the rule that a warrior may only buy wargear matching one of its profile's options, and a hero any
// wargear that some profile of its company's charts has. A warrior whose profile has no options buys no wargear (the Battle
// Companies armoury rule: "Warrior models may only spend influence in order to match one of their profile's Options");
// only a unit whose profile is unknown is not restricted.
func (sh shop) legalWargear(u CompanyUnit, o GearOffer) bool {
	var options []ChoiceOption
	if !u.IsHero() {
		v, ok := sh.profileOf(u.Type)
		if !ok {
			return true
		}
		options = v.Options
	} else {
		options = sh.heroOptions(u)
	}
	return slices.ContainsFunc(options, func(opt ChoiceOption) bool { return GearMatchesOption(o, opt.Name) })
}

// heroOptions are the options a hero may take from: those of the profiles in its company's charts, promotions and starting
// company (a hero may buy any wargear available to a member of its company, start units included), what those profiles start
// with, and its own profile.
func (sh shop) heroOptions(u CompanyUnit) []ChoiceOption {
	var options []ChoiceOption
	seen := map[string]bool{}
	add := func(unit string) {
		if v, ok := sh.profileOf(unit); ok && !seen[unit] {
			seen[unit] = true
			options = append(options, v.Options...)
			for _, w := range v.Wargear { // what a profile starts with counts as available to it too (a knight's horse)
				options = append(options, ChoiceOption{Name: w})
			}
		}
	}
	for _, chart := range [][]ChartRow{sh.bcC.Reinforcements, sh.bcC.SpecialChart} {
		for _, r := range chart {
			add(r.Result.Unit)
			for _, c := range r.Result.Options {
				add(c.Unit)
			}
		}
	}
	for _, adv := range sh.rs.BC.Advancements(sh.bcC) { // "including those that could be promoted"
		for _, to := range adv.To {
			add(to.Unit)
			for _, w := range to.Wargear {
				options = append(options, ChoiceOption{Name: w})
			}
		}
	}
	if profile, ok := sh.rs.BC.Profile(sh.bcC.Name); ok {
		for _, s := range profile.Start {
			add(s.Unit)
			for _, w := range s.Wargear {
				options = append(options, ChoiceOption{Name: w})
			}
		}
		for _, p := range profile.Promotions {
			add(p.Becomes)
		}
	}
	add(u.Type)
	return options
}

// beastMayCarry applies the Battle Companies rule that a model with the Beast keyword takes no additional wargear and carries
// only equipment that exclusively gives a special rule (Trophy Pelt, Badge of Courage): an item that grants exactly one rule.
func beastMayCarry(o GearOffer) bool {
	return o.Size != "" && len(o.Grants) == 1 && !o.WeaponModifier
}

// whyNot says why the unit cannot buy the item now (empty when it can) and what stops it.
func (sh shop) whyNot(u CompanyUnit, o GearOffer) (string, Blocker) {
	if o.ArmourStage > 0 {
		return sh.armourReason(u, o)
	}
	for _, e := range u.Equipment {
		if SameItem(e.Name, e.Spellings, o.Name, o.AlsoCalled) {
			return "Already carried", BlockedByRule
		}
	}
	for _, r := range o.Restrictions {
		switch r {
		case "hero":
			if !u.IsHero() {
				return "Heroes only", BlockedByRule
			}
		case "cavalry":
			if !u.isCavalry() {
				return "Cavalry only", BlockedByRule
			}
		case "evil", "good":
			if sh.bcC.Alignment != r {
				return strings.ToUpper(r[:1]) + r[1:] + " companies only", BlockedByRule
			}
		}
	}
	if reason := sh.elvesOnly(u, o); reason != "" {
		return reason, BlockedByRule
	}
	if u.Kind == KindBeast && !beastMayCarry(o) { // Battle Companies armoury: "Beasts"
		return "Beasts carry no wargear and only equipment that gives a special rule", BlockedByRule
	}
	if o.Only != "" { // a unique item for a kind of model ("Hobbit Heroes: Pony") is not one of the profile's options
		if reason := sh.uniqueReason(u, o); reason != "" {
			return reason, BlockedByRule
		}
	} else if o.Size == "" && !sh.legalWargear(u, o) { // wargear, not equipment
		return "Not an option of its profile", BlockedByRule
	}
	for _, l := range sh.d.Limits {
		ranged := (l.ID == "bow" && (slices.Contains(o.Classes, EquipBow) || slices.Contains(o.Classes, EquipCrossbow))) ||
			(l.ID == "throwing" && slices.Contains(o.Classes, EquipThrowing)) ||
			(l.ID == "cavalry" && slices.Contains(o.Classes, EquipMount) && !u.isCavalry())
		if ranged && l.Full() {
			return fmt.Sprintf("%s limit reached (%d of %d)", l.Label, l.Used, l.Max), BlockedByLimit
		}
	}
	slots := u.Slots()
	switch {
	case o.Size == "large" && !slots.LargeFree():
		return "No free large slot", BlockedBySlots
	case o.Size == "small" && !slots.SmallFree():
		return "No free small slot", BlockedBySlots
	}
	if inf, gold := canPay(sh.d, o.Cost, o.Gold.For(u.IsHero())); !inf && !gold {
		return costReason(sh.d, o.Cost, o.Gold.For(u.IsHero())), BlockedByInfluence
	}
	return "", ""
}

// uniqueReason says why a unit cannot take a company's unique item that names who may have it ("Hobbit Heroes", "Corsair",
// "Mahûd") or how many may carry it; empty when it can.
func (sh shop) uniqueReason(u CompanyUnit, o GearOffer) string {
	v, known := sh.profileOf(u.Type)
	switch {
	case len(o.OnlyUnits) > 0: // by unit identity (War Camel: the Mahûd Warrior, not every Mahûd); a company's own name for a profile counts as the profile
		if !slices.ContainsFunc(o.OnlyUnits, func(n string) bool { return strings.EqualFold(n, u.Type) || (known && strings.EqualFold(n, v.Name)) }) {
			return strings.Join(o.OnlyUnits, ", ") + " only"
		}
	case o.OnlyKind != nil:
		if o.OnlyKind.HeroesOnly && !u.IsHero() {
			return "Heroes only"
		}
		if known && o.OnlyKind.Race != "" && !hasKeyword(v, o.OnlyKind.Race) {
			return o.OnlyKind.Race + " only"
		}
	default:
		only := strings.TrimSpace(o.Only)
		for _, suffix := range []string{" heroes", " hero"} {
			if rest, found := strings.CutSuffix(strings.ToLower(only), suffix); found {
				if !u.IsHero() {
					return "Heroes only"
				}
				only = only[:len(rest)]
			}
		}
		if known && only != "" && !(HeroUpgrade{Restriction: only}).Allows(v) {
			return only + " only"
		}
	}
	if o.Limit > 0 {
		carried := 0
		for _, r := range sh.d.Roster {
			if slices.ContainsFunc(r.Equipment, func(e Equipment) bool { return strings.EqualFold(e.Name, o.Name) }) {
				carried++
			}
		}
		if carried >= o.Limit {
			return fmt.Sprintf("Limit %d reached", o.Limit)
		}
	}
	return ""
}

// BuyEquipment buys an item for a unit of the user's company with its influence (pay "") or Gold (pay "gold"). What the rules
// do not allow is an error wrapping ErrRefused with the reason; an item the unit's rules never offer is ErrNotFound.
func (a *ArmouryService) BuyEquipment(ctx context.Context, user User, campaignID, unitID int64, item, how string) error {
	id, err := a.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	rs, settings, err := a.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	return a.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		if err := st.Locked(); err != nil {
			return err
		}
		sh := newShop(rs, settings, *st)
		items, err := sh.items(unitID)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(items, func(it ArmouryItem) bool { return strings.EqualFold(it.Name, item) })
		if i < 0 { // not offered to this unit: an item of the armoury is refused with the rule that keeps it out, anything else is unknown
			unit, _ := sh.unit(unitID)
			for _, offer := range sh.offers() {
				if strings.EqualFold(offer.Name, item) {
					if why, _ := sh.whyNot(unit, offer); why != "" {
						return Refuse("%s: %s", offer.Name, lowerFirst(why))
					}
					return Refuse("%s cannot take %s.", unit.Title(), offer.Name)
				}
			}
			return ErrNotFound
		}
		it := items[i]
		if !it.Buyable() {
			return Refuse("%s: %s", it.Name, lowerFirst(it.Reason))
		}
		if err := pay(st, how, it.Name, it.Cost, it.GoldPrice, it.PayInfluence, it.PayGold); err != nil {
			return err
		}
		u, _ := stateUnit(st, unitID)
		if it.ArmourStage > 0 {
			unit, _ := sh.unit(unitID)
			upgradeArmour(sh, u, unit, it.GearOffer)
			return nil
		}
		u.Bought = append(u.Bought, it.Name)
		return nil
	})
}

// DropEquipment makes a unit drop an item it carries, if the Battle Companies rules allow it (DropReason): the item is gone
// for good, nothing is refunded, its rating leaves the unit's points, and it stops counting against slots and limits, which
// can make room for another unit to buy a limited item.
func (a *ArmouryService) DropEquipment(ctx context.Context, user User, campaignID, unitID int64, item string) error {
	id, err := a.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	rs, settings, err := a.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	return a.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		if err := st.Locked(); err != nil {
			return err
		}
		sh := newShop(rs, settings, *st)
		unit, ok := sh.unit(unitID)
		if !ok {
			return ErrNotFound
		}
		j := slices.IndexFunc(unit.Equipment, func(e Equipment) bool { return e.Name == item })
		if j < 0 {
			return Refuse("%s does not carry %s.", unit.Title(), item)
		}
		if why := unit.Equipment[j].Drop; why != "" {
			return Refuse("%s cannot drop %s: %s.", unit.Title(), item, lowerFirst(why))
		}
		u, _ := stateUnit(st, unitID)
		u.Dropped = append(u.Dropped, item)
		return nil
	})
}

func isOfficer(rules CreatureRules, u CompanyUnit) bool {
	return slices.Contains(rules.AttachTo, string(u.Role))
}

// creatureReason is why an officer cannot take a creature now; empty when it can.
func creatureReason(cr CreatureRules, d CompanyDetail, o CreatureOffer, u CompanyUnit) string {
	switch {
	case u.Creature != nil:
		return "Already has " + u.Creature.Name
	case len(d.Creatures) >= cr.MaxPerCompany:
		return fmt.Sprintf("The company already has %d creatures", cr.MaxPerCompany)
	}
	if inf, gold := canPay(d, o.Cost, o.Gold); !inf && !gold {
		return costReason(d, o.Cost, o.Gold)
	}
	return ""
}

// ArmouryCreatures lists the creatures of the armoury with, for each officer, whether they can take it now.
func (a *ArmouryService) ArmouryCreatures(ctx context.Context, campaignID, companyID int64) ([]ArmouryCreature, error) {
	sh, err := a.readShop(ctx, campaignID, companyID)
	if err != nil {
		return nil, err
	}
	return sh.creatures(), nil
}

func (sh shop) creatures() []ArmouryCreature {
	cr, offers := sh.rs.BC.Creatures()
	var out []ArmouryCreature
	for _, o := range offers {
		entry := ArmouryCreature{CreatureOffer: o, StatBlock: creatureStats(sh.rs, o)}
		entry.PayInfluence, entry.PayGold = canPay(sh.d, o.Cost, o.Gold)
		for _, u := range sh.d.Roster {
			if !isOfficer(cr, u) {
				continue
			}
			why := creatureReason(cr, sh.d, o, u)
			blocker := Blocker("")
			if strings.HasPrefix(why, "Costs") {
				blocker = BlockedByInfluence
			} else if why != "" {
				blocker = BlockedByRule
			}
			entry.Owners = append(entry.Owners, UnitOption{ID: u.ID, Title: u.Title(), Type: u.Type, Role: u.Role, Kind: u.Kind, Reason: why, Blocker: blocker})
		}
		out = append(out, entry)
	}
	return out
}

// BuyCreature buys a creature for an officer (the leader or a sergeant) of the user's company.
func (a *ArmouryService) BuyCreature(ctx context.Context, user User, campaignID, ownerID int64, name, how string) error {
	id, err := a.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	rs, settings, err := a.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	return a.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		if err := st.Locked(); err != nil {
			return err
		}
		list := newShop(rs, settings, *st).creatures()
		k := slices.IndexFunc(list, func(c ArmouryCreature) bool { return c.Name == name })
		if k < 0 {
			return ErrNotFound
		}
		j := slices.IndexFunc(list[k].Owners, func(o UnitOption) bool { return o.ID == ownerID })
		if j < 0 {
			return Refuse("Only the leader and the sergeants can have a creature.")
		}
		if o := list[k].Owners[j]; !o.Able() {
			return Refuse("%s: %s.", name, lowerFirst(o.Reason))
		}
		if err := pay(st, how, name, list[k].Cost, list[k].Gold, list[k].PayInfluence, list[k].PayGold); err != nil {
			return err
		}
		u, _ := stateUnit(st, ownerID)
		u.Creature = name
		return nil
	})
}

// ArmouryWanderers lists the wanderers of the armoury and whether the company can hire them now.
func (a *ArmouryService) ArmouryWanderers(ctx context.Context, campaignID, companyID int64) ([]ArmouryWanderer, error) {
	sh, err := a.readShop(ctx, campaignID, companyID)
	if err != nil {
		return nil, err
	}
	return sh.wanderers(), nil
}

func (sh shop) wanderers() []ArmouryWanderer {
	wr, offers := sh.rs.BC.Wanderers()
	hired := 0
	for _, u := range sh.d.Roster {
		if u.Wanderer {
			hired++
		}
	}
	var out []ArmouryWanderer
	for _, o := range offers {
		entry := ArmouryWanderer{WandererOffer: o, StatBlock: sh.rs.Vanilla.Profile(o.AsUnit(), true, nil)}
		entry.PayInfluence, entry.PayGold = canPay(sh.d, o.Cost, o.Gold)
		switch {
		case hired >= max(1, wr.MaxAtOnce):
			entry.Reason = "The company already has a wanderer"
		case sh.d.ModelCount() >= sh.d.MaxUnits:
			entry.Reason = fmt.Sprintf("The company is full (%d of %d units)", sh.d.ModelCount(), sh.d.MaxUnits)
		case !entry.PayInfluence && !entry.PayGold:
			entry.Reason = costReason(sh.d, o.Cost, o.Gold)
		default:
			if why, _ := companyBlockFor(sh.rs, sh.bcC, sh.d.Roster, o.AsUnit()); why != "" {
				entry.Reason = "The company cannot take " + o.Name + ": " + why
			}
		}
		out = append(out, entry)
	}
	return out
}

// HireWanderer hires a wanderer for the user's company: it joins as a hero and counts towards the unit limit.
func (a *ArmouryService) HireWanderer(ctx context.Context, user User, campaignID int64, name, how string) error {
	id, err := a.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	rs, settings, err := a.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	return a.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		if err := st.Locked(); err != nil {
			return err
		}
		list := newShop(rs, settings, *st).wanderers()
		k := slices.IndexFunc(list, func(w ArmouryWanderer) bool { return w.Name == name })
		if k < 0 {
			return ErrNotFound
		}
		if !list[k].Hirable() {
			return Refuse("%s: %s.", name, lowerFirst(list[k].Reason))
		}
		if err := pay(st, how, name, list[k].Cost, list[k].Gold, list[k].PayInfluence, list[k].PayGold); err != nil {
			return err
		}
		st.Units = append(st.Units, UnitState{Base: name, Wanderer: name, Role: RoleHero}) // the repository numbers it
		return nil
	})
}

// DismissWanderer sends a wanderer of the user's company away; the influence is not returned.
func (a *ArmouryService) DismissWanderer(ctx context.Context, user User, campaignID, unitID int64) error {
	id, err := a.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	return a.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		if err := st.Locked(); err != nil {
			return err
		}
		i := slices.IndexFunc(st.Units, func(u UnitState) bool { return u.ID == unitID && u.Wanderer != "" })
		if i < 0 {
			return Refuse("Only a wanderer can be dismissed.")
		}
		st.Units = slices.Delete(st.Units, i, i+1)
		return nil
	})
}

// Carry stores gear of a unit with the Baggage Pony, takes it back out or swaps it, free of charge. What a unit gets must be
// something it could buy (its wargear, slots and the company limits), mounts cannot be carried, and the pack holds a limited
// number of pieces. Only gear that could be dropped can be stored.
func (a *ArmouryService) Carry(ctx context.Context, user User, campaignID, unitID int64, act CarryAction) error {
	id, err := a.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	rs, settings, err := a.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	return a.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		if err := st.Locked(); err != nil {
			return err
		}
		sh := newShop(rs, settings, *st)
		if sh.d.Pack == nil {
			return Refuse("The company has no Baggage Pony.")
		}
		unit, ok := sh.unit(unitID)
		if !ok {
			return ErrNotFound
		}
		store, retrieve := act.Kind == "store" || act.Kind == "swap", act.Kind == "retrieve" || act.Kind == "swap"
		if !store && !retrieve {
			return Refuse("Choose what to do with the pack.")
		}
		var carry *CarryFeature
		for _, cr := range sh.d.Creatures {
			if cr.Carry != nil {
				carry = cr.Carry
			}
		}
		if store {
			j := slices.IndexFunc(unit.Equipment, func(e Equipment) bool { return e.Name == act.Item })
			if j < 0 {
				return Refuse("%s does not carry %s.", unit.Title(), act.Item)
			}
			e := unit.Equipment[j]
			switch {
			case e.Upgrade || slices.ContainsFunc(e.Classes(), func(c EquipmentClass) bool { return slices.Contains(carry.Excludes, c) }):
				return Refuse("%s cannot be carried by the pony.", e.Name)
			case e.Drop != "":
				return Refuse("%s cannot be stored: %s.", e.Name, lowerFirst(e.Drop))
			case act.Kind == "store" && sh.d.Pack.Free() < 1:
				return Refuse("The pack is full (%d pieces).", sh.d.Pack.Capacity)
			}
		}
		if retrieve && !slices.Contains(st.Pack, act.Take) {
			return Refuse("%s is not in the pack.", act.Take)
		}
		u, _ := stateUnit(st, unitID)
		if store {
			u.Dropped = append(u.Dropped, act.Item)
			st.Pack = append(st.Pack, act.Item)
		}
		if retrieve {
			// the item leaves the pack first: it must not count twice against a limit. A refusal below drops the whole change.
			if p := slices.Index(st.Pack, act.Take); p >= 0 {
				st.Pack = slices.Delete(st.Pack, p, p+1)
			}
			after := newShop(rs, settings, *st)
			k, _ := after.unit(unitID)
			offer, ok := rs.BC.Offer(sh.bcC, act.Take)
			if !ok {
				e, _ := equipmentFor(rs, sh.bcC, act.Take, false, false, unit.IsHero())
				offer = GearOffer{Name: act.Take, Classes: e.Classes(), Size: e.Size}
			}
			offer.Cost = 0 // carrying gear is free
			if why, _ := after.whyNot(k, offer); why != "" {
				return Refuse("%s cannot take %s: %s.", unit.Title(), act.Take, lowerFirst(why))
			}
			u.Bought = append(u.Bought, act.Take)
		}
		return nil
	})
}
