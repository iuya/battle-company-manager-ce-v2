package postgres

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// accountInvitationStore implements the core account invitation repository; Store.AccountInvitations returns it.
type accountInvitationStore struct{ s *Store }

// AccountInvitations returns the account invitation repository.
func (s *Store) AccountInvitations() accountInvitationStore { return accountInvitationStore{s} }

// invitationStateSQL derives the state of an invitation at the timestamp parameter $1: revoked, used, expired, pending, in that
// order; an invitation that expires at exactly now is expired.
const invitationStateSQL = `CASE WHEN i.revoked_at IS NOT NULL THEN 'revoked' WHEN i.uses >= i.max_uses THEN 'used'
	WHEN i.expires_at <= $1 THEN 'expired' ELSE 'pending' END`

const invitationColumns = `i.id, i.kind, coalesce(i.email, ''), i.can_create_campaigns, i.max_uses, i.uses, i.created_by, c.display_name,
	i.created_at, i.expires_at, i.revoked_at, i.revoked_by, coalesce(r.display_name, ''), ` + invitationStateSQL

const invitationFrom = ` FROM account_invitations i JOIN users c ON c.id = i.created_by LEFT JOIN users r ON r.id = i.revoked_by `

func invitationDest(a *core.AccountInvitation, state *string) []any {
	return []any{&a.ID, &a.Kind, &a.Email, &a.CanCreateCampaigns, &a.MaxUses, &a.Uses, &a.CreatedBy, &a.CreatedByName,
		&a.CreatedAt, &a.ExpiresAt, &a.RevokedAt, &a.RevokedBy, &a.RevokedByName, state}
}

// Create stores the invitation and its audit entry in one transaction and sets the entry's target to the new id.
func (st accountInvitationStore) Create(ctx context.Context, in core.StoredAccountInvitation, audit core.AuditEntry) (core.AccountInvitation, error) {
	tx, err := st.s.pool.Begin(ctx)
	if err != nil {
		return core.AccountInvitation{}, err
	}
	defer tx.Rollback(ctx)
	var email *string
	if in.Email != "" {
		email = &in.Email
	}
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO account_invitations (token_hash, kind, email, can_create_campaigns, max_uses, created_by, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		in.TokenHash, string(in.Kind), email, in.CanCreateCampaigns, in.MaxUses, in.CreatedBy, in.CreatedAt, in.ExpiresAt).Scan(&id)
	if err != nil {
		return core.AccountInvitation{}, err
	}
	audit.TargetID = id
	if err := insertAudit(ctx, tx, audit); err != nil {
		return core.AccountInvitation{}, err
	}
	inv, err := invitationByID(ctx, tx, id, in.CreatedAt)
	if err != nil {
		return core.AccountInvitation{}, err
	}
	return inv, tx.Commit(ctx)
}

func invitationByID(ctx context.Context, tx pgx.Tx, id int64, now time.Time) (core.AccountInvitation, error) {
	var a core.AccountInvitation
	var state string
	err := tx.QueryRow(ctx, `SELECT `+invitationColumns+invitationFrom+` WHERE i.id = $2`, now, id).Scan(invitationDest(&a, &state)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.AccountInvitation{}, core.ErrNotFound
	}
	a.State = core.InvitationState(state)
	return a, err
}

