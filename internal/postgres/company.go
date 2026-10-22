package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// companyStore implements the core company repository; Store.Companies returns it.
type companyStore struct{ s *Store }

// Companies returns the company repository.
func (s *Store) Companies() companyStore { return companyStore{s} }

// querier is what a pool and a transaction share.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// The jsonb columns are wrapped in an object with a version, so the shape can change without a migration.
type progressJSON struct {
	V int            `json:"v"`
	P *core.Progress `json:"p"`
}
type injuriesJSON struct {
	V      int            `json:"v"`
	Counts map[string]int `json:"counts,omitempty"`
}
type unitDataJSON struct {
	V             int              `json:"v"`
	Bonus         map[string]int   `json:"bonus,omitempty"`
	Actions       []string         `json:"actions,omitempty"`
	BoughtActions []string         `json:"bought_actions,omitempty"`
	Spells        []core.PathSpell `json:"spells,omitempty"`
}
type companyPendingJSON struct {
	V       int                  `json:"v"`
	Recruit core.RecruitProgress `json:"recruit"`
}
type unitPendingJSON struct {
	V       int              `json:"v"`
	Pending core.UnitPending `json:"pending"`
}
type snapshotJSON struct {
	V    int              `json:"v"`
	Unit core.CompanyUnit `json:"unit"`
}

const unitColumns = `id, base, custom_name, role, kind, path, path_rules, level, xp, points, wanderer, creature, injured,
	sent_to_healers, progress, injuries, data, pending`

// load reads the whole state of a company in the campaign.
func (cs companyStore) load(ctx context.Context, q querier, campaignID, companyID int64, lock bool) (core.CompanyState, error) {
	var st core.CompanyState
	query := `SELECT c.id, c.name, c.faction, c.owner_id, u.display_name, c.influence, c.gold, c.rerolls, c.pack, c.unit_order, c.grave_order, c.pending
	            FROM companies c JOIN users u ON u.id = c.owner_id WHERE c.campaign_id = $1 AND c.id = $2`
	if lock {
		query += ` FOR UPDATE OF c`
	}
	co := &st.Company
	var companyPending []byte
	err := q.QueryRow(ctx, query, campaignID, companyID).
		Scan(&co.ID, &co.Name, &co.Faction, &co.OwnerID, &co.Owner, &st.Influence, &st.Gold, &st.Rerolls, &st.Pack, &st.Order, &st.Graveyard, &companyPending)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, core.ErrNotFound
	}
	if err != nil {
		return st, err
	}
	var cp companyPendingJSON
	if err := json.Unmarshal(companyPending, &cp); err != nil {
		return st, fmt.Errorf("company %d pending: %w", companyID, err)
	}
	st.Recruit = cp.Recruit
	if st.Aftermaths, err = pendingRefs(ctx, q, companyID, st.Company.Name); err != nil {
		return st, err
	}
	if st.OpenMatch, err = openMatchOf(ctx, q, companyID); err != nil {
		return st, err
	}
	if st.Perks, err = unusedPerks(ctx, q, companyID); err != nil {
		return st, err
	}
	st.Pack, st.Order, st.Graveyard = nilIfEmpty(st.Pack), nilIfEmpty(st.Order), nilIfEmpty(st.Graveyard)

	units, err := cs.loadUnits(ctx, q, []int64{companyID})
	if err != nil {
		return st, err
	}
	st.Units = units[companyID]

	fr, err := q.Query(ctx, `SELECT fate, fell_in, snapshot FROM fallen_units WHERE company_id = $1 ORDER BY position`, companyID)
	if err != nil {
		return st, err
	}
	defer fr.Close()
	for fr.Next() {
		var f core.FallenUnit
		var raw []byte
		if err := fr.Scan(&f.Fate, &f.Match, &raw); err != nil {
			return st, err
		}
		var s snapshotJSON
		if err := json.Unmarshal(raw, &s); err != nil {
			return st, fmt.Errorf("fallen snapshot: %w", err)
		}
		f.Unit = s.Unit
		st.Cemetery = append(st.Cemetery, f)
	}
	return st, fr.Err()
}

func nilIfEmpty[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}

