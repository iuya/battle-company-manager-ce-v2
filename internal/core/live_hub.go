package core

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// Caps on the streams one process serves (REVIEW: 10 per user, 50 per match).
const (
	LiveStreamsPerUser  = 10
	LiveStreamsPerMatch = 50
)

// ErrLiveClosed is what Next returns once the stream or the hub is closed.
var ErrLiveClosed = errors.New("live stream closed")

// LiveHub fans the notifications of the live feed out to the streams of the browsers. It holds no state of the matches (that is
// in the database) and nothing but one coalesced batch per stream, so a slow client costs one batch, never a backlog, and a
// restart loses nothing: clients reconnect and refetch (every new stream starts with a batch for all companies).
type LiveHub struct {
	feed liveFeed

	mu       sync.Mutex
	closed   bool
	streams  map[*liveStream]struct{}
	perUser  map[int64]int
	perMatch map[int64]int
}

// NewLiveHub builds the hub over the feed (the Postgres LISTEN connection, or a fake in tests).
func NewLiveHub(feed liveFeed) *LiveHub {
	return &LiveHub{feed: feed, streams: map[*liveStream]struct{}{}, perUser: map[int64]int{}, perMatch: map[int64]int{}}
}

// Start runs the feed until ctx is done: every event reaches the streams of its match, a (re)connect marks everything changed.
func (h *LiveHub) Start(ctx context.Context) error {
	return h.feed.Run(ctx, h.publish, h.resync)
}

// Close ends every stream (Next returns ErrLiveClosed) and refuses new ones: wire it to the server's shutdown.
func (h *LiveHub) Close() {
	h.mu.Lock()
	h.closed = true
	list := make([]*liveStream, 0, len(h.streams))
	for s := range h.streams {
		list = append(list, s)
	}
	h.mu.Unlock()
	for _, s := range list {
		s.Close()
	}
}

func (h *LiveHub) publish(ev LiveEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.streams {
		if s.match == ev.MatchID {
			s.add(ev)
		}
	}
}

func (h *LiveHub) resync() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.streams {
		s.markAll(0)
	}
}

// subscribe opens a stream of a match whose companies are given; its first batch refetches all of them (initial sync).
func (h *LiveHub) subscribe(userID, matchID int64, companies []int64, seq int64) (LiveStream, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.closed:
		return nil, Refuse("The server is shutting down.")
	case h.perUser[userID] >= LiveStreamsPerUser:
		return nil, Refuse("Too many live connections open; close another tab.")
	case h.perMatch[matchID] >= LiveStreamsPerMatch:
		return nil, Refuse("Too many people are watching this match live; the page updates every few seconds instead.")
	}
	s := &liveStream{hub: h, user: userID, match: matchID, companies: slices.Clone(companies), wake: make(chan struct{}, 1), done: make(chan struct{})}
	s.markAll(seq)
	h.streams[s] = struct{}{}
	h.perUser[userID]++
	h.perMatch[matchID]++
	return s, nil
}

type liveStream struct {
	hub       *LiveHub
	user      int64
	match     int64
	companies []int64

	mu      sync.Mutex
	pending LiveBatch
	has     bool
	wake    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (s *liveStream) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *liveStream) markAll(seq int64) {
	s.mu.Lock()
	for _, c := range s.companies {
		if !slices.Contains(s.pending.Companies, c) {
			s.pending.Companies = append(s.pending.Companies, c)
		}
	}
	s.pending.Match, s.has = true, true
	s.pending.Seq = max(s.pending.Seq, seq)
	s.mu.Unlock()
	s.signal()
}

func (s *liveStream) add(ev LiveEvent) {
	switch {
	case ev.Kind == LiveEnded:
		s.mu.Lock()
		s.pending.Ended, s.has = true, true
		s.mu.Unlock()
		s.signal()
	case ev.CompanyID == 0:
		s.markAll(ev.Seq)
	default:
		s.mu.Lock()
		if !slices.Contains(s.pending.Companies, ev.CompanyID) {
			s.pending.Companies = append(s.pending.Companies, ev.CompanyID)
		}
		s.pending.Match, s.has = true, true
		s.pending.Seq = max(s.pending.Seq, ev.Seq)
		s.mu.Unlock()
		s.signal()
	}
}

// Next blocks until something changed and returns what changed since the last call.
func (s *liveStream) Next(ctx context.Context) (LiveBatch, error) {
	for {
		select {
		case <-s.done:
			return LiveBatch{}, ErrLiveClosed
		default:
		}
		s.mu.Lock()
		if s.has {
			b := s.pending
			s.pending, s.has = LiveBatch{}, false
			s.mu.Unlock()
			return b, nil
		}
		s.mu.Unlock()
		select {
		case <-s.wake:
		case <-s.done:
			return LiveBatch{}, ErrLiveClosed
		case <-ctx.Done():
			return LiveBatch{}, ctx.Err()
		}
	}
}

// Close unsubscribes and frees the slot in the caps; it is safe to call twice.
func (s *liveStream) Close() {
	s.once.Do(func() {
		close(s.done)
		h := s.hub
		h.mu.Lock()
		delete(h.streams, s)
		h.perUser[s.user]--
		h.perMatch[s.match]--
		h.mu.Unlock()
	})
}
