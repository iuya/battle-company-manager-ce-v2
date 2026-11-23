package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// However many logins are in flight, at most cap(argonSlots) hashes run at once (each holds 19 MiB); the rest wait and all finish.
func TestArgonSlotsBoundConcurrentHashes(t *testing.T) {
	oldSlots, oldHook := argonSlots, argonInSlot
	t.Cleanup(func() { argonSlots, argonInSlot = oldSlots, oldHook })
	const slots, callers = 3, 40
	argonSlots = make(chan struct{}, slots)
	var inflight, peak atomic.Int32
	argonInSlot = func() {
		n := inflight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond) // long enough for the others to arrive and have to wait
		inflight.Add(-1)
	}
	hash, err := hashPassword("a long enough password", fastArgon)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var wrong atomic.Int32
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pw := "a long enough password"
			if i%2 == 1 {
				pw = "another long password"
			}
			if ok, err := verifyPassword(pw, hash); err != nil || ok != (i%2 == 0) {
				wrong.Add(1)
			}
		}()
	}
	wg.Wait()
	if wrong.Load() != 0 {
		t.Errorf("%d answers were wrong: a waiting hash must give the same answer as any other", wrong.Load())
	}
	if got := peak.Load(); got > slots || got < 2 {
		t.Errorf("peak concurrent hashes = %d, want 2..%d (the bound, and proof the test overlapped them)", got, slots)
	}
}

func TestArgonSlotsDefault(t *testing.T) {
	if c := cap(argonSlots); c < 2 || c > 8 {
		t.Errorf("slots = %d, want 2..8", c)
	}
}
