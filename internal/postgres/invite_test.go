package postgres

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestInvites(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Invites()
	org := testUser(t, s, ctx, "iorg")
	camp, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Invites"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mk := func(token string, expires time.Time, maxUses int) string {
		h := core.HashInviteToken(token + now.String())
		if err := repo.Create(ctx, core.NewInvite{CampaignID: camp.ID, TokenHash: h, ExpiresAt: expires, MaxUses: maxUses, CreatedBy: org.ID}); err != nil {
			t.Fatal(err)
		}
		return h
	}
	uses := func(h string) (n int) {
		if err := s.pool.QueryRow(ctx, `SELECT uses FROM campaign_invites WHERE token_hash = $1`, h).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := repo.Create(ctx, core.NewInvite{CampaignID: 0, TokenHash: core.HashInviteToken("none"), ExpiresAt: now, CreatedBy: org.ID}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown campaign: %v", err)
	}

	// Unlimited: joins, a repeat or the organiser does not use it up.
	open := mk("open", now.Add(time.Hour), 0)
	p1 := testUser(t, s, ctx, "ione")
	if id, err := repo.Redeem(ctx, p1.ID, open, now); err != nil || id != camp.ID {
		t.Fatalf("redeem: %d, %v", id, err)
	}
	if _, err := repo.Redeem(ctx, p1.ID, open, now); err != nil || uses(open) != 1 {
		t.Fatalf("repeat: uses %d, %v", uses(open), err)
	}
	if _, err := repo.Redeem(ctx, org.ID, open, now); err != nil || uses(open) != 1 {
		t.Fatalf("organiser: uses %d, %v", uses(open), err)
	}
	if d, err := s.Campaigns().Get(ctx, p1.ID, camp.ID); err != nil || d.ID != camp.ID {
		t.Fatalf("member: %+v, %v", d, err)
	}

	// Lookup changes nothing and says whether the visitor is already in.
	if pv, err := repo.Lookup(ctx, p1.ID, open, now); err != nil || pv.CampaignID != camp.ID || pv.CampaignName != "Invites" || !pv.Member || uses(open) != 1 {
		t.Fatalf("lookup of a member: %+v, %v", pv, err)
	}
	outsider := testUser(t, s, ctx, "iout")
	if pv, err := repo.Lookup(ctx, outsider.ID, open, now); err != nil || pv.Member {
		t.Fatalf("lookup of an outsider: %+v, %v", pv, err)
	}
	if _, err := repo.Lookup(ctx, outsider.ID, core.HashInviteToken("nope"), now); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("lookup of an unknown token: %v", err)
	}

	// Unknown, expired.
	if _, err := repo.Redeem(ctx, p1.ID, core.HashInviteToken("nope"), now); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	old := mk("old", now.Add(time.Minute), 0)
	p2 := testUser(t, s, ctx, "itwo")
	if _, err := repo.Redeem(ctx, p2.ID, old, now.Add(time.Minute)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expired: %v", err)
	}
	if _, err := s.Campaigns().Get(ctx, p2.ID, camp.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expired invite made a member: %v", err)
	}

	// Limited to 2 uses, redeemed concurrently by 5 users: exactly 2 succeed.
	lim := mk("lim", now.Add(time.Hour), 2)
	var wg sync.WaitGroup
	results := make([]error, 5)
	for i := range results {
		u := testUser(t, s, ctx, "ilim")
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, results[i] = repo.Redeem(ctx, u.ID, lim, now)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, core.ErrNotFound):
			t.Fatal(err)
		}
	}
	if ok != 2 || uses(lim) != 2 {
		t.Fatalf("limited invite: %d succeeded, uses %d", ok, uses(lim))
	}
}

