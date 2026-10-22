package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// campaignStore implements the core campaign repository; Store.Campaigns returns it.
type campaignStore struct{ s *Store }

// Campaigns returns the campaign repository.
func (s *Store) Campaigns() campaignStore { return campaignStore{s} }

// settingsJSON is the stored shape of core.CampaignSettings. "v" lets the code migrate old shapes lazily.
type settingsJSON struct {
	V                     int  `json:"v"`
	HeroesDropProfileGear bool `json:"heroes_drop_profile_gear,omitempty"`
	ShakeTheTable         bool `json:"shake_the_table,omitempty"`
	ManualRolls           bool `json:"manual_rolls,omitempty"`
	CustomOnTheirOwnPath  bool `json:"custom_on_their_own_path,omitempty"`
}

func encodeSettings(s core.CampaignSettings) ([]byte, error) {
	return json.Marshal(settingsJSON{1, s.HeroesDropProfileGear, s.ShakeTheTable, s.ManualRolls, s.CustomOnTheirOwnPath})
}

func decodeSettings(b []byte) (core.CampaignSettings, error) {
	var j settingsJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return core.CampaignSettings{}, err
	}
	return core.CampaignSettings{HeroesDropProfileGear: j.HeroesDropProfileGear, ShakeTheTable: j.ShakeTheTable,
		ManualRolls: j.ManualRolls, CustomOnTheirOwnPath: j.CustomOnTheirOwnPath}, nil
}

// companyFigures selects a company with the figures derived from its rows. Points and FullPoints depend on the rules files
// (the price of a unit is in the vanilla profile), which SQL cannot read: they are left at zero for the service to fill in
// from BuildDetail. Wins, draws and losses count the company's ended matches: it won when its side is the match's winner
// (matches.winner is the 1-based side, 0 a draw).
const companyFigures = `
	SELECT c.id, c.name, c.faction, c.owner_id, u.display_name,
	       (SELECT count(*) FROM company_units cu WHERE cu.company_id = c.id)::int,
	       (SELECT count(*) FROM match_side_companies mc JOIN matches m ON m.id = mc.match_id
	         WHERE mc.company_id = c.id AND m.ended AND m.winner = mc.position + 1)::int,
	       (SELECT count(*) FROM match_side_companies mc JOIN matches m ON m.id = mc.match_id
	         WHERE mc.company_id = c.id AND m.ended AND m.winner = 0)::int,
	       (SELECT count(*) FROM match_side_companies mc JOIN matches m ON m.id = mc.match_id
	         WHERE mc.company_id = c.id AND m.ended AND m.winner <> 0 AND m.winner <> mc.position + 1)::int
	  FROM companies c JOIN users u ON u.id = c.owner_id`

func scanCompany(row pgx.Row) (core.CampaignCompany, error) {
	var c core.CampaignCompany
	err := row.Scan(&c.ID, &c.Name, &c.Faction, &c.OwnerID, &c.Owner, &c.Units, &c.Wins, &c.Draws, &c.Losses)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, core.ErrNotFound
	}
	return c, err
}

