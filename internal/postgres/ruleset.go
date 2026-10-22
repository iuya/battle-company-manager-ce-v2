package postgres

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// rulesetStore implements the core ruleset repository; Store.Rulesets returns it.
type rulesetStore struct{ s *Store }

// Rulesets returns the rules file repository.
func (s *Store) Rulesets() rulesetStore { return rulesetStore{s} }

const rulesetInfoColumns = `kind, hash, name, version, is_default`

func scanRulesetInfo(row pgx.Row) (core.RulesetInfo, error) {
	var r core.RulesetInfo
	var kind string
	err := row.Scan(&kind, &r.Hash, &r.Name, &r.Version, &r.Default)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.RulesetInfo{}, core.ErrNotFound
	}
	r.Kind = core.RulesetKind(kind)
	return r, err
}

// Insert stores the file unless one with the same hash exists. The Default flag is ignored: use SetDefaultRuleset.
func (rs rulesetStore) Insert(ctx context.Context, in core.StoredRuleset) error {
	var uploader *int64
	if in.UploadedBy != 0 {
		uploader = &in.UploadedBy
	}
	_, err := rs.s.pool.Exec(ctx,
		`INSERT INTO rulesets (hash, kind, name, version, size, content, uploaded_by) VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (hash) DO NOTHING`,
		in.Hash, string(in.Kind), in.Name, in.Version, len(in.Content), in.Content, uploader)
	return err
}

func (rs rulesetStore) Get(ctx context.Context, hash string) (core.StoredRuleset, error) {
	var r core.StoredRuleset
	var kind string
	var uploader *int64
	err := rs.s.pool.QueryRow(ctx,
		`SELECT `+rulesetInfoColumns+`, size, content, uploaded_by FROM rulesets WHERE hash = $1`, hash).
		Scan(&kind, &r.Hash, &r.Name, &r.Version, &r.Default, &r.Size, &r.Content, &uploader)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.StoredRuleset{}, core.ErrNotFound
	}
	r.Kind = core.RulesetKind(kind)
	if uploader != nil {
		r.UploadedBy = *uploader
	}
	return r, err
}

func (rs rulesetStore) Info(ctx context.Context, hash string) (core.RulesetInfo, error) {
	return scanRulesetInfo(rs.s.pool.QueryRow(ctx, `SELECT `+rulesetInfoColumns+` FROM rulesets WHERE hash = $1`, hash))
}

// SetDefault makes the file the default of its kind (which must be the given kind) and unmarks the previous one.
func (rs rulesetStore) SetDefault(ctx context.Context, kind core.RulesetKind, hash string) error {
	tx, err := rs.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var found string
	err = tx.QueryRow(ctx, `SELECT hash FROM rulesets WHERE hash = $1 AND kind = $2 FOR UPDATE`, hash, string(kind)).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE rulesets SET is_default = false WHERE kind = $1 AND is_default AND hash <> $2`, string(kind), hash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE rulesets SET is_default = true WHERE hash = $1`, hash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (rs rulesetStore) Default(ctx context.Context, kind core.RulesetKind) (core.RulesetInfo, error) {
	return scanRulesetInfo(rs.s.pool.QueryRow(ctx, `SELECT `+rulesetInfoColumns+` FROM rulesets WHERE kind = $1 AND is_default`, string(kind)))
}

const unusedByCampaigns = `NOT EXISTS (SELECT 1 FROM campaigns c WHERE c.vanilla_hash = r.hash OR c.bc_hash = r.hash OR c.glossary_hash = r.hash)`

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}

var kindOrder = map[core.RulesetKind]int{core.KindVanilla: 0, core.KindBattleCompanies: 1, core.KindGlossary: 2}

func (rs rulesetStore) ListUsage(ctx context.Context) ([]core.RulesetUsage, error) {
	rows, err := rs.s.pool.Query(ctx,
		`SELECT r.kind, r.hash, r.name, r.version, r.is_default, r.size, r.created_at, u.id, u.display_name
		   FROM rulesets r LEFT JOIN users u ON u.id = r.uploaded_by`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.RulesetUsage{}
	index := map[string]int{}
	for rows.Next() {
		var r core.RulesetUsage
		var kind string
		var uid *int64
		var uname *string
		if err := rows.Scan(&kind, &r.Hash, &r.Name, &r.Version, &r.Default, &r.Size, &r.CreatedAt, &uid, &uname); err != nil {
			return nil, err
		}
		r.Kind = core.RulesetKind(kind)
		if uid != nil {
			r.UploadedBy = &core.User{ID: *uid, DisplayName: *uname}
		}
		r.Campaigns = []core.CampaignRef{}
		index[r.Hash] = len(out)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	use, err := rs.s.pool.Query(ctx,
		`SELECT h, id, name FROM (
		     SELECT vanilla_hash AS h, id, name FROM campaigns
		     UNION SELECT bc_hash, id, name FROM campaigns
		     UNION SELECT glossary_hash, id, name FROM campaigns) x
		  ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer use.Close()
	for use.Next() {
		var hash string
		var ref core.CampaignRef
		if err := use.Scan(&hash, &ref.ID, &ref.Name); err != nil {
			return nil, err
		}
		if i, ok := index[hash]; ok {
			out[i].Campaigns = append(out[i].Campaigns, ref)
		}
	}
	if err := use.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b core.RulesetUsage) int {
		if d := kindOrder[a.Kind] - kindOrder[b.Kind]; d != 0 {
			return d
		}
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.Hash, b.Hash)
	})
	return out, nil
}

func (rs rulesetStore) Delete(ctx context.Context, hash string) error {
	tag, err := rs.s.pool.Exec(ctx, `DELETE FROM rulesets r WHERE r.hash = $1 AND NOT r.is_default AND `+unusedByCampaigns, hash)
	if isForeignKeyViolation(err) { // a campaign started using it after the check
		return core.Refuse("A campaign uses this file")
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var isDefault bool
	err = rs.s.pool.QueryRow(ctx, `SELECT is_default FROM rulesets WHERE hash = $1`, hash).Scan(&isDefault)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return core.ErrNotFound
	case err != nil:
		return err
	case isDefault:
		return core.Refuse("This is the default file of its kind; make another file the default first")
	}
	return core.Refuse("A campaign uses this file")
}

func (rs rulesetStore) DeleteUnused(ctx context.Context) (int, error) {
	tag, err := rs.s.pool.Exec(ctx, `DELETE FROM rulesets r WHERE NOT r.is_default AND `+unusedByCampaigns)
	if isForeignKeyViolation(err) {
		return 0, core.Refuse("A campaign started using one of the files; try again")
	}
	return int(tag.RowsAffected()), err
}
