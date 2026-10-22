package core

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func (f *companyFixture) armoury(t *testing.T) *ArmouryService {
	t.Helper()
	f.update()
	return NewArmouryService(f.campaigns, f.companies, staticRules{shippedRules(t)})
}

func itemsByName(items []ArmouryItem) map[string]ArmouryItem {
	m := map[string]ArmouryItem{}
	for _, i := range items {
		m[i.Name] = i
	}
	return m
}

func (f *companyFixture) unit(t *testing.T, id int64) CompanyUnit {
	t.Helper()
	d := BuildDetail(shippedRules(t), f.settings, f.state)
	for _, u := range d.Roster {
		if u.ID == id {
			return u
		}
	}
	t.Fatalf("unit %d is missing", id)
	return CompanyUnit{}
}

func TestBuyingEquipmentSpendsInfluenceAndTakesASlot(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	offered := func(unitID int64) map[string]ArmouryItem {
		items, err := s.Armoury(f.ctx, 5, 1, unitID)
		if err != nil {
			t.Fatal(err)
		}
		return itemsByName(items)
	}
	hador := offered(2)
	if !hador["Badge of Courage"].Buyable() || hador["Badge of Courage"].Icon != "badge" {
		t.Errorf("a free small slot and enough influence: %+v", hador["Badge of Courage"])
	}
	warrior := offered(6)
	for _, name := range []string{"Healing Herbs", "Barding", "Horse", "Axe of Lossarnach", "Hand-and-a-half Weapon"} {
		if _, ok := warrior[name]; ok {
			t.Errorf("%s must not be offered to Cirion: heroes only or not an option of his profile", name)
		}
	}
	for name, why := range map[string]string{"Barding": "barding is for cavalry", "Pike": "Hador already carries a spear (also sold as Pike)"} {
		if _, ok := hador[name]; ok {
			t.Error(why)
		}
	}
	if _, ok := hador["Horse"]; !ok {
		t.Error("a hero of Minas Tirith may buy a horse: its charts have knights")
	}

	if err := s.BuyEquipment(f.ctx, f.user, 5, 2, "Badge of Courage", ""); err != nil {
		t.Fatal(err)
	}
	if f.state.Influence != 10 {
		t.Errorf("influence 12 -> %d, the badge costs 2", f.state.Influence)
	}
	u := f.unit(t, 2)
	if got := u.Slots().SmallUsed; got != 1 {
		t.Errorf("the badge takes the small slot: %+v", u.Slots())
	}
	granted := false
	for _, r := range u.Rules {
		granted = granted || (r.Name == "Fearless" && r.Source == RuleFromEquipment)
	}
	if !granted {
		t.Error("the Badge of Courage grants Fearless")
	}
	if got := offered(2)["Map"]; got.Blocker != BlockedBySlots || got.Reason != "No free small slot" {
		t.Errorf("no free small slot is a reason worth showing: %+v", got)
	}
	if err := s.BuyEquipment(f.ctx, f.user, 5, 2, "Map", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("no free small slot: %v", err)
	}
	if err := s.BuyEquipment(f.ctx, f.user, 5, 2, "No Such Thing", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown item: %v", err)
	}
	for _, c := range []struct {
		unit int64
		item string
	}{{6, "Horse"}, {6, "Healing Herbs"}, {2, "Pike"}} { // in the armoury, but not for this unit: the refusal says why, not a 404
		if err := s.BuyEquipment(f.ctx, f.user, 5, c.unit, c.item, ""); !errors.Is(err, ErrRefused) {
			t.Errorf("%s for unit %d: want a refusal, got %v", c.item, c.unit, err)
		}
	}
	if err := s.BuyEquipment(f.ctx, f.user, 5, 999, "Map", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown unit: %v", err)
	}
	if got := f.state.Influence; got != 10 {
		t.Errorf("refusals spend nothing: %d", got)
	}
}

func TestPurchasesAreRefusedForStrangersAndWhenTooDear(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	if err := s.BuyEquipment(f.ctx, User{ID: 8}, 5, 2, "Badge of Courage", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("a member without a company: %v", err)
	}
	f.state.Influence = 1
	if got := itemsByName(mustItems(t, s, f, 3))["Armoured Horse"]; got.Name != "" && got.Blocker != BlockedByInfluence {
		t.Errorf("1 influence left: %+v", got)
	}
	if err := s.BuyEquipment(f.ctx, f.user, 5, 3, "Badge of Courage", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("too dear: %v", err)
	}
	if f.state.Influence != 1 || len(f.state.Units[2].Bought) != 1 {
		t.Errorf("a refused purchase changes nothing: influence %d, bought %v", f.state.Influence, f.state.Units[2].Bought)
	}
}

