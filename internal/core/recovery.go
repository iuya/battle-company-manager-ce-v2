package core

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"
)

//go:generate go tool mockgen -typed -source=recovery.go -destination=mock_recovery_test.go -package=core

// RecoveryState is derived. At most one of used, revoked and invalidated is set; expired means none of them is and the expiry passed.
type RecoveryState string

const (
	RecoveryPending     RecoveryState = "pending"
	RecoveryUsed        RecoveryState = "used"
	RecoveryRevoked     RecoveryState = "revoked"
	RecoveryInvalidated RecoveryState = "invalidated"
	RecoveryExpired     RecoveryState = "expired"
)

// RecoveryReason is why a link was invalidated by the app.
type RecoveryReason string

const (
	RecoveryByLogin   RecoveryReason = "login"    // the person signed in
	RecoveryByAccess  RecoveryReason = "access"   // the person opened the app with a session
	RecoveryReplaced  RecoveryReason = "replaced" // a newer link was generated
	RecoveryByClosing RecoveryReason = "closed"   // the account was closed
)

// Limits of a recovery link.
const (
	RecoveryDefaultTTL = 24 * time.Hour
	RecoveryMinTTL     = time.Hour
	RecoveryMaxTTL     = 7 * 24 * time.Hour
)

// StoredRecovery is what the repository stores: the SHA-256 of the token, never the token.
type StoredRecovery struct {
	UserID    int64
	TokenHash []byte
	CreatedBy int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// RecoveryLink is a freshly generated link; Token is the secret, set by the service after the repository stored the hash.
type RecoveryLink struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
}

// RecoveryStatus is the latest link of an account, for the Users page (superusers only). The repository fills the raw fields; the
// service derives State from its clock.
type RecoveryStatus struct {
	ID            int64
	State         RecoveryState
	CreatedAt     time.Time
	ExpiresAt     time.Time
	CreatedByName string
	UsedAt        *time.Time
	RevokedAt     *time.Time
	RevokedByName string
	InvalidatedAt *time.Time
	Reason        RecoveryReason // when invalidated
}

// StateAt derives the state at now.
func (r RecoveryStatus) StateAt(now time.Time) RecoveryState {
	switch {
	case r.UsedAt != nil:
		return RecoveryUsed
	case r.RevokedAt != nil:
		return RecoveryRevoked
	case r.InvalidatedAt != nil:
		return RecoveryInvalidated
	case !r.ExpiresAt.After(now):
		return RecoveryExpired
	}
	return RecoveryPending
}

// RecoveryPeek is an unlocked look at a link by token hash.
type RecoveryPeek struct {
	TokenHash []byte // as stored; compared in constant time with the computed hash
	UserID    int64
	State     RecoveryState
}

// recoveryRepository keeps the password recovery links (table password_recovery_links). Writes carry their audit entry and
// make it in the same transaction.
type recoveryRepository interface {
	// Create, in one transaction: lock the user's row, mark the user's live (and expired but unmarked) links invalidated
	// 'replaced' (one recovery.invalidated entry each), insert the link, set audit.TargetID and audit.WithReplaced. ErrNotFound
	// for an unknown user, ErrAccountClosed for a closed one; ErrConflict when a concurrent Create got there first.
	Create(ctx context.Context, in StoredRecovery, audit AuditEntry) (RecoveryLink, error)
	// Revoke revokes the user's live link (audit.TargetID is set to its id); ErrNotFound when there is none.
	Revoke(ctx context.Context, userID, by int64, now time.Time, audit AuditEntry) error
	// Peek looks a link up by token hash without locking; ErrNotFound for an unknown one.
	Peek(ctx context.Context, tokenHash []byte, now time.Time) (RecoveryPeek, error)
	// Use, in one transaction: lock the user's row and the link's, re-check it is pending at now, set used_at, store the
	// password hash, delete every session of the user, write the audit entry. ErrNotFound: no usable link.
	Use(ctx context.Context, tokenHash []byte, passwordHash string, now time.Time, audit func(userID, linkID int64) AuditEntry) (userID int64, err error)
}

// sessionControl is the session cache as the recovery service needs it. *Auth implements it (it owns the cache).
type sessionControl interface {
	// forgetUser drops every cached session of the user.
	forgetUser(userID int64)
	// setRecoveryPending sets the recovery flag on every cached session of the user.
	setRecoveryPending(userID int64, pending bool)
}

// Recovery generates, revokes and redeems password recovery links. Only superusers generate and revoke; the public side
// (Preview, SetPassword) knows a single failure, ErrLinkUnusable. A link is invalidated by the person signing in or opening the
// app (see Auth.Login and Auth.Authenticate), and using it signs nobody in.
type Recovery struct {
	repo     recoveryRepository
	sessions sessionControl
	opts     serviceOptions
}

