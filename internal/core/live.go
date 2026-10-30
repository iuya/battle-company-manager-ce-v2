package core

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Live match mode: the contract between the web layer, the live service and the Postgres adapter.
// Rules and the design are in docs/live-match.md and docs/architecture.md ("Live match"). This file holds declarations only;
// the fold of the log, Apply, Undo and the hub are implemented in live_fold.go, live_service.go and live_hub.go.

//go:generate go tool mockgen -typed -source=live.go -destination=mock_live_test.go -package=core

// Zone says where a unit is during the match.
type Zone string

const (
	ZoneField     Zone = "field"
	ZoneRetreated Zone = "retreated"
	ZoneCasualty  Zone = "casualty"
)

// LiveStat is a tracker: Current within 0..Start. Start 0 means the unit has no such tracker.
type LiveStat struct{ Current, Start int }

// LiveUnit is the state of one unit in the match: a pure function of its LiveUnitInit and the not-undone entries.
type LiveUnit struct {
	UnitID                    int64
	Zone                      Zone
	Reason                    string // "Failed courage", "Left the board", "Wounded by X (Company)", "Died: note", "Wounds lowered to 0"
	Wounds, Might, Will, Fate LiveStat
	HasMount, Mounted         bool
	MountWounds               LiveStat       // Start 1 unless the mount's profile says more
	Gains                     map[string]int // experience earned so far by campaign.experience.gains key (flags 0 or 1)
	Objective                 bool           // got the objective (an end check, not in Gains: the aftermath adds it)
	// MatchXP is the experience earned so far, at the file's amounts: taking part, Gains and the objective. The MVP, Against the Odds and
	// the scenario rewards are only known at the end. Set by the service (0 without a rules file); the card shows it ahead of the bar.
	MatchXP int
	// FateEntry and BrokenEntry are the unit's last not-undone fate and broken entries (0 for none): what pressing a lit Fate or Courage
	// milestone undoes.
	FateEntry, BrokenEntry int64
	// BreakModels is how many models the unit counts as for the company's Break Point (at least 1). CourageTurn is the turn its courage
	// was last resolved (a broken, courageSkip or failed-courage retreat entry), StandFastTurn the turn it called Stand Fast. Temporary
	// is set for the temporary Wanderer of Against the Odds (its UnitID is TemporaryUnitID(company), never a roster id).
	BreakModels, CourageTurn, StandFastTurn int
	Temporary                               bool
	// CourageHow is how the courage of CourageTurn was resolved: "passed" (tested), "failed" (tested, retreated), or the courageSkip reason
	// (standFast, heroicResolve, chariot, scenario: automatic passes; engaged, cannotActivate: exemptions). Empty before any.
	CourageHow string
	// Loaned are the items a Tool Kit put on the unit for this match (Used when an itemUsed entry spent it); UsedItems the names of the
	// owned items it used.
	Loaned    []LoanedItem
	UsedItems []string
}

// TemporaryUnitID is the id of the temporary Wanderer a company took with Against the Odds: negative, so that it never meets a roster id,
// and one per company.
func TemporaryUnitID(companyID int64) int64 { return -companyID }

// LiveUnitInit is what is stored once when the match opens (or lazily at the first read of a match that was open before the
// feature): the starting values of a unit. Absent units (Injured, AtHealers) take no part and have no entries.
type LiveUnitInit struct {
	UnitID, CompanyID         int64
	Wounds, Might, Will, Fate int
	HasMount                  bool
	MountWounds               int  // 0 when the mount has no counter of its own (one wound)
	CountsAsKill              bool // a Hero or Monster: slaying it earns slainHeroOrMonster
	Absent                    bool
	Temporary                 bool // the temporary Wanderer: added by the service at read time, never stored
}

// LiveCompany is what a company page needs.
type LiveCompany struct {
	MatchID, CompanyID int64
	MatchNumber        int
	Seq                int64 // last sequence of the match; the page shows it so a tab can ignore its own events
	Units              []LiveUnit
	Absent             []int64
	Access             LiveAccess
	Break              LiveBreak          // the company's Break Point state
	Wanderer           *TemporaryWanderer // the temporary Wanderer this company took with Against the Odds, nil without
	Turn               int                // match level: the last started turn, 0 before the first
}

