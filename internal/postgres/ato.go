package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// atoStore implements the core Against the Odds repository; Store.ATO returns it.
type atoStore struct{ s *Store }

// ATO returns the Against the Odds repository.
func (s *Store) ATO() atoStore { return atoStore{s} }

// atoSelectionJSON is the stored shape of a core.ATOSelection, versioned like the other jsonb columns.
type atoSelectionJSON struct {
	V        int                 `json:"v"`
	Bonuses  []string            `json:"bonuses,omitempty"`
	Kits     []string            `json:"kits,omitempty"`
	Items    []atoAssignmentJSON `json:"items,omitempty"`
	Wanderer string              `json:"wanderer,omitempty"`
	Ambush   string              `json:"ambush,omitempty"`
}

type atoAssignmentJSON struct {
	Kit    string `json:"kit"`
	Item   string `json:"item"`
	Index  int    `json:"index,omitempty"`
	Unit   int64  `json:"unit,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type atoScenarioJSON struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Victory     string `json:"victory,omitempty"`
}

func encodeSelection(s core.ATOSelection) ([]byte, error) {
	j := atoSelectionJSON{V: 1, Bonuses: s.Bonuses, Kits: s.Kits, Wanderer: s.Wanderer, Ambush: s.Ambush}
	for _, a := range s.Items {
		j.Items = append(j.Items, atoAssignmentJSON{Kit: a.Kit, Item: a.Item, Index: a.Index, Unit: a.UnitID, Detail: a.Detail})
	}
	return json.Marshal(j)
}

func decodeSelection(raw []byte) (core.ATOSelection, error) {
	var j atoSelectionJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return core.ATOSelection{}, err
	}
	s := core.ATOSelection{Bonuses: j.Bonuses, Kits: j.Kits, Wanderer: j.Wanderer, Ambush: j.Ambush}
	for _, a := range j.Items {
		s.Items = append(s.Items, core.KitAssignment{Kit: a.Kit, Item: a.Item, Index: a.Index, UnitID: a.Unit, Detail: a.Detail})
	}
	return s, nil
}

// read loads the record of a match (the zero record when none is stored).
func (as atoStore) read(ctx context.Context, q querier, matchID int64) (core.ATORecord, error) {
	var companyID int64
	var had bool
	var sel, prev []byte
	err := q.QueryRow(ctx, `SELECT company_id, selection, previous_scenario, had_previous FROM match_ato WHERE match_id = $1`, matchID).
		Scan(&companyID, &sel, &prev, &had)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ATORecord{}, nil
	}
	if err != nil {
		return core.ATORecord{}, err
	}
	return as.decode(matchID, companyID, sel, prev, had)
}

// decode turns the columns of a stored record into it.
func (as atoStore) decode(matchID, companyID int64, sel, prev []byte, hadPrevious bool) (core.ATORecord, error) {
	rec := core.ATORecord{CompanyID: companyID, HadPrevious: hadPrevious, MatchID: matchID}
	var err error
	if rec.Selection, err = decodeSelection(sel); err != nil {
		return core.ATORecord{}, err
	}
	if prev != nil {
		var p atoScenarioJSON
		if err := json.Unmarshal(prev, &p); err != nil {
			return core.ATORecord{}, err
		}
		rec.Previous = &core.MatchScenario{Kind: core.ScenarioKind(p.Kind), Name: p.Name, Description: p.Description, Victory: p.Victory}
	}
	return rec, nil
}

// Get returns the stored record, or the zero record; ErrNotFound when the match is not one of the campaign. One statement: the match row
// proves the match belongs to the campaign, the record joins to it.
func (as atoStore) Get(ctx context.Context, campaignID, matchID int64) (core.ATORecord, error) {
	var companyID *int64
	var sel, prev []byte
	var had *bool
	err := as.s.pool.QueryRow(ctx, `SELECT a.company_id, a.selection, a.previous_scenario, a.had_previous
		FROM matches m LEFT JOIN match_ato a ON a.match_id = m.id WHERE m.id = $1 AND m.campaign_id = $2`, matchID, campaignID).
		Scan(&companyID, &sel, &prev, &had)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.ATORecord{}, core.ErrNotFound
	}
	if err != nil {
		return core.ATORecord{}, err
	}
	if companyID == nil { // no record stored
		return core.ATORecord{}, nil
	}
	return as.decode(matchID, *companyID, sel, prev, had != nil && *had)
}

func gameStarted(ctx context.Context, q querier, matchID int64) (bool, error) {
	var started bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM match_live_log WHERE match_id = $1 AND undone_at IS NULL)`, matchID).Scan(&started)
	return started, err
}

