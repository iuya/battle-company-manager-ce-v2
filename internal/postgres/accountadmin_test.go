package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Integration tests of the account administration tables: audit_events, account_invitations, password_recovery_links.

var tokenSeq atomic.Int64

var t0 = time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)

// tokenAndHash makes a distinct fake raw token and its SHA-256, like the core does.
func tokenAndHash(label string) (string, []byte) {
	raw := fmt.Sprintf("raw-%s-%d-%d", label, time.Now().UnixNano(), tokenSeq.Add(1))
	h := sha256.Sum256([]byte(raw))
	return raw, h[:]
}

func countWhere(t *testing.T, s *Store, ctx context.Context, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func newNamed(t *testing.T, repo accountInvitationStore, ctx context.Context, by core.User, email string, now time.Time, ttl time.Duration) (core.AccountInvitation, []byte) {
	t.Helper()
	_, h := tokenAndHash("named")
	inv, err := repo.Create(ctx, core.StoredAccountInvitation{TokenHash: h, Kind: core.InvitationNamed, Email: email, MaxUses: 1, CreatedBy: by.ID, CreatedAt: now, ExpiresAt: now.Add(ttl)},
		core.InvitationCreatedAudit(now, by.ID, core.InvitationNamed, now.Add(ttl), 1, false))
	if err != nil {
		t.Fatal(err)
	}
	return inv, h
}

func newAnon(t *testing.T, repo accountInvitationStore, ctx context.Context, by core.User, max int, now time.Time, ttl time.Duration, can bool) (core.AccountInvitation, []byte) {
	t.Helper()
	_, h := tokenAndHash("anon")
	inv, err := repo.Create(ctx, core.StoredAccountInvitation{TokenHash: h, Kind: core.InvitationAnonymous, MaxUses: max, CanCreateCampaigns: can, CreatedBy: by.ID, CreatedAt: now, ExpiresAt: now.Add(ttl)},
		core.InvitationCreatedAudit(now, by.ID, core.InvitationAnonymous, now.Add(ttl), max, can))
	if err != nil {
		t.Fatal(err)
	}
	return inv, h
}

func register(repo accountInvitationStore, ctx context.Context, h []byte, email, name string, now time.Time) (core.User, error) {
	_, sess := tokenAndHash("session")
	return repo.Register(ctx, core.RegisterAccount{
		TokenHash: h, Email: email, DisplayName: name, PasswordHash: "pw", SessionHash: sess, SessionExpires: now.Add(time.Hour), Now: now,
		Audit: func(userID, invID int64) core.AuditEntry { return core.InvitationUsedAudit(now, userID, invID) },
	})
}

// start runs n functions at the same moment and returns their errors.
func start(n int, f func(i int) error) []error {
	errs := make([]error, n)
	var ready, wg sync.WaitGroup
	gate := make(chan struct{})
	ready.Add(n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ready.Done()
			<-gate
			errs[i] = f(i)
		}()
	}
	ready.Wait()
	close(gate)
	wg.Wait()
	return errs
}

func countErr(errs []error, target error) (ok, matching int) {
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, target):
			matching++
		}
	}
	return
}

func TestAccountTablesRejectBadRows(t *testing.T) {
	s, ctx := testStore(t)
	u := testUser(t, s, ctx, "checks")
	hash := func() []byte { _, h := tokenAndHash("c"); return h }
	bad := map[string]struct {
		sql  string
		args []any
	}{
		"named without email":   {`INSERT INTO account_invitations (token_hash, kind, max_uses, created_by, created_at, expires_at) VALUES ($1,'named',1,$2,$3,$4)`, []any{hash(), u.ID, t0, t0.Add(time.Hour)}},
		"named with 2 uses":     {`INSERT INTO account_invitations (token_hash, kind, email, max_uses, created_by, created_at, expires_at) VALUES ($1,'named','a@b.c',2,$2,$3,$4)`, []any{hash(), u.ID, t0, t0.Add(time.Hour)}},
		"anonymous with mail":   {`INSERT INTO account_invitations (token_hash, kind, email, max_uses, created_by, created_at, expires_at) VALUES ($1,'anonymous','a@b.c',2,$2,$3,$4)`, []any{hash(), u.ID, t0, t0.Add(time.Hour)}},
		"max_uses 0":            {`INSERT INTO account_invitations (token_hash, kind, max_uses, created_by, created_at, expires_at) VALUES ($1,'anonymous',0,$2,$3,$4)`, []any{hash(), u.ID, t0, t0.Add(time.Hour)}},
		"max_uses 51":           {`INSERT INTO account_invitations (token_hash, kind, max_uses, created_by, created_at, expires_at) VALUES ($1,'anonymous',51,$2,$3,$4)`, []any{hash(), u.ID, t0, t0.Add(time.Hour)}},
		"uses above max":        {`INSERT INTO account_invitations (token_hash, kind, max_uses, uses, created_by, created_at, expires_at) VALUES ($1,'anonymous',2,3,$2,$3,$4)`, []any{hash(), u.ID, t0, t0.Add(time.Hour)}},
		"expiry beyond 30 d":    {`INSERT INTO account_invitations (token_hash, kind, max_uses, created_by, created_at, expires_at) VALUES ($1,'anonymous',2,$2,$3,$4)`, []any{hash(), u.ID, t0, t0.Add(31 * 24 * time.Hour)}},
		"expiry before start":   {`INSERT INTO account_invitations (token_hash, kind, max_uses, created_by, created_at, expires_at) VALUES ($1,'anonymous',2,$2,$3,$4)`, []any{hash(), u.ID, t0, t0}},
		"revoked without who":   {`INSERT INTO account_invitations (token_hash, kind, max_uses, created_by, created_at, expires_at, revoked_at) VALUES ($1,'anonymous',2,$2,$3,$4,$3)`, []any{hash(), u.ID, t0, t0.Add(time.Hour)}},
		"short invitation hash": {`INSERT INTO account_invitations (token_hash, kind, max_uses, created_by, created_at, expires_at) VALUES ($1,'anonymous',2,$2,$3,$4)`, []any{[]byte("short"), u.ID, t0, t0.Add(time.Hour)}},
		"short recovery hash":   {`INSERT INTO password_recovery_links (user_id, token_hash, created_by, created_at, expires_at) VALUES ($1,$2,$1,$3,$4)`, []any{u.ID, []byte("short"), t0, t0.Add(time.Hour)}},
		"recovery beyond 7 d":   {`INSERT INTO password_recovery_links (user_id, token_hash, created_by, created_at, expires_at) VALUES ($1,$2,$1,$3,$4)`, []any{u.ID, hash(), t0, t0.Add(8 * 24 * time.Hour)}},
		"two terminal flags":    {`INSERT INTO password_recovery_links (user_id, token_hash, created_by, created_at, expires_at, used_at, invalidated_at, invalidated_reason) VALUES ($1,$2,$1,$3,$4,$3,$3,'login')`, []any{u.ID, hash(), t0, t0.Add(time.Hour)}},
		"invalidated no reason": {`INSERT INTO password_recovery_links (user_id, token_hash, created_by, created_at, expires_at, invalidated_at) VALUES ($1,$2,$1,$3,$4,$3)`, []any{u.ID, hash(), t0, t0.Add(time.Hour)}},
		"unknown reason":        {`INSERT INTO password_recovery_links (user_id, token_hash, created_by, created_at, expires_at, invalidated_at, invalidated_reason) VALUES ($1,$2,$1,$3,$4,$3,'because')`, []any{u.ID, hash(), t0, t0.Add(time.Hour)}},
		"web audit no actor":    {`INSERT INTO audit_events (at, source, action) VALUES ($1,'web','role.changed')`, []any{t0}},
		"unknown audit action":  {`INSERT INTO audit_events (at, source, action) VALUES ($1,'system','bogus')`, []any{t0}},
		"audit kind no id":      {`INSERT INTO audit_events (at, source, action, target_kind) VALUES ($1,'system','recovery.used','recovery_link')`, []any{t0}},
	}
	for name, b := range bad {
		if _, err := s.pool.Exec(ctx, b.sql, b.args...); err == nil {
			t.Errorf("%s: the database accepted it", name)
		}
	}
	// two pending links for one user are refused; once the first is finished the second may exist
	ins := `INSERT INTO password_recovery_links (user_id, token_hash, created_by, created_at, expires_at) VALUES ($1,$2,$1,$3,$4)`
	if _, err := s.pool.Exec(ctx, ins, u.ID, hash(), t0, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, ins, u.ID, hash(), t0, t0.Add(time.Hour)); err == nil {
		t.Fatal("two pending links for one user")
	}
	if _, err := s.pool.Exec(ctx, `UPDATE password_recovery_links SET used_at = $2 WHERE user_id = $1`, u.ID, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, ins, u.ID, hash(), t0, t0.Add(time.Hour)); err != nil {
		t.Fatalf("a finished link must not block a new one: %v", err)
	}
}

