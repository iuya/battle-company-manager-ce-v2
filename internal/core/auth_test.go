package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

var fastArgon = argonParams{memoryKiB: 8, time: 1, threads: 1}

var t0 = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func newTestAuth(t *testing.T) (*Auth, *MockauthRepository) {
	repo := NewMockauthRepository(gomock.NewController(t))
	return newAuth(repo, func() time.Time { return t0 }, fastArgon), repo
}

// newClockedAuth returns an Auth whose clock the test moves.
func newClockedAuth(t *testing.T) (*Auth, *MockauthRepository, *time.Time) {
	repo := NewMockauthRepository(gomock.NewController(t))
	now := t0
	return newAuth(repo, func() time.Time { return now }, fastArgon), repo, &now
}

func TestPasswordHash(t *testing.T) {
	h, err := hashPassword("correct horse battery", fastArgon)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := verifyPassword("correct horse battery", h); !ok || err != nil {
		t.Fatalf("right password rejected: %v %v", ok, err)
	}
	if ok, _ := verifyPassword("wrong", h); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := verifyPassword("x", "not-a-hash"); err == nil {
		t.Fatal("garbage hash should be an error")
	}
	h2, _ := hashPassword("correct horse battery", fastArgon)
	if h == h2 {
		t.Fatal("salts must differ")
	}
}

func TestAuth_CreateUser(t *testing.T) {
	a, repo := newTestAuth(t)
	repo.EXPECT().CreateUser(gomock.Any(), "frodo@shire.me", "Frodo", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, hash string) (User, error) {
			if ok, _ := verifyPassword("a long enough password", hash); !ok {
				t.Error("the stored hash must verify the password")
			}
			return User{ID: 1, Email: "frodo@shire.me", DisplayName: "Frodo"}, nil
		})
	if _, err := a.CreateUser(context.Background(), "  Frodo@Shire.me ", " Frodo ", "a long enough password"); err != nil {
		t.Fatal(err)
	}

	for name, args := range map[string][3]string{
		"bad email":      {"nope", "Frodo", "a long enough password"},
		"display name":   {"f@shire.me", "  ", "a long enough password"},
		"short password": {"f@shire.me", "Frodo", "short"},
	} {
		if _, err := a.CreateUser(context.Background(), args[0], args[1], args[2]); !errors.Is(err, ErrInvalidUser) {
			t.Errorf("%s: got %v, want ErrInvalidUser", name, err)
		}
	}
}

func TestAuth_Login(t *testing.T) {
	hash, _ := hashPassword("a long enough password", fastArgon)
	rec := Credentials{User: User{ID: 7, Email: "frodo@shire.me", DisplayName: "Frodo"}, PasswordHash: hash}

	t.Run("opens a session storing only the token hash", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "frodo@shire.me").Return(rec, nil)
		var stored []byte
		gomock.InOrder(
			repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByLogin, t0).Return(0, nil),
			repo.EXPECT().CreateSession(gomock.Any(), gomock.Any(), int64(7), t0.Add(sessionTTL)).
				DoAndReturn(func(_ context.Context, h []byte, _ int64, _ time.Time) error { stored = h; return nil }),
		)
		repo.EXPECT().DeleteExpiredSessions(gomock.Any(), t0).Return(nil)
		repo.EXPECT().PurgeStaleTokens(gomock.Any(), t0.Add(-staleTokenAge)).Return(nil)

		token, expires, user, err := a.Login(context.Background(), " Frodo@shire.me", "a long enough password")
		if err != nil || user.ID != 7 || !expires.Equal(t0.Add(sessionTTL)) {
			t.Fatalf("got %v %v %+v", expires, err, user)
		}
		if string(stored) != string(hashToken(token)) || token == "" {
			t.Fatal("the repository must receive the hash of the returned token")
		}
	})

	t.Run("wrong password and unknown email look the same", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "frodo@shire.me").Return(rec, nil)
		_, _, _, err1 := a.Login(context.Background(), "frodo@shire.me", "not the password")
		repo.EXPECT().UserByEmail(gomock.Any(), "ghost@shire.me").Return(Credentials{}, ErrNotFound)
		_, _, _, err2 := a.Login(context.Background(), "ghost@shire.me", "whatever it is")
		if !errors.Is(err1, ErrInvalidCredentials) || !errors.Is(err2, ErrInvalidCredentials) {
			t.Fatalf("got %v and %v", err1, err2)
		}
	})
}

