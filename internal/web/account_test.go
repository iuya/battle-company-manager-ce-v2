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

func accountPost(f *fixture, path string, v url.Values) (int, string, http.Header) {
	r := f.signedIn(form(path, v))
	r.Header.Set("HX-Request", "true")
	rec := f.do(r)
	return rec.Code, rec.Body.String(), rec.Header()
}

func TestAccountPageShowsTheCards(t *testing.T) {
	f := newFixture(t, Config{})
	body := f.do(f.signedIn(httpGet("/account"))).Body.String()
	for _, want := range []string{"Profile", "Password", "Sessions", "Close account", `value="Frodo"`, `value="frodo@shire.me" readonly`, `for="acc-current"`, `for="close-email"`, `<details `} {
		if !strings.Contains(body, want) {
			t.Errorf("account page lacks %q", want)
		}
	}
	if strings.Contains(body, "<details open") {
		t.Error("the close-account card starts collapsed")
	}
}

func TestAccountProfile(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().UpdateProfile(gomock.Any(), frodo.ID, "").Return(core.User{}, errors.Join(core.ErrInvalidUser, errors.New("the name must be between 1 and 60 characters")))
	code, body, _ := accountPost(f, "/account/profile", url.Values{"name": {""}})
	if code != 422 || !strings.Contains(body, "between 1 and 60") || !strings.Contains(body, `aria-invalid="true"`) {
		t.Errorf("empty name: %d\n%s", code, body)
	}
	f.auth.EXPECT().UpdateProfile(gomock.Any(), frodo.ID, "Mr Baggins").Return(core.User{ID: 1, Email: frodo.Email, DisplayName: "Mr Baggins"}, nil)
	code, body, _ = accountPost(f, "/account/profile", url.Values{"name": {"Mr Baggins"}})
	if code != 200 || !strings.Contains(body, "Your name is saved.") || !strings.Contains(body, `value="Mr Baggins"`) {
		t.Errorf("success: %d\n%s", code, body)
	}
}

func TestAccountPassword(t *testing.T) {
	f := newFixture(t, Config{})
	good := url.Values{"current": {"old-password"}, "new": {"a-long-new-password"}, "confirm": {"a-long-new-password"}}
	with := func(k, v string) url.Values {
		c := url.Values{}
		for key, val := range good {
			c[key] = val
		}
		c.Set(k, v)
		return c
	}
	// validation never reaches the core
	for name, v := range map[string]url.Values{"short": with("new", "short"), "mismatch": with("confirm", "other-password-1"), "no current": with("current", "")} {
		if code, body, _ := accountPost(f, "/account/password", v); code != 422 || !strings.Contains(body, `role="alert"`) {
			t.Errorf("%s: %d\n%s", name, code, body)
		}
	}
	f.auth.EXPECT().ChangePassword(gomock.Any(), frodo.ID, "old-password", "a-long-new-password", "tok").Return(core.ErrInvalidCredentials)
	code, body, _ := accountPost(f, "/account/password", good)
	if code != 422 || !strings.Contains(body, "not your current password") || strings.Contains(body, "a-long-new-password") {
		t.Errorf("wrong current password: %d\n%s", code, body)
	}
	f.auth.EXPECT().ChangePassword(gomock.Any(), frodo.ID, "old-password", "a-long-new-password", "tok").Return(nil)
	code, body, _ = accountPost(f, "/account/password", good)
	if code != 200 || !strings.Contains(body, "Every other device was signed out") {
		t.Errorf("success: %d\n%s", code, body)
	}
}

func TestAccountPasswordThrottle(t *testing.T) {
	f := newFixture(t, Config{})
	v := url.Values{"current": {"wrong"}, "new": {"a-long-new-password"}, "confirm": {"a-long-new-password"}}
	f.auth.EXPECT().ChangePassword(gomock.Any(), frodo.ID, "wrong", gomock.Any(), "tok").Return(core.ErrInvalidCredentials).Times(maxFailsPerAccount)
	for range maxFailsPerAccount {
		accountPost(f, "/account/password", v)
	}
	if code, body, _ := accountPost(f, "/account/password", v); code != http.StatusTooManyRequests || !strings.Contains(body, "Too many") {
		t.Errorf("after %d failures: %d\n%s", maxFailsPerAccount, code, body)
	}
}

func TestAccountSignOutOthers(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().SignOutOthers(gomock.Any(), frodo.ID, "tok").Return(3, nil)
	if code, body, _ := accountPost(f, "/account/sessions/sign-out-others", nil); code != 200 || !strings.Contains(body, "Signed out of 3 other sessions.") {
		t.Errorf("%d\n%s", code, body)
	}
	f.auth.EXPECT().SignOutOthers(gomock.Any(), frodo.ID, "tok").Return(0, nil)
	if _, body, _ := accountPost(f, "/account/sessions/sign-out-others", nil); !strings.Contains(body, "not signed in anywhere else") {
		t.Errorf("none: %s", body)
	}
}

