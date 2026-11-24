package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var viewer = core.User{ID: 5, Email: "smoke@example.org", DisplayName: "Smoke", ReadOnly: true}

// A read-only account looks at everything and changes nothing: every write route answers 403 before its handler (and so any service) is
// reached. The routes are read from server.go, so a route added later is covered too.
func TestReadOnlyAccountCannotWrite(t *testing.T) {
	f := newFixture(t, Config{}) // no service call is expected: the refusal comes first
	n := 0
	for _, r := range writeRoutes(t) {
		if r.pattern == "POST /login" || r.pattern == "POST /logout" || r.pattern == "POST /csp-report" || r.pattern == "POST /recover/{token}" {
			continue // signing in and out are allowed; the last two never read a session
		}
		n++
		req := f.signedInAs(viewer, r.request())
		rec := f.do(req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s as a read-only account = %d, want 403", r.pattern, rec.Code)
		} else if !strings.Contains(rec.Body.String(), "read-only") {
			t.Errorf("%s: the refusal does not say why: %s", r.pattern, rec.Body)
		}
	}
	if n < 40 {
		t.Fatalf("only %d write routes were checked", n)
	}
}

func TestReadOnlyAccountCanReadAndLeave(t *testing.T) {
	f := newFixture(t, Config{})
	if rec := f.do(f.signedInAs(viewer, httpGet("/account"))); rec.Code != 200 {
		t.Errorf("GET /account = %d, want 200", rec.Code)
	}
	f.auth.EXPECT().Logout(gomock.Any(), "tok").Return(nil)
	if rec := f.do(f.signedInAs(viewer, form("/logout", nil))); rec.Code != http.StatusSeeOther {
		t.Errorf("POST /logout = %d, want 303", rec.Code)
	}
	// a browser that still holds the session may sign in as somebody else
	if rec := f.do(f.signedInAs(viewer, form("/login", url.Values{"email": {"frodo@shire.me"}, "password": {"wrong"}}))); rec.Code == http.StatusForbidden {
		t.Errorf("POST /login = 403: the read-only session must not stop a new sign-in")
	}
}

// Everybody else writes as before (the gate is the flag, not the method).
func TestReadOnlyGateLetsOtherAccountsThrough(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().Logout(gomock.Any(), "tok").Return(nil)
	if rec := f.do(f.signedIn(form("/logout", nil))); rec.Code != http.StatusSeeOther {
		t.Errorf("POST /logout = %d", rec.Code)
	}
	f.auth.EXPECT().SetAppearance(gomock.Any(), frodo.ID, gomock.Any(), gomock.Any()).Return(nil) // reached: the gate did not stop it
	req := f.signedIn(httptest.NewRequest("PUT", "/account/appearance", strings.NewReader("palette=mithril&mode=dark")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if rec := f.do(req); rec.Code == http.StatusForbidden {
		t.Errorf("a plain account was refused: %d %s", rec.Code, rec.Body)
	}
}

func TestReadRequest(t *testing.T) {
	for _, tt := range []struct {
		method, path string
		want         bool
	}{
		{"GET", "/campaigns", true}, {"HEAD", "/campaigns", true}, {"OPTIONS", "/campaigns", true},
		{"POST", "/login", true}, {"POST", "/logout", true},
		{"POST", "/campaigns", false}, {"PUT", "/account/appearance", false}, {"DELETE", "/x", false}, {"PATCH", "/x", false},
		{"POST", "/login/x", false}, {"PUT", "/login", false},
	} {
		if got := readRequest(httptest.NewRequest(tt.method, tt.path, nil)); got != tt.want {
			t.Errorf("%s %s = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}
