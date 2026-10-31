package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrNotFound is returned by repositories when a row does not exist.
	ErrNotFound = errors.New("not found")
	// ErrEmailTaken is returned when creating a user whose email is already registered.
	ErrEmailTaken = errors.New("email already registered")
	// ErrInvalidCredentials is the only login failure users see: it never says whether the email exists.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrNoSession means the session token is unknown or expired.
	ErrNoSession = errors.New("no valid session")
	// ErrInvalidUser carries the reason a new user was rejected.
	ErrInvalidUser = errors.New("invalid user")
)

const (
	minPasswordLen = 10
	maxPasswordLen = 128 // argon2 is deliberately expensive: do not let a request choose an unbounded input
	sessionTTL     = 30 * 24 * time.Hour
	// staleTokenAge is how long an unused invitation or a terminal recovery link is kept after it ended.
	staleTokenAge = 90 * 24 * time.Hour
)

// Role is a user's global role. Per-campaign roles (organiser, player) are a separate matter.
type Role string

const (
	RoleUser      Role = "user"
	RoleAdmin     Role = "admin"
	RoleSuperuser Role = "superuser" // also sees the administration section
)

func (r Role) Valid() bool { return r == RoleUser || r == RoleAdmin || r == RoleSuperuser }

type User struct {
	ID          int64
	Email       string
	DisplayName string
	Role        Role
	// CanCreateCampaigns lets a plain user create campaigns; admins and superusers always can.
	CanCreateCampaigns bool
	// Appearance is the palette and mode saved for the account; empty means never chosen.
	Appearance Appearance
	// Timezone is the IANA name of the zone times are shown in; empty means automatic (the browser's zone, see UserLocation).
	Timezone string
}

// Appearance is a user's saved look. Both empty means the user never chose (the browser's cookie decides).
type Appearance struct {
	Palette string
	Mode    string
}

// Palettes and Modes are the keys an Appearance may hold (the settings menu offers exactly these).
var (
	Palettes = []string{"forest", "ember", "mithril"}
	Modes    = []string{"system", "light", "dark"}
)

// Valid reports whether both keys are known.
func (a Appearance) Valid() bool {
	return slices.Contains(Palettes, a.Palette) && slices.Contains(Modes, a.Mode)
}

func (u User) IsSuperuser() bool { return u.Role == RoleSuperuser }

// MayAdminister says the user sees the Administration section: administrators and superusers.
func (u User) MayAdminister() bool { return u.Role == RoleAdmin || u.Role == RoleSuperuser }

func (u User) MayCreateCampaigns() bool {
	return u.Role == RoleAdmin || u.Role == RoleSuperuser || u.CanCreateCampaigns
}

type Credentials struct {
	User
	PasswordHash string
}

