package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

func atoCfg() LiveFoldConfig {
	return LiveFoldConfig{
		Break:       map[int64]BreakConfig{},
		NoStandFast: map[int64]bool{},
		Loaned: []LoanedItem{
			{UnitID: 1, Item: "Dwarven Brew", Kit: "Raider's Kit", Index: 0},
			{UnitID: 1, Item: "Torching Brand", Kit: "Raider's Kit", Index: 0},
			{UnitID: 1, Item: "Torching Brand", Kit: "Raider's Kit", Index: 1},
			{UnitID: 2, Item: "Healing Herbs", Kit: "Healer's Kit", Index: 0, AfterGame: true},
			{UnitID: 2, Item: "Envenom Weapon (X)", Kit: "Hunter's Kit", Detail: "Spear"},
		},
		Owned:     map[int64][]string{1: {"Throwing Spear", "Lucky Talisman"}, 2: {"Bow"}},
		AfterGame: map[string]bool{"healing herbs": true, "wondrous cram": true},
	}
}

func TestLoanedItemsAndTheirUse(t *testing.T) {
	b := newBP(t, 4, 50)
	b.cfg = atoCfg()
	b.cfg.Break = map[int64]BreakConfig{1: {Percent: 50}, 2: {Percent: 50}}
	if u, _ := b.state().Unit(1); len(u.Loaned) != 3 || u.Loaned[0].Used || u.Loaned[0].Kit != "Raider's Kit" {
		t.Fatalf("loaned: %+v", u.Loaned)
	}
	text := b.do(1, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "dwarven brew"})
	if text != "unit 1 used Dwarven Brew" {
		t.Errorf("text: %q", text)
	}
	u, _ := b.state().Unit(1)
	if !u.Loaned[0].Used || u.Loaned[1].Used {
		t.Errorf("only that item is struck through: %+v", u.Loaned)
	}
	if _, err := b.try(1, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "Dwarven Brew"}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "already used") {
		t.Errorf("twice: %v", err)
	}
	// two copies are used one at a time, by index
	b.do(1, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "Torching Brand", Index: 1})
	if u, _ = b.state().Unit(1); u.Loaned[1].Used || !u.Loaned[2].Used {
		t.Errorf("copy 1 only: %+v", u.Loaned)
	}
	b.do(1, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "Torching Brand", Index: 0})
	// undo: the fold without the entry has it unused again
	if u, _ = FoldLiveWith(b.inits, b.entries[:len(b.entries)-1], b.cfg).Unit(1); u.Loaned[1].Used {
		t.Errorf("after the undo: %+v", u.Loaned)
	}
	// items that act in the aftermath are not used during the game
	if _, err := b.try(1, LiveAction{Kind: LiveItemUsed, UnitID: 2, Item: "Healing Herbs"}); !errors.Is(err, ErrRefused) {
		t.Errorf("a unit without it: %v", err)
	}
	if _, err := b.try(1, LiveAction{Kind: LiveItemUsed, UnitID: 2, Item: "Healing Herbs"}); err == nil {
		t.Error("Healing Herbs act in the aftermath")
	}
	b2 := newBP(t, 4, 50)
	b2.cfg = atoCfg()
	if _, err := b2.try(1, LiveAction{Kind: LiveItemUsed, UnitID: 2, Item: "Healing Herbs"}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "acts after the game") {
		t.Errorf("kit herbs: %v", err)
	}
	// an owned item is used once; one that is not carried is refused; an item of the profile that acts in the aftermath too
	b2.do(1, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "lucky talisman"})
	if u, _ := b2.state().Unit(1); len(u.UsedItems) != 1 || u.UsedItems[0] != "Lucky Talisman" {
		t.Errorf("used owned: %+v", u.UsedItems)
	}
	if _, err := b2.try(1, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "Lucky Talisman"}); !errors.Is(err, ErrRefused) {
		t.Errorf("owned twice: %v", err)
	}
	if _, err := b2.try(1, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "Map"}); !errors.Is(err, ErrRefused) {
		t.Errorf("not carried: %v", err)
	}
	if _, err := b2.try(1, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "  "}); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("no item: %v", err)
	}
	// a unit that is out of the battle uses nothing
	b2.do(1, LiveAction{Kind: LiveDied, UnitID: 2})
	if _, err := b2.try(1, LiveAction{Kind: LiveItemUsed, UnitID: 2, Item: "Bow"}); !errors.Is(err, ErrRefused) {
		t.Errorf("dead: %v", err)
	}
	// another company's unit
	if _, err := b2.try(2, LiveAction{Kind: LiveItemUsed, UnitID: 1, Item: "Dwarven Brew"}); !errors.Is(err, ErrRefused) {
		t.Errorf("another company: %v", err)
	}
}