func TestSetUserPermissionsAudited(t *testing.T) {
	s, ctx := testStore(t)
	actor, target := testUser(t, s, ctx, "actor"), testUser(t, s, ctx, "target")
	aid := actor.ID
	audit := core.RoleChangedAudit(t0, core.AuditWeb, &aid, target.ID, core.RoleUser, core.RoleAdmin, false, true)
	if err := s.SetUserPermissionsByID(ctx, target.ID, core.RoleAdmin, true, audit); err != nil {
		t.Fatal(err)
	}
	var source, action, detail string
	var at time.Time
	var gotActor int64
	err := s.pool.QueryRow(ctx, `SELECT at, source, actor_id, action, detail::text FROM audit_events WHERE subject_user_id = $1`, target.ID).Scan(&at, &source, &gotActor, &action, &detail)
	if err != nil || !at.Equal(t0) || source != "web" || gotActor != actor.ID || action != "role.changed" || !strings.Contains(detail, `"to_role": "admin"`) {
		t.Fatalf("%v %v %s %d %s %s", err, at, source, gotActor, action, detail)
	}
	// console: no actor; an unknown user writes no audit entry
	if err := s.SetUserPermissionsByID(ctx, target.ID, core.RoleUser, false, core.RoleChangedAudit(t0, core.AuditConsole, nil, target.ID, core.RoleAdmin, core.RoleUser, true, false)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserPermissionsByID(ctx, -1, core.RoleUser, false, core.RoleChangedAudit(t0, core.AuditConsole, nil, -1, core.RoleAdmin, core.RoleUser, true, false)); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events WHERE subject_user_id IN ($1, -1)`, target.ID); n != 2 {
		t.Fatalf("audit rows: %d", n)
	}
	// an invalid audit entry (web without actor) rolls the change back
	bad := core.RoleChangedAudit(t0, core.AuditWeb, nil, target.ID, core.RoleUser, core.RoleAdmin, false, false)
	if err := s.SetUserPermissionsByID(ctx, target.ID, core.RoleAdmin, false, bad); err == nil {
		t.Fatal("the audit insert must fail")
	}
	if u, _, _ := s.UserByID(ctx, target.ID); u.Role != core.RoleUser {
		t.Fatalf("the change must roll back with its audit: %+v", u)
	}
}

func TestAccountInvitationsLifecycle(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.AccountInvitations()
	admin := testUser(t, s, ctx, "inviter")
	email := fmt.Sprintf("guest-%d@example.com", time.Now().UnixNano())

	named, nh := newNamed(t, repo, ctx, admin, email, t0, 24*time.Hour)
	anon, ah := newAnon(t, repo, ctx, admin, 3, t0.Add(time.Minute), 48*time.Hour, true)
	if named.ID == 0 || named.State != core.InvitationPending || named.CreatedByName != "inviter" || named.Email != email || anon.Email != "" || anon.MaxUses != 3 {
		t.Fatalf("%+v %+v", named, anon)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events WHERE action = 'invitation.created' AND target_id = ANY($1)`, []int64{named.ID, anon.ID}); n != 2 {
		t.Fatalf("created audit rows: %d", n)
	}
	if has, err := repo.EmailHasAccount(ctx, strings.ToUpper(email)); err != nil || has {
		t.Fatalf("%v %v", has, err)
	}

	// peek: the stored hash, the kind, the address, the state at now; expiry == now is expired
	p, err := repo.Peek(ctx, nh, t0.Add(time.Hour))
	if err != nil || string(p.TokenHash) != string(nh) || p.Kind != core.InvitationNamed || p.Email != email || p.State != core.InvitationPending {
		t.Fatalf("%+v %v", p, err)
	}
	if p, _ := repo.Peek(ctx, nh, t0.Add(24*time.Hour-time.Nanosecond)); p.State != core.InvitationPending {
		t.Fatalf("one tick before expiry: %s", p.State)
	}
	if p, _ := repo.Peek(ctx, nh, t0.Add(24*time.Hour)); p.State != core.InvitationExpired {
		t.Fatalf("expires == now must be expired, got %s", p.State)
	}
	if _, err := repo.Peek(ctx, []byte("nope"), t0); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := register(repo, ctx, nh, "", "Late", t0.Add(24*time.Hour)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("registering at the expiry instant: %v", err)
	}

	// a named registration ignores the address in the request and uses the stored one
	u, err := register(repo, ctx, nh, "someone-else@example.com", "Guest", t0.Add(time.Hour))
	if err != nil || u.Email != email || u.Role != core.RoleUser || u.CanCreateCampaigns {
		t.Fatalf("%+v %v", u, err)
	}
	if has, _ := repo.EmailHasAccount(ctx, strings.ToUpper(email)); !has {
		t.Fatal("the account must exist")
	}
	if _, err := register(repo, ctx, nh, "", "Again", t0.Add(time.Hour)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a used named invitation: %v", err)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM sessions WHERE user_id = $1`, u.ID); n != 1 {
		t.Fatalf("sessions: %d", n)
	}
	var used time.Time
	var source string
	var actor *int64
	if err := s.pool.QueryRow(ctx, `SELECT x.used_at, a.source, a.actor_id FROM account_invitation_uses x JOIN audit_events a ON a.action = 'invitation.used' AND a.subject_user_id = x.user_id AND a.target_id = x.invitation_id WHERE x.user_id = $1`, u.ID).Scan(&used, &source, &actor); err != nil ||
		!used.Equal(t0.Add(time.Hour)) || source != "public" || actor != nil {
		t.Fatalf("%v %v %s %v", err, used, source, actor)
	}

	// anonymous: the address is the visitor's, the flag comes from the invitation, uses counts, the list shows the accounts
	au, err := register(repo, ctx, ah, fmt.Sprintf("anon-%d@example.com", time.Now().UnixNano()), "Anon", t0.Add(2*time.Minute))
	if err != nil || !au.CanCreateCampaigns {
		t.Fatalf("%+v %v", au, err)
	}
	list, total, err := repo.List(ctx, core.InvitationFilter{}, t0.Add(2*time.Minute), 200, 0)
	if err != nil || total < 2 {
		t.Fatalf("%d %v", total, err)
	}
	got := map[int64]core.AccountInvitation{}
	for _, inv := range list {
		got[inv.ID] = inv
	}
	if a := got[anon.ID]; a.Uses != 1 || a.State != core.InvitationPending || len(a.Accounts) != 1 || a.Accounts[0].UserID != au.ID || a.Accounts[0].DisplayName != "Anon" {
		t.Fatalf("%+v", a)
	}
	if n := got[named.ID]; n.State != core.InvitationUsed || len(n.Accounts) != 1 {
		t.Fatalf("%+v", n)
	}
	// newest first
	if list[0].CreatedAt.Before(list[len(list)-1].CreatedAt) {
		t.Fatal("not newest first")
	}
	// the filter and the paging use the same state
	only, n, err := repo.List(ctx, core.InvitationFilter{State: core.InvitationUsed}, t0.Add(2*time.Minute), 200, 0)
	if err != nil || n < 1 {
		t.Fatal(n, err)
	}
	for _, inv := range only {
		if inv.State != core.InvitationUsed {
			t.Fatalf("filter leaked %s", inv.State)
		}
	}
	if page, tot, err := repo.List(ctx, core.InvitationFilter{}, t0, 1, 0); err != nil || len(page) != 1 || tot != total {
		t.Fatalf("%d %d %v", len(page), tot, err)
	}
	if exp, _, _ := repo.List(ctx, core.InvitationFilter{State: core.InvitationExpired}, t0.Add(100*24*time.Hour), 200, 0); len(exp) < 1 {
		t.Fatal("expired filter")
	}

	// revoke: only while pending; the state is reported otherwise
	rv, err := repo.Revoke(ctx, anon.ID, admin.ID, t0.Add(3*time.Minute), core.InvitationRevokedAudit(t0.Add(3*time.Minute), admin.ID, anon.ID))
	if err != nil || rv.State != core.InvitationRevoked || rv.RevokedBy == nil || *rv.RevokedBy != admin.ID || rv.RevokedByName != "inviter" {
		t.Fatalf("%+v %v", rv, err)
	}
	var np *core.NotPendingError
	if _, err := repo.Revoke(ctx, anon.ID, admin.ID, t0, core.InvitationRevokedAudit(t0, admin.ID, anon.ID)); !errors.As(err, &np) || np.State != core.InvitationRevoked {
		t.Fatalf("%v", err)
	}
	if _, err := repo.Revoke(ctx, named.ID, admin.ID, t0, core.InvitationRevokedAudit(t0, admin.ID, named.ID)); !errors.As(err, &np) || np.State != core.InvitationUsed {
		t.Fatalf("%v", err)
	}
	if _, err := repo.Revoke(ctx, -5, admin.ID, t0, core.InvitationRevokedAudit(t0, admin.ID, -5)); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events WHERE action = 'invitation.revoked' AND target_id = $1`, anon.ID); n != 1 {
		t.Fatalf("revoked audit rows: %d", n)
	}
	if _, err := register(repo, ctx, ah, fmt.Sprintf("r-%d@example.com", time.Now().UnixNano()), "Late", t0.Add(4*time.Minute)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a revoked invitation: %v", err)
	}
	// an expired invitation cannot be revoked
	exp, _ := newAnon(t, repo, ctx, admin, 1, t0, time.Hour, false)
	if _, err := repo.Revoke(ctx, exp.ID, admin.ID, t0.Add(time.Hour), core.InvitationRevokedAudit(t0, admin.ID, exp.ID)); !errors.As(err, &np) || np.State != core.InvitationExpired {
		t.Fatalf("%v", err)
	}
}

func TestRegisterIsAtomic(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.AccountInvitations()
	admin := testUser(t, s, ctx, "atomic")
	taken := testUser(t, s, ctx, "taken")
	now := t0.Add(time.Hour)

	// an address that already has an account: ErrEmailTaken, nothing consumed
	anon, ah := newAnon(t, repo, ctx, admin, 2, t0, 24*time.Hour, false)
	if _, err := register(repo, ctx, ah, strings.ToUpper(taken.Email), "Dup", now); !errors.Is(err, core.ErrEmailTaken) {
		t.Fatalf("%v", err)
	}
	// a failure at a later step (a session hash that already exists) rolls the new user back as well
	_, dupSess := tokenAndHash("dup")
	if err := s.CreateSession(ctx, dupSess, admin.ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	failEmail := fmt.Sprintf("fail-%d@example.com", time.Now().UnixNano())
	_, err := repo.Register(ctx, core.RegisterAccount{TokenHash: ah, Email: failEmail, DisplayName: "Fail", PasswordHash: "pw", SessionHash: dupSess, SessionExpires: now.Add(time.Hour), Now: now,
		Audit: func(u, i int64) core.AuditEntry { return core.InvitationUsedAudit(now, u, i) }})
	if err == nil {
		t.Fatal("must fail on the duplicate session")
	}
	// a failing audit entry (bad action) rolls everything back too
	badAudit := core.RegisterAccount{TokenHash: ah, Email: failEmail, DisplayName: "Fail", PasswordHash: "pw", SessionHash: []byte("x-" + failEmail), SessionExpires: now.Add(time.Hour), Now: now,
		Audit: func(u, i int64) core.AuditEntry {
			e := core.InvitationUsedAudit(now, u, i)
			e.Action = "bogus"
			return e
		}}
	if _, err := repo.Register(ctx, badAudit); err == nil {
		t.Fatal("must fail on the audit entry")
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM users WHERE lower(email) = $1`, failEmail); n != 0 {
		t.Fatal("the user must be rolled back")
	}
	if n := countWhere(t, s, ctx, `SELECT uses FROM account_invitations WHERE id = $1`, anon.ID); n != 0 {
		t.Fatalf("uses must be unchanged, got %d", n)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM account_invitation_uses WHERE invitation_id = $1`, anon.ID); n != 0 {
		t.Fatal("no use row")
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events WHERE action = 'invitation.used' AND target_id = $1`, anon.ID); n != 0 {
		t.Fatal("no audit row")
	}
	// and the invitation still works
	if _, err := register(repo, ctx, ah, failEmail, "Fine", now); err != nil {
		t.Fatalf("still usable: %v", err)
	}
}

