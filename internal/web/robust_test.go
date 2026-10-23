package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestGuardInputCapsFormsButLeavesUploadsToTheirHandlers(t *testing.T) {
	f := newFixture(t, Config{})
	var read int
	var rerr error
	h := f.srv.guardInput(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		read, rerr = len(b), err
	}))
	big := strings.Repeat("a", 2*maxFormBody)
	for _, tc := range []struct {
		name, ctype string
		wantCode    int
		wantErr     bool
	}{
		{"urlencoded form", "application/x-www-form-urlencoded", 413, false},
		{"no content type", "", 413, false},
		{"multipart upload", "multipart/form-data; boundary=x", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rerr, read = nil, 0
			r := httptest.NewRequest("POST", "/x", strings.NewReader(big))
			r.Header.Set("Content-Type", tc.ctype)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Code != tc.wantCode {
				t.Errorf("code %d, want %d", rec.Code, tc.wantCode)
			}
			if tc.wantCode == 200 && (rerr != nil || read != len(big)) {
				t.Errorf("the handler read %d bytes, err %v", read, rerr)
			}
		})
	}
}

func TestGuardInputRefusesNULAndInvalidUTF8ButPassesTheBodyOn(t *testing.T) {
	f := newFixture(t, Config{})
	var got string
	h := f.srv.guardInput(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.PostFormValue("name") + "|" + r.URL.Query().Get("q")
	}))
	do := func(target, body string) int {
		r := httptest.NewRequest("POST", target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	for _, tc := range []struct {
		target, body string
		want         int
	}{
		{"/x", "name=Bilbo+%C3%9F", 200},
		{"/x?q=%E2%9C%93", "name=a", 200},
		{"/x", "name=a%00b", 400},
		{"/x", "na%00me=a", 400},
		{"/x", "name=%ff%fe", 400},
		{"/x?q=%00", "name=a", 400},
		{"/x?q=%ff", "name=a", 400},
		{"/x", "name=100%", 200}, // a stray percent sign is the handler's business
	} {
		if code := do(tc.target, tc.body); code != tc.want {
			t.Errorf("%s %q: %d, want %d", tc.target, tc.body, code, tc.want)
		}
	}
	if code := do("/x", "name=Frodo"); code != 200 || got != "Frodo|" {
		t.Errorf("the body was not passed on: %d %q", code, got)
	}
}

func TestRecovererAnswers500AndKeepsServing(t *testing.T) {
	f := newFixture(t, Config{})
	h := f.srv.recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			panic("boom")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))
	if rec.Code != 500 {
		t.Errorf("code = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ok", nil))
	if rec.Code != 204 {
		t.Errorf("after a panic: %d", rec.Code)
	}
}

// The server's read and write timeouts must not cut the live stream: it lifts the read deadline and sets a deadline per write.
func TestLiveStreamOutlivesTheServerTimeouts(t *testing.T) {
	f := newFixture(t, Config{})
	st := newFakeStream()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(openMatch(), nil)
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Subscribe(gomock.Any(), frodo, int64(7), int64(3)).Return(st, nil)
	srv := httptest.NewUnstartedServer(f.srv.Handler())
	srv.Config.ReadTimeout, srv.Config.WriteTimeout = 200*time.Millisecond, 200*time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest("GET", srv.URL+"/campaigns/7/matches/3/events", nil)
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(frodo, time.Time{}, nil).AnyTimes()
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	time.Sleep(600 * time.Millisecond)
	st.batches <- core.LiveBatch{Seq: 5, Match: true}
	rd := bufio.NewReader(resp.Body)
	var got strings.Builder
	for i := 0; i < 7 && !strings.Contains(got.String(), "event: match"); i++ {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("stream cut after the timeouts: %v (%q)", err, got.String())
		}
		got.WriteString(line)
	}
	if !strings.Contains(got.String(), "event: match") {
		t.Errorf("got %q", got.String())
	}
}

func TestLoginThrottleTableIsBounded(t *testing.T) {
	th := newLoginThrottle()
	for i := 0; i < 3*maxThrottleKeys; i++ {
		th.fail(fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255), "who@example.com")
	}
	th.mu.Lock()
	defer th.mu.Unlock()
	if n := len(th.fails); n > maxThrottleKeys+2 {
		t.Errorf("%d keys held", n)
	}
}

// A once-guarded request that hangs holds up that person's next request, not everybody's.
func TestOnceDoesNotSerialiseDifferentPeople(t *testing.T) {
	f := newFixture(t, Config{})
	release := make(chan struct{})
	started := make(chan struct{})
	h := f.srv.once(func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()).ID == 1 {
			close(started)
			<-release
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := func(id int64) *http.Request {
		r := httptest.NewRequest("POST", "/x", strings.NewReader("a=1"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r.WithContext(context.WithValue(r.Context(), ctxKey{}, &core.User{ID: id}))
	}
	go h(httptest.NewRecorder(), req(1))
	<-started
	done := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h(rec, req(2))
		done <- rec.Code
	}()
	select {
	case code := <-done:
		if code != 204 {
			t.Errorf("code %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Error("a request of another person waits for a stuck one")
	}
	close(release)
}
