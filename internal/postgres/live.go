package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// liveChannel is the NOTIFY channel of the live mode.
const liveChannel = "live_match"

// liveFeedName names the listening connection in pg_stat_activity.
const liveFeedName = "bcm-live-feed"

// liveNotice is the payload of a notification: ids only, never state.
type liveNotice struct {
	Match   int64  `json:"m"`
	Company int64  `json:"c"`
	Seq     int64  `json:"s"`
	Kind    string `json:"k"` // "changed" | "ended"
}

// notifyLive sends the notification inside the transaction, so listeners hear it after the commit only.
func notifyLive(ctx context.Context, q querier, n liveNotice) error {
	raw, err := json.Marshal(n)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `SELECT pg_notify($1, $2)`, liveChannel, string(raw))
	return err
}

// liveStore implements the core live repository; Store.Live returns it.
type liveStore struct{ s *Store }

// Live returns the live match repository.
func (s *Store) Live() liveStore { return liveStore{s} }

// liveActionJSON is the stored shape of a core.LiveAction, versioned like the other jsonb columns.
type liveActionJSON struct {
	V       int                `json:"v"`
	Kind    core.LiveKind      `json:"kind"`
	UnitID  int64              `json:"unit,omitempty"`
	Stat    string             `json:"stat,omitempty"`
	Delta   int                `json:"delta,omitempty"`
	Set     *int               `json:"set,omitempty"`
	Targets []core.WoundTarget `json:"targets,omitempty"`
	Count   int                `json:"count,omitempty"`
	Why     string             `json:"why,omitempty"`
	Note    string             `json:"note,omitempty"`
	Mounted bool               `json:"mounted,omitempty"`
	On      bool               `json:"on,omitempty"`
	Key     string             `json:"key,omitempty"`
	Tier    int                `json:"tier,omitempty"`
	Target  int64              `json:"target,omitempty"`
	// the Break Point and the Against the Odds items (36y)
	Turn      int    `json:"turn,omitempty"`
	Field     string `json:"field,omitempty"`
	StandFast bool   `json:"standFast,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Flee      bool   `json:"flee,omitempty"`
	Item      string `json:"item,omitempty"`
	Index     int    `json:"index,omitempty"`
}

func encodeAction(a core.LiveAction) ([]byte, error) {
	return json.Marshal(liveActionJSON{V: 1, Kind: a.Kind, UnitID: a.UnitID, Stat: a.Stat, Delta: a.Delta, Set: a.Set,
		Targets: a.Targets, Count: a.Count, Why: a.Why, Note: a.Note, Mounted: a.Mounted, On: a.On, Target: a.TargetCompanyID, Key: a.ObjectiveKey, Tier: a.Tier,
		Turn: a.Turn, Field: a.Field, StandFast: a.StandFast, Reason: a.Reason, Flee: a.Flee, Item: a.Item, Index: a.Index})
}

func decodeAction(raw []byte) (core.LiveAction, error) {
	var j liveActionJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return core.LiveAction{}, err
	}
	return core.LiveAction{Kind: j.Kind, UnitID: j.UnitID, Stat: j.Stat, Delta: j.Delta, Set: j.Set, Targets: j.Targets,
		Count: j.Count, Why: j.Why, Note: j.Note, Mounted: j.Mounted, On: j.On, TargetCompanyID: j.Target, ObjectiveKey: j.Key, Tier: j.Tier,
		Turn: j.Turn, Field: j.Field, StandFast: j.StandFast, Reason: j.Reason, Flee: j.Flee, Item: j.Item, Index: j.Index}, nil
}

const liveEntryColumns = `l.id, l.seq, l.match_id, l.company_id, l.unit_id, l.touches, l.payload, l.text, coalesce(u.display_name, ''),
	coalesce(l.by_user_id, 0), l.created_at`

func scanEntry(rows pgx.Rows) (core.LiveEntry, error) {
	var e core.LiveEntry
	var payload []byte
	if err := rows.Scan(&e.ID, &e.Seq, &e.MatchID, &e.CompanyID, &e.UnitID, &e.Touches, &payload, &e.Text, &e.By, &e.ByUserID, &e.On); err != nil {
		return e, err
	}
	if len(e.Touches) == 0 {
		e.Touches = nil
	}
	a, err := decodeAction(payload)
	if err != nil {
		return e, fmt.Errorf("live entry %d payload: %w", e.ID, err)
	}
	e.Action = a
	return e, nil
}

// snapshot reads the match of the campaign (ErrNotFound otherwise), its counter, units and not-undone entries.
func (ls liveStore) snapshot(ctx context.Context, q querier, campaignID, matchID int64) (core.LiveSnapshot, error) {
	snap := core.LiveSnapshot{MatchID: matchID}
	err := q.QueryRow(ctx, `
		SELECT m.number, m.ended, coalesce(l.seq, 0), l.match_id IS NOT NULL
		  FROM matches m LEFT JOIN match_live l ON l.match_id = m.id
		 WHERE m.id = $1 AND m.campaign_id = $2`, matchID, campaignID).Scan(&snap.Number, &snap.Ended, &snap.Seq, &snap.Initialised)
	if errors.Is(err, pgx.ErrNoRows) {
		return snap, core.ErrNotFound
	}
	if err != nil {
		return snap, err
	}
	if !snap.Initialised {
		return snap, nil
	}
	rows, err := q.Query(ctx, `
		SELECT unit_id, company_id, start_wounds, start_might, start_will, start_fate, has_mount, mount_wounds, counts_as_kill, absent
		  FROM match_live_unit WHERE match_id = $1 ORDER BY company_id, unit_id`, matchID)
	if err != nil {
		return snap, err
	}
	for rows.Next() {
		var u core.LiveUnitInit
		if err := rows.Scan(&u.UnitID, &u.CompanyID, &u.Wounds, &u.Might, &u.Will, &u.Fate, &u.HasMount, &u.MountWounds, &u.CountsAsKill, &u.Absent); err != nil {
			rows.Close()
			return snap, err
		}
		snap.Units = append(snap.Units, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return snap, err
	}
	rows, err = q.Query(ctx, `
		SELECT `+liveEntryColumns+` FROM match_live_log l LEFT JOIN users u ON u.id = l.by_user_id
		 WHERE l.match_id = $1 AND l.undone_at IS NULL ORDER BY l.seq`, matchID)
	if err != nil {
		return snap, err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return snap, err
		}
		snap.Entries = append(snap.Entries, e)
	}
	return snap, rows.Err()
}

// Init stores the units once and creates the counter row; it does nothing when the match is already initialised.
func (ls liveStore) Init(ctx context.Context, campaignID, matchID int64, units []core.LiveUnitInit) error {
	tx, err := ls.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT true FROM matches WHERE id = $1 AND campaign_id = $2`, matchID, campaignID).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
		return core.ErrNotFound
	} else if err != nil {
		return err
	}
	// A concurrent Init waits here for the first to commit, then finds the row and does nothing.
	tag, err := tx.Exec(ctx, `INSERT INTO match_live (match_id) VALUES ($1) ON CONFLICT DO NOTHING`, matchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	for _, u := range units {
		_, err := tx.Exec(ctx, `
			INSERT INTO match_live_unit (match_id, unit_id, company_id, start_wounds, start_might, start_will, start_fate,
			                             has_mount, mount_wounds, counts_as_kill, absent)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			matchID, u.UnitID, u.CompanyID, u.Wounds, u.Might, u.Will, u.Fate, u.HasMount, u.MountWounds, u.CountsAsKill, u.Absent)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case foreignKeyViolation: // a company that is not in the match
				return core.ErrNotFound
			case uniqueViolation:
				return errors.Join(core.ErrInvalidLive, errors.New("a unit appears twice"))
			case checkViolation:
				return errors.Join(core.ErrInvalidLive, errors.New("negative starting value"))
			}
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Read returns the snapshot in one repeatable-read transaction, without locking.
func (ls liveStore) Read(ctx context.Context, campaignID, matchID int64) (core.LiveSnapshot, error) {
	tx, err := ls.s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return core.LiveSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	return ls.snapshot(ctx, tx, campaignID, matchID)
}

var errLiveNotInitialised = errors.New("postgres: live match is not initialised")

// Update locks the match row, runs fn on a fresh snapshot and applies its change; see core.liveRepository.
func (ls liveStore) Update(ctx context.Context, campaignID, matchID int64, by core.User, fn func(core.LiveSnapshot) (core.LiveChange, error)) (core.LiveEntry, error) {
	tx, err := ls.s.pool.Begin(ctx)
	if err != nil {
		return core.LiveEntry{}, err
	}
	defer tx.Rollback(ctx)
	var locked int64
	err = tx.QueryRow(ctx, `SELECT id FROM matches WHERE id = $1 AND campaign_id = $2 FOR NO KEY UPDATE`, matchID, campaignID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.LiveEntry{}, core.ErrNotFound
	}
	if err != nil {
		return core.LiveEntry{}, err
	}
	snap, err := ls.snapshot(ctx, tx, campaignID, matchID)
	if err != nil {
		return core.LiveEntry{}, err
	}
	change, err := fn(snap)
	if err != nil {
		return core.LiveEntry{}, err
	}
	if !snap.Initialised {
		return core.LiveEntry{}, errLiveNotInitialised
	}
	var seq int64
	if err := tx.QueryRow(ctx, `UPDATE match_live SET seq = seq + 1 WHERE match_id = $1 RETURNING seq`, matchID).Scan(&seq); err != nil {
		return core.LiveEntry{}, err
	}
	var out core.LiveEntry
	switch {
	case change.Append != nil && change.Undo == 0:
		e := *change.Append
		payload, err := encodeAction(e.Action)
		if err != nil {
			return core.LiveEntry{}, err
		}
		touches := e.Touches
		if touches == nil {
			touches = []int64{}
		}
		var uid *int64
		if by.ID != 0 {
			uid = &by.ID
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO match_live_log (match_id, seq, company_id, unit_id, kind, payload, text, touches, by_user_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id, created_at`,
			matchID, seq, e.CompanyID, e.UnitID, string(e.Action.Kind), payload, e.Text, touches, uid).Scan(&e.ID, &e.On)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == checkViolation {
			return core.LiveEntry{}, errors.Join(core.ErrInvalidLive, errors.New("unknown kind of action"))
		}
		if err != nil {
			return core.LiveEntry{}, err
		}
		e.Seq, e.MatchID, e.By, e.ByUserID = seq, matchID, by.DisplayName, by.ID
		out = e
	case change.Append == nil && change.Undo != 0:
		rows, err := tx.Query(ctx, `
			WITH x AS (UPDATE match_live_log SET undone_at = now(), undone_by = $3, undone_seq = $4
			            WHERE id = $1 AND match_id = $2 AND undone_at IS NULL RETURNING *)
			SELECT `+liveEntryColumns+` FROM x l LEFT JOIN users u ON u.id = l.by_user_id`, change.Undo, matchID, nilIfZero(by.ID), seq)
		if err != nil {
			return core.LiveEntry{}, err
		}
		if !rows.Next() {
			rows.Close()
			if err := rows.Err(); err != nil {
				return core.LiveEntry{}, err
			}
			return core.LiveEntry{}, core.ErrNotFound
		}
		out, err = scanEntry(rows)
		rows.Close()
		if err != nil {
			return core.LiveEntry{}, err
		}
	default:
		return core.LiveEntry{}, errors.Join(core.ErrInvalidLive, errors.New("a change is exactly one of append and undo"))
	}
	if out.Action.Kind.Secret() { // a secret objective notifies nobody: the other players' pages learn nothing before the end
		return out, tx.Commit(ctx)
	}
	companies := []int64{out.CompanyID}
	for _, c := range out.Touches {
		if !slices.Contains(companies, c) {
			companies = append(companies, c)
		}
	}
	for _, c := range companies {
		if err := notifyLive(ctx, tx, liveNotice{Match: matchID, Company: c, Seq: seq, Kind: "changed"}); err != nil {
			return core.LiveEntry{}, err
		}
	}
	return out, tx.Commit(ctx)
}

