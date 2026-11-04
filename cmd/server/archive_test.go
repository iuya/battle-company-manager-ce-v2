package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

// pageData is what the read-only pages of a campaign are built from, gathered through the services: the campaign, its companies, the
// members, every company's detail, the matches with their history and every company's grants. Before and after archiving it must be the same.
func pageData(t *testing.T, ctx context.Context, b *backend, viewer core.User, id int64) map[string]any {
	t.Helper()
	out := map[string]any{}
	put := func(key string, v any, err error, drop ...string) {
		t.Helper()
		if err != nil {
			t.Fatalf("campaign %d %s: %v", id, key, err)
		}
		raw, _ := json.Marshal(v)
		var m any
		_ = json.Unmarshal(raw, &m)
		for _, d := range drop {
			dropKey(m, d)
		}
		out[key] = m
	}
	c := b.campaigns
	d, err := c.Get(ctx, viewer.ID, id)
	// the rules files are the archive's own; what a management dialog counts (open matches, links) only an open campaign has
	put("campaign", d, err, "Rulesets", "Hashes", "Archived", "OpenMatches", "PendingAftermaths", "ActiveInvites")
	cs, err := c.Companies(ctx, id)
	put("companies", cs, err)
	ms, err := c.Members(ctx, id)
	put("members", ms, err)
	for _, co := range cs {
		cd, err := c.CompanyDetail(ctx, id, co.ID)
		// what only a company in an open campaign has: aftermaths to finish, perks to use, the live match, and the dice
		// a level-up rolls per path (the paths a campaign did not use are not in its archive)
		put(fmt.Sprintf("company %d", co.ID), cd, err, "Aftermaths", "OpenMatch", "Perks", "RuleDice")
		gr, err := c.Grants(ctx, id, co.ID)
		put(fmt.Sprintf("grants %d", co.ID), gr, err)
	}
	list, err := c.Matches(ctx, id)
	put("matches", list, err)
	for _, m := range list {
		one, err := c.Match(ctx, id, m.ID)
		put(fmt.Sprintf("match %d", m.ID), one, err)
	}
	return out
}

func dropKey(v any, key string) {
	switch x := v.(type) {
	case map[string]any:
		delete(x, key)
		for _, e := range x {
			dropKey(e, key)
		}
	case []any:
		for _, e := range x {
			dropKey(e, key)
		}
	}
}

func diffPages(before, after map[string]any) []string {
	var diffs []string
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		a, _ := json.Marshal(before[k])
		b, _ := json.Marshal(after[k])
		if string(a) != string(b) {
			at := 0
			for at < len(a) && at < len(b) && a[at] == b[at] {
				at++
			}
			diffs = append(diffs, fmt.Sprintf("%s differs at byte %d\n    before …%s\n    after  …%s", k, at, around(a, at), around(b, at)))
		}
	}
	return diffs
}

func around(b []byte, at int) string {
	lo, hi := max(0, at-60), min(len(b), at+120)
	return string(b[lo:hi])
}

// Ending a campaign archives it, and every read-only page of it shows exactly what it showed before: the companies with their rosters,
// cemetery and ratings, the rankings, the members, the matches with their results and history, the grants. The rows are gone, the rules
// files are released, and nothing in the campaign can change any more.
func TestArchiveKeepsTheReadOnlyPages(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	t.Parallel()
	ctx := context.Background()
	url := seededDatabase(ctx, t, base)
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b, err := newBackendWithDice(ctx, store, &demoDice{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, admin, err := b.auth.Login(ctx, "admin@example.com", "demo-password")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	list, err := b.campaigns.List(ctx, admin.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	b.campaigns.campaignsSvc.WithArchiver(nil) // end first, look, then archive: the comparison needs the campaign as it was ended
	archived := 0
	for _, cs := range list {
		if cs.Matches == 0 {
			continue
		}
		cs := cs
		t.Run(cs.Name, func(t *testing.T) {
			d, err := b.campaigns.Get(ctx, admin.ID, cs.ID)
			if err != nil {
				t.Fatal(err)
			}
			if d.Status != core.CampaignFinished {
				if err := b.campaigns.SetCampaignStatus(ctx, admin, cs.ID, core.CampaignFinished, cs.Name); err != nil {
					t.Fatal(err)
				}
			}
			before := pageData(t, ctx, b, admin, cs.ID)
			rowsBefore := rowCounts(t, ctx, conn, cs.ID)
			if err := b.archiver.Archive(ctx, cs.ID); err != nil {
				t.Fatalf("archive: %v", err)
			}
			archived++
			after := pageData(t, ctx, b, admin, cs.ID)
			if diffs := diffPages(before, after); len(diffs) > 0 {
				t.Errorf("the archived campaign shows something else:\n  %s", strings.Join(diffs, "\n  "))
			}
			rowsAfter := rowCounts(t, ctx, conn, cs.ID)
			t.Logf("rows before %v, after %v", rowsBefore, rowsAfter)
			for _, table := range []string{"company_units", "fallen_units", "matches", "grants", "company_perks", "campaign_invites"} {
				if rowsAfter[table] != 0 {
					t.Errorf("%s still has %d rows", table, rowsAfter[table])
				}
			}
			if rowsAfter["companies"] != rowsBefore["companies"] || rowsAfter["campaign_members"] != rowsBefore["campaign_members"] {
				t.Errorf("companies or members changed: %v -> %v", rowsBefore, rowsAfter)
			}
			if rowsAfter["pinned_files"] != 0 {
				t.Errorf("the campaign still pins %d rules files", rowsAfter["pinned_files"])
			}
			// archiving again is a no-op; a change is refused
			if err := b.archiver.Archive(ctx, cs.ID); err != nil {
				t.Errorf("a second archive: %v", err)
			}
			if err := b.campaigns.SetCampaignStatus(ctx, admin, cs.ID, core.CampaignActive, ""); err == nil || !errors.Is(err, core.ErrRefused) {
				t.Errorf("reopening an ended campaign: %v, want a refusal", err)
			}
		})
	}
	if archived < 3 {
		t.Fatalf("only %d campaigns were archived: the demo world should offer more", archived)
	}
}

func rowCounts(t *testing.T, ctx context.Context, conn *pgx.Conn, id int64) map[string]int {
	t.Helper()
	out := map[string]int{}
	for table, q := range map[string]string{
		"company_units":    `SELECT count(*) FROM company_units WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`,
		"fallen_units":     `SELECT count(*) FROM fallen_units WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`,
		"matches":          `SELECT count(*) FROM matches WHERE campaign_id = $1`,
		"grants":           `SELECT count(*) FROM grants WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`,
		"company_perks":    `SELECT count(*) FROM company_perks WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`,
		"campaign_invites": `SELECT count(*) FROM campaign_invites WHERE campaign_id = $1`,
		"companies":        `SELECT count(*) FROM companies WHERE campaign_id = $1`,
		"campaign_members": `SELECT count(*) FROM campaign_members WHERE campaign_id = $1`,
		"pinned_files":     `SELECT (vanilla_hash IS NOT NULL)::int + (bc_hash IS NOT NULL)::int + (glossary_hash IS NOT NULL)::int FROM campaigns WHERE id = $1`,
	} {
		var n int
		if err := conn.QueryRow(ctx, q, id).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		out[table] = n
	}
	return out
}
