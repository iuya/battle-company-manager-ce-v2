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
	// perAccount and perAddress are how many failures in the window block one key or one whole address.
	perAccount, perAddress int
}

const (
	maxFailsPerAccount = 5
	maxFailsPerAddress = 20
	maxThrottleKeys    = 50_000 // memory bound of the table
)

func newLoginThrottle() *loginThrottle { return newThrottle(maxFailsPerAccount, maxFailsPerAddress) }

// newThrottle makes a throttle with its own limits (the registration and recovery pages use the login ones, on their own tables).
func newThrottle(perAccount, perAddress int) *loginThrottle {
	return &loginThrottle{window: 10 * time.Minute, fails: map[string][]time.Time{}, now: time.Now, perAccount: perAccount, perAddress: perAddress}
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
	return len(t.prune(account)) >= t.perAccount || len(t.prune(address)) >= t.perAddress
}

func (t *loginThrottle) fail(ip, email string) {
	account, address := keys(ip, email)
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if len(t.fails) >= maxThrottleKeys { // keys of addresses and emails nobody tries again would stay for ever: sweep the expired ones
		for k := range t.fails {
			t.prune(k)
		}
		if len(t.fails) >= maxThrottleKeys {
			clear(t.fails) // a flood from many addresses: forget rather than grow (the limits start again)
		}
	}
	t.fails[account] = append(t.fails[account], now)
	t.fails[address] = append(t.fails[address], now)
}

func (t *loginThrottle) reset(ip, email string) {
	account, _ := keys(ip, email)
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.fails, account)
}