func mustItems(t *testing.T, s *ArmouryService, f *companyFixture, unit int64) []ArmouryItem {
	t.Helper()
	items, err := s.Armoury(f.ctx, 5, 1, unit)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestConcurrentPurchasesCannotOverspend(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	f.state.Influence = 5 // a badge costs 2: at most two of the four heroes below can have one
	var bought atomic.Int32
	var wg sync.WaitGroup
	for _, unit := range []int64{2, 3, 4, 5, 6, 8} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.BuyEquipment(f.ctx, f.user, 5, unit, "Badge of Courage", ""); err == nil {
				bought.Add(1)
			} else if !errors.Is(err, ErrRefused) && !errors.Is(err, ErrNotFound) {
				t.Errorf("unit %d: %v", unit, err)
			}
		}()
	}
	wg.Wait()
	if bought.Load() > 2 || f.state.Influence < 0 {
		t.Errorf("%d purchases with 5 influence at 2 each: influence left %d", bought.Load(), f.state.Influence)
	}
	if bought.Load() == 0 {
		t.Error("some purchase must succeed")
	}
	if spent := 5 - f.state.Influence; int32(spent) != 2*bought.Load() {
		t.Errorf("spent %d for %d purchases", spent, bought.Load())
	}
}

func TestEquipmentCanBeBoughtWithGold(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	f.state.Gold = 30
	var item ArmouryItem
	for _, it := range mustItems(t, s, f, 2) {
		if it.Buyable() && it.PayGold && it.Cost > 0 && it.Size != "" {
			item = it
			break
		}
	}
	if item.Name == "" {
		t.Fatal("some item can be bought with gold")
	}
	if err := s.BuyEquipment(f.ctx, f.user, 5, 2, item.Name, "gold"); err != nil {
		t.Fatal(err)
	}
	if f.state.Gold != 30-item.GoldPrice || f.state.Influence != 12 {
		t.Errorf("gold 30 -> %d (price %d), influence %d", f.state.Gold, item.GoldPrice, f.state.Influence)
	}
	f.state.Gold = 0
	if err := s.BuyEquipment(f.ctx, f.user, 5, 3, "Badge of Courage", "gold"); !errors.Is(err, ErrRefused) {
		t.Errorf("no gold left: %v", err)
	}
}

func TestDroppingFollowsTheBattleCompaniesRules(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	// A hero drops anything it bought, without conditions; its points follow.
	hero := f.unit(t, 1)
	if err := s.DropEquipment(f.ctx, f.user, 5, 1, "Throwing Spear"); err != nil {
		t.Errorf("a hero may drop what it bought: %v", err)
	}
	if after := f.unit(t, 1); after.Points != hero.Points-5 { // the hero rating of a throwing weapon
		t.Errorf("points: %d -> %d", hero.Points, after.Points)
	}
	// ...but not its profile's wargear unless the optional rule is on.
	var profile string
	for _, e := range hero.Equipment {
		if e.Profile {
			profile = e.Name
		}
	}
	if profile == "" {
		t.Fatal("the leader carries profile gear")
	}
	if err := s.DropEquipment(f.ctx, f.user, 5, 1, profile); !errors.Is(err, ErrRefused) {
		t.Errorf("heroes keep their profile's wargear unless the optional rule is on: %v", err)
	}
	if err := s.DropEquipment(f.ctx, f.user, 5, 1, "Badge of Courage"); err != nil {
		t.Errorf("equipment is always free to drop: %v", err)
	}
	if err := s.DropEquipment(f.ctx, f.user, 5, 1, "Banner of Doom"); !errors.Is(err, ErrRefused) {
		t.Errorf("not carried: %v", err)
	}
	if err := s.DropEquipment(f.ctx, f.user, 5, 999, "Spear"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown unit: %v", err)
	}
	if err := s.DropEquipment(f.ctx, User{ID: 8}, 5, 1, "Spear"); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
}

