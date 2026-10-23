package web

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// matchCards are the cards the match page used to fetch after loading (Against the Odds, secret objectives, end checks, log).
// The full page builds them in parallel and draws them in its own answer, so the first paint has them; a card that failed is nil
// and its slot falls back to the old fetch (hx-trigger="load"). The answers to writes (match_body) leave Cards nil: their slots
// stay lazy as before.
type matchCards struct {
	ATO     *atoView
	Secret  *secretView
	End     *endView
	EndNone bool // the end checks answered "no such open match": the card is empty
	Log     *liveLogView
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
	if !m.Ended && v.Campaign.Status != "finished" {
		run(func() {
			e := endView{Campaign: id, MatchID: m.ID, Card: true}
			checks, err := s.campaigns.EndChecks(r.Context(), *userFrom(r.Context()), id, m.ID)
			switch {
			case err == nil:
				e.Checks = checks
				c.End = &e
			case errors.Is(err, core.ErrNotFound):
				c.EndNone = true // the handler answers an empty card here
			}
		})
	}
	run(func() {
		if lv, err := s.logViewOf(r, id, m, 0); err == nil {
			lv.Card = true
			c.Log = &lv
		}
	})
	wg.Wait()
	v.Cards = c
}
