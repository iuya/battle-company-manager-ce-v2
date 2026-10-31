package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func loginCreds(t *testing.T) Credentials {
	hash, _ := hashPassword(goodPassword, fastArgon)
	return Credentials{User: User{ID: 7, Email: "frodo@shire.me", DisplayName: "Frodo"}, PasswordHash: hash}
}

func TestAuth_Login_InvalidatesTheRecoveryLink(t *testing.T) {
	ctx := context.Background()
	t.Run("a failed password does nothing to the link (no InvalidateRecovery call is expected)", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "frodo@shire.me").Return(loginCreds(t), nil)
		if _, _, _, err := a.Login(ctx, "frodo@shire.me", "wrong password here"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a failing invalidation fails the login closed and opens no session", func(t *testing.T) {
		a, repo := newTestAuth(t)
		boom := errors.New("boom")
		repo.EXPECT().UserByEmail(gomock.Any(), "frodo@shire.me").Return(loginCreds(t), nil)
		repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByLogin, t0).Return(0, boom)
		if _, _, _, err := a.Login(ctx, "frodo@shire.me", goodPassword); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("housekeeping failures do not fail the login", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserByEmail(gomock.Any(), "frodo@shire.me").Return(loginCreds(t), nil)
		repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByLogin, t0).Return(1, nil)
		repo.EXPECT().CreateSession(gomock.Any(), gomock.Any(), int64(7), gomock.Any()).Return(nil)
		repo.EXPECT().DeleteExpiredSessions(gomock.Any(), t0).Return(errors.New("x"))
		repo.EXPECT().PurgeStaleTokens(gomock.Any(), gomock.Any()).Return(errors.New("y"))
		if _, _, _, err := a.Login(ctx, "frodo@shire.me", goodPassword); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a login clears the pending flag of the cached sessions", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil)
		if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
			t.Fatal(err)
		}
		a.setRecoveryPending(7, true) // a link was generated
		repo.EXPECT().UserByEmail(gomock.Any(), "frodo@shire.me").Return(loginCreds(t), nil)
		repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByLogin, t0).Return(1, nil)
		repo.EXPECT().CreateSession(gomock.Any(), gomock.Any(), int64(7), gomock.Any()).Return(nil)
		repo.EXPECT().DeleteExpiredSessions(gomock.Any(), t0).Return(nil)
		repo.EXPECT().PurgeStaleTokens(gomock.Any(), gomock.Any()).Return(nil)
		if _, _, _, err := a.Login(ctx, "frodo@shire.me", goodPassword); err != nil {
			t.Fatal(err)
		}
		// no InvalidateRecovery(.., access) is expected: the login already did it
		if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestAuth_Authenticate_InvalidatesAPendingRecoveryLink(t *testing.T) {
	ctx := context.Background()
	session := func(pending bool) func(*MockauthRepository) {
		return func(repo *MockauthRepository) {
			repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), pending, nil)
		}
	}
	t.Run("a cache miss with a pending link invalidates it once, then the cache answers with zero calls", func(t *testing.T) {
		a, repo := newTestAuth(t)
		session(true)(repo)
		repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByAccess, t0).Return(1, nil).Times(1)
		for range 3 {
			if u, _, err := a.Authenticate(ctx, "tok"); err != nil || u.ID != 7 {
				t.Fatalf("%+v %v", u, err)
			}
		}
	})
	t.Run("a hit with no pending link makes no repository call at all", func(t *testing.T) {
		a, repo := newTestAuth(t)
		session(false)(repo) // exactly one lookup, nothing else
		for range 5 {
			if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("Generate sets the flag on a hot entry: the next request invalidates at once", func(t *testing.T) {
		a, repo := newTestAuth(t)
		session(false)(repo)
		if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
			t.Fatal(err)
		}
		a.setRecoveryPending(7, true)
		repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByAccess, t0).Return(1, nil).Times(1)
		for range 3 {
			if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("the flag is cleared for all the user's cached sessions", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("a"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("b"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil)
		for _, tok := range []string{"a", "b"} {
			if _, _, err := a.Authenticate(ctx, tok); err != nil {
				t.Fatal(err)
			}
		}
		a.setRecoveryPending(7, true)
		repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByAccess, t0).Return(1, nil).Times(1)
		for _, tok := range []string{"a", "b", "a", "b"} {
			if _, _, err := a.Authenticate(ctx, tok); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("the flag of another user is untouched", func(t *testing.T) {
		a, repo := newTestAuth(t)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("a"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("b"), t0).Return(User{ID: 8}, t0.Add(sessionTTL), false, nil)
		for _, tok := range []string{"a", "b"} {
			_, _, _ = a.Authenticate(ctx, tok)
		}
		a.setRecoveryPending(7, true)
		if _, _, err := a.Authenticate(ctx, "b"); err != nil { // no repository call expected
			t.Fatal(err)
		}
	})
	t.Run("a failing invalidation is an error, not a silent pass, and the flag stays set", func(t *testing.T) {
		a, repo := newTestAuth(t)
		boom := errors.New("boom")
		session(false)(repo)
		if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
			t.Fatal(err)
		}
		a.setRecoveryPending(7, true)
		gomock.InOrder(
			repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByAccess, t0).Return(0, boom),
			repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByAccess, t0).Return(1, nil),
		)
		if _, _, err := a.Authenticate(ctx, "tok"); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
		if _, _, err := a.Authenticate(ctx, "tok"); err != nil { // the retry reaches the repository again
			t.Fatal(err)
		}
	})
	t.Run("a failing invalidation on a miss caches nothing", func(t *testing.T) {
		a, repo := newTestAuth(t)
		boom := errors.New("boom")
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), true, nil).Times(2)
		gomock.InOrder(
			repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByAccess, t0).Return(0, boom),
			repo.EXPECT().InvalidateRecovery(gomock.Any(), int64(7), RecoveryByAccess, t0).Return(1, nil),
		)
		if _, _, err := a.Authenticate(ctx, "tok"); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
		if _, _, err := a.Authenticate(ctx, "tok"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a stale cached flag costs one harmless update, once a minute at most", func(t *testing.T) {
		a, repo, now := newClockedAuth(t)
		repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), gomock.Any()).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil).Times(2)
		_, _, _ = a.Authenticate(ctx, "tok")
		*now = t0.Add(sessionCacheTTL + time.Second) // the entry is re-read from the database
		_, _, _ = a.Authenticate(ctx, "tok")
	})
}

func TestAuth_ImplementsSessionControl(t *testing.T) {
	var _ sessionControl = (*Auth)(nil)
	a, repo := newTestAuth(t)
	repo.EXPECT().UserBySession(gomock.Any(), hashToken("tok"), t0).Return(User{ID: 7}, t0.Add(sessionTTL), false, nil).Times(2)
	_, _, _ = a.Authenticate(context.Background(), "tok")
	a.forgetUser(7)
	_, _, _ = a.Authenticate(context.Background(), "tok") // the second lookup proves the entry was dropped
}