// Read returns the company's state without locking it.
func (cs companyStore) Read(ctx context.Context, campaignID, companyID int64) (core.CompanyState, error) {
	tx, err := cs.s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}) // one snapshot for all the queries
	if err != nil {
		return core.CompanyState{}, err
	}
	defer tx.Rollback(ctx)
	return cs.load(ctx, tx, campaignID, companyID, false)
}

// loadUnits reads the units of the companies (with their equipment) in two queries, however many companies there are, in each company's
// roster order.
func (cs companyStore) loadUnits(ctx context.Context, q querier, ids []int64) (map[int64][]core.UnitState, error) {
	rows, err := q.Query(ctx, `SELECT company_id, `+unitColumns+` FROM company_units WHERE company_id = ANY($1) ORDER BY company_id, position`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []core.UnitState
	var owner []int64 // the company of all[i]
	for rows.Next() {
		var u core.UnitState
		var role, kind string
		var cid int64
		var progress, injuries, data, pending []byte
		if err := rows.Scan(&cid, &u.ID, &u.Base, &u.Name, &role, &kind, &u.Path, &u.PathRules, &u.Level, &u.XP, &u.Points, &u.Wanderer,
			&u.Creature, &u.Injured, &u.SentToHealers, &progress, &injuries, &data, &pending); err != nil {
			return nil, err
		}
		u.Role, u.Kind = core.UnitRole(role), core.UnitKind(kind)
		if progress != nil {
			var p progressJSON
			if err := json.Unmarshal(progress, &p); err != nil {
				return nil, fmt.Errorf("unit %d progress: %w", u.ID, err)
			}
			u.Progress = p.P
		}
		var inj injuriesJSON
		var d unitDataJSON
		if err := json.Unmarshal(injuries, &inj); err != nil {
			return nil, fmt.Errorf("unit %d injuries: %w", u.ID, err)
		}
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("unit %d data: %w", u.ID, err)
		}
		var up unitPendingJSON
		if err := json.Unmarshal(pending, &up); err != nil {
			return nil, fmt.Errorf("unit %d pending: %w", u.ID, err)
		}
		u.Injuries, u.Bonus, u.Actions, u.BoughtActions, u.Spells, u.Pending = inj.Counts, d.Bonus, d.Actions, d.BoughtActions, d.Spells, up.Pending
		all, owner = append(all, u), append(owner, cid)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	at := make(map[int64]int, len(all)) // unit id -> index in all
	for i := range all {
		at[all[i].ID] = i
	}

	eq, err := q.Query(ctx, `SELECT e.unit_id, e.origin, e.name FROM unit_equipment e JOIN company_units u ON u.id = e.unit_id
		WHERE u.company_id = ANY($1) ORDER BY e.unit_id, e.origin, e.position`, ids)
	if err != nil {
		return nil, err
	}
	defer eq.Close()
	for eq.Next() {
		var id int64
		var origin, name string
		if err := eq.Scan(&id, &origin, &name); err != nil {
			return nil, err
		}
		u := &all[at[id]]
		switch origin {
		case "gear":
			u.Gear = append(u.Gear, name)
		case "bought":
			u.Bought = append(u.Bought, name)
		case "dropped":
			u.Dropped = append(u.Dropped, name)
		}
	}
	if err := eq.Err(); err != nil {
		return nil, err
	}
	eq.Close()

	out := make(map[int64][]core.UnitState, len(ids))
	for i := range all {
		out[owner[i]] = append(out[owner[i]], all[i])
	}
	return out, nil
}

// ReadRoster reads the state of every company of the campaign in four queries: the companies' own figures and their units with
// equipment, enough to rate them. Pending aftermaths, the open match, perks and the cemetery are left out (Read has them).
func (cs companyStore) ReadRoster(ctx context.Context, campaignID int64) ([]core.CompanyState, error) {
	tx, err := cs.s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT c.id, c.name, c.faction, c.owner_id, u.display_name, c.influence, c.gold, c.rerolls, c.pack, c.unit_order, c.grave_order
	            FROM companies c JOIN users u ON u.id = c.owner_id WHERE c.campaign_id = $1 ORDER BY c.id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.CompanyState
	var ids []int64
	for rows.Next() {
		var st core.CompanyState
		co := &st.Company
		if err := rows.Scan(&co.ID, &co.Name, &co.Faction, &co.OwnerID, &co.Owner, &st.Influence, &st.Gold, &st.Rerolls, &st.Pack, &st.Order, &st.Graveyard); err != nil {
			return nil, err
		}
		st.Pack, st.Order, st.Graveyard = nilIfEmpty(st.Pack), nilIfEmpty(st.Order), nilIfEmpty(st.Graveyard)
		out, ids = append(out, st), append(ids, co.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	units, err := cs.loadUnits(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Units = units[out[i].Company.ID]
	}
	return out, nil
}

// Update locks the company row, runs fn on the state and stores the result; an error from fn rolls everything back.
func (cs companyStore) Update(ctx context.Context, campaignID, companyID int64, fn func(*core.CompanyState) error) error {
	tx, err := cs.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	st, err := cs.load(ctx, tx, campaignID, companyID, true)
	if err != nil {
		return err
	}
	if err := fn(&st); err != nil {
		return err
	}
	if err := cs.save(ctx, tx, st); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// save writes the state back: the company row, then units (UPDATE by id, INSERT new, DELETE removed) and the cemetery.
func (cs companyStore) save(ctx context.Context, q querier, st core.CompanyState) error {
	id := st.Company.ID
	pending, err := json.Marshal(companyPendingJSON{1, st.Recruit})
	if err != nil {
		return err
	}
	if _, err := q.Exec(ctx, `UPDATE companies SET name = $2, faction = $3, influence = $4, gold = $5, rerolls = $6, pack = $7,
		unit_order = $8, grave_order = $9, pending = $10 WHERE id = $1`,
		id, st.Company.Name, st.Company.Faction, st.Influence, st.Gold, st.Rerolls, orEmpty(st.Pack), orEmpty(st.Order), orEmpty(st.Graveyard), pending); err != nil {
		return err
	}

	if len(st.PerksUsed) > 0 { // a perk is used up in the transaction of the change that uses it, once
		tag, err := q.Exec(ctx, `UPDATE company_perks SET used_on = now() WHERE company_id = $1 AND id = ANY($2) AND used_on IS NULL`, id, st.PerksUsed)
		if err != nil {
			return err
		}
		if int(tag.RowsAffected()) != len(st.PerksUsed) {
			return core.Refuse("That perk has been used already.")
		}
	}

	keep := make([]int64, 0, len(st.Units))
	for _, u := range st.Units {
		if u.ID != 0 {
			keep = append(keep, u.ID)
		}
	}
	if _, err := q.Exec(ctx, `DELETE FROM company_units WHERE company_id = $1 AND NOT (id = ANY($2))`, id, keep); err != nil {
		return err
	}
	for pos, u := range st.Units {
		if err := cs.saveUnit(ctx, q, id, pos, u); err != nil {
			return err
		}
	}

	fallen := make([]int64, 0, len(st.Cemetery))
	for _, f := range st.Cemetery {
		fallen = append(fallen, f.Unit.ID)
	}
	if _, err := q.Exec(ctx, `DELETE FROM fallen_units WHERE company_id = $1 AND NOT (unit_id = ANY($2))`, id, fallen); err != nil {
		return err
	}
	for pos, f := range st.Cemetery {
		snap, err := json.Marshal(snapshotJSON{1, f.Unit})
		if err != nil {
			return err
		}
		if f.Unit.ID == 0 {
			return errors.New("a fallen unit needs the id it had on the roster")
		}
		if _, err := q.Exec(ctx, `INSERT INTO fallen_units (company_id, unit_id, position, fate, fell_in, snapshot)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (unit_id) DO UPDATE SET position = $3, fate = $4, fell_in = $5, snapshot = $6
			WHERE fallen_units.company_id = $1`, id, f.Unit.ID, pos, f.Fate, f.Match, snap); err != nil {
			return err
		}
	}
	return nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// saveUnit inserts the unit (id 0, which is then filled by the caller's copy only in the database) or updates it by id.
func (cs companyStore) saveUnit(ctx context.Context, q querier, companyID int64, pos int, u core.UnitState) error {
	var progress []byte
	if u.Progress != nil {
		b, err := json.Marshal(progressJSON{1, u.Progress})
		if err != nil {
			return err
		}
		progress = b
	}
	injuries, err := json.Marshal(injuriesJSON{1, u.Injuries})
	if err != nil {
		return err
	}
	data, err := json.Marshal(unitDataJSON{1, u.Bonus, u.Actions, u.BoughtActions, u.Spells})
	if err != nil {
		return err
	}
	pending, err := json.Marshal(unitPendingJSON{1, u.Pending})
	if err != nil {
		return err
	}
	args := []any{companyID, pos, u.Base, u.Name, string(u.Role), string(u.Kind), u.Path, u.PathRules, u.Level, u.XP, u.Points,
		u.Wanderer, u.Creature, u.Injured, u.SentToHealers, progress, injuries, data, pending}
	unitID := u.ID
	if unitID == 0 {
		err = q.QueryRow(ctx, `INSERT INTO company_units (company_id, position, base, custom_name, role, kind, path, path_rules, level, xp,
			points, wanderer, creature, injured, sent_to_healers, progress, injuries, data, pending)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING id`, args...).Scan(&unitID)
	} else {
		var tag pgconn.CommandTag
		tag, err = q.Exec(ctx, `UPDATE company_units SET position = $2, base = $3, custom_name = $4, role = $5, kind = $6, path = $7,
			path_rules = $8, level = $9, xp = $10, points = $11, wanderer = $12, creature = $13, injured = $14, sent_to_healers = $15,
			progress = $16, injuries = $17, data = $18, pending = $19 WHERE company_id = $1 AND id = $20`, append(args, unitID)...)
		if err == nil && tag.RowsAffected() == 0 {
			return fmt.Errorf("unit %d is not on the roster of company %d", unitID, companyID)
		}
		if err == nil {
			_, err = q.Exec(ctx, `DELETE FROM unit_equipment WHERE unit_id = $1`, unitID)
		}
	}
	if err != nil {
		return err
	}
	for origin, names := range map[string][]string{"gear": u.Gear, "bought": u.Bought, "dropped": u.Dropped} {
		for i, name := range names {
			if _, err := q.Exec(ctx, `INSERT INTO unit_equipment (unit_id, origin, position, name) VALUES ($1, $2, $3, $4)`, unitID, origin, i, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Create stores the company and its units for the owner, who must be a member of the campaign.
func (cs companyStore) Create(ctx context.Context, campaignID int64, owner core.User, c core.NewCompany) (core.CampaignCompany, error) {
	for _, u := range c.Units {
		if u.ID != 0 {
			return core.CampaignCompany{}, errors.Join(core.ErrInvalidCompany, errors.New("starting units get their ids from the repository"))
		}
	}
	tx, err := cs.s.pool.Begin(ctx)
	if err != nil {
		return core.CampaignCompany{}, err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO companies (campaign_id, owner_id, name, faction, influence, gold) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		campaignID, owner.ID, c.Name, c.Faction, c.Influence, c.Gold).Scan(&id)
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == "companies_campaign_owner_key":
		return core.CampaignCompany{}, core.ErrHasCompany
	case errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation:
		return core.CampaignCompany{}, core.ErrNotFound // not a member of that campaign
	case err != nil:
		return core.CampaignCompany{}, err
	}
	for pos, u := range slices.Clone(c.Units) {
		if err := cs.saveUnit(ctx, tx, id, pos, u); err != nil {
			return core.CampaignCompany{}, err
		}
	}
	out, err := scanCompany(tx.QueryRow(ctx, companyFigures+` WHERE c.id = $1`, id))
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

// unusedPerks lists the company's perks that are not used yet, oldest first.
func unusedPerks(ctx context.Context, q querier, companyID int64) ([]core.Perk, error) {
	rows, err := q.Query(ctx, `
		SELECT id, COALESCE(match_id, 0), COALESCE(unit_id, 0), effect, name, text, applies
		  FROM company_perks WHERE company_id = $1 AND used_on IS NULL ORDER BY id`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Perk
	for rows.Next() {
		var p core.Perk
		if err := rows.Scan(&p.ID, &p.MatchID, &p.UnitID, &p.Effect, &p.Name, &p.Text, &p.When); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
