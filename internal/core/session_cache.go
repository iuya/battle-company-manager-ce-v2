package core

import (
	"sync"
	"time"
)

const (
	// sessionCacheTTL bounds how stale an authenticated session can be: a session revoked behind our back (e.g. deleted in
	// the database) keeps working for at most this long. Logging out through Auth clears the entry immediately.
	sessionCacheTTL = time.Minute
	// sessionCacheMax keeps memory bounded; when full, expired entries are dropped first, then everything.
	sessionCacheMax = 10_000
)

type cachedSession struct {
	user    User
	expires time.Time // when the session itself expires
	checked time.Time // when it was last confirmed against the database
	// recoveryPending says the user has a live password recovery link that the next authenticated request must invalidate.
	// UserBySession fills it on a miss; Recovery.Generate sets it on the cached entries; Authenticate clears it.
	recoveryPending bool
}

// sessionCache lets the many requests of a page (htmx partials included) skip the database: a session confirmed less
// than sessionCacheTTL ago is trusted. It lives in memory, per process; losing it only costs a few lookups.
type sessionCache struct {
	mu sync.Mutex
	m  map[string]cachedSession
}

func newSessionCache() *sessionCache { return &sessionCache{m: map[string]cachedSession{}} }

func (c *sessionCache) get(key string, now time.Time) (cachedSession, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return cachedSession{}, false
	}
	if now.Sub(e.checked) >= sessionCacheTTL || !now.Before(e.expires) {
		delete(c.m, key)
		return cachedSession{}, false
	}
	return e, true
}

func (c *sessionCache) put(key string, e cachedSession, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= sessionCacheMax {
		for k, old := range c.m {
			if now.Sub(old.checked) >= sessionCacheTTL {
				delete(c.m, k)
			}
		}
		if len(c.m) >= sessionCacheMax {
			clear(c.m)
		}
	}
	c.m[key] = e
}

func (c *sessionCache) delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, key)
}

// updateUser applies change to the cached user of every cached session of userID.
func (c *sessionCache) updateUser(userID int64, change func(*User)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		if e.user.ID == userID {
			change(&e.user)
			c.m[k] = e
		}
	}
}

// dropUser forgets every cached session of userID except the one under key keep ("" keeps none).
func (c *sessionCache) dropUser(userID int64, keep string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		if e.user.ID == userID && (keep == "" || k != keep) {
			delete(c.m, k)
		}
	}
}

// setRecoveryPending sets the recovery flag of every cached session of userID.
func (c *sessionCache) setRecoveryPending(userID int64, pending bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		if e.user.ID == userID {
			e.recoveryPending = pending
			c.m[k] = e
		}
	}
}
