package core

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

// fakeLive is an in-memory liveRepository with the semantics of the Postgres one (seq per match, an undo takes a seq, entries stay).
type fakeLive struct {
	mu      sync.Mutex
	matches map[int64]*fakeLiveMatch
	feed    *fakeFeed // told after each write, like the NOTIFY
	ended   map[int64]bool
	updates int
}

type fakeLiveMatch struct {
	inits   []LiveUnitInit
	entries []LiveEntry
	undone  map[int64]bool
	seq     int64
	nextID  int64
}

func newFakeLive(feed *fakeFeed) *fakeLive {
	return &fakeLive{matches: map[int64]*fakeLiveMatch{}, ended: map[int64]bool{}, feed: feed}
}

func (f *fakeLive) Init(_ context.Context, _, matchID int64, units []LiveUnitInit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.matches[matchID]; !ok {
		f.matches[matchID] = &fakeLiveMatch{inits: slices.Clone(units), undone: map[int64]bool{}}
	}
	return nil
}

func (f *fakeLive) snapshot(matchID int64) LiveSnapshot {
	m, ok := f.matches[matchID]
	snap := LiveSnapshot{MatchID: matchID, Number: 1, Ended: f.ended[matchID], Initialised: ok}
	if !ok {
		return snap
	}
	snap.Seq, snap.Units = m.seq, slices.Clone(m.inits)
	for _, e := range m.entries {
		if !m.undone[e.ID] {
			snap.Entries = append(snap.Entries, e)
		}
	}
	return snap
}

func (f *fakeLive) Read(_ context.Context, _, matchID int64) (LiveSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot(matchID), nil
}

func (f *fakeLive) Update(_ context.Context, _, matchID int64, by User, fn func(LiveSnapshot) (LiveChange, error)) (LiveEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	m, ok := f.matches[matchID]
	if !ok {
		return LiveEntry{}, errors.New("match has no live data")
	}
	ch, err := fn(f.snapshot(matchID))
	if err != nil {
		return LiveEntry{}, err
	}
	var stored LiveEntry
	var touched []int64
	if ch.Append != nil {
		m.seq++
		m.nextID++
		e := *ch.Append
		e.ID, e.Seq, e.MatchID, e.ByUserID, e.By, e.On = m.nextID, m.seq, matchID, by.ID, by.DisplayName, time.Now()
		m.entries = append(m.entries, e)
		stored, touched = e, append([]int64{e.CompanyID}, e.Touches...)
	} else {
		for _, e := range m.entries {
			if e.ID == ch.Undo && !m.undone[e.ID] {
				m.undone[e.ID] = true
				m.seq++
				stored, touched = e, append([]int64{e.CompanyID}, e.Touches...)
			}
		}
		if stored.ID == 0 {
			return LiveEntry{}, ErrNotFound
		}
	}
	if f.feed != nil {
		for _, c := range touched {
			f.feed.emit(LiveEvent{MatchID: matchID, CompanyID: c, Seq: m.seq, Kind: LiveChanged})
		}
	}
	return stored, nil
}

func (f *fakeLive) Log(_ context.Context, _, matchID, before int64, limit int) ([]LiveEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []LiveEntry
	for _, e := range f.snapshot(matchID).Entries {
		if before == 0 || e.Seq < before {
			out = append(out, e)
		}
	}
	slices.Reverse(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// fakeFeed is a liveFeed driven by the test.
type fakeFeed struct {
	mu       sync.Mutex
	onEvent  func(LiveEvent)
	onResync func()
	started  chan struct{}
	once     sync.Once
}

func newFakeFeed() *fakeFeed { return &fakeFeed{started: make(chan struct{})} }

func (f *fakeFeed) Run(ctx context.Context, onEvent func(LiveEvent), onResync func()) error {
	f.mu.Lock()
	f.onEvent, f.onResync = onEvent, onResync
	f.mu.Unlock()
	f.once.Do(func() { close(f.started) })
	onResync()
	<-ctx.Done()
	return nil
}

func (f *fakeFeed) emit(ev LiveEvent) {
	f.mu.Lock()
	fn := f.onEvent
	f.mu.Unlock()
	if fn != nil {
		fn(ev)
	}
}

func (f *fakeFeed) resync() {
	f.mu.Lock()
	fn := f.onResync
	f.mu.Unlock()
	if fn != nil {
		fn()
	}
}

type liveFixture struct {
	*matchesFixture
	repo *fakeLive
	feed *fakeFeed
	svc  *LiveService
}

const (
	liveOwner1 = 10 // owns company 1 (Rangers)
	liveOwner2 = 20 // owns company 2 (Raiders)
	liveOwner3 = 30 // owns company 3 (Wolves)
)

func newLiveFixture(t *testing.T) *liveFixture {
	t.Helper()
	f := &liveFixture{matchesFixture: newMatchesFixture(t)}
	f.feed = newFakeFeed()
	f.repo = newFakeLive(f.feed)
	// every company is the Gondor demo company with unit ids offset by company*100
	f.companies = NewMockcompanyRepository(gomock.NewController(t))
	f.companies.EXPECT().Read(gomock.Any(), int64(5), gomock.Any()).DoAndReturn(func(_ context.Context, _, id int64) (CompanyState, error) {
		st := gondor()
		st.Company.ID, st.Company.Name = id, map[int64]string{1: "Rangers", 2: "Raiders", 3: "Wolves"}[id]
		for i := range st.Units {
			st.Units[i].ID += id * 100
		}
		st.OpenMatch = &OpenMatchRef{MatchID: 1, Number: 1}
		return st, nil
	}).AnyTimes()
	f.svc = NewLiveService(f.repo, f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)})
	return f
}

