package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// matchStore implements the core match repository; Store.Matches returns it.
type matchStore struct{ s *Store }

// Matches returns the match repository.
func (s *Store) Matches() matchStore { return matchStore{s} }

const matchColumns = `m.id, m.number, m.played_on, m.ended, m.ended_on, m.revision, m.decided, m.winner, m.note,
	m.scenario_kind, m.scenario_name, m.scenario_description, m.scenario_victory, m.practice`

// matchJSON are the columns of a match that come from other tables, as JSON built by the match's own statement (one round trip for
// the whole match instead of one per table). Times are microseconds since the epoch.
const matchJSON = `
	(SELECT COALESCE(jsonb_agg(jsonb_build_array(s.name, s.victory_points, coalesce(s.role, '')) ORDER BY s.position), '[]')
	   FROM match_sides s WHERE s.match_id = m.id),
	(SELECT COALESCE(jsonb_agg(jsonb_build_array(mc.position, mc.fled, c.id, c.name, c.faction, c.owner_id, u.display_name,
	                                             (SELECT count(*) FROM company_units cu WHERE cu.company_id = c.id)) ORDER BY mc.position, mc.slot), '[]')
	   FROM match_side_companies mc JOIN companies c ON c.id = mc.company_id JOIN users u ON u.id = c.owner_id WHERE mc.match_id = m.id),
	(SELECT jsonb_build_array(p.proposed_by, pu.display_name, (extract(epoch FROM p.proposed_on) * 1000000)::bigint, p.notice)
	   FROM match_end_proposal p JOIN users pu ON pu.id = p.proposed_by WHERE p.match_id = m.id),
	(SELECT COALESCE(jsonb_agg(jsonb_build_array(c.company_id, u.display_name, (extract(epoch FROM c.confirmed_on) * 1000000)::bigint, c.override)), '[]')
	   FROM match_end_confirmation c JOIN users u ON u.id = c.confirmed_by WHERE c.match_id = m.id),
	(SELECT COALESCE(jsonb_agg(jsonb_build_array(o.company_id, o.key, o.times) ORDER BY o.company_id, o.key), '[]')
	   FROM match_objectives o WHERE o.match_id = m.id)`

