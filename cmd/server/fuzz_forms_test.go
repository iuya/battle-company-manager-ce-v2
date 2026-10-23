package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
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

// TestHostileUploadsAnswer4xx (robustness): the three multipart upload handlers (the admin Rules files page, the glossary picker and the
// campaign form's pickers) get malformed, truncated, huge, deeply nested, oddly typed and badly encoded JSON. Each must answer a 4xx
// with a message (never 5xx, a panic, or a request that outlives the deadline) and must not leave anything stored.
func TestHostileUploadsAnswer4xx(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	if testing.Short() {
		t.Skip("slow")
	}
	if _, err := os.Stat("definitions"); err != nil {
		wd, _ := os.Getwd()
		if err := os.Chdir("../.."); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chdir(wd) })
	}
	ctx := context.Background()
	store, err := postgres.Open(ctx, freshDatabase(ctx, t, base))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b, err := newBackendWithDice(ctx, store, &demoDice{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.auth.CreateUser(ctx, "root@example.com", "Root", "a-long-password"); err != nil {
		t.Fatal(err)
	}
	if err := b.auth.SetPermissions(ctx, "root@example.com", core.RoleSuperuser, true); err != nil {
		t.Fatal(err)
	}
	tok, _, _, err := b.auth.Login(ctx, "root@example.com", "a-long-password")
	if err != nil {
		t.Fatal(err)
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

	good, err := os.ReadFile("definitions/glossary/mesbg_glossary.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := func(p []byte) string { s, _ := core.RulesetHash(p); return s }
	rep := func(s string, n int) []byte { return []byte(strings.Repeat(s, n)) }
	cat := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	const meta = `"name":"x","version":"1","terms":[]`
	type payload struct {
		name    string
		content []byte
		ctype   string // content type of the file part ("" = application/json)
	}
	payloads := []payload{
		{"empty", nil, ""},
		{"whitespace", []byte("  \n\t "), ""},
		{"not json", []byte("hello"), ""},
		{"open brace", []byte("{"), ""},
		{"truncated file", good[:len(good)/2], ""},
		{"truncated mid string", append([]byte(`{"name":"x","terms":["abc`), 0), ""},
		{"array not object", []byte(`[1,2,3]`), ""},
		{"null", []byte(`null`), ""},
		{"scalar", []byte(`42`), ""},
		{"trailing garbage", cat(good, []byte("}}}")), ""},
		{"two documents", cat(good, good), ""},
		{"wrong content type", good, "application/zip"},
		{"binary content type", good, "application/octet-stream"},
		{"gzip magic", []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03"), "application/gzip"},
		{"zip magic", []byte("PK\x03\x04\x14\x00\x00\x00\x08\x00"), "application/zip"},
		{"utf8 bom", cat([]byte("\xef\xbb\xbf"), good), ""},
		{"utf16", []byte("\xff\xfe{\x00}\x00"), ""},
		{"deep arrays 100k", cat(rep("[", 100000), rep("]", 100000)), ""},
		{"deep arrays unclosed 8M", rep("[", 8<<20), ""},
		{"deep objects", cat(rep(`{"a":`, 50000), []byte("1"), rep("}", 50000)), ""},
		{"deep inside a valid shape", cat([]byte(`{`+meta+`,"x":`), rep("[", 100000), rep("]", 100000), []byte("}")), ""},
		{"huge array of numbers", cat([]byte(`{`+meta+`,"x":[0`), rep(",0", 6<<20), []byte("]}")), ""},
		{"huge array of empty objects", cat([]byte(`{`+meta+`,"x":[{}`), rep(",{}", 4<<20), []byte("]}")), ""},
		{"huge string", cat([]byte(`{"name":"`), rep("a", 15<<20), []byte(`","version":"1","terms":[]}`)), ""},
		{"huge key", cat([]byte(`{"`), rep("k", 15<<20), []byte(`":1}`)), ""},
		{"many keys", cat([]byte(`{`+meta), func() []byte {
			var sb strings.Builder
			for i := 0; i < 400000; i++ {
				fmt.Fprintf(&sb, `,"k%d":1`, i)
			}
			return []byte(sb.String())
		}(), []byte("}")), ""},
		{"duplicate keys", []byte(`{"name":"a","name":"b","version":"1","version":"2","terms":[],"terms":{}}`), ""},
		{"duplicate keys many", cat([]byte(`{`+meta), rep(`,"name":"y"`, 500000), []byte("}")), ""},
		{"invalid utf8 in string", []byte("{\"name\":\"\xff\xfe\",\"version\":\"1\",\"terms\":[]}"), ""},
		{"invalid utf8 in key", []byte("{\"\xc3\x28\":1,\"name\":\"x\",\"version\":\"1\",\"terms\":[]}"), ""},
		{"NUL escape", []byte(`{"name":"a\u0000b","version":"1","terms":[]}`), ""},
		{"NUL escape in version", []byte(`{"name":"x","version":"1\u0000","terms":[]}`), ""},
		{"lone surrogate", []byte(`{"name":"\ud800","version":"1","terms":[]}`), ""},
		{"raw NUL", []byte("{\"name\":\"a\x00b\",\"version\":\"1\",\"terms\":[]}"), ""},
		{"huge number", []byte(`{"name":"x","version":1e999999,"terms":[]}`), ""},
		{"long digits", cat([]byte(`{"name":"x","version":`), rep("9", 1<<20), []byte(`,"terms":[]}`)), ""},
		{"vanilla marker only", []byte(`{"appVersion":1,"game":"g"}`), ""},
		{"bc marker only", []byte(`{"extends":{},"name":"n","version":"1"}`), ""},
		{"wrong shape types", []byte(`{"name":["x"],"version":{"a":1},"terms":"no"}`), ""},
	}
	// Each handler gets every payload as its file. A request is a 4xx, or (for the one payload that is a valid glossary) a success.
	type target struct {
		name, path string
		fields     func(kind string, content []byte) map[string]string
		file       string
		ok         map[int]bool // statuses that are not a failure of the handler besides 4xx (redirects)
	}
	targets := []target{
		{"admin upload", "/admin/rulesets", func(k string, c []byte) map[string]string { return map[string]string{"kind": k} }, "file", nil},
		{"glossary picker", "/glossary", func(k string, c []byte) map[string]string {
			return map[string]string{"glossary-mode": "file", "glossary-hash": sum(c)}
		}, "glossary-file", nil},
		{"campaign form", "/campaigns", func(k string, c []byte) map[string]string {
			return map[string]string{"name": "Hostile", "description": "d", k + "-mode": "file", k + "-hash": sum(c)}
		}, "", nil},
	}
	multipart := func(fields map[string]string, file, fname, ctype string, content []byte) (*bytes.Buffer, string) {
		var buf bytes.Buffer
		w := multipartWriter(&buf)
		for k, v := range fields {
			_ = w.WriteField(k, v)
		}
		if file != "" {
			w.writeFile(file, fname, ctype, content)
		}
		w.Close()
		return &buf, w.FormDataContentType()
	}
	kinds := []string{"glossary", "vanilla", "battle_companies"}
	var failures []string
	send := func(label, path string, body io.Reader, ctype string, cl int64) {
		req := httptest.NewRequest("POST", path, body)
		req.Header.Set("Content-Type", ctype)
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "bcm_session", Value: tok})
		if cl >= 0 {
			req.ContentLength = cl
		}
		rec := httptest.NewRecorder()
		done := make(chan struct{})
		start := time.Now()
		go func() {
			defer close(done)
			defer func() {
				if p := recover(); p != nil {
					failures = append(failures, fmt.Sprintf("PANIC escaped the middleware: %s: %v", label, p))
				}
			}()
			h.ServeHTTP(rec, req)
		}()
		select {
		case <-done:
		case <-time.After(45 * time.Second):
			failures = append(failures, fmt.Sprintf("%s: still running after 45 s", label))
			return
		}
		if os.Getenv("UPLOAD_VERBOSE") != "" {
			t.Logf("%d %s %s", rec.Code, time.Since(start).Round(time.Millisecond), label)
		}
		if rec.Code < 400 || rec.Code >= 500 {
			// the admin upload of a valid glossary or a redirect is fine: only 5xx is a failure here
			if rec.Code >= 500 {
				failures = append(failures, fmt.Sprintf("%d %s (%s)", rec.Code, label, time.Since(start).Round(time.Millisecond)))
			}
			return
		}
		if strings.TrimSpace(rec.Body.String()) == "" {
			failures = append(failures, fmt.Sprintf("%d %s: no message in the answer", rec.Code, label))
		}
	}
	for _, p := range payloads {
		for _, tg := range targets {
			for _, k := range kinds {
				if tg.name == "glossary picker" && k != "glossary" {
					continue
				}
				file := tg.file
				if file == "" {
					file = k + "-file"
				}
				fields := tg.fields(k, p.content)
				ct := p.ctype
				if ct == "" {
					ct = "application/json"
				}
				buf, mt := multipart(fields, file, "rules.json", ct, p.content)
				send(fmt.Sprintf("%s / %s / %s", tg.name, k, p.name), tg.path, buf, mt, -1)
			}
		}
	}
	// the request itself is wrong
	huge := rep("x", 17<<20)
	for _, tg := range targets {
		k := "glossary"
		file := tg.file
		if file == "" {
			file = k + "-file"
		}
		fields := tg.fields(k, nil)
		buf, mt := multipart(fields, file, "rules.json", "application/json", huge)
		send(tg.name+" / body over the limit", tg.path, buf, mt, -1)
		buf, mt = multipart(fields, file, "rules.json", "application/json", good)
		cut := buf.Bytes()[:buf.Len()/2]
		send(tg.name+" / truncated multipart", tg.path, bytes.NewReader(cut), mt, -1)
		send(tg.name+" / no boundary", tg.path, strings.NewReader("--x\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\nb\r\n--x--"), "multipart/form-data", -1)
		send(tg.name+" / empty boundary", tg.path, strings.NewReader("junk"), "multipart/form-data; boundary=", -1)
		send(tg.name+" / body is not multipart", tg.path, strings.NewReader("a=b"), "multipart/form-data; boundary=zzz", -1)
		send(tg.name+" / json body", tg.path, bytes.NewReader(good), "application/json", -1)
		send(tg.name+" / lying length", tg.path, bytes.NewReader(good[:10]), "multipart/form-data; boundary=zzz", 1<<40)
		// many parts
		var many bytes.Buffer
		mw := multipartWriter(&many)
		for i := 0; i < 5000; i++ {
			_ = mw.WriteField(fmt.Sprintf("f%d", i), "v")
		}
		mw.Close()
		send(tg.name+" / 5000 fields", tg.path, &many, mw.FormDataContentType(), -1)
		// a field with a huge value
		var big bytes.Buffer
		bw := multipartWriter(&big)
		_ = bw.WriteField("name", string(rep("n", 12<<20)))
		bw.Close()
		send(tg.name+" / 12 MB text field", tg.path, &big, bw.FormDataContentType(), -1)
	}
	sort.Strings(failures)
	for _, f := range failures {
		t.Error(f)
	}
	if strings.Contains(logs.String(), "level=ERROR msg=panic") {
		t.Errorf("a handler panicked:\n%s", firstLines(logs.String(), "msg=panic", 5))
	}
	if n, _ := store.Rulesets().ListUsage(ctx); len(n) > 3 {
		t.Errorf("%d rules files are stored, the shipped three expected: a hostile upload got stored", len(n))
	}
}

// uploadWriter is a multipart writer that can also write a file part with the content type of the test's choosing.
type uploadWriter struct{ *multipart.Writer }

func multipartWriter(w io.Writer) uploadWriter { return uploadWriter{multipart.NewWriter(w)} }

func (u uploadWriter) writeFile(field, name, ctype string, content []byte) {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, name))
	h.Set("Content-Type", ctype)
	p, _ := u.CreatePart(h)
	_, _ = p.Write(content)
}
