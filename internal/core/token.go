package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"regexp"
	"time"
)

// TalkToAdministrator is the text of the only failure a public link page ever shows.
const TalkToAdministrator = "Talk with the administrator."

var (
	// ErrLinkUnusable is the only public failure of an invitation or recovery link: malformed, unknown, revoked, used, expired or
	// invalidated tokens, and an address that already has an account, all look the same. Its text is TalkToAdministrator.
	ErrLinkUnusable = errors.New(TalkToAdministrator)
	// ErrForbidden means the actor may not do this at all (the permission matrix, a non-administrator calling an administrator
	// service); it is always joined with a reason fit to show. The web answers 403, or 404 when the whole route is not for them.
	ErrForbidden = errors.New("forbidden")
	// ErrInvalidInput carries the reasons an administrator's form values were rejected (expiry out of range, maximum uses, address).
	ErrInvalidInput = errors.New("invalid input")
	// ErrConflict means a concurrent change got there first (e.g. another recovery link was just made for the account).
	ErrConflict = errors.New("conflict")
	// ErrNotPending is returned by a repository that cannot revoke an invitation that is not pending; see NotPendingError.
	ErrNotPending = errors.New("not pending")
)

// NotPendingError is the typed ErrNotPending: it carries the state the invitation was found in, for the message.
type NotPendingError struct{ State InvitationState }

func (e *NotPendingError) Error() string        { return "the invitation is " + string(e.State) }
func (e *NotPendingError) Is(target error) bool { return target == ErrNotPending }

// Forbid builds an ErrForbidden with a reason.
func Forbid(reason string) error { return errors.Join(ErrForbidden, errors.New(reason)) }

// ServiceOption configures a service (the clock and the source of randomness), so tests and the demo world control time and tokens.
type ServiceOption func(*serviceOptions)

type serviceOptions struct {
	now    func() time.Time
	random io.Reader
	argon  argonParams
}

func newServiceOptions(opts []ServiceOption) serviceOptions {
	o := serviceOptions{now: time.Now, random: rand.Reader, argon: defaultArgon}
	for _, f := range opts {
		f(&o)
	}
	return o
}

// WithClock replaces time.Now.
func WithClock(now func() time.Time) ServiceOption { return func(o *serviceOptions) { o.now = now } }

// WithRandom replaces crypto/rand as the source of tokens.
func WithRandom(r io.Reader) ServiceOption { return func(o *serviceOptions) { o.random = r } }

const tokenBytes = 32

var tokenFormat = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// newToken reads 32 bytes from r and returns them base64url-encoded (43 characters).
func newToken(r io.Reader) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(r, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// parseToken returns the SHA-256 of a well-formed token without any lookup; anything else is ErrLinkUnusable.
func parseToken(s string) ([]byte, error) {
	if !tokenFormat.MatchString(s) {
		return nil, ErrLinkUnusable
	}
	return hashToken(s), nil
}

// sameHash compares a stored hash with the computed one in constant time (defence in depth: the database already compared hashes).
func sameHash(stored, computed []byte) bool { return subtle.ConstantTimeCompare(stored, computed) == 1 }

func defaultRandom() io.Reader { return rand.Reader }

// withArgon lowers the password hashing cost in tests.
func withArgon(p argonParams) ServiceOption { return func(o *serviceOptions) { o.argon = p } }