func TestRegisterConcurrently(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.AccountInvitations()
	admin := testUser(t, s, ctx, "race")
	now := t0.Add(time.Minute)
	stamp := time.Now().UnixNano()

	// two simultaneous registrations on a named invitation: one account
	named, nh := newNamed(t, repo, ctx, admin, fmt.Sprintf("race-%d@example.com", stamp), t0, time.Hour)
	errs := start(2, func(i int) error { _, err := register(repo, ctx, nh, "", fmt.Sprintf("N%d", i), now); return err })
	if ok, nf := countErr(errs, core.ErrNotFound); ok != 1 || nf != 1 {
		t.Fatalf("named: %v", errs)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM account_invitation_uses WHERE invitation_id = $1`, named.ID); n != 1 {
		t.Fatalf("uses rows: %d", n)
	}

	// five on max_uses = 2: exactly two
	anon, ah := newAnon(t, repo, ctx, admin, 2, t0, time.Hour, false)
	errs = start(5, func(i int) error {
		_, err := register(repo, ctx, ah, fmt.Sprintf("many-%d-%d@example.com", stamp, i), fmt.Sprintf("M%d", i), now)
		return err
	})
	if ok, nf := countErr(errs, core.ErrNotFound); ok != 2 || nf != 3 {
		t.Fatalf("anonymous: %v", errs)
	}
	if n := countWhere(t, s, ctx, `SELECT uses FROM account_invitations WHERE id = $1`, anon.ID); n != 2 {
		t.Fatalf("uses: %d", n)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events WHERE action = 'invitation.used' AND target_id = $1`, anon.ID); n != 2 {
		t.Fatalf("audit rows: %d", n)
	}

	// two simultaneous registrations of the same address on an anonymous invitation: one account, the other ErrEmailTaken
	dup, dh := newAnon(t, repo, ctx, admin, 5, t0, time.Hour, false)
	same := fmt.Sprintf("same-%d@example.com", stamp)
	errs = start(2, func(i int) error { _, err := register(repo, ctx, dh, same, "S", now); return err })
	if ok, tk := countErr(errs, core.ErrEmailTaken); ok != 1 || tk != 1 {
		t.Fatalf("same address: %v", errs)
	}
	if n := countWhere(t, s, ctx, `SELECT uses FROM account_invitations WHERE id = $1`, dup.ID); n != 1 {
		t.Fatalf("uses after the refused one: %d", n)
	}

	// revoke racing the last registration: exactly one wins, and the outcome is consistent
	for round := range 5 {
		inv, h := newAnon(t, repo, ctx, admin, 1, t0, time.Hour, false)
		errs = start(2, func(i int) error {
			if i == 0 {
				_, err := register(repo, ctx, h, fmt.Sprintf("rv-%d-%d@example.com", stamp, round), "R", now)
				return err
			}
			_, err := repo.Revoke(ctx, inv.ID, admin.ID, now, core.InvitationRevokedAudit(now, admin.ID, inv.ID))
			return err
		})
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatalf("exactly one must win: %v", errs)
		}
		uses := countWhere(t, s, ctx, `SELECT uses FROM account_invitations WHERE id = $1`, inv.ID)
		revoked := countWhere(t, s, ctx, `SELECT count(*) FROM account_invitations WHERE id = $1 AND revoked_at IS NOT NULL`, inv.ID)
		if (errs[0] == nil) != (uses == 1) || (errs[1] == nil) != (revoked == 1) {
			t.Fatalf("round %d: %v uses=%d revoked=%d", round, errs, uses, revoked)
		}
	}
}

