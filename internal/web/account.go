package web

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The Account page (docs/auth.md, "The Account page"): one column of cards, each an htmx form that swaps only itself and
// answers 422 with what was typed kept, so a mistake never loses the form.

type accountView struct {
	pageBase
	Profile accountCard
	Pass    accountCard
	Others  accountCard
	Zone    accountCard
	Close   accountCard
}

// accountCard is what one card shows: the raw text of its fields, the message of each field ("" for the form as a whole)
// and a notice (success, or a failure when NoticeErr is set).
type accountCard struct {
	User      core.User
	Name      string
	Email     string // what was typed in the close-account confirmation
	Errors    map[string]string
	Notice    string
	NoticeErr bool
	Open      bool      // the close-account card is expanded
	Now       time.Time // the time zone card: the time now in the zone shown
}

func (s *Server) newAccountView(r *http.Request) accountView {
	u := *userFrom(r.Context())
	return accountView{pageBase: s.base(r), Profile: accountCard{User: u, Name: u.DisplayName}, Pass: accountCard{User: u}, Others: accountCard{User: u}, Zone: accountCard{User: u, Name: u.Timezone, Now: time.Now().In(locationFrom(r.Context()))}, Close: accountCard{User: u}}
}

// TimezoneNames feeds the zone list of the Account page.
func (accountCard) TimezoneNames() []string { return timezoneNames }

func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	s.render.page(w, r, http.StatusOK, "account", s.newAccountView(r))
}

// sessionToken is the caller's own session, which stays signed in when the others are ended.
func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

func (s *Server) handleAccountProfile(w http.ResponseWriter, r *http.Request) {
	user := *userFrom(r.Context())
	card := accountCard{User: user, Name: r.PostFormValue("name"), Errors: map[string]string{}}
	updated, err := s.auth.UpdateProfile(r.Context(), user.ID, card.Name)
	switch {
	case errors.Is(err, core.ErrInvalidUser):
		card.Errors["name"] = reason(err)
		s.render.partial(w, http.StatusUnprocessableEntity, "account", "account_profile", card)
		return
	case s.fail(w, r, err):
		return
	}
	card.User, card.Name, card.Notice = updated, updated.DisplayName, "Your name is saved."
	s.render.partial(w, http.StatusOK, "account", "account_profile", card)
}

func (s *Server) handleAccountPassword(w http.ResponseWriter, r *http.Request) {
	user := *userFrom(r.Context())
	current, next, confirm := r.PostFormValue("current"), r.PostFormValue("new"), r.PostFormValue("confirm")
	card := accountCard{User: user, Errors: map[string]string{}}
	rejected := func(status int) { s.render.partial(w, status, "account", "account_password", card) }
	if s.accountThrottle.blocked(throttleKey(user), "") {
		card.Errors[""] = "Too many wrong passwords. Please wait a few minutes and try again."
		rejected(http.StatusTooManyRequests)
		return
	}
	if current == "" {
		card.Errors["current"] = "Enter your current password"
	}
	if n := utf8.RuneCountInString(next); n < 10 || n > 128 {
		card.Errors["new"] = "The new password must be between 10 and 128 characters"
	}
	if confirm != next {
		card.Errors["confirm"] = "The two passwords do not match"
	}
	if len(card.Errors) > 0 {
		rejected(http.StatusUnprocessableEntity)
		return
	}
	err := s.auth.ChangePassword(r.Context(), user.ID, current, next, sessionToken(r))
	switch {
	case errors.Is(err, core.ErrInvalidCredentials):
		s.accountThrottle.fail(throttleKey(user), "")
		card.Errors["current"] = "That is not your current password"
		rejected(http.StatusUnprocessableEntity)
		return
	case errors.Is(err, core.ErrInvalidUser):
		card.Errors["new"] = reason(err)
		rejected(http.StatusUnprocessableEntity)
		return
	case s.fail(w, r, err):
		return
	}
	s.accountThrottle.reset(throttleKey(user), "")
	card.Notice = "Your password is changed. Every other device was signed out."
	rejected(http.StatusOK)
}

func (s *Server) handleAccountSignOutOthers(w http.ResponseWriter, r *http.Request) {
	user := *userFrom(r.Context())
	n, err := s.auth.SignOutOthers(r.Context(), user.ID, sessionToken(r))
	if s.fail(w, r, err) {
		return
	}
	card := accountCard{User: user, Notice: "You are not signed in anywhere else."}
	switch n {
	case 0:
	case 1:
		card.Notice = "Signed out of 1 other session."
	default:
		card.Notice = "Signed out of " + itoa(n) + " other sessions."
	}
	s.render.partial(w, http.StatusOK, "account", "account_sessions", card)
}

