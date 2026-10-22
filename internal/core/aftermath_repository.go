package core

import (
	"context"
	"time"
)

//go:generate go tool mockgen -typed -source=aftermath_repository.go -destination=mock_aftermath_repository_test.go -package=core

// AftermathWork is what the aftermath keeps between two requests besides the sheet: the roster the sheet was written for, the
// casualties and the answers given. It is stored with the record (match_aftermaths.state) and is private to AftermathService.
type AftermathWork struct {
	Roster      []CompanyUnit     // the roster at Start
	Absent      []int64           // units that missed the game and recovered at Start
	Undo        map[int64][2]bool // absent unit -> Injured, SentToHealers before Start (for ReopenSheet)
	PrevRerolls int               // the company's Rerolls before Start
	Casualties  []Casualty        // one per unit with outcome casualty
	Second      map[int64]bool    // casualties on their second roll after a Scratch
	Rerolls     int               // Against the Odds re-rolls left
	Continued   bool              // the player moved on with re-rolls left
	Applied     bool              // the injuries' effects are applied
	LeaderFell  bool              // the Leader died or retired
	NoSergeant  bool              // the player chose not to fill the Sergeant place
	// SergeantsToFill counts the Sergeants who died or retired in this aftermath and have not been replaced yet. While it is above
	// 0 and no Hero is left, a Warrior must take the place (Hero in the Making); with a Hero available the place stays optional.
	SergeantsToFill int
	Answers         map[string]string // reward answers: "reward:<key>" -> "<item>|<unit id>"
	WaivedNote      string
}

// AftermathRecord is the stored aftermath of one company in one match. A company with nothing saved has the zero record with
// Status pending and Revision 0.
type AftermathRecord struct {
	MatchID, CompanyID int64
	MatchEnded         bool // read under the lock: the match cannot be reopened while the record is being changed
	Status             AftermathStatus
	Revision           int
	Input              AftermathInput
	Work               AftermathWork
	Lines              []AftermathLine
	StartedAt          *time.Time
	FinishedAt         *time.Time

	// To store with the record when fn succeeds (write only; the service has already applied them to the company state):
	NewGrants []Grant
	NewPerks  []Perk
}

// Perk is a lasting reward of a scenario (a Rendezvous bonus, a Vantage Point, A Final Blessing) kept by the company until used.
type Perk struct {
	ID, MatchID int64
	UnitID      int64  // the relic bearer; 0 for the company
	Effect      string // campaign.aftermathEffects key
	Name        string
	Text        string
	When        string // the data's when: "firstReinforcementRollThisAftermath" | "nextScenarioTableRoll" | "nextGame" (older rows: "nextReinforcementRoll" | "nextScenarioRoll")
	UsedOn      *time.Time
}

// aftermathRepository keeps the War's Aftermath of the companies. Update is the transaction of every step: it locks the match
// (shared) and the company row, so the company state and the record change together or not at all.
type aftermathRepository interface {
	// Get returns the record (the zero record when nothing is stored), or ErrNotFound when the company is not in the match.
	Get(ctx context.Context, campaignID, matchID, companyID int64) (AftermathRecord, error)
	// Update runs fn on the company's state and the record in one transaction and stores both when fn returns nil. NewGrants are
	// inserted as history only (the company state already carries their effect) and NewPerks are inserted.
	Update(ctx context.Context, campaignID, matchID, companyID int64, fn func(*CompanyState, *AftermathRecord) error) error
	// Statuses lists the companies of the match with their status ("" while the match is open, pending when nothing is stored).
	Statuses(ctx context.Context, campaignID, matchID int64) ([]AftermathRef, error)
	// Pending lists the aftermaths of ended matches the company still has to finish (pending, sheet, injuries, levelups), oldest first.
	Pending(ctx context.Context, campaignID, companyID int64) ([]AftermathRef, error)
	// Started says whether any company of the match has started its aftermath (or has it waived): the match cannot be reopened.
	Started(ctx context.Context, campaignID, matchID int64) (bool, error)
	// Perks lists the company's perks, unused first.
	Perks(ctx context.Context, campaignID, companyID int64) ([]Perk, error)
	// UsePerk marks an unused perk of the company used; ErrNotFound for an unknown perk, ErrRefused if it was used already.
	UsePerk(ctx context.Context, campaignID, companyID, perkID int64) error
	// History is the latest lines of the company's aftermaths, newest aftermath first, at most limit lines.
	History(ctx context.Context, campaignID, companyID int64, limit int) ([]AftermathLine, error)
}