func TestAuth_Authenticate(t *testing.T) {
	ctx := context.Background()
	u := User{ID: 7, Email: "frodo@shire.me", DisplayName: "Frodo"}

	t.Run("a young session is left alone", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(u, t0.Add(sessionTTL-time.Hour), false, nil)
		got, renewed, err := a.Authenticate(ctx, "tok")
		if err != nil || got != u || !renewed.IsZero() {
			t.Fatalf("got %+v %v %v", got, renewed, err)
		}
	})

	t.Run("a session in its second half is renewed", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(u, t0.Add(sessionTTL/2-time.Hour), false, nil)
		repo.EXPECT().ExtendSession(gomock.Any(), hashToken("tok"), t0.Add(sessionTTL)).Return(nil)
		_, renewed, err := a.Authenticate(ctx, "tok")
		if err != nil || !renewed.Equal(t0.Add(sessionTTL)) {
			t.Fatalf("got %v %v", renewed, err)
		}
	})

	t.Run("unknown or empty tokens", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("old"), t0).Return(User{}, time.Time{}, false, ErrNotFound)
		if _, _, err := a.Authenticate(ctx, "old"); !errors.Is(err, ErrNoSession) {
			t.Fatalf("got %v", err)
		}
		if _, _, err := a.Authenticate(ctx, ""); !errors.Is(err, ErrNoSession) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestAuth_Logout(t *testing.T) {
	a, repo := newTestAuth(t)
	repo.EXPECT().DeleteSession(gomock.Any(), hashToken("tok")).Return(nil)
	if err := a.Logout(context.Background(), "tok"); err != nil {
		t.Fatal(err)
	}
	if err := a.Logout(context.Background(), ""); err != nil { // no repository call expected
		t.Fatal(err)
	}
}

func TestAuth_Authenticate_IsCached(t *testing.T) {
	ctx := context.Background()
	u := User{ID: 7, Email: "frodo@shire.me", DisplayName: "Frodo"}
	a, repo, now := newClockedAuth(t)

	// one database lookup serves every request of the next minute
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(u, t0.Add(sessionTTL), false, nil)
	for i := 0; i < 5; i++ {
		if got, _, err := a.Authenticate(ctx, "tok"); err != nil || got != u {
			t.Fatalf("request %d: %+v %v", i, got, err)
		}
	}

	// after the cache TTL the session is confirmed again; here it was revoked in the meantime
	*now = t0.Add(sessionCacheTTL)
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), *now).Return(User{}, time.Time{}, false, ErrNotFound)
	if _, _, err := a.Authenticate(ctx, "tok"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("got %v", err)
	}
}

func TestAuth_Logout_RevokesImmediately(t *testing.T) {
	ctx := context.Background()
	a, repo, _ := newClockedAuth(t)
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 1}, t0.Add(sessionTTL), false, nil)
	if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	repo.EXPECT().DeleteSession(gomock.Any(), hashToken("tok")).Return(nil)
	if err := a.Logout(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	// the cached entry is gone, so the next request asks the database, which no longer has the session
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{}, time.Time{}, false, ErrNotFound)
	if _, _, err := a.Authenticate(ctx, "tok"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("got %v", err)
	}
}