func newLink(t *testing.T, repo recoveryStore, ctx context.Context, user, by core.User, now time.Time, ttl time.Duration) (core.RecoveryLink, string, []byte) {
	t.Helper()
	raw, h := tokenAndHash("rec")
	exp := now.Add(ttl)
	link, err := repo.Create(ctx, core.StoredRecovery{UserID: user.ID, TokenHash: h, CreatedBy: by.ID, CreatedAt: now, ExpiresAt: exp}, core.RecoveryCreatedAudit(now, by.ID, user.ID, exp))
	if err != nil {
		t.Fatal(err)
	}
	return link, raw, h
}

func useLink(repo recoveryStore, ctx context.Context, h []byte, pw string, now time.Time) (int64, error) {
	return repo.Use(ctx, h, pw, now, func(u, l int64) core.AuditEntry { return core.RecoveryUsedAudit(now, u, l) })
}

func password(t *testing.T, s *Store, ctx context.Context, id int64) string {
	t.Helper()
	c, err := s.CredentialsByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return c.PasswordHash
}

func TestRecoveryLifecycle(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Recovery()
	su := testUser(t, s, ctx, "root")
	victim := testUser(t, s, ctx, "victim")
	other := testUser(t, s, ctx, "other")

	link, _, h := newLink(t, repo, ctx, victim, su, t0, time.Hour)
	if link.ID == 0 || link.UserID != victim.ID || !link.ExpiresAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("%+v", link)
	}
	var replaced string
	if err := s.pool.QueryRow(ctx, `SELECT detail->>'replaced' FROM audit_events WHERE action = 'recovery.created' AND target_id = $1`, link.ID).Scan(&replaced); err != nil || replaced != "false" {
		t.Fatalf("%v %s", err, replaced)
	}
	// errors
	if _, err := repo.Create(ctx, core.StoredRecovery{UserID: -1, TokenHash: h, CreatedBy: su.ID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}, core.RecoveryCreatedAudit(t0, su.ID, -1, t0)); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}

	// the session flag: true only for a live unexpired link
	sess := []byte("rec-session-" + victim.Email)
	if err := s.CreateSession(ctx, sess, victim.ID, t0.Add(100*time.Hour)); err != nil {
		t.Fatal(err)
	}
	osess := []byte("rec-session-" + other.Email)
	if err := s.CreateSession(ctx, osess, other.ID, t0.Add(100*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		at   time.Time
		want bool
	}{{t0, true}, {t0.Add(time.Hour - time.Nanosecond), true}, {t0.Add(time.Hour), false}, {t0.Add(2 * time.Hour), false}} {
		if _, _, pending, err := s.UserBySession(ctx, sess, c.at); err != nil || pending != c.want {
			t.Fatalf("at %v: pending=%v err=%v", c.at, pending, err)
		}
	}
	if _, _, pending, _ := s.UserBySession(ctx, osess, t0); pending {
		t.Fatal("another user's link")
	}

	// peek
	p, err := repo.Peek(ctx, h, t0)
	if err != nil || p.UserID != victim.ID || p.State != core.RecoveryPending || string(p.TokenHash) != string(h) {
		t.Fatalf("%+v %v", p, err)
	}
	if p, _ := repo.Peek(ctx, h, t0.Add(time.Hour)); p.State != core.RecoveryExpired {
		t.Fatalf("expires == now must be expired: %s", p.State)
	}
	if _, err := repo.Peek(ctx, []byte("nope"), t0); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
	// use at the expiry instant fails and changes nothing
	if _, err := useLink(repo, ctx, h, "late", t0.Add(time.Hour)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if password(t, s, ctx, victim.ID) != "hash" {
		t.Fatal("password changed by an expired link")
	}

	// replace: the old link is invalidated 'replaced', with its audit entry, and no longer usable
	link2, _, h2 := newLink(t, repo, ctx, victim, su, t0.Add(time.Minute), time.Hour)
	if p, _ := repo.Peek(ctx, h, t0.Add(time.Minute)); p.State != core.RecoveryInvalidated {
		t.Fatalf("replaced link: %s", p.State)
	}
	var reason string
	if err := s.pool.QueryRow(ctx, `SELECT invalidated_reason FROM password_recovery_links WHERE id = $1`, link.ID).Scan(&reason); err != nil || reason != "replaced" {
		t.Fatalf("%v %s", err, reason)
	}
	if err := s.pool.QueryRow(ctx, `SELECT detail->>'reason' FROM audit_events WHERE action = 'recovery.invalidated' AND target_id = $1 AND source = 'system'`, link.ID).Scan(&reason); err != nil || reason != "replaced" {
		t.Fatalf("%v %s", err, reason)
	}
	if err := s.pool.QueryRow(ctx, `SELECT detail->>'replaced' FROM audit_events WHERE action = 'recovery.created' AND target_id = $1`, link2.ID).Scan(&replaced); err != nil || replaced != "false" {
		// link was already expired at the time link2 was made (t0+1min < t0+1h: still live), so replaced must be true
		if replaced != "true" {
			t.Fatalf("%v %s", err, replaced)
		}
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM password_recovery_links WHERE user_id = $1 AND used_at IS NULL AND revoked_at IS NULL AND invalidated_at IS NULL`, victim.ID); n != 1 {
		t.Fatalf("one live row: %d", n)
	}
	// an expired but unmarked link is replaced too (and does not block the unique index)
	_, _, h3 := newLink(t, repo, ctx, victim, su, t0.Add(3*time.Hour), time.Hour)
	if p, _ := repo.Peek(ctx, h2, t0.Add(3*time.Hour)); p.State != core.RecoveryInvalidated {
		t.Fatalf("expired unmarked link: %s", p.State)
	}

	// revoke: the pending link only
	if err := repo.Revoke(ctx, other.ID, su.ID, t0, core.RecoveryRevokedAudit(t0, su.ID, other.ID)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("no link: %v", err)
	}
	if err := repo.Revoke(ctx, -1, su.ID, t0, core.RecoveryRevokedAudit(t0, su.ID, -1)); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
	if err := repo.Revoke(ctx, victim.ID, su.ID, t0.Add(4*time.Hour), core.RecoveryRevokedAudit(t0, su.ID, victim.ID)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("an expired link is not revocable: %v", err)
	}
	if err := repo.Revoke(ctx, victim.ID, su.ID, t0.Add(3*time.Hour+time.Minute), core.RecoveryRevokedAudit(t0.Add(3*time.Hour+time.Minute), su.ID, victim.ID)); err != nil {
		t.Fatal(err)
	}
	if p, _ := repo.Peek(ctx, h3, t0.Add(3*time.Hour+time.Minute)); p.State != core.RecoveryRevoked {
		t.Fatalf("%s", p.State)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events a JOIN password_recovery_links l ON l.id = a.target_id WHERE a.action = 'recovery.revoked' AND l.token_hash = $1 AND a.actor_id = $2`, h3, su.ID); n != 1 {
		t.Fatalf("revoked audit: %d", n)
	}
	if _, err := useLink(repo, ctx, h3, "x", t0.Add(3*time.Hour+time.Minute)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a revoked link: %v", err)
	}

	// use: new password, every session of the user gone, the other user's sessions kept, audit, single use
	_, _, h4 := newLink(t, repo, ctx, victim, su, t0.Add(5*time.Hour), time.Hour)
	uid, err := useLink(repo, ctx, h4, "brand-new-hash", t0.Add(5*time.Hour+time.Minute))
	if err != nil || uid != victim.ID {
		t.Fatalf("%d %v", uid, err)
	}
	if password(t, s, ctx, victim.ID) != "brand-new-hash" {
		t.Fatal("password not set")
	}
	if _, _, _, err := s.UserBySession(ctx, sess, t0); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("sessions must be deleted: %v", err)
	}
	if _, _, _, err := s.UserBySession(ctx, osess, t0); err != nil {
		t.Fatalf("another user's session: %v", err)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events a JOIN password_recovery_links l ON l.id = a.target_id WHERE a.action = 'recovery.used' AND l.token_hash = $1 AND a.source = 'public' AND a.actor_id IS NULL AND a.subject_user_id = $2`, h4, victim.ID); n != 1 {
		t.Fatalf("used audit: %d", n)
	}
	if _, err := useLink(repo, ctx, h4, "again", t0.Add(6*time.Hour)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("second use: %v", err)
	}
	if p, _ := repo.Peek(ctx, h4, t0.Add(5*time.Hour+time.Minute)); p.State != core.RecoveryUsed {
		t.Fatal(p.State)
	}
	if _, err := useLink(repo, ctx, []byte("unknown"), "x", t0); !errors.Is(err, core.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestRecoveryInvalidationAndClosing(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Recovery()
	su := testUser(t, s, ctx, "root2")
	a, b, c := testUser(t, s, ctx, "inv-a"), testUser(t, s, ctx, "inv-b"), testUser(t, s, ctx, "inv-c")

	// nothing to invalidate: n = 0, no audit row
	if n, err := s.InvalidateRecovery(ctx, a.ID, core.RecoveryByLogin, t0); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	la, _, ha := newLink(t, repo, ctx, a, su, t0, time.Hour)
	lb, _, hb := newLink(t, repo, ctx, b, su, t0, time.Hour)
	if n, err := s.InvalidateRecovery(ctx, a.ID, core.RecoveryByLogin, t0.Add(time.Minute)); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if n, _ := s.InvalidateRecovery(ctx, a.ID, core.RecoveryByLogin, t0.Add(2*time.Minute)); n != 0 {
		t.Fatal("second invalidation must change nothing")
	}
	var reason, source string
	var at time.Time
	if err := s.pool.QueryRow(ctx, `SELECT at, source, detail->>'reason' FROM audit_events WHERE action = 'recovery.invalidated' AND target_id = $1`, la.ID).Scan(&at, &source, &reason); err != nil ||
		!at.Equal(t0.Add(time.Minute)) || source != "system" || reason != "login" {
		t.Fatalf("%v %v %s %s", err, at, source, reason)
	}
	if p, _ := repo.Peek(ctx, ha, t0); p.State != core.RecoveryInvalidated {
		t.Fatal(p.State)
	}
	if _, err := useLink(repo, ctx, ha, "x", t0.Add(3*time.Minute)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("%v", err)
	}
	// b is untouched; an expired link is not touched either
	if p, _ := repo.Peek(ctx, hb, t0); p.State != core.RecoveryPending {
		t.Fatal(p.State)
	}
	if n, _ := s.InvalidateRecovery(ctx, b.ID, core.RecoveryByAccess, t0.Add(time.Hour)); n != 0 {
		t.Fatal("expired link counted")
	}
	if n, _ := s.InvalidateRecovery(ctx, b.ID, core.RecoveryByAccess, t0.Add(time.Minute)); n != 1 {
		t.Fatal("access")
	}
	_ = lb

	// closing the account invalidates its pending link as 'closed'
	lc, _, hc := newLink(t, repo, ctx, c, su, t0, time.Hour)
	if err := s.CloseUser(ctx, c.ID, "Closed", fmt.Sprintf("closed-%d@closed.invalid", c.ID), t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT invalidated_reason FROM password_recovery_links WHERE id = $1`, lc.ID).Scan(&reason); err != nil || reason != "closed" {
		t.Fatalf("%v %s", err, reason)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events WHERE action = 'recovery.invalidated' AND target_id = $1 AND detail->>'reason' = 'closed'`, lc.ID); n != 1 {
		t.Fatalf("closed audit: %d", n)
	}
	if _, err := useLink(repo, ctx, hc, "x", t0.Add(2*time.Minute)); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("%v", err)
	}
	// a closed account gets no link
	if _, err := repo.Create(ctx, core.StoredRecovery{UserID: c.ID, TokenHash: func() []byte { _, h := tokenAndHash("x"); return h }(), CreatedBy: su.ID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)}, core.RecoveryCreatedAudit(t0, su.ID, c.ID, t0.Add(time.Hour))); !errors.Is(err, core.ErrAccountClosed) {
		t.Fatalf("%v", err)
	}
}

func TestRecoveryConcurrently(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Recovery()
	su := testUser(t, s, ctx, "root3")

	for round := range 5 {
		// two simultaneous uses of one link: exactly one wins
		u := testUser(t, s, ctx, fmt.Sprintf("two-uses-%d", round))
		_, _, h := newLink(t, repo, ctx, u, su, t0, time.Hour)
		now := t0.Add(time.Minute)
		errs := start(2, func(i int) error { _, err := useLink(repo, ctx, h, fmt.Sprintf("pw%d", i), now); return err })
		if ok, nf := countErr(errs, core.ErrNotFound); ok != 1 || nf != 1 {
			t.Fatalf("two uses: %v", errs)
		}
		if got := password(t, s, ctx, u.ID); got != "pw0" && got != "pw1" {
			t.Fatal(got)
		}
		if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events a JOIN password_recovery_links l ON l.id = a.target_id WHERE a.action = 'recovery.used' AND l.user_id = $1`, u.ID); n != 1 {
			t.Fatalf("used audit rows: %d", n)
		}

		// use racing an invalidation: exactly one wins and the link is finished one way only
		v := testUser(t, s, ctx, fmt.Sprintf("use-vs-inv-%d", round))
		_, _, hv := newLink(t, repo, ctx, v, su, t0, time.Hour)
		var n int
		errs = start(2, func(i int) error {
			if i == 0 {
				_, err := useLink(repo, ctx, hv, "won", now)
				return err
			}
			var err error
			n, err = s.InvalidateRecovery(ctx, v.ID, core.RecoveryByAccess, now)
			return err
		})
		if errs[1] != nil {
			t.Fatal(errs[1])
		}
		used := errs[0] == nil
		if used == (n == 1) {
			t.Fatalf("use=%v invalidated=%d: exactly one must win", used, n)
		}
		if used != (password(t, s, ctx, v.ID) == "won") {
			t.Fatal("password and outcome disagree")
		}

		// use racing a revoke
		w := testUser(t, s, ctx, fmt.Sprintf("use-vs-rev-%d", round))
		_, _, hw := newLink(t, repo, ctx, w, su, t0, time.Hour)
		errs = start(2, func(i int) error {
			if i == 0 {
				_, err := useLink(repo, ctx, hw, "won", now)
				return err
			}
			return repo.Revoke(ctx, w.ID, su.ID, now, core.RecoveryRevokedAudit(now, su.ID, w.ID))
		})
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatalf("use vs revoke: %v", errs)
		}

		// use racing the closing of the account
		x := testUser(t, s, ctx, fmt.Sprintf("use-vs-close-%d", round))
		_, _, hx := newLink(t, repo, ctx, x, su, t0, time.Hour)
		errs = start(2, func(i int) error {
			if i == 0 {
				_, err := useLink(repo, ctx, hx, "won", now)
				return err
			}
			return s.CloseUser(ctx, x.ID, "Gone", fmt.Sprintf("gone-%d@closed.invalid", x.ID), now)
		})
		if errs[1] != nil {
			t.Fatal(errs[1])
		}
		var st string
		if err := s.pool.QueryRow(ctx, `SELECT CASE WHEN used_at IS NOT NULL THEN 'used' WHEN invalidated_reason = 'closed' THEN 'closed' ELSE 'other' END FROM password_recovery_links WHERE token_hash = $1`, hx).Scan(&st); err != nil {
			t.Fatal(err)
		}
		if (errs[0] == nil) != (st == "used") || (errs[0] != nil && st != "closed") {
			t.Fatalf("use vs close: %v, link %s", errs, st)
		}

		// two simultaneous Create for one user: one live row; the loser replaces it or gets a clean conflict
		y := testUser(t, s, ctx, fmt.Sprintf("two-creates-%d", round))
		errs = start(2, func(i int) error {
			_, h := tokenAndHash(fmt.Sprintf("create-%d", i))
			exp := now.Add(time.Hour)
			_, err := repo.Create(ctx, core.StoredRecovery{UserID: y.ID, TokenHash: h, CreatedBy: su.ID, CreatedAt: now, ExpiresAt: exp}, core.RecoveryCreatedAudit(now, su.ID, y.ID, exp))
			return err
		})
		for _, e := range errs {
			if e != nil && !errors.Is(e, core.ErrConflict) {
				t.Fatalf("two creates: %v", errs)
			}
		}
		if n := countWhere(t, s, ctx, `SELECT count(*) FROM password_recovery_links WHERE user_id = $1 AND used_at IS NULL AND revoked_at IS NULL AND invalidated_at IS NULL`, y.ID); n != 1 {
			t.Fatalf("live rows: %d", n)
		}
	}
}

