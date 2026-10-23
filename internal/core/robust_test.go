package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

// panickyFeed panics on its first run and behaves like fakeFeed afterwards.
type panickyFeed struct {
	*fakeFeed
	runs atomic.Int32
}

func (f *panickyFeed) Run(ctx context.Context, onEvent func(LiveEvent), onResync func()) error {
	if f.runs.Add(1) == 1 {
		panic("feed exploded")
	}
	return f.fakeFeed.Run(ctx, onEvent, onResync)
}

func TestHubSurvivesAPanicInTheFeedAndRestartsIt(t *testing.T) {
	old := liveRestartDelay
	liveRestartDelay = time.Millisecond
	t.Cleanup(func() { liveRestartDelay = old })
	feed := &panickyFeed{fakeFeed: newFakeFeed()}
	hub := NewLiveHub(feed)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- hub.Start(ctx) }()
	select {
	case <-feed.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the feed was not restarted after the panic")
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Start = %v", err)
	}
}

func TestRulesForTurnsAPanicInTheBuildIntoAnError(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	repo.EXPECT().Get(gomock.Any(), hV).DoAndReturn(func(context.Context, string) (StoredRuleset, error) { panic("parser bug") })
	rules := NewRules(repo, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := rules.For(ctx, triple); err == nil {
		t.Fatal("want an error")
	}
	if n := rules.Cached(); n != 0 {
		t.Errorf("a failed build stays cached: %d", n)
	}
}
