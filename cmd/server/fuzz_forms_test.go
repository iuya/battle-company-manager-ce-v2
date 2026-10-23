package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
	"github.com/iuya/battle-company-manager-ce-v2/internal/web"
)

// TestMalformedInputNeverCrashesAHandler (robustness): every route of the server (read from internal/web/server.go, so a new one is
// covered without editing this file) is sent, as a superuser, over the real services and the loaded demo world, with every form field
// the handlers and templates know set to a series of hostile values (empty, huge, negative, overflowing, NaN, NUL, invalid UTF-8,
// markup, SQL) and with hostile path ids. No request may answer 5xx or log a panic: bad input is a 4xx or a redirect with a notice.
func TestMalformedInputNeverCrashesAHandler(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	if testing.Short() {
		t.Skip("slow")
	}
	if _, err := os.Stat("definitions"); err != nil { // the rules are read relative to the repository root; put the directory back after
		wd, _ := os.Getwd()
		if err := os.Chdir("../.."); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(wd) })
	}
	ctx := context.Background()
	dsn := freshDatabase(ctx, t, base)
	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dice := &demoDice{}
	b, err := newBackendWithDice(ctx, store, dice)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	exec := func(ctx context.Context, q string, args ...any) error {
		_, err := conn.Exec(ctx, q, args...)
		return err
	}
	var out bytes.Buffer
	if err := seedWorld(ctx, b, dice, "demo-password", &out, exec); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}

	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	srv, err := web.NewServer(web.Config{}, b.campaigns, b.rulesets, b.glossaries, b.auth)
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	// real ids for the path placeholders
	id := func(q string) string {
		var n any
		if err := conn.QueryRow(ctx, q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return fmt.Sprint(n)
	}
	camp := id(`SELECT id FROM campaigns WHERE name LIKE 'Matches:%'`)
	real := map[string]string{
		"id":      camp,
		"match":   id(`SELECT min(id) FROM matches WHERE campaign_id = ` + camp),
		"company": id(`SELECT min(id) FROM companies WHERE campaign_id = ` + camp),
		"unit":    id(`SELECT min(id) FROM company_units`),
		"entry":   id(`SELECT coalesce(min(id), 1) FROM match_live_log`),
		"hash":    id(`SELECT min(hash) FROM rulesets`),
		"token":   "x", "kind": "stat", "perk": "x",
	}

	login := func() *http.Cookie {
		tok, _, _, err := b.auth.Login(ctx, "admin@example.com", "demo-password")
		if err != nil {
			t.Fatal(err)
		}
		return &http.Cookie{Name: "bcm_session", Value: tok}
	}
	cookie := login()

	routes := routesOf(t)
	fields := fieldNames(t)
	hostile := []string{"", " ", "0", "-1", "99999999999999999999", "9223372036854775807", "-9223372036854775808", "1e999", "NaN", "abc",
		"1.5", "\x00", "\xff\xfe\xfd", "<script>alert(1)</script>", "'; DROP TABLE users; --", "%", "[]", "a,,b,", "true",
		strings.Repeat("a", 70000), strings.Repeat("9", 400), "../../etc/passwd", "ß‮\u0000"}

	if os.Getenv("SWEEP_FULL") == "" { // the default run keeps the values that found bugs; SWEEP_FULL=1 tries them all
		hostile = []string{"", "0", "-1", "9223372036854775807", "abc", "\x00", "\xff\xfe\xfd", strings.Repeat("a", 70000)}
	}
	var failures []string
	var requests int
	send := func(method, pattern, path string, form url.Values) {
		var body io.Reader
		target := path
		if method == "GET" {
			if len(form) > 0 {
				target += "?" + form.Encode()
			}
		} else {
			body = strings.NewReader(form.Encode())
		}
		req := httptest.NewRequest(method, target, body)
		if method != "GET" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.Header.Set("HX-Request", "true")
		req.AddCookie(cookie)
		if strings.HasSuffix(pattern, "/events") { // the live stream never ends by itself
			c, cancel := context.WithTimeout(req.Context(), 300*time.Millisecond)
			defer cancel()
			req = req.WithContext(c)
		}
		rec := httptest.NewRecorder()
		func() {
			defer func() {
				if p := recover(); p != nil {
					failures = append(failures, fmt.Sprintf("PANIC escaped the middleware: %s %s: %v", method, pattern, p))
				}
			}()
			h.ServeHTTP(rec, req)
		}()
		requests++
		if rec.Code == http.StatusSeeOther && strings.HasPrefix(rec.Header().Get("Location"), "/login") {
			cookie = login() // a route ended the session (sign out, close account): go on with a new one
		}
		if rec.Code >= 500 {
			v := form.Encode()
			if len(v) > 80 {
				v = v[:80] + "..."
			}
			failures = append(failures, fmt.Sprintf("%d %s %s <- %s", rec.Code, method, path, v))
		}
	}
	for _, rt := range routes {
		if rt.pattern == "/logout" || strings.Contains(rt.pattern, "/account/close") {
			continue
		}
		path := placeholder.ReplaceAllStringFunc(rt.pattern, func(p string) string { return real[p[1:len(p)-1]] })
		allFields := func(v string) url.Values {
			f := url.Values{}
			for i, n := range fields {
				if len(v) > 1000 && i%20 != 0 { // a few long fields, not megabytes per request
					continue
				}
				f.Set(n, v)
			}
			return f
		}
		send(rt.method, rt.pattern, path, nil)
		for _, v := range hostile {
			send(rt.method, rt.pattern, path, allFields(v))
		}
		// hostile path ids, one placeholder at a time
		for _, ph := range placeholder.FindAllString(rt.pattern, -1) {
			for _, v := range []string{"0", "-1", "99999999999999999999", "abc"} {
				p := strings.Replace(rt.pattern, ph, v, 1)
				p = placeholder.ReplaceAllStringFunc(p, func(q string) string { return real[q[1:len(q)-1]] })
				send(rt.method, rt.pattern, p, nil)
			}
		}
	}
	sort.Strings(failures)
	failures = dedupe(failures)
	for _, f := range failures {
		t.Error(f)
	}
	if strings.Contains(logs.String(), "level=ERROR msg=panic") {
		t.Errorf("a handler panicked (recovered by the middleware); first lines:\n%s", firstLines(logs.String(), "msg=panic", 5))
	}
	seenErr := map[string]bool{}
	for _, l := range strings.Split(logs.String(), "\n") {
		if strings.Contains(l, "level=ERROR") {
			if i := strings.Index(l, "stack="); i > 0 {
				l = l[:i]
			}
			if len(l) > 300 {
				l = l[:300]
			}
			if !seenErr[l] {
				seenErr[l] = true
				t.Log(l)
			}
		}
	}
	t.Logf("%d routes, %d requests", len(routes), requests)
}

