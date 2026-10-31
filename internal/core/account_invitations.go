package core

import (
	"context"
	"errors"
	"net/mail"
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

// InvitationStateAt derives the state of an invitation at now. Precedence: revoked, used, expired, pending; an invitation that
// expires at exactly now is expired.
func InvitationStateAt(expiresAt time.Time, revokedAt *time.Time, uses, maxUses int, now time.Time) InvitationState {
	switch {
	case revokedAt != nil:
		return InvitationRevoked
	case uses >= maxUses:
		return InvitationUsed
	case !expiresAt.After(now):
		return InvitationExpired
	}
	return InvitationPending
}

// AccountInvitations creates, lists, revokes and redeems account invitations. Administrators and superusers manage them;
// the public side (Preview, Register) knows a single failure, ErrLinkUnusable.
type AccountInvitations struct {
	repo accountInvitationRepository
	opts serviceOptions
}

// NewAccountInvitations builds the service; WithClock and WithRandom control time and tokens.
func NewAccountInvitations(repo accountInvitationRepository, opts ...ServiceOption) *AccountInvitations {
	return &AccountInvitations{repo: repo, opts: newServiceOptions(opts)}
}

func mayAdminister(actor User, what string) error {
	if !actor.MayAdminister() {
		return Forbid("Only administrators can " + what)
	}
	return nil
}

// Create makes an invitation. Only administrators and superusers (ErrForbidden); ErrInvalidInput for a kind, address, expiry
// or maximum out of range; ErrRefused when a named invitation is for an address that already has an account. The raw token
// is in the result and nowhere else: only its hash is stored.
func (s *AccountInvitations) Create(ctx context.Context, actor User, in NewAccountInvitation) (CreatedAccountInvitation, error) {
	if err := mayAdminister(actor, "invite people"); err != nil {
		return CreatedAccountInvitation{}, err
	}
	now := s.opts.now()
	expiry := in.Expiry
	if expiry == 0 {
		expiry = InvitationDefaultExpiry
	}
	var problems []error
	if expiry < InvitationMinExpiry || expiry > InvitationMaxExpiry {
		problems = append(problems, errors.New("the invitation must expire between 1 hour and 30 days from now"))
	}
	stored := StoredAccountInvitation{Kind: in.Kind, CanCreateCampaigns: in.CanCreateCampaigns, CreatedBy: actor.ID, CreatedAt: now, ExpiresAt: now.Add(expiry)}
	switch in.Kind {
	case InvitationNamed:
		stored.MaxUses = 1
		email := normalizeEmail(in.Email)
		if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
			problems = append(problems, errors.New("the email address is not valid"))
		}
		stored.Email = email
	case InvitationAnonymous:
		stored.MaxUses = in.MaxUses
		if stored.MaxUses == 0 {
			stored.MaxUses = InvitationDefaultMaxUses
		}
		if stored.MaxUses < 1 || stored.MaxUses > InvitationMaxUsesLimit {
			problems = append(problems, errors.New("the maximum number of accounts must be between 1 and 50"))
		}
	default:
		problems = append(problems, errors.New("the invitation must be named or anonymous"))
	}
	if len(problems) > 0 {
		return CreatedAccountInvitation{}, errors.Join(append([]error{ErrInvalidInput}, problems...)...)
	}
	if stored.Kind == InvitationNamed {
		taken, err := s.repo.EmailHasAccount(ctx, stored.Email)
		if err != nil {
			return CreatedAccountInvitation{}, err
		}
		if taken {
			return CreatedAccountInvitation{}, Refuse("There is already an account with this address.")
		}
	}
	token, err := newToken(s.opts.random)
	if err != nil {
		return CreatedAccountInvitation{}, err
	}
	stored.TokenHash = hashToken(token)
	audit := InvitationCreatedAudit(now, actor.ID, stored.Kind, stored.ExpiresAt, stored.MaxUses, stored.CanCreateCampaigns)
	created, err := s.repo.Create(ctx, stored, audit)
	if err != nil {
		return CreatedAccountInvitation{}, err
	}
	return CreatedAccountInvitation{AccountInvitation: created, Token: token}, nil
}

