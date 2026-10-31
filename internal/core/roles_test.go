package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

// TestRoleChangeMatrix covers every cell: 5 actors x 4 levels x 4 levels. 'o' is allowed (or a no-op), 'F' is ErrForbidden.
// Rows are the level the account has now (P, O, A, S), columns the level it would get.
func TestRoleChangeMatrix(t *testing.T) {
	levels := []Level{LevelPlain, LevelOrganiser, LevelAdministrator, LevelSuperuser}
	names := []string{"P", "O", "A", "S"}
	matrix := map[string]struct {
		actor Actor
		rows  [4]string
	}{
		"plain user":    {ActorPlain, [4]string{"FFFF", "FFFF", "FFFF", "FFFF"}},
		"organiser":     {ActorOrganiser, [4]string{"FFFF", "FFFF", "FFFF", "FFFF"}},
		"administrator": {ActorAdministrator, [4]string{"ooFF", "ooFF", "FFFF", "FFFF"}},
		"superuser":     {ActorSuperuser, [4]string{"oooF", "oooF", "oooF", "FFFF"}},
		"console":       {ActorConsole, [4]string{"oooo", "oooo", "oooo", "oooo"}},
	}
	cells := 0
	for who, m := range matrix {
		for i, from := range levels {
			for j, to := range levels {
				cells++
				err := RoleChangeAllowed(m.actor, from, to)
				want := m.rows[i][j]
				switch {
				case want == 'o' && err != nil:
					t.Errorf("%s %s->%s: got %v, want allowed", who, names[i], names[j], err)
				case want == 'F' && !errors.Is(err, ErrForbidden):
					t.Errorf("%s %s->%s: got %v, want ErrForbidden", who, names[i], names[j], err)
				}
			}
		}
	}
	if cells != 80 {
		t.Fatalf("%d cells", cells)
	}
}

func TestRoleChangeReasons(t *testing.T) {
	for _, c := range []struct {
		actor    Actor
		from, to Level
		want     string
	}{
		{ActorAdministrator, LevelAdministrator, LevelPlain, "Only a superuser can change an administrator"},
		{ActorAdministrator, LevelPlain, LevelAdministrator, "Only a superuser can change an administrator"},
		{ActorAdministrator, LevelSuperuser, LevelPlain, "Superusers can only be changed from the console"},
		{ActorAdministrator, LevelPlain, LevelSuperuser, "Superusers are set from the console"},
		{ActorSuperuser, LevelPlain, LevelSuperuser, "Superusers are set from the console"},
		{ActorSuperuser, LevelSuperuser, LevelOrganiser, "Superusers can only be changed from the console"},
		{ActorOrganiser, LevelPlain, LevelOrganiser, "Only administrators"},
	} {
		if err := RoleChangeAllowed(c.actor, c.from, c.to); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: got %v, want %q", c, err, c.want)
		}
	}
}

func TestLevelOfAndActorOf(t *testing.T) {
	for _, c := range []struct {
		role Role
		can  bool
		want Level
	}{
		{RoleUser, false, LevelPlain}, {RoleUser, true, LevelOrganiser},
		{RoleAdmin, false, LevelAdministrator}, {RoleAdmin, true, LevelAdministrator},
		{RoleSuperuser, false, LevelSuperuser}, {RoleSuperuser, true, LevelSuperuser},
	} {
		if got := LevelOf(c.role, c.can); got != c.want {
			t.Errorf("%v %v: %v", c.role, c.can, got)
		}
	}
	if ActorOf(User{Role: RoleUser}) != ActorPlain || ActorOf(User{Role: RoleUser, CanCreateCampaigns: true}) != ActorOrganiser ||
		ActorOf(User{Role: RoleAdmin}) != ActorAdministrator || ActorOf(User{Role: RoleSuperuser}) != ActorSuperuser {
		t.Fatal("ActorOf")
	}
	if !(User{Role: RoleAdmin}).MayAdminister() || !(User{Role: RoleSuperuser}).MayAdminister() || (User{Role: RoleUser, CanCreateCampaigns: true}).MayAdminister() {
		t.Fatal("MayAdminister")
	}
}