// NewRecovery builds the service; sessions is the *Auth that owns the session cache.
func NewRecovery(repo recoveryRepository, sessions sessionControl, opts ...ServiceOption) *Recovery {
	return &Recovery{repo: repo, sessions: sessions, opts: newServiceOptions(opts)}
}

// Generate makes a link for the user, replacing any live one. ttl 0 is 24 hours; 1 hour to 7 days otherwise (ErrInvalidInput).
// ErrForbidden unless the actor is a superuser; ErrRefused for the actor's own account (use the Account page) and for a closed
// one; ErrNotFound for an unknown user. The raw token is in the result and nowhere else.
func (s *Recovery) Generate(ctx context.Context, actor User, userID int64, ttl time.Duration) (RecoveryLink, error) {
	if !actor.IsSuperuser() {
		return RecoveryLink{}, Forbid("Only a superuser can generate recovery links")
	}
	if ttl == 0 {
		ttl = RecoveryDefaultTTL
	}
	if ttl < RecoveryMinTTL || ttl > RecoveryMaxTTL {
		return RecoveryLink{}, errors.Join(ErrInvalidInput, errors.New("the link must expire between 1 hour and 7 days from now"))
	}
	if userID == actor.ID {
		return RecoveryLink{}, Refuse("Use the Account page to change your own password.")
	}
	token, err := newToken(s.opts.random)
	if err != nil {
		return RecoveryLink{}, err
	}
	now := s.opts.now()
	in := StoredRecovery{UserID: userID, TokenHash: hashToken(token), CreatedBy: actor.ID, CreatedAt: now, ExpiresAt: now.Add(ttl)}
	link, err := s.repo.Create(ctx, in, RecoveryCreatedAudit(now, actor.ID, userID, in.ExpiresAt))
	switch {
	case errors.Is(err, ErrAccountClosed):
		return RecoveryLink{}, Refuse("This account is closed")
	case err != nil:
		return RecoveryLink{}, err
	}
	// A person who is browsing right now has a hot cache entry without the flag: set it, so their next request invalidates the link.
	s.sessions.setRecoveryPending(userID, true)
	link.Token = token
	return link, nil
}

// Revoke revokes the user's live link. ErrForbidden unless the actor is a superuser; ErrRefused when there is none.
func (s *Recovery) Revoke(ctx context.Context, actor User, userID int64) error {
	if !actor.IsSuperuser() {
		return Forbid("Only a superuser can revoke recovery links")
	}
	now := s.opts.now()
	err := s.repo.Revoke(ctx, userID, actor.ID, now, RecoveryRevokedAudit(now, actor.ID, userID))
	if errors.Is(err, ErrNotFound) {
		return Refuse("There is no pending link for this account.")
	}
	if err != nil {
		return err
	}
	s.sessions.setRecoveryPending(userID, false)
	return nil
}

func (s *Recovery) usable(ctx context.Context, token string) (hash []byte, err error) {
	hash, err = parseToken(token)
	if err != nil {
		return nil, err
	}
	peek, err := s.repo.Peek(ctx, hash, s.opts.now())
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, ErrLinkUnusable
	case err != nil:
		return nil, err
	case !sameHash(peek.TokenHash, hash) || peek.State != RecoveryPending:
		return nil, ErrLinkUnusable
	}
	return hash, nil
}

// Preview says whether the link can be used, without changing anything. ErrLinkUnusable for every dead link. It never touches a
// session: opening the link must not count as an access of the app.
func (s *Recovery) Preview(ctx context.Context, token string) error {
	_, err := s.usable(ctx, token)
	return err
}

// SetPassword sets the new password through the link, ends every session of the account and uses the link up, in one
// repository transaction. ErrLinkUnusable for every dead link (checked first, without hashing); ErrInvalidUser for a password
// of the wrong length. Nobody is signed in by it.
func (s *Recovery) SetPassword(ctx context.Context, token, password string) error {
	hash, err := s.usable(ctx, token)
	if err != nil {
		return err
	}
	if n := utf8.RuneCountInString(password); n < minPasswordLen || n > maxPasswordLen {
		return errors.Join(ErrInvalidUser, errors.New("the password must be between 10 and 128 characters"))
	}
	pw, err := hashPassword(password, s.opts.argon)
	if err != nil {
		return err
	}
	now := s.opts.now()
	userID, err := s.repo.Use(ctx, hash, pw, now, func(userID, linkID int64) AuditEntry { return RecoveryUsedAudit(now, userID, linkID) })
	if errors.Is(err, ErrNotFound) {
		return ErrLinkUnusable
	}
	if err != nil {
		return err
	}
	s.sessions.forgetUser(userID)
	s.sessions.setRecoveryPending(userID, false)
	return nil
}
