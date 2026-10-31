package core

import (
	"context"
	"time"
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
	// 'replaced' (one recovery.invalidated entry each), insert the link and set audit.TargetID. ErrNotFound for an unknown or
	// closed user; ErrConflict when a concurrent Create got there first.
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