func TestAccountClose(t *testing.T) {
	f := newFixture(t, Config{})
	// the confirmation needs both fields, and the typed email must be the account's
	for name, v := range map[string]url.Values{
		"no password": {"email": {frodo.Email}},
		"no email":    {"password": {"pw"}},
		"wrong email": {"email": {"sam@shire.me"}, "password": {"pw"}},
	} {
		code, body, _ := accountPost(f, "/account/close", v)
		if code != 422 || !strings.Contains(body, "<details open") {
			t.Errorf("%s: %d\n%s", name, code, body)
		}
	}
	f.auth.EXPECT().CloseAccount(gomock.Any(), frodo.ID, "pw").Return(core.ErrInvalidCredentials)
	if code, body, _ := accountPost(f, "/account/close", url.Values{"email": {"FRODO@shire.me"}, "password": {"pw"}}); code != 422 || !strings.Contains(body, "not your password") {
		t.Errorf("wrong password: %d\n%s", code, body)
	}
	f.auth.EXPECT().CloseAccount(gomock.Any(), frodo.ID, "pw").Return(core.Refuse("Hand over your campaigns (on the campaign overview) or finish them first"))
	if code, body, _ := accountPost(f, "/account/close", url.Values{"email": {frodo.Email}, "password": {"pw"}}); code != 422 || !strings.Contains(body, "Hand over your campaigns (on the campaign overview) or finish them first") {
		t.Errorf("refused: %d\n%s", code, body)
	}
	f.auth.EXPECT().CloseAccount(gomock.Any(), frodo.ID, "pw").Return(nil)
	code, _, h := accountPost(f, "/account/close", url.Values{"email": {frodo.Email}, "password": {"pw"}})
	if code != http.StatusNoContent || h.Get("HX-Redirect") != "/login?notice=closed" || !strings.Contains(h.Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("success: %d %v", code, h)
	}
	if body := f.do(httpGet("/login?notice=closed")).Body.String(); !strings.Contains(body, "Your account is closed") {
		t.Errorf("the login page lacks the notice:\n%s", body)
	}
}

func httpGet(path string) *http.Request { return httptest.NewRequest("GET", path, nil) }

func appearancePut(f *fixture, signedIn bool, v url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest("PUT", "/account/appearance", strings.NewReader(v.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if signedIn {
		r = f.signedIn(r)
	}
	return f.do(r)
}

func TestAccountAppearance(t *testing.T) {
	f := newFixture(t, Config{})
	if rec := appearancePut(f, false, url.Values{"palette": {"ember"}, "mode": {"dark"}}); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", rec.Code)
	}
	f.auth.EXPECT().SetAppearance(gomock.Any(), frodo.ID, "ember", "dark").Return(nil)
	if rec := appearancePut(f, true, url.Values{"palette": {"ember"}, "mode": {"dark"}}); rec.Code != http.StatusNoContent {
		t.Errorf("saved: %d", rec.Code)
	}
	f.auth.EXPECT().SetAppearance(gomock.Any(), frodo.ID, "neon", "dark").Return(errors.Join(core.ErrInvalidUser, errors.New("unknown palette or mode")))
	if rec := appearancePut(f, true, url.Values{"palette": {"neon"}, "mode": {"dark"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown key: %d", rec.Code)
	}
}

func TestLayoutCarriesTheSavedAppearance(t *testing.T) {
	f := newFixture(t, Config{})
	u := frodo
	u.Appearance = core.Appearance{Palette: "mithril", Mode: "dark"}
	body := f.do(f.signedInAs(u, httpGet("/account"))).Body.String()
	for _, want := range []string{`data-auth="1" data-pref="mithril.dark"`, `id="account-appearance"`, `aria-label="Palette"`} {
		if !strings.Contains(body, want) {
			t.Errorf("signed-in page lacks %q", want)
		}
	}
	f = newFixture(t, Config{})
	if body := f.do(f.signedIn(httpGet("/account"))).Body.String(); strings.Contains(body, `data-pref="`) || !strings.Contains(body, `data-auth="1"`) {
		t.Error("a user who never chose has no data-pref, but is marked signed in")
	}
	if body := f.do(httpGet("/login")).Body.String(); strings.Contains(body, `data-auth="1"`) {
		t.Error("anonymous visitors are not marked signed in")
	}
}

func TestLoginAdoptsOrWritesTheAppearance(t *testing.T) {
	login := func(f *fixture, cookie string) *httptest.ResponseRecorder {
		r := form("/login", url.Values{"email": {"frodo@shire.me"}, "password": {"pw"}})
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: themeCookie, Value: cookie})
		}
		return f.do(r)
	}
	// never chose: the browser's cookie is saved to the account
	f := newFixture(t, Config{})
	f.auth.EXPECT().Login(gomock.Any(), "frodo@shire.me", "pw").Return("tok", time.Now().Add(time.Hour), frodo, nil)
	f.auth.EXPECT().SetAppearance(gomock.Any(), frodo.ID, "ember", "light").Return(nil)
	login(f, "ember.light")
	// chose on another device: the saved look is written to the cookie
	f = newFixture(t, Config{})
	u := frodo
	u.Appearance = core.Appearance{Palette: "mithril", Mode: "system"}
	f.auth.EXPECT().Login(gomock.Any(), "frodo@shire.me", "pw").Return("tok", time.Now().Add(time.Hour), u, nil)
	rec := login(f, "ember.light")
	if got := strings.Join(rec.Header().Values("Set-Cookie"), "\n"); !strings.Contains(got, "bcm-theme=mithril.system") {
		t.Errorf("cookies: %s", got)
	}
	// a bad cookie is ignored
	f = newFixture(t, Config{})
	f.auth.EXPECT().Login(gomock.Any(), "frodo@shire.me", "pw").Return("tok", time.Now().Add(time.Hour), frodo, nil)
	login(f, "neon.dark")
}

func TestClientIPOnlyTrustsTheProxyWhenAsked(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = "10.0.0.1:5555"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")
	if got := (&Server{}).clientIP(r); got != "10.0.0.1" {
		t.Errorf("without a trusted proxy the header can be forged: %q", got)
	}
	if got := (&Server{cfg: Config{TrustProxy: true}}).clientIP(r); got != "203.0.113.9" {
		t.Errorf("behind a proxy the last entry is the client: %q", got)
	}
	r.Header.Del("X-Forwarded-For")
	if got := (&Server{cfg: Config{TrustProxy: true}}).clientIP(r); got != "10.0.0.1" {
		t.Errorf("no header: %q", got)
	}
}
