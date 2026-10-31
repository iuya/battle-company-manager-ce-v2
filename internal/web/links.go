package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// accountInvitations is what the Invitations page and the registration page need (docs/auth.md, "Account invitations");
// core.AccountInvitations implements it. A dead link is always core.ErrLinkUnusable, whatever the cause.
//
//go:generate go tool mockgen -typed -source=links.go -destination=mock_links_test.go -package=web
type accountInvitations interface {
	// Create makes an invitation (administrators; core.ErrForbidden otherwise, core.ErrInvalidInput for out-of-range values,
	// core.ErrRefused for a named invitation to an address that has an account). The raw token is in the result only.
	Create(ctx context.Context, actor core.User, in core.NewAccountInvitation) (core.CreatedAccountInvitation, error)
	List(ctx context.Context, actor core.User, f core.InvitationFilter, limit, offset int) ([]core.AccountInvitation, int, error)
	// Revoke revokes a pending invitation (core.ErrRefused with the state otherwise, core.ErrNotFound for an unknown id).
	Revoke(ctx context.Context, actor core.User, id int64) (core.AccountInvitation, error)
	// Preview says what a link offers, changing nothing; core.ErrLinkUnusable for every dead link.
	Preview(ctx context.Context, token string) (core.InvitationPreview, error)
	// Register creates the account and its session; core.ErrLinkUnusable (dead link, address taken), core.ErrInvalidUser (fields).
	Register(ctx context.Context, token string, in core.Registration) (core.RegisteredSession, error)
}

// recoveries is what the recovery links of the Users page and the set-password page need; core.Recovery implements it.
type recoveries interface {
	// Generate makes the account's link (superusers; core.ErrForbidden otherwise); ttl 0 is the default of 24 hours.
	Generate(ctx context.Context, actor core.User, userID int64, ttl time.Duration) (core.RecoveryLink, error)
	// Revoke revokes the account's pending link (core.ErrRefused when there is none).
	Revoke(ctx context.Context, actor core.User, userID int64) error
	// Preview says nothing changes and the link can be used; core.ErrLinkUnusable otherwise.
	Preview(ctx context.Context, token string) error
	// SetPassword uses the link; core.ErrLinkUnusable, core.ErrInvalidUser (a weak password).
	SetPassword(ctx context.Context, token, password string) error
}

// SetAccountLinks installs the services behind account invitations and recovery links (the Invitations page, the public
// registration and set-password pages, the recovery card of the Users page).
func (s *Server) SetAccountLinks(inv accountInvitations, rec recoveries) {
	s.invitations, s.recovery = inv, rec
}

// tokenKey is what the throttles of the public link pages count by instead of the secret: a short fingerprint of the token.
func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:8])
}

// secretPage marks the answer of a public link page: nothing caches it, no referrer carries the address on, no search engine keeps it.
func secretPage(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Robots-Tag", "noindex")
}

const tooManyAttempts = "Too many attempts. Please wait a few minutes and try again."
