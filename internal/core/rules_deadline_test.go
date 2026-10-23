package core

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func slowRules(t *testing.T, timeout time.Duration, fn func(context.Context, RulesetHashes) (*RuleSet, error)) *Rules {
	t.Helper()
	r := NewRules(NewMockrulesetRepository(gomock.NewController(t)), nil).WithBuildTimeout(timeout)
	r.buildFn = fn
	return r
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A parser that loops (it ignores its context) ends the build with a deadline error after the timeout, the failure is not cached, and the
// goroutine of the parser is counted until it returns and then gone.
func TestRulesBuildDeadlineGivesUpOnAParserThatLoops(t *testing.T) {
	before := runtime.NumGoroutine()
	release := make(chan struct{})
	r := slowRules(t, 40*time.Millisecond, func(context.Context, RulesetHashes) (*RuleSet, error) {
		<-release // the parser loop
		return &RuleSet{}, nil
	})
	start := time.Now()
	_, err := r.For(context.Background(), triple)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want one wrapping DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %s", d)
	}
	waitFor(t, "the failed build to leave the cache", func() bool { return r.Cached() == 0 })
	if r.Stuck() != 1 {
		t.Errorf("Stuck = %d, want 1", r.Stuck())
	}
	close(release)
	waitFor(t, "the parser goroutine to end", func() bool { return r.Stuck() == 0 && runtime.NumGoroutine() <= before })
}

// The next request for the same triple builds again (the failure was not cached) and can succeed.
func TestRulesBuildDeadlineIsNotCached(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	defer close(release)
	r := slowRules(t, 30*time.Millisecond, func(context.Context, RulesetHashes) (*RuleSet, error) {
		if calls.Add(1) == 1 {
			<-release
		}
		return &RuleSet{Hashes: triple}, nil
	})
	if _, err := r.For(context.Background(), triple); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first: %v", err)
	}
	waitFor(t, "the cache to forget the failure", func() bool { return r.Cached() == 0 })
	if rs, err := r.For(context.Background(), triple); err != nil || rs == nil {
		t.Errorf("second: %v", err)
	}
}

// A build that honours its context (the database reads, the checks between the parsing stages) ends at the deadline and leaves nothing stuck.
func TestRulesBuildDeadlineCooperates(t *testing.T) {
	r := slowRules(t, 30*time.Millisecond, func(ctx context.Context, _ RulesetHashes) (*RuleSet, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if _, err := r.For(context.Background(), triple); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	waitFor(t, "no stuck build", func() bool { return r.Stuck() == 0 && r.Cached() == 0 })
}

// Only a few builds may be stuck at once: the next one is refused without starting another goroutine.
func TestRulesBuildStuckBuildsAreBounded(t *testing.T) {
	release := make(chan struct{})
	var started atomic.Int32
	r := slowRules(t, 10*time.Millisecond, func(context.Context, RulesetHashes) (*RuleSet, error) {
		started.Add(1)
		<-release
		return nil, errors.New("late")
	})
	for i := 0; i < maxStuckBuilds; i++ {
		h := RulesetHashes{Vanilla: string(rune('a' + i)), BattleCompanies: "b", Glossary: "g"}
		if _, err := r.For(context.Background(), h); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("build %d: %v", i, err)
		}
	}
	waitFor(t, "failed builds to leave the cache", func() bool { return r.Cached() == 0 })
	if _, err := r.For(context.Background(), triple); !errors.Is(err, ErrBuildsStuck) {
		t.Errorf("err = %v, want ErrBuildsStuck", err)
	}
	if n := started.Load(); int(n) != maxStuckBuilds {
		t.Errorf("%d parsers started, want %d", n, maxStuckBuilds)
	}
	close(release)
	waitFor(t, "the stuck builds to end", func() bool { return r.Stuck() == 0 })
	waitFor(t, "cache empty", func() bool { return r.Cached() == 0 })
}

func TestRulesBuildTimeoutSetting(t *testing.T) {
	r := NewRules(nil, nil)
	if r.timeout != DefaultBuildTimeout {
		t.Errorf("default = %s", r.timeout)
	}
	if r.WithBuildTimeout(0).timeout != DefaultBuildTimeout || r.WithBuildTimeout(-time.Second).timeout != DefaultBuildTimeout {
		t.Error("a non-positive timeout must be ignored")
	}
	if r.WithBuildTimeout(5*time.Second).timeout != 5*time.Second {
		t.Error("timeout not set")
	}
}
