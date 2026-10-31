package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The write routes and their guards. The routes are read from server.go, so a new POST/PUT/DELETE route is covered
// without editing this file: it must be wrapped in s.protected (signed-in accounts only) or s.requireSuperuser, or be listed in
// publicWrites with the reason it is open. The other layers of the matrix (campaign roles and states) are tested against the real
// services in cmd/server (TestPermissionMatrix); the web layer only checks the role of the account here.

// publicWrites are the write routes that anonymous visitors may reach.
var publicWrites = map[string]string{
	"POST /csp-report":        "browsers report violations without a session; rate limited",
	"POST /login":             "signing in",
	"POST /logout":            "signing out of whatever session there is",
	"PUT /account/appearance": "the theme of a signed-out visitor is kept in a cookie",
	"POST /register/{token}":  "registering with an account invitation; the token is the credential, throttled",
}

var routeRe = regexp.MustCompile(`mux\.HandleFunc\("((?:POST|PUT|DELETE|PATCH) [^"]+)", (s\.\w+)\(`)

type writeRoute struct{ pattern, guard string }

func writeRoutes(t *testing.T) []writeRoute {
	t.Helper()
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []writeRoute
	for _, m := range routeRe.FindAllStringSubmatch(string(src), -1) {
		out = append(out, writeRoute{m[1], m[2]})
	}
	if len(out) < 50 {
		t.Fatalf("found only %d write routes in server.go: did the route table change shape?", len(out))
	}
	return out
}

var placeholder = regexp.MustCompile(`\{[^}]+\}`)

func (r writeRoute) request() *http.Request {
	method, path, _ := strings.Cut(r.pattern, " ")
	return httptest.NewRequest(method, placeholder.ReplaceAllString(path, "1"), strings.NewReader(""))
}

func TestAnonymousWritesRedirectToLogin(t *testing.T) {
	f := newFixture(t, Config{})
	for _, r := range writeRoutes(t) {
		if _, open := publicWrites[r.pattern]; open {
			continue
		}
		if r.guard == "s.session" {
			t.Errorf("%s only reads the session: wrap it in s.protected or list it in publicWrites with the reason", r.pattern)
			continue
		}
		req := r.request()
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := f.do(req)
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
			t.Errorf("anonymous %s: got %d -> %q, want a redirect to /login", r.pattern, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestOnlyAdministratorsReachTheAdministrationWrites(t *testing.T) {
	f := newFixture(t, Config{})
	checked := 0
	for _, r := range writeRoutes(t) {
		isAdminPath := strings.Contains(r.pattern, " /admin/")
		isAdminGuard := r.guard == "s.requireSuperuser" || r.guard == "s.requireAdmin"
		if isAdminPath != isAdminGuard {
			t.Errorf("%s is guarded by %s: /admin/ routes and s.requireAdmin or s.requireSuperuser go together", r.pattern, r.guard)
		}
		if !isAdminGuard {
			continue
		}
		roles := []core.Role{core.RoleUser, core.RoleAdmin}
		if r.guard == "s.requireAdmin" {
			roles = roles[:1] // administrators reach these (the service decides each cell); the guard keeps everybody else out
		}
		for _, role := range roles {
			user := frodo
			user.Role = role
			f.auth.EXPECT().Authenticate(gomock.Any(), "tok-"+string(role)).Return(user, time.Time{}, nil).AnyTimes()
			req := r.request()
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok-" + string(role)})
			if rec := f.do(req); rec.Code != http.StatusNotFound {
				t.Errorf("%s as %s: got %d, want 404", r.pattern, role, rec.Code)
			}
		}
		checked++
	}
	if checked < 5 {
		t.Errorf("only %d administration writes found", checked)
	}
}