func (f *liveFixture) user(id int64) User { return User{ID: id, Role: RoleUser} }

func TestLiveInitAndLazyInit(t *testing.T) {
	f := newLiveFixture(t)
	if snap, _ := f.repo.Read(f.ctx, 5, 1); snap.Initialised {
		t.Fatal("not initialised yet")
	}
	lc, err := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(lc.Units) == 0 || lc.Access != LiveEdit || lc.MatchID != 1 {
		t.Errorf("live company: %+v", lc)
	}
	snap, _ := f.repo.Read(f.ctx, 5, 1)
	if !snap.Initialised || len(snap.Units) == 0 {
		t.Errorf("lazy init: %+v", snap)
	}
	var absent, hero, mounted bool
	for _, in := range snap.Units {
		absent = absent || in.Absent
		hero = hero || in.CountsAsKill
		mounted = mounted || in.HasMount
		if !in.Absent && in.Wounds < 1 {
			t.Errorf("a unit without Wounds: %+v", in)
		}
	}
	if !absent || !hero || !mounted {
		t.Errorf("injured units are absent, heroes count as kills, knights have a mount: %v %v %v", absent, hero, mounted)
	}
	// Init is idempotent
	if err := f.svc.Init(f.ctx, 5, 1); err != nil {
		t.Fatal(err)
	}
	// a company in no open match
	f.companies = NewMockcompanyRepository(gomock.NewController(t))
	f.companies.EXPECT().Read(gomock.Any(), int64(5), gomock.Any()).Return(CompanyState{Company: CampaignCompany{ID: 1}}, nil).AnyTimes()
	svc := NewLiveService(f.repo, f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)})
	if _, err := svc.Live(f.ctx, f.user(liveOwner1), 5, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("no open match: %v", err)
	}
}

func firstUnit(lc LiveCompany) int64 { return lc.Units[0].UnitID }

func TestLiveApplyPermissions(t *testing.T) {
	f := newLiveFixture(t)
	lc, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	act := LiveAction{Kind: LiveBroken, UnitID: firstUnit(lc)}

	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner2), 5, 1, 1, act); !errors.Is(err, ErrRefused) {
		t.Errorf("another company's owner: %v", err)
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(7), 5, 1, 1, act); !errors.Is(err, ErrRefused) {
		t.Errorf("a stranger: %v", err)
	}
	if snap, _ := f.repo.Read(f.ctx, 5, 1); len(snap.Entries) != 0 {
		t.Fatal("refused actions store nothing")
	}
	for _, u := range []User{f.user(liveOwner1), f.user(matchOrganiser), {ID: 5, Role: RoleAdmin}} {
		if _, e, err := f.svc.Apply(f.ctx, u, 5, 1, 1, LiveAction{Kind: LiveBroken, UnitID: firstUnit(lc)}); err != nil || e.Text == "" {
			t.Errorf("user %d: %v %+v", u.ID, err, e)
		}
	}
	// a unit of another company is refused
	other, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveBroken, UnitID: firstUnit(other)}); !errors.Is(err, ErrRefused) {
		t.Errorf("not your unit: %v", err)
	}
	// ended match and finished campaign
	f.repo.ended[1] = true
	f.match.Ended = true
	for _, u := range []User{f.user(liveOwner1), {ID: 5, Role: RoleAdmin}} {
		if _, _, err := f.svc.Apply(f.ctx, u, 5, 1, 1, act); !errors.Is(err, ErrRefused) {
			t.Errorf("ended match, user %d: %v", u.ID, err)
		}
	}
}

