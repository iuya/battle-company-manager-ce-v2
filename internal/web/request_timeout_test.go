package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestTimeout(t *testing.T) {
	waits := func(seen *error) http.Handler { // a handler stuck on its context, as a slow query is
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				*seen = r.Context().Err()
			case <-time.After(5 * time.Second):
			}
		})
	}
	tests := []struct {
		name, method, path string
		d                  time.Duration
		want               error
	}{
		{"time is up", "GET", "/campaigns/1", 30 * time.Millisecond, context.DeadlineExceeded},
		{"a POST is bound too", "POST", "/campaigns/1/status", 30 * time.Millisecond, context.DeadlineExceeded},
		{"an upload is bound too", "POST", "/admin/rulesets", 30 * time.Millisecond, context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen error
			start := time.Now()
			requestTimeout(tt.d, waits(&seen)).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tt.method, tt.path, nil))
			if seen != tt.want {
				t.Errorf("context error = %v, want %v", seen, tt.want)
			}
			if time.Since(start) > 2*time.Second {
				t.Errorf("took %s: the deadline did not cut the handler", time.Since(start))
			}
		})
	}
	t.Run("the client leaving cancels it", func(t *testing.T) {
		var seen error
		ctx, cancel := context.WithCancel(context.Background())
		go func() { time.Sleep(30 * time.Millisecond); cancel() }()
		req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
		requestTimeout(time.Minute, waits(&seen)).ServeHTTP(httptest.NewRecorder(), req)
		if seen != context.Canceled {
			t.Errorf("context error = %v, want Canceled", seen)
		}
	})
	t.Run("the live stream is exempt", func(t *testing.T) {
		var dl bool
		h := requestTimeout(time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, dl = r.Context().Deadline() }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/campaigns/7/matches/3/events", nil))
		if dl {
			t.Error("the event stream got a deadline")
		}
	})
	t.Run("zero turns it off", func(t *testing.T) {
		var dl bool
		h := requestTimeout(0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, dl = r.Context().Deadline() }))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
		if dl {
			t.Error("a deadline was set")
		}
	})
}

// A handler that gives up on its context answers 503, not 500 and not a stack trace.
func TestServerErrorOnDeadlineIs503(t *testing.T) {
	f := newFixture(t, Config{})
	for _, err := range []error{context.DeadlineExceeded, context.Canceled} {
		rec := httptest.NewRecorder()
		f.srv.serverError(rec, httptest.NewRequest("GET", "/", nil), err)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%v: status %d, want 503", err, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	f.srv.serverError(rec, httptest.NewRequest("GET", "/", nil), http.ErrAbortHandler)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("other errors: status %d, want 500", rec.Code)
	}
}

func TestResolveRefusesNegativeRequestTimeout(t *testing.T) {
	c := Config{AppEnv: "development", CSPMode: "report", RequestTimeout: -time.Second}
	if err := c.Resolve(); err == nil {
		t.Error("a negative REQUEST_TIMEOUT was accepted")
	}
	c.RequestTimeout = 0
	if err := c.Resolve(); err != nil {
		t.Errorf("0 (off) refused: %v", err)
	}
}