func TestTemporaryWandererIsAUnitOfTheFold(t *testing.T) {
	id := TemporaryUnitID(1)
	if id >= 0 {
		t.Fatal("never a roster id")
	}
	b := newBP(t, 4, 50)
	b.inits = append(b.inits, LiveUnitInit{UnitID: id, CompanyID: 1, Wounds: 2, Might: 1, Will: 1, Fate: 1, CountsAsKill: true, Temporary: true})
	b.cfg.NoStandFast[id] = true
	u, ok := b.state().Unit(id)
	if !ok || !u.Temporary || u.Wounds.Start != 2 || u.BreakModels != 1 {
		t.Fatalf("wanderer: %+v %v", u, ok)
	}
	if got := b.state().Break(1); got.StartingModels != 5 {
		t.Errorf("it is a model for the Break Point: %+v", got)
	}
	// it fights like any hero
	b.do(1, LiveAction{Kind: LiveWound, UnitID: id, Targets: []WoundTarget{{UnitID: 101, Caused: 1}}})
	if x, _ := b.state().Unit(101); x.Zone != ZoneCasualty {
		t.Errorf("its wound lands: %+v", x)
	}
	if x, _ := b.state().Unit(id); x.Gains[GainWoundedEnemy] != 1 || x.Gains[GainSlainHeroMons] != 0 {
		t.Errorf("gains: %+v", x.Gains)
	}
	// and falls like any hero: one more casualty for its company
	b.do(2, LiveAction{Kind: LiveWound, UnitID: 102, Targets: []WoundTarget{{UnitID: id, Caused: 2}}})
	if got := b.state().Break(1); got.Casualties != 1 {
		t.Errorf("a casualty: %+v", got)
	}
	// no Stand Fast, and no MVP vote
	if _, err := b.try(1, LiveAction{Kind: LiveBroken, UnitID: id, StandFast: true}); err == nil {
		// it is in the casualty zone, so first bring it back
		t.Log("refused as a casualty")
	}
	b.do(1, LiveAction{Kind: LiveReturn, UnitID: id})
	if _, err := b.try(1, LiveAction{Kind: LiveBroken, UnitID: id, StandFast: true}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "Stand Fast") {
		t.Errorf("no Stand Fast: %v", err)
	}
	vote := LiveAction{Kind: LiveMVPVote, TargetCompanyID: 1, UnitID: id}
	if _, _, _, err := b.state().CheckAs(vote, 1, LiveVoter{ID: 5, Name: "Ann"}); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "temporary Wanderer") {
		t.Errorf("no MVP: %v", err)
	}
}

// --- through the service ---

func newATOLiveFixture(t *testing.T, rec *ATORecord) (*liveFixture, *MockatoRepository) {
	t.Helper()
	f := newLiveFixture(t)
	repo := NewMockatoRepository(gomock.NewController(t))
	repo.EXPECT().Get(gomock.Any(), int64(5), int64(1)).DoAndReturn(func(_ context.Context, _, _ int64) (ATORecord, error) { return *rec, nil }).AnyTimes()
	f.svc.WithATO(repo)
	return f, repo
}

