package web

import "testing"

// TestLinkThrottleCapsOneLinkWhateverTheAddress: rotating addresses does not give a link more attempts than maxFailsPerLink, and
// one address cannot lock the others out of a link it did not fail on.
func TestLinkThrottleCapsOneLinkWhateverTheAddress(t *testing.T) {
	th := newLinkThrottle()
	for i := range maxFailsPerLink - 1 {
		th.fail("10.0.0."+itoa(i), "linkA")
	}
	if th.blocked("10.9.9.9", "linkA") {
		t.Fatal("blocked before the cap")
	}
	th.fail("10.8.8.8", "linkA")
	if !th.blocked("10.9.9.9", "linkA") {
		t.Error("a fresh address must be blocked once the link hit its cap")
	}
	if th.blocked("10.9.9.9", "linkB") {
		t.Error("another link must not be affected")
	}
	if l := newLoginThrottle(); func() bool {
		for i := range maxFailsPerLink + 1 {
			l.fail("10.1.1."+itoa(i), "a@example.com")
		}
		return l.blocked("10.2.2.2", "a@example.com")
	}() {
		t.Error("the login throttle has no per-link cap")
	}
}
