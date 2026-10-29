package core

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// RuleSet is the rules a campaign is played with, built from its three stored files. It is immutable once built and shared
// by every campaign with the same hashes. Resolution order for a name is entity > Battle Companies > vanilla.
type RuleSet struct {
	Hashes   RulesetHashes
	Vanilla  *VanillaBook
	BC       *BCBook
	Glossary Glossary

	// Doc is the typed lean document of a schemaVersion 2 file; nil for the old format. The consumers read it
	// instead of decoding the bytes again.
	Doc *lean.Document

	// equipment remembers equipmentFor per item (see there); the set never changes, so nothing in it can go stale.
	equipment  sync.Map
	equipmentN atomic.Int64
}

// Keyword returns the definition of a special rule: the one embedded in the entity that uses it (a unit's own rule, a hero
// upgrade; pass "" when there is none) wins over Battle Companies, which wins over the vanilla file.
func (r *RuleSet) Keyword(embedded, name string) (string, bool) {
	if embedded != "" {
		return embedded, true
	}
	if d, ok := r.BC.DefinedKeyword(name); ok {
		return d, true
	}
	if d, ok := r.Vanilla.Keyword(name); ok {
		return d, true
	}
	return r.BC.ParameterisedKeyword(name) // Hatred (Spirit): the rule with its parameter
}

// Rules is the loader: it turns a campaign's hashes into a RuleSet. Sets are cached by the hash triple, built once however
// many requests ask at the same time, and capped: the least recently used triples beyond MaxCachedRuleSets are dropped (a set
// in use stays valid for whoever holds it; the next request for a dropped triple builds it again). The database is only read
// when a triple is first needed.
type Rules struct {
	repo rulesetRepository
	log  *slog.Logger

	mu    sync.Mutex
	calls map[RulesetHashes]*ruleSetCall
	lru   *list.List // of RulesetHashes, most recently used first
	max   int

	timeout time.Duration // deadline of one build
	stuck   int           // builds given up on that are still running (their parser has not returned)
	buildFn func(context.Context, RulesetHashes) (*RuleSet, error)
}

// DefaultBuildTimeout is how long building a rule set from its three files may take before it is given up on; RULES_BUILD_TIMEOUT sets
// it (WithBuildTimeout). Parsing the shipped files takes well under a second.
const DefaultBuildTimeout = 60 * time.Second

// maxStuckBuilds bounds the goroutines of builds that were given up on but whose parser has not returned (parsers take no context, so a
// loop on a hostile file cannot be stopped from outside). Beyond it new builds are refused at once instead of piling up more.
const maxStuckBuilds = 4

// ErrBuildsStuck says too many earlier builds are still stuck in a parser.
var ErrBuildsStuck = errors.New("too many rule set builds are stuck")

// MaxCachedRuleSets is the default of how many rule sets (hash triples) stay in memory; MAX_CACHED_RULESETS sets it (WithMaxCached). A parsed set is a few MB; a handful of triples exist in
// practice (the shipped files plus the odd upload a campaign pins), so the cap only bites when a superuser uploads many files.
const MaxCachedRuleSets = 16

type ruleSetCall struct {
	done chan struct{}
	set  *RuleSet
	err  error
	elem *list.Element
}

func NewRules(repo rulesetRepository, log *slog.Logger) *Rules {
	if log == nil {
		log = slog.Default()
	}
	r := &Rules{repo: repo, log: log, calls: map[RulesetHashes]*ruleSetCall{}, lru: list.New(), max: MaxCachedRuleSets, timeout: DefaultBuildTimeout}
	r.buildFn = r.build
	return r
}

// WithBuildTimeout sets the deadline of one build (at least 1 ms; smaller values are ignored). Call it before the first For.
func (r *Rules) WithBuildTimeout(d time.Duration) *Rules {
	if d >= time.Millisecond {
		r.mu.Lock()
		r.timeout = d
		r.mu.Unlock()
	}
	return r
}

// WithMaxCached sets the cap of the cache (at least 1; smaller values are ignored). Call it before the first For.
func (r *Rules) WithMaxCached(n int) *Rules {
	if n >= 1 {
		r.mu.Lock()
		r.max = n
		r.mu.Unlock()
	}
	return r
}