func TestAuth_Authenticate_CacheRespectsSessionExpiry(t *testing.T) {
	ctx := context.Background()
	a, repo, now := newClockedAuth(t)
	// the session ends 10 seconds from now, well inside the cache TTL
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 1}, t0.Add(10*time.Second), false, nil)
	repo.EXPECT().ExtendSession(gomock.Any(), hashToken("tok"), t0.Add(sessionTTL)).Return(nil)
	if _, renewed, err := a.Authenticate(ctx, "tok"); err != nil || renewed.IsZero() {
		t.Fatalf("a nearly expired session is renewed: %v %v", renewed, err)
	}
	// ... and the renewed expiry is what the cache now knows
	*now = t0.Add(30 * time.Second)
	if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestMayCreateCampaigns(t *testing.T) {
	cases := []struct {
		u    User
		want bool
	}{
		{User{}, false},
		{User{Role: RoleUser}, false},
		{User{Role: RoleUser, CanCreateCampaigns: true}, true},
		{User{Role: RoleAdmin}, true},
		{User{Role: RoleSuperuser}, true},
	}
	for _, c := range cases {
		if got := c.u.MayCreateCampaigns(); got != c.want {
			t.Errorf("%+v: got %t, want %t", c.u, got, c.want)
		}
	}
}

func TestAuth_UpdateProfile(t *testing.T) {
	ctx := context.Background()
	a, repo, _ := newClockedAuth(t)
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 7, DisplayName: "Frodo"}, t0.Add(sessionTTL), false, nil)
	if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"  ", strings.Repeat("x", 61)} {
		if _, err := a.UpdateProfile(ctx, 7, bad); !errors.Is(err, ErrInvalidUser) {
			t.Errorf("%q: got %v", bad, err)
		}
	}
	repo.EXPECT().UpdateDisplayName(gomock.Any(), int64(7), "Mr Baggins").Return(User{ID: 7, DisplayName: "Mr Baggins"}, nil)
	if u, err := a.UpdateProfile(ctx, 7, " Mr Baggins "); err != nil || u.DisplayName != "Mr Baggins" {
		t.Fatalf("%+v %v", u, err)
	}
	// the cached session shows the new name without a lookup
	if u, _, err := a.Authenticate(ctx, "tok"); err != nil || u.DisplayName != "Mr Baggins" {
		t.Fatalf("%+v %v", u, err)
	}
}

