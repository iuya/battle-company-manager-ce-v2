package web

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
)

// matchCards are the cards of the match page that are not part of its main body (Against the Odds, secret objectives, log).
// The full page builds them in parallel and draws them in its own answer, so the first paint has them; a card that failed is nil
// and its slot falls back to a fetch on load (hx-trigger="load"). The answers to writes (match_body) leave Cards nil: their slots
// stay lazy.
type matchCards struct {
	ATO    *atoView
	Secret *secretView
	Log    *liveLogView
}

// loadMatchCards builds the cards of a full match page, one goroutine each.
func (s *Server) loadMatchCards(r *http.Request, v *matchView) {
	c := &matchCards{}
	id, m := v.Campaign.ID, v.Match
	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A panic here would not reach the HTTP recover middleware and would end the process: the card stays nil
			// and its slot falls back to the lazy fetch, which does run under the middleware.
			defer func() {
				if p := recover(); p != nil {
					slog.Error("match card panicked", "panic", p, "stack", string(debug.Stack()))
				}
			}()
			f()
		}()
	}
	run(func() {
		if a, err := s.atoCard(r, id, m); err == nil {
			c.ATO = &a
		}
	})
	if v.ShowSecret() {
		run(func() {
			if sv, err := s.secretCard(r, id, m); err == nil {
				c.Secret = &sv
			}
		})
	}
	run(func() {
		if lv, err := s.logViewOf(r, id, m, 0); err == nil {
			c.Log = &lv
		}
	})
	wg.Wait()
	v.Cards = c
}