func TestAuth_SetUserPermissions(t *testing.T) {
	ctx := context.Background()
	super := User{ID: 1, Role: RoleSuperuser}
	admin := User{ID: 2, Role: RoleAdmin}
	organiser := User{ID: 3, Role: RoleUser, CanCreateCampaigns: true}
	plain := User{ID: 4, Role: RoleUser}
	target := func(role Role, can bool) (User, bool, error) {
		return User{ID: 7, Role: role, CanCreateCampaigns: can}, false, nil
	}
	// expectSet expects the write and checks the audit entry it receives.
	expectSet := func(repo *MockauthRepository, role Role, can bool, actorID int64, fromRole Role, fromCan bool) {
		repo.EXPECT().SetUserPermissionsByID(gomock.Any(), int64(7), role, can, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ int64, _ Role, _ bool, e AuditEntry) error {
				if e.Action != AuditRoleChanged || e.Source != AuditWeb || e.ActorID == nil || *e.ActorID != actorID ||
					e.SubjectUserID == nil || *e.SubjectUserID != 7 || !e.At.Equal(t0) {
					t.Errorf("audit entry: %+v", e)
				}
				if e.Detail["from_role"] != string(fromRole) || e.Detail["to_role"] != string(role) ||
					e.Detail["from_can_create"] != boolStr(fromCan) || e.Detail["to_can_create"] != boolStr(can) {
					t.Errorf("audit detail: %v", e.Detail)
				}
				return nil
			})
	}

	t.Run("an administrator makes a plain user an organiser and back", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleUser, false))
		expectSet(repo, RoleUser, true, 2, RoleUser, false)
		if err := a.SetUserPermissions(ctx, admin, 7, RoleUser, true); err != nil {
			t.Fatal(err)
		}
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleUser, true))
		expectSet(repo, RoleUser, false, 2, RoleUser, true)
		if err := a.SetUserPermissions(ctx, admin, 7, RoleUser, false); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("an administrator may not touch administrators or superusers, nor forge a role", func(t *testing.T) {
		for name, c := range map[string]struct {
			role     Role
			can      bool
			to       Role
			loadRepo bool
		}{
			"target is an administrator":   {RoleAdmin, false, RoleUser, true},
			"target is an administrator 2": {RoleAdmin, true, RoleAdmin, true},
			"target is a superuser":        {RoleSuperuser, false, RoleUser, true},
			"promotion to administrator":   {RoleUser, false, RoleAdmin, true},
			"forged superuser":             {RoleUser, false, RoleSuperuser, false},
		} {
			a, repo := newTestAuth(t)
			if c.loadRepo {
				repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(c.role, c.can))
			}
			if err := a.SetUserPermissions(ctx, admin, 7, c.to, false); !errors.Is(err, ErrForbidden) {
				t.Errorf("%s: got %v", name, err)
			}
		}
	})
	t.Run("an administrator may not change their own account", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByID(gomock.Any(), int64(2)).Return(User{ID: 2, Role: RoleAdmin}, false, nil)
		if err := a.SetUserPermissions(ctx, admin, 2, RoleUser, false); !errors.Is(err, ErrForbidden) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("plain users and organisers are forbidden before the repository is reached", func(t *testing.T) {
		for _, actor := range []User{organiser, plain} {
			a, _ := newTestAuth(t)
			if err := a.SetUserPermissions(ctx, actor, 7, RoleUser, true); !errors.Is(err, ErrForbidden) {
				t.Errorf("%+v: %v", actor, err)
			}
		}
	})
	t.Run("a superuser promotes to administrator keeping the stored flag, and demotes with the submitted one", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleUser, true))
		expectSet(repo, RoleAdmin, true, 1, RoleUser, true) // the submitted false is ignored
		if err := a.SetUserPermissions(ctx, super, 7, RoleAdmin, false); err != nil {
			t.Fatal(err)
		}
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleAdmin, true))
		expectSet(repo, RoleUser, false, 1, RoleAdmin, true)
		if err := a.SetUserPermissions(ctx, super, 7, RoleUser, false); err != nil {
			t.Fatal(err)
		}
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleAdmin, false))
		expectSet(repo, RoleUser, true, 1, RoleAdmin, false)
		if err := a.SetUserPermissions(ctx, super, 7, RoleUser, true); err != nil {
			t.Fatal(err)
		}
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleUser, false))
		expectSet(repo, RoleUser, true, 1, RoleUser, false)
		if err := a.SetUserPermissions(ctx, super, 7, RoleUser, true); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("nothing changed: no write, no audit", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleUser, true)).Times(2)
		if err := a.SetUserPermissions(ctx, super, 7, RoleUser, true); err != nil {
			t.Fatal(err)
		}
		if err := a.SetUserPermissions(ctx, admin, 7, RoleUser, true); err != nil {
			t.Fatal(err)
		}
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleAdmin, false))
		if err := a.SetUserPermissions(ctx, super, 7, RoleAdmin, true); err != nil { // the flag is ignored while administrator
			t.Fatal(err)
		}
	})
	t.Run("a forged superuser role is forbidden for everybody, a superuser included", func(t *testing.T) {
		for _, actor := range []User{super, admin, organiser, plain} {
			a, _ := newTestAuth(t)
			if err := a.SetUserPermissions(ctx, actor, 7, RoleSuperuser, false); !errors.Is(err, ErrForbidden) {
				t.Errorf("%v: got %v", actor.Role, err)
			}
		}
	})
	t.Run("a superuser account is never changed from the web", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleSuperuser, false))
		if err := a.SetUserPermissions(ctx, super, 7, RoleUser, false); !errors.Is(err, ErrForbidden) {
			t.Fatalf("got %v", err)
		}
		a, repo = newTestAuth(t)
		repo.EXPECT().UserByID(gomock.Any(), int64(1)).Return(User{ID: 1, Role: RoleSuperuser}, false, nil)
		if err := a.SetUserPermissions(ctx, super, 1, RoleUser, false); !errors.Is(err, ErrForbidden) {
			t.Fatalf("own account: %v", err)
		}
	})
	t.Run("bad role", func(t *testing.T) {
		a, _ := newTestAuth(t)
		if err := a.SetUserPermissions(ctx, super, 7, "boss", false); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("unknown id", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(User{}, false, ErrNotFound)
		if err := a.SetUserPermissions(ctx, super, 7, RoleAdmin, false); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("closed account", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(User{ID: 7}, true, nil)
		if err := a.SetUserPermissions(ctx, super, 7, RoleAdmin, false); !errors.Is(err, ErrRefused) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("only the target's cached sessions are dropped", func(t *testing.T) {
		a, repo, _ := newClockedAuth(t)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("mine"), t0).Return(User{ID: 7, Role: RoleUser}, t0.Add(sessionTTL), false, nil).Times(2)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("other"), t0).Return(User{ID: 8, Role: RoleUser}, t0.Add(sessionTTL), false, nil)
		for _, tok := range []string{"mine", "other"} {
			if _, _, err := a.Authenticate(ctx, tok); err != nil {
				t.Fatal(err)
			}
		}
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleUser, false))
		repo.EXPECT().SetUserPermissionsByID(gomock.Any(), int64(7), RoleUser, true, gomock.Any()).Return(nil)
		if err := a.SetUserPermissions(ctx, admin, 7, RoleUser, true); err != nil {
			t.Fatal(err)
		}
		for _, tok := range []string{"mine", "other"} { // "other" is still cached: no second lookup is expected
			if _, _, err := a.Authenticate(ctx, tok); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("a failing write keeps the cache", func(t *testing.T) {
		a, repo := newTestAuth(t)
		boom := errors.New("boom")
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(target(RoleUser, false))
		repo.EXPECT().SetUserPermissionsByID(gomock.Any(), int64(7), RoleUser, true, gomock.Any()).Return(boom)
		if err := a.SetUserPermissions(ctx, admin, 7, RoleUser, true); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestAuth_SetPermissions_Console(t *testing.T) {
	ctx := context.Background()
	cred := func(role Role, can bool) Credentials {
		return Credentials{User: User{ID: 7, Role: role, CanCreateCampaigns: can}}
	}

	t.Run("every cell is allowed, audited as console without an actor", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "a@b.c").Return(cred(RoleUser, false), nil)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(User{ID: 7, Role: RoleUser}, false, nil)
		repo.EXPECT().SetUserPermissionsByID(gomock.Any(), int64(7), RoleSuperuser, false, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ int64, _ Role, _ bool, e AuditEntry) error {
				if e.Source != AuditConsole || e.ActorID != nil || e.Action != AuditRoleChanged || e.Detail["to_role"] != "superuser" {
					t.Errorf("audit: %+v", e)
				}
				return nil
			})
		if err := a.SetPermissions(ctx, " A@b.c ", RoleSuperuser, false); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("demoting a superuser with a colleague", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "a@b.c").Return(cred(RoleSuperuser, false), nil)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(User{ID: 7, Role: RoleSuperuser}, false, nil)
		repo.EXPECT().OtherSuperusers(gomock.Any(), int64(7)).Return(1, nil)
		repo.EXPECT().SetUserPermissionsByID(gomock.Any(), int64(7), RoleUser, true, gomock.Any()).Return(nil)
		if err := a.SetPermissions(ctx, "a@b.c", RoleUser, true); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("the last superuser cannot be demoted", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "a@b.c").Return(cred(RoleSuperuser, false), nil)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(User{ID: 7, Role: RoleSuperuser}, false, nil)
		repo.EXPECT().OtherSuperusers(gomock.Any(), int64(7)).Return(0, nil)
		if err := a.SetPermissions(ctx, "a@b.c", RoleUser, false); !errors.Is(err, ErrRefused) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("unknown email and bad role", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "x@b.c").Return(Credentials{}, ErrNotFound)
		if err := a.SetPermissions(ctx, "x@b.c", RoleUser, false); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
		if err := a.SetPermissions(ctx, "a@b.c", "boss", false); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a closed account is refused", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "a@b.c").Return(cred(RoleUser, false), nil)
		repo.EXPECT().UserByID(gomock.Any(), int64(7)).Return(User{ID: 7}, true, nil)
		if err := a.SetPermissions(ctx, "a@b.c", RoleAdmin, false); !errors.Is(err, ErrRefused) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestAuth_ListUsers(t *testing.T) {
	ctx := context.Background()
	rec := &RecoveryStatus{ID: 9, ExpiresAt: t0.Add(time.Hour)}
	want := []AdminUser{{User: User{ID: 1}, Campaigns: 2, Recovery: rec}}
	t.Run("plain users and organisers are forbidden", func(t *testing.T) {
		a, _ := newTestAuth(t)
		for _, u := range []User{{Role: RoleUser}, {Role: RoleUser, CanCreateCampaigns: true}} {
			if _, _, err := a.ListUsers(ctx, u, "", 25, 0); !errors.Is(err, ErrForbidden) {
				t.Fatalf("got %v", err)
			}
		}
	})
	t.Run("paging", func(t *testing.T) {
		a, repo := newTestAuth(t)
		for _, c := range []struct{ limit, offset, wantLimit, wantOffset int }{{0, -3, 25, 0}, {10, 20, 10, 20}, {5000, 0, 200, 0}} {
			repo.EXPECT().ListUsers(gomock.Any(), "bob", c.wantLimit, c.wantOffset).Return(copyUsers(want), 41, nil)
			rows, total, err := a.ListUsers(ctx, User{Role: RoleSuperuser}, " bob ", c.limit, c.offset)
			if err != nil || total != 41 || len(rows) != 1 {
				t.Fatalf("%+v: %v %d %v", c, rows, total, err)
			}
		}
	})
	t.Run("the recovery status is for superusers only, with its state derived", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().ListUsers(gomock.Any(), "", 25, 0).Return(copyUsers(want), 1, nil)
		rows, _, err := a.ListUsers(ctx, User{Role: RoleSuperuser}, "", 0, 0)
		if err != nil || rows[0].Recovery == nil || rows[0].Recovery.State != RecoveryPending {
			t.Fatalf("superuser: %+v %v", rows, err)
		}
		repo.EXPECT().ListUsers(gomock.Any(), "", 25, 0).Return(copyUsers(want), 1, nil)
		rows, _, err = a.ListUsers(ctx, User{Role: RoleAdmin}, "", 0, 0)
		if err != nil || rows[0].Recovery != nil {
			t.Fatalf("administrator: %+v %v", rows, err)
		}
	})
}

func copyUsers(in []AdminUser) []AdminUser {
	out := append([]AdminUser(nil), in...)
	for i := range out {
		if out[i].Recovery != nil {
			r := *out[i].Recovery
			out[i].Recovery = &r
		}
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
