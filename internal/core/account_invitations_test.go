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

const goodPassword = "a long enough password"

func newInvService(t *testing.T) (*AccountInvitations, *MockaccountInvitationRepository) {
	repo := NewMockaccountInvitationRepository(gomock.NewController(t))
	s := NewAccountInvitations(repo, WithClock(func() time.Time { return t0 }), withArgon(fastArgon),
		WithRandom(bytes.NewReader(bytes.Repeat([]byte{7}, 4096))))
	return s, repo
}

var (
	invAdmin = User{ID: 2, Role: RoleAdmin}
	invSuper = User{ID: 1, Role: RoleSuperuser}
	// a well-formed token that no repository knows
	someToken = strings.Repeat("k", 43)
)

func TestAccountInvitations_Create(t *testing.T) {
	ctx := context.Background()
	type tc struct {
		name    string
		actor   User
		in      NewAccountInvitation
		hasAcct bool // EmailHasAccount answers true
		wantErr error
		stored  StoredAccountInvitation // compared on the fields below when wantErr is nil
	}
	week := 7 * 24 * time.Hour
	cases := []tc{
		{name: "named, defaults, email normalised", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "  Pippin@Shire.me "},
			stored: StoredAccountInvitation{Kind: InvitationNamed, Email: "pippin@shire.me", MaxUses: 1, ExpiresAt: t0.Add(week), CreatedBy: 2}},
		{name: "named ignores max uses and keeps the flag", actor: invSuper, in: NewAccountInvitation{Kind: InvitationNamed, Email: "p@s.me", MaxUses: 40, CanCreateCampaigns: true, Expiry: 30 * 24 * time.Hour},
			stored: StoredAccountInvitation{Kind: InvitationNamed, Email: "p@s.me", MaxUses: 1, CanCreateCampaigns: true, ExpiresAt: t0.Add(30 * 24 * time.Hour), CreatedBy: 1}},
		{name: "one hour is the minimum", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "p@s.me", Expiry: time.Hour},
			stored: StoredAccountInvitation{Kind: InvitationNamed, Email: "p@s.me", MaxUses: 1, ExpiresAt: t0.Add(time.Hour), CreatedBy: 2}},
		{name: "anonymous default 10 uses, no email", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationAnonymous, Email: "ignored@s.me"},
			stored: StoredAccountInvitation{Kind: InvitationAnonymous, MaxUses: 10, ExpiresAt: t0.Add(week), CreatedBy: 2}},
		{name: "anonymous 1 and 50 uses", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationAnonymous, MaxUses: 50, Expiry: 24 * time.Hour},
			stored: StoredAccountInvitation{Kind: InvitationAnonymous, MaxUses: 50, ExpiresAt: t0.Add(24 * time.Hour), CreatedBy: 2}},
		{name: "anonymous 1 use", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationAnonymous, MaxUses: 1},
			stored: StoredAccountInvitation{Kind: InvitationAnonymous, MaxUses: 1, ExpiresAt: t0.Add(week), CreatedBy: 2}},

		{name: "31 days", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "p@s.me", Expiry: 31 * 24 * time.Hour}, wantErr: ErrInvalidInput},
		{name: "30 days and a second", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "p@s.me", Expiry: 30*24*time.Hour + time.Second}, wantErr: ErrInvalidInput},
		{name: "59 minutes", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "p@s.me", Expiry: 59 * time.Minute}, wantErr: ErrInvalidInput},
		{name: "negative expiry", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "p@s.me", Expiry: -time.Hour}, wantErr: ErrInvalidInput},
		{name: "bad email", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "nope"}, wantErr: ErrInvalidInput},
		{name: "display-name email", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "Pip <p@s.me>"}, wantErr: ErrInvalidInput},
		{name: "empty email", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed}, wantErr: ErrInvalidInput},
		{name: "unknown kind", actor: invAdmin, in: NewAccountInvitation{Kind: "group"}, wantErr: ErrInvalidInput},
		{name: "empty kind", actor: invAdmin, in: NewAccountInvitation{}, wantErr: ErrInvalidInput},
		{name: "anonymous 51 uses", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationAnonymous, MaxUses: 51}, wantErr: ErrInvalidInput},
		{name: "anonymous negative uses", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationAnonymous, MaxUses: -1}, wantErr: ErrInvalidInput},
		{name: "address that has an account", actor: invAdmin, in: NewAccountInvitation{Kind: InvitationNamed, Email: "frodo@shire.me"}, hasAcct: true, wantErr: ErrRefused},

		{name: "plain user", actor: User{ID: 5, Role: RoleUser}, in: NewAccountInvitation{Kind: InvitationNamed, Email: "p@s.me"}, wantErr: ErrForbidden},
		{name: "organiser", actor: User{ID: 5, Role: RoleUser, CanCreateCampaigns: true}, in: NewAccountInvitation{Kind: InvitationAnonymous}, wantErr: ErrForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, repo := newInvService(t)
			if c.in.Kind == InvitationNamed && (c.wantErr == nil || c.wantErr == ErrRefused) {
				repo.EXPECT().EmailHasAccount(gomock.Any(), gomock.Any()).Return(c.hasAcct, nil)
			}
			var seen StoredAccountInvitation
			if c.wantErr == nil {
				repo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in StoredAccountInvitation, a AuditEntry) (AccountInvitation, error) {
						seen = in
						if a.Action != AuditInvitationCreated || a.Source != AuditWeb || a.ActorID == nil || *a.ActorID != c.actor.ID || !a.At.Equal(t0) ||
							a.TargetKind != AuditTargetInvitation || a.Detail["kind"] != string(in.Kind) {
							t.Errorf("audit: %+v", a)
						}
						return AccountInvitation{ID: 11, Kind: in.Kind, Email: in.Email, MaxUses: in.MaxUses, ExpiresAt: in.ExpiresAt, State: InvitationPending}, nil
					})
			}
			got, err := s.Create(ctx, c.actor, c.in)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("got %v, want %v", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if seen.Kind != c.stored.Kind || seen.Email != c.stored.Email || seen.MaxUses != c.stored.MaxUses || seen.CanCreateCampaigns != c.stored.CanCreateCampaigns ||
				!seen.ExpiresAt.Equal(c.stored.ExpiresAt) || !seen.CreatedAt.Equal(t0) || seen.CreatedBy != c.stored.CreatedBy {
				t.Errorf("stored %+v, want %+v", seen, c.stored)
			}
			// the token is returned once and its hash is what was stored
			if _, err := parseToken(got.Token); err != nil || !bytes.Equal(hashToken(got.Token), seen.TokenHash) {
				t.Errorf("token %q does not match the stored hash", got.Token)
			}
			if got.ID != 11 {
				t.Errorf("the repository's row is returned: %+v", got)
			}
		})
	}
}

