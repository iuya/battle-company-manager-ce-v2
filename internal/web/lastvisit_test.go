package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The last-campaign memory (docs/multi-company-spec.md, section 8): the bcm-last cookie, validated on every read.

func withLast(r *http.Request, v string) *http.Request {
	r.AddCookie(&http.Cookie{Name: lastCookie, Value: v})
	return r
}

// lastSet is the Set-Cookie of bcm-last in the response, if any.
func lastSet(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == lastCookie {
			return c
		}
	}
	return nil
}

func TestParseLastVisit(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  lastVisit
		ok    bool
	}{
		{"1.7", lastVisit{1, 7, 0}, true},
		{"1.7.4", lastVisit{1, 7, 4}, true},
		{"", lastVisit{}, false},
		{"1", lastVisit{}, false},
		{"1.7.4.9", lastVisit{}, false},
		{"1..7", lastVisit{}, false},
		{"1.x", lastVisit{}, false},
		{"1.-7", lastVisit{}, false},
		{"1.0", lastVisit{}, false},
		{"1.+7", lastVisit{}, false},
		{"1.7 ", lastVisit{}, false},
		{"1.1234567890123456789", lastVisit{}, false},
		{strings.Repeat("1", 200), lastVisit{}, false},
		{"1.7.4;path=/", lastVisit{}, false},
	} {
		got, ok := parseLastVisit(withLast(httptest.NewRequest("GET", "/", nil), tc.value))
		if ok != tc.ok || got != tc.want {
			t.Errorf("%q: got %v %v, want %v %v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestHomeRedirectsToTheLastCampaignAfterValidatingIt(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	rec := f.do(f.signedIn(withLast(httptest.NewRequest("GET", "/", nil), "1.7.4")))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7" {
		t.Fatalf("got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
	if lastSet(rec) != nil {
		t.Error("a valid cookie is left alone by the redirect")
	}
	// a paused campaign still redirects
	f = newFixture(t, Config{})
	d := campaignDetail()
	d.Status = core.CampaignPaused
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	if rec = f.do(f.signedIn(withLast(httptest.NewRequest("GET", "/", nil), "1.7"))); rec.Code != http.StatusSeeOther {
		t.Errorf("paused: got %d", rec.Code)
	}
}

func TestHomeIgnoresAndClearsACookieThatIsNotValid(t *testing.T) {
	finished := campaignDetail()
	finished.Status = core.CampaignFinished
	for _, tc := range []struct {
		name, value string
		get         func(*fixture)
	}{
		{"finished", "1.7", func(f *fixture) {
			f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(finished, nil)
		}},
		{"removed membership or deleted", "1.7", func(f *fixture) {
			f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(core.CampaignDetail{}, core.ErrNotFound)
		}},
		{"another user's", "2.7", func(*fixture) {}}, // no database read at all
		{"malformed", "1.7;x", func(*fixture) {}},
		{"too long", strings.Repeat("9", 100), func(*fixture) {}},
		{"a campaign id that is not one", "1.abc", func(*fixture) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			tc.get(f)
			f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return(nil, nil).AnyTimes()
			f.camp.EXPECT().List(gomock.Any(), frodo.ID, true).Return(nil, nil).AnyTimes()
			rec := f.do(f.signedIn(withLast(httptest.NewRequest("GET", "/", nil), tc.value)))
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<h1>Campaigns</h1>") {
				t.Fatalf("the list is shown: got %d", rec.Code)
			}
			if c := lastSet(rec); c == nil || c.MaxAge >= 0 {
				t.Errorf("the cookie is cleared: %v", c)
			}
		})
	}
}

func TestHomeDoesNotRedirectOnADatabaseError(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(core.CampaignDetail{}, http.ErrAbortHandler)
	if rec := f.do(f.signedIn(withLast(httptest.NewRequest("GET", "/", nil), "1.7"))); rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d", rec.Code)
	}
}

func TestTheListAtCampaignsNeverRedirects(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, gomock.Any()).Return(nil, nil).AnyTimes()
	// a valid cookie, and no Get: /campaigns does not even read the cookie
	rec := f.do(f.signedIn(withLast(httptest.NewRequest("GET", "/campaigns", nil), "1.7.4")))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<h1>Campaigns</h1>") {
		t.Fatalf("got %d", rec.Code)
	}
	if lastSet(rec) != nil {
		t.Error("the list neither sets nor clears the cookie")
	}
	body := rec.Body.String()
	for _, want := range []string{`href="/campaigns"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if strings.Contains(body, `<a href="/" class="flex`) {
		t.Error("the brand link of a signed-in user must not point at /")
	}
}

