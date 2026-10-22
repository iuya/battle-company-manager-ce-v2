package core

import (
	"container/list"
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

// RuleSet is the rules a campaign is played with, built from its three stored files. It is immutable once built and shared
// by every campaign with the same hashes. Resolution order for a name is entity > Battle Companies > vanilla.
type RuleSet struct {
	Hashes   RulesetHashes
	Vanilla  *VanillaBook
	BC       *BCBook
	Glossary Glossary
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
}

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
	return &Rules{repo: repo, log: log, calls: map[RulesetHashes]*ruleSetCall{}, lru: list.New(), max: MaxCachedRuleSets}
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
		// The build outlives the request that triggered it: the others wait for it too.
		go func() {
			c.set, c.err = r.build(context.WithoutCancel(ctx), h)
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
	reviewed, err := ParseVanillaAmbiguities(bc.Content)
	if err != nil {
		return nil, err
	}
	vb, err := ParseVanillaWith(vanilla.Content, reviewed, r.log.With("vanilla", vanilla.ShortHash()))
	if err != nil {
		return nil, err
	}
	bb, err := ParseBattleCompanies(bc.Content)
	if err != nil {
		return nil, err
	}
	bb.SpellWargearLikeVanilla(vb)
	bb.ApplyUnitFillIns(vb)
	gl, err := BuildGlossaryWith(glossary.RulesetInfo, glossary.Content, GlossaryExtras(vanilla.Content, bc.Content))
	if err != nil {
		return nil, err
	}
	return &RuleSet{Hashes: h, Vanilla: vb, BC: bb, Glossary: gl}, nil
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
