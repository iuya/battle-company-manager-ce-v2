package postgres

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The archive of an ended campaign (docs/archive.md): one gzipped document in campaign_archives, read back as a core.Archive and served in
// place of the rows it replaced. The reads that need it ask only when the rows they would have read are gone (an empty list, an unknown
// match) or the row says so (companies.archived_at), so a campaign that is not archived pays nothing for it.

// archiveStore implements the core archive repository; Store.Archives returns it.
type archiveStore struct{ s *Store }

// Archives returns the repository of the archives of ended campaigns.
func (s *Store) Archives() archiveStore { return archiveStore{s} }

// archiveCache keeps the most recently read archives decoded: they never change, and a page reads its campaign several times.
type archiveCache struct {
	mu  sync.Mutex
	max int
	m   map[int64]*list.Element
	lru *list.List // of *cachedArchive, most recently used first
}

type cachedArchive struct {
	id int64
	a  *core.Archive
}

func newArchiveCache(max int) *archiveCache {
	return &archiveCache{max: max, m: map[int64]*list.Element{}, lru: list.New()}
}

func (c *archiveCache) get(id int64) *core.Archive {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.m[id]; ok {
		c.lru.MoveToFront(e)
		return e.Value.(*cachedArchive).a
	}
	return nil
}

func (c *archiveCache) put(id int64, a *core.Archive) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.m[id]; ok {
		c.lru.MoveToFront(e)
		return
	}
	c.m[id] = c.lru.PushFront(&cachedArchive{id, a})
	for c.lru.Len() > c.max {
		last := c.lru.Back()
		c.lru.Remove(last)
		delete(c.m, last.Value.(*cachedArchive).id)
	}
}

// maxCachedArchives is how many decoded archives stay in memory (a few hundred KB each).
const maxCachedArchives = 8

// archiveOf returns the decoded archive of a campaign, or core.ErrNotFound when it has none.
func (s *Store) archiveOf(ctx context.Context, campaignID int64) (*core.Archive, error) {
	if a := s.archives.get(campaignID); a != nil {
		return a, nil
	}
	var packed []byte
	err := s.pool.QueryRow(ctx, `SELECT document FROM campaign_archives WHERE campaign_id = $1`, campaignID).Scan(&packed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a, err := core.UnpackArchive(packed)
	if err != nil {
		return nil, fmt.Errorf("archive of campaign %d: %w", campaignID, err)
	}
	s.archives.put(campaignID, &a)
	return &a, nil
}

// isArchived says whether the campaign has been archived. Used where the rows an answer would come from may be gone.
func (s *Store) isArchived(ctx context.Context, campaignID int64) (bool, error) {
	var archived bool
	err := s.pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM campaigns WHERE id = $1`, campaignID).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return archived, err
}

// archivedFiles are the three files an archived campaign was played with, as the Overview lists them.
func (s *Store) archivedFiles(ctx context.Context, campaignID int64) ([]core.RulesetInfo, error) {
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT files FROM campaign_archives WHERE campaign_id = $1`, campaignID).Scan(&raw); err != nil {
		return nil, err
	}
	var out []core.RulesetInfo
	return out, json.Unmarshal(raw, &out)
}

// The three pruned files of an archive are served by the ruleset repository under hashes of their own, "archive:<campaign>:<kind>", so the
// loader that builds a rule set from three stored files builds the rule set of an archived campaign from its document, unchanged.
const archiveHashPrefix = "archive:"

func archiveHashes(campaignID int64) core.RulesetHashes {
	h := func(k core.RulesetKind) string {
		return archiveHashPrefix + strconv.FormatInt(campaignID, 10) + ":" + string(k)
	}
	return core.RulesetHashes{Vanilla: h(core.KindVanilla), BattleCompanies: h(core.KindBattleCompanies), Glossary: h(core.KindGlossary)}
}

