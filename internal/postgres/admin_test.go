package postgres

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestAdminUsers(t *testing.T) {
	s, ctx := testStore(t)
	tag := fmt.Sprint(time.Now().UnixNano())
	mk := func(name string) core.User {
		u, err := s.CreateUser(ctx, fmt.Sprintf("%s.%s@example.com", name, tag), name+" "+tag, "hash")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	zed, amy, gone := mk("Zed"), mk("amy"), mk("Gone")
	if _, err := s.Campaigns().Create(ctx, zed, testNewCampaign(t, s, ctx, "Admin "+tag)); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseUser(ctx, gone.ID, "Former player", fmt.Sprintf("closed-%d@invalid", gone.ID), time.Now()); err != nil {
		t.Fatal(err)
	}

	// the search is a case-insensitive substring of name or email; open accounts by name, closed ones last
	rows, total, err := s.ListUsers(ctx, tag, 10, 0)
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("%v %d %v", rows, total, err)
	}
	if rows[0].ID != amy.ID || rows[1].ID != zed.ID || rows[0].Campaigns != 0 || rows[1].Campaigns != 1 || rows[0].Closed || rows[0].CreatedAt.IsZero() {
		t.Fatalf("order or counts: %+v", rows)
	}
	if rows, total, _ = s.ListUsers(ctx, "ZED."+tag, 10, 0); total != 1 || len(rows) != 1 || rows[0].ID != zed.ID {
		t.Fatalf("by email: %+v %d", rows, total)
	}
	if rows, total, _ = s.ListUsers(ctx, tag, 1, 1); total != 2 || len(rows) != 1 || rows[0].ID != zed.ID {
		t.Fatalf("paging: %+v %d", rows, total)
	}
	if rows, total, _ = s.ListUsers(ctx, tag, 1, 5); total != 2 || len(rows) != 0 {
		t.Fatalf("past the end: %+v %d", rows, total)
	}
	if rows, total, _ = s.ListUsers(ctx, "%_\\"+tag, 10, 0); total != 0 || len(rows) != 0 {
		t.Fatalf("wildcards must be literal: %+v %d", rows, total)
	}
	// closed accounts are listed after every open one
	seenClosed, closedOurs := false, false
	for offset := 0; ; offset += 200 { // every page, whatever the size of the database
		page, total, err := s.ListUsers(ctx, "", 200, offset)
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range page {
			if seenClosed && !u.Closed {
				t.Fatalf("open account %d after a closed one", u.ID)
			}
			seenClosed = seenClosed || u.Closed
			closedOurs = closedOurs || (u.Closed && u.ID == gone.ID)
		}
		if offset+len(page) >= total || len(page) == 0 {
			break
		}
	}
	if !closedOurs {
		t.Fatal("the closed account must be listed")
	}

	// by id
	if u, closed, err := s.UserByID(ctx, zed.ID); err != nil || closed || u.Email != zed.Email {
		t.Fatalf("%+v %v %v", u, closed, err)
	}
	if _, closed, err := s.UserByID(ctx, gone.ID); err != nil || !closed {
		t.Fatalf("closed: %v %v", closed, err)
	}
	if _, _, err := s.UserByID(ctx, -1); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if err := s.SetUserPermissionsByID(ctx, amy.ID, core.RoleAdmin, true, core.AuditEntry{}); err != nil {
		t.Fatal(err)
	}
	if u, _, _ := s.UserByID(ctx, amy.ID); u.Role != core.RoleAdmin || !u.CanCreateCampaigns {
		t.Fatalf("%+v", u)
	}
	for _, id := range []int64{-1, gone.ID} {
		if err := s.SetUserPermissionsByID(ctx, id, core.RoleAdmin, false, core.AuditEntry{}); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("%d: %v", id, err)
		}
	}
}

