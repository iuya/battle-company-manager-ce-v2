package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// recentMatches remembers the match a person has just made, so that the same form sent twice within a few seconds (a double
// click, a retry on a slow line) opens the first match instead of making a second, identical one. Two matches of the same
// companies are legal, but not within seconds of each other from the same person.
type recentMatches struct {
	mu   sync.Mutex
	seen map[string]recentMatch
}

type recentMatch struct {
	id int64
	at time.Time
}

const recentMatchWindow = 10 * time.Second

// create runs make unless the same thing (what: who, where and its content) was made within the window; it returns its id.
// Calls are serialised, so two requests arriving together make one match.
func (c *recentMatches) create(what any, make func() (int64, error)) (int64, error) {
	key, merr := json.Marshal(what)
	if merr != nil { // cannot tell two apart: do not guess
		return make()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, v := range c.seen {
		if now.Sub(v.at) > recentMatchWindow {
			delete(c.seen, k)
		}
	}
	if v, ok := c.seen[string(key)]; ok {
		return v.id, nil
	}
	id, err := make()
	if err != nil {
		return 0, err
	}
	if c.seen == nil {
		c.seen = map[string]recentMatch{}
	}
	c.seen[string(key)] = recentMatch{id: id, at: now}
	return id, nil
}

// once guards an action that spends or rolls something (a purchase, a roll, a grant) against being sent twice: a request
// identical to one the same person made within onceWindow (same address, same body) is not run again, it gets the first
// answer. That is what a double click or a retry on a slow line sends. Requests are run one at a time, so two sent together
// are handled in order.
func (s *Server) once(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		if u == nil || r.Method != http.MethodPost || strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			next(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			s.refuse(w, r, http.StatusRequestEntityTooLarge, "the request is too large or malformed")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		sum := sha256.Sum256(body)
		key := fmt.Sprint(u.ID, " ", r.URL.Path, " ", r.Header.Get("HX-Request"), " ", hex.EncodeToString(sum[:]))

		s.onceMu.Lock()
		defer s.onceMu.Unlock()
		now := time.Now()
		for k, v := range s.onceSeen {
			if now.Sub(v.at) > onceWindow {
				delete(s.onceSeen, k)
			}
		}
		if v, ok := s.onceSeen[key]; ok {
			for h, vals := range v.header {
				w.Header()[h] = vals
			}
			w.WriteHeader(v.status)
			w.Write(v.body)
			return
		}
		rec := &replayRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		if len(rec.body) <= 1<<20 && rec.status < 500 {
			if s.onceSeen == nil {
				s.onceSeen = map[string]onceAnswer{}
			}
			s.onceSeen[key] = onceAnswer{at: now, status: rec.status, header: rec.Header().Clone(), body: rec.body}
		}
	}
}

const onceWindow = 3 * time.Second

type onceAnswer struct {
	at     time.Time
	status int
	header http.Header
	body   []byte
}

// replayRecorder passes the answer through and keeps a copy of it.
type replayRecorder struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (r *replayRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }
func (r *replayRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}