func TestLiveFinishedCampaignRefuses(t *testing.T) {
	f := newLiveFixture(t)
	f.campaigns = NewMockcampaignRepository(gomock.NewController(t))
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(5)).Return(RulesetHashes{BattleCompanies: "bc"}, CampaignSettings{}, nil).AnyTimes()
	f.campaigns.EXPECT().Get(gomock.Any(), gomock.Any(), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Status: CampaignFinished}, OrganiserID: matchOrganiser}, nil).AnyTimes()
	svc := NewLiveService(f.repo, f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)})
	lc, err := svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil || lc.Access != LiveNone {
		t.Fatalf("read only: %+v %v", lc, err)
	}
	for name, act := range map[string]LiveAction{
		"tracker": {Kind: LiveBroken, UnitID: firstUnit(lc)}, "objective": {Kind: LiveObjective, UnitID: firstUnit(lc), On: true},
	} {
		if _, _, err := svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, act); !errors.Is(err, ErrRefused) {
			t.Errorf("%s in a finished campaign: %v", name, err)
		}
	}
	if _, _, err := svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 2, LiveAction{Kind: LiveMVPVote, UnitID: 201}); !errors.Is(err, ErrRefused) {
		t.Errorf("a vote in a finished campaign: %v", err)
	}
	if _, err := svc.Undo(f.ctx, f.user(liveOwner1), 5, 1, 1, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("undo in a finished campaign: %v", err)
	}
}

func TestLiveCrossCompanyWoundAndUndo(t *testing.T) {
	f := newLiveFixture(t)
	mine, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	theirs, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)
	attacker, target := firstUnit(mine), firstUnit(theirs)
	targetWounds := theirs.Units[0].Wounds.Current

	lc, entry, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveWound, UnitID: attacker, Targets: []WoundTarget{{UnitID: target, Caused: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(entry.Touches, []int64{2}) || lc.Seq != 1 {
		t.Errorf("entry %+v, seq %d", entry, lc.Seq)
	}
	if got, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2); got.Units[0].Wounds.Current != targetWounds-1 {
		t.Errorf("the target lost a wound: %+v", got.Units[0])
	}
	if got, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1); got.Units[0].Gains[GainWoundedEnemy] != 1 {
		t.Errorf("the attacker earned the flag: %+v", got.Units[0])
	}
	if _, err := f.svc.Undo(f.ctx, f.user(liveOwner3), 5, 1, entry.ID, 3); !errors.Is(err, ErrRefused) {
		t.Errorf("a third player cannot undo: %v", err)
	}
	back, err := f.svc.Undo(f.ctx, f.user(liveOwner2), 5, 1, entry.ID, 2)
	if err != nil {
		t.Fatalf("the target's owner may undo: %v", err)
	}
	if back.Units[0].Wounds.Current != targetWounds {
		t.Errorf("undo restores the wounds: %+v", back.Units[0])
	}
	if _, err := f.svc.Undo(f.ctx, f.user(liveOwner2), 5, 1, entry.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("already undone: %v", err)
	}
	if log, _ := f.svc.Log(f.ctx, f.user(liveOwner1), 5, 1, 0, 50); len(log) != 0 {
		t.Errorf("undone entries disappear from the log: %+v", log)
	}
}

func TestLiveUndoThatBreaksALaterEntryIsRefused(t *testing.T) {
	f := newLiveFixture(t)
	lc, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	var hero LiveUnit
	for _, u := range lc.Units {
		if u.Fate.Start >= 1 {
			hero = u
			break
		}
	}
	if hero.UnitID == 0 {
		t.Skip("no unit with Fate")
	}
	_, spend, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveFate, UnitID: hero.UnitID, Count: hero.Fate.Start})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveStatChange, UnitID: hero.UnitID, Stat: "fate", Delta: 1}); err != nil {
		t.Fatal(err)
	}
	before := f.repo.updates
	if _, err := f.svc.Undo(f.ctx, f.user(liveOwner1), 5, 1, spend.ID, 1); !errors.Is(err, ErrLiveDepends) {
		t.Fatalf("got %v", err)
	}
	snap, _ := f.repo.Read(f.ctx, 5, 1)
	if len(snap.Entries) != 2 || f.repo.updates != before+1 {
		t.Errorf("nothing stored: %+v", snap.Entries)
	}
}

