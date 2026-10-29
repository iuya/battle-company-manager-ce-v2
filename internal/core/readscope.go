package core

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
)

// A read scope remembers the answers of a few repository reads for the length of one request, so a page that several services build
// on (the aftermath page asks for the same match, company and Against the Odds record from five places) reads each once. It is
// opt-in: only a context made by WithReadScope has one, and a context without it reads straight through. Open a scope only around
// code that reads: a write inside it would not invalidate anything, so the callers open a new scope after a write.
// The results are shared between the callers of one scope: they must not change the slices and maps in them (the services copy
// before they change, as the rating of a match does).
type readScope struct {
	mu sync.Mutex
	m  map[any]*scopeEntry
}

// scopeEntry is one remembered read; done closes when the load has finished, so a second caller that arrives while the first is still
// reading (the cards of the match page are built side by side) waits for that answer instead of asking the database again.
type scopeEntry struct {
	done chan struct{}
	val  any
	err  error
}

type readScopeKey struct{}

// WithReadScope returns a context with a read scope (the same context when it already has one).
func WithReadScope(ctx context.Context) context.Context {
	if ctx.Value(readScopeKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, readScopeKey{}, &readScope{m: map[any]*scopeEntry{}})
}

// scoped answers from the context's scope, or loads and remembers (only successful reads: a failed one is forgotten, so the next caller
// tries again). Callers of the same key that overlap in time share one load. Without a scope it just loads.
func scoped[T any](ctx context.Context, key any, load func() (T, error)) (T, error) {
	sc, _ := ctx.Value(readScopeKey{}).(*readScope)
	if sc == nil {
		return load()
	}
	sc.mu.Lock()
	e, ok := sc.m[key]
	if !ok {
		e = &scopeEntry{done: make(chan struct{})}
		sc.m[key] = e
	}
	sc.mu.Unlock()
	if ok {
		select {
		case <-e.done:
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
		if e.err == nil {
			return e.val.(T), nil
		}
		return load() // the load this one waited for failed: try on its own
	}
	out, err := load()
	e.val, e.err = out, err
	if err != nil {
		sc.mu.Lock()
		delete(sc.m, key)
		sc.mu.Unlock()
	}
	close(e.done)
	return out, err
}

type (
	matchGetKey    struct{ campaign, match int64 }
	companyReadKey struct{ campaign, company int64 }
	atoGetKey      struct{ campaign, match int64 }
	liveReadKey    struct{ campaign, match int64 }
	campaignGetKey struct{ user, campaign int64 }
	campaignCoKey  struct{ campaign, company int64 }
	hashesKey      struct{ campaign int64 }
	rosterKey      struct{ campaign int64 }
	rosterOfKey    string // the company ids, comma separated
	eventsKey      struct{ campaign, match int64 }
)

// ScopedMatches, ScopedCompanies, ScopedATO, ScopedLive and ScopedCampaigns wrap a repository so the reads of one request that
// carries a read scope are made once; everything else passes through.
type ScopedMatches struct{ matchRepository }

func ScopeMatches(r matchRepository) ScopedMatches { return ScopedMatches{r} }

func (r ScopedMatches) Get(ctx context.Context, campaignID, matchID int64) (Match, error) {
	return scoped(ctx, matchGetKey{campaignID, matchID}, func() (Match, error) { return r.matchRepository.Get(ctx, campaignID, matchID) })
}

// Events is read once per request too (every complete() of the same match asks for it).
func (r ScopedMatches) Events(ctx context.Context, campaignID, matchID int64) ([]MatchEvent, error) {
	return scoped(ctx, eventsKey{campaignID, matchID}, func() ([]MatchEvent, error) { return r.matchRepository.Events(ctx, campaignID, matchID) })
}

type ScopedCompanies struct{ companyRepository }

func ScopeCompanies(r companyRepository) ScopedCompanies { return ScopedCompanies{r} }

func (r ScopedCompanies) Read(ctx context.Context, campaignID, companyID int64) (CompanyState, error) {
	return scoped(ctx, companyReadKey{campaignID, companyID}, func() (CompanyState, error) { return r.companyRepository.Read(ctx, campaignID, companyID) })
}

// ReadRoster is read once per request as well: the figures of the campaign's companies are asked for by every card of a page.
func (r ScopedCompanies) ReadRoster(ctx context.Context, campaignID int64) ([]CompanyState, error) {
	return scoped(ctx, rosterKey{campaignID}, func() ([]CompanyState, error) { return r.companyRepository.ReadRoster(ctx, campaignID) })
}

// errRosterOfUnsupported is what ScopedCompanies.ReadRosterOf answers over a repository without it: the callers read the whole campaign instead.
var errRosterOfUnsupported = errors.New("the repository cannot read rosters by company")

// ReadRosterOf is remembered per request by the ids asked for (see rosterOfReader). It exists only when the wrapped repository has it.
func (r ScopedCompanies) ReadRosterOf(ctx context.Context, companyIDs []int64) ([]CompanyState, error) {
	inner, ok := r.companyRepository.(rosterOfReader)
	if !ok {
		return nil, errRosterOfUnsupported
	}
	var key strings.Builder
	for _, id := range companyIDs {
		key.WriteString(strconv.FormatInt(id, 10))
		key.WriteByte(',')
	}
	return scoped(ctx, rosterOfKey(key.String()), func() ([]CompanyState, error) { return inner.ReadRosterOf(ctx, companyIDs) })
}

type ScopedATO struct{ atoRepository }

func ScopeATO(r atoRepository) ScopedATO { return ScopedATO{r} }

func (r ScopedATO) Get(ctx context.Context, campaignID, matchID int64) (ATORecord, error) {
	return scoped(ctx, atoGetKey{campaignID, matchID}, func() (ATORecord, error) { return r.atoRepository.Get(ctx, campaignID, matchID) })
}

type ScopedLive struct{ liveRepository }

func ScopeLive(r liveRepository) ScopedLive { return ScopedLive{r} }

func (r ScopedLive) Read(ctx context.Context, campaignID, matchID int64) (LiveSnapshot, error) {
	return scoped(ctx, liveReadKey{campaignID, matchID}, func() (LiveSnapshot, error) { return r.liveRepository.Read(ctx, campaignID, matchID) })
}

type ScopedCampaigns struct{ campaignRepository }

func ScopeCampaigns(r campaignRepository) ScopedCampaigns { return ScopedCampaigns{r} }

func (r ScopedCampaigns) Get(ctx context.Context, userID, id int64) (CampaignDetail, error) {
	return scoped(ctx, campaignGetKey{userID, id}, func() (CampaignDetail, error) { return r.campaignRepository.Get(ctx, userID, id) })
}

func (r ScopedCampaigns) Company(ctx context.Context, id, companyID int64) (CampaignCompany, error) {
	return scoped(ctx, campaignCoKey{id, companyID}, func() (CampaignCompany, error) { return r.campaignRepository.Company(ctx, id, companyID) })
}

// Nav keeps the one-statement sidebar read of the wrapped repository (Campaigns.NavCampaigns finds it by type).
func (r ScopedCampaigns) Nav(ctx context.Context, userID int64, withFinished bool) ([]CampaignNav, error) {
	if nl, ok := r.campaignRepository.(campaignNavLister); ok {
		return nl.Nav(ctx, userID, withFinished)
	}
	list, err := r.campaignRepository.List(ctx, userID, withFinished)
	if err != nil {
		return nil, err
	}
	out := make([]CampaignNav, len(list))
	for i, g := range list {
		out[i] = CampaignNav{ID: g.ID, Name: g.Name, Status: g.Status}
	}
	return out, nil
}
