package core

import (
	"context"
	"errors"
	"time"
)

//go:generate go tool mockgen -typed -source=match_repository.go -destination=mock_match_repository_test.go -package=core

// MatchEvent is an entry of a match's history: who ended or reopened it, and when.
type MatchEvent struct {
	Kind   string // "ended" or "reopened"
	By     string // display name
	On     time.Time
	Detail string // "ended": who confirmed ("confirmed by Hador, Gorgoroth") or the override ("overriding Gorgoroth: note")
}

// GrantKind says where an award came from.
type GrantKind string

const (
	GrantOrganiser GrantKind = "organiser"
	GrantScenario  GrantKind = "scenario"
)

// Grant is one entry of a company's history of awards (docs/matches.md, "Organiser and scenario rewards"). Gold, Influence
// and Experience may be negative to correct a mistake. Experience goes to UnitID and needs it. MatchID is 0 for a free grant.
type Grant struct {
	ID              int64
	CompanyID       int64
	Kind            GrantKind
	Gold, Influence int
	Experience      int
	UnitID          int64
	MatchID         int64
	Note            string // required for an organiser grant, at most 200 characters
	By              string // display name of who granted it; empty for the system
	On              time.Time
}

// matchRepository keeps matches, their results and the grants of awards. It stores; the rules (Match.Access, CanEnd,
// ValidateResult, the revision check) are the service's, applied inside Update where the match is locked. A match belongs to a
// campaign: ids that do not match are ErrNotFound.
type matchRepository interface {
	// List returns the campaign's matches, newest first (highest number first), with sides, companies, scenario and result.
	List(ctx context.Context, campaignID int64) ([]Match, error)
	// Get returns one match, or ErrNotFound.
	Get(ctx context.Context, campaignID, matchID int64) (Match, error)
	// Events is the match's history of ending and reopening, oldest first.
	Events(ctx context.Context, campaignID, matchID int64) ([]MatchEvent, error)
	// Create stores an open match for the (already validated, see ValidateSides) sides, numbered after the campaign's last
	// one, at revision 1. ErrNotFound when the creator is not a member of the campaign or a company is not one of its
	// companies; ErrInvalidMatch when a company appears twice.
	Create(ctx context.Context, campaignID int64, by User, sides []NewSide) (Match, error)
	// Update runs fn on the match inside one transaction holding a row lock on it and stores what fn leaves behind when it
	// returns nil (the error is returned as is when it does not, and nothing is stored). fn is where the service checks the
	// access and the revision. What is stored: EndProposal (nil deletes it; an ended match has none), Scenario, Result (Winner and Decided, Note, VictoryPoints per side, Fled and
	// Objectives per company; Result.Sides must be empty or as long as Sides) and Ended. The sides, their companies, Number and
	// PlayedOn never change, except that each side's Role may (the unique index allows one Attacker and one Defender; a swap is stored). The revision is bumped by one; EndedOn is set by the database when Ended turns true and cleared
	// when it turns false, and each of those transitions adds a MatchEvent by `by` (an "ended" one carries Match.EndDetail). The stored match is returned.
	Update(ctx context.Context, campaignID, matchID int64, by User, fn func(*Match) error) (Match, error)

	// AddGrant stores the grant and applies it to the company in one transaction: Gold and Influence are added to the company
	// and Experience to the unit's XP. ErrNotFound for an unknown company or match in the campaign, or a unit that is not on
	// the company's roster. ErrNegativeBalance when Gold or Influence would drop below zero (nothing is stored).
	AddGrant(ctx context.Context, campaignID int64, by User, g Grant) (Grant, error)
	// Grants is the company's history, newest first.
	Grants(ctx context.Context, campaignID, companyID int64) ([]Grant, error)
}

// ErrNegativeBalance is returned by AddGrant when the company would be left with less than 0 Gold or Influence.
var ErrNegativeBalance = errors.New("the company would be left with less than 0 Gold or Influence")