// archivedRuleset returns the pruned file named by a synthetic hash; ok is false for a hash that is not one.
func (s *Store) archivedRuleset(ctx context.Context, hash string) (core.StoredRuleset, bool, error) {
	rest, ok := strings.CutPrefix(hash, archiveHashPrefix)
	if !ok {
		return core.StoredRuleset{}, false, nil
	}
	idText, kind, ok := strings.Cut(rest, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if !ok || err != nil {
		return core.StoredRuleset{}, true, core.ErrNotFound
	}
	a, err := s.archiveOf(ctx, id)
	if err != nil {
		return core.StoredRuleset{}, true, err
	}
	rk := core.RulesetKind(kind)
	content := a.Rules.File(rk)
	if content == nil {
		return core.StoredRuleset{}, true, core.ErrNotFound
	}
	r := core.StoredRuleset{Size: len(content), Content: content}
	r.Kind, r.Hash, r.Name = rk, hash, a.Name
	for _, f := range a.Files {
		if f.Kind == rk {
			r.Name, r.Version = f.Name, f.Version
		}
	}
	return r, true, nil
}

// Header reads the campaign row without a viewer.
func (cs campaignStore) Header(ctx context.Context, campaignID int64) (core.CampaignHeader, error) {
	var h core.CampaignHeader
	var status string
	var raw []byte
	var v, b, g *string
	err := cs.s.pool.QueryRow(ctx, `
		SELECT id, name, description, status, round, rounds, settings, vanilla_hash, bc_hash, glossary_hash, archived_at IS NOT NULL
		  FROM campaigns WHERE id = $1`, campaignID).
		Scan(&h.ID, &h.Name, &h.Description, &status, &h.Round, &h.Rounds, &raw, &v, &b, &g, &h.Archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, core.ErrNotFound
	}
	if err != nil {
		return h, err
	}
	h.Status = core.CampaignStatus(status)
	if h.Settings, err = decodeSettings(raw); err != nil {
		return h, err
	}
	if h.Archived {
		h.Hashes = archiveHashes(campaignID)
	} else if v != nil && b != nil && g != nil {
		h.Hashes = core.RulesetHashes{Vanilla: *v, BattleCompanies: *b, Glossary: *g}
	}
	return h, nil
}

// notFoundOrArchived is the answer for a row that is not there: ErrArchived when the campaign has been archived (its matches and live data
// are in the document and are not changed), else ErrNotFound.
func (s *Store) notFoundOrArchived(ctx context.Context, campaignID int64) error {
	if archived, err := s.isArchived(ctx, campaignID); err == nil && archived {
		return core.ErrArchived
	}
	return core.ErrNotFound
}

// campaignArchived is Header's archived flag for a campaign locked in a transaction, with the lock's own answer.
func campaignArchived(ctx context.Context, tx pgx.Tx, campaignID int64) (bool, error) {
	var archived bool
	err := tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM campaigns WHERE id = $1`, campaignID).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, core.ErrNotFound
	}
	return archived, err
}

// refusedAsArchived turns the error of a trigger of the database (SQLSTATE P0480) into core.ErrArchived.
func refusedAsArchived(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == archivedViolation {
		return core.ErrArchived
	}
	return err
}

const archivedViolation = "P0480"

// Save archives a finished campaign: see core.archiveRepository.
func (as archiveStore) Save(ctx context.Context, campaignID int64, packed []byte, meta core.ArchiveMeta) error {
	files, err := json.Marshal(meta.Files)
	if err != nil {
		return err
	}
	tx, err := as.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	var archived bool
	err = tx.QueryRow(ctx, `SELECT status, archived_at IS NOT NULL FROM campaigns WHERE id = $1 FOR UPDATE`, campaignID).Scan(&status, &archived)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return core.ErrNotFound
	case err != nil:
		return err
	case archived:
		return core.ErrArchived
	case status != string(core.CampaignFinished):
		return core.Refuse("Only an ended campaign is archived.")
	}
	// the figures the lists show, taken from the rows that are about to go
	var matches, practice int
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE NOT practice), count(*) FILTER (WHERE practice) FROM matches WHERE campaign_id = $1`, campaignID).Scan(&matches, &practice); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE companies c SET archived_units  = (SELECT count(*) FROM company_units cu WHERE cu.company_id = c.id),
		       archived_wins   = (SELECT count(*) FROM match_side_companies mc JOIN matches m ON m.id = mc.match_id
		                           WHERE mc.company_id = c.id AND m.ended AND NOT m.practice AND m.winner = mc.position + 1),
		       archived_draws  = (SELECT count(*) FROM match_side_companies mc JOIN matches m ON m.id = mc.match_id
		                           WHERE mc.company_id = c.id AND m.ended AND NOT m.practice AND m.winner = 0),
		       archived_losses = (SELECT count(*) FROM match_side_companies mc JOIN matches m ON m.id = mc.match_id
		                           WHERE mc.company_id = c.id AND m.ended AND NOT m.practice AND m.winner <> 0 AND m.winner <> mc.position + 1)
		 WHERE c.campaign_id = $1`, campaignID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO campaign_archives (campaign_id, format, raw_size, packed_size, companies, matches, files, document)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		campaignID, meta.Format, meta.RawSize, len(packed), meta.Companies, meta.Matches, files, packed); err != nil {
		return err
	}
	// what the document replaces: the matches (their sides, objectives, history, live data, aftermaths and Against the Odds records go
	// with them), then what hangs from the companies, then the invitation links
	for _, q := range []string{
		`DELETE FROM matches WHERE campaign_id = $1`,
		`DELETE FROM grants WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`,
		`DELETE FROM company_perks WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`,
		`DELETE FROM fallen_units WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`,
		`DELETE FROM company_units WHERE company_id IN (SELECT id FROM companies WHERE campaign_id = $1)`,
		`DELETE FROM campaign_invites WHERE campaign_id = $1`,
	} {
		if _, err := tx.Exec(ctx, q, campaignID); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	}
	// the companies are marked last: from then on the database refuses any change to them
	if _, err := tx.Exec(ctx, `UPDATE companies SET archived_at = now() WHERE campaign_id = $1`, campaignID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE campaigns SET archived_at = now(), archived_matches = $2, archived_practice = $3,
		       vanilla_hash = NULL, bc_hash = NULL, glossary_hash = NULL
		 WHERE id = $1`, campaignID, matches, practice); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	as.s.hashes.Delete(campaignID) // the remembered files are not the campaign's any more
	return nil
}

// Unarchived lists the finished campaigns that have no archive, oldest first.
func (as archiveStore) Unarchived(ctx context.Context) ([]int64, error) {
	rows, err := as.s.pool.Query(ctx, `SELECT id FROM campaigns WHERE status = 'finished' AND archived_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