func TestLiveAftermathDefaults(t *testing.T) {
	f := newLiveFixture(t)
	if sheets, ok, err := f.svc.AftermathDefaults(f.ctx, 5, 1, 1); err != nil || ok || sheets != nil {
		t.Fatalf("no live data: %v %v %v", sheets, ok, err)
	}
	mine, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	theirs, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)
	a, b := mine.Units[0].UnitID, mine.Units[1].UnitID
	do := func(user int64, company int64, act LiveAction) {
		t.Helper()
		if _, _, err := f.svc.Apply(f.ctx, f.user(user), 5, 1, company, act); err != nil {
			t.Fatal(err)
		}
	}
	do(liveOwner1, 1, LiveAction{Kind: LiveWound, UnitID: a, Targets: []WoundTarget{{UnitID: theirs.Units[0].UnitID, Caused: theirs.Units[0].Wounds.Start}}})
	do(liveOwner1, 1, LiveAction{Kind: LiveBroken, UnitID: a})
	do(liveOwner1, 1, LiveAction{Kind: LiveRetreat, UnitID: b, Why: "courage"})
	sheets, ok, err := f.svc.AftermathDefaults(f.ctx, 5, 1, 1)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	byID := map[int64]ModelSheet{}
	for _, s := range sheets {
		byID[s.UnitID] = s
	}
	if byID[a].Outcome != OutcomeSurvived || byID[a].Gains[GainWoundedEnemy] != 1 || byID[a].Gains[GainBrokenPassed] != 1 || byID[a].Gains[participatedKey] != 1 {
		t.Errorf("attacker: %+v", byID[a])
	}
	if byID[b].Outcome != OutcomeFled {
		t.Errorf("retreated: %+v", byID[b])
	}
	if theirSheets, _, _ := f.svc.AftermathDefaults(f.ctx, 5, 1, 2); theirSheets[0].Outcome != OutcomeCasualty {
		t.Errorf("the fallen unit is a casualty: %+v", theirSheets[0])
	}
	for _, s := range sheets {
		for _, in := range f.snapshotInits(t) {
			if in.UnitID == s.UnitID && in.Absent {
				t.Errorf("absent units are skipped: %+v", s)
			}
		}
	}
}

func (f *liveFixture) snapshotInits(t *testing.T) []LiveUnitInit {
	snap, _ := f.repo.Read(f.ctx, 5, 1)
	return snap.Units
}

func TestLiveLogCanUndoFlag(t *testing.T) {
	f := newLiveFixture(t)
	lc, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveBroken, UnitID: firstUnit(lc)}); err != nil {
		t.Fatal(err)
	}
	for user, want := range map[int64]bool{liveOwner1: true, liveOwner2: false, matchOrganiser: true} {
		log, err := f.svc.Log(f.ctx, f.user(user), 5, 1, 0, 50)
		if err != nil || len(log) != 1 || log[0].CanUndo != want {
			t.Errorf("user %d: %+v %v", user, log, err)
		}
	}
}