// load reads matches (newest first) with their sides, companies, result and scenario. where is a condition on m. One statement
// (two with lock: the rows are locked first, so what is read after is what the lock holder left).
func (ms matchStore) load(ctx context.Context, q querier, where string, lock bool, args ...any) ([]core.Match, error) {
	if lock {
		rows, err := q.Query(ctx, `SELECT m.id FROM matches m WHERE `+where+` ORDER BY m.number DESC FOR UPDATE OF m`, args...)
		if err != nil {
			return nil, err
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	rows, err := q.Query(ctx, `SELECT `+matchColumns+`, `+matchJSON+` FROM matches m WHERE `+where+` ORDER BY m.number DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Match
	for rows.Next() {
		var m core.Match
		var kind *string
		var sc core.MatchScenario
		var sides, companies, proposal, confirmations, objectives []byte
		if err := rows.Scan(&m.ID, &m.Number, &m.PlayedOn, &m.Ended, &m.EndedOn, &m.Revision, &m.Result.Decided, &m.Result.Winner,
			&m.Result.Note, &kind, &sc.Name, &sc.Description, &sc.Victory, &m.Practice, &sides, &companies, &proposal, &confirmations, &objectives); err != nil {
			return nil, err
		}
		m.Winner = m.Result.Winner
		if kind != nil {
			sc.Kind = core.ScenarioKind(*kind)
			m.Scenario = &sc
		}
		if err := fillMatch(&m, sides, companies, proposal, confirmations, objectives); err != nil {
			return nil, fmt.Errorf("match %d: %w", m.ID, err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// fillMatch decodes matchJSON into the match.
func fillMatch(m *core.Match, sidesJSON, companiesJSON, proposalJSON, confirmationsJSON, objectivesJSON []byte) error {
	// Sides, in order.
	var sides [][3]json.RawMessage
	if err := json.Unmarshal(sidesJSON, &sides); err != nil {
		return err
	}
	for _, r := range sides {
		var side core.MatchSide
		var vp *int
		if err := unmarshalAll(r[:], &side.Name, &vp, &side.Role); err != nil {
			return err
		}
		m.Sides = append(m.Sides, side)
		m.Result.Sides = append(m.Result.Sides, core.SideResult{VictoryPoints: vp})
	}

	// Companies of the sides, with what was reported for them.
	var companies [][8]json.RawMessage
	if err := json.Unmarshal(companiesJSON, &companies); err != nil {
		return err
	}
	for _, r := range companies {
		var pos int
		var fled bool
		var c core.CampaignCompany
		if err := unmarshalAll(r[:], &pos, &fled, &c.ID, &c.Name, &c.Faction, &c.OwnerID, &c.Owner, &c.Units); err != nil {
			return err
		}
		m.Sides[pos].Companies = append(m.Sides[pos].Companies, c)
		if fled {
			m.Result.Companies = append(m.Result.Companies, core.CompanyResult{CompanyID: c.ID, Fled: true})
		}
	}

	// The pending end, with one entry per company of the match (a company without a confirmation row is waiting).
	if proposalJSON != nil {
		var r [4]json.RawMessage
		if err := json.Unmarshal(proposalJSON, &r); err != nil {
			return err
		}
		var p core.EndProposal
		var on int64
		if err := unmarshalAll(r[:], &p.ProposedByID, &p.ProposedBy, &on, &p.Notice); err != nil {
			return err
		}
		p.ProposedOn = time.UnixMicro(on)
		for _, side := range m.Sides {
			for _, c := range side.Companies {
				p.Companies = append(p.Companies, core.EndConfirmation{CompanyID: c.ID, OwnerID: c.OwnerID, CompanyName: c.Name})
			}
		}
		var confirmations [][4]json.RawMessage
		if err := json.Unmarshal(confirmationsJSON, &confirmations); err != nil {
			return err
		}
		for _, r := range confirmations {
			var company, on int64
			var by string
			var override bool
			if err := unmarshalAll(r[:], &company, &by, &on, &override); err != nil {
				return err
			}
			for i := range p.Companies {
				if p.Companies[i].CompanyID == company {
					p.Companies[i].Confirmed, p.Companies[i].By, p.Companies[i].On, p.Companies[i].Override = true, by, time.UnixMicro(on), override
				}
			}
		}
		m.EndProposal = &p
	}

	// Objectives, merged into the company entries (one per company that fled or completed something).
	var objectives [][3]json.RawMessage
	if err := json.Unmarshal(objectivesJSON, &objectives); err != nil {
		return err
	}
	for _, r := range objectives {
		var company int64
		var key string
		var n int
		if err := unmarshalAll(r[:], &company, &key, &n); err != nil {
			return err
		}
		i := slices.IndexFunc(m.Result.Companies, func(c core.CompanyResult) bool { return c.CompanyID == company })
		if i < 0 {
			m.Result.Companies = append(m.Result.Companies, core.CompanyResult{CompanyID: company})
			i = len(m.Result.Companies) - 1
		}
		if m.Result.Companies[i].Objectives == nil {
			m.Result.Companies[i].Objectives = map[string]int{}
		}
		m.Result.Companies[i].Objectives[key] = n
	}
	order := map[int64]int{}
	for _, s := range m.Sides {
		for _, c := range s.Companies {
			order[c.ID] = len(order)
		}
	}
	sort.SliceStable(m.Result.Companies, func(a, b int) bool {
		return order[m.Result.Companies[a].CompanyID] < order[m.Result.Companies[b].CompanyID]
	})
	return nil
}

// List returns the campaign's matches, newest first.
func (ms matchStore) List(ctx context.Context, campaignID int64) ([]core.Match, error) {
	return ms.load(ctx, ms.s.pool, `m.campaign_id = $1`, false, campaignID)
}

// Get returns one match of the campaign, or ErrNotFound.
func (ms matchStore) Get(ctx context.Context, campaignID, matchID int64) (core.Match, error) {
	list, err := ms.load(ctx, ms.s.pool, `m.campaign_id = $1 AND m.id = $2`, false, campaignID, matchID)
	if err != nil {
		return core.Match{}, err
	}
	if len(list) == 0 {
		return core.Match{}, core.ErrNotFound
	}
	return list[0], nil
}

// Events is the match's history, oldest first; ErrNotFound when the campaign has no such match. One statement: the match row, left
// joined to its events, answers both.
func (ms matchStore) Events(ctx context.Context, campaignID, matchID int64) ([]core.MatchEvent, error) {
	rows, err := ms.s.pool.Query(ctx, `
		SELECT e.kind, u.display_name, e.at, e.detail
		  FROM matches m LEFT JOIN match_events e ON e.match_id = m.id LEFT JOIN users u ON u.id = e.user_id
		 WHERE m.campaign_id = $1 AND m.id = $2 ORDER BY e.id`, campaignID, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.MatchEvent
	found := false
	for rows.Next() {
		found = true
		var kind, by, detail *string
		var at *time.Time
		if err := rows.Scan(&kind, &by, &at, &detail); err != nil {
			return nil, err
		}
		if kind == nil { // the match has no events
			continue
		}
		out = append(out, core.MatchEvent{Kind: *kind, By: *by, On: *at, Detail: *detail})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !found {
		return nil, core.ErrNotFound
	}
	return out, nil
}

// Create stores an open match numbered after the campaign's last one. The campaign row is locked meanwhile, so two matches
// created at once get different numbers.
func (ms matchStore) Create(ctx context.Context, campaignID int64, by core.User, in core.NewMatch) (core.Match, error) {
	tx, err := ms.s.pool.Begin(ctx)
	if err != nil {
		return core.Match{}, err
	}
	defer tx.Rollback(ctx)
	var member bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM campaign_members WHERE campaign_id = $1 AND user_id = $2)
		  FROM campaigns WHERE id = $1 FOR UPDATE`, campaignID, by.ID).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !member) {
		return core.Match{}, core.ErrNotFound
	}
	if err != nil {
		return core.Match{}, err
	}
	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO matches (campaign_id, number, created_by, practice)
		VALUES ($1, (SELECT coalesce(max(number), 0) + 1 FROM matches WHERE campaign_id = $1), $2, $3) RETURNING id`,
		campaignID, by.ID, in.Practice).Scan(&id); err != nil {
		return core.Match{}, err
	}
	for pos, side := range in.Sides {
		if _, err := tx.Exec(ctx, `INSERT INTO match_sides (match_id, position, name, role) VALUES ($1, $2, $3, $4)`, id, pos, side.Name, nullRole(side.Role)); err != nil {
			return core.Match{}, err
		}
		for slot, companyID := range side.Companies {
			_, err := tx.Exec(ctx, `
				INSERT INTO match_side_companies (match_id, campaign_id, position, slot, company_id) VALUES ($1, $2, $3, $4, $5)`,
				id, campaignID, pos, slot, companyID)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				switch pgErr.Code {
				case foreignKeyViolation: // a company that is not one of the campaign's
					return core.Match{}, core.ErrNotFound
				case uniqueViolation:
					return core.Match{}, errors.Join(core.ErrInvalidMatch, errors.New("a company cannot appear twice in a match"))
				}
			}
			if err != nil {
				return core.Match{}, err
			}
		}
	}
	if err := busyCompany(ctx, tx, campaignID, id); err != nil {
		return core.Match{}, err
	}
	list, err := ms.load(ctx, tx, `m.id = $1`, false, id)
	if err != nil || len(list) == 0 {
		return core.Match{}, err
	}
	return list[0], tx.Commit(ctx)
}

// busyCompany refuses with *core.CompanyBusyError when a company of the match is in another open match of the campaign.
// The caller holds the campaign row lock, so two matches cannot both pass.
func busyCompany(ctx context.Context, q querier, campaignID, matchID int64) error {
	var e core.CompanyBusyError
	err := q.QueryRow(ctx, `
		SELECT c.name, o.id, o.number
		  FROM match_side_companies mine
		  JOIN match_side_companies sc ON sc.company_id = mine.company_id AND sc.match_id <> mine.match_id
		  JOIN matches o ON o.id = sc.match_id AND NOT o.ended AND o.campaign_id = $1
		  JOIN companies c ON c.id = mine.company_id
		 WHERE mine.match_id = $2
		 ORDER BY o.number, mine.position, mine.slot LIMIT 1`, campaignID, matchID).Scan(&e.Company, &e.MatchID, &e.Number)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return &e
}

// openMatchOf is the open match the company is in, or nil.
func openMatchOf(ctx context.Context, q querier, companyID int64) (*core.OpenMatchRef, error) {
	var r core.OpenMatchRef
	err := q.QueryRow(ctx, `
		SELECT m.id, m.number FROM match_side_companies sc JOIN matches m ON m.id = sc.match_id AND NOT m.ended
		 WHERE sc.company_id = $1 ORDER BY m.number LIMIT 1`, companyID).Scan(&r.MatchID, &r.Number)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Update runs fn on the locked match and stores the scenario, result and ended flag it leaves behind.
func (ms matchStore) Update(ctx context.Context, campaignID, matchID int64, by core.User, fn func(*core.Match) error) (core.Match, error) {
	tx, err := ms.s.pool.Begin(ctx)
	if err != nil {
		return core.Match{}, err
	}
	defer tx.Rollback(ctx)
	list, err := ms.load(ctx, tx, `m.campaign_id = $1 AND m.id = $2`, true, campaignID, matchID)
	if err != nil {
		return core.Match{}, err
	}
	if len(list) == 0 {
		return core.Match{}, core.ErrNotFound
	}
	before := list[0]
	m := before
	if err := fn(&m); err != nil {
		return core.Match{}, err
	}
	if before.Ended && !m.Ended { // reopening: the same rule as creating, under the same campaign lock
		if _, err := tx.Exec(ctx, `SELECT 1 FROM campaigns WHERE id = $1 FOR UPDATE`, campaignID); err != nil {
			return core.Match{}, err
		}
		if err := busyCompany(ctx, tx, campaignID, matchID); err != nil {
			return core.Match{}, err
		}
	}
	if len(m.Result.Sides) != 0 && len(m.Result.Sides) != len(before.Sides) {
		return core.Match{}, errors.New("postgres: result sides do not match the match's sides")
	}
	var kind *string
	var sc core.MatchScenario
	if m.Scenario != nil {
		k := string(m.Scenario.Kind)
		kind, sc = &k, *m.Scenario
	}
	if _, err := tx.Exec(ctx, `
		UPDATE matches SET scenario_kind = $2, scenario_name = $3, scenario_description = $4, scenario_victory = $5,
		       decided = $6, winner = $7, note = $8, ended = $9,
		       ended_on = CASE WHEN NOT $9 THEN NULL WHEN ended THEN ended_on ELSE now() END,
		       revision = revision + 1
		 WHERE id = $1`,
		matchID, kind, sc.Name, sc.Description, sc.Victory, m.Result.Decided, m.Result.Winner, m.Result.Note, m.Ended); err != nil {
		return core.Match{}, err
	}
	rolesChanged := false
	for pos := range before.Sides {
		rolesChanged = rolesChanged || (pos < len(m.Sides) && m.Sides[pos].Role != before.Sides[pos].Role)
	}
	if rolesChanged { // clear first: swapping Attacker and Defender passes through a state the unique index forbids
		if _, err := tx.Exec(ctx, `UPDATE match_sides SET role = NULL WHERE match_id = $1`, matchID); err != nil {
			return core.Match{}, err
		}
		for pos, side := range m.Sides {
			if side.Role != "" {
				if _, err := tx.Exec(ctx, `UPDATE match_sides SET role = $3 WHERE match_id = $1 AND position = $2`, matchID, pos, side.Role); err != nil {
					return core.Match{}, err
				}
			}
		}
	}
	for pos := range before.Sides {
		var vp *int16
		if pos < len(m.Result.Sides) && m.Result.Sides[pos].VictoryPoints != nil {
			v := int16(*m.Result.Sides[pos].VictoryPoints)
			vp = &v
		}
		if _, err := tx.Exec(ctx, `UPDATE match_sides SET victory_points = $3 WHERE match_id = $1 AND position = $2`, matchID, pos, vp); err != nil {
			return core.Match{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE match_side_companies SET fled = false WHERE match_id = $1`, matchID); err != nil {
		return core.Match{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM match_objectives WHERE match_id = $1`, matchID); err != nil {
		return core.Match{}, err
	}
	for _, c := range m.Result.Companies {
		tag, err := tx.Exec(ctx, `UPDATE match_side_companies SET fled = $3 WHERE match_id = $1 AND company_id = $2`, matchID, c.CompanyID, c.Fled)
		if err != nil {
			return core.Match{}, err
		}
		if tag.RowsAffected() == 0 {
			return core.Match{}, errors.Join(core.ErrInvalidMatch, errors.New("a company in the result is not in this match"))
		}
		for key, n := range c.Objectives {
			if n == 0 {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO match_objectives (match_id, company_id, key, times) VALUES ($1, $2, $3, $4)`,
				matchID, c.CompanyID, key, n); err != nil {
				return core.Match{}, err
			}
		}
	}
	if !reflect.DeepEqual(before.EndProposal, m.EndProposal) {
		if err := storeEndProposal(ctx, tx, matchID, by, m.EndProposal); err != nil {
			return core.Match{}, err
		}
		if !m.Ended { // the pages of the match refetch the pending end; the end itself is announced below
			if err := notifyLive(ctx, tx, liveNotice{Match: matchID, Kind: "changed"}); err != nil {
				return core.Match{}, err
			}
		}
	}
	if m.Ended != before.Ended {
		kind := "reopened"
		if m.Ended {
			kind = "ended"
		}
		detail := ""
		if m.Ended {
			detail = m.EndDetail
		}
		if _, err := tx.Exec(ctx, `INSERT INTO match_events (match_id, kind, user_id, detail) VALUES ($1, $2, $3, $4)`, matchID, kind, by.ID, detail); err != nil {
			return core.Match{}, err
		}
		if err := notifyLive(ctx, tx, liveNotice{Match: matchID, Kind: "ended"}); err != nil {
			return core.Match{}, err
		}
	}
	list, err = ms.load(ctx, tx, `m.id = $1`, false, matchID)
	if err != nil || len(list) == 0 {
		return core.Match{}, err
	}
	return list[0], tx.Commit(ctx)
}

