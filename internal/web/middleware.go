package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

func itoa(i int) string { return strconv.Itoa(i) }

// tokenRoutes are the path prefixes whose next segment is a secret (a campaign invitation, an account invitation, a recovery link).
var tokenRoutes = []string{"/join/", "/register/", "/recover/"}

// logPath is the path to write in a log line: the route pattern ("/register/{token}") for the routes whose path holds a secret,
// never the secret itself, otherwise the path as it is.
func logPath(r *http.Request) string {
	for _, prefix := range tokenRoutes {
		if strings.HasPrefix(r.URL.Path, prefix) {
			return prefix + "{token}"
		}
	}
	return r.URL.Path
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	loc    *time.Location // the viewer's zone, set by the session middleware for the renderer (location.go)
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.NewResponseController reach the real writer (Flush and write deadlines of the live stream).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.InfoContext(r.Context(), "request", "method", r.Method, "path", logPath(r), "status", rec.status, "took", time.Since(start))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v) // the handler asked net/http to drop the connection quietly
				}
				slog.ErrorContext(r.Context(), "panic", "value", v, "method", r.Method, "path", logPath(r), "stack", string(debug.Stack()))
				s.refuse(w, r, http.StatusInternalServerError, "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// requestTimeout gives every request a context that ends after d (0: no limit), so the queries and the rule computation behind a
// request stop when the time is up, and, because the context of a request also ends when its client leaves, when the client is gone.
// net/http's WriteTimeout does neither. The live stream is long lived by design and is exempt (it ends with its client).
func requestTimeout(d time.Duration, next http.Handler) http.Handler {
	if d <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/events") {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// maxFormBody caps the body of every request that is not a multipart upload (those set their own, larger limit): the forms of the
// app are a few hundred bytes, and net/http's own default would read 10 MB of urlencoded values into memory per request.
const maxFormBody = 1 << 20

// guardInput is the door every request passes before a handler reads its input. It caps the body (maxFormBody; multipart uploads set
// their own limit), and answers 400 to a query or urlencoded form that holds a NUL byte or invalid UTF-8: PostgreSQL refuses such
// text, which would otherwise surface as a 500 from whichever handler stores it. The body is read once here and put back for the
// handlers (and the double-submit guard) to read again.
func (s *Server) guardInput(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !cleanText(r.URL.RawQuery) {
			s.refuse(w, r, http.StatusBadRequest, "The request holds characters that cannot be stored.")
			return
		}
		if r.Body != nil && r.Body != http.NoBody && !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFormBody))
			if err != nil {
				s.refuse(w, r, http.StatusRequestEntityTooLarge, "The request is too large.")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") && !cleanText(string(body)) {
				s.refuse(w, r, http.StatusBadRequest, "The request holds characters that cannot be stored.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// cleanText reports whether the percent-decoded form of s (a query string or an urlencoded body) is valid UTF-8 without NUL bytes.
func cleanText(s string) bool {
	if !strings.ContainsAny(s, "%\x00") && utf8.ValidString(s) {
		return true // no escapes to decode
	}
	vals, err := url.ParseQuery(s)
	if err != nil {
		return utf8.ValidString(s) && !strings.ContainsRune(s, 0) // a malformed escape: the handlers see what net/http leaves of it
	}
	for k, vs := range vals {
		if !utf8.ValidString(k) || strings.ContainsRune(k, 0) {
			return false
		}
		for _, v := range vs {
			if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
				return false
			}
		}
	}
	return true
}