// List returns the user's campaigns: unfinished first, then finished, each newest first.
func (cs campaignStore) List(ctx context.Context, userID int64, withFinished bool) ([]core.CampaignSummary, error) {
	rows, err := cs.s.pool.Query(ctx, `
		SELECT g.id, g.name, g.description, g.status, g.round, g.rounds, c.id,
		       (SELECT count(*) FROM matches x WHERE x.campaign_id = g.id)::int
		  FROM campaign_members m
		  JOIN campaigns g ON g.id = m.campaign_id
		  LEFT JOIN companies c ON c.campaign_id = g.id AND c.owner_id = m.user_id
		 WHERE m.user_id = $1 AND ($2 OR g.status <> 'finished')
		 ORDER BY (g.status = 'finished'), g.created_at DESC, g.id DESC`, userID, withFinished)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.CampaignSummary
	var companyIDs []*int64
	for rows.Next() {
		var c core.CampaignSummary
		var status string
		var companyID *int64
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &status, &c.Round, &c.Rounds, &companyID, &c.Matches); err != nil {
			return nil, err
		}
		c.Status = core.CampaignStatus(status)
		out = append(out, c)
		companyIDs = append(companyIDs, companyID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i, id := range companyIDs {
		if id == nil {
			continue
		}
		co, err := scanCompany(cs.s.pool.QueryRow(ctx, companyFigures+` WHERE c.id = $1`, *id))
		if err != nil {
			return nil, err
		}
		out[i].Company = &co
	}
	return out, nil
}

// Create stores the campaign and makes the owner its organiser and first member in one transaction.
func (cs campaignStore) Create(ctx context.Context, owner core.User, c core.NewCampaign) (core.CampaignSummary, error) {
	settings, err := encodeSettings(c.Settings)
	if err != nil {
		return core.CampaignSummary{}, err
	}
	tx, err := cs.s.pool.Begin(ctx)
	if err != nil {
		return core.CampaignSummary{}, err
	}
	defer tx.Rollback(ctx)
	out := core.CampaignSummary{Name: c.Name, Description: c.Description, Status: core.CampaignActive, Rounds: c.Rounds}
	err = tx.QueryRow(ctx, `
		INSERT INTO campaigns (name, description, rounds, vanilla_hash, bc_hash, glossary_hash, settings, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		c.Name, c.Description, c.Rounds, c.Vanilla, c.BattleCompanies, c.Glossary, settings, owner.ID).Scan(&out.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return core.CampaignSummary{}, core.ErrNotFound // a rules file that is not stored (or of the wrong kind)
	}
	if err != nil {
		return core.CampaignSummary{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES ($1, $2, 'organiser')`, out.ID, owner.ID); err != nil {
		return core.CampaignSummary{}, err
	}
	return out, tx.Commit(ctx)
}

// Get returns the campaign as a member sees it; ErrNotFound for non-members.
func (cs campaignStore) Get(ctx context.Context, userID, id int64) (core.CampaignDetail, error) {
	var d core.CampaignDetail
	var status string
	var settings []byte
	var vh, bh, gh string
	var companyID *int64
	err := cs.s.pool.QueryRow(ctx, `
		SELECT g.id, g.name, g.description, g.status, g.round, g.rounds, g.settings, g.vanilla_hash, g.bc_hash, g.glossary_hash,
		       o.user_id, ou.display_name, (SELECT count(*) FROM campaign_members x WHERE x.campaign_id = g.id)::int, c.id,
		       (SELECT count(*) FROM matches x WHERE x.campaign_id = g.id)::int,
		       (SELECT count(*) FROM campaign_invites i WHERE i.campaign_id = g.id AND i.revoked_at IS NULL AND i.expires_at > now()
		           AND (i.max_uses = 0 OR i.uses < i.max_uses))::int,
		       (SELECT count(*) FROM matches x WHERE x.campaign_id = g.id AND NOT x.ended)::int,
		       (SELECT count(*) FROM match_side_companies sc JOIN matches x ON x.id = sc.match_id AND x.ended
		          LEFT JOIN match_aftermaths a ON a.match_id = sc.match_id AND a.company_id = sc.company_id
		         WHERE sc.campaign_id = g.id AND COALESCE(a.status, 'pending') NOT IN ('done', 'waived'))::int
		  FROM campaigns g
		  JOIN campaign_members m ON m.campaign_id = g.id AND m.user_id = $1
		  JOIN campaign_members o ON o.campaign_id = g.id AND o.role = 'organiser'
		  JOIN users ou ON ou.id = o.user_id
		  LEFT JOIN companies c ON c.campaign_id = g.id AND c.owner_id = m.user_id
		 WHERE g.id = $2`, userID, id).
		Scan(&d.ID, &d.Name, &d.Description, &status, &d.Round, &d.Rounds, &settings, &vh, &bh, &gh,
			&d.OrganiserID, &d.Organiser, &d.Players, &companyID, &d.Matches, &d.ActiveInvites, &d.OpenMatches, &d.PendingAftermaths)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.CampaignDetail{}, core.ErrNotFound
	}
	if err != nil {
		return core.CampaignDetail{}, err
	}
	d.Status = core.CampaignStatus(status)
	if d.Settings, err = decodeSettings(settings); err != nil {
		return core.CampaignDetail{}, err
	}
	for _, h := range []string{vh, bh, gh} {
		info, err := cs.s.Rulesets().Info(ctx, h)
		if err != nil {
			return core.CampaignDetail{}, err
		}
		d.Rulesets = append(d.Rulesets, info)
	}
	if companyID != nil {
		co, err := scanCompany(cs.s.pool.QueryRow(ctx, companyFigures+` WHERE c.id = $1`, *companyID))
		if err != nil {
			return core.CampaignDetail{}, err
		}
		d.Company = &co
	}
	return d, nil
}