// List returns the invitations newest first, 25 at a time by default (at most 200), with the total. Administrators and superusers.
func (s *AccountInvitations) List(ctx context.Context, actor User, f InvitationFilter, limit, offset int) ([]AccountInvitation, int, error) {
	if err := mayAdminister(actor, "list the invitations"); err != nil {
		return nil, 0, err
	}
	switch f.State {
	case "", InvitationPending, InvitationUsed, InvitationExpired, InvitationRevoked:
	default:
		return nil, 0, errors.Join(ErrInvalidInput, errors.New("unknown invitation state"))
	}
	if limit <= 0 {
		limit = defaultInvitationPage
	}
	limit = min(limit, maxInvitationPage)
	return s.repo.List(ctx, f, s.opts.now(), limit, max(offset, 0))
}

// Revoke revokes a pending invitation. ErrRefused with the state otherwise ("This invitation is already used."), ErrNotFound
// for an unknown id.
func (s *AccountInvitations) Revoke(ctx context.Context, actor User, id int64) (AccountInvitation, error) {
	if err := mayAdminister(actor, "revoke invitations"); err != nil {
		return AccountInvitation{}, err
	}
	now := s.opts.now()
	inv, err := s.repo.Revoke(ctx, id, actor.ID, now, InvitationRevokedAudit(now, actor.ID, id))
	var np *NotPendingError
	if errors.As(err, &np) {
		return AccountInvitation{}, Refuse("This invitation is already %s.", np.State)
	}
	return inv, err
}

// usable looks the token up without locking and says whether it is a pending invitation; everything else is ErrLinkUnusable
// (an internal error of the repository is returned as is).
func (s *AccountInvitations) usable(ctx context.Context, token string) (hash []byte, peek InvitationPeek, err error) {
	hash, err = parseToken(token)
	if err != nil {
		return nil, InvitationPeek{}, err
	}
	peek, err = s.repo.Peek(ctx, hash, s.opts.now())
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, InvitationPeek{}, ErrLinkUnusable
	case err != nil:
		return nil, InvitationPeek{}, err
	case !sameHash(peek.TokenHash, hash) || peek.State != InvitationPending:
		return nil, InvitationPeek{}, ErrLinkUnusable
	}
	return hash, peek, nil
}

// Preview says what a link offers, without changing anything. ErrLinkUnusable for every dead link.
func (s *AccountInvitations) Preview(ctx context.Context, token string) (InvitationPreview, error) {
	_, peek, err := s.usable(ctx, token)
	if err != nil {
		return InvitationPreview{}, err
	}
	return InvitationPreview{Kind: peek.Kind, Email: peek.Email}, nil
}

// Register creates the account and signs it in, all in one repository transaction. Order: token format, an unlocked look that
// rejects a dead link before any hashing, field validation (ErrInvalidUser with every problem), argon2, then the transaction;
// whatever fails there, an address that already has an account included, is ErrLinkUnusable and consumes nothing. The
// address of a named invitation is the stored one: in.Email is ignored.
func (s *AccountInvitations) Register(ctx context.Context, token string, in Registration) (RegisteredSession, error) {
	hash, peek, err := s.usable(ctx, token)
	if err != nil {
		return RegisteredSession{}, err
	}
	named := peek.Kind == InvitationNamed
	email, name, err := validateAccount(in.Email, in.DisplayName, in.Password, !named)
	if err != nil {
		return RegisteredSession{}, err
	}
	if named {
		email = peek.Email
	}
	pwHash, err := hashPassword(in.Password, s.opts.argon)
	if err != nil {
		return RegisteredSession{}, err
	}
	session, err := newToken(s.opts.random)
	if err != nil {
		return RegisteredSession{}, err
	}
	now := s.opts.now()
	expires := now.Add(sessionTTL)
	user, err := s.repo.Register(ctx, RegisterAccount{
		TokenHash: hash, Email: email, DisplayName: name, PasswordHash: pwHash,
		SessionHash: hashToken(session), SessionExpires: expires, Now: now,
		Audit: func(userID, invitationID int64) AuditEntry { return InvitationUsedAudit(now, userID, invitationID) },
	})
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrEmailTaken) {
		return RegisteredSession{}, ErrLinkUnusable
	}
	if err != nil {
		return RegisteredSession{}, err
	}
	return RegisteredSession{Token: session, Expires: expires, User: user}, nil
}