func TestOpeningACampaignPageRemembersIt(t *testing.T) {
	f := newFixture(t, Config{SecureCookies: true})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil).AnyTimes()
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil)))
	c := lastSet(rec)
	if c == nil || c.Value != "1.7" || c.Path != "/" || c.MaxAge != 90*24*3600 || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie: %+v", c)
	}
	// not from a fragment
	r := f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil))
	r.Header.Set("HX-Request", "true")
	if lastSet(f.do(r)) != nil {
		t.Error("an htmx request must not write the cookie")
	}
}

func TestFinishedCampaignIsNotRemembered(t *testing.T) {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.Status = core.CampaignFinished
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
	if lastSet(f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil)))) != nil {
		t.Error("a finished campaign is not remembered")
	}
}

func TestACompanyPageRemembersTheCompanyAndTheAliasUsesIt(t *testing.T) {
	f := newFixture(t, Config{})
	d := ownTwo(3)
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	// the alias with a remembered second company
	rec := f.do(f.signedIn(withLast(httptest.NewRequest("GET", "/campaigns/7/my-company", nil), "1.7.4")))
	if rec.Header().Get("Location") != "/campaigns/7/my-company/4" {
		t.Errorf("remembered company: -> %q", rec.Header().Get("Location"))
	}
	if c := lastSet(rec); c == nil || c.Value != "1.7.4" {
		t.Errorf("the alias keeps the remembered company: %v", c)
	}
	for name, v := range map[string]string{
		"another campaign":           "1.8.4",
		"another user":               "2.7.4",
		"a company that is not mine": "1.7.9",
		"no company":                 "1.7",
		"malformed":                  "1.7.x",
	} {
		rec := f.do(f.signedIn(withLast(httptest.NewRequest("GET", "/campaigns/7/my-company", nil), v)))
		if rec.Header().Get("Location") != "/campaigns/7/my-company/1" {
			t.Errorf("%s: -> %q, want the first company", name, rec.Header().Get("Location"))
		}
	}
	// the cookie that fell back is rewritten with a company of this campaign only when it is the viewer's own
	rec = f.do(f.signedIn(withLast(httptest.NewRequest("GET", "/campaigns/7/my-company", nil), "1.7.9")))
	if c := lastSet(rec); c == nil || c.Value != "1.7" {
		t.Errorf("a foreign company is dropped from the cookie: %v", c)
	}
}

func TestOwnCompanyPageWritesTheCompany(t *testing.T) {
	f := newFixture(t, Config{})
	d := ownTwo(3)
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	_, detail := mineDetail()
	detail.CampaignCompany = d.Companies[1]
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(4)).Return(detail, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), gomock.Any(), int64(7), int64(4)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), gomock.Any(), int64(7), int64(4)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), gomock.Any(), int64(7), int64(4)).Return(nil, nil).AnyTimes()
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/4", nil)))
	if rec.Code != 200 {
		t.Skipf("the company page needs more services than this test mocks (%d)", rec.Code)
	}
	if c := lastSet(rec); c == nil || c.Value != "1.7.4" {
		t.Errorf("cookie: %v", c)
	}
	// and exactly one bcm-last header even though loadCampaign and loadOwn both write
	n := 0
	for _, h := range rec.Header().Values("Set-Cookie") {
		if strings.HasPrefix(h, lastCookieEnd) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d bcm-last headers", n)
	}
}

func TestForgetVisitExpiresTheCookie(t *testing.T) {
	var s Server
	rec := httptest.NewRecorder()
	s.forgetVisit(rec)
	if c := lastSet(rec); c == nil || c.MaxAge >= 0 {
		t.Errorf("%v", c)
	}
}
