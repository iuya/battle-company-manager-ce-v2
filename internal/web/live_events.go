package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

const (
	liveKeepAlive   = 25 * time.Second // a comment line keeps proxies from closing an idle stream
	liveWriteLimit  = 10 * time.Second
	liveStreamLimit = 2 * time.Hour // the client reconnects (and refetches) after this
)

// handleLiveEvents is the Server-Sent Events stream of a match: it carries pointers ("company 12 changed, seq 41"), never state or
// HTML; the page refetches its fragment (docs/architecture.md "Live match").
func (s *Server) handleLiveEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	matchID, ok := s.idParam(w, r, "match")
	if !ok {
		return
	}
	user := userFrom(r.Context())
	m, err := s.campaigns.Match(r.Context(), id, matchID)
	if s.fail(w, r, err) {
		return
	}
	if _, err := s.campaigns.Get(r.Context(), user.ID, id); s.fail(w, r, err) { // not a member: 404 like the page
		return
	}
	rc := http.NewResponseController(w)
	start := func() {
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache, no-transform")
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		_ = rc.SetWriteDeadline(time.Now().Add(liveWriteLimit))
		fmt.Fprint(w, "retry: 3000\n: connected\n\n")
		_ = rc.Flush()
	}
	if m.Ended { // nothing to follow: tell the page to reload into the view it has now
		start()
		sendEvent(w, rc, "ended", 0)
		return
	}
	stream, err := s.campaigns.Subscribe(r.Context(), *user, id, matchID)
	switch {
	case errors.Is(err, core.ErrNotFound):
		s.notFound(w, r)
		return
	case errors.Is(err, core.ErrRefused):
		w.Header().Set("Retry-After", "10")
		http.Error(w, err.Error(), http.StatusTooManyRequests) // the page polls instead
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	defer stream.Close()
	start()
	ctx, cancel := context.WithTimeout(r.Context(), liveStreamLimit)
	defer cancel()
	for {
		wait, stop := context.WithTimeout(ctx, liveKeepAlive)
		batch, err := stream.Next(wait)
		stop()
		switch {
		case err == nil:
		case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil: // idle: keep the connection alive
			_ = rc.SetWriteDeadline(time.Now().Add(liveWriteLimit))
			if _, werr := fmt.Fprint(w, ": ping\n\n"); werr != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
			continue
		default: // the client left, the hub closed at shutdown, or the stream timed out
			return
		}
		for _, c := range batch.Companies {
			if !sendEvent(w, rc, fmt.Sprintf("company-%d", c), batch.Seq) {
				return
			}
		}
		if batch.Match || len(batch.Companies) > 0 {
			if !sendEvent(w, rc, "match", batch.Seq) {
				return
			}
		}
		if batch.Ended {
			sendEvent(w, rc, "ended", batch.Seq)
			return
		}
	}
}

// sendEvent writes one event and flushes; false when the client is gone or too slow.
func sendEvent(w http.ResponseWriter, rc *http.ResponseController, name string, seq int64) bool {
	_ = rc.SetWriteDeadline(time.Now().Add(liveWriteLimit))
	if _, err := fmt.Fprintf(w, "event: %s\nid: %d\ndata: {\"seq\":%d}\n\n", name, seq, seq); err != nil {
		return false
	}
	return rc.Flush() == nil
}
