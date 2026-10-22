package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Production reports whether the server runs in production mode (APP_ENV=production, the default of the environment loader).
func (c Config) Production() bool { return c.AppEnv == "production" }

// Resolve fills the settings derived from the environment: SecureCookies from SECURE_COOKIES, or from the mode when unset. It
// rejects values it does not know, so a typo does not silently turn a protection off.
func (c *Config) Resolve() error {
	switch c.AppEnv {
	case "production", "development":
	default:
		return fmt.Errorf("APP_ENV %q: want production or development", c.AppEnv)
	}
	switch strings.ToLower(c.SecureCookiesEnv) {
	case "":
		c.SecureCookies = c.Production()
	case "true", "1":
		c.SecureCookies = true
	case "false", "0":
		c.SecureCookies = false
	default:
		return fmt.Errorf("SECURE_COOKIES %q: want true or false", c.SecureCookiesEnv)
	}
	if c.Production() && !c.SecureCookies && !c.AllowInsecureCookies {
		return errors.New("refusing to start: production with insecure cookies; set SECURE_COOKIES=true (behind a TLS-terminating proxy), " +
			"or ALLOW_INSECURE_COOKIES=true to accept session cookies over plain http (docs/operations.md)")
	}
	switch c.CSPMode {
	case "off", "report", "enforce":
	default:
		return fmt.Errorf("CSP_MODE %q: want off, report or enforce", c.CSPMode)
	}
	return nil
}

// Warnings lists settings that are legal but weak in production; the server logs them at start. Insecure cookies reach it only
// with ALLOW_INSECURE_COOKIES=true (Resolve refuses them otherwise) and are still warned about.
func (c Config) Warnings() []string {
	if !c.Production() {
		return nil
	}
	var w []string
	if !c.SecureCookies {
		w = append(w, "SECURE_COOKIES is off in production: the session cookie travels over plain http; serve over HTTPS and leave it on")
	}
	if !c.TrustProxy {
		w = append(w, "TRUST_PROXY is off: behind a reverse proxy every login attempt counts against the proxy's address; set TRUST_PROXY=true when a proxy that sets X-Forwarded-For is in front (docs/architecture.md, Deploying)")
	}
	if c.CSPMode == "off" {
		w = append(w, "CSP_MODE=off: no Content-Security-Policy header is sent")
	}
	return w
}

// SetReadiness installs the check /readyz runs (the database ping). Without one /readyz answers like /healthz.
func (s *Server) SetReadiness(check func(context.Context) error) { s.ready = check }

// handleHealthz says the process is up and serving; it touches nothing else, so an orchestrator can use it as a liveness probe.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok\n")
}

// handleReadyz says the app can serve requests: the database answers. The reason stays in the log, not in the response.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if s.ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.ready(ctx); err != nil {
			slog.WarnContext(r.Context(), "not ready", "err", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "unavailable\n")
			return
		}
	}
	_, _ = io.WriteString(w, "ok\n")
}

// cspPolicy is the policy of docs/security-and-risks.md section 1: everything from our own origin, no plugins, no framing, and
// 'unsafe-eval' / style 'unsafe-inline' only because Alpine's standard build and the style= attributes need them.
const cspPolicy = "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"font-src 'self'; connect-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; object-src 'none'; " +
	"report-uri /csp-report"

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		if s.cfg.SecureCookies {
			// Browsers ignore this over plain http, so it is safe to send whenever the site is meant to be served over HTTPS.
			// No includeSubDomains/preload: they commit the whole domain and are the owner's decision.
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		switch s.cfg.CSPMode {
		case "report":
			h.Set("Content-Security-Policy-Report-Only", cspPolicy)
		case "enforce":
			h.Set("Content-Security-Policy", cspPolicy)
		}
		next.ServeHTTP(w, r)
	})
}

const (
	maxCSPReportBytes = 4 << 10
	maxCSPReportsMin  = 60 // per minute, whole server: a flood becomes a count, not a full disk
	maxCSPField       = 200
)

// reportLimiter is a fixed window counter of the reports logged per minute.
type reportLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	start   time.Time
	n       int
	dropped int
}

func newReportLimiter() *reportLimiter { return &reportLimiter{now: time.Now} }

// allow reports whether one more report may be logged now; dropped is how many the last window refused (for one summary line).
func (l *reportLimiter) allow() (ok bool, dropped int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := l.now(); now.Sub(l.start) >= time.Minute {
		dropped, l.dropped, l.n, l.start = l.dropped, 0, 0, now
	}
	if l.n >= maxCSPReportsMin {
		l.dropped++
		return false, dropped
	}
	l.n++
	return true, dropped
}

// handleCSPReport logs a Content-Security-Policy violation report as one line. It answers 204 whatever it gets, reads at most 4 KB
// and logs at most 60 reports a minute; fields are cut so a report cannot fill the log or forge lines.
func (s *Server) handleCSPReport(w http.ResponseWriter, r *http.Request) {
	defer w.WriteHeader(http.StatusNoContent)
	if s.cfg.CSPMode == "off" {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCSPReportBytes))
	if err != nil {
		return
	}
	ok, dropped := s.cspReports.allow()
	if dropped > 0 {
		slog.Warn("csp reports dropped by the rate limit", "count", dropped)
	}
	if !ok {
		return
	}
	var rep struct {
		R map[string]any `json:"csp-report"`
	}
	if json.Unmarshal(body, &rep) != nil || rep.R == nil {
		return
	}
	field := func(k string) string { return cutField(fmt.Sprint(rep.R[k])) }
	slog.Warn("csp violation",
		"directive", field("effective-directive"), "violated", field("violated-directive"), "blocked", field("blocked-uri"),
		"document", field("document-uri"), "source", field("source-file"), "line", field("line-number"), "disposition", field("disposition"))
}

// cutField makes a reported value safe for one log line: printable, no line breaks, at most maxCSPField runes.
func cutField(v string) string {
	if v == "<nil>" {
		return ""
	}
	v = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return ' '
		}
		return r
	}, v)
	if r := []rune(v); len(r) > maxCSPField {
		return string(r[:maxCSPField]) + "..."
	}
	return v
}