func TestAdminRulesets(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Rulesets()
	uploader := testUser(t, s, ctx, "uploader")
	in := testNewCampaign(t, s, ctx, "Uses files")
	free := testRuleset(core.KindGlossary, "Free "+in.Glossary[:8], "1", `{"free":"`+in.Glossary+`"}`)
	free.UploadedBy = uploader.ID
	deflt := testRuleset(core.KindGlossary, "Default "+in.Glossary[:8], "1", `{"default":"`+in.Glossary+`"}`)
	for _, r := range []core.StoredRuleset{free, deflt} {
		if err := repo.Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	prev, prevErr := repo.Default(ctx, core.KindGlossary)
	if err := repo.SetDefault(ctx, core.KindGlossary, deflt.Hash); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if prevErr == nil {
			_ = repo.SetDefault(ctx, core.KindGlossary, prev.Hash)
		}
	})
	owner := testUser(t, s, ctx, "owner")
	camp, err := s.Campaigns().Create(ctx, owner, in)
	if err != nil {
		t.Fatal(err)
	}

	usage, err := repo.ListUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byHash := map[string]core.RulesetUsage{}
	for _, u := range usage {
		byHash[u.Hash] = u
	}
	if u := byHash[free.Hash]; u.UploadedBy == nil || u.UploadedBy.ID != uploader.ID || u.Size != free.Size || len(u.Campaigns) != 0 || u.Default || u.CreatedAt.IsZero() || u.Kind != core.KindGlossary {
		t.Fatalf("free file: %+v", u)
	}
	if u := byHash[deflt.Hash]; !u.Default || u.UploadedBy != nil {
		t.Fatalf("default file: %+v", u)
	}
	for _, h := range []string{in.Vanilla, in.BattleCompanies, in.Glossary} {
		if u := byHash[h]; len(u.Campaigns) != 1 || u.Campaigns[0] != (core.CampaignRef{ID: camp.ID, Name: "Uses files"}) {
			t.Fatalf("used file %s: %+v", h[:8], u)
		}
	}
	for i := 1; i < len(usage); i++ { // grouped by kind, newest first within a group
		a, b := usage[i-1], usage[i]
		if kindOrder[a.Kind] > kindOrder[b.Kind] || (a.Kind == b.Kind && a.CreatedAt.Before(b.CreatedAt)) {
			t.Fatalf("order: %v before %v", a.Hash[:8], b.Hash[:8])
		}
	}

	// delete: unknown, in use, default, then the free one
	if err := repo.Delete(ctx, "nope"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if err := repo.Delete(ctx, in.Vanilla); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("in use: %v", err)
	}
	if err := repo.Delete(ctx, deflt.Hash); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("default: %v", err)
	}
	if _, err := repo.Info(ctx, in.Vanilla); err != nil {
		t.Fatalf("a refused delete must keep the file: %v", err)
	}
	if err := repo.Delete(ctx, free.Hash); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Info(ctx, free.Hash); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}

	// prune removes unused non-default files only, and counts them
	a := testRuleset(core.KindGlossary, "Prune A "+in.Glossary[:8], "1", `{"pa":"`+in.Glossary+`"}`)
	b := testRuleset(core.KindBattleCompanies, "Prune B "+in.Glossary[:8], "1", `{"pb":"`+in.Glossary+`"}`)
	for _, r := range []core.StoredRuleset{a, b} {
		if err := repo.Insert(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	n, err := repo.DeleteUnused(ctx)
	if err != nil || n < 2 {
		t.Fatalf("pruned %d, %v", n, err)
	}
	for _, h := range []string{a.Hash, b.Hash} {
		if _, err := repo.Info(ctx, h); !errors.Is(err, core.ErrNotFound) {
			t.Fatalf("%s must be pruned: %v", h[:8], err)
		}
	}
	for _, h := range []string{deflt.Hash, in.Vanilla, in.BattleCompanies, in.Glossary} {
		if _, err := repo.Info(ctx, h); err != nil {
			t.Fatalf("%s must survive: %v", h[:8], err)
		}
	}
	if n, err = repo.DeleteUnused(ctx); err != nil || n != 0 {
		t.Fatalf("second prune: %d %v", n, err)
	}
}

// A campaign that appears while a delete runs ends in a refusal, never in a broken campaign.
func TestRulesetDeleteRace(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Rulesets()
	in := testNewCampaign(t, s, ctx, "Race")
	owner := testUser(t, s, ctx, "racer")
	done := make(chan error, 2)
	go func() { _, err := s.Campaigns().Create(ctx, owner, in); done <- err }()
	go func() { done <- repo.Delete(ctx, in.Glossary) }()
	e1, e2 := <-done, <-done
	if e1 != nil && e2 != nil {
		t.Fatalf("one of them must win: %v / %v", e1, e2)
	}
	for _, err := range []error{e1, e2} {
		if err != nil && !errors.Is(err, core.ErrRefused) && !errors.Is(err, core.ErrNotFound) && !isForeignKeyViolation(err) {
			t.Fatalf("unexpected: %v", err)
		}
	}
}

func TestCampaignHandOver(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Campaigns()
	org, player, outsider := testUser(t, s, ctx, "ho-org"), testUser(t, s, ctx, "ho-pl"), testUser(t, s, ctx, "ho-out")
	camp, err := repo.Create(ctx, org, testNewCampaign(t, s, ctx, "Handover"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, player.ID); err != nil {
		t.Fatal(err)
	}
	roles := func() map[int64]string {
		rows, err := s.pool.Query(ctx, `SELECT user_id, role FROM campaign_members WHERE campaign_id = $1`, camp.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		m := map[int64]string{}
		for rows.Next() {
			var id int64
			var r string
			if err := rows.Scan(&id, &r); err != nil {
				t.Fatal(err)
			}
			m[id] = r
		}
		return m
	}
	yes := func(int64) error { return nil }

	if err := repo.HandOver(ctx, -1, player.ID, yes); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown campaign: %v", err)
	}
	if err := repo.HandOver(ctx, camp.ID, outsider.ID, yes); !errors.Is(err, core.ErrRefused) {
		t.Fatalf("non-member: %v", err)
	}
	denied := errors.New("denied")
	var seen int64
	if err := repo.HandOver(ctx, camp.ID, player.ID, func(c int64) error { seen = c; return denied }); err != denied || seen != org.ID {
		t.Fatalf("allowed must see the organiser and veto: %v %d", err, seen)
	}
	if r := roles(); r[org.ID] != "organiser" || r[player.ID] != "player" {
		t.Fatalf("a vetoed hand over must change nothing: %v", r)
	}
	if err := repo.HandOver(ctx, camp.ID, player.ID, yes); err != nil {
		t.Fatal(err)
	}
	if r := roles(); r[org.ID] != "player" || r[player.ID] != "organiser" || len(r) != 2 {
		t.Fatalf("swapped: %v", r)
	}
}

func TestListUsersShowsTheCampaignsAUserOrganises(t *testing.T) {
	s, ctx := testStore(t)
	org, other := testUser(t, s, ctx, "orgl"), testUser(t, s, ctx, "othl")
	c1, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Organised one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Campaigns().Create(ctx, org, testNewCampaign(t, s, ctx, "Organised two")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, c1.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	rows, _, err := s.ListUsers(ctx, org.Email, 10, 0)
	if err != nil || len(rows) != 1 || len(rows[0].Organises) != 2 || rows[0].Organises[0].Name != "Organised two" || rows[0].Organises[1].Players != 2 || rows[0].Organises[1].Status != core.CampaignActive {
		t.Fatalf("organiser: %+v %v", rows, err)
	}
	if rows, _, _ = s.ListUsers(ctx, other.Email, 10, 0); len(rows) != 1 || len(rows[0].Organises) != 0 || rows[0].Campaigns != 1 {
		t.Fatalf("a player organises none: %+v", rows)
	}
}

func TestCampaignMembersIncludeThoseWithoutACompany(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Campaigns()
	org, bob, anna := testUser(t, s, ctx, "mem-org"), testUser(t, s, ctx, "mem-bob"), testUser(t, s, ctx, "mem-anna")
	camp, err := repo.Create(ctx, org, testNewCampaign(t, s, ctx, "Members"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []core.User{bob, anna} {
		if _, err := s.pool.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player')`, camp.ID, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.Members(ctx, camp.ID)
	if err != nil || len(got) != 3 {
		t.Fatalf("Members = %+v, %v", got, err)
	}
	if !got[0].Organiser || got[0].UserID != org.ID {
		t.Errorf("the organiser comes first: %+v", got)
	}
	for _, m := range got {
		if len(m.Companies) != 0 {
			t.Errorf("nobody has a company yet: %+v", m)
		}
	}
	if _, err := repo.Members(ctx, -1); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown campaign: %v", err)
	}
	// a member without a company can take over
	if err := repo.HandOver(ctx, camp.ID, anna.ID, func(int64) error { return nil }); err != nil {
		t.Errorf("hand over to a member without a company: %v", err)
	}
}