// LiveAccess is what the viewer may do with a company's live state; the page only hides controls with it.
type LiveAccess int

const (
	LiveNone LiveAccess = iota // read only
	LiveEdit
)

// LiveAccessFor: the owner of the company, the campaign organiser and administrators edit while the match is open.
// A finished campaign is checked by the service.
func LiveAccessFor(u User, m Match, c CampaignCompany, organiserID int64) LiveAccess {
	if m.Ended {
		return LiveNone
	}
	if CanManageCampaign(u, organiserID) || (c.OwnerID != 0 && c.OwnerID == u.ID) {
		return LiveEdit
	}
	return LiveNone
}

// LiveKind is the kind of an action.
type LiveKind string

const (
	LiveStatChange LiveKind = "stat"      // a tracker changed by hand
	LiveWound      LiveKind = "wound"     // wounded one or more units
	LiveFate       LiveKind = "fate"      // spent Fate to prevent a wound
	LiveBroken     LiveKind = "broken"    // passed a Broken courage test
	LiveRetreat    LiveKind = "retreat"   // left the table
	LiveDied       LiveKind = "died"      // died without an enemy wound
	LiveMount      LiveKind = "mount"     // mount removed or re-added
	LiveReturn     LiveKind = "return"    // back to the battle from Retreated or Field casualties (Wounds 0 becomes 1)
	LiveObjective  LiveKind = "objective" // a unit got (or no longer has) the objective at the end of the game
	LiveMVPVote    LiveKind = "mvpVote"   // a player votes a unit of an opposing company as MVP (UnitID 0 withdraws)
	// The secret objectives of Fog of War: kept by their author and hidden from everybody else until the match ends.
	LiveSecretNominate LiveKind = "secretNominate" // what the player noted down at the start (a Hero, a terrain piece)
	LiveSecretDone     LiveKind = "secretDone"     // the tier the player reports at the end

	// The company Break Point in play and the Against the Odds items (epic 36y; docs/live-match.md).
	LiveTurn        LiveKind = "turn"        // a new turn starts (Turn); match level, company 0
	LiveBreakAdjust LiveKind = "breakAdjust" // the manual correction of a company's starting models or casualties (Field, Delta, Note)
	LiveCourageSkip LiveKind = "courageSkip" // a Broken company's unit passes or is exempt from the courage test without a roll (UnitID, Reason)
	LiveAllIsLost   LiveKind = "allIsLost"   // a Broken company flees the battlefield (Flee true) or fights on (false)
	LiveItemUsed    LiveKind = "itemUsed"    // a loaned or owned consumable used during the game (UnitID, Item, Index)

	// Informational entries about a pending end of the match (epic 6uh): written by the app, shown in the log, never undone and never
	// part of the fold.
	LiveEndProposed  LiveKind = "endProposed"
	LiveEndConfirmed LiveKind = "endConfirmed"
	LiveEndWithdrawn LiveKind = "endWithdrawn"
	LiveEndCancelled LiveKind = "endCancelled"
	LiveEndOverride  LiveKind = "endOverride"
)

// Informational says whether entries of the kind only tell what happened to a pending end: they change nothing in the state.
func (k LiveKind) Informational() bool {
	switch k {
	case LiveEndProposed, LiveEndConfirmed, LiveEndWithdrawn, LiveEndCancelled, LiveEndOverride:
		return true
	}
	return false
}

// EndCheckKind says whether entries of the kind belong to the end checks (the objectives, the MVP votes and the secret objectives): they are
// made when the game is over and do not cancel a pending end of the match.
func (k LiveKind) EndCheckKind() bool {
	return k == LiveObjective || k == LiveMVPVote || k.Secret() || k.Informational()
}

// Secret says whether entries of the kind are hidden from other players while the match is open.
func (k LiveKind) Secret() bool { return k == LiveSecretNominate || k == LiveSecretDone }