func (s *Server) handleAccountClose(w http.ResponseWriter, r *http.Request) {
	user := *userFrom(r.Context())
	typed, password := strings.TrimSpace(r.PostFormValue("email")), r.PostFormValue("password")
	card := accountCard{User: user, Email: typed, Errors: map[string]string{}, Open: true}
	rejected := func(status int) { s.render.partial(w, status, "account", "account_close", card) }
	if s.accountThrottle.blocked(throttleKey(user), "") {
		card.Errors[""] = "Too many wrong passwords. Please wait a few minutes and try again."
		rejected(http.StatusTooManyRequests)
		return
	}
	if !strings.EqualFold(typed, user.Email) {
		card.Errors["email"] = "Type your email address exactly to confirm"
	}
	if password == "" {
		card.Errors["password"] = "Enter your password"
	}
	if len(card.Errors) > 0 {
		rejected(http.StatusUnprocessableEntity)
		return
	}
	err := s.auth.CloseAccount(r.Context(), user.ID, password)
	switch {
	case errors.Is(err, core.ErrInvalidCredentials):
		s.accountThrottle.fail(throttleKey(user), "")
		card.Errors["password"] = "That is not your password"
		rejected(http.StatusUnprocessableEntity)
		return
	case errors.Is(err, core.ErrRefused):
		card.Errors[""] = reason(err)
		rejected(http.StatusUnprocessableEntity)
		return
	case s.fail(w, r, err):
		return
	}
	s.clearSessionCookie(w)
	redirect(w, r, "/login?notice=closed")
}

// throttleKey names a user in the account throttle (its first argument is normally an address).
func throttleKey(u core.User) string { return "user-" + itoa(int(u.ID)) }

// handleAccountAppearance saves the palette and mode chosen in the settings menu (the browser's theme store calls it
// after every change). 401 for anonymous visitors, who only have the cookie; 422 for an unknown key.
func (s *Server) handleAccountAppearance(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if user == nil {
		s.refuse(w, r, http.StatusUnauthorized, "sign in first")
		return
	}
	err := s.auth.SetAppearance(r.Context(), user.ID, r.PostFormValue("palette"), r.PostFormValue("mode"))
	switch {
	case errors.Is(err, core.ErrInvalidUser):
		s.refuse(w, r, http.StatusUnprocessableEntity, reason(err))
	case s.fail(w, r, err):
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

const themeCookie = "bcm-theme"

// themeCookieValue is the cookie's "<palette>.<mode>" as an Appearance (zero when absent or unknown).
func themeCookieValue(r *http.Request) core.Appearance {
	c, err := r.Cookie(themeCookie)
	if err != nil {
		return core.Appearance{}
	}
	palette, mode, _ := strings.Cut(c.Value, ".")
	if a := (core.Appearance{Palette: palette, Mode: mode}); a.Valid() {
		return a
	}
	return core.Appearance{}
}

// adoptAppearance runs at sign-in: a user who never chose a look keeps what this browser shows (saved to the account), and
// otherwise the saved look is written to the cookie, so the new device starts right.
func (s *Server) adoptAppearance(w http.ResponseWriter, r *http.Request, u core.User) {
	if u.Appearance.Valid() {
		http.SetCookie(w, &http.Cookie{Name: themeCookie, Value: u.Appearance.Palette + "." + u.Appearance.Mode, Path: "/", MaxAge: 31536000, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies})
		return
	}
	if a := themeCookieValue(r); a.Valid() {
		if err := s.auth.SetAppearance(r.Context(), u.ID, a.Palette, a.Mode); err != nil {
			slog.Warn("saving the appearance at sign-in failed", "err", err)
		}
	}
}

// handleAccountTimezone saves the zone times are shown in; an empty name is automatic (the browser's zone).
func (s *Server) handleAccountTimezone(w http.ResponseWriter, r *http.Request) {
	user := *userFrom(r.Context())
	zone := strings.TrimSpace(r.PostFormValue("zone"))
	card := accountCard{User: user, Name: zone, Errors: map[string]string{}}
	err := s.auth.SetTimezone(r.Context(), user.ID, zone)
	switch {
	case errors.Is(err, core.ErrInvalidUser):
		card.Errors["zone"] = "That is not a time zone name. Pick one from the list, for example Europe/London, or leave it empty for automatic."
		card.Now = time.Now().In(locationFrom(r.Context()))
		s.render.partial(w, http.StatusUnprocessableEntity, "account", "account_timezone", card)
		return
	case s.fail(w, r, err):
		return
	}
	card.User.Timezone = zone
	card.Notice = "Your time zone is saved."
	if zone == "" {
		card.Notice = "Times follow this device's zone again."
	}
	loc := core.UserLocation(&card.User, tzCookieValue(r), s.cfg.AppTimezone)
	card.Now = time.Now().In(loc)
	s.render.partial(w, http.StatusOK, "account", "account_timezone", card)
}

func tzCookieValue(r *http.Request) string {
	if c, err := r.Cookie(tzCookie); err == nil {
		if v, err := url.QueryUnescape(c.Value); err == nil { // an older script percent-encoded the slash
			return v
		}
	}
	return ""
}
