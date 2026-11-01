package main

import (
	"bytes"
	"context"
	"fmt"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"

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
	name := fmt.Sprintf("bcm_world_test_%d_%d", os.Getpid(), time.Now().UnixNano()%1000000) // unique per run: two test runs at once must not drop each other's database
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
	t.Parallel() // its own database (a copy of the seeded template), no state shared with the other tests
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
	checkAdminStates(t, ctx, b, admin)
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

	// several companies per player: the cap is 3, Galadriel is at it, Éowyn below it, and the viewer's own companies are all listed
	if bands, ok := byName["Two bands: several companies per player"]; !ok {
		t.Error("the campaign with several companies per player is missing")
	} else {
		if set, err := b.campaigns.Settings(ctx, bands.ID); err != nil || set.CompanyCap() != 3 {
			t.Errorf("Two bands allows %d companies per player (%v), want 3", set.CompanyCap(), err)
		}
		for key, want := range map[string]int{"galadriel@example.com": 3, "eowyn@example.com": 2, "gimli@example.com": 1, "sauron@example.com": 1} {
			_, _, u, err := b.auth.Login(ctx, key, "demo-password")
			if err != nil {
				t.Fatal(err)
			}
			d, err := b.campaigns.Get(ctx, u.ID, bands.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Companies) != want {
				t.Errorf("%s has %d companies in Two bands, want %d", key, len(d.Companies), want)
			}
			if d.Matches != 2 || d.PracticeMatches != 2 {
				t.Errorf("Two bands counts %d matches and %d practice matches, want 2 and 2", d.Matches, d.PracticeMatches)
			}
			if d.CanCreateCompany() != (want < 3) {
				t.Errorf("%s: CanCreateCompany = %v with %d of 3", key, d.CanCreateCompany(), want)
			}
		}
		matches, err := b.campaigns.Matches(ctx, bands.ID)
		if err != nil || len(matches) != 4 {
			t.Fatalf("Two bands has %d matches (%v), want 4", len(matches), err)
		}
		same, practice, openPractice := 0, 0, 0
		for _, m := range matches {
			same += len(core.SamePlayers(m.Sides))
			if m.Practice {
				practice++
				openPractice += map[bool]int{false: 1}[m.Ended]
			}
		}
		if same != 3 {
			t.Errorf("three matches have two companies of one player (two of them practice), found %d", same)
		}
		if practice != 2 || openPractice != 1 {
			t.Errorf("Two bands has %d practice matches (%d open), want 2 (1 open)", practice, openPractice)
		}

	}

	// everything on: the Wood Elf Sentinels are warriors with experience; one rolled a 6 and is a hero on its own path
	if all, ok := byName["Settings: everything on, own paths level up"]; !ok {
		t.Error("the campaign with every setting on is missing")
	} else {
		allCos, err := b.campaigns.Companies(ctx, all.ID)
		if err != nil || len(allCos) == 0 {
			t.Fatalf("companies of the all-settings campaign: %v %v", allCos, err)
		}
		d, err := b.campaigns.CompanyDetail(ctx, all.ID, allCos[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		waiting, heroes := 0, 0
		for _, u := range d.Roster {
			if u.Type != "Wood Elf Sentinel" {
				continue
			}
			switch {
			case u.IsHero() && u.Path != nil && u.Path.Name == "Path of Their Own Making":
				heroes++
			case u.Role == core.RoleWarrior && !u.OwnPath && u.PendingLevelUps() > 0:
				waiting++
			}
		}
		if waiting == 0 || heroes == 0 {
			t.Errorf("Sentinels waiting to level up as warriors: %d, heroes on their own path: %d", waiting, heroes)
		}
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

	// the Aura parade: ten companies, each of which sends rings to Tabletop Simulator; the script itself reports (as a gap) an aura entry
	// that no company carries and the list of unreachable ones does not explain
	parade, ok := byName[auraParadeName]
	if !ok {
		t.Fatal("the Aura parade is missing")
	}
	paradeCos, err := b.campaigns.Companies(ctx, parade.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(paradeCos) != 10 {
		t.Errorf("the Aura parade has %d companies, want 10", len(paradeCos))
	}
	for _, c := range paradeCos {
		sum, err := b.campaigns.TTSAuras(ctx, parade.ID, c.ID, nil, core.TTSOptions{FootVersions: true, MountFigures: true}, core.AuraSelection{Draw: true, Declared: true, Conditional: true})
		if err != nil || sum.Rings == 0 || len(sum.Kinds) == 0 {
			t.Errorf("%s sends no aura ring: %v %+v", c.Name, err, sum)
		}
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
	// ward 3: three companies wait at the screen where no hero is left and a Warrior has to be chosen (one, two tied, a Leader)
	ward3 := byName["Aftermath ward 3: no hero left to step up"]
	m3, err := b.campaigns.Matches(ctx, ward3.ID)
	if err != nil || len(m3) != 1 {
		t.Fatalf("ward 3 matches: %v %v", m3, err)
	}
	refs3, err := b.campaigns.AftermathStatuses(ctx, ward3.ID, m3[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	fallback := map[string]int{}
	for _, r := range refs3 {
		a, err := b.campaigns.Aftermath(ctx, you, ward3.ID, m3[0].ID, r.CompanyID)
		if err != nil {
			t.Fatal(err)
		}
		if a.Succession != nil {
			fallback[r.CompanyName] = len(a.Succession.Fallback)
			if r.CompanyName == "Rangers who lost every officer" && !a.Succession.NeedLeader {
				t.Error("the Rangers need a Leader from the Warriors")
			}
		}
	}
	if fallback["Orcs with one clear veteran"] != 1 || fallback["Dwarves with two equal veterans"] != 2 || fallback["Rangers who lost every officer"] < 1 {
		t.Errorf("ward 3 Warriors to choose from: %v", fallback)
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
	checkLiveAndLimit(t, ctx, b, byName, you)
	t.Run("queries do not grow with the data", func(t *testing.T) { assertQueryCounts(ctx, t, b, counter, conn, admin) })
}

// checkAdminStates asserts every state of the account invitations and recovery links through the services' list methods.
func checkAdminStates(t *testing.T, ctx context.Context, b *backend, admin core.User) {
	t.Helper()
	invs, _, err := b.invitations.List(ctx, admin, core.InvitationFilter{}, 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		kind  core.InvitationKind
		state core.InvitationState
		uses  int
		max   int
	}
	got := map[string]core.AccountInvitation{}
	for _, in := range invs {
		key := in.Email
		if in.Kind == core.InvitationAnonymous {
			key = "anonymous"
		}
		got[key] = in
	}
	for key, w := range map[string]want{
		invitePendingEmail: {core.InvitationNamed, core.InvitationPending, 0, 1},
		inviteUsedEmail:    {core.InvitationNamed, core.InvitationUsed, 1, 1},
		inviteExpiredEmail: {core.InvitationNamed, core.InvitationExpired, 0, 1},
		inviteRevokedEmail: {core.InvitationNamed, core.InvitationRevoked, 0, 1},
		"anonymous":        {core.InvitationAnonymous, core.InvitationPending, 2, 5},
	} {
		in, ok := got[key]
		switch {
		case !ok:
			t.Errorf("no invitation %q", key)
		case in.Kind != w.kind || in.State != w.state || in.Uses != w.uses || in.MaxUses != w.max:
			t.Errorf("invitation %q is %s %s %d/%d, want %s %s %d/%d", key, in.Kind, in.State, in.Uses, in.MaxUses, w.kind, w.state, w.uses, w.max)
		}
	}
	if in := got["anonymous"]; len(in.Accounts) != 2 {
		t.Errorf("the anonymous invitation made %d accounts, want 2", len(in.Accounts))
	}
	if in := got[inviteUsedEmail]; len(in.Accounts) != 1 {
		t.Errorf("the used invitation made %d accounts, want 1", len(in.Accounts))
	}
	for _, e := range []string{inviteUsedEmail, inviteGuestOneEmail, inviteGuestTwoEmail} {
		if _, _, _, err := b.auth.Login(ctx, e, "demo-password"); err != nil {
			t.Errorf("%s made through an invitation cannot sign in: %v", e, err)
		}
	}
	for _, e := range []string{invitePendingEmail, inviteExpiredEmail, inviteRevokedEmail} {
		if _, _, _, err := b.auth.Login(ctx, e, "demo-password"); err == nil {
			t.Errorf("%s has an account although its invitation was not used", e)
		}
	}

	rows, _, err := b.auth.ListUsers(ctx, admin, "recover-", 200, 0)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]*core.RecoveryStatus{}
	for _, r := range rows {
		status[r.Email] = r.Recovery
	}
	for _, s := range recoveryStates {
		r := status[recoveryEmail(s.key)]
		switch {
		case r == nil:
			t.Errorf("%s has no recovery link", recoveryEmail(s.key))
		case r.State != s.state:
			t.Errorf("%s: link is %s, want %s", recoveryEmail(s.key), r.State, s.state)
		}
	}
	for key, reason := range map[string]core.RecoveryReason{"login": core.RecoveryByLogin, "access": core.RecoveryByAccess} {
		if r := status[recoveryEmail(key)]; r != nil && r.Reason != reason {
			t.Errorf("%s: invalidated by %q, want %q", recoveryEmail(key), r.Reason, reason)
		}
	}
	if _, _, _, err := b.auth.Login(ctx, recoveryEmail("used"), "demo-password"); err != nil {
		t.Errorf("the account with the used link cannot sign in: %v", err)
	}
	// the world stays usable: the pending named invitation and the pending recovery link are still pending
	if r := status[recoveryPendingEmail]; r == nil || r.State != core.RecoveryPending {
		t.Errorf("the pending recovery link is %+v", r)
	}
}

// checkLiveAndLimit asserts the Fog of War game (secret notes, the zones of retreated and fallen units, the live Tabletop Simulator
// export with a second warband) and the campaign whose limit was lowered after companies existed.
func checkLiveAndLimit(t *testing.T, ctx context.Context, b *backend, byName map[string]core.CampaignSummary, you core.User) {
	t.Helper()
	fog, ok := byName[fogName]
	if !ok {
		t.Fatal("the Fog of War campaign is missing")
	}
	matches, err := b.campaigns.Matches(ctx, fog.ID)
	if err != nil || len(matches) != 1 {
		t.Fatalf("Fog of War matches: %v %v", matches, err)
	}
	view, err := b.campaigns.SecretObjectives(ctx, you, fog.ID, matches[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Revealed || len(view.Others) == 0 {
		t.Errorf("the secret notes of an open match are hidden from a stranger: %+v", view)
	}
	cos, err := b.campaigns.Companies(ctx, fog.ID)
	if err != nil || len(cos) != 2 {
		t.Fatalf("Fog of War companies: %v %v", cos, err)
	}
	for _, c := range cos {
		_, _, owner, err := b.auth.Login(ctx, map[string]string{"Dwarves who keep their counsel": "gimli@example.com", "Rangers of the silent road": "aragorn@example.com"}[c.Name], "demo-password")
		if err != nil {
			t.Fatal(err)
		}
		own, err := b.campaigns.SecretObjectives(ctx, owner, fog.ID, matches[0].ID)
		if err != nil || len(own.Lines) == 0 {
			t.Errorf("%s sees no secret note of their own: %v %+v", c.Name, err, own)
		}
		lc, err := b.campaigns.Live(ctx, owner, fog.ID, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		out := 0
		for _, u := range lc.Units {
			if u.Zone == core.ZoneRetreated || u.Zone == core.ZoneCasualty {
				out++
			}
		}
		if out == 0 {
			t.Errorf("%s has no unit that left the table", c.Name)
		}
		exp, err := b.campaigns.TTSExport(ctx, fog.ID, c.ID, &core.TTSLive{Units: lc.Units, Absent: lc.Absent}, core.TTSOptions{})
		if err != nil || len(exp.Warbands) != 2 {
			t.Errorf("%s: the live export has %d warbands (%v), want 2", c.Name, len(exp.Warbands), err)
		}
	}

	low, ok := byName[loweredName]
	if !ok {
		t.Fatal("the campaign with a lowered limit is missing")
	}
	if set, err := b.campaigns.Settings(ctx, low.ID); err != nil || set.CompanyCap() != 2 {
		t.Errorf("the lowered limit is %d (%v), want 2", set.CompanyCap(), err)
	}
	for key, want := range map[string]struct {
		n      int
		create bool
	}{"galadriel@example.com": {3, false}, "eowyn@example.com": {2, false}, "aragorn@example.com": {1, true}} {
		_, _, u, err := b.auth.Login(ctx, key, "demo-password")
		if err != nil {
			t.Fatal(err)
		}
		d, err := b.campaigns.Get(ctx, u.ID, low.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Companies) != want.n || d.CanCreateCompany() != want.create {
			t.Errorf("%s has %d companies and may create another: %v; want %d and %v", key, len(d.Companies), d.CanCreateCompany(), want.n, want.create)
		}
	}
}
