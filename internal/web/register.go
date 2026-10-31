package web

import (
	"errors"
	"net/http"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The public pages of single-use links (docs/auth.md, "Account invitations" and "Password recovery"): register with an account
// invitation, set a new password with a recovery link. Neither says why a link does not work, only core.TalkToAdministrator, so
// they cannot be used to find out which addresses have an account.

type registerView struct {
	pageBase
	Token       string
	Kind        core.InvitationKind
	StoredEmail string // the address of a named invitation: read from the invitation, never from the request
	Email       string // what was typed (anonymous invitations)
	Name        string
	Errors      []string
}

// linkView is the shared "this link does not work" page: Message is the generic text, or the throttle's.
type linkView struct {
	pageBase
	Message string
}

// unusable draws the generic page of a dead link with the status of the request (404 on GET, 422 on POST, 429 when throttled).
func (s *Server) unusable(w http.ResponseWriter, r *http.Request, status int, message string) {
	secretPage(w)
	s.render.page(w, r, status, "link_unusable", linkView{pageBase: s.bare(r), Message: message})
}

// handleRegisterPage shows the registration form of a pending invitation. A signed-in visitor is sent to the home page without the
// link being looked at. The ?email= of a named link is for people to read; the address shown is the invitation's own.
func (s *Server) handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	secretPage(w)
	if userFrom(r.Context()) != nil {
		redirect(w, r, "/")
		return
	}
	token, ip := r.PathValue("token"), s.clientIP(r)
	key := tokenKey(token)
	if s.registerThrottle.blocked(ip, key) {
		s.unusable(w, r, http.StatusTooManyRequests, tooManyAttempts)
		return
	}
	pre, err := s.invitations.Preview(r.Context(), token)
	switch {
	case errors.Is(err, core.ErrLinkUnusable): // a look at a dead link is cheap and does not count: only attempts to use one do
		s.unusable(w, r, http.StatusNotFound, core.TalkToAdministrator)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	s.render.page(w, r, http.StatusOK, "register", registerView{pageBase: s.bare(r), Token: token, Kind: pre.Kind, StoredEmail: pre.Email})
}

// handleRegister creates the account and signs it in. Every reason the link cannot be used answers the same page with 422.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	secretPage(w)
	if userFrom(r.Context()) != nil {
		redirect(w, r, "/")
		return
	}
	token, ip := r.PathValue("token"), s.clientIP(r)
	key := tokenKey(token)
	if s.registerThrottle.blocked(ip, key) {
		s.unusable(w, r, http.StatusTooManyRequests, tooManyAttempts)
		return
	}
	in := core.Registration{Email: r.PostFormValue("email"), DisplayName: r.PostFormValue("name"), Password: r.PostFormValue("password")}
	view := registerView{pageBase: s.bare(r), Token: token, Email: in.Email, Name: in.DisplayName}
	if r.PostFormValue("confirm") != in.Password {
		pre, err := s.invitations.Preview(r.Context(), token)
		switch {
		case errors.Is(err, core.ErrLinkUnusable):
			s.registerThrottle.fail(ip, key)
			s.unusable(w, r, http.StatusUnprocessableEntity, core.TalkToAdministrator)
		case err != nil:
			s.serverError(w, r, err)
		default:
			view.Kind, view.StoredEmail, view.Errors = pre.Kind, pre.Email, []string{"The two passwords do not match."}
			s.render.page(w, r, http.StatusUnprocessableEntity, "register", view)
		}
		return
	}
	session, err := s.invitations.Register(r.Context(), token, in)
	switch {
	case errors.Is(err, core.ErrLinkUnusable):
		s.registerThrottle.fail(ip, key)
		s.unusable(w, r, http.StatusUnprocessableEntity, core.TalkToAdministrator)
		return
	case errors.Is(err, core.ErrInvalidUser):
		// the link is good (the service looked at it before the fields): the form comes back with what is wrong
		pre, perr := s.invitations.Preview(r.Context(), token)
		if perr != nil {
			s.registerThrottle.fail(ip, key)
			s.unusable(w, r, http.StatusUnprocessableEntity, core.TalkToAdministrator)
			return
		}
		view.Kind, view.StoredEmail, view.Errors = pre.Kind, pre.Email, reasons(err)
		s.render.page(w, r, http.StatusUnprocessableEntity, "register", view)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	s.setSessionCookie(w, session.Token, session.Expires)
	s.adoptAppearance(w, r, session.User)
	redirect(w, r, "/campaigns")
}

type recoverView struct {
	pageBase
	Token  string
	Errors []string
}

// handleRecoverPage shows the new-password form of a pending recovery link. It never reads the session cookie: opening the link
// is not an access to the app, so it does not invalidate itself.
func (s *Server) handleRecoverPage(w http.ResponseWriter, r *http.Request) {
	secretPage(w)
	token, ip := r.PathValue("token"), s.clientIP(r)
	key := tokenKey(token)
	if s.recoverThrottle.blocked(ip, key) {
		s.unusable(w, r, http.StatusTooManyRequests, tooManyAttempts)
		return
	}
	err := s.recovery.Preview(r.Context(), token)
	switch {
	case errors.Is(err, core.ErrLinkUnusable):
		s.unusable(w, r, http.StatusNotFound, core.TalkToAdministrator)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	s.render.page(w, r, http.StatusOK, "recover", recoverView{pageBase: s.bare(r), Token: token})
}

// handleRecover sets the new password. On success every session of the account is gone and the person signs in with the new one:
// the link alone never opens a session.
func (s *Server) handleRecover(w http.ResponseWriter, r *http.Request) {
	secretPage(w)
	token, ip := r.PathValue("token"), s.clientIP(r)
	key := tokenKey(token)
	if s.recoverThrottle.blocked(ip, key) {
		s.unusable(w, r, http.StatusTooManyRequests, tooManyAttempts)
		return
	}
	password := r.PostFormValue("password")
	view := recoverView{pageBase: s.bare(r), Token: token}
	if r.PostFormValue("confirm") != password {
		if err := s.recovery.Preview(r.Context(), token); err != nil {
			s.recoverDead(w, r, ip, key, err)
			return
		}
		view.Errors = []string{"The two passwords do not match."}
		s.render.page(w, r, http.StatusUnprocessableEntity, "recover", view)
		return
	}
	err := s.recovery.SetPassword(r.Context(), token, password)
	switch {
	case errors.Is(err, core.ErrInvalidUser):
		if perr := s.recovery.Preview(r.Context(), token); perr != nil {
			s.recoverDead(w, r, ip, key, perr)
			return
		}
		view.Errors = reasons(err)
		s.render.page(w, r, http.StatusUnprocessableEntity, "recover", view)
		return
	case err != nil:
		s.recoverDead(w, r, ip, key, err)
		return
	}
	s.recoverThrottle.reset(ip, key)
	redirect(w, r, "/login?notice=password-set")
}

// recoverDead answers a failure of the link itself (generic page) or a broken service (500).
func (s *Server) recoverDead(w http.ResponseWriter, r *http.Request, ip, key string, err error) {
	if errors.Is(err, core.ErrLinkUnusable) {
		s.recoverThrottle.fail(ip, key)
		s.unusable(w, r, http.StatusUnprocessableEntity, core.TalkToAdministrator)
		return
	}
	s.serverError(w, r, err)
}
