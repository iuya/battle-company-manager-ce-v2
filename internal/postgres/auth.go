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

// TEMPORARY (database-master replaces it): the audit entry is not written yet.
func (s *Store) SetUserPermissionsByID(ctx context.Context, userID int64, role core.Role, canCreateCampaigns bool, _ core.AuditEntry) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET role = $2, can_create_campaigns = $3 WHERE id = $1 AND closed_at IS NULL`,
		userID, string(role), canCreateCampaigns)
	if err == nil && tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return err
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
		`SELECT `+userColumns+`, u.created_at, u.closed_at,
		        (SELECT count(*) FROM campaign_members m WHERE m.user_id = u.id)
		   FROM users u
		  WHERE display_name ILIKE $1 OR email ILIKE $1
		  ORDER BY closed_at IS NOT NULL, lower(display_name), id
		  LIMIT $2 OFFSET $3`, pattern, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []core.AdminUser{}
	for rows.Next() {
		var u core.AdminUser
		var closedAt *time.Time
		if err := rows.Scan(append(userDest(&u.User), &u.CreatedAt, &closedAt, &u.Campaigns)...); err != nil {
			return nil, 0, err
		}
		u.Closed = closedAt != nil
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

// TEMPORARY (database-master replaces it): recoveryPending is always false until migration 00028 exists.
func (s *Store) UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (core.User, time.Time, bool, error) {
	var u core.User
	var expires time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name, u.role, u.can_create_campaigns, u.palette, u.mode, u.timezone, s.expires_at
		   FROM sessions s JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now).
		Scan(append(userDest(&u), &expires)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.User{}, time.Time{}, false, core.ErrNotFound
	}
	return u, expires, false, err
}

// InvalidateRecovery is TEMPORARY (database-master replaces it with migration 00028): there are no recovery links yet.
func (s *Store) InvalidateRecovery(ctx context.Context, userID int64, reason core.RecoveryReason, now time.Time) (int, error) {
	return 0, nil
}

// PurgeStaleTokens is TEMPORARY (database-master replaces it with migrations 00027 and 00028): there are no tokens yet.
func (s *Store) PurgeStaleTokens(ctx context.Context, before time.Time) error { return nil }

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
