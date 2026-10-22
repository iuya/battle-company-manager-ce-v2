package core

import "context"

//go:generate go tool mockgen -typed -source=ato_repository.go -destination=mock_ato_repository_test.go -package=core

// ATOState is what a save of the Against the Odds card sees under one lock: the match (its scenario is what Ambush! changes), the stored
// selection and whether the game has begun (a live entry exists that has not been undone).
type ATOState struct {
	Match   Match
	Record  ATORecord
	Started bool
}

// atoRepository keeps the Against the Odds selection of a match (one row per match: the lower company took it). The live service and the
// aftermath read it; Matches saves it together with the scenario Ambush! sets.
type atoRepository interface {
	// Get returns the stored record, or the zero record (CompanyID 0, empty selection) when none is stored. ErrNotFound when the
	// match is not one of the campaign.
	Get(ctx context.Context, campaignID, matchID int64) (ATORecord, error)
	// GameStarted says whether a live entry that has not been undone exists for the match (the card is then locked).
	GameStarted(ctx context.Context, campaignID, matchID int64) (bool, error)
	// Update runs fn under a lock on the match and stores what it leaves behind: Match.Scenario and the record (an empty selection
	// deletes the row). The match revision is bumped by one. The stored match is returned (sides without ratings).
	Update(ctx context.Context, campaignID, matchID int64, by User, fn func(*ATOState) error) (Match, error)
}
