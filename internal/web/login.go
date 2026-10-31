package web

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

type loginView struct {
	pageBase
	Email  string
	Error  string
	Notice string // a fixed message chosen by ?notice= (after closing the account)
	Next   string // where to go after signing in: a path of this site, from ?next=
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if userFrom(r.Context()) != nil {
		redirect(w, r, "/")
		return
	}
	view := loginView{pageBase: s.base(r), Next: safeNext(r.URL.Query().Get("next"))}
	switch r.URL.Query().Get("notice") {
	case "closed":
		view.Notice = "Your account is closed. Your companies stay in their campaigns under “Former player”."
	case "password-set":
		view.Notice = "Your password was changed. Sign in with the new one."
	}
	s.render.page(w, r, http.StatusOK, "login", view)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if userFrom(r.Context()) != nil {
		redirect(w, r, "/")
		return
	}
	email, password := r.PostFormValue("email"), r.PostFormValue("password")
	next := safeNext(r.PostFormValue("next"))
	view := loginView{pageBase: s.base(r), Email: email, Next: next}
	ip := s.clientIP(r)

	if s.throttle.blocked(ip, email) {
		view.Error = "Too many attempts. Please wait a few minutes and try again."
		s.render.page(w, r, http.StatusTooManyRequests, "login", view)
		return
	}
	token, expires, user, err := s.auth.Login(r.Context(), email, password)
	switch {
	case errors.Is(err, core.ErrInvalidCredentials):
		s.throttle.fail(ip, email)
		view.Error = "Invalid email or password."
		s.render.page(w, r, http.StatusUnprocessableEntity, "login", view)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	s.throttle.reset(ip, email)
	s.setSessionCookie(w, token, expires)
	s.adoptAppearance(w, r, user)
	if next == "" {
		next = "/"
	}
	redirect(w, r, next)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.auth.Logout(r.Context(), c.Value); err != nil {
			s.serverError(w, r, err)
			return
		}
	}
	s.clearSessionCookie(w)
	redirect(w, r, "/login")
}

// safeNext keeps a ?next= value only when it is a path of this site (never another address), so a login link cannot send
// somebody elsewhere.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") || strings.ContainsAny(next, "\r\n") {
		return ""
	}
	return next
}

// clientIP is the address the login throttle counts: the peer's, or with TrustProxy the last X-Forwarded-For entry (behind a
// reverse proxy every peer is the proxy, so counting those would lock everybody out together).
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
			return last
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