// List returns the invitations newest first with the accounts of each, the state computed from now, and the total.
func (st accountInvitationStore) List(ctx context.Context, filter core.InvitationFilter, now time.Time, limit, offset int) ([]core.AccountInvitation, int, error) {
	where := ``
	args := []any{now}
	if filter.State != "" {
		where = ` WHERE ` + invitationStateSQL + ` = $2`
		args = append(args, string(filter.State))
	}
	var total int
	countArgs := args
	if filter.State == "" {
		countArgs = nil // the state expression is the only user of $1
	}
	if err := st.s.pool.QueryRow(ctx, `SELECT count(*)`+invitationFrom+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	n := len(args)
	rows, err := st.s.pool.Query(ctx, `SELECT `+invitationColumns+invitationFrom+where+
		` ORDER BY i.created_at DESC, i.id DESC LIMIT $`+strconv.Itoa(n-1)+` OFFSET $`+strconv.Itoa(n), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []core.AccountInvitation{}
	at := map[int64]int{}
	var ids []int64
	for rows.Next() {
		var a core.AccountInvitation
		var state string
		if err := rows.Scan(invitationDest(&a, &state)...); err != nil {
			return nil, 0, err
		}
		a.State = core.InvitationState(state)
		at[a.ID] = len(out)
		ids = append(ids, a.ID)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	if len(ids) == 0 {
		return out, total, nil
	}
	uses, err := st.s.pool.Query(ctx, `
		SELECT x.invitation_id, x.user_id, u.display_name, x.used_at
		  FROM account_invitation_uses x JOIN users u ON u.id = x.user_id
		 WHERE x.invitation_id = ANY($1) ORDER BY x.used_at, x.user_id`, ids)
	if err != nil {
		return nil, 0, err
	}
	defer uses.Close()
	for uses.Next() {
		var inv int64
		var acc core.InvitationAccount
		if err := uses.Scan(&inv, &acc.UserID, &acc.DisplayName, &acc.UsedAt); err != nil {
			return nil, 0, err
		}
		out[at[inv]].Accounts = append(out[at[inv]].Accounts, acc)
	}
	return out, total, uses.Err()
}

// Revoke locks the invitation row, so a revoke racing a registration is serialised: whichever locks first wins, and the other
// sees the new state. ErrNotFound for an unknown id; a *NotPendingError when it is not pending at now.
func (st accountInvitationStore) Revoke(ctx context.Context, id, by int64, now time.Time, audit core.AuditEntry) (core.AccountInvitation, error) {
	tx, err := st.s.pool.Begin(ctx)
	if err != nil {
		return core.AccountInvitation{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	err = tx.QueryRow(ctx, `SELECT `+invitationStateSQL+` FROM account_invitations i WHERE i.id = $2 FOR UPDATE`, now, id).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.AccountInvitation{}, core.ErrNotFound
	}
	if err != nil {
		return core.AccountInvitation{}, err
	}
	if state != string(core.InvitationPending) {
		return core.AccountInvitation{}, &core.NotPendingError{State: core.InvitationState(state)}
	}
	if _, err := tx.Exec(ctx, `UPDATE account_invitations SET revoked_at = $2, revoked_by = $3 WHERE id = $1`, id, now, by); err != nil {
		return core.AccountInvitation{}, err
	}
	audit.TargetID = id
	if err := insertAudit(ctx, tx, audit); err != nil {
		return core.AccountInvitation{}, err
	}
	inv, err := invitationByID(ctx, tx, id, now)
	if err != nil {
		return core.AccountInvitation{}, err
	}
	return inv, tx.Commit(ctx)
}

// Peek looks an invitation up by token hash without locking.
func (st accountInvitationStore) Peek(ctx context.Context, tokenHash []byte, now time.Time) (core.InvitationPeek, error) {
	var p core.InvitationPeek
	var state string
	err := st.s.pool.QueryRow(ctx, `SELECT i.token_hash, i.kind, coalesce(i.email, ''), `+invitationStateSQL+`
		FROM account_invitations i WHERE i.token_hash = $2`, now, tokenHash).Scan(&p.TokenHash, &p.Kind, &p.Email, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.InvitationPeek{}, core.ErrNotFound
	}
	p.State = core.InvitationState(state)
	return p, err
}

// Register is the whole registration in one transaction. The invitation row is locked first (FOR UPDATE) and its state is
// re-checked under the lock at in.Now, so two simultaneous uses of a one-use invitation produce one account and max_uses
// accounts at most of an anonymous one; the second one waits for the lock and then finds the invitation used. The account,
// its session, the use row, the counter and the audit entry are written or none is: ErrEmailTaken rolls back and consumes
// nothing. ErrNotFound: no usable invitation.
func (st accountInvitationStore) Register(ctx context.Context, in core.RegisterAccount) (core.User, error) {
	tx, err := st.s.pool.Begin(ctx)
	if err != nil {
		return core.User{}, err
	}
	defer tx.Rollback(ctx)
	var invID int64
	var email *string
	var can bool
	var state string
	err = tx.QueryRow(ctx, `SELECT i.id, i.email, i.can_create_campaigns, `+invitationStateSQL+`
		FROM account_invitations i WHERE i.token_hash = $2 FOR UPDATE`, in.Now, in.TokenHash).Scan(&invID, &email, &can, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.User{}, core.ErrNotFound
	}
	if err != nil {
		return core.User{}, err
	}
	if state != string(core.InvitationPending) {
		return core.User{}, core.ErrNotFound
	}
	address := in.Email
	if email != nil {
		address = *email // a named invitation is tied to its address
	}
	var u core.User
	err = tx.QueryRow(ctx, `INSERT INTO users (email, display_name, password_hash, can_create_campaigns) VALUES ($1, $2, $3, $4) RETURNING `+userColumns,
		address, in.DisplayName, in.PasswordHash, can).Scan(userDest(&u)...)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return core.User{}, core.ErrEmailTaken
	}
	if err != nil {
		return core.User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, in.SessionHash, u.ID, in.SessionExpires); err != nil {
		return core.User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO account_invitation_uses (invitation_id, user_id, used_at) VALUES ($1, $2, $3)`, invID, u.ID, in.Now); err != nil {
		return core.User{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE account_invitations SET uses = uses + 1 WHERE id = $1`, invID); err != nil {
		return core.User{}, err
	}
	if err := insertAudit(ctx, tx, in.Audit(u.ID, invID)); err != nil {
		return core.User{}, err
	}
	return u, tx.Commit(ctx)
}

// EmailHasAccount reports whether an open account has that address (case-insensitive).
func (st accountInvitationStore) EmailHasAccount(ctx context.Context, email string) (bool, error) {
	var ok bool
	err := st.s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1) AND closed_at IS NULL)`, email).Scan(&ok)
	return ok, err
}
