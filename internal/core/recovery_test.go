package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func newRecoveryService(t *testing.T) (*Recovery, *MockrecoveryRepository, *MocksessionControl) {
	ctrl := gomock.NewController(t)
	repo, sessions := NewMockrecoveryRepository(ctrl), NewMocksessionControl(ctrl)
	s := NewRecovery(repo, sessions, WithClock(func() time.Time { return t0 }), withArgon(fastArgon),
		WithRandom(bytes.NewReader(bytes.Repeat([]byte{9}, 1024))))
	return s, repo, sessions
}

func TestRecovery_Generate(t *testing.T) {
	ctx := context.Background()
	super := User{ID: 1, Role: RoleSuperuser}
	type tc struct {
		name    string
		ttl     time.Duration
		userID  int64
		want    time.Duration
		wantErr error
	}
	for _, c := range []tc{
		{name: "default 24 h", userID: 7, want: 24 * time.Hour},
		{name: "one hour", ttl: time.Hour, userID: 7, want: time.Hour},
		{name: "seven days", ttl: 7 * 24 * time.Hour, userID: 7, want: 7 * 24 * time.Hour},
		{name: "eight days", ttl: 8 * 24 * time.Hour, userID: 7, wantErr: ErrInvalidInput},
		{name: "59 minutes", ttl: 59 * time.Minute, userID: 7, wantErr: ErrInvalidInput},
		{name: "negative", ttl: -time.Hour, userID: 7, wantErr: ErrInvalidInput},
		{name: "own account", userID: 1, wantErr: ErrRefused},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, repo, sessions := newRecoveryService(t)
			var recorded string
			if c.wantErr == nil {
				repo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in StoredRecovery, a AuditEntry) (RecoveryLink, error) {
					recorded = fmt.Sprintf("%+v %+v", in, a)
					if in.UserID != c.userID || in.CreatedBy != 1 || !in.CreatedAt.Equal(t0) || !in.ExpiresAt.Equal(t0.Add(c.want)) {
						t.Errorf("stored %+v", in)
					}
					if a.Action != AuditRecoveryCreated || a.Source != AuditWeb || a.ActorID == nil || *a.ActorID != 1 || a.SubjectUserID == nil || *a.SubjectUserID != c.userID {
						t.Errorf("audit %+v", a)
					}
					return RecoveryLink{ID: 3, UserID: in.UserID, ExpiresAt: in.ExpiresAt}, nil
				})
				sessions.EXPECT().setRecoveryPending(c.userID, true)
			}
			got, err := s.Generate(ctx, super, c.userID, c.ttl)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseToken(got.Token); err != nil || got.ID != 3 || !got.ExpiresAt.Equal(t0.Add(c.want)) {
				t.Fatalf("%+v %v", got, err)
			}
			if strings.Contains(recorded, got.Token) {
				t.Fatal("the raw token reached the repository or the audit entry")
			}
		})
	}
	t.Run("only a superuser", func(t *testing.T) {
		for _, actor := range []User{{ID: 2, Role: RoleAdmin}, {ID: 3, Role: RoleUser, CanCreateCampaigns: true}, {ID: 4, Role: RoleUser}} {
			s, _, _ := newRecoveryService(t)
			if _, err := s.Generate(ctx, actor, 7, 0); !errors.Is(err, ErrForbidden) {
				t.Errorf("%v: got %v", actor.Role, err)
			}
		}
	})
	t.Run("closed and unknown accounts", func(t *testing.T) {
		s, repo, _ := newRecoveryService(t)
		repo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(RecoveryLink{}, ErrAccountClosed)
		if _, err := s.Generate(ctx, super, 7, 0); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "closed") {
			t.Fatalf("closed: %v", err)
		}
		repo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(RecoveryLink{}, ErrNotFound)
		if _, err := s.Generate(ctx, super, 8, 0); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown: %v", err)
		}
	})
	t.Run("a conflict and failures show no token and do not set the flag", func(t *testing.T) {
		s, repo, _ := newRecoveryService(t)
		repo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(RecoveryLink{}, ErrConflict)
		got, err := s.Generate(ctx, super, 7, 0)
		if !errors.Is(err, ErrConflict) || got.Token != "" {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("the repository replaces the live link and the flag is set", func(t *testing.T) {
		s, repo, sessions := newRecoveryService(t)
		repo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(RecoveryLink{ID: 4}, nil)
		sessions.EXPECT().setRecoveryPending(int64(7), true).Times(1)
		if _, err := s.Generate(ctx, super, 7, 0); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRecovery_Revoke(t *testing.T) {
	ctx := context.Background()
	t.Run("a pending link", func(t *testing.T) {
		s, repo, sessions := newRecoveryService(t)
		repo.EXPECT().Revoke(gomock.Any(), int64(7), int64(1), t0, gomock.Any()).DoAndReturn(func(_ context.Context, _, _ int64, _ time.Time, a AuditEntry) error {
			if a.Action != AuditRecoveryRevoked || a.ActorID == nil || *a.ActorID != 1 || a.SubjectUserID == nil || *a.SubjectUserID != 7 {
				t.Errorf("audit %+v", a)
			}
			return nil
		})
		sessions.EXPECT().setRecoveryPending(int64(7), false)
		if err := s.Revoke(ctx, User{ID: 1, Role: RoleSuperuser}, 7); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no pending link", func(t *testing.T) {
		s, repo, _ := newRecoveryService(t)
		repo.EXPECT().Revoke(gomock.Any(), int64(7), int64(1), t0, gomock.Any()).Return(ErrNotFound)
		err := s.Revoke(ctx, User{ID: 1, Role: RoleSuperuser}, 7)
		if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "no pending link") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("an administrator may not", func(t *testing.T) {
		s, _, _ := newRecoveryService(t)
		if err := s.Revoke(ctx, User{ID: 2, Role: RoleAdmin}, 7); !errors.Is(err, ErrForbidden) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("an internal error passes", func(t *testing.T) {
		s, repo, _ := newRecoveryService(t)
		boom := errors.New("boom")
		repo.EXPECT().Revoke(gomock.Any(), int64(7), int64(1), t0, gomock.Any()).Return(boom)
		if err := s.Revoke(ctx, User{ID: 1, Role: RoleSuperuser}, 7); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
}

func recoveryDeadPeeks() map[string]func(*MockrecoveryRepository, string) {
	peek := func(state RecoveryState, hash []byte) func(*MockrecoveryRepository, string) {
		return func(r *MockrecoveryRepository, tok string) {
			r.EXPECT().Peek(gomock.Any(), hashToken(tok), t0).Return(RecoveryPeek{TokenHash: hash, UserID: 7, State: state}, nil)
		}
	}
	h := hashToken(someToken)
	return map[string]func(*MockrecoveryRepository, string){
		"unknown": func(r *MockrecoveryRepository, tok string) {
			r.EXPECT().Peek(gomock.Any(), hashToken(tok), t0).Return(RecoveryPeek{}, ErrNotFound)
		},
		"used":            peek(RecoveryUsed, h),
		"revoked":         peek(RecoveryRevoked, h),
		"invalidated":     peek(RecoveryInvalidated, h),
		"expired":         peek(RecoveryExpired, h),
		"mismatching row": peek(RecoveryPending, hashToken("another")),
	}
}

func TestRecovery_PublicFailuresAreAllTheSame(t *testing.T) {
	ctx := context.Background()
	for name, expect := range recoveryDeadPeeks() {
		t.Run(name, func(t *testing.T) {
			s, repo, _ := newRecoveryService(t)
			expect(repo, someToken)
			if err := s.Preview(ctx, someToken); err != ErrLinkUnusable {
				t.Errorf("preview: %v", err)
			}
			expect(repo, someToken)
			if err := s.SetPassword(ctx, someToken, "short"); err != ErrLinkUnusable { // a weak password does not change the answer
				t.Errorf("set password: %v", err)
			}
		})
	}
	for _, tok := range malformedTokens {
		s, _, _ := newRecoveryService(t)
		if s.Preview(ctx, tok) != ErrLinkUnusable || s.SetPassword(ctx, tok, goodPassword) != ErrLinkUnusable {
			t.Errorf("malformed %q", tok)
		}
	}
	t.Run("the link died between the peek and the lock", func(t *testing.T) {
		s, repo, sessions := newRecoveryService(t)
		repo.EXPECT().Peek(gomock.Any(), hashToken(someToken), t0).Return(RecoveryPeek{TokenHash: hashToken(someToken), UserID: 7, State: RecoveryPending}, nil)
		repo.EXPECT().Use(gomock.Any(), hashToken(someToken), gomock.Any(), t0, gomock.Any()).Return(int64(0), ErrNotFound)
		_ = sessions // no forgetUser is expected
		if err := s.SetPassword(ctx, someToken, goodPassword); err != ErrLinkUnusable {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("an internal error is not hidden", func(t *testing.T) {
		s, repo, _ := newRecoveryService(t)
		boom := errors.New("boom")
		repo.EXPECT().Peek(gomock.Any(), gomock.Any(), t0).Return(RecoveryPeek{}, boom)
		if err := s.Preview(ctx, someToken); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestRecovery_PreviewPending(t *testing.T) {
	s, repo, _ := newRecoveryService(t)
	repo.EXPECT().Peek(gomock.Any(), hashToken(someToken), t0).Return(RecoveryPeek{TokenHash: hashToken(someToken), UserID: 7, State: RecoveryPending}, nil)
	if err := s.Preview(context.Background(), someToken); err != nil {
		t.Fatal(err)
	}
}

func TestRecovery_SetPassword(t *testing.T) {
	ctx := context.Background()
	pending := func(repo *MockrecoveryRepository) {
		repo.EXPECT().Peek(gomock.Any(), hashToken(someToken), t0).Return(RecoveryPeek{TokenHash: hashToken(someToken), UserID: 7, State: RecoveryPending}, nil)
	}
	t.Run("success stores an argon2id hash, audits, and drops the cached sessions", func(t *testing.T) {
		s, repo, sessions := newRecoveryService(t)
		pending(repo)
		repo.EXPECT().Use(gomock.Any(), hashToken(someToken), gomock.Any(), t0, gomock.Any()).DoAndReturn(
			func(_ context.Context, _ []byte, pw string, _ time.Time, audit func(int64, int64) AuditEntry) (int64, error) {
				if ok, err := verifyPassword(goodPassword, pw); !ok || err != nil || !strings.HasPrefix(pw, "$argon2id$") {
					t.Errorf("hash %q", pw)
				}
				e := audit(7, 3)
				if e.Action != AuditRecoveryUsed || e.Source != AuditPublic || e.ActorID != nil || *e.SubjectUserID != 7 || e.TargetID != 3 {
					t.Errorf("audit %+v", e)
				}
				if strings.Contains(fmt.Sprintf("%+v", e), someToken) {
					t.Error("token in audit")
				}
				return 7, nil
			})
		sessions.EXPECT().forgetUser(int64(7))
		sessions.EXPECT().setRecoveryPending(int64(7), false)
		if err := s.SetPassword(ctx, someToken, goodPassword); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a weak password is rejected before any write", func(t *testing.T) {
		for _, pw := range []string{"", strings.Repeat("p", 9), strings.Repeat("p", 129), strings.Repeat("é", 9)} {
			s, repo, _ := newRecoveryService(t)
			pending(repo)
			if err := s.SetPassword(ctx, someToken, pw); !errors.Is(err, ErrInvalidUser) {
				t.Errorf("%d chars: %v", len(pw), err)
			}
		}
	})
	t.Run("lengths 10 and 128", func(t *testing.T) {
		for _, n := range []int{10, 128} {
			s, repo, sessions := newRecoveryService(t)
			pending(repo)
			repo.EXPECT().Use(gomock.Any(), gomock.Any(), gomock.Any(), t0, gomock.Any()).Return(int64(7), nil)
			sessions.EXPECT().forgetUser(int64(7))
			sessions.EXPECT().setRecoveryPending(int64(7), false)
			if err := s.SetPassword(ctx, someToken, strings.Repeat("p", n)); err != nil {
				t.Errorf("%d: %v", n, err)
			}
		}
	})
	t.Run("a repository failure is returned", func(t *testing.T) {
		s, repo, _ := newRecoveryService(t)
		boom := errors.New("boom")
		pending(repo)
		repo.EXPECT().Use(gomock.Any(), gomock.Any(), gomock.Any(), t0, gomock.Any()).Return(int64(0), boom)
		if err := s.SetPassword(ctx, someToken, goodPassword); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestRecoveryStatusStateAt(t *testing.T) {
	later, earlier := t0.Add(time.Hour), t0.Add(-time.Hour)
	for _, c := range []struct {
		name string
		r    RecoveryStatus
		want RecoveryState
	}{
		{"pending", RecoveryStatus{ExpiresAt: later}, RecoveryPending},
		{"expires_at == now is expired", RecoveryStatus{ExpiresAt: t0}, RecoveryExpired},
		{"used", RecoveryStatus{ExpiresAt: later, UsedAt: &earlier}, RecoveryUsed},
		{"revoked", RecoveryStatus{ExpiresAt: later, RevokedAt: &earlier}, RecoveryRevoked},
		{"invalidated", RecoveryStatus{ExpiresAt: later, InvalidatedAt: &earlier, Reason: RecoveryByLogin}, RecoveryInvalidated},
		{"used after expiry stays used", RecoveryStatus{ExpiresAt: earlier, UsedAt: &earlier}, RecoveryUsed},
	} {
		if got := c.r.StateAt(t0); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}
