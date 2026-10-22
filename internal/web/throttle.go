package web

import (
	"strings"
	"sync"
	"time"
)

// loginThrottle limits failed logins held in memory: per address+email (guessing one account) and per address
// (trying many accounts). It is per process, which is enough for a single instance.
type loginThrottle struct {
	mu     sync.Mutex
	window time.Duration
	fails  map[string][]time.Time
	now    func() time.Time
}

const (
	maxFailsPerAccount = 5
	maxFailsPerAddress = 20
)

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{window: 10 * time.Minute, fails: map[string][]time.Time{}, now: time.Now}
}

func keys(ip, email string) (account, address string) {
	return ip + "|" + strings.ToLower(strings.TrimSpace(email)), ip + "|*"
}

// prune drops the attempts outside the window; callers hold the lock.
func (t *loginThrottle) prune(key string) []time.Time {
	cutoff := t.now().Add(-t.window)
	kept := t.fails[key][:0]
	for _, at := range t.fails[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(t.fails, key)
		return nil
	}
	t.fails[key] = kept
	return kept
}

func (t *loginThrottle) blocked(ip, email string) bool {
	account, address := keys(ip, email)
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.prune(account)) >= maxFailsPerAccount || len(t.prune(address)) >= maxFailsPerAddress
}

func (t *loginThrottle) fail(ip, email string) {
	account, address := keys(ip, email)
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	t.fails[account] = append(t.fails[account], now)
	t.fails[address] = append(t.fails[address], now)
}

func (t *loginThrottle) reset(ip, email string) {
	account, _ := keys(ip, email)
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, account)
}
