package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// liveWorld is a campaign with two companies in an open match.
type liveWorld struct {
	s        *Store
	ctx      context.Context
	org, p2  core.User
	p3       core.User
	camp     int64
	a, b     core.CampaignCompany
	match    core.Match
	unitsOf  map[int64][]int64
	campaign core.NewCampaign
}

func newLiveWorld(t *testing.T, tag string) liveWorld {
	t.Helper()
	s, ctx := testStore(t)
	w := liveWorld{s: s, ctx: ctx, org: testUser(t, s, ctx, tag+"org"), p2: testUser(t, s, ctx, tag+"two"), unitsOf: map[int64][]int64{}}
	camp, err := s.Campaigns().Create(ctx, w.org, testNewCampaign(t, s, ctx, "Live "+tag))
	if err != nil {
		t.Fatal(err)
	}
	w.camp = camp.ID
	if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, w.p2.ID); err != nil {
		t.Fatal(err)
	}
	units := []core.UnitState{{Base: "Captain", Role: core.RoleLeader}, {Base: "Warrior", Role: core.RoleWarrior}}
	mk := func(owner core.User, name string) core.CampaignCompany {
		c, err := s.Companies().Create(ctx, camp.ID, owner, core.NewCompany{Name: name, Faction: "Rohan", Units: units})
		if err != nil {
			t.Fatal(err)
		}
		st, err := s.Companies().Read(ctx, camp.ID, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range st.Units {
			w.unitsOf[c.ID] = append(w.unitsOf[c.ID], u.ID)
		}
		return c
	}
	w.a, w.b = mk(w.org, "Alpha"), mk(w.p2, "Beta")
	w.match, err = s.Matches().Create(ctx, camp.ID, w.org, []core.NewSide{{Name: "A", Companies: []int64{w.a.ID}}, {Name: "B", Companies: []int64{w.b.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func (w liveWorld) inits() []core.LiveUnitInit {
	var out []core.LiveUnitInit
	for _, c := range []int64{w.a.ID, w.b.ID} {
		for i, u := range w.unitsOf[c] {
			out = append(out, core.LiveUnitInit{UnitID: u, CompanyID: c, Wounds: 2, Might: 1, Will: 1, Fate: i, CountsAsKill: i == 0})
		}
	}
	return out
}

func (w liveWorld) append(t *testing.T, by core.User, e core.LiveEntry) (core.LiveEntry, error) {
	t.Helper()
	return w.s.Live().Update(w.ctx, w.camp, w.match.ID, by, func(core.LiveSnapshot) (core.LiveChange, error) {
		return core.LiveChange{Append: &e}, nil
	})
}

func stat(unit, company int64, delta int, touches ...int64) core.LiveEntry {
	return core.LiveEntry{CompanyID: company, UnitID: unit, Touches: touches, Text: "t",
		Action: core.LiveAction{Kind: core.LiveStatChange, UnitID: unit, Stat: "might", Delta: delta}}
}

func TestLiveInitReadLog(t *testing.T) {
	w := newLiveWorld(t, "li")
	live := w.s.Live()
	snap, err := live.Read(w.ctx, w.camp, w.match.ID)
	if err != nil || snap.Initialised || snap.Number != 1 || snap.Ended || len(snap.Units) != 0 {
		t.Fatalf("before init: %+v, %v", snap, err)
	}
	if _, err := live.Read(w.ctx, w.camp+1000, w.match.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("other campaign: %v", err)
	}
	if err := live.Init(w.ctx, w.camp, w.match.ID, w.inits()); err != nil {
		t.Fatal(err)
	}
	// A second Init (even with other values) stores nothing more.
	if err := live.Init(w.ctx, w.camp, w.match.ID, w.inits()[:1]); err != nil {
		t.Fatal(err)
	}
	snap, err = live.Read(w.ctx, w.camp, w.match.ID)
	if err != nil || !snap.Initialised || len(snap.Units) != 4 || snap.Seq != 0 || snap.Units[0].Wounds != 2 || !snap.Units[0].CountsAsKill {
		t.Fatalf("after init: %+v, %v", snap, err)
	}
	if err := live.Init(w.ctx, w.camp+1000, w.match.ID, nil); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("init in another campaign: %v", err)
	}

	// Units of a company that is not in the match, negative starts, duplicates.
	w2 := newLiveWorld(t, "lj")
	bad := w2.inits()
	bad[0].CompanyID = w.a.ID
	if err := w2.s.Live().Init(w2.ctx, w2.camp, w2.match.ID, bad); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("company not in the match: %v", err)
	}
	neg := w2.inits()
	neg[0].Fate = -1
	if err := w2.s.Live().Init(w2.ctx, w2.camp, w2.match.ID, neg); !errors.Is(err, core.ErrInvalidLive) {
		t.Fatalf("negative start: %v", err)
	}
	if s2, _ := w2.s.Live().Read(w2.ctx, w2.camp, w2.match.ID); s2.Initialised {
		t.Fatal("a failed Init stores nothing")
	}

	// Append, Log, Undo.
	ua := w.unitsOf[w.a.ID][0]
	e1, err := w.append(t, w.org, stat(ua, w.a.ID, 1))
	if err != nil || e1.Seq != 1 || e1.ID == 0 || e1.By != "liorg" || e1.On.IsZero() {
		t.Fatalf("append: %+v, %v", e1, err)
	}
	wound := core.LiveEntry{CompanyID: w.a.ID, UnitID: ua, Touches: []int64{w.b.ID}, Text: "hit",
		Action: core.LiveAction{Kind: core.LiveWound, UnitID: ua, Targets: []core.WoundTarget{{UnitID: w.unitsOf[w.b.ID][1], Caused: 2, SavedByFate: 1}}}}
	e2, err := w.append(t, w.p2, wound)
	if err != nil || e2.Seq != 2 {
		t.Fatalf("append 2: %+v, %v", e2, err)
	}
	snap, _ = live.Read(w.ctx, w.camp, w.match.ID)
	if snap.Seq != 2 || len(snap.Entries) != 2 || snap.Entries[1].Action.Targets[0].SavedByFate != 1 || snap.Entries[1].Touches[0] != w.b.ID || snap.Entries[0].Touches != nil {
		t.Fatalf("snapshot entries: %+v", snap)
	}
	log, err := live.Log(w.ctx, w.camp, w.match.ID, 0, 10)
	if err != nil || len(log) != 2 || log[0].Seq != 2 || log[0].By != "litwo" || log[0].Action.Kind != core.LiveWound {
		t.Fatalf("log: %+v, %v", log, err)
	}
	if log, _ = live.Log(w.ctx, w.camp, w.match.ID, 2, 10); len(log) != 1 || log[0].Seq != 1 {
		t.Fatalf("log before: %+v", log)
	}
	if log, _ = live.Log(w.ctx, w.camp, w.match.ID, 0, 1); len(log) != 1 || log[0].Seq != 2 {
		t.Fatalf("log limit: %+v", log)
	}

	undone, err := live.Update(w.ctx, w.camp, w.match.ID, w.org, func(s core.LiveSnapshot) (core.LiveChange, error) {
		return core.LiveChange{Undo: s.Entries[0].ID}, nil
	})
	if err != nil || undone.ID != e1.ID || undone.Seq != 1 {
		t.Fatalf("undo returns the undone entry: %+v, %v", undone, err)
	}
	snap, _ = live.Read(w.ctx, w.camp, w.match.ID)
	if snap.Seq != 3 || len(snap.Entries) != 1 || snap.Entries[0].ID != e2.ID {
		t.Fatalf("undo takes a seq and leaves Entries: %+v", snap)
	}
	if log, _ = live.Log(w.ctx, w.camp, w.match.ID, 0, 10); len(log) != 1 {
		t.Fatalf("log after undo: %+v", log)
	}
	var rows, undoneBy, undoneSeq int64
	if err := w.s.pool.QueryRow(w.ctx, `SELECT count(*), max(undone_by), max(undone_seq) FROM match_live_log WHERE match_id = $1`, w.match.ID).Scan(&rows, &undoneBy, &undoneSeq); err != nil ||
		rows != 2 || undoneBy != w.org.ID || undoneSeq != 3 {
		t.Fatalf("audit: %d %d %d %v", rows, undoneBy, undoneSeq, err)
	}
	// An entry is undone once; ids of another match are not found; a bad change is rejected and stores nothing.
	if _, err := live.Update(w.ctx, w.camp, w.match.ID, w.org, func(core.LiveSnapshot) (core.LiveChange, error) {
		return core.LiveChange{Undo: e1.ID}, nil
	}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("undo twice: %v", err)
	}
	if _, err := live.Update(w.ctx, w.camp, w.match.ID, w.org, func(core.LiveSnapshot) (core.LiveChange, error) {
		return core.LiveChange{}, nil
	}); !errors.Is(err, core.ErrInvalidLive) {
		t.Fatalf("empty change: %v", err)
	}
	if snap, _ = live.Read(w.ctx, w.camp, w.match.ID); snap.Seq != 3 {
		t.Fatalf("failed updates must not take a seq: %d", snap.Seq)
	}

	// Ended is read under the lock; deleting the match cascades.
	if _, err := w.s.Matches().Update(w.ctx, w.camp, w.match.ID, w.org, func(m *core.Match) error { m.Ended = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := live.Init(w.ctx, w.camp, w.match.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := live.Update(w.ctx, w.camp, w.match.ID, w.org, func(s core.LiveSnapshot) (core.LiveChange, error) {
		if !s.Ended {
			t.Error("the snapshot of an ended match says Ended")
		}
		return core.LiveChange{}, core.ErrRefused
	}); !errors.Is(err, core.ErrRefused) {
		t.Fatal(err)
	}
	if _, err := w.s.pool.Exec(w.ctx, `DELETE FROM matches WHERE id = $1`, w.match.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := w.s.pool.QueryRow(w.ctx, `SELECT (SELECT count(*) FROM match_live WHERE match_id = $1) + (SELECT count(*) FROM match_live_unit WHERE match_id = $1)
	  + (SELECT count(*) FROM match_live_log WHERE match_id = $1)`, w.match.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("cascade: %d, %v", n, err)
	}
}

func TestLiveConcurrentAndNotify(t *testing.T) {
	w := newLiveWorld(t, "lc")
	live := w.s.Live()
	if err := live.Init(w.ctx, w.camp, w.match.ID, w.inits()); err != nil {
		t.Fatal(err)
	}
	// A second connection listens.
	conn, err := pgx.Connect(w.ctx, w.s.url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(w.ctx)
	if _, err := conn.Exec(w.ctx, "LISTEN "+liveChannel); err != nil {
		t.Fatal(err)
	}

	// An error rolls back and sends nothing.
	boom := errors.New("boom")
	if _, err := live.Update(w.ctx, w.camp, w.match.ID, w.org, func(core.LiveSnapshot) (core.LiveChange, error) { return core.LiveChange{}, boom }); err != boom {
		t.Fatalf("error as is: %v", err)
	}
	// One touched company: two notifications (acting + touched), one more for a plain entry.
	ua := w.unitsOf[w.a.ID][0]
	if _, err := w.append(t, w.org, stat(ua, w.a.ID, 1, w.b.ID, w.a.ID)); err != nil {
		t.Fatal(err)
	}
	got := map[int64]int{}
	for range 2 {
		wctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
		n, err := conn.WaitForNotification(wctx)
		cancel()
		if err != nil {
			t.Fatalf("notification: %v", err)
		}
		var p liveNotice
		if err := jsonUnmarshal(n.Payload, &p); err != nil || p.Match != w.match.ID || p.Seq != 1 || p.Kind != "changed" {
			t.Fatalf("payload %q: %v", n.Payload, err)
		}
		got[p.Company]++
	}
	if got[w.a.ID] != 1 || got[w.b.ID] != 1 {
		t.Fatalf("one notification per company: %v", got)
	}
	wctx, cancel := context.WithTimeout(w.ctx, 300*time.Millisecond)
	defer cancel()
	if n, err := conn.WaitForNotification(wctx); err == nil {
		t.Fatalf("nothing more expected (the failed update sent nothing): %+v", n)
	}

	// 20 concurrent writers: seq 1.. without gaps or duplicates.
	var wg sync.WaitGroup
	seqs := make(chan int64, 20)
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			by := w.org
			if i%2 == 1 {
				by = w.p2
			}
			e, err := w.append(t, by, stat(ua, w.a.ID, 1))
			if err != nil {
				t.Error(err)
				return
			}
			seqs <- e.Seq
		}()
	}
	wg.Wait()
	close(seqs)
	seen := map[int64]bool{}
	for q := range seqs {
		seen[q] = true
	}
	for q := int64(2); q <= 21; q++ {
		if !seen[q] {
			t.Fatalf("seq %d missing: %v", q, seen)
		}
	}
	if len(seen) != 20 {
		t.Fatalf("duplicates: %v", seen)
	}
}

func TestLiveFeed(t *testing.T) {
	w := newLiveWorld(t, "lf")
	live := w.s.Live()
	if err := live.Init(w.ctx, w.camp, w.match.ID, w.inits()); err != nil {
		t.Fatal(err)
	}
	feed := w.s.LiveFeed()
	feed.minBackoff, feed.idle = 100*time.Millisecond, 200*time.Millisecond // exercises the ping path too
	ctx, cancel := context.WithCancel(w.ctx)
	events := make(chan core.LiveEvent, 16)
	resyncs := make(chan struct{}, 16)
	done := make(chan error, 1)
	go func() {
		done <- feed.Run(ctx, func(e core.LiveEvent) { events <- e }, func() { resyncs <- struct{}{} })
	}()
	waitResync := func() {
		t.Helper()
		select {
		case <-resyncs:
		case <-time.After(10 * time.Second):
			t.Fatal("no resync")
		}
	}
	next := func() core.LiveEvent {
		t.Helper()
		select {
		case e := <-events:
			return e
		case <-time.After(10 * time.Second):
			t.Fatal("no event")
			return core.LiveEvent{}
		}
	}
	waitResync()

	// Nothing before the commit.
	ua := w.unitsOf[w.a.ID][0]
	tx, err := w.s.pool.Begin(w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := notifyLive(w.ctx, tx, liveNotice{Match: w.match.ID, Company: w.a.ID, Seq: 9, Kind: "changed"}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-events:
		t.Fatalf("event before commit: %+v", e)
	case <-time.After(500 * time.Millisecond): // longer than the idle ping
	}
	if err := tx.Commit(w.ctx); err != nil {
		t.Fatal(err)
	}
	if e := next(); e != (core.LiveEvent{MatchID: w.match.ID, CompanyID: w.a.ID, Seq: 9, Kind: core.LiveChanged}) {
		t.Fatalf("event: %+v", e)
	}

	// Ending and reopening each send one "ended".
	for _, ended := range []bool{true, false} {
		if _, err := w.s.Matches().Update(w.ctx, w.camp, w.match.ID, w.org, func(m *core.Match) error { m.Ended = ended; return nil }); err != nil {
			t.Fatal(err)
		}
		if e := next(); e.Kind != core.LiveEnded || e.MatchID != w.match.ID || e.CompanyID != 0 {
			t.Fatalf("ended event: %+v", e)
		}
	}
	// A plain save does not.
	if _, err := w.s.Matches().Update(w.ctx, w.camp, w.match.ID, w.org, func(m *core.Match) error { m.Result.Note = "x"; return nil }); err != nil {
		t.Fatal(err)
	}

	// Kill the listening connection: the feed reconnects, resyncs and goes on.
	if _, err := w.s.pool.Exec(w.ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = $1`, liveFeedName); err != nil {
		t.Fatal(err)
	}
	waitResync()
	if _, err := w.append(t, w.org, stat(ua, w.a.ID, 1)); err != nil {
		t.Fatal(err)
	}
	if e := next(); e.Kind != core.LiveChanged || e.CompanyID != w.a.ID || e.Seq != 1 {
		t.Fatalf("after reconnect: %+v", e)
	}
	select {
	case e := <-events:
		t.Fatalf("unexpected event (plain save must not notify): %+v", e)
	default:
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestOpenMatchGuards(t *testing.T) {
	w := newLiveWorld(t, "og")
	w.p3 = testUser(t, w.s, w.ctx, "ogthree")
	if _, err := w.s.pool.Exec(w.ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, w.camp, w.p3.ID); err != nil {
		t.Fatal(err)
	}
	companies, matches := w.s.Companies(), w.s.Matches()
	st, err := companies.Read(w.ctx, w.camp, w.a.ID)
	if err != nil || st.OpenMatch == nil || st.OpenMatch.MatchID != w.match.ID || st.OpenMatch.Number != 1 {
		t.Fatalf("open match on read: %+v, %v", st.OpenMatch, err)
	}
	if err := companies.Update(w.ctx, w.camp, w.a.ID, func(st *core.CompanyState) error {
		if st.OpenMatch == nil {
			t.Error("OpenMatch on update")
		}
		st.OpenMatch = nil // ignored on write
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	end := func(id int64, ended bool) (core.Match, error) {
		return matches.Update(w.ctx, w.camp, id, w.org, func(m *core.Match) error { m.Ended = ended; return nil })
	}
	if _, err := end(w.match.ID, true); err != nil {
		t.Fatal(err)
	}
	if st, _ = companies.Read(w.ctx, w.camp, w.a.ID); st.OpenMatch != nil {
		t.Fatalf("ended match: %+v", st.OpenMatch)
	}

	// Two concurrent Creates with the same company: one wins.
	extra, err := companies.Create(w.ctx, w.camp, w.p3, core.NewCompany{Name: "Gamma", Faction: "Rohan", Units: []core.UnitState{{Base: "Captain", Role: core.RoleLeader}}})
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, foe := range []int64{w.b.ID, extra.ID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := matches.Create(w.ctx, w.camp, w.org, []core.NewSide{{Name: "X", Companies: []int64{w.a.ID}}, {Name: "Y", Companies: []int64{foe}}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	var ok, busyN int
	for err := range errs {
		var busy *core.CompanyBusyError
		switch {
		case err == nil:
			ok++
		case errors.As(err, &busy) && errors.Is(err, core.ErrRefused) && busy.Company == "Alpha" && busy.Number >= 2:
			busyN++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	if ok != 1 || busyN != 1 {
		t.Fatalf("exactly one create wins: ok %d busy %d", ok, busyN)
	}
	st, _ = companies.Read(w.ctx, w.camp, w.a.ID)
	if st.OpenMatch == nil || st.OpenMatch.Number != 2 {
		t.Fatalf("open match 2: %+v", st.OpenMatch)
	}

	// Reopening match 1 while Alpha is in open match 2 is refused; after match 2 ends it works.
	_, err = end(w.match.ID, false)
	var busy *core.CompanyBusyError
	if !errors.As(err, &busy) || busy.Number != 2 || busy.Company != "Alpha" {
		t.Fatalf("reopen: %v", err)
	}
	if m, _ := matches.Get(w.ctx, w.camp, w.match.ID); !m.Ended {
		t.Fatal("a refused reopen changes nothing")
	}
	if _, err := end(busy.MatchID, true); err != nil {
		t.Fatal(err)
	}
	if m, err := end(w.match.ID, false); err != nil || m.Ended {
		t.Fatalf("reopen: %+v, %v", m, err)
	}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

// The new kinds survive the payload, and a secret objective notifies nobody.
func TestLiveEndChecksAndSecretEntries(t *testing.T) {
	w := newLiveWorld(t, "sec")
	live := w.s.Live()
	if err := live.Init(w.ctx, w.camp, w.match.ID, w.inits()); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(w.ctx, w.s.url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(w.ctx)
	if _, err := conn.Exec(w.ctx, "LISTEN "+liveChannel); err != nil {
		t.Fatal(err)
	}
	ua, ub := w.unitsOf[w.a.ID][0], w.unitsOf[w.b.ID][0]
	for _, e := range []core.LiveEntry{
		{CompanyID: w.a.ID, UnitID: ua, Text: "o", Action: core.LiveAction{Kind: core.LiveObjective, UnitID: ua, On: true}},
		{CompanyID: w.b.ID, UnitID: ub, Text: "v", Action: core.LiveAction{Kind: core.LiveMVPVote, UnitID: ub, TargetCompanyID: w.b.ID}},
	} {
		if _, err := w.append(t, w.org, e); err != nil {
			t.Fatalf("%s: %v", e.Action.Kind, err)
		}
	}
	for range 2 { // one notification each
		wctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
		_, err := conn.WaitForNotification(wctx)
		cancel()
		if err != nil {
			t.Fatalf("notification: %v", err)
		}
	}
	for _, e := range []core.LiveEntry{
		{CompanyID: w.a.ID, UnitID: ub, Text: "n", Action: core.LiveAction{Kind: core.LiveSecretNominate, ObjectiveKey: "enemyHeroHarmed", UnitID: ub, Note: ""}},
		{CompanyID: w.a.ID, Text: "d", Action: core.LiveAction{Kind: core.LiveSecretDone, ObjectiveKey: "enemyHeroHarmed", Tier: 2}},
	} {
		if _, err := w.append(t, w.org, e); err != nil {
			t.Fatalf("%s: %v", e.Action.Kind, err)
		}
	}
	wctx, cancel := context.WithTimeout(w.ctx, 400*time.Millisecond)
	defer cancel()
	if n, err := conn.WaitForNotification(wctx); err == nil {
		t.Fatalf("a secret entry must notify nobody: %+v", n)
	}
	snap, err := live.Read(w.ctx, w.camp, w.match.ID)
	if err != nil || len(snap.Entries) != 4 {
		t.Fatalf("%v %+v", err, snap.Entries)
	}
	got := map[core.LiveKind]core.LiveAction{}
	for _, e := range snap.Entries {
		got[e.Action.Kind] = e.Action
	}
	if !got[core.LiveObjective].On || got[core.LiveMVPVote].TargetCompanyID != w.b.ID || got[core.LiveSecretNominate].ObjectiveKey != "enemyHeroHarmed" || got[core.LiveSecretDone].Tier != 2 {
		t.Errorf("payloads: %+v", got)
	}
}

// The kinds of the Break Point and the Against the Odds items (migration 00021): every field of the payload survives, a match-level
// turn entry (company 0, unit 0) is stored, undo works on them, and a kind nobody declared is still refused by the constraint.
func TestLiveBreakPointAndItemKinds(t *testing.T) {
	w := newLiveWorld(t, "brk")
	live := w.s.Live()
	if err := live.Init(w.ctx, w.camp, w.match.ID, w.inits()); err != nil {
		t.Fatal(err)
	}
	ua := w.unitsOf[w.a.ID][0]
	actions := []core.LiveEntry{
		{CompanyID: 0, Text: "Turn 2", Action: core.LiveAction{Kind: core.LiveTurn, Turn: 2}},
		{CompanyID: w.a.ID, Text: "adj", Action: core.LiveAction{Kind: core.LiveBreakAdjust, Field: "casualties", Delta: -1, Note: "pony"}},
		{CompanyID: w.a.ID, UnitID: ua, Text: "skip", Action: core.LiveAction{Kind: core.LiveCourageSkip, UnitID: ua, Reason: "standFast"}},
		{CompanyID: w.a.ID, UnitID: ua, Text: "passed", Action: core.LiveAction{Kind: core.LiveBroken, UnitID: ua, StandFast: true}},
		{CompanyID: w.a.ID, Text: "flee", Action: core.LiveAction{Kind: core.LiveAllIsLost, Flee: true}},
		{CompanyID: w.a.ID, UnitID: ua, Text: "used", Action: core.LiveAction{Kind: core.LiveItemUsed, UnitID: ua, Item: "Dwarven Brew", Index: 2}},
	}
	var last core.LiveEntry
	for _, e := range actions {
		var err error
		if last, err = w.append(t, w.org, e); err != nil {
			t.Fatalf("%s: %v", e.Action.Kind, err)
		}
	}
	snap, err := live.Read(w.ctx, w.camp, w.match.ID)
	if err != nil || len(snap.Entries) != len(actions) {
		t.Fatalf("%v %+v", err, snap.Entries)
	}
	for i, want := range actions {
		got := snap.Entries[i]
		if got.Action.Kind != want.Action.Kind || got.CompanyID != want.CompanyID || got.UnitID != want.UnitID {
			t.Errorf("entry %d: %+v, want %+v", i, got, want)
		}
		if g, w := got.Action, want.Action; g.Turn != w.Turn || g.Field != w.Field || g.Delta != w.Delta || g.Note != w.Note || g.Reason != w.Reason ||
			g.StandFast != w.StandFast || g.Flee != w.Flee || g.Item != w.Item || g.Index != w.Index || g.UnitID != w.UnitID {
			t.Errorf("payload %d: %+v, want %+v", i, g, w)
		}
	}
	// undo a new kind: the snapshot no longer carries it
	if _, err := live.Update(w.ctx, w.camp, w.match.ID, w.org, func(core.LiveSnapshot) (core.LiveChange, error) {
		return core.LiveChange{Undo: last.ID}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if snap, err = live.Read(w.ctx, w.camp, w.match.ID); err != nil || len(snap.Entries) != len(actions)-1 {
		t.Fatalf("undo: %v %+v", err, snap.Entries)
	}
	// a restart changes nothing: a new store on the same database reads the same entries
	again, err := Open(w.ctx, w.s.url)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if re, err := again.Live().Read(w.ctx, w.camp, w.match.ID); err != nil || len(re.Entries) != len(actions)-1 || re.Entries[0].Action.Turn != 2 {
		t.Fatalf("after a restart: %v %+v", err, re.Entries)
	}
	if _, err := w.append(t, w.org, core.LiveEntry{CompanyID: w.a.ID, Text: "x", Action: core.LiveAction{Kind: "nonsense"}}); !errors.Is(err, core.ErrInvalidLive) {
		t.Errorf("an unknown kind must be refused: %v", err)
	}
}

// The informational lines of a pending end (6uh.3) are accepted by the log's kind constraint.
func TestLiveEndLinesAreStored(t *testing.T) {
	w := newLiveWorld(t, "end")
	live := w.s.Live()
	if err := live.Init(w.ctx, w.camp, w.match.ID, w.inits()); err != nil {
		t.Fatal(err)
	}
	for _, k := range []core.LiveKind{core.LiveEndProposed, core.LiveEndConfirmed, core.LiveEndWithdrawn, core.LiveEndCancelled, core.LiveEndOverride} {
		if _, err := w.append(t, w.org, core.LiveEntry{CompanyID: w.a.ID, Text: string(k), Action: core.LiveAction{Kind: k}}); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
	if snap, err := live.Read(w.ctx, w.camp, w.match.ID); err != nil || len(snap.Entries) != 5 {
		t.Fatalf("%v %+v", err, snap.Entries)
	}
}

// Read answers in one statement with JSON; snapshot (used under the lock by Update) in several. They must agree for every match.
func TestLiveReadEqualsSnapshot(t *testing.T) {
	s, ctx := testStore(t)
	ls := s.Live()
	rows, err := s.pool.Query(ctx, `SELECT campaign_id, id FROM matches ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var ids [][2]int64
	for rows.Next() {
		var p [2]int64
		if err := rows.Scan(&p[0], &p[1]); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p)
	}
	rows.Close()
	for _, p := range ids {
		got, err := ls.Read(ctx, p[0], p[1])
		if err != nil {
			t.Fatal(err)
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want, err := ls.snapshot(ctx, tx, p[0], p[1])
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("match %d: Read differs from snapshot\n got %+v\nwant %+v", p[1], got, want)
		}
	}
	t.Logf("%d matches compared", len(ids))
}
