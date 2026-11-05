package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
	"github.com/iuya/battle-company-manager-ce-v2/internal/web"
)

// The rendered pages of an archived campaign are the pages of the same campaign ended and not archived: the overview, the rankings, the matches
// and every match, every company, the glossary tab, the My company page. Only what the archive says about itself differs, and the test removes
// exactly that (the tab bar, which has no Armory in an archive, and the Manage card, which says the campaign is an archive).
func TestArchivedPagesRenderTheSameAsTheFinishedCampaign(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	t.Parallel()
	ctx := context.Background()
	store, err := postgres.Open(ctx, seededDatabase(ctx, t, base))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b, err := newBackendWithDice(ctx, store, &demoDice{})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.NewServer(web.Config{}, b.campaigns, b.rulesets, b.glossaries, b.auth)
	if err != nil {
		t.Fatal(err)
	}
	srv.SetAccountLinks(b.invitations, b.recovery)
	h := srv.Handler()
	tok, _, admin, err := b.auth.Login(ctx, "admin@example.com", "demo-password")
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) (int, string) {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(&http.Cookie{Name: "bcm_session", Value: tok})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	tabs := regexp.MustCompile(`(?s)<nav class="tab-bar.*?</nav>`)
	manage := regexp.MustCompile(`(?s)<section id="manage".*?</section>`)
	// the ends of a page that differ by themselves: the clock of a "x minutes ago", the nonce of a form
	noise := regexp.MustCompile(`name="nonce" value="[^"]*"|\?v=[0-9a-f]+`)
	// Against the Odds is not kept in an archive (docs/archive.md): its card is cut from the live page, up to the log that follows it
	ato := regexp.MustCompile(`(?s)<div id="ato-slot".*?(<details class="card mt-6" id="log")`)
	// what a live match leaves on the page and an archive does not keep: its log card, and the Tabletop Simulator button of a company
	logCard := regexp.MustCompile(`(?s)<div id="live-log-slot"[^>]*>.*?</div>\s*(</details>)`)
	// without its log card, the Log of an open match with no history holds nothing, and an archive does not draw it
	emptyLog := regexp.MustCompile(`(?s)<details class="card mt-6" id="log"[^>]*>\s*<summary[^>]*>.*?</summary>\s*</details>`)
	ttsButton := regexp.MustCompile(`(?s)<div class="inline-flex" x-data="\{ open: false \}".*?(<span class="badge badge-muted">Viewing only</span>)`)
	// a glossary term inside a text is a link only when the archive kept the term: one level of links, never the terms of those
	termLink := regexp.MustCompile(`<a href="/campaigns/\d+/glossary#[^"]*" class="term-link"[^>]*>([^<]*)</a>`)
	// the Aftermath card of a match (the sheets are not kept; what they did to a company stays in the company's log)
	aftermathCard := regexp.MustCompile(`(?s)<section class="card mt-6 space-y-4" id="aftermath">.*?</section>`)
	// the perks a company has not used yet: an open campaign's, not kept
	perksCard := regexp.MustCompile(`(?s)<section class="card mt-6 space-y-3" id="perks">.*?</section>`)
	space := regexp.MustCompile(`\s+`)
	norm := func(s string) string {
		s = ato.ReplaceAllString(s, "$1")
		s = aftermathCard.ReplaceAllString(s, "")
		s = perksCard.ReplaceAllString(s, "")
		s = termLink.ReplaceAllString(s, "$1")
		s = logCard.ReplaceAllString(s, "$1")
		s = emptyLog.ReplaceAllString(s, "")
		s = ttsButton.ReplaceAllString(s, "$1")
		s = space.ReplaceAllString(s, " ")
		s = strings.ReplaceAll(strings.ReplaceAll(s, "> <", "><"), "> ", ">")
		s = tabs.ReplaceAllString(s, "")
		s = manage.ReplaceAllString(s, "")
		return noise.ReplaceAllString(s, "")
	}
	list, err := b.campaigns.List(ctx, admin.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	b.campaigns.campaignsSvc.WithArchiver(nil)
	checked, pages := 0, 0
	for _, cs := range list {
		if cs.Matches == 0 || checked >= 8 {
			continue
		}
		checked++
		id := cs.ID
		if d, _ := b.campaigns.Get(ctx, admin.ID, id); d.Status != core.CampaignFinished {
			if err := b.campaigns.SetCampaignStatus(ctx, admin, id, core.CampaignFinished, cs.Name); err != nil {
				t.Fatal(err)
			}
		}
		paths := []string{fmt.Sprintf("/campaigns/%d", id), fmt.Sprintf("/campaigns/%d/rankings", id), fmt.Sprintf("/campaigns/%d/matches", id)}
		_, list := get(paths[2])
		for _, m := range regexp.MustCompile(fmt.Sprintf(`/campaigns/%d/matches/\d+"`, id)).FindAllString(list, -1) {
			paths = append(paths, strings.TrimSuffix(m, `"`))
		}
		_, ov := get(paths[0])
		seen := map[string]bool{}
		for _, c := range regexp.MustCompile(fmt.Sprintf(`/campaigns/%d/(?:companies|my-company)/\d+"`, id)).FindAllString(ov+list, -1) {
			if p := strings.TrimSuffix(c, `"`); !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
		for _, c := range regexp.MustCompile(fmt.Sprintf(`/campaigns/%d/companies/\d+"`, id)).FindAllString(get2(get, paths[1]), -1) {
			if p := strings.TrimSuffix(c, `"`); !seen[p] {
				seen[p] = true
				paths = append(paths, p)
			}
		}
		_, glossaryLive := get(fmt.Sprintf("/campaigns/%d/glossary", id))
		before := map[string]string{}
		for _, p := range paths {
			code, body := get(p)
			if code != 200 {
				t.Fatalf("%s before archiving: %d", p, code)
			}
			if strings.Contains(body, `id="live-banner"`) || strings.Contains(body, `data-lock="Finish the aftermath`) { // a company in a match left open or with an aftermath unplayed: the live campaign shows its live view or its lock there, the archive its sheet
				continue
			}
			before[p] = norm(body)
			if again := func() string { _, b2 := get(p); return norm(b2) }(); again != before[p] {
				delete(before, p) // a page that differs from itself (a clock) cannot be compared
			}
		}
		if err := b.archiver.Archive(ctx, id); err != nil {
			t.Fatalf("campaign %d: %v", id, err)
		}
		// the glossary of an archive is the terms in use (and what they link to): fewer, never none
		terms := regexp.MustCompile(`(\d+) terms`)
		code, glossaryArchived := get(fmt.Sprintf("/campaigns/%d/glossary", id))
		var nLive, nArchived int
		fmt.Sscan(terms.FindStringSubmatch(glossaryLive)[1], &nLive)
		fmt.Sscan(terms.FindStringSubmatch(glossaryArchived)[1], &nArchived)
		if code != 200 || nArchived == 0 || nArchived >= nLive {
			t.Errorf("campaign %d glossary: %d, %d terms archived against %d", id, code, nArchived, nLive)
		}
		for p, want := range before {
			code, body := get(p)
			if code != 200 {
				t.Errorf("%s after archiving: %d", p, code)
				continue
			}
			pages++
			if got := norm(body); got != want {
				at := 0
				for at < len(got) && at < len(want) && got[at] == want[at] {
					at++
				}
				t.Errorf("%s differs at byte %d\n   live     …%s\n   archived …%s", p, at, want[max(0, at-80):min(len(want), at+160)], got[max(0, at-80):min(len(got), at+160)])
			}
		}
	}
	if checked < 5 || pages < 40 {
		t.Fatalf("compared %d pages of %d campaigns: the demo world should offer more", pages, checked)
	}
	t.Logf("%d pages of %d campaigns render the same before and after archiving", pages, checked)
}

func get2(get func(string) (int, string), p string) string { _, b := get(p); return b }
