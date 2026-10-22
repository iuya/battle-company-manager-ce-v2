package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestUnknownAddressIsARenderedNotFoundPage(t *testing.T) {
	f := newFixture(t, Config{})
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/no/such/page", nil)))
	body := rec.Body.String()
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, want := range []string{"<h1", "Page not found", `lang="en"`, `href="/"`, "<title>Page not found"} {
		if !strings.Contains(body, want) {
			t.Errorf("the error page lacks %q", want)
		}
	}
}

func TestErrorPageForVisitorsPointsToSignIn(t *testing.T) {
	f := newFixture(t, Config{})
	rec := f.do(httptest.NewRequest("GET", "/no/such/page", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `href="/login"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestErrorForHTMXStaysPlainText(t *testing.T) {
	f := newFixture(t, Config{})
	r := f.signedIn(httptest.NewRequest("GET", "/admin", nil))
	r.Header.Set("HX-Request", "true")
	rec := f.do(r)
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "<html") {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestServerErrorDoesNotLeakItsCause(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("pq: password authentication failed for secret_user")).AnyTimes()
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(frodo, time.Time{}, nil).AnyTimes()
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec := f.do(r)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret_user") || !strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestTheSameMatchSentTwiceIsMadeOnce(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).Times(2)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1}, {ID: 2}, {ID: 3}}, nil).Times(2)
	f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), gomock.Any()).Return(core.Match{ID: 12}, nil).Times(1)
	send := func(a, b string) *httptest.ResponseRecorder {
		return f.do(f.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {a}, "side-2": {b}})))
	}
	for i := 0; i < 2; i++ {
		if rec := send("1", "2"); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/matches/12" {
			t.Fatalf("send %d: %d -> %q", i, rec.Code, rec.Header().Get("Location"))
		}
	}
}