func nilIfZero(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

// Log lists the not-undone entries, newest first, below the seq `before` (0: from the newest).
func (ls liveStore) Log(ctx context.Context, campaignID, matchID, before int64, limit int) ([]core.LiveEntry, error) {
	var exists bool
	if err := ls.s.pool.QueryRow(ctx, `SELECT true FROM matches WHERE id = $1 AND campaign_id = $2`, matchID, campaignID).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
		return nil, core.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := ls.s.pool.Query(ctx, `
		SELECT `+liveEntryColumns+` FROM match_live_log l LEFT JOIN users u ON u.id = l.by_user_id
		 WHERE l.match_id = $1 AND l.undone_at IS NULL AND ($2 = 0 OR l.seq < $2) ORDER BY l.seq DESC LIMIT $3`, matchID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []core.LiveEntry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// liveFeed implements the core live feed on one connection outside the pool.
type liveFeed struct {
	url        string
	idle       time.Duration // how long to wait for a notification before pinging
	minBackoff time.Duration
	maxBackoff time.Duration
}

// LiveFeed returns the listening side of the live NOTIFY.
func (s *Store) LiveFeed() liveFeed {
	return liveFeed{url: s.url, idle: 30 * time.Second, minBackoff: time.Second, maxBackoff: 30 * time.Second}
}

// Run listens until ctx is done. It calls onResync after every (re)connect, because notifications sent meanwhile are lost.
func (f liveFeed) Run(ctx context.Context, onEvent func(core.LiveEvent), onResync func()) error {
	backoff := f.minBackoff
	for ctx.Err() == nil {
		connected, err := f.listen(ctx, onEvent, onResync)
		if ctx.Err() != nil {
			return nil
		}
		if connected {
			backoff = f.minBackoff
		}
		_ = err // the next attempt reports itself through onResync; the cause is transient
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, f.maxBackoff)
	}
	return nil
}

func (f liveFeed) listen(ctx context.Context, onEvent func(core.LiveEvent), onResync func()) (connected bool, err error) {
	cfg, err := pgx.ParseConfig(f.url)
	if err != nil {
		return false, err
	}
	cfg.RuntimeParams["application_name"] = liveFeedName
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return false, err
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		conn.Close(cctx)
	}()
	if _, err := conn.Exec(ctx, "LISTEN "+liveChannel); err != nil {
		return false, err
	}
	onResync()
	for {
		wctx, cancel := context.WithTimeout(ctx, f.idle)
		n, err := conn.WaitForNotification(wctx)
		cancel()
		switch {
		case err == nil:
			var p liveNotice
			if json.Unmarshal([]byte(n.Payload), &p) != nil || p.Match == 0 {
				continue
			}
			kind := core.LiveChanged
			if p.Kind == "ended" {
				kind = core.LiveEnded
			}
			onEvent(core.LiveEvent{MatchID: p.Match, CompanyID: p.Company, Seq: p.Seq, Kind: kind})
		case ctx.Err() != nil:
			return true, ctx.Err()
		case errors.Is(err, context.DeadlineExceeded):
			pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
			perr := conn.Ping(pctx)
			pcancel()
			if perr != nil {
				return true, perr
			}
		default:
			return true, err
		}
	}
}
