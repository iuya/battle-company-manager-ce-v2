package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// aftermathStore implements the core aftermath repository; Store.Aftermaths returns it.
type aftermathStore struct{ s *Store }

// Aftermaths returns the aftermath repository.
func (s *Store) Aftermaths() aftermathStore { return aftermathStore{s} }

type aftermathInputJSON struct {
	V     int                 `json:"v"`
	Input core.AftermathInput `json:"input"`
}
type aftermathStateJSON struct {
	V    int                `json:"v"`
	Work core.AftermathWork `json:"work"`
}
type aftermathLinesJSON struct {
	V     int                  `json:"v"`
	Lines []core.AftermathLine `json:"lines"`
}

// pendingRefs lists the aftermaths of ended matches the company has not finished, oldest first.
func pendingRefs(ctx context.Context, q querier, companyID int64, name string) ([]core.AftermathRef, error) {
	rows, err := q.Query(ctx, `
		SELECT m.id, m.number, COALESCE(a.status, 'pending')
		  FROM match_side_companies sc
		  JOIN matches m ON m.id = sc.match_id AND m.ended
		  LEFT JOIN match_aftermaths a ON a.match_id = sc.match_id AND a.company_id = sc.company_id
		 WHERE sc.company_id = $1 AND COALESCE(a.status, 'pending') NOT IN ('done', 'waived')
		 ORDER BY m.number`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.AftermathRef
	for rows.Next() {
		r := core.AftermathRef{CompanyID: companyID, CompanyName: name}
		var status string
		if err := rows.Scan(&r.MatchID, &r.MatchNumber, &status); err != nil {
			return nil, err
		}
		r.Status = core.AftermathStatus(status)
		out = append(out, r)
	}
	return out, rows.Err()
}

// readRecord reads the record (the zero one when none is stored). The match row is locked shared (the match cannot be reopened
// meanwhile) and checked to belong to the campaign and to have the company; lock also locks the record for update.
func readRecord(ctx context.Context, q querier, campaignID, matchID, companyID int64, lock bool) (core.AftermathRecord, error) {
	rec := core.AftermathRecord{MatchID: matchID, CompanyID: companyID, Status: core.AftermathPending}
	query := `SELECT m.ended FROM matches m JOIN match_side_companies sc ON sc.match_id = m.id
	           WHERE m.id = $1 AND m.campaign_id = $2 AND sc.company_id = $3`
	if lock {
		query += ` FOR SHARE OF m`
	}
	if err := q.QueryRow(ctx, query, matchID, campaignID, companyID).Scan(&rec.MatchEnded); errors.Is(err, pgx.ErrNoRows) {
		return rec, core.ErrNotFound
	} else if err != nil {
		return rec, err
	}
	return rec, nil
}

func loadRecord(ctx context.Context, q querier, rec *core.AftermathRecord, lock bool) error {
	query := `SELECT status, revision, input, state, lines, started_at, finished_at FROM match_aftermaths WHERE match_id = $1 AND company_id = $2`
	if lock {
		query += ` FOR UPDATE`
	}
	var status string
	var input, state, lines []byte
	err := q.QueryRow(ctx, query, rec.MatchID, rec.CompanyID).Scan(&status, &rec.Revision, &input, &state, &lines, &rec.StartedAt, &rec.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rec.Status = core.AftermathStatus(status)
	var in aftermathInputJSON
	var st aftermathStateJSON
	var ln aftermathLinesJSON
	for _, x := range []struct {
		raw  []byte
		into any
		what string
	}{{input, &in, "input"}, {state, &st, "state"}, {lines, &ln, "lines"}} {
		if err := json.Unmarshal(x.raw, x.into); err != nil {
			return fmt.Errorf("aftermath of company %d in match %d, %s: %w", rec.CompanyID, rec.MatchID, x.what, err)
		}
	}
	rec.Input, rec.Work, rec.Lines = in.Input, st.Work, ln.Lines
	return nil
}

// Get returns the record, or ErrNotFound when the company is not in the match of the campaign.
func (as aftermathStore) Get(ctx context.Context, campaignID, matchID, companyID int64) (core.AftermathRecord, error) {
	tx, err := as.s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return core.AftermathRecord{}, err
	}
	defer tx.Rollback(ctx)
	rec, err := readRecord(ctx, tx, campaignID, matchID, companyID, false)
	if err != nil {
		return rec, err
	}
	return rec, loadRecord(ctx, tx, &rec, false)
}

// Update runs fn on the company and its record under the locks of the match (shared), the company and the record.
func (as aftermathStore) Update(ctx context.Context, campaignID, matchID, companyID int64, fn func(*core.CompanyState, *core.AftermathRecord) error) error {
	tx, err := as.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rec, err := readRecord(ctx, tx, campaignID, matchID, companyID, true)
	if err != nil {
		return err
	}
	cs := as.s.Companies()
	st, err := cs.load(ctx, tx, campaignID, companyID, true)
	if err != nil {
		return err
	}
	if err := loadRecord(ctx, tx, &rec, true); err != nil {
		return err
	}
	if err := fn(&st, &rec); err != nil {
		return err
	}
	if err := cs.save(ctx, tx, st); err != nil {
		return err
	}
	in, _ := json.Marshal(aftermathInputJSON{1, rec.Input})
	work, _ := json.Marshal(aftermathStateJSON{1, rec.Work})
	lines, _ := json.Marshal(aftermathLinesJSON{1, rec.Lines})
	if _, err := tx.Exec(ctx, `
		INSERT INTO match_aftermaths (match_id, company_id, status, revision, input, state, lines, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (match_id, company_id) DO UPDATE SET status = $3, revision = $4, input = $5, state = $6, lines = $7,
		       started_at = $8, finished_at = $9`,
		matchID, companyID, string(rec.Status), rec.Revision, in, work, lines, rec.StartedAt, rec.FinishedAt); err != nil {
		return err
	}
	for _, g := range rec.NewGrants {
		if _, err := tx.Exec(ctx, `INSERT INTO grants (company_id, kind, gold, influence, experience, unit_id, match_id, note)
			VALUES ($1, 'scenario', $2, $3, $4, NULLIF($5::bigint, 0), $6, $7)`,
			companyID, g.Gold, g.Influence, g.Experience, g.UnitID, matchID, g.Note); err != nil {
			return err
		}
	}
	for _, p := range rec.NewPerks {
		if _, err := tx.Exec(ctx, `INSERT INTO company_perks (company_id, match_id, unit_id, effect, name, text, applies)
			VALUES ($1, $2, NULLIF($3, 0), $4, $5, $6, $7)`, companyID, matchID, p.UnitID, p.Effect, p.Name, p.Text, p.When); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Statuses lists the companies of the match with their status.
func (as aftermathStore) Statuses(ctx context.Context, campaignID, matchID int64) ([]core.AftermathRef, error) {
	rows, err := as.s.pool.Query(ctx, `
		SELECT c.id, c.name, m.number, m.ended, COALESCE(a.status, 'pending')
		  FROM match_side_companies sc
		  JOIN matches m ON m.id = sc.match_id
		  JOIN companies c ON c.id = sc.company_id
		  LEFT JOIN match_aftermaths a ON a.match_id = sc.match_id AND a.company_id = sc.company_id
		 WHERE sc.match_id = $1 AND m.campaign_id = $2
		 ORDER BY sc.position, sc.slot`, matchID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.AftermathRef
	for rows.Next() {
		r := core.AftermathRef{MatchID: matchID}
		var ended bool
		var status string
		if err := rows.Scan(&r.CompanyID, &r.CompanyName, &r.MatchNumber, &ended, &status); err != nil {
			return nil, err
		}
		if ended {
			r.Status = core.AftermathStatus(status)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, core.ErrNotFound
	}
	return out, nil
}

// Pending lists the aftermaths the company has to finish.
func (as aftermathStore) Pending(ctx context.Context, campaignID, companyID int64) ([]core.AftermathRef, error) {
	var name string
	if err := as.s.pool.QueryRow(ctx, `SELECT name FROM companies WHERE id = $1 AND campaign_id = $2`, companyID, campaignID).Scan(&name); errors.Is(err, pgx.ErrNoRows) {
		return nil, core.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return pendingRefs(ctx, as.s.pool, companyID, name)
}

// Started says whether a company has started the aftermath of the match or has it waived.
func (as aftermathStore) Started(ctx context.Context, campaignID, matchID int64) (bool, error) {
	var started bool
	err := as.s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM match_aftermaths a JOIN matches m ON m.id = a.match_id
		                WHERE a.match_id = $1 AND m.campaign_id = $2 AND a.status IN ('injuries', 'levelups', 'done', 'waived'))`,
		matchID, campaignID).Scan(&started)
	return started, err
}

// Perks lists the company's perks, unused first.
func (as aftermathStore) Perks(ctx context.Context, campaignID, companyID int64) ([]core.Perk, error) {
	rows, err := as.s.pool.Query(ctx, `
		SELECT p.id, COALESCE(p.match_id, 0), COALESCE(p.unit_id, 0), p.effect, p.name, p.text, p.applies, p.used_on
		  FROM company_perks p JOIN companies c ON c.id = p.company_id
		 WHERE p.company_id = $1 AND c.campaign_id = $2
		 ORDER BY (p.used_on IS NOT NULL), p.id`, companyID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.Perk
	for rows.Next() {
		var p core.Perk
		if err := rows.Scan(&p.ID, &p.MatchID, &p.UnitID, &p.Effect, &p.Name, &p.Text, &p.When, &p.UsedOn); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UsePerk marks an unused perk used.
func (as aftermathStore) UsePerk(ctx context.Context, campaignID, companyID, perkID int64) error {
	var used bool
	err := as.s.pool.QueryRow(ctx, `
		WITH target AS (
			SELECT p.id, p.used_on IS NOT NULL AS was_used FROM company_perks p JOIN companies c ON c.id = p.company_id
			 WHERE p.id = $1 AND p.company_id = $2 AND c.campaign_id = $3 FOR UPDATE OF p),
		     upd AS (UPDATE company_perks SET used_on = now() WHERE id IN (SELECT id FROM target WHERE NOT was_used))
		SELECT was_used FROM target`, perkID, companyID, campaignID).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	}
	if err != nil {
		return err
	}
	if used {
		return core.Refuse("That perk has been used already.")
	}
	return nil
}

// History is the latest lines of the company's aftermaths, newest aftermath first (each in the order it was written). It takes
// whole aftermaths until it has at least limit lines, so the last one is never cut in the middle.
func (as aftermathStore) History(ctx context.Context, campaignID, companyID int64, limit int) ([]core.AftermathLine, error) {
	rows, err := as.s.pool.Query(ctx, `
		SELECT m.id, m.number, COALESCE(a.finished_at, a.started_at, m.ended_on), a.lines
		  FROM match_aftermaths a JOIN matches m ON m.id = a.match_id
		 WHERE a.company_id = $1 AND m.campaign_id = $2 AND a.lines <> '{"v": 1}'
		 ORDER BY COALESCE(a.finished_at, a.started_at) DESC NULLS LAST, m.number DESC LIMIT $3`, companyID, campaignID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.AftermathLine
	for rows.Next() && len(out) < limit {
		var matchID int64
		var number int
		var on *time.Time
		var raw []byte
		if err := rows.Scan(&matchID, &number, &on, &raw); err != nil {
			return nil, err
		}
		var ln aftermathLinesJSON
		if err := json.Unmarshal(raw, &ln); err != nil {
			return nil, err
		}
		for _, l := range ln.Lines { // a whole aftermath at a time: the limit never cuts one in the middle
			l.Match, l.MatchID = number, matchID
			if on != nil {
				l.On = *on
			}
			out = append(out, l)
		}
	}
	return out, rows.Err()
}