func TestLiveOverlaysTheAgainstTheOddsRecord(t *testing.T) {
	rec := &ATORecord{}
	f, _ := newATOLiveFixture(t, rec)
	before, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil || before.Wanderer != nil {
		t.Fatalf("without a record: %+v %v", before.Wanderer, err)
	}
	hero := before.Units[0].UnitID
	wand := realATO(t).Wanderers[0]
	// the player saves the selection after the live state was first read: it shows at once
	*rec = ATORecord{MatchID: 1, CompanyID: 1, Selection: ATOSelection{Bonuses: []string{"toolKit", "wanderer"}, Kits: []string{"raider", "healer"}, Wanderer: wand.Name,
		Items: []KitAssignment{{Kit: "raider", Item: "Dwarven Brew", UnitID: hero}, {Kit: "healer", Item: "Healing Herbs", UnitID: hero}, {Kit: "raider", Item: "Torching Brand", Index: 2}}}}
	lc, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if lc.Wanderer == nil || lc.Wanderer.UnitID != -1 || lc.Wanderer.Name != wand.Name || lc.Wanderer.Rating != 45 || lc.Wanderer.Courage != wand.Courage {
		t.Fatalf("wanderer: %+v", lc.Wanderer)
	}
	var temp, carrier LiveUnit
	for _, u := range lc.Units {
		if u.Temporary {
			temp = u
		}
		if u.UnitID == hero {
			carrier = u
		}
	}
	if temp.UnitID != -1 || temp.Wounds.Start != wand.Wounds || temp.Might.Start != wand.Might || temp.Zone != ZoneField {
		t.Errorf("the temporary unit: %+v", temp)
	}
	if len(carrier.Loaned) != 2 || carrier.Loaned[0].Item != "Dwarven Brew" || carrier.Loaned[0].Kit != "Raider's Kit" || carrier.Loaned[0].AfterGame ||
		carrier.Loaned[1].Item != "Healing Herbs" || !carrier.Loaned[1].AfterGame {
		t.Errorf("loaned: %+v", carrier.Loaned)
	}
	if lc.Break.StartingModels != before.Break.StartingModels+1 {
		t.Errorf("the Wanderer is a model for the Break Point: %d vs %d", lc.Break.StartingModels, before.Break.StartingModels)
	}
	// the opposing company sees the Wanderer too (table information), but it belongs to the first
	if o, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2); o.Wanderer != nil {
		t.Error("the other company has no Wanderer")
	}
	// item used: logged, struck through, undone
	_, e, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveItemUsed, UnitID: hero, Item: "Dwarven Brew"})
	if err != nil || !strings.HasSuffix(e.Text, "used Dwarven Brew") {
		t.Fatalf("item used: %+v %v", e, err)
	}
	lc, _ = f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if !unitOf(lc, hero).Loaned[0].Used {
		t.Error("struck through")
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveItemUsed, UnitID: hero, Item: "Healing Herbs"}); !errors.Is(err, ErrRefused) {
		t.Errorf("Herbs act in the aftermath: %v", err)
	}
	if lc, err = f.svc.Undo(f.ctx, f.user(liveOwner1), 5, 1, e.ID, 1); err != nil || unitOf(lc, hero).Loaned[0].Used {
		t.Errorf("undo: %v", err)
	}
	// the Wanderer fights: it can wound an enemy, cannot call Stand Fast, and rolls its Courage
	enemy, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveWound, UnitID: -1, Targets: []WoundTarget{{UnitID: enemy.Units[0].UnitID, Caused: 1}}}); err != nil {
		t.Fatalf("the Wanderer wounds: %v", err)
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveBroken, UnitID: -1, StandFast: true}); !errors.Is(err, ErrRefused) {
		t.Errorf("no Stand Fast: %v", err)
	}
	f.svc.dice = &FixedDice{Rolls: []int{wand.Courage - 1, 1}}
	if r, err := f.svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, -1, nil, false); err != nil || r.Courage != wand.Courage || !r.Passed {
		t.Errorf("courage: %+v %v", r, err)
	}
	// it is not on the aftermath sheet
	sheets, ok, err := f.svc.AftermathDefaults(f.ctx, 5, 1, 1)
	if err != nil || !ok {
		t.Fatalf("defaults: %v %v", ok, err)
	}
	for _, sh := range sheets {
		if sh.UnitID == -1 {
			t.Error("the temporary Wanderer is not on the sheet: no roll, no experience")
		}
	}
	// the record goes away (selection cleared): the Wanderer and the items are gone
	*rec = ATORecord{}
	if lc, _ = f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1); lc.Wanderer != nil || len(unitOf(lc, hero).Loaned) != 0 {
		t.Errorf("no overlay without a record: %+v", lc.Wanderer)
	}
}

func unitOf(lc LiveCompany, id int64) LiveUnit {
	for _, u := range lc.Units {
		if u.UnitID == id {
			return u
		}
	}
	return LiveUnit{}
}