// authRepository is what the core needs from the persistence layer. Implemented by internal/postgres.
//
//go:generate go tool mockgen -typed -source=auth.go -destination=mock_auth_repository_test.go -package=core
type authRepository interface {
	// CreateUser returns ErrEmailTaken when the (case-insensitive) email exists.
	CreateUser(ctx context.Context, email, displayName, passwordHash string) (User, error)
	// SetUserPermissionsByID changes the role and the flag and writes the audit entry in the same transaction; ErrNotFound when
	// there is no such open user.
	SetUserPermissionsByID(ctx context.Context, userID int64, role Role, canCreateCampaigns bool, audit AuditEntry) error
	// UserByID returns the user, closed accounts included (closed is true for them), or ErrNotFound.
	UserByID(ctx context.Context, userID int64) (user User, closed bool, err error)
	// ListUsers returns the accounts whose name or email contains q (case-insensitive, empty: all): open ones by display
	// name, then closed ones. limit and offset page the result; total is the number of matches.
	ListUsers(ctx context.Context, q string, limit, offset int) (rows []AdminUser, total int, err error)
	// UserByEmail returns ErrNotFound when there is no such user.
	UserByEmail(ctx context.Context, email string) (Credentials, error)
	CreateSession(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error
	// UserBySession returns ErrNotFound when the session does not exist or expired before now. recoveryPending says the user has
	// a live (pending, unexpired at now) recovery link; it is part of the same query, so it costs no extra round trip.
	UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (user User, expires time.Time, recoveryPending bool, err error)
	// InvalidateRecovery marks the user's live recovery link (if any) invalidated for reason (RecoveryByLogin or RecoveryByAccess)
	// and, when n > 0, writes a recovery.invalidated entry (source system) in the same transaction. Normally n is 0.
	InvalidateRecovery(ctx context.Context, userID int64, reason RecoveryReason, now time.Time) (n int, err error)
	// PurgeStaleTokens deletes the invitations that were never used and ended (revoked or expired) before the cutoff and the
	// terminal recovery links older than it. Invitations that made an account and audit_events are kept.
	PurgeStaleTokens(ctx context.Context, before time.Time) error
	ExtendSession(ctx context.Context, tokenHash []byte, expiresAt time.Time) error
	DeleteSession(ctx context.Context, tokenHash []byte) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error

	// CredentialsByID returns ErrNotFound for an unknown or closed user.
	CredentialsByID(ctx context.Context, userID int64) (Credentials, error)
	// UpdateDisplayName returns the updated user, or ErrNotFound.
	UpdateDisplayName(ctx context.Context, userID int64, displayName string) (User, error)
	UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string) error
	// DeleteSessionsExcept deletes the user's sessions other than keep (nil keeps none) and returns how many.
	DeleteSessionsExcept(ctx context.Context, userID int64, keep []byte) (int, error)
	// CloseUser anonymises the user (name, email, empty hash, closed_at) and deletes all their sessions.
	CloseUser(ctx context.Context, userID int64, displayName, email string, now time.Time) error
	// OrganisesUnfinishedCampaign reports whether the user organises a campaign that is not finished.
	OrganisesUnfinishedCampaign(ctx context.Context, userID int64) (bool, error)
	// OtherSuperusers counts the open superuser accounts other than userID.
	OtherSuperusers(ctx context.Context, userID int64) (int, error)
	// SetAppearance returns ErrNotFound for an unknown user.
	SetAppearance(ctx context.Context, userID int64, palette, mode string) error
	// SetTimezone stores the IANA zone name ("" clears it: automatic); ErrNotFound for an unknown user.
	SetTimezone(ctx context.Context, userID int64, zone string) error
}

// Auth handles accounts and sessions. A session is a random token handed to the browser; only its SHA-256 is stored,
// so a leaked database cannot be used to impersonate anybody.
type Auth struct {
	repo  authRepository
	now   func() time.Time
	argon argonParams
	cache *sessionCache
	// dummyHash is verified when the email is unknown so that both outcomes take the same time.
	dummyHash string
}

func NewAuth(repo authRepository) *Auth {
	return newAuth(repo, time.Now, defaultArgon)
}

func newAuth(repo authRepository, now func() time.Time, p argonParams) *Auth {
	dummy, _ := hashPassword("dummy-password-for-timing", p)
	return &Auth{repo: repo, now: now, argon: p, cache: newSessionCache(), dummyHash: dummy}
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// validateAccount checks the fields of a new account and returns the normalised email and name. Every problem is reported
// (ErrInvalidUser joined with one message each). checkEmail is false when the address comes from an invitation.
func validateAccount(email, displayName, password string, checkEmail bool) (string, string, error) {
	var problems []error
	if checkEmail {
		email = normalizeEmail(email)
		if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
			problems = append(problems, errors.New("the email address is not valid"))
		}
	}
	displayName = strings.TrimSpace(displayName)
	if n := utf8.RuneCountInString(displayName); n == 0 || n > 60 {
		problems = append(problems, errors.New("the name must be between 1 and 60 characters"))
	}
	if n := utf8.RuneCountInString(password); n < minPasswordLen || n > maxPasswordLen {
		problems = append(problems, errors.New("the password must be between 10 and 128 characters"))
	}
	if len(problems) > 0 {
		return "", "", errors.Join(append([]error{ErrInvalidUser}, problems...)...)
	}
	return email, displayName, nil
}