// LiveAction is one thing the player did. Fields by Kind:
//
//	stat:    UnitID, Stat ("wounds"|"might"|"will"|"fate"), Delta (non-zero) or Set; Set is stored as the Delta it means when applied
//	wound:   UnitID (the attacker), Targets (at least one)
//	fate:    UnitID, Count (0 to its current Fate; 0 only marks the Fate milestone, no Fate is spent)
//	broken:  UnitID
//	retreat: UnitID, Why ("courage"|"left"), Note
//	died:    UnitID, Note
//	mount:   UnitID, Mounted (the new value)
//	return:  UnitID
//	objective: UnitID, On (true: got the objective; false: no longer)
//	mvpVote: TargetCompanyID (the company voted on), UnitID (a unit of it; 0 withdraws the voter's vote); the voter is the entry's author
type LiveAction struct {
	Kind    LiveKind
	UnitID  int64
	Stat    string
	Delta   int
	Set     *int
	Targets []WoundTarget
	Count   int
	Why     string
	Note    string // 0-200 characters
	Mounted bool
	On      bool
	// ObjectiveKey and Tier belong to the secret objectives: the scoring objective's key, and the tier reached (1-based; 0 clears).
	// A secretNominate names the nominated Hero in UnitID, or a terrain piece in Note (UnitID 0); both empty clears the note.
	ObjectiveKey string
	Tier         int
	// TargetCompanyID is the company an mvpVote is about.
	TargetCompanyID int64
	// Turn is the number of the turn a turn entry starts (1 for the first). Field is "start" or "casualties" for a breakAdjust.
	// StandFast is set on a broken entry when the Hero called Stand Fast. Reason is a courageSkip's reason (standFast|heroicResolve|
	// chariot|scenario|engaged|cannotActivate). Flee is an allIsLost's answer. Item and Index name the consumable of an itemUsed.
	Turn      int
	Field     string
	StandFast bool
	Reason    string
	Flee      bool
	Item      string
	Index     int
}

// WoundTarget is one unit wounded by a declaration.
type WoundTarget struct {
	UnitID      int64
	Part        string // "rider" (default) or "mount"
	Caused      int    // 1-9
	SavedByFate int    // 0..min(Caused, the target's Fate); not for a mount
}

// LiveEntry is a line of the log. Seq is per match and also bumped by an undo (UndoneSeq), so a stream sees every change.
type LiveEntry struct {
	ID, Seq   int64
	MatchID   int64
	CompanyID int64   // the acting company
	UnitID    int64   // the acting unit
	Touches   []int64 // other companies whose units changed
	Action    LiveAction
	Text      string // fit to show, built by the service
	By        string // display name
	ByUserID  int64
	On        time.Time
	CanUndo   bool // for the viewer asking; set by the service
}

// LiveEvent is what the Postgres NOTIFY carries (see the hub): never state, never HTML.
type LiveEvent struct {
	MatchID, CompanyID, Seq int64
	Kind                    LiveEventKind
}

// LiveEventKind: Changed (one company, or CompanyID 0 for the whole match after a resync), Ended (the match was ended or reopened:
// the page reloads into the view it has now).
type LiveEventKind string

const (
	LiveChanged LiveEventKind = "changed"
	LiveEnded   LiveEventKind = "ended"
)

// LiveBatch is what a subscriber receives: events coalesced since it last looked, so a slow client never has a backlog.
type LiveBatch struct {
	Seq       int64   // highest seq seen
	Companies []int64 // companies to refetch; all of the match's companies after a resync
	Match     bool    // something changed in the match (the log card refetches)
	Ended     bool
}

// LiveStream is one subscriber's view of a match: Next blocks until something changed (or ctx is done) and returns the coalesced
// batch; Close unsubscribes (idempotent). Returned by LiveService.Subscribe, implemented by the hub.
type LiveStream interface {
	Next(ctx context.Context) (LiveBatch, error)
	Close()
}

// Errors of the live mode. ErrRefused (not an editor, match ended, unit not in the Field, no Fate left, a tracker out of range)
// and ErrNotFound are the shared ones.
var (
	// ErrInvalidLive is a bad action (joined, fit to show).
	ErrInvalidLive = errors.New("invalid live action")
	// ErrLiveDepends is returned (wrapping ErrRefused) when undoing an entry would make a later one impossible.
	ErrLiveDepends = fmt.Errorf("%w: a later change depends on it; undo that first", ErrRefused)
)