func TestAccountInvitations_Create_NeverStoresTheToken(t *testing.T) {
	s, repo := newInvService(t)
	var recorded string
	repo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in StoredAccountInvitation, a AuditEntry) (AccountInvitation, error) {
		recorded = fmt.Sprintf("%+v %#v %+v", in, in, a)
		return AccountInvitation{ID: 1}, nil
	})
	got, err := s.Create(context.Background(), invAdmin, NewAccountInvitation{Kind: InvitationAnonymous})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(recorded, got.Token) {
		t.Fatal("the raw token reached the repository or the audit entry")
	}
}

func TestAccountInvitations_Create_RepositoryErrorsPass(t *testing.T) {
	s, repo := newInvService(t)
	boom := errors.New("boom")
	repo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(AccountInvitation{}, boom)
	if got, err := s.Create(context.Background(), invAdmin, NewAccountInvitation{Kind: InvitationAnonymous}); !errors.Is(err, boom) || got.Token != "" {
		t.Fatalf("got %+v %v: no token must be shown when nothing was stored", got, err)
	}
	repo2 := NewMockaccountInvitationRepository(gomock.NewController(t))
	s2 := NewAccountInvitations(repo2, WithClock(func() time.Time { return t0 }), WithRandom(bytes.NewReader(nil)))
	if _, err := s2.Create(context.Background(), invAdmin, NewAccountInvitation{Kind: InvitationAnonymous}); err == nil {
		t.Fatal("a failing random source is an error")
	}
}