// GameStarted says whether a live entry that has not been undone exists.
func (as atoStore) GameStarted(ctx context.Context, campaignID, matchID int64) (bool, error) {
	var started bool
	err := as.s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM match_live_log WHERE match_id = m.id AND undone_at IS NULL)
		FROM matches m WHERE m.id = $1 AND m.campaign_id = $2`, matchID, campaignID).Scan(&started)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, core.ErrNotFound
	}
	return started, err
}

// Update runs fn under a lock on the match and stores the scenario and the record it leaves behind (an empty selection deletes the row);
// the objectives reported belong to the scenario, so they go when it changes. The match revision goes up by one.
func (as atoStore) Update(ctx context.Context, campaignID, matchID int64, by core.User, fn func(*core.ATOState) error) (core.Match, error) {
	ms := as.s.Matches()
	tx, err := as.s.pool.Begin(ctx)
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
	st := core.ATOState{Match: before}
	if st.Record, err = as.read(ctx, tx, matchID); err != nil {
		return core.Match{}, err
	}
	if st.Started, err = gameStarted(ctx, tx, matchID); err != nil {
		return core.Match{}, err
	}
	if err := fn(&st); err != nil {
		return core.Match{}, err
	}
	var kind *string
	var sc core.MatchScenario
	if st.Match.Scenario != nil {
		k := string(st.Match.Scenario.Kind)
		kind, sc = &k, *st.Match.Scenario
	}
	if _, err := tx.Exec(ctx, `
		UPDATE matches SET scenario_kind = $2, scenario_name = $3, scenario_description = $4, scenario_victory = $5, revision = revision + 1
		 WHERE id = $1`, matchID, kind, sc.Name, sc.Description, sc.Victory); err != nil {
		return core.Match{}, err
	}
	if !reflect.DeepEqual(before.EndProposal, st.Match.EndProposal) { // a change of the selection resets the confirmations of a pending end
		if err := storeEndProposal(ctx, tx, matchID, by, st.Match.EndProposal); err != nil {
			return core.Match{}, err
		}
		if err := notifyLive(ctx, tx, liveNotice{Match: matchID, Kind: "changed"}); err != nil {
			return core.Match{}, err
		}
	}
	if !sameScenario(before.Scenario, st.Match.Scenario) {
		if _, err := tx.Exec(ctx, `DELETE FROM match_objectives WHERE match_id = $1`, matchID); err != nil {
			return core.Match{}, err
		}
	}
	if st.Record.Selection.Empty() {
		if _, err := tx.Exec(ctx, `DELETE FROM match_ato WHERE match_id = $1`, matchID); err != nil {
			return core.Match{}, err
		}
	} else {
		sel, err := encodeSelection(st.Record.Selection)
		if err != nil {
			return core.Match{}, err
		}
		var prev []byte
		if p := st.Record.Previous; p != nil {
			if prev, err = json.Marshal(atoScenarioJSON{Kind: string(p.Kind), Name: p.Name, Description: p.Description, Victory: p.Victory}); err != nil {
				return core.Match{}, err
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO match_ato (match_id, company_id, selection, previous_scenario, had_previous)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (match_id) DO UPDATE SET company_id = EXCLUDED.company_id, selection = EXCLUDED.selection,
			       previous_scenario = EXCLUDED.previous_scenario, had_previous = EXCLUDED.had_previous,
			       revision = match_ato.revision + 1, updated_at = now()`,
			matchID, st.Record.CompanyID, sel, prev, st.Record.HadPrevious); err != nil {
			return core.Match{}, err
		}
	}
	list, err = ms.load(ctx, tx, `m.id = $1`, false, matchID)
	if err != nil || len(list) == 0 {
		return core.Match{}, err
	}
	return list[0], tx.Commit(ctx)
}

func sameScenario(a, b *core.MatchScenario) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
