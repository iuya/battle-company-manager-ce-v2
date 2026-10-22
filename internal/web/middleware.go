package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

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
		slog.InfoContext(r.Context(), "request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "took", time.Since(start))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.ErrorContext(r.Context(), "panic", "value", v)
				s.refuse(w, r, http.StatusInternalServerError, "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