func TestListUsersLatestRecovery(t *testing.T) {
	s, ctx := testStore(t)
	repo := s.Recovery()
	su := testUser(t, s, ctx, "Lister Root")
	tag := fmt.Sprintf("zzlist-%d", time.Now().UnixNano())
	u := testUser(t, s, ctx, tag)
	find := func() core.AdminUser {
		rows, _, err := s.ListUsers(ctx, tag, 10, 0)
		if err != nil || len(rows) != 1 {
			t.Fatalf("%d %v", len(rows), err)
		}
		return rows[0]
	}
	if find().Recovery != nil {
		t.Fatal("never had a link")
	}
	l1, _, _ := newLink(t, repo, ctx, u, su, t0, time.Hour)
	l2, _, _ := newLink(t, repo, ctx, u, su, t0.Add(time.Minute), 2*time.Hour)
	r := find().Recovery
	if r == nil || r.ID != l2.ID || r.ID == l1.ID || r.CreatedByName != "Lister Root" || !r.ExpiresAt.Equal(t0.Add(time.Minute+2*time.Hour)) || r.UsedAt != nil || r.RevokedAt != nil || r.InvalidatedAt != nil {
		t.Fatalf("%+v", r)
	}
	if err := repo.Revoke(ctx, u.ID, su.ID, t0.Add(2*time.Minute), core.RecoveryRevokedAudit(t0, su.ID, u.ID)); err != nil {
		t.Fatal(err)
	}
	r = find().Recovery
	if r.RevokedAt == nil || r.RevokedByName != "Lister Root" || r.StateAt(t0.Add(3*time.Minute)) != core.RecoveryRevoked {
		t.Fatalf("%+v", r)
	}
	_, _, h3 := newLink(t, repo, ctx, u, su, t0.Add(time.Hour), time.Hour)
	if _, err := s.InvalidateRecovery(ctx, u.ID, core.RecoveryByLogin, t0.Add(time.Hour+time.Minute)); err != nil {
		t.Fatal(err)
	}
	_ = h3
	r = find().Recovery
	if r.InvalidatedAt == nil || r.Reason != core.RecoveryByLogin || r.StateAt(t0.Add(2*time.Hour)) != core.RecoveryInvalidated {
		t.Fatalf("%+v", r)
	}
	_, _, h4 := newLink(t, repo, ctx, u, su, t0.Add(4*time.Hour), time.Hour)
	if _, err := useLink(repo, ctx, h4, "p", t0.Add(4*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if r = find().Recovery; r.UsedAt == nil || r.StateAt(t0.Add(10*time.Hour)) != core.RecoveryUsed {
		t.Fatalf("%+v", r)
	}
}

func TestPurgeStaleTokens(t *testing.T) {
	s, ctx := testStore(t)
	inv, rec := s.AccountInvitations(), s.Recovery()
	su := testUser(t, s, ctx, "purger")
	victim := testUser(t, s, ctx, "purged")
	day := 24 * time.Hour
	old := t0                      // everything ended long before the cutoff
	cutoff := t0.Add(120 * day)    // "now - 90 days"
	recent := cutoff.Add(10 * day) // ended after the cutoff

	expiredOld, _ := newAnon(t, inv, ctx, su, 2, old, day, false)
	revokedOld, _ := newAnon(t, inv, ctx, su, 2, old, 20*day, false)
	if _, err := inv.Revoke(ctx, revokedOld.ID, su.ID, old.Add(day), core.InvitationRevokedAudit(old, su.ID, revokedOld.ID)); err != nil {
		t.Fatal(err)
	}
	expiredRecent, _ := newAnon(t, inv, ctx, su, 2, recent, day, false)
	pendingLong, _ := newAnon(t, inv, ctx, su, 2, cutoff, 20*day, false)
	usedOld, uh := newAnon(t, inv, ctx, su, 1, old, day, false)
	if _, err := register(inv, ctx, uh, fmt.Sprintf("purge-%d@example.com", time.Now().UnixNano()), "Keeps", old); err != nil {
		t.Fatal(err)
	}
	usedThenExpired, uh2 := newAnon(t, inv, ctx, su, 5, old, day, false)
	if _, err := register(inv, ctx, uh2, fmt.Sprintf("purge2-%d@example.com", time.Now().UnixNano()), "Keeps2", old); err != nil {
		t.Fatal(err)
	}

	// links: the user has an old finished one and (after replacement) a recent pending one
	oldLink, _, oh := newLink(t, rec, ctx, victim, su, old, day)
	newer, _, nh := newLink(t, rec, ctx, victim, su, recent, day)
	if p, _ := rec.Peek(ctx, oh, recent); p.State != core.RecoveryInvalidated {
		t.Fatal(p.State)
	}
	// invalidated at `recent` by the replacement => not older than the cutoff; make one genuinely old
	other := testUser(t, s, ctx, "purged2")
	oldUsed, _, ouh := newLink(t, rec, ctx, other, su, old, day)
	if _, err := useLink(rec, ctx, ouh, "p", old.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	third := testUser(t, s, ctx, "purged3")
	oldExpired, _, _ := newLink(t, rec, ctx, third, su, old, day)

	auditBefore := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events`)
	if err := s.PurgeStaleTokens(ctx, cutoff); err != nil {
		t.Fatal(err)
	}
	exists := func(table string, id int64) bool {
		return countWhere(t, s, ctx, `SELECT count(*) FROM `+table+` WHERE id = $1`, id) == 1
	}
	for name, c := range map[string]struct {
		table string
		id    int64
		kept  bool
	}{
		"expired old invitation":       {"account_invitations", expiredOld.ID, false},
		"revoked old invitation":       {"account_invitations", revokedOld.ID, false},
		"recently expired invitation":  {"account_invitations", expiredRecent.ID, true},
		"pending invitation":           {"account_invitations", pendingLong.ID, true},
		"used invitation":              {"account_invitations", usedOld.ID, true},
		"used then expired invitation": {"account_invitations", usedThenExpired.ID, true},
		"old used link":                {"password_recovery_links", oldUsed.ID, false},
		"old expired link":             {"password_recovery_links", oldExpired.ID, false},
		"pending recent link":          {"password_recovery_links", newer.ID, true},
		"link replaced at the cutoff+": {"password_recovery_links", oldLink.ID, true},
	} {
		if exists(c.table, c.id) != c.kept {
			t.Errorf("%s: kept=%v, want %v", name, !c.kept, c.kept)
		}
	}
	_ = nh
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM audit_events`); n != auditBefore {
		t.Fatalf("the audit trail must be kept: %d -> %d", auditBefore, n)
	}
	if n := countWhere(t, s, ctx, `SELECT count(*) FROM account_invitation_uses WHERE invitation_id = $1`, usedOld.ID); n != 1 {
		t.Fatal("use rows of a kept invitation")
	}
	// the cutoff is exclusive: an invitation that ended exactly at the cutoff stays
	edge, _ := newAnon(t, inv, ctx, su, 1, cutoff.Add(-day), day, false)
	if err := s.PurgeStaleTokens(ctx, cutoff); err != nil {
		t.Fatal(err)
	}
	if !exists("account_invitations", edge.ID) {
		t.Fatal("ended exactly at the cutoff must stay")
	}
}

// No raw token is stored anywhere in the three new tables: a token used in every flow does not appear in any text or bytea column.
func TestNoRawTokenIsStored(t *testing.T) {
	s, ctx := testStore(t)
	inv, rec := s.AccountInvitations(), s.Recovery()
	su := testUser(t, s, ctx, "scanner")
	victim := testUser(t, s, ctx, "scanned")

	rawInv, hInv := tokenAndHash("scan-inv")
	made, err := inv.Create(ctx, core.StoredAccountInvitation{TokenHash: hInv, Kind: core.InvitationAnonymous, MaxUses: 1, CreatedBy: su.ID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)},
		core.InvitationCreatedAudit(t0, su.ID, core.InvitationAnonymous, t0.Add(time.Hour), 1, false))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := register(inv, ctx, hInv, fmt.Sprintf("scan-%d@example.com", time.Now().UnixNano()), "Scan", t0); err != nil {
		t.Fatal(err)
	}
	rawRec, hRec := tokenAndHash("scan-rec")
	exp := t0.Add(time.Hour)
	if _, err := rec.Create(ctx, core.StoredRecovery{UserID: victim.ID, TokenHash: hRec, CreatedBy: su.ID, CreatedAt: t0, ExpiresAt: exp}, core.RecoveryCreatedAudit(t0, su.ID, victim.ID, exp)); err != nil {
		t.Fatal(err)
	}
	if _, err := useLink(rec, ctx, hRec, "pw", t0); err != nil {
		t.Fatal(err)
	}
	_ = made
	for _, table := range []string{"audit_events", "account_invitations", "account_invitation_uses", "password_recovery_links"} {
		rows, err := s.pool.Query(ctx, `SELECT row_to_json(x)::text FROM `+table+` x`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				t.Fatal(err)
			}
			for _, raw := range []string{rawInv, rawRec} {
				if strings.Contains(row, raw) {
					t.Fatalf("%s holds a raw token: %s", table, row)
				}
			}
		}
		rows.Close()
	}
	// the hash columns are exactly 32 bytes
	var l int
	if err := s.pool.QueryRow(ctx, `SELECT octet_length(token_hash) FROM password_recovery_links WHERE token_hash = $1`, hRec).Scan(&l); err != nil || l != 32 {
		t.Fatalf("%d %v", l, err)
	}
	var _ pgx.Tx
}

// The three migrations go down and up again on a database that already has rows.
func TestAccountMigrationsDownAndUp(t *testing.T) {
	dsn := scratchDatabase(t)
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u := testUser(t, s, ctx, "migr")
	if _, err := s.AccountInvitations().Create(ctx, core.StoredAccountInvitation{TokenHash: sha32("a"), Kind: core.InvitationAnonymous, MaxUses: 1, CreatedBy: u.ID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour)},
		core.InvitationCreatedAudit(t0, u.ID, core.InvitationAnonymous, t0.Add(time.Hour), 1, false)); err != nil {
		t.Fatal(err)
	}
	sub, _ := fs.Sub(migrations, "migrations")
	db := stdlib.OpenDBFromPool(s.pool)
	p, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := p.Down(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name IN ('audit_events','account_invitations','account_invitation_uses','password_recovery_links')`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name IN ('audit_events','account_invitations','account_invitation_uses','password_recovery_links')`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("%d %v", n, err)
	}
}

func sha32(label string) []byte { _, h := tokenAndHash(label); return h }