func TestInviteOfAFinishedCampaignCannotBeRedeemed(t *testing.T) {
	s, ctx := testStore(t)
	org := testUser(t, s, ctx, "fiorg")
	camp, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Finished invites"))
	if err != nil {
		t.Fatal(err)
	}
	h := core.HashInviteToken("finished" + time.Now().String())
	if err := s.Invites().Create(ctx, core.NewInvite{CampaignID: camp.ID, TokenHash: h, ExpiresAt: time.Now().Add(time.Hour), CreatedBy: org.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE campaigns SET status = 'finished' WHERE id = $1`, camp.ID); err != nil {
		t.Fatal(err)
	}
	late := testUser(t, s, ctx, "fiplayer")
	if _, err := s.Invites().Redeem(ctx, late.ID, h, time.Now()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("finished campaign: %v", err)
	}
}

func TestPausedCampaignTakesNoNewMembers(t *testing.T) {
	s, ctx := testStore(t)
	org := testUser(t, s, ctx, "paorg")
	camp, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Paused invites"))
	if err != nil {
		t.Fatal(err)
	}
	h := core.HashInviteToken("paused" + time.Now().String())
	if err := s.Invites().Create(ctx, core.NewInvite{CampaignID: camp.ID, TokenHash: h, ExpiresAt: time.Now().Add(time.Hour), CreatedBy: org.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE campaigns SET status = 'paused' WHERE id = $1`, camp.ID); err != nil {
		t.Fatal(err)
	}
	late := testUser(t, s, ctx, "paplayer")
	if p, err := s.Invites().Lookup(ctx, late.ID, h, time.Now()); err != nil || !p.Paused {
		t.Fatalf("the preview says paused: %+v %v", p, err)
	}
	if _, err := s.Invites().Redeem(ctx, late.ID, h, time.Now()); !errors.Is(err, core.ErrCampaignPaused) || !errors.Is(err, core.ErrRefused) {
		t.Fatalf("a new member of a paused campaign: %v", err)
	}
	if id, err := s.Invites().Redeem(ctx, org.ID, h, time.Now()); err != nil || id != camp.ID {
		t.Fatalf("a member just goes in: %d %v", id, err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE campaigns SET status = 'active' WHERE id = $1`, camp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Invites().Redeem(ctx, late.ID, h, time.Now()); err != nil {
		t.Fatalf("resumed: %v", err)
	}
}

func TestCampaignStatusAndInviteRevocation(t *testing.T) {
	s, ctx := testStore(t)
	org := testUser(t, s, ctx, "strorg")
	camp, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Status"))
	if err != nil {
		t.Fatal(err)
	}
	mk := func(label string) string {
		h := core.HashInviteToken(label + time.Now().String())
		if err := s.Invites().Create(ctx, core.NewInvite{CampaignID: camp.ID, TokenHash: h, ExpiresAt: time.Now().Add(time.Hour), CreatedBy: org.ID}); err != nil {
			t.Fatal(err)
		}
		return h
	}
	h1, h2 := mk("one"), mk("two")
	d, err := s.Campaigns().Get(ctx, org.ID, camp.ID)
	if err != nil || d.ActiveInvites != 2 || d.OpenMatches != 0 || d.PendingAftermaths != 0 {
		t.Fatalf("counts: %+v %v", d, err)
	}
	if n, err := s.Invites().Revoke(ctx, camp.ID); err != nil || n != 2 {
		t.Fatalf("revoke: %d %v", n, err)
	}
	p := testUser(t, s, ctx, "strplayer")
	if _, err := s.Invites().Redeem(ctx, p.ID, h1, time.Now()); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("a revoked link: %v", err)
	}
	if _, err := s.Invites().Lookup(ctx, p.ID, h2, time.Now()); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("a revoked link in the preview: %v", err)
	}
	fresh := core.HashInviteToken("fresh" + time.Now().String())
	if n, err := s.Invites().Replace(ctx, core.NewInvite{CampaignID: camp.ID, TokenHash: fresh, ExpiresAt: time.Now().Add(time.Hour), CreatedBy: org.ID}); err != nil || n != 0 {
		t.Fatalf("replace: %d %v", n, err)
	}
	if _, err := s.Invites().Redeem(ctx, p.ID, fresh, time.Now()); err != nil {
		t.Errorf("the new link works: %v", err)
	}

	// status: the organiser's id reaches fn; ending revokes the links
	var seen int64
	if err := s.Campaigns().SetStatus(ctx, camp.ID, func(o int64, _ string, cur core.CampaignStatus) (core.CampaignStatus, error) {
		seen = o
		return core.CampaignFinished, nil
	}); err != nil || seen != org.ID {
		t.Fatalf("set status: %v (organiser %d)", err, seen)
	}
	d, _ = s.Campaigns().Get(ctx, org.ID, camp.ID)
	if d.Status != core.CampaignFinished || d.ActiveInvites != 0 {
		t.Errorf("finished: %+v", d)
	}
	boom := errors.New("refused")
	if err := s.Campaigns().SetStatus(ctx, camp.ID, func(int64, string, core.CampaignStatus) (core.CampaignStatus, error) {
		return core.CampaignActive, boom
	}); !errors.Is(err, boom) {
		t.Errorf("fn error: %v", err)
	}
	if d, _ = s.Campaigns().Get(ctx, org.ID, camp.ID); d.Status != core.CampaignFinished {
		t.Error("an error changes nothing")
	}
	if err := s.Campaigns().SetStatus(ctx, camp.ID+99999, func(int64, string, core.CampaignStatus) (core.CampaignStatus, error) { return core.CampaignActive, nil }); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}
}
