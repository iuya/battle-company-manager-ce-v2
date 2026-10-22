package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// inviteStore implements the core invite repository; Store.Invites returns it.
type inviteStore struct{ s *Store }

// Invites returns the invite repository.
func (s *Store) Invites() inviteStore { return inviteStore{s} }

// Create stores the invite. ErrNotFound for an unknown campaign.
func (is inviteStore) Create(ctx context.Context, in core.NewInvite) error {
	var pgErr *pgconn.PgError
	_, err := is.s.pool.Exec(ctx, `
		INSERT INTO campaign_invites (token_hash, campaign_id, created_by, expires_at, max_uses) VALUES ($1, $2, $3, $4, $5)`,
		in.TokenHash, in.CampaignID, in.CreatedBy, in.ExpiresAt, in.MaxUses)
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return core.ErrNotFound
	}
	return err
}

// Redeem locks the invite row, so concurrent redemptions are serialised and cannot exceed max_uses. A user who is already a
// member does not use the invite up. The invite of a finished campaign is ErrNotFound; a paused campaign refuses new members (ErrCampaignPaused).
func (is inviteStore) Redeem(ctx context.Context, userID int64, tokenHash string, now time.Time) (int64, error) {
	tx, err := is.s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var campaignID int64
	var status string
	err = tx.QueryRow(ctx, `
		SELECT i.campaign_id, c.status FROM campaign_invites i JOIN campaigns c ON c.id = i.campaign_id
		 WHERE i.token_hash = $1 AND i.revoked_at IS NULL AND i.expires_at > $2 AND (i.max_uses = 0 OR i.uses < i.max_uses) AND c.status <> 'finished' FOR UPDATE OF i`, tokenHash, now).Scan(&campaignID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, core.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if status == string(core.CampaignPaused) { // nobody new joins a paused campaign; a member who opens the link just goes in
		var member bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM campaign_members WHERE campaign_id = $1 AND user_id = $2)`, campaignID, userID).Scan(&member); err != nil {
			return 0, err
		}
		if !member {
			return 0, core.ErrCampaignPaused
		}
	}
	tag, err := tx.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'player') ON CONFLICT DO NOTHING`, campaignID, userID)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx, `UPDATE campaign_invites SET uses = uses + 1 WHERE token_hash = $1`, tokenHash); err != nil {
			return 0, err
		}
	}
	return campaignID, tx.Commit(ctx)
}

// Lookup reads what Redeem would use, without locking or changing anything.
func (is inviteStore) Lookup(ctx context.Context, userID int64, tokenHash string, now time.Time) (core.InvitePreview, error) {
	var p core.InvitePreview
	err := is.s.pool.QueryRow(ctx, `
		SELECT c.id, c.name, EXISTS (SELECT 1 FROM campaign_members m WHERE m.campaign_id = c.id AND m.user_id = $3), c.status = 'paused'
		  FROM campaign_invites i JOIN campaigns c ON c.id = i.campaign_id
		 WHERE i.token_hash = $1 AND i.revoked_at IS NULL AND i.expires_at > $2 AND (i.max_uses = 0 OR i.uses < i.max_uses) AND c.status <> 'finished'`,
		tokenHash, now, userID).Scan(&p.CampaignID, &p.CampaignName, &p.Member, &p.Paused)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.InvitePreview{}, core.ErrNotFound
	}
	return p, err
}

// Revoke stops every working link of the campaign and returns how many there were.
func (is inviteStore) Revoke(ctx context.Context, campaignID int64) (int, error) {
	tag, err := is.s.pool.Exec(ctx, revokeSQL, campaignID)
	return int(tag.RowsAffected()), err
}

const revokeSQL = `UPDATE campaign_invites SET revoked_at = now()
	WHERE campaign_id = $1 AND revoked_at IS NULL AND expires_at > now() AND (max_uses = 0 OR uses < max_uses)`

// Replace revokes the campaign's working links and stores the new one in one transaction.
func (is inviteStore) Replace(ctx context.Context, in core.NewInvite) (int, error) {
	tx, err := is.s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, revokeSQL, in.CampaignID)
	if err != nil {
		return 0, err
	}
	var pgErr *pgconn.PgError
	_, err = tx.Exec(ctx, `
		INSERT INTO campaign_invites (token_hash, campaign_id, created_by, expires_at, max_uses) VALUES ($1, $2, $3, $4, $5)`,
		in.TokenHash, in.CampaignID, in.CreatedBy, in.ExpiresAt, in.MaxUses)
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return 0, core.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), tx.Commit(ctx)
}
