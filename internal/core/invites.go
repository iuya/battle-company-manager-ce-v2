package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"time"
)

//go:generate go tool mockgen -typed -source=invites.go -destination=mock_invites_test.go -package=core

// Defaults of an invite link (REVIEW: bead 7v5.15): it expires after a week and can be used any number of times.
const (
	InviteLifetime = 7 * 24 * time.Hour
	InviteMaxUses  = 0 // 0: unlimited
)

// Invite is a freshly made invite link. Token is the secret to put in the link; only its hash is stored, so it cannot be
// shown again.
type Invite struct {
	Token     string
	ExpiresAt time.Time
	MaxUses   int // 0: unlimited
}

// NewInvite is what the repository stores: the SHA-256 (hex) of the token, never the token.
type NewInvite struct {
	CampaignID int64
	TokenHash  string
	ExpiresAt  time.Time
	MaxUses    int
	CreatedBy  int64
}

// inviteRepository keeps the invite links of campaigns (table campaign_invites).
type inviteRepository interface {
	// Create stores the invite.
	Create(ctx context.Context, in NewInvite) error
	// Redeem finds the invite with that token hash that has not expired at `now` and is not used up, makes the user a member
	// of its campaign (a no-op for a member, which does not use up the invite either) and returns the campaign's id, all in one
	// transaction that locks the invite so that concurrent redemptions cannot exceed its uses. An unknown, expired or used-up
	// invite is ErrNotFound; a visitor who is not a member of a paused campaign gets ErrCampaignPaused.
	Redeem(ctx context.Context, userID int64, tokenHash string, now time.Time) (campaignID int64, err error)
	// Revoke makes every working link of the campaign stop working and returns how many it was.
	Revoke(ctx context.Context, campaignID int64) (int, error)
	// Replace revokes the campaign's links and stores the new one in one transaction; it returns how many links it revoked.
	Replace(ctx context.Context, in NewInvite) (revoked int, err error)
	// Lookup is Redeem without changing anything: the campaign the invite leads to and whether the user is already in it.
	// ErrNotFound under the same conditions as Redeem.
	Lookup(ctx context.Context, userID int64, tokenHash string, now time.Time) (InvitePreview, error)
}

// InvitePreview is what opening an invite link shows before the visitor confirms.
type InvitePreview struct {
	CampaignID   int64
	CampaignName string
	Member       bool // the visitor is already a member
	Paused       bool // the campaign is paused: nobody joins until the organiser resumes it
}

// Invites creates and redeems invite links to campaigns.
type Invites struct {
	campaigns campaignRepository
	invites   inviteRepository
	now       func() time.Time
	random    io.Reader
}

// NewInvites builds the service.
func NewInvites(campaigns campaignRepository, invites inviteRepository) *Invites {
	return &Invites{campaigns: campaigns, invites: invites, now: time.Now, random: rand.Reader}
}

// HashInviteToken is what is stored of a token.
func HashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Invite makes a link for the campaign. Only its organiser and administrators may (ErrRefused), and not once it is finished;
// ErrNotFound when the user is not in the campaign.
func (s *Invites) Invite(ctx context.Context, user User, campaignID int64) (Invite, error) {
	camp, err := s.campaigns.Get(ctx, user.ID, campaignID)
	if err != nil {
		return Invite{}, err
	}
	switch {
	case !camp.CanManage(user):
		return Invite{}, Refuse("Only the organiser of the campaign can invite players.")
	case camp.Status == CampaignPaused:
		return Invite{}, ErrCampaignPaused
	case camp.Status == CampaignFinished:
		return Invite{}, Refuse("The campaign is finished.")
	}
	raw := make([]byte, 32)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return Invite{}, err
	}
	inv := Invite{Token: base64.RawURLEncoding.EncodeToString(raw), ExpiresAt: s.now().Add(InviteLifetime), MaxUses: InviteMaxUses}
	err = s.invites.Create(ctx, NewInvite{CampaignID: campaignID, TokenHash: HashInviteToken(inv.Token), ExpiresAt: inv.ExpiresAt, MaxUses: inv.MaxUses, CreatedBy: user.ID})
	return inv, err
}

// Join makes the user a member of the campaign the token invites to and returns its id. An unknown, expired or used-up token is
// ErrNotFound; joining twice is a no-op.
func (s *Invites) Join(ctx context.Context, user User, token string) (int64, error) {
	if token == "" {
		return 0, ErrNotFound
	}
	return s.invites.Redeem(ctx, user.ID, HashInviteToken(token), s.now())
}

// Preview says where a token leads, without joining. An unknown, expired or used-up token is ErrNotFound.
func (s *Invites) Preview(ctx context.Context, user User, token string) (InvitePreview, error) {
	if token == "" {
		return InvitePreview{}, ErrNotFound
	}
	return s.invites.Lookup(ctx, user.ID, HashInviteToken(token), s.now())
}

// InvalidateInvites makes every invitation link of the campaign stop working (players who joined stay) and returns how many there
// were. The organiser and administrators; not once the campaign is finished (ending it already revoked them).
func (s *Invites) InvalidateInvites(ctx context.Context, actor User, campaignID int64) (int, error) {
	camp, err := s.campaigns.Get(ctx, actor.ID, campaignID)
	if err != nil {
		return 0, err
	}
	switch {
	case !camp.CanManage(actor):
		return 0, Refuse("Only the organiser of the campaign can invalidate invitations.")
	case camp.Status == CampaignFinished:
		return 0, Refuse("The campaign is finished.")
	}
	return s.invites.Revoke(ctx, campaignID)
}

// RegenerateInvite revokes the campaign's links and makes a new one, in one transaction. Like Invite it is refused for a paused or
// finished campaign.
func (s *Invites) RegenerateInvite(ctx context.Context, actor User, campaignID int64) (Invite, error) {
	camp, err := s.campaigns.Get(ctx, actor.ID, campaignID)
	if err != nil {
		return Invite{}, err
	}
	switch {
	case !camp.CanManage(actor):
		return Invite{}, Refuse("Only the organiser of the campaign can invite players.")
	case camp.Status == CampaignPaused:
		return Invite{}, ErrCampaignPaused
	case camp.Status == CampaignFinished:
		return Invite{}, Refuse("The campaign is finished.")
	}
	raw := make([]byte, 32)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return Invite{}, err
	}
	inv := Invite{Token: base64.RawURLEncoding.EncodeToString(raw), ExpiresAt: s.now().Add(InviteLifetime), MaxUses: InviteMaxUses}
	_, err = s.invites.Replace(ctx, NewInvite{CampaignID: campaignID, TokenHash: HashInviteToken(inv.Token), ExpiresAt: inv.ExpiresAt, MaxUses: inv.MaxUses, CreatedBy: actor.ID})
	return inv, err
}