// CreateUser validates and registers a user (the console: accounts are otherwise made through invitations).
func (a *Auth) CreateUser(ctx context.Context, email, displayName, password string) (User, error) {
	email, displayName, err := validateAccount(email, displayName, password, true)
	if err != nil {
		return User{}, err
	}
	hash, err := hashPassword(password, a.argon)
	if err != nil {
		return User{}, err
	}
	return a.repo.CreateUser(ctx, email, displayName, hash)
}

// SetPermissions changes a user's global role and campaign-creation permission (used by the CLI, make grant): the console may
// make anybody anything, including a superuser, subject to the state rules of SetUserPermissions.
func (a *Auth) SetPermissions(ctx context.Context, email string, role Role, canCreateCampaigns bool) error {
	if !role.Valid() {
		return errors.Join(ErrInvalidUser, errors.New("the role must be user, admin or superuser"))
	}
	rec, err := a.repo.UserByEmail(ctx, normalizeEmail(email))
	if err != nil {
		return err
	}
	return a.setPermissions(ctx, ActorConsole, nil, rec.ID, role, canCreateCampaigns)
}

// SetUserPermissions changes the role and the creation flag of the user with that id as actor, following RoleChangeAllowed:
// ErrForbidden (with a reason fit to show) when the matrix refuses, which includes every request for the superuser role
// (superusers are set from the console only); ErrRefused for a closed account and for demoting the last open superuser;
// ErrInvalidUser for an unknown role; ErrNotFound for an unknown id. A change that changes nothing succeeds without a write.
// Promotion to administrator keeps the stored flag. The user's cached sessions are dropped so the change applies at once, and
// the change is audited in the same transaction.
func (a *Auth) SetUserPermissions(ctx context.Context, actor User, userID int64, role Role, canCreateCampaigns bool) error {
	if !role.Valid() {
		return errors.Join(ErrInvalidUser, errors.New("the role must be user or admin"))
	}
	if !actor.MayAdminister() {
		return RoleChangeAllowed(ActorOf(actor), LevelPlain, LevelPlain)
	}
	if role == RoleSuperuser {
		return RoleChangeAllowed(ActorOf(actor), LevelPlain, LevelSuperuser)
	}
	id := actor.ID
	return a.setPermissions(ctx, ActorOf(actor), &id, userID, role, canCreateCampaigns)
}

func (a *Auth) setPermissions(ctx context.Context, actor Actor, actorID *int64, userID int64, role Role, canCreateCampaigns bool) error {
	target, closed, err := a.repo.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	from, to := LevelOf(target.Role, target.CanCreateCampaigns), LevelOf(role, canCreateCampaigns)
	if err := RoleChangeAllowed(actor, from, to); err != nil {
		return err
	}
	if closed {
		return Refuse("This account is closed")
	}
	if from == to {
		return nil // nothing changes, nothing is recorded
	}
	if to == LevelAdministrator || to == LevelSuperuser {
		canCreateCampaigns = target.CanCreateCampaigns // a later demotion restores what the account had
	}
	if target.Role == RoleSuperuser && role != RoleSuperuser {
		others, err := a.repo.OtherSuperusers(ctx, userID)
		if err != nil {
			return err
		}
		if others == 0 {
			return Refuse("This is the last superuser; make someone else a superuser first")
		}
	}
	source := AuditWeb
	if actor == ActorConsole {
		source = AuditConsole
	}
	audit := RoleChangedAudit(a.now(), source, actorID, userID, target.Role, role, target.CanCreateCampaigns, canCreateCampaigns)
	if err := a.repo.SetUserPermissionsByID(ctx, userID, role, canCreateCampaigns, audit); err != nil {
		return err
	}
	a.cache.dropUser(userID, "")
	return nil
}

