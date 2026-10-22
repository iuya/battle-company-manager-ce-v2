package core

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"testing"
	"time"
)

func startHub(t *testing.T) (*LiveHub, *fakeFeed) {
	t.Helper()
	feed := newFakeFeed()
	hub := NewLiveHub(feed)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = hub.Start(ctx) }()
	<-feed.started
	return hub, feed
}

func next(t *testing.T, s LiveStream) LiveBatch {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, err := s.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	return b
}

func TestHubFirstBatchIsAnInitialSyncOfAllCompanies(t *testing.T) {
	hub, _ := startHub(t)
	s, err := hub.subscribe(1, 7, []int64{1, 2}, 41)
	if err != nil {
		t.Fatal(err)
	}
	b := next(t, s)
	if !slices.Equal(b.Companies, []int64{1, 2}) || b.Seq != 41 || !b.Match || b.Ended {
		t.Errorf("%+v", b)
	}
}

func TestHubCoalescesEventsForASlowClient(t *testing.T) {
	hub, feed := startHub(t)
	s, _ := hub.subscribe(1, 7, []int64{1, 2, 3}, 0)
	next(t, s)
	for i := range 1000 {
		feed.emit(LiveEvent{MatchID: 7, CompanyID: int64(i%3 + 1), Seq: int64(i + 1), Kind: LiveChanged})
	}
	st := s.(*liveStream)
	st.mu.Lock()
	n := len(st.pending.Companies)
	st.mu.Unlock()
	if n != 3 || len(st.wake) != 1 {
		t.Errorf("one pending batch with three companies: %d, wake %d", n, len(st.wake))
	}
	b := next(t, s)
	if b.Seq != 1000 || len(b.Companies) != 3 {
		t.Errorf("%+v", b)
	}
	feed.emit(LiveEvent{MatchID: 7, Kind: LiveEnded})
	if b := next(t, s); !b.Ended {
		t.Errorf("%+v", b)
	}
}

func TestHubKeepsMatchesApartAndFreesSlots(t *testing.T) {
	hub, feed := startHub(t)
	a, _ := hub.subscribe(1, 7, []int64{1}, 0)
	b, _ := hub.subscribe(2, 8, []int64{5}, 0)
	next(t, a)
	next(t, b)
	feed.emit(LiveEvent{MatchID: 7, CompanyID: 1, Seq: 2, Kind: LiveChanged})
	if got := next(t, a); got.Seq != 2 {
		t.Errorf("%+v", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := b.Next(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the other match hears nothing: %v", err)
	}
	a.Close()
	a.Close() // idempotent
	if hub.perUser[1] != 0 || hub.perMatch[7] != 0 {
		t.Errorf("slots freed: %v %v", hub.perUser, hub.perMatch)
	}
	if _, err := a.Next(context.Background()); !errors.Is(err, ErrLiveClosed) {
		t.Errorf("closed stream: %v", err)
	}
	feed.emit(LiveEvent{MatchID: 8, CompanyID: 5, Seq: 3, Kind: LiveChanged})
	if got := next(t, b); got.Seq != 3 {
		t.Errorf("closing one stream does not affect the other: %+v", got)
	}
}

func TestHubCaps(t *testing.T) {
	hub, _ := startHub(t)
	var first LiveStream
	for i := range LiveStreamsPerUser {
		s, err := hub.subscribe(1, int64(i+1), []int64{1}, 0)
		if err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
		if i == 0 {
			first = s
		}
	}
	if _, err := hub.subscribe(1, 99, nil, 0); !errors.Is(err, ErrRefused) {
		t.Errorf("11th stream of a user: %v", err)
	}
	first.Close()
	if _, err := hub.subscribe(1, 99, nil, 0); err != nil {
		t.Errorf("a closed stream frees its slot: %v", err)
	}
	for i := range LiveStreamsPerMatch {
		if _, err := hub.subscribe(int64(100+i), 500, nil, 0); err != nil {
			t.Fatalf("match stream %d: %v", i, err)
		}
	}
	if _, err := hub.subscribe(9999, 500, nil, 0); !errors.Is(err, ErrRefused) {
		t.Errorf("51st stream of a match: %v", err)
	}
}

func TestHubResyncAndClose(t *testing.T) {
	hub, feed := startHub(t)
	s, _ := hub.subscribe(1, 7, []int64{1, 2}, 0)
	next(t, s)
	feed.resync()
	if b := next(t, s); !slices.Equal(b.Companies, []int64{1, 2}) || !b.Match {
		t.Errorf("resync marks everything: %+v", b)
	}
	hub.Close()
	if _, err := s.Next(context.Background()); !errors.Is(err, ErrLiveClosed) {
		t.Errorf("hub.Close ends the streams: %v", err)
	}
	if _, err := hub.subscribe(1, 7, nil, 0); !errors.Is(err, ErrRefused) {
		t.Errorf("no new streams after close: %v", err)
	}
}

func TestHubNextLeavesNoGoroutineBehind(t *testing.T) {
	hub, _ := startHub(t)
	s, _ := hub.subscribe(1, 7, []int64{1}, 0)
	next(t, s)
	before := runtime.NumGoroutine()
	for range 20 {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error)
		go func() { _, err := s.Next(ctx); done <- err }()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("%v", err)
		}
	}
	time.Sleep(20 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+1 {
		t.Errorf("goroutines %d -> %d", before, after)
	}
}