// storeEndProposal replaces the stored pending end by p (nil: none). A confirmation made by its company's owner is stored under the
// owner; an override under the user who made it.
func storeEndProposal(ctx context.Context, tx pgx.Tx, matchID int64, by core.User, p *core.EndProposal) error {
	if _, err := tx.Exec(ctx, `DELETE FROM match_end_proposal WHERE match_id = $1`, matchID); err != nil {
		return err
	}
	if p == nil {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO match_end_proposal (match_id, proposed_by, proposed_on, notice) VALUES ($1, $2, $3, $4)`,
		matchID, p.ProposedByID, p.ProposedOn, p.Notice); err != nil {
		return err
	}
	for _, c := range p.Companies {
		if !c.Confirmed {
			continue
		}
		who := c.OwnerID
		if c.Override {
			who = by.ID
		}
		on := c.On
		if on.IsZero() {
			on = time.Now()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO match_end_confirmation (match_id, company_id, confirmed_by, confirmed_on, override) VALUES ($1, $2, $3, $4, $5)`,
			matchID, c.CompanyID, who, on, c.Override); err != nil {
			return err
		}
	}
	return nil
}

// AddGrant stores the grant and applies it to the company (and the unit's XP) in one transaction.
func (ms matchStore) AddGrant(ctx context.Context, campaignID int64, by core.User, g core.Grant) (core.Grant, error) {
	if g.Experience != 0 && g.UnitID == 0 {
		return core.Grant{}, errors.Join(core.ErrInvalidCompany, errors.New("experience goes to a unit"))
	}
	tx, err := ms.s.pool.Begin(ctx)
	if err != nil {
		return core.Grant{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE companies SET gold = gold + $3, influence = influence + $4
		  WHERE campaign_id = $1 AND id = $2 AND gold + $3 >= 0 AND influence + $4 >= 0`,
		campaignID, g.CompanyID, g.Gold, g.Influence)
	if err != nil {
		return core.Grant{}, err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM companies WHERE campaign_id = $1 AND id = $2)`, campaignID, g.CompanyID).Scan(&exists); err != nil {
			return core.Grant{}, err
		}
		if exists {
			return core.Grant{}, core.ErrNegativeBalance
		}
		return core.Grant{}, core.ErrNotFound
	}
	if g.Experience != 0 {
		tag, err := tx.Exec(ctx, `UPDATE company_units SET xp = xp + $3 WHERE company_id = $1 AND id = $2`, g.CompanyID, g.UnitID, g.Experience)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) { // xp would go below zero
			return core.Grant{}, errors.Join(core.ErrInvalidCompany, errors.New("a unit cannot have negative experience"))
		}
		if err != nil {
			return core.Grant{}, err
		}
		if tag.RowsAffected() == 0 {
			return core.Grant{}, core.ErrNotFound
		}
	}
	var unit, match *int64
	if g.UnitID != 0 {
		unit = &g.UnitID
	}
	if g.MatchID != 0 {
		match = &g.MatchID
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO grants (company_id, kind, gold, influence, experience, unit_id, match_id, note, granted_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id, granted_at,
		       (SELECT display_name FROM users WHERE id = $9)`,
		g.CompanyID, string(g.Kind), g.Gold, g.Influence, g.Experience, unit, match, g.Note, by.ID).Scan(&g.ID, &g.On, &g.By)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case foreignKeyViolation: // a match that does not exist
			return core.Grant{}, core.ErrNotFound
		case "23514": // check violation: note or unit missing
			return core.Grant{}, errors.Join(core.ErrInvalidCompany, errors.New("the grant needs a note, and a unit for experience"))
		}
	}
	if err != nil {
		return core.Grant{}, err
	}
	return g, tx.Commit(ctx)
}

// Grants is the company's history, newest first.
func (ms matchStore) Grants(ctx context.Context, campaignID, companyID int64) ([]core.Grant, error) {
	var exists bool
	if err := ms.s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM companies WHERE campaign_id = $1 AND id = $2)`, campaignID, companyID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, core.ErrNotFound
	}
	rows, err := ms.s.pool.Query(ctx, `
		SELECT g.id, g.kind, g.gold, g.influence, g.experience, g.unit_id, g.match_id, g.note, coalesce(u.display_name, ''), g.granted_at
		  FROM grants g LEFT JOIN users u ON u.id = g.granted_by
		 WHERE g.company_id = $1 ORDER BY g.id DESC`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Grant
	for rows.Next() {
		g := core.Grant{CompanyID: companyID}
		var kind string
		var unit, match *int64
		var on time.Time
		if err := rows.Scan(&g.ID, &kind, &g.Gold, &g.Influence, &g.Experience, &unit, &match, &g.Note, &g.By, &on); err != nil {
			return nil, err
		}
		g.Kind, g.On = core.GrantKind(kind), on
		if unit != nil {
			g.UnitID = *unit
		}
		if match != nil {
			g.MatchID = *match
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// nullRole stores no tag as NULL.
func nullRole(role string) *string {
	if role == "" {
		return nil
	}
	return &role
}
