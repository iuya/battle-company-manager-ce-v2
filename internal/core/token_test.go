package core

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestParseToken(t *testing.T) {
	good := strings.Repeat("a", 43)
	h, err := parseToken(good)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256([]byte(good)); !bytes.Equal(h, sum[:]) {
		t.Fatal("the hash is the SHA-256 of the token")
	}
	for name, tok := range map[string]string{
		"empty": "", "short": strings.Repeat("a", 42), "long": strings.Repeat("a", 44), "plus": strings.Repeat("a", 42) + "+",
		"slash": strings.Repeat("a", 42) + "/", "padding": strings.Repeat("a", 42) + "=", "newline": strings.Repeat("a", 42) + "\n",
		"space": strings.Repeat("a", 42) + " ", "unicode": strings.Repeat("a", 42) + "é", "nul": strings.Repeat("a", 42) + "\x00",
		"prefix newline": "\n" + strings.Repeat("a", 43),
	} {
		if _, err := parseToken(tok); err != ErrLinkUnusable {
			t.Errorf("%s: got %v, want ErrLinkUnusable", name, err)
		}
	}
	if _, err := parseToken("A-_9" + strings.Repeat("z", 39)); err != nil {
		t.Errorf("url-safe characters: %v", err)
	}
}

func TestNewToken(t *testing.T) {
	a, err := newToken(bytes.NewReader(bytes.Repeat([]byte{1}, 32)))
	if err != nil || len(a) != 43 {
		t.Fatalf("%q %v", a, err)
	}
	if again, _ := newToken(bytes.NewReader(bytes.Repeat([]byte{1}, 32))); again != a {
		t.Fatal("the injected reader makes the token deterministic")
	}
	if _, err := parseToken(a); err != nil {
		t.Fatal("a generated token must parse")
	}
	if _, err := newToken(bytes.NewReader([]byte{1, 2})); err == nil {
		t.Fatal("a short read is an error")
	}
	x, _ := newToken(rand.Reader)
	y, _ := newToken(rand.Reader)
	if x == y {
		t.Fatal("two tokens differ")
	}
}

func TestSameHash(t *testing.T) {
	a := hashToken("a")
	if !sameHash(a, hashToken("a")) || sameHash(a, hashToken("b")) || sameHash(nil, a) || sameHash(a[:16], a) {
		t.Fatal("sameHash")
	}
}

func TestErrors(t *testing.T) {
	if ErrLinkUnusable.Error() != TalkToAdministrator || TalkToAdministrator != "Talk with the administrator." {
		t.Fatal("the generic public error text")
	}
	err := Forbid("because")
	if !errors.Is(err, ErrForbidden) || !strings.Contains(err.Error(), "because") {
		t.Fatalf("%v", err)
	}
	if np := error(&NotPendingError{State: InvitationUsed}); !errors.Is(np, ErrNotPending) {
		t.Fatal("NotPendingError is ErrNotPending")
	}
}

// WithRandom replaces crypto/rand as the source of tokens.
func WithRandom(r io.Reader) ServiceOption { return func(o *serviceOptions) { o.random = r } }

// withArgon lowers the password hashing cost in tests.
func withArgon(p argonParams) ServiceOption { return func(o *serviceOptions) { o.argon = p } }
