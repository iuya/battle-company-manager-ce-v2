package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

const uniqueViolation = "23505"

const userColumns = `id, email, display_name, role, can_create_campaigns, palette, mode, timezone`

// userDest scans userColumns; palette and mode are NULL until the user chooses.
func userDest(u *core.User) []any {
	return []any{&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.CanCreateCampaigns, &nullText{&u.Appearance.Palette}, &nullText{&u.Appearance.Mode}, &nullText{&u.Timezone}}
}

// nullText scans a nullable text column into a string (NULL becomes "").
type nullText struct{ p *string }

func (n *nullText) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*n.p = ""
	case string:
		*n.p = v
	default:
		return fmt.Errorf("unexpected %T for a text column", src)
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, email, displayName, passwordHash string) (core.User, error) {
	var u core.User
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (email, display_name, password_hash) VALUES ($1, $2, $3) RETURNING `+userColumns+``,
		email, displayName, passwordHash).Scan(userDest(&u)...)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return core.User{}, core.ErrEmailTaken
	}
	return u, err
}

// SetUserPermissionsByID changes the role and the flag of an open account and writes the audit entry in the same transaction.
func (s *Store) SetUserPermissionsByID(ctx context.Context, userID int64, role core.Role, canCreateCampaigns bool, audit core.AuditEntry) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE users SET role = $2, can_create_campaigns = $3 WHERE id = $1 AND closed_at IS NULL`,
		userID, string(role), canCreateCampaigns)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UserByID(ctx context.Context, userID int64) (core.User, bool, error) {
	var u core.User
	var closedAt *time.Time
	err := s.pool.QueryRow(ctx, `SELECT `+userColumns+`, closed_at FROM users WHERE id = $1`, userID).
		Scan(append(userDest(&u), &closedAt)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.User{}, false, core.ErrNotFound
	}
	return u, closedAt != nil, err
}

// likePattern turns q into a LIKE pattern matching it as a plain substring.
func likePattern(q string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
}

func (s *Store) ListUsers(ctx context.Context, q string, limit, offset int) ([]core.AdminUser, int, error) {
	pattern := likePattern(q)
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE display_name ILIKE $1 OR email ILIKE $1`, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+strings.ReplaceAll(userColumns, "id, email, display_name, role, can_create_campaigns, palette, mode, timezone", "u.id, u.email, u.display_name, u.role, u.can_create_campaigns, u.palette, u.mode, u.timezone")+`, u.created_at, u.closed_at,
		        (SELECT count(*) FROM campaign_members m WHERE m.user_id = u.id),
		        l.id, l.created_at, l.expires_at, coalesce(lc.display_name, ''), l.used_at, l.revoked_at, coalesce(lr.display_name, ''),
		        l.invalidated_at, coalesce(l.invalidated_reason, '')
		   FROM users u
		   LEFT JOIN LATERAL (SELECT * FROM password_recovery_links x WHERE x.user_id = u.id ORDER BY x.created_at DESC, x.id DESC LIMIT 1) l ON true
		   LEFT JOIN users lc ON lc.id = l.created_by
		   LEFT JOIN users lr ON lr.id = l.revoked_by
		  WHERE u.display_name ILIKE $1 OR u.email ILIKE $1
		  ORDER BY u.closed_at IS NOT NULL, lower(u.display_name), u.id
		  LIMIT $2 OFFSET $3`, pattern, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []core.AdminUser{}
	for rows.Next() {
		var u core.AdminUser
		var closedAt *time.Time
		var link core.RecoveryStatus
		var linkID *int64
		var linkCreated, linkExpires *time.Time
		var reason string
		if err := rows.Scan(append(userDest(&u.User), &u.CreatedAt, &closedAt, &u.Campaigns, &linkID, &linkCreated, &linkExpires,
			&link.CreatedByName, &link.UsedAt, &link.RevokedAt, &link.RevokedByName, &link.InvalidatedAt, &reason)...); err != nil {
			return nil, 0, err
		}
		u.Closed = closedAt != nil
		if linkID != nil { // raw timestamps: the core derives the state from its clock
			link.ID, link.CreatedAt, link.ExpiresAt, link.Reason = *linkID, *linkCreated, *linkExpires, core.RecoveryReason(reason)
			u.Recovery = &link
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	if len(out) == 0 {
		return out, total, nil
	}
	ids := make([]int64, len(out))
	at := map[int64]int{}
	for i, u := range out {
		ids[i], at[u.ID] = u.ID, i
	}
	org, err := s.pool.Query(ctx, `
		SELECT o.user_id, g.id, g.name, g.status, (SELECT count(*) FROM campaign_members x WHERE x.campaign_id = g.id)::int
		  FROM campaign_members o JOIN campaigns g ON g.id = o.campaign_id
		 WHERE o.role = 'organiser' AND o.user_id = ANY($1)
		 ORDER BY g.created_at DESC, g.id DESC`, ids)
	if err != nil {
		return nil, 0, err
	}
	defer org.Close()
	for org.Next() {
		var uid int64
		var c core.OrganisedCampaign
		var status string
		if err := org.Scan(&uid, &c.ID, &c.Name, &status, &c.Players); err != nil {
			return nil, 0, err
		}
		c.Status = core.CampaignStatus(status)
		out[at[uid]].Organises = append(out[at[uid]].Organises, c)
	}
	return out, total, org.Err()
}

func (s *Store) UserByEmail(ctx context.Context, email string) (core.Credentials, error) {
	var c core.Credentials
	err := s.pool.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM users WHERE lower(email) = lower($1)`, email).
		Scan(append(userDest(&c.User), &c.PasswordHash)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Credentials{}, core.ErrNotFound
	}
	return c, err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, userID int64, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, userID, expiresAt)
	return err
}

