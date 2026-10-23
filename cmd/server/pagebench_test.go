package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
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
	if _, err := os.Stat("definitions"); err != nil {
		if err := os.Chdir("../.."); err != nil {
			b.Fatal(err)
		}
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
	conn, err := pgx.Connect(ctx, url)
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
	pages := []struct {
		name   string
		c      *http.Cookie
		target string
	}{
		{"home", eowyn, "/"}, {"nav", eowyn, "/nav/campaigns"},
		{"overview", eowyn, fmt.Sprintf("/campaigns/%d", m)}, {"my-company", eowyn, fmt.Sprintf("/campaigns/%d/my-company", m)},
		{"armoury", eowyn, fmt.Sprintf("/campaigns/%d/armory", m)}, {"matches", eowyn, fmt.Sprintf("/campaigns/%d/matches", m)},
		{"match", admin, matchPath}, {"ato-card", admin, matchPath + "/ato"}, {"log-card", admin, matchPath + "/live/log?card=1"},
		{"live", admin, livePath}, {"aftermath", admin, aftermath[:len(aftermath)-1]},
	}
	for _, p := range pages {
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
				fmt.Fprintf(os.Stderr, "%s\t%3d x %.150s\n", p.name, e.n, e.s)
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
}
