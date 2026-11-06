package web

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
)

// diceScript is the queue of scripted dice the end-to-end tests fill (core.ScriptedDice): the server rolls from it before it rolls at random.
type diceScript interface {
	Push(values ...int)
	Clear()
	Len() int
	Die(sides int) int
}

// e2eDiceTokenHeader carries E2E_DICE_TOKEN on every request to /_e2e/dice.
const e2eDiceTokenHeader = "X-E2E-Dice-Token"

// SetDiceScript makes the server roll from the script and serves /_e2e/dice to fill it (docs/testing.md, "Scripting the server's dice").
// Only a development server started with E2E_DICE and a token takes it: in production, or without either, it does nothing, so
// the route does not exist and the server rolls at random.
func (s *Server) SetDiceScript(d diceScript) {
	if d == nil || !s.diceScriptAllowed() {
		return
	}
	s.diceScript = d
	s.die = d.Die
}

func (s *Server) diceScriptAllowed() bool {
	return s.cfg.E2EDice && !s.cfg.Production() && s.cfg.E2EDiceToken != ""
}

// registerDiceScript adds the routes of the script. It is not called into existence unless SetDiceScript took a script, so a
// production configuration answers 404 like any unknown path.
func (s *Server) registerDiceScript(mux *http.ServeMux) {
	if s.diceScript == nil || !s.diceScriptAllowed() {
		return
	}
	mux.HandleFunc("GET /_e2e/dice", s.diceGuard(s.handleDiceState))
	mux.HandleFunc("POST /_e2e/dice", s.diceGuard(s.handleDiceQueue))
	mux.HandleFunc("DELETE /_e2e/dice", s.diceGuard(s.handleDiceClear))
}

// diceGuard lets only the test harness in: a loopback peer that knows the token. No session, no cookie.
func (s *Server) diceGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(e2eDiceTokenHeader)), []byte(s.cfg.E2EDiceToken)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) writeDiceState(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"queued": s.diceScript.Len()})
}

func (s *Server) handleDiceState(w http.ResponseWriter, r *http.Request) { s.writeDiceState(w) }

// handleDiceQueue appends {"next":[3,5,...]} to the queue. Each result is checked against the die when a roll takes it.
func (s *Server) handleDiceQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Next []int `json:"next"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || len(body.Next) == 0 {
		http.Error(w, `want {"next":[3,5,...]}`, http.StatusBadRequest)
		return
	}
	s.diceScript.Push(body.Next...)
	s.writeDiceState(w)
}

func (s *Server) handleDiceClear(w http.ResponseWriter, r *http.Request) {
	s.diceScript.Clear()
	s.writeDiceState(w)
}