func TestTheOptionalRuleLetsHeroesDropProfileGear(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	profile := ""
	for _, e := range f.unit(t, 1).Equipment {
		if e.Profile {
			profile = e.Name
		}
	}
	f.settings = CampaignSettings{HeroesDropProfileGear: true} // the same company in a campaign that plays with the optional rule
	if err := s.DropEquipment(f.ctx, f.user, 5, 1, profile); err != nil {
		t.Errorf("with the optional rule a hero may drop %s: %v", profile, err)
	}
	if len(f.state.Units[0].Dropped) != 1 {
		t.Errorf("dropped: %v", f.state.Units[0].Dropped)
	}
}

func TestAWarriorMayOnlyDropGearWhenWhatIsLeftIsAnOption(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	// A recruit with a spear and a shield (the "Shield and Spear" option).
	f.state.Units = append(f.state.Units, UnitState{ID: 100, Base: "Warrior of Minas Tirith", Role: RoleWarrior, Gear: []string{"Spear", "Shield"}})
	before := f.unit(t, 100)
	if before.Points != 8+1+1 {
		t.Errorf("points of the recruit: %d", before.Points)
	}
	if err := s.DropEquipment(f.ctx, f.user, 5, 100, "Shield"); !errors.Is(err, ErrRefused) {
		t.Errorf("a spear alone is not an option of the profile: %v", err)
	}
	if err := s.DropEquipment(f.ctx, f.user, 5, 100, "Spear"); err != nil {
		t.Fatalf("a shield alone is an option: %v", err)
	}
	if after := f.unit(t, 100); after.Points != before.Points-1 {
		t.Errorf("dropping the spear marks down its rating: %d -> %d", before.Points, after.Points)
	}
}

func TestTheArmouryListsEveryItemWithWhoCanTakeIt(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	entries, err := s.ArmouryEntries(f.ctx, 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]ArmouryEntry{}
	for _, e := range entries {
		byName[e.Name] = e
		if len(e.Units) != len(f.state.Units) {
			t.Errorf("%s: every unit is listed, able or not: %d of %d", e.Name, len(e.Units), len(f.state.Units))
		}
	}
	barding, ok := byName["Barding"]
	if !ok {
		t.Fatal("an item no infantry can take is still listed")
	}
	reasons := map[string]int{}
	for _, u := range barding.Units {
		if !u.Able() {
			reasons[u.Reason]++
		}
	}
	if reasons["Cavalry only"] < 8 {
		t.Errorf("most units cannot take barding because it is for cavalry: %v", reasons)
	}
	for _, u := range byName["Seeing Stone"].Units {
		if u.ID == 6 && u.Reason != "Heroes only" {
			t.Errorf("a warrior cannot take a hero's item: %q", u.Reason)
		}
	}
	if !byName["Badge of Courage"].CanBuy() {
		t.Error("some unit can take a badge")
	}
}

