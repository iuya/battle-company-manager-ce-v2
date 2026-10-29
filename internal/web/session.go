package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

const sessionCookie = "bcm_session"

type ctxKey struct{}

// userFrom returns the signed-in user of the request, or nil.
func userFrom(ctx context.Context) *core.User {
	u, _ := ctx.Value(ctxKey{}).(*core.User)
	return u
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies,
	})
}

// session resolves the session cookie, if any, and makes the user available to the handler. It never rejects the
// request: pages that need a user are wrapped with protected.
func (s *Server) session(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			user, renewed, err := s.auth.Authenticate(r.Context(), c.Value)
			switch {
			case err == nil:
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, &user))
				if !renewed.IsZero() {
					s.setSessionCookie(w, c.Value, renewed)
				}
			case errors.Is(err, core.ErrNoSession):
				s.clearSessionCookie(w) // expired or revoked: drop the stale cookie
			default:
				s.serverError(w, r, err)
				return
			}
		}
		next(w, s.withLocation(w, r))
	}
}

// protected sends visitors who are not signed in to the login page.
func (s *Server) protected(next http.HandlerFunc) http.HandlerFunc {
	return s.session(func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()) == nil {
			redirect(w, r, loginURL(r))
			return
		}
		next(w, r)
	})
}

// loginURL is the login page with ?next= set to the page the visitor wanted, so signing in brings them back to it. Only GET
// pages are remembered (a POST cannot be replayed); for an htmx request that is the page the browser shows, not the fragment.
func loginURL(r *http.Request) string {
	next := ""
	switch {
	case r.Header.Get("HX-Request") == "true":
		if u, err := url.Parse(r.Header.Get("HX-Current-URL")); err == nil && u.Host == r.Host {
			next = u.RequestURI()
		}
	case r.Method == http.MethodGet:
		next = r.URL.RequestURI()
	}
	if next = safeNext(next); next == "" || next == "/" || strings.HasPrefix(next, "/login") {
		return "/login"
	}
	return "/login?next=" + url.QueryEscape(next)
}

// requireSuperuser answers 404 to everybody else, so the administration section does not even announce itself.
func (s *Server) requireSuperuser(next http.HandlerFunc) http.HandlerFunc {
	return s.protected(func(w http.ResponseWriter, r *http.Request) {
		if !userFrom(r.Context()).IsSuperuser() {
			s.notFound(w, r)
			return
		}
		next(w, r)
	})
}

// redirect sends the browser to url. For htmx requests it asks for a full page load (HX-Redirect) so the whole
// layout, including the header that depends on who is signed in, is rendered again.
func redirect(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// readOnly opens a core read scope for the length of a request that only reads (a GET page or card): the services it calls share the
// company, campaign, match, Against the Odds record and live snapshot they read instead of asking the database again
// (core.WithReadScope). Never wrap a handler that writes: a write does not invalidate the scope.
func readOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r.WithContext(core.WithReadScope(r.Context())))
	}
}
