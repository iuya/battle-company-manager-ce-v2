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
	// SetUserPermissionsByID returns ErrNotFound when there is no such user.
	SetUserPermissionsByID(ctx context.Context, userID int64, role Role, canCreateCampaigns bool) error
	// UserByID returns the user, closed accounts included (closed is true for them), or ErrNotFound.
	UserByID(ctx context.Context, userID int64) (user User, closed bool, err error)
	// ListUsers returns the accounts whose name or email contains q (case-insensitive, empty: all): open ones by display
	// name, then closed ones. limit and offset page the result; total is the number of matches.
	ListUsers(ctx context.Context, q string, limit, offset int) (rows []AdminUser, total int, err error)
	// UserByEmail returns ErrNotFound when there is no such user.
	UserByEmail(ctx context.Context, email string) (Credentials, error)
	CreateSession(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error
	// UserBySession returns ErrNotFound when the session does not exist or expired before now.
	UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (User, time.Time, error)
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

// CreateUser validates and registers a user. There is no sign-up page: accounts are created with the server's CLI.
func (a *Auth) CreateUser(ctx context.Context, email, displayName, password string) (User, error) {
	email = normalizeEmail(email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return User{}, errors.Join(ErrInvalidUser, errors.New("the email address is not valid"))
	}
	displayName = strings.TrimSpace(displayName)
	if n := utf8.RuneCountInString(displayName); n == 0 || n > 60 {
		return User{}, errors.Join(ErrInvalidUser, errors.New("the name must be between 1 and 60 characters"))
	}
	if n := utf8.RuneCountInString(password); n < minPasswordLen || n > maxPasswordLen {
		return User{}, errors.Join(ErrInvalidUser, errors.New("the password must be between 10 and 128 characters"))
	}
	hash, err := hashPassword(password, a.argon)
	if err != nil {
		return User{}, err
	}
	return a.repo.CreateUser(ctx, email, displayName, hash)
}

// SetPermissions changes a user's global role and campaign-creation permission (used by the CLI). The checks of
// SetUserPermissions apply, without an actor.
func (a *Auth) SetPermissions(ctx context.Context, email string, role Role, canCreateCampaigns bool) error {
	if !role.Valid() {
		return errors.Join(ErrInvalidUser, errors.New("the role must be user, admin or superuser"))
	}
	rec, err := a.repo.UserByEmail(ctx, normalizeEmail(email))
	if err != nil {
		return err
	}
	return a.setPermissions(ctx, rec.ID, role, canCreateCampaigns)
}

// SetUserPermissions changes the role and the creation flag of the user with that id. ErrRefused (with a reason fit to
// show) unless actor is a superuser, for a closed account and for demoting the last open superuser; ErrInvalidUser for an
// unknown role; ErrNotFound for an unknown id. The user's cached sessions are dropped so the change applies at once.
func (a *Auth) SetUserPermissions(ctx context.Context, actor User, userID int64, role Role, canCreateCampaigns bool) error {
	if !actor.IsSuperuser() {
		return Refuse("Only a superuser can change permissions")
	}
	if !role.Valid() {
		return errors.Join(ErrInvalidUser, errors.New("the role must be user, admin or superuser"))
	}
	return a.setPermissions(ctx, userID, role, canCreateCampaigns)
}

func (a *Auth) setPermissions(ctx context.Context, userID int64, role Role, canCreateCampaigns bool) error {
	target, closed, err := a.repo.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if closed {
		return Refuse("This account is closed")
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
	if err := a.repo.SetUserPermissionsByID(ctx, userID, role, canCreateCampaigns); err != nil {
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
// ErrRefused unless actor is a superuser.
func (a *Auth) ListUsers(ctx context.Context, actor User, q string, limit, offset int) (rows []AdminUser, total int, err error) {
	if !actor.IsSuperuser() {
		return nil, 0, Refuse("Only a superuser can list the accounts")
	}
	if limit <= 0 {
		limit = defaultUserPage
	}
	limit = min(limit, maxUserPage)
	offset = max(offset, 0)
	return a.repo.ListUsers(ctx, strings.TrimSpace(q), limit, offset)
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
	if err := a.repo.CreateSession(ctx, hashToken(token), rec.ID, expires); err != nil {
		return "", time.Time{}, User{}, err
	}
	_ = a.repo.DeleteExpiredSessions(ctx, now) // housekeeping; a failure must not fail the login
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
		user, expires, err := a.repo.UserBySession(ctx, h, now)
		if errors.Is(err, ErrNotFound) {
			return User{}, time.Time{}, ErrNoSession
		}
		if err != nil {
			return User{}, time.Time{}, err
		}
		entry = cachedSession{user: user, expires: expires, checked: now}
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