// A simulated restart: a new service and hub on the same store give the same state and log, and a reconnecting client resyncs.
func TestLiveSurvivesARestart(t *testing.T) {
	f := newLiveFixture(t)
	mine, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	theirs, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)
	for _, act := range []LiveAction{
		{Kind: LiveWound, UnitID: firstUnit(mine), Targets: []WoundTarget{{UnitID: firstUnit(theirs), Caused: 1}}},
		{Kind: LiveBroken, UnitID: firstUnit(mine)},
	} {
		if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, act); err != nil {
			t.Fatal(err)
		}
	}
	logBefore, _ := f.svc.Log(f.ctx, f.user(liveOwner1), 5, 1, 0, 50)
	stateBefore, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)

	// the process dies; a new one starts on the same database
	feed2 := newFakeFeed()
	hub := NewLiveHub(feed2)
	svc2 := NewLiveService(f.repo, f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)}).WithHub(hub)
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	go func() { _ = hub.Start(ctx) }()
	<-feed2.started

	logAfter, err := svc2.Log(f.ctx, f.user(liveOwner1), 5, 1, 0, 50)
	if err != nil || len(logAfter) != len(logBefore) || logAfter[0].Text != logBefore[0].Text {
		t.Fatalf("log after restart: %+v %v", logAfter, err)
	}
	stateAfter, _ := svc2.Live(f.ctx, f.user(liveOwner2), 5, 2)
	if stateAfter.Seq != stateBefore.Seq || stateAfter.Units[0].Wounds != stateBefore.Units[0].Wounds || stateAfter.Units[0].Gains[GainFatePrevented] != stateBefore.Units[0].Gains[GainFatePrevented] {
		t.Errorf("state after restart: %+v vs %+v", stateAfter.Units[0], stateBefore.Units[0])
	}
	// a reconnecting client gets a batch for every company at once
	stream, err := svc2.Subscribe(ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	batch, err := stream.Next(ctx)
	if err != nil || len(batch.Companies) != 3 || !batch.Match || batch.Seq != stateAfter.Seq {
		t.Errorf("first batch: %+v %v", batch, err)
	}
	// and a write after the restart reaches it
	feed2.emit(LiveEvent{MatchID: 1, CompanyID: 2, Seq: stateAfter.Seq + 1, Kind: LiveChanged})
	batch, err = stream.Next(ctx)
	if err != nil || !slices.Equal(batch.Companies, []int64{2}) || batch.Seq != stateAfter.Seq+1 {
		t.Errorf("event batch: %+v %v", batch, err)
	}
}

func TestLiveSubscribeRules(t *testing.T) {
	f := newLiveFixture(t)
	feed := newFakeFeed()
	hub := NewLiveHub(feed)
	svc := NewLiveService(f.repo, f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)}).WithHub(hub)
	ctx, cancel := context.WithCancel(f.ctx)
	if _, err := svc.Subscribe(ctx, f.user(liveOwner1), 5, 1); err != nil {
		t.Fatal(err)
	}
	hub.mu.Lock()
	taken := hub.perUser[liveOwner1]
	hub.mu.Unlock()
	if taken != 1 {
		t.Fatal("slot taken")
	}
	cancel() // a cancelled request frees its slot
	deadline := time.Now().Add(2 * time.Second)
	for {
		hub.mu.Lock()
		n := hub.perUser[liveOwner1]
		hub.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot not freed on cancel: %d", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.match.Ended = true
	if _, err := svc.Subscribe(f.ctx, f.user(liveOwner1), 5, 1); !errors.Is(err, ErrRefused) {
		t.Errorf("ended match: %v", err)
	}
}