// Companies lists the campaign's companies, oldest first.
func (cs campaignStore) Companies(ctx context.Context, id int64) ([]core.CampaignCompany, error) {
	rows, err := cs.s.pool.Query(ctx, companyFigures+` WHERE c.campaign_id = $1 ORDER BY c.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.CampaignCompany
	for rows.Next() {
		var c core.CampaignCompany
		if err := rows.Scan(&c.ID, &c.Name, &c.Faction, &c.OwnerID, &c.Owner, &c.Units, &c.Wins, &c.Draws, &c.Losses); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Company returns one company of the campaign, or ErrNotFound.
func (cs campaignStore) Company(ctx context.Context, id, companyID int64) (core.CampaignCompany, error) {
	return scanCompany(cs.s.pool.QueryRow(ctx, companyFigures+` WHERE c.campaign_id = $1 AND c.id = $2`, id, companyID))
}

// Hashes returns the rules file hashes of the campaign and its settings.
func (cs campaignStore) Hashes(ctx context.Context, id int64) (core.RulesetHashes, core.CampaignSettings, error) {
	var h core.RulesetHashes
	var raw []byte
	err := cs.s.pool.QueryRow(ctx, `SELECT vanilla_hash, bc_hash, glossary_hash, settings FROM campaigns WHERE id = $1`, id).
		Scan(&h.Vanilla, &h.BattleCompanies, &h.Glossary, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, core.CampaignSettings{}, core.ErrNotFound
	}
	if err != nil {
		return h, core.CampaignSettings{}, err
	}
	s, err := decodeSettings(raw)
	return h, s, err
}

const foreignKeyViolation = "23503"
const checkViolation = "23514"

// Members lists the campaign's members, the organiser first, then by name; the company is the member's, if they have one.
func (cs campaignStore) Members(ctx context.Context, id int64) ([]core.CampaignMember, error) {
	var exists bool
	if err := cs.s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM campaigns WHERE id = $1)`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, core.ErrNotFound
	}
	rows, err := cs.s.pool.Query(ctx, `
		SELECT m.user_id, u.display_name, m.role = 'organiser', COALESCE(c.id, 0), COALESCE(c.name, '')
		  FROM campaign_members m
		  JOIN users u ON u.id = m.user_id
		  LEFT JOIN companies c ON c.campaign_id = m.campaign_id AND c.owner_id = m.user_id
		 WHERE m.campaign_id = $1
		 ORDER BY (m.role <> 'organiser'), lower(u.display_name), m.user_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.CampaignMember
	for rows.Next() {
		var m core.CampaignMember
		if err := rows.Scan(&m.UserID, &m.Name, &m.Organiser, &m.CompanyID, &m.CompanyName); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (cs campaignStore) HandOver(ctx context.Context, campaignID, newOrganiser int64, allowed func(currentOrganiser int64) error) error {
	tx, err := cs.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current int64
	err = tx.QueryRow(ctx,
		`SELECT user_id FROM campaign_members WHERE campaign_id = $1 AND role = 'organiser' FOR UPDATE`, campaignID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := allowed(current); err != nil {
		return err
	}
	var member int64
	err = tx.QueryRow(ctx,
		`SELECT user_id FROM campaign_members WHERE campaign_id = $1 AND user_id = $2 FOR UPDATE`, campaignID, newOrganiser).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Refuse("The new organiser must be a member of the campaign")
	}
	if err != nil {
		return err
	}
	// the partial unique index allows one organiser: demote before promoting
	if _, err := tx.Exec(ctx, `UPDATE campaign_members SET role = 'player' WHERE campaign_id = $1 AND user_id = $2`, campaignID, current); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE campaign_members SET role = 'organiser' WHERE campaign_id = $1 AND user_id = $2`, campaignID, newOrganiser); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetStatus changes the status under a row lock; ending a campaign revokes its invitation links in the same transaction.
func (cs campaignStore) SetStatus(ctx context.Context, campaignID int64, fn func(organiserID int64, name string, current core.CampaignStatus) (core.CampaignStatus, error)) error {
	tx, err := cs.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status, name string
	var organiser int64
	err = tx.QueryRow(ctx, `
		SELECT g.status, g.name, o.user_id FROM campaigns g JOIN campaign_members o ON o.campaign_id = g.id AND o.role = 'organiser'
		 WHERE g.id = $1 FOR UPDATE OF g`, campaignID).Scan(&status, &name, &organiser)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	if err != nil {
		return err
	}
	to, err := fn(organiser, name, core.CampaignStatus(status))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE campaigns SET status = $2 WHERE id = $1`, campaignID, string(to)); err != nil {
		return err
	}
	if to == core.CampaignFinished {
		if _, err := tx.Exec(ctx, `UPDATE campaign_invites SET revoked_at = now() WHERE campaign_id = $1 AND revoked_at IS NULL`, campaignID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
