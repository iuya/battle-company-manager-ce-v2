package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
	"github.com/iuya/battle-company-manager-ce-v2/internal/web"
)

// BenchmarkPages times the whole server-side cost of the main pages (handlers, the services, the database, template rendering) of a loaded
// demo world, without a browser or a network: the tool for profiling a slow page (docs/performance.md). Point WORLD_DATABASE_URL at a
// scratch copy of the loaded world (the pages are only read; the sign-ins add session rows), for example
//
//	WORLD_DATABASE_URL=postgres://bcm:bcm@localhost:5432/bcm_perf?sslmode=disable go test ./cmd/server -run xxx -bench Pages -benchtime 20x -cpuprofile /tmp/cpu.out
//
// It reports the statements per request next to the time.
func BenchmarkPages(b *testing.B) {
	url := os.Getenv("WORLD_DATABASE_URL")
	if url == "" {
		b.Skip("WORLD_DATABASE_URL is not set")
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil))) // the request log would drown the results
	ctx := context.Background()
	counter := &queryCounter{}
	store, err := postgres.OpenWith(ctx, url, postgres.Options{Tracer: counter})
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	be, err := newBackendWithDice(ctx, store, &demoDice{})
	if err != nil {
		b.Fatal(err)
	}
	srv, err := web.NewServer(web.Config{}, be.campaigns, be.rulesets, be.glossaries, be.auth)
	if err != nil {
		b.Fatal(err)
	}
	h := srv.Handler()
	cookie := func(email string) *http.Cookie {
		tok, _, _, err := be.auth.Login(ctx, email, "demo-password")
		if err != nil {
			b.Fatal(err)
		}
		return &http.Cookie{Name: "bcm_session", Value: tok}
	}
	eowyn, admin := cookie("eowyn@example.com"), cookie("admin@example.com")
	get := func(c *http.Cookie, path string) (int, string) {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	conn, err := pgx.Connect(ctx, withoutPoolParams(url)) // the pool settings (pool_max_conns=...) are the pool's, not the server's
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close(ctx)
	var matchesCampaign, wardCampaign int64
	if err := conn.QueryRow(ctx, "SELECT id FROM campaigns WHERE name LIKE 'Matches:%'").Scan(&matchesCampaign); err != nil {
		b.Fatal(err)
	}
	if err := conn.QueryRow(ctx, "SELECT id FROM campaigns WHERE name LIKE 'Aftermath ward 1%'").Scan(&wardCampaign); err != nil {
		b.Fatal(err)
	}
	_, list := get(admin, fmt.Sprintf("/campaigns/%d/matches", matchesCampaign))
	matchLinks := regexp.MustCompile(fmt.Sprintf(`/campaigns/%d/matches/\d+"`, matchesCampaign)).FindAllString(list, -1)
	companyRe := regexp.MustCompile(fmt.Sprintf(`/campaigns/%d/companies/\d+"`, matchesCampaign))
	var matchPath, livePath string
	for _, m := range matchLinks {
		mp := m[:len(m)-1]
		_, page := get(admin, mp)
		for _, c := range companyRe.FindAllString(page, 2) {
			if _, cp := get(admin, c[:len(c)-1]); regexp.MustCompile(`live-dot-text`).MatchString(cp) {
				matchPath, livePath = mp, c[:len(c)-1]
				break
			}
		}
		if livePath != "" {
			break
		}
	}
	if livePath == "" {
		b.Fatal("no live match found in the world")
	}
	_, wl := get(admin, fmt.Sprintf("/campaigns/%d/matches", wardCampaign))
	wm := regexp.MustCompile(fmt.Sprintf(`/campaigns/%d/matches/\d+"`, wardCampaign)).FindString(wl)
	_, wp := get(admin, wm[:len(wm)-1])
	aftermath := regexp.MustCompile(fmt.Sprintf(`/campaigns/%d/matches/\d+/aftermath/\d+"`, wardCampaign)).FindString(wp)
	m := matchesCampaign
	// The new-company page answers only a member who may still found a company: the first campaign and user of the world it opens for.
	var newCompanyPath string
	var newCompanyUser *http.Cookie
	ids, err := conn.Query(ctx, "SELECT id FROM campaigns ORDER BY id")
	if err != nil {
		b.Fatal(err)
	}
	var campaignIDs []int64
	for ids.Next() {
		var id int64
		if err := ids.Scan(&id); err != nil {
			b.Fatal(err)
		}
		campaignIDs = append(campaignIDs, id)
	}
	ids.Close()
	for _, id := range campaignIDs {
		for _, c := range []*http.Cookie{eowyn, admin} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", fmt.Sprintf("/campaigns/%d/company/new", id), nil)
			req.AddCookie(c)
			h.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK && newCompanyPath == "" {
				newCompanyPath, newCompanyUser = req.URL.Path, c
			}
		}
	}
	if newCompanyPath == "" {
		b.Fatal("no campaign of the world opens the new-company page")
	}
	// /campaigns/{id}/my-company redirects to the company the player opened last (or the oldest): the page measured is the one it leads to.
	alias := httptest.NewRecorder()
	aliasReq := httptest.NewRequest("GET", fmt.Sprintf("/campaigns/%d/my-company", m), nil)
	aliasReq.AddCookie(eowyn)
	h.ServeHTTP(alias, aliasReq)
	myCompany := alias.Header().Get("Location")
	ownCompany := regexp.MustCompile(`/my-company/(\d+)`).FindStringSubmatch(myCompany)
	if myCompany == "" || ownCompany == nil {
		b.Fatalf("my-company alias answered %d with Location %q", alias.Code, myCompany)
	}
	pages := []struct {
		name   string
		c      *http.Cookie
		target string
	}{
		{"home", eowyn, "/"}, {"nav", eowyn, "/nav/campaigns"},
		{"overview", eowyn, fmt.Sprintf("/campaigns/%d", m)}, {"my-company", eowyn, myCompany},
		{"armoury", eowyn, fmt.Sprintf("/campaigns/%d/armory", m)}, {"matches", eowyn, fmt.Sprintf("/campaigns/%d/matches", m)},
		{"match", admin, matchPath}, {"ato-card", admin, matchPath + "/ato"}, {"log-card", admin, matchPath + "/live/log?card=1"},
		{"live", admin, livePath}, {"aftermath", admin, aftermath[:len(aftermath)-1]},
		{"new-company", newCompanyUser, newCompanyPath},
		{"tts", eowyn, fmt.Sprintf("/campaigns/%d/companies/%s/tts", m, ownCompany[1])},
		{"tts-no-auras", eowyn, fmt.Sprintf("/campaigns/%d/companies/%s/tts?auras=0", m, ownCompany[1])},
	}
	if dir := os.Getenv("PAGES_DUMP"); dir != "" { // PAGES_DUMP=<dir>: write the pages' bodies so two builds can be diffed byte for byte
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatal(err)
		}
		for _, id := range campaignIDs {
			for _, suffix := range []string{"", "/matches", "/armory"} {
				path := fmt.Sprintf("/campaigns/%d%s", id, suffix)
				_, body := get(admin, path)
				os.WriteFile(fmt.Sprintf("%s/campaign-%d%s.html", dir, id, strings.ReplaceAll(suffix, "/", "-")), []byte(body), 0o644)
			}
		}
		for _, p := range pages {
			_, body := get(p.c, p.target)
			os.WriteFile(dir+"/"+p.name+".html", []byte(body), 0o644)
		}
	}
	for _, p := range pages {
		// A page that redirects (the armoury to the company the player opened last) is measured at the address it leads to.
		probe := httptest.NewRecorder()
		probeReq := httptest.NewRequest("GET", p.target, nil)
		probeReq.AddCookie(p.c)
		h.ServeHTTP(probe, probeReq)
		if loc := probe.Header().Get("Location"); probe.Code == http.StatusSeeOther && loc != "" {
			p.target = loc
		}
		if code, _ := get(p.c, p.target); code != 200 { // also warms the rule sets
			b.Fatalf("%s %s: %d", p.name, p.target, code)
		}
		if os.Getenv("PERF_SQL") != "" { // PERF_SQL=1: which statements one request runs, most frequent first
			counter.mu.Lock()
			counter.sql = nil
			counter.mu.Unlock()
			get(p.c, p.target)
			counter.mu.Lock()
			type kv struct {
				s string
				n int
			}
			var all []kv
			for k, v := range counter.sql {
				all = append(all, kv{k, v})
			}
			counter.mu.Unlock()
			sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
			for _, e := range all {
				fmt.Fprintf(os.Stderr, "%s\t%3d x %s\n", p.name, e.n, abbreviate(e.s))
			}
		}
		b.Run(p.name, func(b *testing.B) {
			var size int
			q := counter.n.Load()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, body := get(p.c, p.target)
				size = len(body)
			}
			b.ReportMetric(float64(counter.n.Load()-q)/float64(b.N), "stmts/op")
			b.ReportMetric(float64(size)/1024, "KB")
		})
	}
	// Many visitors at once: the match page (four cards on four connections) requested by every CPU at the same time, to see what the
	// pool size does under load (WORLD_DATABASE_URL=...&pool_max_conns=N).
	b.Run("match-parallel", func(b *testing.B) {
		b.SetParallelism(4)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if code, _ := get(admin, matchPath); code != 200 {
					b.Error("match page answered", code)
					return
				}
			}
		})
	})
}

// abbreviate shows the start and the end of a statement: the starts of several statements are alike, the WHERE clauses tell them apart.
func abbreviate(s string) string {
	if len(s) <= 170 {
		return s
	}
	return s[:70] + " ... " + s[len(s)-95:]
}

// withoutPoolParams drops the pgxpool settings (pool_max_conns, ...) from a database URL, so a plain connection accepts it.
func withoutPoolParams(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(k, "pool_") {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
