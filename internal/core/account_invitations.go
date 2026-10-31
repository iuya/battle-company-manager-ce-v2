package core

import (
	"context"
	"time"
)

//go:generate go tool mockgen -typed -source=account_invitations.go -destination=mock_account_invitations_test.go -package=core

// InvitationKind is named (one person, one address, one use) or anonymous (anyone with the link, up to MaxUses accounts).
type InvitationKind string

const (
	InvitationNamed     InvitationKind = "named"
	InvitationAnonymous InvitationKind = "anonymous"
)

// InvitationState is derived, never stored. Precedence when several apply: revoked, used, expired, pending.
type InvitationState string

const (
	InvitationPending InvitationState = "pending"
	InvitationUsed    InvitationState = "used"
	InvitationExpired InvitationState = "expired"
	InvitationRevoked InvitationState = "revoked"
)

// Limits of an account invitation (judgement calls the owner accepted).
const (
	InvitationDefaultExpiry  = 7 * 24 * time.Hour
	InvitationMinExpiry      = time.Hour
	InvitationMaxExpiry      = 30 * 24 * time.Hour
	InvitationDefaultMaxUses = 10
	InvitationMaxUsesLimit   = 50
	defaultInvitationPage    = 25
	maxInvitationPage        = 200
)

// StoredAccountInvitation is what the repository stores: the SHA-256 of the token, never the token. Email is empty for an
// anonymous invitation.
type StoredAccountInvitation struct {
	TokenHash          []byte
	Kind               InvitationKind
	Email              string
	CanCreateCampaigns bool
	MaxUses            int
	CreatedBy          int64
	CreatedAt          time.Time
	ExpiresAt          time.Time
}

// InvitationAccount is an account made through an invitation.
type InvitationAccount struct {
	UserID      int64
	DisplayName string
	UsedAt      time.Time
}

// AccountInvitation is one invitation as the Invitations page lists it (no token, no hash).
type AccountInvitation struct {
	ID                 int64
	Kind               InvitationKind
	Email              string // empty for anonymous
	CanCreateCampaigns bool
	MaxUses            int
	Uses               int
	CreatedBy          int64
	CreatedByName      string
	CreatedAt          time.Time
	ExpiresAt          time.Time
	RevokedAt          *time.Time
	RevokedBy          *int64
	RevokedByName      string
	State              InvitationState // computed from the `now` the repository was given
	Accounts           []InvitationAccount
}

// InvitationFilter narrows the list; the zero value is every invitation.
type InvitationFilter struct {
	State InvitationState
}

// InvitationPeek is an unlocked look at an invitation by token hash, to reject a dead link before paying for argon2.
type InvitationPeek struct {
	TokenHash []byte // as stored; the service compares it in constant time with the hash it computed
	Kind      InvitationKind
	Email     string // the stored address of a named invitation
	State     InvitationState
}

// RegisterAccount is the single transaction of a registration: lock the invitation row (FOR UPDATE), re-check it is
// pending at Now, create the account (role user, the invitation's flag), a session, the use row, increment uses, and the
// audit entry, all or nothing.
type RegisterAccount struct {
	TokenHash []byte
	// Email is used for an anonymous invitation only; a named invitation uses the stored address.
	Email          string
	DisplayName    string
	PasswordHash   string
	SessionHash    []byte
	SessionExpires time.Time
	Now            time.Time
	// Audit builds the invitation.used entry once the ids are known.
	Audit func(userID, invitationID int64) AuditEntry
}

// accountInvitationRepository keeps the account invitations (tables account_invitations and account_invitation_uses). Every
// write that has an audit entry writes it in the same transaction.
type accountInvitationRepository interface {
	// Create stores the invitation and sets audit.TargetID to the new id; the result has the ID filled.
	Create(ctx context.Context, in StoredAccountInvitation, audit AuditEntry) (AccountInvitation, error)
	// List returns the invitations newest first (the accounts of each included) with State computed from now, and the total.
	List(ctx context.Context, filter InvitationFilter, now time.Time, limit, offset int) ([]AccountInvitation, int, error)
	// Revoke revokes a pending invitation: ErrNotFound for an unknown id, a *NotPendingError (ErrNotPending) when it is used,
	// expired or revoked.
	Revoke(ctx context.Context, id, by int64, now time.Time, audit AuditEntry) (AccountInvitation, error)
	// Peek looks an invitation up by token hash without locking; ErrNotFound for an unknown one.
	Peek(ctx context.Context, tokenHash []byte, now time.Time) (InvitationPeek, error)
	// Register is the transaction of RegisterAccount. ErrNotFound: no usable invitation (unknown, revoked, used, expired);
	// ErrEmailTaken: the address has an account (nothing is consumed).
	Register(ctx context.Context, in RegisterAccount) (User, error)
	// EmailHasAccount reports whether an open account has that (normalised) address.
	EmailHasAccount(ctx context.Context, email string) (bool, error)
}

// NewAccountInvitation is the administrator's form.
type NewAccountInvitation struct {
	Kind               InvitationKind
	Email              string        // named only
	CanCreateCampaigns bool          // the flag the new account gets
	Expiry             time.Duration // 0: the default of 7 days; 1 hour to 30 days
	MaxUses            int           // anonymous only: 1 to 50, 0: 10; a named invitation is always 1
}

// CreatedAccountInvitation is the one place the raw token exists.
type CreatedAccountInvitation struct {
	AccountInvitation
	Token string
}

// InvitationPreview is what opening an invitation link shows.
type InvitationPreview struct {
	Kind  InvitationKind
	Email string // the stored address of a named invitation
}

// Registration is the visitor's form. Email is read for an anonymous invitation only.
type Registration struct {
	Email, DisplayName, Password string
}

// RegisteredSession is what the handler turns into the session cookie (the same as Login returns).
type RegisteredSession struct {
	Token   string
	Expires time.Time
	User    User
}