func TestAccountInvitations_List(t *testing.T) {
	ctx := context.Background()
	s, repo := newInvService(t)
	if _, _, err := s.List(ctx, User{Role: RoleUser, CanCreateCampaigns: true}, InvitationFilter{}, 0, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v", err)
	}
	for _, c := range []struct{ limit, offset, wantLimit, wantOffset int }{{0, -1, 25, 0}, {10, 20, 10, 20}, {900, 0, 200, 0}} {
		repo.EXPECT().List(gomock.Any(), InvitationFilter{State: InvitationPending}, t0, c.wantLimit, c.wantOffset).Return([]AccountInvitation{{ID: 1}}, 3, nil)
		rows, total, err := s.List(ctx, invAdmin, InvitationFilter{State: InvitationPending}, c.limit, c.offset)
		if err != nil || total != 3 || len(rows) != 1 {
			t.Fatalf("%+v: %v %v %v", c, rows, total, err)
		}
	}
	if _, _, err := s.List(ctx, invSuper, InvitationFilter{State: "weird"}, 0, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
}

func TestAccountInvitations_Revoke(t *testing.T) {
	ctx := context.Background()
	t.Run("a pending invitation", func(t *testing.T) {
		s, repo := newInvService(t)
		repo.EXPECT().Revoke(gomock.Any(), int64(5), int64(2), t0, gomock.Any()).DoAndReturn(func(_ context.Context, id, by int64, _ time.Time, a AuditEntry) (AccountInvitation, error) {
			if a.Action != AuditInvitationRevoked || a.TargetID != 5 || a.ActorID == nil || *a.ActorID != 2 || a.TargetKind != AuditTargetInvitation {
				t.Errorf("audit: %+v", a)
			}
			return AccountInvitation{ID: 5, State: InvitationRevoked}, nil
		})
		if got, err := s.Revoke(ctx, invAdmin, 5); err != nil || got.State != InvitationRevoked {
			t.Fatalf("%+v %v", got, err)
		}
	})
	for _, state := range []InvitationState{InvitationUsed, InvitationExpired, InvitationRevoked} {
		t.Run(string(state), func(t *testing.T) {
			s, repo := newInvService(t)
			repo.EXPECT().Revoke(gomock.Any(), int64(5), int64(1), t0, gomock.Any()).Return(AccountInvitation{}, &NotPendingError{State: state})
			_, err := s.Revoke(ctx, invSuper, 5)
			if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "already "+string(state)) {
				t.Fatalf("got %v", err)
			}
		})
	}
	t.Run("unknown", func(t *testing.T) {
		s, repo := newInvService(t)
		repo.EXPECT().Revoke(gomock.Any(), int64(9), int64(2), t0, gomock.Any()).Return(AccountInvitation{}, ErrNotFound)
		if _, err := s.Revoke(ctx, invAdmin, 9); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("not an administrator", func(t *testing.T) {
		s, _ := newInvService(t)
		if _, err := s.Revoke(ctx, User{ID: 3, Role: RoleUser}, 5); !errors.Is(err, ErrForbidden) {
			t.Fatalf("got %v", err)
		}
	})
}

// deadLinks are the causes that must all look the same to the public.
func deadPeeks() map[string]func(repo *MockaccountInvitationRepository, tok string) {
	return map[string]func(*MockaccountInvitationRepository, string){
		"unknown": func(r *MockaccountInvitationRepository, tok string) {
			r.EXPECT().Peek(gomock.Any(), hashToken(tok), t0).Return(InvitationPeek{}, ErrNotFound)
		},
		"revoked": func(r *MockaccountInvitationRepository, tok string) {
			r.EXPECT().Peek(gomock.Any(), hashToken(tok), t0).Return(InvitationPeek{TokenHash: hashToken(tok), Kind: InvitationNamed, Email: "p@s.me", State: InvitationRevoked}, nil)
		},
		"used": func(r *MockaccountInvitationRepository, tok string) {
			r.EXPECT().Peek(gomock.Any(), hashToken(tok), t0).Return(InvitationPeek{TokenHash: hashToken(tok), Kind: InvitationAnonymous, State: InvitationUsed}, nil)
		},
		"expired": func(r *MockaccountInvitationRepository, tok string) {
			r.EXPECT().Peek(gomock.Any(), hashToken(tok), t0).Return(InvitationPeek{TokenHash: hashToken(tok), Kind: InvitationNamed, Email: "p@s.me", State: InvitationExpired}, nil)
		},
		"a row whose hash does not match": func(r *MockaccountInvitationRepository, tok string) {
			r.EXPECT().Peek(gomock.Any(), hashToken(tok), t0).Return(InvitationPeek{TokenHash: hashToken("another"), Kind: InvitationNamed, Email: "p@s.me", State: InvitationPending}, nil)
		},
	}
}

var malformedTokens = []string{"", "short", strings.Repeat("a", 44), strings.Repeat("a", 42) + "+", strings.Repeat("é", 43)}

func TestAccountInvitations_PublicFailuresAreAllTheSame(t *testing.T) {
	ctx := context.Background()
	reg := Registration{Email: "p@s.me", DisplayName: "Pippin", Password: goodPassword}
	check := func(t *testing.T, what string, err error) {
		t.Helper()
		if err != ErrLinkUnusable || err.Error() != TalkToAdministrator {
			t.Errorf("%s: got %v, want exactly ErrLinkUnusable", what, err)
		}
	}
	for name, expect := range deadPeeks() {
		t.Run("preview "+name, func(t *testing.T) {
			s, repo := newInvService(t)
			expect(repo, someToken)
			_, err := s.Preview(ctx, someToken)
			check(t, name, err)
		})
		t.Run("register "+name, func(t *testing.T) { // no repository write, and invalid fields do not change the answer
			s, repo := newInvService(t)
			expect(repo, someToken)
			_, err := s.Register(ctx, someToken, Registration{Email: "bad", DisplayName: "", Password: "x"})
			check(t, name, err)
		})
	}
	for _, tok := range malformedTokens {
		t.Run(fmt.Sprintf("malformed %q", tok), func(t *testing.T) { // no repository call at all
			s, _ := newInvService(t)
			_, err := s.Preview(ctx, tok)
			check(t, "preview", err)
			_, err = s.Register(ctx, tok, reg)
			check(t, "register", err)
		})
	}
	t.Run("register: the address already has an account", func(t *testing.T) {
		s, repo := newInvService(t)
		repo.EXPECT().Peek(gomock.Any(), hashToken(someToken), t0).Return(InvitationPeek{TokenHash: hashToken(someToken), Kind: InvitationAnonymous, State: InvitationPending}, nil)
		repo.EXPECT().Register(gomock.Any(), gomock.Any()).Return(User{}, ErrEmailTaken)
		_, err := s.Register(ctx, someToken, reg)
		check(t, "email taken", err)
	})
	t.Run("register: the invitation died between the peek and the lock", func(t *testing.T) {
		s, repo := newInvService(t)
		repo.EXPECT().Peek(gomock.Any(), hashToken(someToken), t0).Return(InvitationPeek{TokenHash: hashToken(someToken), Kind: InvitationAnonymous, State: InvitationPending}, nil)
		repo.EXPECT().Register(gomock.Any(), gomock.Any()).Return(User{}, ErrNotFound)
		_, err := s.Register(ctx, someToken, reg)
		check(t, "lost the race", err)
	})
	t.Run("an internal error is not hidden as a dead link", func(t *testing.T) {
		s, repo := newInvService(t)
		boom := errors.New("boom")
		repo.EXPECT().Peek(gomock.Any(), gomock.Any(), t0).Return(InvitationPeek{}, boom)
		if _, err := s.Preview(ctx, someToken); !errors.Is(err, boom) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestAccountInvitations_Preview(t *testing.T) {
	s, repo := newInvService(t)
	repo.EXPECT().Peek(gomock.Any(), hashToken(someToken), t0).Return(InvitationPeek{TokenHash: hashToken(someToken), Kind: InvitationNamed, Email: "p@s.me", State: InvitationPending}, nil)
	got, err := s.Preview(context.Background(), someToken)
	if err != nil || got != (InvitationPreview{Kind: InvitationNamed, Email: "p@s.me"}) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestAccountInvitations_Register(t *testing.T) {
	ctx := context.Background()
	pending := func(kind InvitationKind, email string) InvitationPeek {
		return InvitationPeek{TokenHash: hashToken(someToken), Kind: kind, Email: email, State: InvitationPending}
	}
	t.Run("a named invitation uses the stored address and ignores the submitted one", func(t *testing.T) {
		s, repo := newInvService(t)
		repo.EXPECT().Peek(gomock.Any(), hashToken(someToken), t0).Return(pending(InvitationNamed, "pippin@shire.me"), nil)
		var sessionHash []byte
		repo.EXPECT().Register(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in RegisterAccount) (User, error) {
			sessionHash = in.SessionHash
			if in.Email != "pippin@shire.me" || in.DisplayName != "Pippin" || !in.Now.Equal(t0) || !in.SessionExpires.Equal(t0.Add(sessionTTL)) ||
				!bytes.Equal(in.TokenHash, hashToken(someToken)) {
				t.Errorf("register input: %+v", in)
			}
			if ok, err := verifyPassword(goodPassword, in.PasswordHash); !ok || err != nil || !strings.HasPrefix(in.PasswordHash, "$argon2id$") {
				t.Errorf("the stored hash must be argon2id of the password: %q", in.PasswordHash)
			}
			e := in.Audit(42, 5)
			if e.Action != AuditInvitationUsed || e.Source != AuditPublic || e.ActorID != nil || e.SubjectUserID == nil || *e.SubjectUserID != 42 || e.TargetID != 5 {
				t.Errorf("audit: %+v", e)
			}
			return User{ID: 42, Email: in.Email, DisplayName: in.DisplayName}, nil
		})
		got, err := s.Register(ctx, someToken, Registration{Email: "evil@x.me", DisplayName: "  Pippin ", Password: goodPassword})
		if err != nil {
			t.Fatal(err)
		}
		if got.User.ID != 42 || !got.Expires.Equal(t0.Add(sessionTTL)) || !bytes.Equal(hashToken(got.Token), sessionHash) {
			t.Errorf("session %+v does not match the hash handed to the repository", got)
		}
	})
	t.Run("an anonymous invitation takes the submitted address, normalised", func(t *testing.T) {
		s, repo := newInvService(t)
		repo.EXPECT().Peek(gomock.Any(), hashToken(someToken), t0).Return(pending(InvitationAnonymous, ""), nil)
		repo.EXPECT().Register(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in RegisterAccount) (User, error) {
			if in.Email != "merry@shire.me" {
				t.Errorf("email %q", in.Email)
			}
			return User{ID: 43}, nil
		})
		if _, err := s.Register(ctx, someToken, Registration{Email: " Merry@Shire.me", DisplayName: "Merry", Password: goodPassword}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("field validation of a valid link, with no write", func(t *testing.T) {
		long := func(n int) string { return strings.Repeat("p", n) }
		for name, c := range map[string]struct {
			kind InvitationKind
			in   Registration
		}{
			"bad email":            {InvitationAnonymous, Registration{Email: "nope", DisplayName: "M", Password: goodPassword}},
			"empty email":          {InvitationAnonymous, Registration{DisplayName: "M", Password: goodPassword}},
			"empty name":           {InvitationNamed, Registration{DisplayName: "  ", Password: goodPassword}},
			"name 61":              {InvitationNamed, Registration{DisplayName: strings.Repeat("n", 61), Password: goodPassword}},
			"password 9":           {InvitationNamed, Registration{DisplayName: "M", Password: long(9)}},
			"password 129":         {InvitationNamed, Registration{DisplayName: "M", Password: long(129)}},
			"password multibyte 9": {InvitationNamed, Registration{DisplayName: "M", Password: strings.Repeat("é", 9)}},
		} {
			s, repo := newInvService(t)
			repo.EXPECT().Peek(gomock.Any(), gomock.Any(), t0).Return(pending(c.kind, "stored@s.me"), nil)
			if _, err := s.Register(ctx, someToken, c.in); !errors.Is(err, ErrInvalidUser) {
				t.Errorf("%s: got %v", name, err)
			}
		}
	})
	t.Run("password lengths 10 and 128 are fine", func(t *testing.T) {
		for _, n := range []int{10, 128} {
			s, repo := newInvService(t)
			repo.EXPECT().Peek(gomock.Any(), gomock.Any(), t0).Return(pending(InvitationNamed, "stored@s.me"), nil)
			repo.EXPECT().Register(gomock.Any(), gomock.Any()).Return(User{ID: 1}, nil)
			if _, err := s.Register(ctx, someToken, Registration{DisplayName: "M", Password: strings.Repeat("p", n)}); err != nil {
				t.Errorf("%d: %v", n, err)
			}
		}
	})
	t.Run("a repository failure is returned and no session is", func(t *testing.T) {
		s, repo := newInvService(t)
		boom := errors.New("boom")
		repo.EXPECT().Peek(gomock.Any(), gomock.Any(), t0).Return(pending(InvitationNamed, "stored@s.me"), nil)
		repo.EXPECT().Register(gomock.Any(), gomock.Any()).Return(User{}, boom)
		if got, err := s.Register(ctx, someToken, Registration{DisplayName: "M", Password: goodPassword}); !errors.Is(err, boom) || got.Token != "" {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("the raw tokens never reach the audit entry", func(t *testing.T) {
		s, repo := newInvService(t)
		repo.EXPECT().Peek(gomock.Any(), gomock.Any(), t0).Return(pending(InvitationNamed, "stored@s.me"), nil)
		var dump string
		repo.EXPECT().Register(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in RegisterAccount) (User, error) {
			dump = fmt.Sprintf("%+v", in.Audit(1, 2))
			return User{ID: 1}, nil
		})
		got, err := s.Register(ctx, someToken, Registration{DisplayName: "M", Password: goodPassword})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(dump, someToken) || strings.Contains(dump, got.Token) || strings.Contains(dump, "stored@s.me") {
			t.Fatalf("audit leaks: %s", dump)
		}
	})
}

func TestInvitationStateAt(t *testing.T) {
	rev := t0.Add(-time.Hour)
	for _, c := range []struct {
		name    string
		expires time.Time
		revoked *time.Time
		uses    int
		max     int
		want    InvitationState
	}{
		{"pending", t0.Add(time.Second), nil, 0, 1, InvitationPending},
		{"partly used anonymous is pending", t0.Add(time.Second), nil, 2, 5, InvitationPending},
		{"expires_at == now is expired", t0, nil, 0, 1, InvitationExpired},
		{"used", t0.Add(time.Hour), nil, 5, 5, InvitationUsed},
		{"used beats expired", t0.Add(-time.Hour), nil, 1, 1, InvitationUsed},
		{"revoked beats used", t0.Add(time.Hour), &rev, 1, 1, InvitationRevoked},
		{"revoked beats expired", t0.Add(-time.Hour), &rev, 0, 1, InvitationRevoked},
	} {
		if got := InvitationStateAt(c.expires, c.revoked, c.uses, c.max, t0); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
