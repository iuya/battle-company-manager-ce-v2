package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// recoveryStore implements the core recovery repository; Store.Recovery returns it.
//
// Lock order, everywhere: the users row first, then the link row. A user has at most one unfinished link (partial unique
// index), so everything that finishes or replaces a link serialises on the user's row or on that one link row.
type recoveryStore struct{ s *Store }

// Recovery returns the password recovery link repository.
func (s *Store) Recovery() recoveryStore { return recoveryStore{s} }

// unfinished is the condition of a link that has not been used, revoked or invalidated (expired or not).
const unfinished = `used_at IS NULL AND revoked_at IS NULL AND invalidated_at IS NULL`

// lockUser takes the user's row lock (FOR NO KEY UPDATE: it does not block the foreign-key checks of other tables) and says
// whether the account is closed. ErrNotFound for an unknown user.
func lockUser(ctx context.Context, tx pgx.Tx, userID int64) (closed bool, err error) {
	var closedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT closed_at FROM users WHERE id = $1 FOR NO KEY UPDATE`, userID).Scan(&closedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, core.ErrNotFound
	}
	return closedAt != nil, err
}

// Create makes the link in one transaction: lock the user, mark the unfinished links of the user (live or expired but unmarked)
// invalidated 'replaced' with one recovery.invalidated entry each, insert the new link and its recovery.created entry.
func (r recoveryStore) Create(ctx context.Context, in core.StoredRecovery, audit core.AuditEntry) (core.RecoveryLink, error) {
	tx, err := r.s.pool.Begin(ctx)
	if err != nil {
		return core.RecoveryLink{}, err
	}
	defer tx.Rollback(ctx)
	closed, err := lockUser(ctx, tx, in.UserID)
	if err != nil {
		return core.RecoveryLink{}, err
	}
	if closed {
		return core.RecoveryLink{}, core.ErrAccountClosed
	}
	rows, err := tx.Query(ctx, `
		UPDATE password_recovery_links SET invalidated_at = $2, invalidated_reason = 'replaced'
		 WHERE user_id = $1 AND `+unfinished+` RETURNING id, expires_at > $2`, in.UserID, in.CreatedAt)
	if err != nil {
		return core.RecoveryLink{}, err
	}
	var replacedIDs []int64
	replaced := false
	for rows.Next() {
		var id int64
		var live bool
		if err := rows.Scan(&id, &live); err != nil {
			return core.RecoveryLink{}, err
		}
		replacedIDs = append(replacedIDs, id)
		replaced = replaced || live
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return core.RecoveryLink{}, err
	}
	for _, id := range replacedIDs {
		if err := insertAudit(ctx, tx, core.RecoveryInvalidatedAudit(in.CreatedAt, in.UserID, id, core.RecoveryReplaced)); err != nil {
			return core.RecoveryLink{}, err
		}
	}
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO password_recovery_links (user_id, token_hash, created_by, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		in.UserID, in.TokenHash, in.CreatedBy, in.CreatedAt, in.ExpiresAt).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == "password_recovery_pending_key" {
		return core.RecoveryLink{}, core.ErrConflict
	}
	if err != nil {
		return core.RecoveryLink{}, err
	}
	audit.TargetID = id
	if err := insertAudit(ctx, tx, audit.WithReplaced(replaced)); err != nil {
		return core.RecoveryLink{}, err
	}
	return core.RecoveryLink{ID: id, UserID: in.UserID, ExpiresAt: in.ExpiresAt}, tx.Commit(ctx)
}

// Revoke revokes the user's pending link (unexpired at now). ErrNotFound when there is none.
func (r recoveryStore) Revoke(ctx context.Context, userID, by int64, now time.Time, audit core.AuditEntry) error {
	tx, err := r.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := lockUser(ctx, tx, userID); err != nil {
		return err
	}
	var id int64
	err = tx.QueryRow(ctx, `
		UPDATE password_recovery_links SET revoked_at = $2, revoked_by = $3
		 WHERE user_id = $1 AND `+unfinished+` AND expires_at > $2 RETURNING id`, userID, now, by).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	if err != nil {
		return err
	}
	audit.TargetID = id
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// recoveryStateSQL derives a link's state at the timestamp parameter $1.
const recoveryStateSQL = `CASE WHEN l.used_at IS NOT NULL THEN 'used' WHEN l.revoked_at IS NOT NULL THEN 'revoked'
	WHEN l.invalidated_at IS NOT NULL THEN 'invalidated' WHEN l.expires_at <= $1 THEN 'expired' ELSE 'pending' END`

// Peek looks a link up by token hash without locking.
func (r recoveryStore) Peek(ctx context.Context, tokenHash []byte, now time.Time) (core.RecoveryPeek, error) {
	var p core.RecoveryPeek
	var state string
	err := r.s.pool.QueryRow(ctx, `SELECT l.token_hash, l.user_id, `+recoveryStateSQL+`
		FROM password_recovery_links l WHERE l.token_hash = $2`, now, tokenHash).Scan(&p.TokenHash, &p.UserID, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.RecoveryPeek{}, core.ErrNotFound
	}
	p.State = core.RecoveryState(state)
	return p, err
}

// Use redeems a link in one transaction. The user's row is locked first, then the link's, and the link is re-checked under
// the lock at now; so of two simultaneous uses exactly one wins, and a use racing an invalidation, a revoke or a closing sees
// either the finished link (ErrNotFound) or wins before them. The password is set, every session of the user is deleted and the
// audit entry written in the same transaction. ErrNotFound: no usable link.
func (r recoveryStore) Use(ctx context.Context, tokenHash []byte, passwordHash string, now time.Time, audit func(userID, linkID int64) core.AuditEntry) (int64, error) {
	tx, err := r.s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	// The user id is read unlocked (it never changes for a link); everything else is read again under the locks.
	var userID int64
	err = tx.QueryRow(ctx, `SELECT user_id FROM password_recovery_links WHERE token_hash = $1`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, core.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	closed, err := lockUser(ctx, tx, userID)
	if err != nil {
		return 0, err
	}
	var linkID int64
	var state string
	err = tx.QueryRow(ctx, `SELECT l.id, `+recoveryStateSQL+` FROM password_recovery_links l WHERE l.token_hash = $2 FOR UPDATE`, now, tokenHash).Scan(&linkID, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, core.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if closed || state != string(core.RecoveryPending) {
		return 0, core.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE password_recovery_links SET used_at = $2 WHERE id = $1`, linkID, now); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, passwordHash); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
		return 0, err
	}
	if err := insertAudit(ctx, tx, audit(userID, linkID)); err != nil {
		return 0, err
	}
	return userID, tx.Commit(ctx)
}
