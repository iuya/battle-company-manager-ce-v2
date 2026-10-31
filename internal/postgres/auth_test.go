package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Integration test: needs a throwaway database, e.g. `make db-up` then
// TEST_DATABASE_URL=postgres://bcm:bcm@localhost:5432/bcm?sslmode=disable go test ./...
func TestUsersAndSessions(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	email := fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
	u, err := s.CreateUser(ctx, email, "Tester", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, strings.ToUpper(email), "Again", "hash"); !errors.Is(err, core.ErrEmailTaken) {
		t.Fatalf("email uniqueness must be case-insensitive, got %v", err)
	}
	got, err := s.UserByEmail(ctx, strings.ToUpper(email))
	if err != nil || got.ID != u.ID || got.PasswordHash != "hash" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := s.UserByEmail(ctx, "nobody@example.com"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	tok := []byte("token-" + email)
	if err := s.CreateSession(ctx, tok, u.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	su, exp, _, err := s.UserBySession(ctx, tok, now)
	if err != nil || su.ID != u.ID || !exp.Equal(now.Add(time.Hour)) {
		t.Fatalf("got %+v %v %v", su, exp, err)
	}
	if _, _, _, err := s.UserBySession(ctx, tok, now.Add(2*time.Hour)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("an expired session must not resolve, got %v", err)
	}
	if err := s.ExtendSession(ctx, tok, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.UserBySession(ctx, tok, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("an extended session must resolve, got %v", err)
	}
	if err := s.DeleteExpiredSessions(ctx, now.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.UserBySession(ctx, tok, now); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a purged session must be gone, got %v", err)
	}
	_ = s.DeleteSession(ctx, tok)
}

func TestAccount(t *testing.T) {
	s, ctx := testStore(t)
	u := testUser(t, s, ctx, "acct")
	now := time.Now().UTC().Truncate(time.Second)
	tokA, tokB := []byte("a-"+u.Email), []byte("b-"+u.Email)
	for _, tok := range [][]byte{tokA, tokB} {
		if err := s.CreateSession(ctx, tok, u.ID, now.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	// appearance: empty until chosen, then loaded with the session; the database refuses unknown keys
	got, _, _, err := s.UserBySession(ctx, tokA, now)
	if err != nil || got.Appearance != (core.Appearance{}) {
		t.Fatalf("never chosen: %+v %v", got.Appearance, err)
	}
	if err := s.SetAppearance(ctx, u.ID, "ember", "dark"); err != nil {
		t.Fatal(err)
	}
	if got, _, _, _ = s.UserBySession(ctx, tokA, now); got.Appearance != (core.Appearance{Palette: "ember", Mode: "dark"}) {
		t.Fatalf("appearance: %+v", got.Appearance)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE users SET palette = 'bogus', mode = 'dark' WHERE id = $1`, u.ID); err == nil {
		t.Fatal("an unknown palette must violate the check")
	}
	if err := s.SetAppearance(ctx, -1, "ember", "dark"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}

	// time zone: automatic until chosen, cleared with ""
	if got, _, _, _ = s.UserBySession(ctx, tokA, now); got.Timezone != "" {
		t.Fatalf("automatic: %q", got.Timezone)
	}
	if err := s.SetTimezone(ctx, u.ID, "Pacific/Auckland"); err != nil {
		t.Fatal(err)
	}
	if got, _, _, _ = s.UserBySession(ctx, tokA, now); got.Timezone != "Pacific/Auckland" {
		t.Fatalf("timezone: %q", got.Timezone)
	}
	if err := s.SetTimezone(ctx, u.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _, _, _ = s.UserBySession(ctx, tokA, now); got.Timezone != "" {
		t.Fatalf("cleared: %q", got.Timezone)
	}
	if err := s.SetTimezone(ctx, -1, "UTC"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}

	// profile and password
	if up, err := s.UpdateDisplayName(ctx, u.ID, "Renamed"); err != nil || up.DisplayName != "Renamed" || up.Appearance.Palette != "ember" {
		t.Fatalf("%+v %v", up, err)
	}
	if err := s.UpdatePasswordHash(ctx, u.ID, "new-hash"); err != nil {
		t.Fatal(err)
	}
	if c, err := s.CredentialsByID(ctx, u.ID); err != nil || c.PasswordHash != "new-hash" {
		t.Fatalf("%+v %v", c, err)
	}
	if n, err := s.DeleteSessionsExcept(ctx, u.ID, tokA); err != nil || n != 1 {
		t.Fatalf("deleted %d, %v", n, err)
	}
	if _, _, _, err := s.UserBySession(ctx, tokB, now); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("the other session must be gone: %v", err)
	}
	if _, _, _, err := s.UserBySession(ctx, tokA, now); err != nil {
		t.Fatalf("the kept session must stay: %v", err)
	}

	// closing: organiser of an unfinished campaign is detected; once finished the account closes and stays anonymised
	camp, err := s.Campaigns().Create(ctx, u, testNewCampaign(t, s, ctx, "Mine"))
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.OrganisesUnfinishedCampaign(ctx, u.ID); err != nil || !ok {
		t.Fatalf("organiser: %v %v", ok, err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE campaigns SET status = 'finished' WHERE id = $1`, camp.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.OrganisesUnfinishedCampaign(ctx, u.ID); ok {
		t.Fatal("a finished campaign must not block")
	}
	if err := s.CloseUser(ctx, u.ID, "Former player", fmt.Sprintf("closed-%d@invalid", u.ID), now); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.UserBySession(ctx, tokA, now); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("closing deletes every session: %v", err)
	}
	if _, err := s.CredentialsByID(ctx, u.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a closed user has no credentials: %v", err)
	}
	if err := s.CloseUser(ctx, u.ID, "Former player", "x@invalid", now); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("closing twice: %v", err)
	}
	// the campaign (and so the companies and matches around it) still loads, under the anonymised name
	list, err := s.Campaigns().List(ctx, u.ID, true)
	if err != nil || len(list) != 1 || list[0].ID != camp.ID {
		t.Fatalf("campaigns of a closed user: %+v %v", list, err)
	}
	var name string
	if err := s.pool.QueryRow(ctx, `SELECT u.display_name FROM campaign_members m JOIN users u ON u.id = m.user_id WHERE m.campaign_id = $1`, camp.ID).Scan(&name); err != nil || name != "Former player" {
		t.Fatalf("member name %q, %v", name, err)
	}

	// login of a closed user is impossible: the e-mail is gone and the hash empty (core refuses an empty hash)
	cr, err := s.UserByEmail(ctx, fmt.Sprintf("closed-%d@invalid", u.ID))
	if err != nil || cr.PasswordHash != "" {
		t.Fatalf("%+v %v", cr, err)
	}
	if n, err := s.OtherSuperusers(ctx, u.ID); err != nil || n < 0 {
		t.Fatalf("%d %v", n, err)
	}
}