// For returns the rules of the three files, the same *RuleSet for equal triples. A failed build is not cached.
func (r *Rules) For(ctx context.Context, h RulesetHashes) (*RuleSet, error) {
	r.mu.Lock()
	c, ok := r.calls[h]
	if ok {
		r.lru.MoveToFront(c.elem)
	} else {
		c = &ruleSetCall{done: make(chan struct{})}
		c.elem = r.lru.PushFront(h)
		r.calls[h] = c
		r.evict()
		// The build outlives the request that triggered it: the others wait for it too. It has its own deadline.
		go func() {
			// A panic in the parsers (a malformed upload) must not end the process, and must still release the waiters.
			c.err = safely("rule set build", func() (err error) {
				c.set, err = r.buildBounded(context.WithoutCancel(ctx), h)
				return err
			})
			if c.err != nil {
				r.mu.Lock()
				if r.calls[h] == c {
					delete(r.calls, h)
					r.lru.Remove(c.elem)
				}
				r.mu.Unlock()
			}
			close(c.done)
		}()
	}
	r.mu.Unlock()
	select {
	case <-c.done:
		return c.set, c.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// evict drops the least recently used finished sets beyond the cap; builds in progress are never dropped. Callers hold the lock.
func (r *Rules) evict() {
	for e := r.lru.Back(); e != nil && r.lru.Len() > r.max; {
		prev := e.Prev()
		h := e.Value.(RulesetHashes)
		select {
		case <-r.calls[h].done:
			delete(r.calls, h)
			r.lru.Remove(e)
		default:
		}
		e = prev
	}
}

// Cached is how many rule sets are in memory (finished or being built).
func (r *Rules) Cached() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// buildBounded runs the build under the deadline. The database reads and the steps of build stop with the context; a parser that
// loops cannot be interrupted, so after the deadline the build is given up on (an error that wraps context.DeadlineExceeded, not
// cached) and its goroutine is left to end by itself: at most maxStuckBuilds of them are tolerated at a time.
func (r *Rules) buildBounded(ctx context.Context, h RulesetHashes) (*RuleSet, error) {
	r.mu.Lock()
	timeout, stuck := r.timeout, r.stuck
	r.mu.Unlock()
	if stuck >= maxStuckBuilds {
		return nil, ErrBuildsStuck
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		set *RuleSet
		err error
	}
	out := make(chan result, 1) // buffered: a build given up on must be able to finish without a reader
	var finished, abandoned bool
	go func() {
		var res result
		res.err = safely("rule set build", func() (err error) {
			res.set, err = r.buildFn(ctx, h)
			return err
		})
		r.mu.Lock()
		finished = true
		if abandoned {
			r.stuck--
		}
		r.mu.Unlock()
		out <- res
	}()
	select {
	case res := <-out:
		return res.set, res.err
	case <-ctx.Done():
	}
	r.mu.Lock()
	if finished { // it ended just as the deadline did: take its answer
		r.mu.Unlock()
		res := <-out
		return res.set, res.err
	}
	abandoned = true
	r.stuck++
	r.mu.Unlock()
	r.log.Error("rule set build given up on", "timeout", timeout, "vanilla", h.Vanilla, "battleCompanies", h.BattleCompanies, "glossary", h.Glossary)
	return nil, fmt.Errorf("building the rule set took longer than %s: %w", timeout, context.DeadlineExceeded)
}

// Stuck is how many given-up builds are still running in a parser.
func (r *Rules) Stuck() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stuck
}

func (r *Rules) build(ctx context.Context, h RulesetHashes) (*RuleSet, error) {
	get := func(kind RulesetKind, hash string) (StoredRuleset, error) {
		s, err := r.repo.Get(ctx, hash)
		if err != nil {
			return StoredRuleset{}, fmt.Errorf("%s file %.8s: %w", kind, hash, err)
		}
		if s.Kind != kind {
			return StoredRuleset{}, fmt.Errorf("%s file %.8s is stored as %s: %w", kind, hash, s.Kind, ErrInvalidRuleset)
		}
		return s, nil
	}
	vanilla, err := get(KindVanilla, h.Vanilla)
	if err != nil {
		return nil, err
	}
	bc, err := get(KindBattleCompanies, h.BattleCompanies)
	if err != nil {
		return nil, err
	}
	glossary, err := get(KindGlossary, h.Glossary)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rs, err := loadV2(vanilla.Content, bc.Content, glossary.Content, r.log.With("vanilla", vanilla.ShortHash()))
	if rs != nil {
		rs.Hashes = h
	}
	return rs, err
}

// HeroicActionName is the glossary's spelling of a heroic action ("Heroic Channeling" is "Heroic Channelling", "Heroic
// Defense" is "Heroic Defence"), found through the term names and aliases; a name the glossary does not know stays as written.
func (r *RuleSet) HeroicActionName(name string) string {
	if t, ok := r.Glossary.Find(name); ok && t.Category == "heroic actions" {
		return t.Name
	}
	return name
}

// HeroicActionNames lists every heroic action a hero may learn, once each whatever the spelling the vanilla profiles use.
func (r *RuleSet) HeroicActionNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range r.Vanilla.HeroicActionNames() {
		c := r.HeroicActionName(a)
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}
