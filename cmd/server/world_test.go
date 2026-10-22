package main

import (
	"bytes"
	"context"
	neturl "net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

// freshDatabase makes an empty database next to the test one (the other tests of the repository share and empty the test
// database, so the world would be found half there) and returns its URL; it is dropped when the test ends.
func freshDatabase(ctx context.Context, t *testing.T, url string) string {
	t.Helper()
	u, err := neturl.Parse(url)
	if err != nil {
		t.Fatal(err)
	}
	const name = "bcm_world_test"
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	drop := func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") }
	drop()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close(ctx)
		t.Skipf("cannot create a database for the demo world: %v", err)
	}
	t.Cleanup(func() { drop(); admin.Close(ctx) })
	u.Path = "/" + name
	return u.String()
}

// TestSeedWorld loads the demo world into the test database (a second run changes nothing) and checks the states it promises.
func TestSeedWorld(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	if _, err := os.Stat("definitions"); err != nil { // definitions/ is read relative to the repository root
		if err := os.Chdir("../.."); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	url = freshDatabase(ctx, t, url)
	counter := &queryCounter{}
	store, err := postgres.OpenWith(ctx, url, postgres.Options{Tracer: counter})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dice := &demoDice{}
	b, err := newBackendWithDice(ctx, store, dice)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
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
	if strings.Contains(out.String(), "GAP:") {
		t.Errorf("the script met a state it could not reach:\n%s", out.String())
	}
	var again bytes.Buffer
	if err := seedWorld(ctx, b, dice, "demo-password", &again, exec); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !strings.Contains(again.String(), "exists already") {
		t.Errorf("the second run should do nothing, it printed:\n%s", again.String())
	}

	_, _, admin, err := b.auth.Login(ctx, "admin@example.com", "demo-password")
	if err != nil {
		t.Fatal(err)
	}
	_, _, you, err := b.auth.Login(ctx, "you@example.com", "demo-password")
	if err != nil {
		t.Fatal(err)
	}
	list, err := b.campaigns.List(ctx, you.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]core.CampaignSummary{}
	for _, s := range list {
		byName[s.Name] = s
	}
	for _, name := range []string{worldMarker, galleryName, "Matches: every scenario and every state", "Finished campaign: the Siege of Osgiliath"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("campaign %q is missing", name)
		}
	}
	if s := byName["Finished campaign: the Siege of Osgiliath"]; s.Status != core.CampaignFinished {
		t.Errorf("the finished campaign is %q", s.Status)
	}

	// the gallery has a company of each of the 37 factions
	gal := byName[galleryName]
	cos, err := b.campaigns.Companies(ctx, gal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cos) != 37 {
		t.Errorf("the gallery has %d companies, want 37", len(cos))
	}

	// the aftermath ward has a company in every stage
	ward := byName["Aftermath ward 1: every step of the War's Aftermath"]
	matches, err := b.campaigns.Matches(ctx, ward.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[core.AftermathStatus]bool{}
	for _, m := range matches {
		refs, err := b.campaigns.AftermathStatuses(ctx, ward.ID, m.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range refs {
			seen[r.Status] = true
		}
	}
	for _, st := range []core.AftermathStatus{core.AftermathPending, core.AftermathSheet, core.AftermathInjuries, core.AftermathLevelUps, core.AftermathDone, core.AftermathWaived} {
		if !seen[st] {
			t.Errorf("no company of the aftermath ward is %q", st)
		}
	}
	// the Shire company of the cemetery lost its Ranger in a played match: three officers (a company recruits only with
	// three) and the Ranger in the cemetery, so Recruit may swap a successful result for a new one
	cem, ok := byName["Cemetery: the fallen of the Misty Mountains"]
	if !ok {
		t.Fatal("the cemetery campaign is missing")
	}
	cemCos, err := b.campaigns.Companies(ctx, cem.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range cemCos {
		if c.Name != "Hobbits who lost their Ranger" {
			continue
		}
		found = true
		d, err := b.campaigns.CompanyDetail(ctx, cem.ID, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		officers, ranger := 0, false
		for _, u := range d.Roster {
			if u.Role == core.RoleLeader || u.Role == core.RoleSergeant {
				officers++
			}
			ranger = ranger || u.Type == "Ranger of the North"
		}
		if officers != 3 || ranger {
			t.Errorf("the Shire company has %d officers (want 3) and a Ranger on the roster: %v", officers, ranger)
		}
		if !d.RangerMayReturn {
			t.Error("the fallen Ranger of the North should be offered back by the reinforcements")
		}
	}
	if !found {
		t.Error("the Shire company of the cemetery is missing")
	}
	t.Run("queries do not grow with the data", func(t *testing.T) { assertQueryCounts(ctx, t, b, counter, conn, admin) })
}