// AdminUser is one row of the administration Users page.
type AdminUser struct {
	User
	Campaigns int // campaigns the user is a member of, in any status
	CreatedAt time.Time
	Closed    bool // the account is closed; the other fields are the anonymised ones
	// Recovery is the account's latest recovery link, filled for superusers only (nil: never had one, or the caller may not see it).
	Recovery *RecoveryStatus
	// Organises are the campaigns the user is the organiser of (any status), for the Users page: an administrator hands them over
	// from there (Campaigns.HandOver), also when not a member.
	Organises []OrganisedCampaign
}

// OrganisedCampaign is a campaign an account organises.
type OrganisedCampaign struct {
	ID      int64
	Name    string
	Status  CampaignStatus
	Players int
}

const (
	defaultUserPage = 25
	maxUserPage     = 200
)

// ListUsers returns the accounts matching q (case-insensitive substring of name or email, empty: all), open ones first by
// display name, then closed ones. limit (default 25, at most 200) and offset page it; total is the number of matches.
// ErrForbidden unless actor is an administrator or a superuser; the recovery status of each account is for superusers only.
func (a *Auth) ListUsers(ctx context.Context, actor User, q string, limit, offset int) (rows []AdminUser, total int, err error) {
	if !actor.MayAdminister() {
		return nil, 0, Forbid("Only administrators can list the accounts")
	}
	if limit <= 0 {
		limit = defaultUserPage
	}
	limit = min(limit, maxUserPage)
	offset = max(offset, 0)
	rows, total, err = a.repo.ListUsers(ctx, strings.TrimSpace(q), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	now := a.now()
	for i := range rows {
		switch {
		case !actor.IsSuperuser():
			rows[i].Recovery = nil
		case rows[i].Recovery != nil:
			rows[i].Recovery.State = rows[i].Recovery.StateAt(now)
		}
	}
	return rows, total, nil
}

// Login checks the credentials and opens a session. It returns the token to store in the browser cookie and when
// the session expires.
func (a *Auth) Login(ctx context.Context, email, password string) (token string, expires time.Time, user User, err error) {
	if utf8.RuneCountInString(password) > maxPasswordLen {
		return "", time.Time{}, User{}, ErrInvalidCredentials
	}
	rec, err := a.repo.UserByEmail(ctx, normalizeEmail(email))
	switch {
	case errors.Is(err, ErrNotFound):
		_, _ = verifyPassword(password, a.dummyHash)
		return "", time.Time{}, User{}, ErrInvalidCredentials
	case err != nil:
		return "", time.Time{}, User{}, err
	}
	if rec.PasswordHash == "" { // a closed account
		_, _ = verifyPassword(password, a.dummyHash)
		return "", time.Time{}, User{}, ErrInvalidCredentials
	}
	if ok, verr := verifyPassword(password, rec.PasswordHash); verr != nil || !ok {
		return "", time.Time{}, User{}, ErrInvalidCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, User{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	now := a.now()
	expires = now.Add(sessionTTL)
	// Signing in with the password makes a pending recovery link pointless: it stops working before the session exists, and
	// whichever of this and the link's own use locks the row first wins.
	if _, err := a.repo.InvalidateRecovery(ctx, rec.ID, RecoveryByLogin, now); err != nil {
		return "", time.Time{}, User{}, err
	}
	a.cache.setRecoveryPending(rec.ID, false)
	if err := a.repo.CreateSession(ctx, hashToken(token), rec.ID, expires); err != nil {
		return "", time.Time{}, User{}, err
	}
	_ = a.repo.DeleteExpiredSessions(ctx, now)                // housekeeping; a failure must not fail the login
	_ = a.repo.PurgeStaleTokens(ctx, now.Add(-staleTokenAge)) // same
	return token, expires, rec.User, nil
}

// Authenticate resolves a session token to its user. Every request of a page goes through here, so a session confirmed
// in the last minute is answered from memory (see sessionCache). A session in the second half of its life is renewed,
// so people who keep using the app stay signed in (renewed is the new expiry, zero when unchanged).
func (a *Auth) Authenticate(ctx context.Context, token string) (user User, renewed time.Time, err error) {
	if token == "" {
		return User{}, time.Time{}, ErrNoSession
	}
	h := hashToken(token)
	key := string(h)
	now := a.now()

	entry, ok := a.cache.get(key, now)
	if !ok {
		user, expires, pending, err := a.repo.UserBySession(ctx, h, now)
		if errors.Is(err, ErrNotFound) {
			return User{}, time.Time{}, ErrNoSession
		}
		if err != nil {
			return User{}, time.Time{}, err
		}
		entry = cachedSession{user: user, expires: expires, checked: now, recoveryPending: pending}
	}
	if entry.recoveryPending {
		// Any authenticated request makes a pending recovery link pointless. The flag rides on the cache entry, so the normal
		// case (no link) costs nothing, and a pending link costs one UPDATE, once. A failure is an error (fail closed).
		if _, err := a.repo.InvalidateRecovery(ctx, entry.user.ID, RecoveryByAccess, now); err != nil {
			return User{}, time.Time{}, err
		}
		a.cache.setRecoveryPending(entry.user.ID, false)
		entry.recoveryPending = false
	}
	if entry.expires.Sub(now) < sessionTTL/2 {
		renewed = now.Add(sessionTTL)
		if err := a.repo.ExtendSession(ctx, h, renewed); err != nil {
			return User{}, time.Time{}, err
		}
		entry.expires = renewed
	}
	a.cache.put(key, entry, now)
	return entry.user, renewed, nil
}

// Logout ends the session, immediately; an unknown token is not an error.
func (a *Auth) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	h := hashToken(token)
	a.cache.delete(string(h))
	return a.repo.DeleteSession(ctx, h)
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func validDisplayName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n == 0 || n > 60 {
		return "", errors.Join(ErrInvalidUser, errors.New("the name must be between 1 and 60 characters"))
	}
	return name, nil
}

// UpdateProfile changes the display name (1-60 characters) and refreshes the user's cached sessions.
func (a *Auth) UpdateProfile(ctx context.Context, userID int64, displayName string) (User, error) {
	name, err := validDisplayName(displayName)
	if err != nil {
		return User{}, err
	}
	u, err := a.repo.UpdateDisplayName(ctx, userID, name)
	if err != nil {
		return User{}, err
	}
	a.cache.updateUser(userID, func(c *User) { c.DisplayName = u.DisplayName })
	return u, nil
}

// verifyCurrent checks the password of an open account; any failure is ErrInvalidCredentials.
func (a *Auth) verifyCurrent(ctx context.Context, userID int64, password string) (Credentials, error) {
	if utf8.RuneCountInString(password) > maxPasswordLen {
		return Credentials{}, ErrInvalidCredentials
	}
	rec, err := a.repo.CredentialsByID(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return Credentials{}, ErrInvalidCredentials
	}
	if err != nil {
		return Credentials{}, err
	}
	if ok, verr := verifyPassword(password, rec.PasswordHash); verr != nil || !ok {
		return Credentials{}, ErrInvalidCredentials
	}
	return rec, nil
}

// ChangePassword verifies the current password (ErrInvalidCredentials when wrong), stores the new one and deletes every
// other session of the user; keepToken (the caller's own session cookie) stays signed in.
func (a *Auth) ChangePassword(ctx context.Context, userID int64, current, next, keepToken string) error {
	if n := utf8.RuneCountInString(next); n < minPasswordLen || n > maxPasswordLen {
		return errors.Join(ErrInvalidUser, errors.New("the password must be between 10 and 128 characters"))
	}
	if _, err := a.verifyCurrent(ctx, userID, current); err != nil {
		return err
	}
	hash, err := hashPassword(next, a.argon)
	if err != nil {
		return err
	}
	if err := a.repo.UpdatePasswordHash(ctx, userID, hash); err != nil {
		return err
	}
	_, err = a.SignOutOthers(ctx, userID, keepToken)
	return err
}

// SignOutOthers deletes every session of the user except keepToken's and returns how many were deleted.
func (a *Auth) SignOutOthers(ctx context.Context, userID int64, keepToken string) (int, error) {
	var keep []byte
	if keepToken != "" {
		keep = hashToken(keepToken)
	}
	n, err := a.repo.DeleteSessionsExcept(ctx, userID, keep)
	if err != nil {
		return 0, err
	}
	a.cache.dropUser(userID, string(keep))
	return n, nil
}

// CloseAccount anonymises the account (rows other players rely on stay) after verifying the password, and ends every
// session. Refused (ErrRefused) for the organiser of an unfinished campaign and for the last superuser.
func (a *Auth) CloseAccount(ctx context.Context, userID int64, password string) error {
	rec, err := a.verifyCurrent(ctx, userID, password)
	if err != nil {
		return err
	}
	organises, err := a.repo.OrganisesUnfinishedCampaign(ctx, userID)
	if err != nil {
		return err
	}
	if organises {
		return Refuse("Hand over your campaigns (on the campaign overview) or finish them first")
	}
	if rec.Role == RoleSuperuser {
		others, err := a.repo.OtherSuperusers(ctx, userID)
		if err != nil {
			return err
		}
		if others == 0 {
			return Refuse("You are the last superuser; make someone else a superuser first")
		}
	}
	email := fmt.Sprintf("closed-%d@invalid", userID)
	if err := a.repo.CloseUser(ctx, userID, "Former player", email, a.now()); err != nil {
		return err
	}
	a.cache.dropUser(userID, "")
	return nil
}

// SetAppearance saves the palette and mode of the account (ErrInvalidUser for an unknown key) and updates the cached
// sessions at once, so the next page already renders it.
func (a *Auth) SetAppearance(ctx context.Context, userID int64, palette, mode string) error {
	ap := Appearance{Palette: palette, Mode: mode}
	if !ap.Valid() {
		return errors.Join(ErrInvalidUser, errors.New("unknown palette or mode"))
	}
	if err := a.repo.SetAppearance(ctx, userID, palette, mode); err != nil {
		return err
	}
	a.cache.updateUser(userID, func(c *User) { c.Appearance = ap })
	return nil
}

// ValidTimezone says whether name is an IANA zone the server knows ("" is automatic and valid).
func ValidTimezone(name string) bool {
	if name == "" {
		return true
	}
	if len(name) > 64 || strings.EqualFold(name, "local") {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// SetTimezone saves the zone the account wants times shown in ("" for automatic; ErrInvalidUser for a name that is no IANA zone) and
// updates the cached sessions at once. Stored timestamps are UTC and never change.
func (a *Auth) SetTimezone(ctx context.Context, userID int64, name string) error {
	name = strings.TrimSpace(name)
	if !ValidTimezone(name) {
		return errors.Join(ErrInvalidUser, errors.New("unknown time zone"))
	}
	if err := a.repo.SetTimezone(ctx, userID, name); err != nil {
		return err
	}
	a.cache.updateUser(userID, func(c *User) { c.Timezone = name })
	return nil
}

// UserLocation is the zone to show a user's times in: their saved zone, else the browser's (the tz cookie, already read by the web
// layer), else the server's fallback (APP_TIMEZONE), else UTC. Unknown names are skipped.
func UserLocation(u *User, browser, fallback string) *time.Location {
	for _, name := range []string{userZone(u), browser, fallback} {
		if name != "" && ValidTimezone(name) {
			if loc, err := time.LoadLocation(name); err == nil {
				return loc
			}
		}
	}
	return time.UTC
}

func userZone(u *User) string {
	if u == nil {
		return ""
	}
	return u.Timezone
}

// forgetUser and setRecoveryPending make *Auth the sessionControl of the Recovery service: Auth owns the session cache.
func (a *Auth) forgetUser(userID int64) { a.cache.dropUser(userID, "") }

func (a *Auth) setRecoveryPending(userID int64, pending bool) {
	a.cache.setRecoveryPending(userID, pending)
}