// UserBySession resolves a session. recoveryPending is part of the same query (an EXISTS on the partial unique index of the
// pending links), so it costs no extra round trip.
func (s *Store) UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (core.User, time.Time, bool, error) {
	var u core.User
	var expires time.Time
	var pending bool
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name, u.role, u.can_create_campaigns, u.palette, u.mode, u.timezone, s.expires_at,
		        EXISTS (SELECT 1 FROM password_recovery_links r
		                 WHERE r.user_id = u.id AND r.used_at IS NULL AND r.revoked_at IS NULL AND r.invalidated_at IS NULL AND r.expires_at > $2)
		   FROM sessions s JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now).
		Scan(append(userDest(&u), &expires, &pending)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.User{}, time.Time{}, false, core.ErrNotFound
	}
	return u, expires, pending, err
}

// InvalidateRecovery marks the user's pending link (if any) invalidated for reason and, in the same statement, writes the
// recovery.invalidated entry (source system); n is the number of links changed, normally 0. A racing use of the link
// locks the row first or finds it invalidated, never both.
func (s *Store) InvalidateRecovery(ctx context.Context, userID int64, reason core.RecoveryReason, now time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		WITH inv AS (
		    UPDATE password_recovery_links SET invalidated_at = $3, invalidated_reason = $2
		     WHERE user_id = $1 AND `+unfinished+` AND expires_at > $3 RETURNING id)
		INSERT INTO audit_events (at, source, action, subject_user_id, target_kind, target_id, detail)
		SELECT $3, 'system', 'recovery.invalidated', $1, 'recovery_link', id, jsonb_build_object('reason', $2::text) FROM inv`,
		userID, string(reason), now)
	return int(tag.RowsAffected()), err
}

// PurgeStaleTokens deletes the invitations that were never used and ended (revoked or expired) before the cutoff, and the
// recovery links that ended (used, revoked, invalidated or expired) before it. Invitations that made an account and the audit
// trail are kept.
func (s *Store) PurgeStaleTokens(ctx context.Context, before time.Time) error {
	if _, err := s.pool.Exec(ctx, `
		DELETE FROM account_invitations WHERE uses = 0 AND least(coalesce(revoked_at, expires_at), expires_at) < $1`, before); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		DELETE FROM password_recovery_links WHERE coalesce(used_at, revoked_at, invalidated_at, expires_at) < $1`, before)
	return err
}

func (s *Store) ExtendSession(ctx context.Context, tokenHash []byte, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET expires_at = $2 WHERE token_hash = $1`, tokenHash, expiresAt)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now)
	return err
}

func (s *Store) CredentialsByID(ctx context.Context, userID int64) (core.Credentials, error) {
	var c core.Credentials
	err := s.pool.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM users WHERE id = $1 AND closed_at IS NULL`, userID).
		Scan(append(userDest(&c.User), &c.PasswordHash)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Credentials{}, core.ErrNotFound
	}
	return c, err
}

func (s *Store) UpdateDisplayName(ctx context.Context, userID int64, displayName string) (core.User, error) {
	var u core.User
	err := s.pool.QueryRow(ctx,
		`UPDATE users SET display_name = $2 WHERE id = $1 AND closed_at IS NULL RETURNING `+userColumns, userID, displayName).
		Scan(userDest(&u)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.User{}, core.ErrNotFound
	}
	return u, err
}

func (s *Store) UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1 AND closed_at IS NULL`, userID, passwordHash)
	if err == nil && tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return err
}

func (s *Store) DeleteSessionsExcept(ctx context.Context, userID int64, keep []byte) (int, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND ($2::bytea IS NULL OR token_hash <> $2)`, userID, keep)
	return int(tag.RowsAffected()), err
}

func (s *Store) CloseUser(ctx context.Context, userID int64, displayName, email string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx,
		`UPDATE users SET display_name = $2, email = $3, password_hash = '', palette = NULL, mode = NULL, closed_at = $4
		  WHERE id = $1 AND closed_at IS NULL`, userID, displayName, email, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
		return err
	}
	// A closed account has no password to recover: its pending link ends as 'closed' (the users row is locked by the UPDATE above).
	if _, err := tx.Exec(ctx, `
		WITH inv AS (
		    UPDATE password_recovery_links SET invalidated_at = $2, invalidated_reason = 'closed'
		     WHERE user_id = $1 AND `+unfinished+` RETURNING id)
		INSERT INTO audit_events (at, source, action, subject_user_id, target_kind, target_id, detail)
		SELECT $2, 'system', 'recovery.invalidated', $1, 'recovery_link', id, jsonb_build_object('reason', 'closed'::text) FROM inv`,
		userID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) OrganisesUnfinishedCampaign(ctx context.Context, userID int64) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM campaign_members m JOIN campaigns c ON c.id = m.campaign_id
		                 WHERE m.user_id = $1 AND m.role = 'organiser' AND c.status <> 'finished')`, userID).Scan(&ok)
	return ok, err
}

func (s *Store) OtherSuperusers(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE role = 'superuser' AND closed_at IS NULL AND id <> $1`, userID).Scan(&n)
	return n, err
}

func (s *Store) SetAppearance(ctx context.Context, userID int64, palette, mode string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET palette = $2, mode = $3 WHERE id = $1 AND closed_at IS NULL`, userID, palette, mode)
	if err == nil && tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return err
}

func (s *Store) SetTimezone(ctx context.Context, userID int64, zone string) error {
	var z *string
	if zone != "" {
		z = &zone
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET timezone = $2 WHERE id = $1 AND closed_at IS NULL`, userID, z)
	if err == nil && tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return err
}
