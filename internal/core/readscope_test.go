package core

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingCompanies struct {
	companyRepository
	reads int
	err   error
}

func (c *countingCompanies) Read(_ context.Context, campaignID, companyID int64) (CompanyState, error) {
	c.reads++
	if c.err != nil {
		return CompanyState{}, c.err
	}
	return CompanyState{Company: CampaignCompany{ID: companyID, Name: "x"}}, nil
}

func TestReadScopeReadsOnceInAScopeAndAlwaysWithout(t *testing.T) {
	inner := &countingCompanies{}
	r := ScopeCompanies(inner)
	plain := context.Background()
	for range 3 {
		if _, err := r.Read(plain, 1, 2); err != nil {
			t.Fatal(err)
		}
	}
	if inner.reads != 3 {
		t.Fatalf("without a scope every read goes through: %d", inner.reads)
	}
	inner.reads = 0
	ctx := WithReadScope(plain)
	if WithReadScope(ctx) != ctx {
		t.Fatal("a second scope must not replace the first")
	}
	for range 3 {
		st, err := r.Read(ctx, 1, 2)
		if err != nil || st.Company.ID != 2 {
			t.Fatal(st, err)
		}
	}
	if _, err := r.Read(ctx, 1, 3); err != nil { // another company is another read
		t.Fatal(err)
	}
	if inner.reads != 2 {
		t.Fatalf("one read per company in a scope: %d", inner.reads)
	}
	// a new scope sees fresh data
	if _, err := r.Read(WithReadScope(plain), 1, 2); err != nil || inner.reads != 3 {
		t.Fatal(err, inner.reads)
	}
}

func TestReadScopeDoesNotRememberErrors(t *testing.T) {
	inner := &countingCompanies{err: ErrNotFound}
	r := ScopeCompanies(inner)
	ctx := WithReadScope(context.Background())
	for range 2 {
		if _, err := r.Read(ctx, 1, 2); !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
	}
	if inner.reads != 2 {
		t.Fatalf("a failed read is tried again: %d", inner.reads)
	}
}

type slowCompanies struct {
	companyRepository
	reads   atomic.Int32
	release chan struct{}
}

func (c *slowCompanies) ReadRoster(_ context.Context, campaignID int64) ([]CompanyState, error) {
	c.reads.Add(1)
	<-c.release
	return []CompanyState{{Company: CampaignCompany{ID: campaignID}}}, nil
}

// Callers that overlap in time (the cards of the match page are built side by side) share one load.
func TestReadScopeSharesALoadInFlight(t *testing.T) {
	inner := &slowCompanies{release: make(chan struct{})}
	r := ScopeCompanies(inner)
	ctx := WithReadScope(context.Background())
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if st, err := r.ReadRoster(ctx, 7); err != nil || len(st) != 1 || st[0].Company.ID != 7 {
				t.Error(st, err)
			}
		}()
	}
	for inner.reads.Load() == 0 {
		runtime.Gosched()
	}
	time.Sleep(20 * time.Millisecond) // the others have reached the entry and wait for it
	close(inner.release)
	wg.Wait()
	if _, err := r.ReadRoster(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if n := inner.reads.Load(); n != 1 {
		t.Fatalf("one roster read for the scope: %d", n)
	}
}

type countingMatches struct {
	matchRepository
	events int
}

func (c *countingMatches) Events(context.Context, int64, int64) ([]MatchEvent, error) {
	c.events++
	return []MatchEvent{{}}, nil
}

func TestReadScopeRemembersMatchEvents(t *testing.T) {
	inner := &countingMatches{}
	r := ScopeMatches(inner)
	ctx := WithReadScope(context.Background())
	for range 3 {
		if _, err := r.Events(ctx, 1, 2); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Events(ctx, 1, 3); err != nil || inner.events != 2 {
		t.Fatal(err, inner.events)
	}
}
