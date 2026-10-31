package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

// queryCounter counts the statements the pool runs (postgres.Options.Tracer).
type queryCounter struct {
	n   atomic.Int64
	mu  sync.Mutex
	sql map[string]int // statements by text, for BenchmarkPages with PERF_SQL=1
}

func (c *queryCounter) note(sql string) {
	if want := os.Getenv("PERF_STACK"); want != "" && strings.Contains(sql, want) { // PERF_STACK=<text>: who runs a statement that contains the text
		var frames []string
		for _, line := range strings.Split(string(debug.Stack()), "\n") {
			if i := strings.Index(line, "battle-company-manager-ce-v2/internal/"); i >= 0 && !strings.HasPrefix(line, "\t") && !strings.Contains(line, "readscope") {
				frames = append(frames, strings.TrimSpace(line[i+len("battle-company-manager-ce-v2/internal/"):]))
			}
		}
		fmt.Fprintf(os.Stderr, "STACK %.40s: %s\n", strings.Join(strings.Fields(sql), " "), strings.Join(frames, " < "))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sql == nil {
		c.sql = map[string]int{}
	}
	c.sql[strings.Join(strings.Fields(sql), " ")]++
}

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	c.note(d.SQL)
	return ctx
}
func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *queryCounter) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return ctx
}
func (c *queryCounter) TraceBatchQuery(_ context.Context, _ *pgx.Conn, d pgx.TraceBatchQueryData) {
	c.n.Add(1)
	c.note(d.SQL)
}
func (c *queryCounter) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {}

// measure runs f and returns the number of statements it made.
func (c *queryCounter) measure(f func()) int {
	before := c.n.Load()
	f()
	return int(c.n.Load() - before)
}

// sample is one measured call and how much data it handled.
type sample struct{ size, queries int }

// budget is what a call may cost: base statements plus perRow for every row it returns. perRow 0 is the goal (a fixed number of
// statements whatever the size); a positive perRow would pin a known N+1 so it cannot get worse.
type budget struct{ base, perRow int }

func (b budget) limit(size int) int { return b.base + b.perRow*size }

// within fails when a sample costs more than its budget and logs the cheapest and the dearest sample, so a shrinking cost is visible.
func within(t *testing.T, name string, samples []sample, b budget) {
	t.Helper()
	if len(samples) < 2 {
		t.Errorf("%s: %d samples, the world has too few to compare", name, len(samples))
		return
	}
	worst, lo, hi := samples[0], samples[0], samples[0]
	for _, s := range samples {
		if s.queries-b.limit(s.size) > worst.queries-b.limit(worst.size) {
			worst = s
		}
		if s.size < lo.size {
			lo = s
		}
		if s.size > hi.size {
			hi = s
		}
	}
	t.Logf("%s: %d queries for %d rows, %d for %d rows (budget %d + %d per row)", name, lo.queries, lo.size, hi.queries, hi.size, b.base, b.perRow)
	if worst.queries > b.limit(worst.size) {
		t.Errorf("%s: %d queries for %d rows, over the budget of %d + %d per row = %d", name, worst.queries, worst.size, b.base, b.perRow, b.limit(worst.size))
	}
}