func TestLiveObjectiveAndMVPEndChecks(t *testing.T) {
	f := newLiveFixture(t)
	mine, _ := f.svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	theirs, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)
	mate, _ := f.svc.Live(f.ctx, f.user(liveOwner3), 5, 3)
	u1, u2 := mine.Units[0].UnitID, mine.Units[1].UnitID
	t2, t2b := theirs.Units[0].UnitID, theirs.Units[1].UnitID

	// objective: the owner sets it for several units; another company's owner cannot; the organiser can
	for _, u := range []int64{u1, u2} {
		if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner1), 5, 1, 1, LiveAction{Kind: LiveObjective, UnitID: u, On: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(liveOwner2), 5, 1, 1, LiveAction{Kind: LiveObjective, UnitID: u1, On: false}); !errors.Is(err, ErrRefused) {
		t.Errorf("a viewer: %v", err)
	}
	if _, _, err := f.svc.Apply(f.ctx, f.user(matchOrganiser), 5, 1, 2, LiveAction{Kind: LiveObjective, UnitID: t2, On: true}); err != nil {
		t.Errorf("the organiser: %v", err)
	}

	// MVP: company 1 (team 1) votes on company 2 (team 2); a teammate of company 2 cannot vote for company 2 or its own team; a stranger has no company
	vote := func(user int64, company, unit int64) error {
		_, _, err := f.svc.Apply(f.ctx, f.user(user), 5, 1, company, LiveAction{Kind: LiveMVPVote, UnitID: unit})
		return err
	}
	if err := vote(liveOwner1, 2, t2); err != nil {
		t.Fatal(err)
	}
	if err := vote(liveOwner3, 2, t2b); !errors.Is(err, ErrRefused) {
		t.Errorf("a teammate: %v", err)
	}
	if err := vote(liveOwner3, 1, u1); err != nil {
		t.Errorf("company 3 votes for company 1: %v", err)
	}
	if err := vote(7, 2, t2); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "no company") {
		t.Errorf("a stranger: %v", err)
	}
	if err := vote(liveOwner1, 1, u1); !errors.Is(err, ErrRefused) {
		t.Errorf("own company: %v", err)
	}
	if err := vote(liveOwner1, 2, mine.Units[0].UnitID); !errors.Is(err, ErrInvalidLive) {
		t.Errorf("a unit of another company: %v", err)
	}
	// changing the vote replaces it
	if err := vote(liveOwner1, 2, t2b); err != nil {
		t.Fatal(err)
	}

	ec, err := f.svc.EndChecks(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ec.Objective) != 1 || ec.Objective[0].CompanyID != 1 || len(ec.MVP) != 2 {
		t.Fatalf("the owner edits one company and votes on the two other teams' companies: %+v", ec)
	}
	for _, b := range ec.MVP {
		if b.CompanyID == 2 && b.Current != t2b {
			t.Errorf("current vote: %+v", b)
		}
	}
	var tally2 MVPTally
	for _, tl := range ec.Tally {
		if tl.CompanyID == 2 {
			tally2 = tl
		}
	}
	if tally2.Winner != t2b || len(tally2.Lines) != 1 {
		t.Errorf("tally: %+v", tally2)
	}
	if admin, _ := f.svc.EndChecks(f.ctx, User{ID: 5, Role: RoleAdmin}, 5, 1); len(admin.Objective) != 3 || len(admin.MVP) != 0 {
		t.Errorf("an administrator edits every company and votes through none: %d %d", len(admin.Objective), len(admin.MVP))
	}
	if len(ec.Pending) == 0 {
		t.Error("the reminder lists what is open")
	}
	_ = mate

	// undo of the changed vote restores the earlier one; only the voter (or an admin/organiser) may undo
	log, _ := f.svc.Log(f.ctx, f.user(liveOwner1), 5, 1, 0, 50)
	var last LiveEntry
	for _, e := range log {
		if e.Action.Kind == LiveMVPVote && e.Action.UnitID == t2b {
			last = e
		}
	}
	if _, err := f.svc.Undo(f.ctx, f.user(liveOwner2), 5, 1, last.ID, 2); !errors.Is(err, ErrRefused) {
		t.Errorf("the voted company's owner cannot undo the vote: %v", err)
	}
	if _, err := f.svc.Undo(f.ctx, f.user(liveOwner1), 5, 1, last.ID, 1); err != nil {
		t.Fatal(err)
	}
	ec, _ = f.svc.EndChecks(f.ctx, f.user(liveOwner1), 5, 1)
	for _, b := range ec.MVP {
		if b.CompanyID == 2 && b.Current != t2 {
			t.Errorf("the earlier vote is back: %+v", b)
		}
	}

	// the aftermath default carries the objective and the voted MVP
	sheets, ok, err := f.svc.AftermathDefaults(f.ctx, 5, 1, 1)
	m1 := map[int64]ModelSheet{}
	for _, sh := range sheets {
		m1[sh.UnitID] = sh
	}
	if err != nil || !ok || m1[u1].Gains[GainObjective] != 1 || m1[u2].Gains[GainObjective] != 1 || m1[u1].Gains[GainMVP] != 1 {
		t.Errorf("company 1: %+v %v %v", m1, ok, err)
	}
	sheets2, _, _ := f.svc.AftermathDefaults(f.ctx, 5, 1, 2)
	for _, sh := range sheets2 {
		if sh.UnitID == t2 && (sh.Gains[GainMVP] != 1 || sh.Gains[GainObjective] != 1) {
			t.Errorf("company 2: %+v", sh)
		}
	}

	// after the end nothing changes
	f.repo.ended[1], f.match.Ended = true, true
	if err := vote(liveOwner1, 2, t2); !errors.Is(err, ErrRefused) {
		t.Errorf("ended: %v", err)
	}
	if _, err := f.svc.EndChecks(f.ctx, f.user(liveOwner1), 5, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("ended match: %v", err)
	}
}

