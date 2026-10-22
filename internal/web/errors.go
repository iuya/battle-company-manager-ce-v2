package web

import (
	"log/slog"
	"net/http"
)

// errorView is the data of the error page.
type errorView struct {
	pageBase
	Status  int
	Heading string
	Message string
}

// notFound answers 404; refuse has the page.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.refuse(w, r, http.StatusNotFound, "")
}

// refuse answers a refused or broken request with the given status. A full page request gets the error page in the layout
// (a title, a heading, the reason and a way back); an htmx request gets the short plain text, which the layout's
// handler shows in its alert. A 5xx never shows its cause: it is logged, the person sees a fixed sentence.
func (s *Server) refuse(w http.ResponseWriter, r *http.Request, status int, message string) {
	if status >= 500 {
		message = "Something went wrong on our side. Try again in a moment; if it keeps happening, tell the organiser."
	}
	if message == "" && status == http.StatusNotFound {
		message = "There is nothing at this address, or you may not have access to it."
	}
	if message == "" {
		message = http.StatusText(status)
	}
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true" {
		http.Error(w, message, status)
		return
	}
	heading := http.StatusText(status)
	switch status {
	case http.StatusNotFound:
		heading = "Page not found"
	case http.StatusForbidden:
		heading = "Not allowed"
	case http.StatusUnauthorized:
		heading = "Sign in needed"
	case http.StatusInternalServerError:
		heading = "Something went wrong"
	}
	if s.render == nil {
		http.Error(w, message, status)
		return
	}
	s.render.page(w, r, status, "error", errorView{pageBase: s.base(r), Status: status, Heading: heading, Message: message})
}

// handleNotFound is the catch-all of the router.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.notFound(w, r)
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "err", err)
	s.refuse(w, r, http.StatusInternalServerError, "")
}