// assertQueryCounts reads the main pages' data of every campaign of the loaded demo world (from 2 to 37 companies) and checks that
// the number of queries does not grow with the number of companies, matches or units (docs/performance.md).
func assertQueryCounts(ctx context.Context, t *testing.T, b *backend, q *queryCounter, conn *pgx.Conn, admin core.User) {
	t.Helper()
	rows, err := conn.Query(ctx, "SELECT id FROM campaigns ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()

	var companies, matches, matchOne, detail, get []sample
	for _, id := range ids {
		var cos []core.CampaignCompany
		var ms []core.Match
		get = append(get, sample{len(ids), q.measure(func() {
			if _, err := b.campaigns.Get(ctx, admin.ID, id); err != nil {
				t.Fatal(err)
			}
		})})
		n := q.measure(func() {
			if cos, err = b.campaigns.Companies(ctx, id); err != nil {
				t.Fatal(err)
			}
		})
		companies = append(companies, sample{len(cos), n})
		n = q.measure(func() {
			if ms, err = b.campaigns.Matches(ctx, id); err != nil {
				t.Fatal(err)
			}
		})
		matches = append(matches, sample{len(ms), n})
		for _, m := range ms[:min(len(ms), 3)] {
			n = q.measure(func() {
				if _, err := b.campaigns.Match(ctx, id, m.ID); err != nil {
					t.Fatal(err)
				}
			})
			matchOne = append(matchOne, sample{1, n})
		}
		for _, c := range cos[:min(len(cos), 3)] {
			var d core.CompanyDetail
			n = q.measure(func() {
				if d, err = b.campaigns.CompanyDetail(ctx, id, c.ID); err != nil {
					t.Fatal(err)
				}
			})
			detail = append(detail, sample{len(d.Roster), n})
		}
	}
	var home []sample // the home page: the campaigns of a user, with and without the finished ones
	for _, finished := range []bool{true, false} {
		home = append(home, sample{0, q.measure(func() {
			if _, err := b.campaigns.List(ctx, admin.ID, finished); err != nil {
				t.Fatal(err)
			}
		})})
	}

	within(t, "Get (campaign page)", get, budget{3, 0})
	within(t, "Companies (companies, rankings, handover)", companies, budget{6, 0})
	within(t, "Matches (matches page)", matches, budget{8, 0})
	within(t, "Match (match page)", matchOne, budget{6, 0})
	within(t, "CompanyDetail (company page)", detail, budget{7, 0})
	within(t, "List (home page)", home, budget{3, 0})
	assertAccountQueryCounts(ctx, t, b, q, admin)
}

// assertAccountQueryCounts checks what the account links cost: a session answered from the cache costs no statement at all (the
// pending-recovery fact rides on the cache entry, so nothing is asked per request), and the Users and Invitations pages read their
// rows (recovery status, creators, accounts made through an invitation) in a fixed number of statements.
func assertAccountQueryCounts(ctx context.Context, t *testing.T, b *backend, q *queryCounter, admin core.User) {
	t.Helper()
	token, _, _, err := b.auth.Login(ctx, "admin@example.com", "demo-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.auth.Authenticate(ctx, token); err != nil { // fills the cache
		t.Fatal(err)
	}
	if n := q.measure(func() {
		for range 5 {
			if _, _, err := b.auth.Authenticate(ctx, token); err != nil {
				t.Fatal(err)
			}
		}
	}); n != 0 {
		t.Errorf("a session cache hit ran %d statements, want 0", n)
	}
	var pages []sample
	for _, limit := range []int{5, 25, 200} {
		var rows []core.AdminUser
		n := q.measure(func() {
			if rows, _, err = b.auth.ListUsers(ctx, admin, "", limit, 0); err != nil {
				t.Fatal(err)
			}
		})
		pages = append(pages, sample{len(rows), n})
	}
	within(t, "ListUsers (Users page)", pages, budget{3, 0})
	pages = nil
	for _, limit := range []int{2, 25, 200} {
		var rows []core.AccountInvitation
		n := q.measure(func() {
			if rows, _, err = b.invitations.List(ctx, admin, core.InvitationFilter{}, limit, 0); err != nil {
				t.Fatal(err)
			}
		})
		pages = append(pages, sample{len(rows), n})
	}
	within(t, "AccountInvitations.List (Invitations page)", pages, budget{3, 0})
}

// TestQueryCountsOnAPreloadedWorld is the quick way to run the checks above while tuning a query: point WORLD_DATABASE_URL at a
// database where "server seed --world" has run (TestSeedWorld does the same on a fresh one after loading the world).
func TestQueryCountsOnAPreloadedWorld(t *testing.T) {
	url := os.Getenv("WORLD_DATABASE_URL")
	if url == "" {
		t.Skip("WORLD_DATABASE_URL is not set")
	}
	ctx := context.Background()
	counter := &queryCounter{}
	store, err := postgres.OpenWith(ctx, url, postgres.Options{Tracer: counter})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b, err := newBackendWithDice(ctx, store, &demoDice{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	_, _, admin, err := b.auth.Login(ctx, "admin@example.com", "demo-password")
	if err != nil {
		t.Fatal(err)
	}
	assertQueryCounts(ctx, t, b, counter, conn, admin)
}