func TestSecretObjectivesAreTheirAuthorsUntilTheEnd(t *testing.T) {
	f := newLiveFixture(t)
	me, them := f.user(liveOwner1), f.user(liveOwner2)
	me.DisplayName, them.DisplayName = "Frodo", "Sam"
	apply := func(u User, company int64, a LiveAction) (LiveEntry, error) {
		_, e, err := f.svc.Apply(f.ctx, u, 5, 1, company, a)
		return e, err
	}
	own := LiveAction{Kind: LiveSecretNominate, ObjectiveKey: "ownHeroSurvives", UnitID: 102}

	if _, err := apply(me, 1, own); !errors.Is(err, ErrRefused) {
		t.Errorf("no scenario yet: %v", err)
	}
	f.match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: "Fog of War"}

	for name, tc := range map[string]struct {
		a   LiveAction
		err error
	}{
		"the Leader with other Heroes": {LiveAction{Kind: LiveSecretNominate, ObjectiveKey: "ownHeroSurvives", UnitID: 101}, ErrRefused},
		"a warrior":                    {LiveAction{Kind: LiveSecretNominate, ObjectiveKey: "ownHeroSurvives", UnitID: 106}, ErrRefused},
		"an enemy for an own hero":     {LiveAction{Kind: LiveSecretNominate, ObjectiveKey: "ownHeroSurvives", UnitID: 202}, ErrRefused},
		"an own hero for an enemy one": {LiveAction{Kind: LiveSecretNominate, ObjectiveKey: "enemyHeroHarmed", UnitID: 102}, ErrRefused},
		"a plain objective":            {LiveAction{Kind: LiveSecretNominate, ObjectiveKey: "enemyLeader", UnitID: 202}, ErrRefused},
		"terrain and a unit":           {LiveAction{Kind: LiveSecretNominate, ObjectiveKey: "terrainPiece", UnitID: 202, Note: "x"}, ErrInvalidLive},
		"a tier past the last":         {LiveAction{Kind: LiveSecretDone, ObjectiveKey: "ownHeroSurvives", Tier: 9}, ErrInvalidLive},
	} {
		if _, err := apply(me, 1, tc.a); !errors.Is(err, tc.err) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// the author notes and reports; it can be changed (the last entry counts)
	for _, a := range []LiveAction{own,
		{Kind: LiveSecretNominate, ObjectiveKey: "enemyHeroHarmed", UnitID: 202},
		{Kind: LiveSecretNominate, ObjectiveKey: "terrainPiece", Note: "the old mill"},
		{Kind: LiveSecretDone, ObjectiveKey: "ownHeroSurvives", Tier: 1},
		{Kind: LiveSecretDone, ObjectiveKey: "ownHeroSurvives", Tier: 2}} {
		if _, err := apply(me, 1, a); err != nil {
			t.Fatalf("%v: %v", a.Kind, err)
		}
	}
	mine, err := f.svc.SecretObjectives(f.ctx, me, 5, 1)
	if err != nil || mine.Revealed || len(mine.Lines) != 3 {
		t.Fatalf("the author sees her three: %+v %v", mine, err)
	}
	for _, l := range mine.Lines {
		if l.Key == "ownHeroSurvives" && (l.Tier != 2 || l.Points != 3 || l.Nominee == "" || l.Label != "Own hero survives") {
			t.Errorf("own hero: %+v", l)
		}
	}

	// everybody else sees nothing: not the log, not the view (the organiser and an administrator are players too)
	for _, u := range []User{them, f.user(liveOwner3), f.user(matchOrganiser), {ID: 5, Role: RoleAdmin}} {
		view, err := f.svc.SecretObjectives(f.ctx, u, 5, 1)
		if err != nil || len(view.Lines) != 0 || view.Revealed || !slices.Equal(view.Others, []string{"Rangers"}) {
			t.Errorf("user %d sees %+v %v", u.ID, view, err)
		}
		log, _ := f.svc.Log(f.ctx, u, 5, 1, 0, 50)
		for _, e := range log {
			if e.Action.Kind.Secret() || strings.Contains(e.Text, "ownHeroSurvives") || strings.Contains(e.Text, "old mill") {
				t.Errorf("user %d sees the secret entry %+v", u.ID, e)
			}
		}
	}
	log, _ := f.svc.Log(f.ctx, me, 5, 1, 0, 50)
	n := 0
	for _, e := range log {
		if e.Action.Kind.Secret() {
			n++
			if !e.CanUndo {
				t.Error("the author can undo her entry")
			}
		}
	}
	if n != 5 {
		t.Errorf("the author's log has her five entries: %d", n)
	}
	// only the author undoes a secret entry; the organiser, who cannot see it, cannot either
	var first LiveEntry
	for _, e := range log {
		if e.Action.Kind == LiveSecretNominate {
			first = e
		}
	}
	for _, u := range []User{them, f.user(matchOrganiser)} {
		if _, err := f.svc.Undo(f.ctx, u, 5, 1, first.ID, 2); !errors.Is(err, ErrRefused) {
			t.Errorf("user %d undoes a secret: %v", u.ID, err)
		}
	}
	if _, err := f.svc.Undo(f.ctx, me, 5, 1, first.ID, 1); err != nil {
		t.Errorf("the author undoes: %v", err)
	}

	// after the end everything is revealed, the score is ready for the result form
	f.repo.ended[1], f.match.Ended = true, true
	view, err := f.svc.SecretObjectives(f.ctx, them, 5, 1)
	if err != nil || !view.Revealed || len(view.Lines) == 0 {
		t.Fatalf("revealed: %+v %v", view, err)
	}
	points, lines, ok, err := f.svc.SecretScore(f.ctx, them, 5, 1, 1)
	if err != nil || !ok || points != 3 || len(lines) != 1 {
		t.Errorf("score: %d %+v %v %v", points, lines, ok, err)
	}
	log, _ = f.svc.Log(f.ctx, them, 5, 1, 0, 50)
	seen := 0
	for _, e := range log {
		if e.Action.Kind.Secret() {
			seen++
		}
	}
	if seen == 0 {
		t.Error("the log shows the secret entries after the end")
	}
	f.repo.ended[1], f.match.Ended = false, false
}