// OpenMatchRef names the open match a company is in.
type OpenMatchRef struct {
	MatchID int64
	Number  int
}

// CompanyBusyError is why a match cannot be created (or reopened): a company is already in another open match. It is an
// ErrRefused and reads "Company X is already in Match N."; the web layer links Match N with errors.As.
type CompanyBusyError struct {
	Company string
	OpenMatchRef
}

func (e *CompanyBusyError) Error() string {
	return fmt.Sprintf("%s is already in Match %d.", e.Company, e.Number)
}

func (e *CompanyBusyError) Is(target error) bool { return target == ErrRefused }

// LockMessageOpenMatch is the refusal of a roster action while the company's match is running.
func LockMessageOpenMatch(m OpenMatchRef) string {
	return fmt.Sprintf("Match %d is running; finish it first.", m.Number)
}

// LiveSnapshot is everything the service folds: read in one transaction.
type LiveSnapshot struct {
	MatchID     int64
	Number      int
	Ended       bool
	Seq         int64
	Initialised bool
	Units       []LiveUnitInit
	Entries     []LiveEntry // not undone, ascending Seq
}

// LiveChange is what the service decides inside LiveRepository.Update: exactly one of Append (Seq, ID, On and By are set by the
// repository) or Undo (an entry id).
type LiveChange struct {
	Append *LiveEntry
	Undo   int64
}

// liveRepository keeps the live state of matches. It stores; the rules (fold, validation, access) are the service's.
// A match belongs to a campaign: ids that do not match are ErrNotFound.
type liveRepository interface {
	// Init stores the units once and creates the match's counter row; it does nothing when already initialised.
	Init(ctx context.Context, campaignID, matchID int64, units []LiveUnitInit) error
	// Read returns the snapshot without locking.
	Read(ctx context.Context, campaignID, matchID int64) (LiveSnapshot, error)
	// Update locks the match row (FOR NO KEY UPDATE, so it serialises with ending the match and with other live writes),
	// reads the snapshot, runs fn and applies its change: it assigns the next Seq (an undo also gets one, UndoneSeq), stores the
	// entry or marks the entry undone (undone_by is `by`) and emits one NOTIFY on `live_match` per touched company plus the
	// acting one, all in the same transaction, so listeners hear it after the commit only. It returns the stored entry
	// (for an undo, the entry that was undone). The error of fn is returned as is and nothing is stored.
	Update(ctx context.Context, campaignID, matchID int64, by User, fn func(LiveSnapshot) (LiveChange, error)) (LiveEntry, error)
	// Log lists not-undone entries, newest first, with Seq below `before` (0: from the newest).
	Log(ctx context.Context, campaignID, matchID, before int64, limit int) ([]LiveEntry, error)
	// UnitLog lists a unit's not-undone entries of one kind, newest first.
	UnitLog(ctx context.Context, campaignID, matchID, unitID int64, kind LiveKind) ([]LiveEntry, error)
}

// liveFeed is the listening side of the NOTIFY: one dedicated connection, outside the pool, reconnecting with backoff. It calls
// onEvent for every notification and onResync after every (re)connect, because notifications sent meanwhile are lost.
// Run returns when ctx is done.
type liveFeed interface {
	Run(ctx context.Context, onEvent func(LiveEvent), onResync func()) error
}

// CourageRoll is a Courage test rolled for a unit of a Broken company: the dice, their total, the unit's Courage and whether it passed.
// Nothing is written: the player records the result with Apply (broken or retreat).
type CourageRoll struct {
	Dice    []int
	Total   int
	Courage int
	Passed  bool
}

// LiveMatchDefaults are what the live log says about the result form of the match: which companies chose to flee (All is Lost) and
// for which companies every opposing company is Broken at the end of the game (the enemyBroken scenario objective).
type LiveMatchDefaults struct {
	Fled        map[int64]bool
	EnemyBroken map[int64]bool
}

// TemporaryWanderer describes the temporary Wanderer of a company (the unit itself is among LiveCompany.Units, Temporary true).
type TemporaryWanderer struct {
	UnitID  int64
	Name    string
	Summary string // a one-line profile
	Rating  int    // what it cost in Against the Odds Rating
	Courage int
}
