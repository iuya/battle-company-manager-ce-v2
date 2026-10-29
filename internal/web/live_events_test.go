package web

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// fakeStream is a core.LiveStream the test feeds.
type fakeStream struct {
	batches chan core.LiveBatch
	closed  chan struct{}
}

func newFakeStream() *fakeStream {
	return &fakeStream{batches: make(chan core.LiveBatch, 4), closed: make(chan struct{})}
}

func (s *fakeStream) Next(ctx context.Context) (core.LiveBatch, error) {
	select {
	case b := <-s.batches:
		return b, nil
	case <-ctx.Done():
		return core.LiveBatch{}, ctx.Err()
	}
}
func (s *fakeStream) Close() { close(s.closed) }

func openMatch() core.Match { return core.Match{ID: 3, Number: 3} }

func eventsServer(t *testing.T, f *fixture) (*httptest.Server, *http.Request) {
	t.Helper()
	srv := httptest.NewServer(f.srv.Handler())
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest("GET", srv.URL+"/campaigns/7/matches/3/events", nil)
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(frodo, time.Time{}, nil).AnyTimes()
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	return srv, req
}

func TestLiveEventsStreamThroughTheMiddlewareAndFlush(t *testing.T) {
	f := newFixture(t, Config{})
	st := newFakeStream()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(openMatch(), nil)
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Subscribe(gomock.Any(), frodo, int64(7), int64(3)).Return(st, nil)
	_, req := eventsServer(t, f)
	resp, err := http.DefaultClient.Do(req) // returns as soon as the headers are flushed, before any event
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" || resp.Header.Get("X-Accel-Buffering") != "no" || !strings.Contains(resp.Header.Get("Cache-Control"), "no-cache") {
		t.Fatalf("headers: %d %v", resp.StatusCode, resp.Header)
	}
	rd := bufio.NewReader(resp.Body)
	read := func(n int) string { // n lines
		var b strings.Builder
		for i := 0; i < n; i++ {
			line, err := rd.ReadString('\n')
			if err != nil {
				t.Fatalf("read: %v (%q)", err, b.String())
			}
			b.WriteString(line)
		}
		return b.String()
	}
	if got := read(3); !strings.HasPrefix(got, "retry: 3000") {
		t.Errorf("first bytes: %q", got)
	}
	st.batches <- core.LiveBatch{Seq: 9, Companies: []int64{3, 4}, Match: true}
	got := read(12)
	for _, want := range []string{"event: company-3\nid: 9\ndata: {\"seq\":9}\n\n", "event: company-4\nid: 9", "event: match\nid: 9"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	st.batches <- core.LiveBatch{Seq: 10, Ended: true}
	if got := read(4); !strings.Contains(got, "event: ended") {
		t.Errorf("ended: %q", got)
	}
	if rest, _ := rd.ReadString('\n'); rest != "" {
		t.Errorf("the response ends after ended: %q", rest)
	}
	select {
	case <-st.closed:
	case <-time.After(2 * time.Second):
		t.Error("the stream is closed when the handler returns")
	}
}

func TestLiveEventsEndedMatchAnswersEndedAndCloses(t *testing.T) {
	f := newFixture(t, Config{})
	m := openMatch()
	m.Ended = true
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(m, nil)
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	_, req := eventsServer(t, f)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		b.WriteString(sc.Text() + "\n")
	}
	if !strings.Contains(b.String(), "event: ended") {
		t.Errorf("got %q", b.String())
	}
}

func TestLiveEventsRefusals(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(openMatch(), nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().Subscribe(gomock.Any(), frodo, int64(7), int64(3)).Return(nil, core.Refuse("Too many live connections open; close another tab."))
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/events", nil)))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("caps: %d", rec.Code)
	}
	g := newFixture(t, Config{})
	g.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(openMatch(), nil)
	g.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(core.CampaignDetail{}, core.ErrNotFound)
	if rec := g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/events", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("not a member: %d", rec.Code)
	}
}

func TestLiveEventsEndWhenTheClientLeaves(t *testing.T) {
	f := newFixture(t, Config{})
	st := newFakeStream()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(openMatch(), nil)
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Subscribe(gomock.Any(), frodo, int64(7), int64(3)).Return(st, nil)
	_, req := eventsServer(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	resp.Body.Close()
	select {
	case <-st.closed:
	case <-time.After(2 * time.Second):
		t.Error("a cancelled request frees its stream")
	}
}

// ?since=<generation> makes the stream skip the initial refetch (SubscribeSince); a value that is not a generation is ignored.
func TestLiveEventsWithAGenerationSubscribesSince(t *testing.T) {
	for _, tc := range []struct {
		query string
		since bool
	}{{"?since=4.41", true}, {"?since=garbage", false}, {"?since=4.41%0A", false}, {"", false}} {
		f := newFixture(t, Config{})
		st := newFakeStream()
		f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(openMatch(), nil)
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
		if tc.since {
			f.camp.EXPECT().SubscribeSince(gomock.Any(), frodo, int64(7), int64(3), "4.41").Return(st, nil)
		} else {
			f.camp.EXPECT().Subscribe(gomock.Any(), frodo, int64(7), int64(3)).Return(st, nil)
		}
		srv, req := eventsServer(t, f)
		req.URL, _ = req.URL.Parse(srv.URL + "/campaigns/7/matches/3/events" + tc.query)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("%q: %d", tc.query, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