func TestMVPHintForTheAftermathSheet(t *testing.T) {
	f := newLiveFixture(t)
	theirs, _ := f.svc.Live(f.ctx, f.user(liveOwner2), 5, 2)
	a, b := theirs.Units[0].UnitID, theirs.Units[1].UnitID
	if h, err := f.svc.MVPHint(f.ctx, 5, 1, 2); err != nil || h != nil {
		t.Fatalf("no votes: %+v %v", h, err)
	}
	vote := func(user, unit int64) {
		if _, _, err := f.svc.Apply(f.ctx, f.user(user), 5, 1, 2, LiveAction{Kind: LiveMVPVote, UnitID: unit}); err != nil {
			t.Fatal(err)
		}
	}
	vote(liveOwner1, a)
	if h, _ := f.svc.MVPHint(f.ctx, 5, 1, 2); h == nil || h.Winner != a || len(h.Tied) != 0 || h.Lines[0].Votes != 1 || h.Lines[0].Title == "" {
		t.Errorf("one vote: %+v", h)
	}
	vote(liveOwner1, b) // the same voter changes: still one vote
	if h, _ := f.svc.MVPHint(f.ctx, 5, 1, 2); h.Winner != b {
		t.Errorf("changed vote: %+v", h)
	}
}

// TestRollCourageEnteredDiceAreValidated (bead 1oe): dice typed by the player outside 1 to 6, or a single one, are a refusal-grade
// input error of the live mode (ErrInvalidLive, a 422), never the aftermath's error (which the web layer answers with a 500).
func TestRollCourageEnteredDiceAreValidated(t *testing.T) {
	f := newLiveFixture(t)
	f.campaigns = NewMockcampaignRepository(gomock.NewController(t))
	f.campaigns.EXPECT().Hashes(gomock.Any(), int64(5)).Return(RulesetHashes{BattleCompanies: "bc"}, CampaignSettings{ManualRolls: true}, nil).AnyTimes()
	f.campaigns.EXPECT().Get(gomock.Any(), gomock.Any(), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{ID: 5, Status: CampaignActive}, OrganiserID: matchOrganiser}, nil).AnyTimes()
	svc := NewLiveService(f.repo, f.campaigns, f.companies, f.matches, staticRules{shippedRules(t)})
	lc, err := svc.Live(f.ctx, f.user(liveOwner1), 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	unit := firstUnit(lc)
	for _, dice := range [][]int{{0, 3}, {3, 0}, {7, 3}, {-1, 3}, {3}, {1, 2, 3}, {1 << 40, 1}, {-1 << 40, 1}} {
		if _, err := svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, unit, dice, false); !errors.Is(err, ErrInvalidLive) || errors.Is(err, ErrInvalidAftermath) {
			t.Errorf("dice %v: %v", dice, err)
		}
	}
	if r, err := svc.RollCourage(f.ctx, f.user(liveOwner1), 5, 1, unit, []int{6, 6}, false); err != nil || r.Total != 12 {
		t.Errorf("two valid dice: %+v %v", r, err)
	}
}