type route struct{ method, pattern string }

var placeholder = regexp.MustCompile(`\{[^}$]+\}`)

// routesOf reads the routes registered in internal/web/server.go (the "{$}" exact-match marker is stripped).
func routesOf(t *testing.T) []route {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("internal", "web", "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	var out []route
	for _, m := range regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) ([^"]+)"`).FindAllStringSubmatch(string(src), -1) {
		out = append(out, route{m[1], strings.ReplaceAll(m[2], "{$}", "")})
	}
	if len(out) < 100 {
		t.Fatalf("found %d routes in server.go: did the route table change shape?", len(out))
	}
	return out
}

// fieldNames collects the form and query names the handlers read and the templates send.
func fieldNames(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	re := []*regexp.Regexp{
		regexp.MustCompile(`(?:FormValue|PostFormValue|Query\(\)\.Get)\("([A-Za-z0-9_\-\[\].]+)"\)`),
		regexp.MustCompile(`Form\["([A-Za-z0-9_\-\[\].]+)"\]`),
		regexp.MustCompile(`\bname="([A-Za-z0-9_\-\[\].]+)"`),
	}
	for _, glob := range []string{"internal/web/*.go", "internal/web/templates/*.html", "internal/web/templates/*/*.html"} {
		files, _ := filepath.Glob(glob)
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range re {
				for _, m := range r.FindAllStringSubmatch(string(src), -1) {
					seen[m[1]] = true
				}
			}
		}
	}
	delete(seen, "Accept")
	var out []string
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	if len(out) < 40 {
		t.Fatalf("found only %d field names", len(out))
	}
	return out
}

func dedupe(in []string) []string {
	var out []string
	var last string
	for _, s := range in {
		key := s
		if i := strings.Index(s, " <- "); i > 0 {
			key = s[:i]
		}
		if key != last {
			out = append(out, s)
		}
		last = key
	}
	return out
}

func firstLines(s, contains string, n int) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, contains) {
			if len(l) > 400 {
				l = l[:400]
			}
			out = append(out, l)
			if len(out) == n {
				break
			}
		}
	}
	return strings.Join(out, "\n")
}
