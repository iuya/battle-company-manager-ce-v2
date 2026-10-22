package core

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestInvitesCreateStoresOnlyTheHash(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	campaigns, repo := NewMockcampaignRepository(ctrl), NewMockinviteRepository(ctrl)
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	s := NewInvites(campaigns, repo)
	s.now, s.random = func() time.Time { return now }, bytes.NewReader(bytes.Repeat([]byte{7}, 64))
	org := User{ID: 1}
	campaigns.EXPECT().Get(ctx, int64(1), int64(5)).Return(CampaignDetail{CampaignSummary: CampaignSummary{Status: CampaignActive}, OrganiserID: 1}, nil)
	var stored NewInvite
	repo.EXPECT().Create(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, in NewInvite) error { stored = in; return nil })
	inv, err := s.Invite(ctx, org, 5)
	if err != nil || inv.Token == "" || !inv.ExpiresAt.Equal(now.Add(7*24*time.Hour)) {
		t.Fatalf("Invite = %+v, %v", inv, err)
	}
	if stored.TokenHash != HashInviteToken(inv.Token) || stored.TokenHash == inv.Token || stored.CampaignID != 5 || stored.CreatedBy != 1 || !stored.ExpiresAt.Equal(inv.ExpiresAt) {
		t.Errorf("stored = %+v", stored)
	}
}

func TestInvitesRefusals(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	campaigns, repo := NewMockcampaignRepository(ctrl), NewMockinviteRepository(ctrl)
	s := NewInvites(campaigns, repo)
	campaigns.EXPECT().Get(ctx, int64(2), int64(5)).Return(CampaignDetail{OrganiserID: 1}, nil)
	campaigns.EXPECT().Get(ctx, int64(3), int64(5)).Return(CampaignDetail{}, ErrNotFound)
	campaigns.EXPECT().Get(ctx, int64(1), int64(6)).Return(CampaignDetail{CampaignSummary: CampaignSummary{Status: CampaignFinished}, OrganiserID: 1}, nil)
	if _, err := s.Invite(ctx, User{ID: 2}, 5); !errors.Is(err, ErrRefused) {
		t.Errorf("a player: %v", err)
	}
	if _, err := s.Invite(ctx, User{ID: 3}, 5); !errors.Is(err, ErrNotFound) {
		t.Errorf("a stranger: %v", err)
	}
	if _, err := s.Invite(ctx, User{ID: 1}, 6); !errors.Is(err, ErrRefused) {
		t.Errorf("finished: %v", err)
	}
	campaigns.EXPECT().Get(ctx, int64(1), int64(7)).Return(CampaignDetail{CampaignSummary: CampaignSummary{Status: CampaignPaused}, OrganiserID: 1}, nil)
	if _, err := s.Invite(ctx, User{ID: 1}, 7); !errors.Is(err, ErrCampaignPaused) {
		t.Errorf("paused: %v", err)
	}
}

func TestInvitesJoin(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	repo := NewMockinviteRepository(ctrl)
	s := NewInvites(NewMockcampaignRepository(ctrl), repo)
	now := time.Now()
	s.now = func() time.Time { return now }
	repo.EXPECT().Redeem(ctx, int64(4), HashInviteToken("tok"), now).Return(int64(5), nil)
	repo.EXPECT().Redeem(ctx, int64(4), HashInviteToken("old"), now).Return(int64(0), ErrNotFound)
	if id, err := s.Join(ctx, User{ID: 4}, "tok"); err != nil || id != 5 {
		t.Errorf("Join = %d, %v", id, err)
	}
	if _, err := s.Join(ctx, User{ID: 4}, "old"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired: %v", err)
	}
	if _, err := s.Join(ctx, User{ID: 4}, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty: %v", err)
	}
}

func TestInvitesPreview(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	repo := NewMockinviteRepository(ctrl)
	s := NewInvites(NewMockcampaignRepository(ctrl), repo)
	now := time.Now()
	s.now = func() time.Time { return now }
	repo.EXPECT().Lookup(ctx, int64(4), HashInviteToken("tok"), now).Return(InvitePreview{CampaignID: 5, CampaignName: "Rohan", Member: true}, nil)
	repo.EXPECT().Lookup(ctx, int64(4), HashInviteToken("old"), now).Return(InvitePreview{}, ErrNotFound)
	if p, err := s.Preview(ctx, User{ID: 4}, "tok"); err != nil || p.CampaignID != 5 || !p.Member {
		t.Errorf("Preview = %+v, %v", p, err)
	}
	if _, err := s.Preview(ctx, User{ID: 4}, "old"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired: %v", err)
	}
	if _, err := s.Preview(ctx, User{ID: 4}, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty: %v", err)
	}
}

func TestInvitesInvalidateAndRegenerate(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	campaigns, repo := NewMockcampaignRepository(ctrl), NewMockinviteRepository(ctrl)
	s := NewInvites(campaigns, repo)
	s.random = bytes.NewReader(bytes.Repeat([]byte{9}, 64))
	active := CampaignDetail{CampaignSummary: CampaignSummary{Status: CampaignActive}, OrganiserID: 1}
	campaigns.EXPECT().Get(ctx, int64(1), int64(5)).Return(active, nil).AnyTimes()
	campaigns.EXPECT().Get(ctx, int64(2), int64(5)).Return(active, nil).AnyTimes()
	campaigns.EXPECT().Get(ctx, int64(5), int64(5)).Return(active, nil).AnyTimes()
	campaigns.EXPECT().Get(ctx, int64(1), int64(6)).Return(CampaignDetail{CampaignSummary: CampaignSummary{Status: CampaignPaused}, OrganiserID: 1}, nil).AnyTimes()
	campaigns.EXPECT().Get(ctx, int64(1), int64(7)).Return(CampaignDetail{CampaignSummary: CampaignSummary{Status: CampaignFinished}, OrganiserID: 1}, nil).AnyTimes()

	repo.EXPECT().Revoke(ctx, int64(5)).Return(3, nil)
	if n, err := s.InvalidateInvites(ctx, User{ID: 1}, 5); err != nil || n != 3 {
		t.Errorf("organiser: %d %v", n, err)
	}
	if _, err := s.InvalidateInvites(ctx, User{ID: 2}, 5); !errors.Is(err, ErrRefused) {
		t.Errorf("a player: %v", err)
	}
	if _, err := s.InvalidateInvites(ctx, User{ID: 1}, 7); !errors.Is(err, ErrRefused) {
		t.Errorf("finished: %v", err)
	}
	var stored NewInvite
	repo.EXPECT().Replace(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, in NewInvite) (int, error) { stored = in; return 2, nil })
	inv, err := s.RegenerateInvite(ctx, User{ID: 5, Role: RoleAdmin}, 5)
	if err != nil || inv.Token == "" || stored.TokenHash != HashInviteToken(inv.Token) || stored.CampaignID != 5 {
		t.Errorf("admin regenerates: %+v %+v %v", inv, stored, err)
	}
	if _, err := s.RegenerateInvite(ctx, User{ID: 1}, 6); !errors.Is(err, ErrCampaignPaused) {
		t.Errorf("paused: %v", err)
	}
	if _, err := s.RegenerateInvite(ctx, User{ID: 2}, 5); !errors.Is(err, ErrRefused) {
		t.Errorf("a player: %v", err)
	}
}