func TestCreaturesGoToOfficersAndTakeNoSlots(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	f.state.Influence = 20
	before := BuildDetail(shippedRules(t), CampaignSettings{}, f.state)
	if err := s.BuyCreature(f.ctx, f.user, 5, 6, "Baggage Pony", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("a warrior cannot have a creature: %v", err)
	}
	if err := s.BuyCreature(f.ctx, f.user, 5, 1, "Baggage Pony", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.BuyCreature(f.ctx, f.user, 5, 1, "Falcon", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("one creature each: %v", err)
	}
	if err := s.BuyCreature(f.ctx, f.user, 5, 1, "Unicorn", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown creature: %v", err)
	}
	after := BuildDetail(shippedRules(t), CampaignSettings{}, f.state)
	if len(after.Creatures) != 1 || after.Creatures[0].OwnerID != 1 || after.Pack == nil || after.Pack.Capacity != 15 {
		t.Fatalf("the pony is the leader's and gives the pack: %+v", after.Creatures)
	}
	if after.Units != before.Units || len(after.Limits) != len(before.Limits) || after.Influence != before.Influence-2 || after.Points != before.Points+5 {
		t.Errorf("a creature is no unit: units %d->%d, influence %d->%d, points %d->%d", before.Units, after.Units, before.Influence, after.Influence, before.Points, after.Points)
	}
	if after.Roster[0].Creature == nil {
		t.Error("the owner's card shows it")
	}
	list, err := s.ArmouryCreatures(f.ctx, 5, 1)
	if err != nil || len(list) == 0 {
		t.Fatalf("creatures: %v %v", len(list), err)
	}
}

func TestTheBaggagePonyStoresAndSwapsGear(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	f.state.Influence = 20
	if err := s.Carry(f.ctx, f.user, 5, 2, CarryAction{Kind: "store", Item: "Spear"}); !errors.Is(err, ErrRefused) {
		t.Errorf("without a pony there is no pack: %v", err)
	}
	if err := s.BuyCreature(f.ctx, f.user, 5, 1, "Baggage Pony", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Carry(f.ctx, f.user, 5, 2, CarryAction{Kind: "store", Item: "Spear"}); err != nil {
		t.Fatal(err)
	}
	if len(f.state.Pack) != 1 || f.state.Pack[0] != "Spear" {
		t.Fatalf("the spear is in the pack: %v", f.state.Pack)
	}
	for _, e := range f.unit(t, 2).Equipment {
		if e.Name == "Spear" {
			t.Error("Hador no longer carries it")
		}
	}
	if err := s.Carry(f.ctx, f.user, 5, 6, CarryAction{Kind: "retrieve", Take: "Spear"}); err != nil {
		if !errors.Is(err, ErrRefused) {
			t.Fatal(err)
		}
		if len(f.state.Pack) != 1 {
			t.Errorf("a refused retrieve leaves the pack as it was: %v", f.state.Pack)
		}
		if err := s.Carry(f.ctx, f.user, 5, 2, CarryAction{Kind: "retrieve", Take: "Spear"}); err != nil {
			t.Errorf("Hador takes his spear back: %v", err)
		}
	}
	if len(f.state.Pack) != 0 {
		t.Errorf("the pack is empty again: %v", f.state.Pack)
	}
	if err := s.Carry(f.ctx, f.user, 5, 2, CarryAction{Kind: "retrieve", Take: "Spear"}); !errors.Is(err, ErrRefused) {
		t.Errorf("not in the pack: %v", err)
	}
	if err := s.Carry(f.ctx, f.user, 5, 2, CarryAction{Kind: "burn"}); !errors.Is(err, ErrRefused) {
		t.Errorf("unknown action: %v", err)
	}
	if err := s.Carry(f.ctx, f.user, 5, 5, CarryAction{Kind: "store", Item: "Horse"}); !errors.Is(err, ErrRefused) {
		t.Errorf("mounts are not carried: %v", err)
	}
}

func TestWanderersJoinAsUnitsAndCanBeDismissed(t *testing.T) {
	f := newCompanyFixture(t)
	s := f.armoury(t)
	f.state.Influence = 5
	if err := s.HireWanderer(f.ctx, f.user, 5, "Wandering Swordsman", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("9 influence is more than the company has: %v", err)
	}
	f.state.Influence = 30
	before := BuildDetail(shippedRules(t), CampaignSettings{}, f.state)
	if err := s.HireWanderer(f.ctx, f.user, 5, "Wandering Swordsman", ""); err != nil {
		t.Fatal(err)
	}
	after := BuildDetail(shippedRules(t), CampaignSettings{}, f.state)
	var w CompanyUnit
	for _, u := range after.Roster {
		if u.Wanderer {
			w = u
		}
	}
	if w.PathPending {
		t.Error("a wanderer has no path to choose, so it never blocks a match")
	}
	if after.Units != before.Units+1 || w.Points != 60 || !w.IsHero() || w.ID == 0 || after.Influence != before.Influence-9 || after.Points != before.Points+60 {
		t.Errorf("a wanderer is a unit worth 60 points: %+v", w)
	}
	if err := s.HireWanderer(f.ctx, f.user, 5, "Wandering Swordsman", ""); !errors.Is(err, ErrRefused) {
		t.Errorf("one wanderer at a time: %v", err)
	}
	if err := s.DismissWanderer(f.ctx, f.user, 5, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("only wanderers can be dismissed: %v", err)
	}
	if err := s.DismissWanderer(f.ctx, User{ID: 8}, 5, w.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-owner: %v", err)
	}
	if err := s.DismissWanderer(f.ctx, f.user, 5, w.ID); err != nil {
		t.Fatal(err)
	}
	if gone := BuildDetail(shippedRules(t), CampaignSettings{}, f.state); gone.Units != before.Units || gone.Influence != after.Influence {
		t.Errorf("dismissed, with no refund: %d units, %d influence", gone.Units, gone.Influence)
	}
	list, err := s.ArmouryWanderers(f.ctx, 5, 1)
	if err != nil || len(list) == 0 {
		t.Fatalf("wanderers: %v %v", len(list), err)
	}
}