func TestAuth_ChangePassword(t *testing.T) {
	ctx := context.Background()
	hash, _ := hashPassword("the current password", fastArgon)
	rec := Credentials{User: User{ID: 7}, PasswordHash: hash}

	t.Run("stores the new hash and signs out the other sessions", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().CredentialsByID(gomock.Any(), int64(7)).Return(rec, nil)
		repo.EXPECT().UpdatePasswordHash(gomock.Any(), int64(7), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, h string) error {
			if ok, _ := verifyPassword("a brand new password", h); !ok {
				t.Error("the stored hash must verify the new password")
			}
			return nil
		})
		repo.EXPECT().DeleteSessionsExcept(gomock.Any(), int64(7), hashToken("mine")).Return(2, nil)
		if err := a.ChangePassword(ctx, 7, "the current password", "a brand new password", "mine"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("wrong current password", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().CredentialsByID(gomock.Any(), int64(7)).Return(rec, nil)
		if err := a.ChangePassword(ctx, 7, "not it at all", "a brand new password", "mine"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("new password too short", func(t *testing.T) {
		a, _ := newTestAuth(t)
		if err := a.ChangePassword(ctx, 7, "the current password", "short", "mine"); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestAuth_SignOutOthers_DropsCachedSessions(t *testing.T) {
	ctx := context.Background()
	a, repo, _ := newClockedAuth(t)
	for _, tok := range []string{"mine", "other"} {
		repo.EXPECT().UserBySession(gomock.Any(), hashToken(tok), t0).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil)
		if _, _, err := a.Authenticate(ctx, tok); err != nil {
			t.Fatal(err)
		}
	}
	repo.EXPECT().DeleteSessionsExcept(gomock.Any(), int64(7), hashToken("mine")).Return(1, nil)
	if n, err := a.SignOutOthers(ctx, 7, "mine"); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	// "mine" is still cached; "other" must go back to the database, which no longer has it
	if _, _, err := a.Authenticate(ctx, "mine"); err != nil {
		t.Fatal(err)
	}
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("other"), t0).Return(User{}, time.Time{}, false, ErrNotFound)
	if _, _, err := a.Authenticate(ctx, "other"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("got %v", err)
	}
}

func TestAuth_CloseAccount(t *testing.T) {
	ctx := context.Background()
	hash, _ := hashPassword("the current password", fastArgon)
	user := Credentials{User: User{ID: 7, Role: RoleUser}, PasswordHash: hash}
	super := Credentials{User: User{ID: 7, Role: RoleSuperuser}, PasswordHash: hash}

	t.Run("anonymises", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().CredentialsByID(gomock.Any(), int64(7)).Return(user, nil)
		repo.EXPECT().OrganisesUnfinishedCampaign(gomock.Any(), int64(7)).Return(false, nil)
		repo.EXPECT().CloseUser(gomock.Any(), int64(7), "Former player", "closed-7@invalid", t0).Return(nil)
		if err := a.CloseAccount(ctx, 7, "the current password"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("wrong password", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().CredentialsByID(gomock.Any(), int64(7)).Return(user, nil)
		if err := a.CloseAccount(ctx, 7, "nope nope nope"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("organiser of an unfinished campaign", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().CredentialsByID(gomock.Any(), int64(7)).Return(user, nil)
		repo.EXPECT().OrganisesUnfinishedCampaign(gomock.Any(), int64(7)).Return(true, nil)
		if err := a.CloseAccount(ctx, 7, "the current password"); !errors.Is(err, ErrRefused) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("last superuser", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().CredentialsByID(gomock.Any(), int64(7)).Return(super, nil)
		repo.EXPECT().OrganisesUnfinishedCampaign(gomock.Any(), int64(7)).Return(false, nil)
		repo.EXPECT().OtherSuperusers(gomock.Any(), int64(7)).Return(0, nil)
		if err := a.CloseAccount(ctx, 7, "the current password"); !errors.Is(err, ErrRefused) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a superuser with a colleague may close", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().CredentialsByID(gomock.Any(), int64(7)).Return(super, nil)
		repo.EXPECT().OrganisesUnfinishedCampaign(gomock.Any(), int64(7)).Return(false, nil)
		repo.EXPECT().OtherSuperusers(gomock.Any(), int64(7)).Return(1, nil)
		repo.EXPECT().CloseUser(gomock.Any(), int64(7), "Former player", "closed-7@invalid", t0).Return(nil)
		if err := a.CloseAccount(ctx, 7, "the current password"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestAuth_Login_ClosedAccountFails(t *testing.T) {
	a, repo := newTestAuth(t)
	repo.EXPECT().UserByEmail(gomock.Any(), "closed-7@invalid").Return(Credentials{User: User{ID: 7}}, nil)
	if _, _, _, err := a.Login(context.Background(), "closed-7@invalid", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("got %v", err)
	}
}

func TestAuth_SetAppearance(t *testing.T) {
	ctx := context.Background()
	a, repo, _ := newClockedAuth(t)
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil)
	if u, _, err := a.Authenticate(ctx, "tok"); err != nil || u.Appearance != (Appearance{}) {
		t.Fatalf("never chosen must be empty: %+v %v", u, err)
	}
	for _, bad := range [][2]string{{"nope", "dark"}, {"forest", "nope"}, {"", ""}} {
		if err := a.SetAppearance(ctx, 7, bad[0], bad[1]); !errors.Is(err, ErrInvalidUser) {
			t.Errorf("%v: got %v", bad, err)
		}
	}
	repo.EXPECT().SetAppearance(gomock.Any(), int64(7), "ember", "dark").Return(nil)
	if err := a.SetAppearance(ctx, 7, "ember", "dark"); err != nil {
		t.Fatal(err)
	}
	// the cache reflects the change at once, without another lookup
	if u, _, err := a.Authenticate(ctx, "tok"); err != nil || u.Appearance != (Appearance{"ember", "dark"}) {
		t.Fatalf("%+v %v", u, err)
	}
}

func TestAuth_SetTimezone(t *testing.T) {
	ctx := context.Background()
	a, repo, _ := newClockedAuth(t)
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil)
	if u, _, err := a.Authenticate(ctx, "tok"); err != nil || u.Timezone != "" {
		t.Fatalf("automatic until chosen: %+v %v", u, err)
	}
	for _, bad := range []string{"Mars/Olympus", "Local", "../etc/passwd", strings.Repeat("x", 65)} {
		if err := a.SetTimezone(ctx, 7, bad); !errors.Is(err, ErrInvalidUser) {
			t.Errorf("%q: got %v", bad, err)
		}
	}
	repo.EXPECT().SetTimezone(gomock.Any(), int64(7), "Pacific/Auckland").Return(nil)
	if err := a.SetTimezone(ctx, 7, " Pacific/Auckland "); err != nil {
		t.Fatal(err)
	}
	if u, _, err := a.Authenticate(ctx, "tok"); err != nil || u.Timezone != "Pacific/Auckland" {
		t.Fatalf("the cache follows at once: %+v %v", u, err)
	}
	repo.EXPECT().SetTimezone(gomock.Any(), int64(7), "").Return(nil)
	if err := a.SetTimezone(ctx, 7, ""); err != nil {
		t.Errorf("automatic: %v", err)
	}
}

func TestUserLocationPrefersTheSavedZone(t *testing.T) {
	mk := func(u *User, browser, fallback string) string { return UserLocation(u, browser, fallback).String() }
	if got := mk(&User{Timezone: "Pacific/Auckland"}, "Europe/Paris", "Asia/Tokyo"); got != "Pacific/Auckland" {
		t.Errorf("saved: %s", got)
	}
	if got := mk(&User{}, "Europe/Paris", "Asia/Tokyo"); got != "Europe/Paris" {
		t.Errorf("browser: %s", got)
	}
	if got := mk(nil, "Mars/Olympus", "Asia/Tokyo"); got != "Asia/Tokyo" {
		t.Errorf("fallback after an unknown browser zone: %s", got)
	}
	if got := mk(nil, "", ""); got != "UTC" {
		t.Errorf("utc: %s", got)
	}
	// the same UTC instant reads differently, and no stored value changes
	at := time.Date(2026, 3, 29, 1, 30, 0, 0, time.UTC) // around a DST change in Europe
	if h := at.In(UserLocation(&User{Timezone: "Europe/Paris"}, "", "")).Hour(); h != 3 {
		t.Errorf("Paris after the change: %d", h)
	}
	if h := at.In(UserLocation(&User{Timezone: "Pacific/Auckland"}, "", "")).Hour(); h != 14 {
		t.Errorf("Auckland: %d", h)
	}
}

// TestAuth_Login_PurgesStaleTokensAtMostOncePerHour: the purge is housekeeping; a second login within the hour skips it.
func TestAuth_Login_PurgesStaleTokensAtMostOncePerHour(t *testing.T) {
	hash, _ := hashPassword("a long enough password", fastArgon)
	rec := Credentials{User: User{ID: 7, Email: "frodo@shire.me"}, PasswordHash: hash}
	a, repo, now := newClockedAuth(t)
	repo.EXPECT().UserByEmail(gomock.Any(), "frodo@shire.me").Return(rec, nil).Times(3)
	repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByLogin, gomock.Any()).Return(0, nil).Times(3)
	repo.EXPECT().CreateSession(gomock.Any(), gomock.Any(), int64(7), gomock.Any()).Return(nil).Times(3)
	repo.EXPECT().DeleteExpiredSessions(gomock.Any(), gomock.Any()).Return(nil).Times(3)
	repo.EXPECT().PurgeStaleTokens(gomock.Any(), gomock.Any()).Return(nil).Times(2) // the first login and the one after an hour
	login := func() {
		if _, _, _, err := a.Login(context.Background(), "frodo@shire.me", "a long enough password"); err != nil {
			t.Fatal(err)
		}
	}
	login()
	*now = now.Add(59 * time.Minute)
	login()
	*now = now.Add(2 * time.Minute)
	login()
}
