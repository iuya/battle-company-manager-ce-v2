package core

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// Limits of one organiser grant (docs/matches.md, "Organiser and scenario rewards").
const (
	maxGrantNote   = 200
	maxGrantGold   = 1000
	maxGrantInflu  = 100
	maxGrantExperi = 100
)

// GrantInput is what the organiser fills in to award a company. Amounts may be negative to correct a mistake. Experience goes
// to the unit UnitID and needs it.
type GrantInput struct {
	Gold, Influence int
	Experience      int
	UnitID          int64
	Note            string
}

// Validate trims the note and checks the input; a rejection wraps ErrInvalidCompany and its message is fit to show. The unit
// is dropped when no experience is given.
func (in GrantInput) Validate() (GrantInput, error) {
	in.Note = strings.TrimSpace(in.Note)
	var errs []error
	if n := utf8.RuneCountInString(in.Note); n == 0 || n > maxGrantNote {
		errs = append(errs, errors.New("the note must be between 1 and 200 characters"))
	}
	if in.Gold == 0 && in.Influence == 0 && in.Experience == 0 {
		errs = append(errs, errors.New("give some Gold, Influence or Experience"))
	}
	if abs(in.Gold) > maxGrantGold {
		errs = append(errs, errors.New("Gold must be between -1000 and 1000"))
	}
	if abs(in.Influence) > maxGrantInflu {
		errs = append(errs, errors.New("Influence must be between -100 and 100"))
	}
	if abs(in.Experience) > maxGrantExperi {
		errs = append(errs, errors.New("Experience must be between -100 and 100"))
	}
	if in.Experience != 0 && in.UnitID == 0 {
		errs = append(errs, errors.New("choose the unit that gets the experience"))
	}
	if in.Experience == 0 {
		in.UnitID = 0
	}
	if len(errs) > 0 {
		return in, errors.Join(append([]error{ErrInvalidCompany}, errs...)...)
	}
	return in, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// CanGrant is nil when u may award companies of this campaign, or ErrRefused with the reason: only the organiser and
// administrators may, and not once the campaign is finished. Every backend applies it to GrantReward; the page uses it to
// show the form.
func (d CampaignDetail) CanGrant(u User) error {
	switch {
	case !d.CanManage(u):
		return Refuse("Only the organiser of the campaign can grant rewards.")
	case d.Status == CampaignFinished:
		return Refuse("The campaign is finished.")
	}
	return nil
}

// ErrCampaignPaused is the refusal of what a paused campaign does not take: new matches and new players (open matches can be finished).
var ErrCampaignPaused = Refuse("This campaign is paused.")

// CanStartMatch is nil while the campaign is active: a paused campaign finishes its open matches but starts no new one, a finished
// one is read only.
func (d CampaignDetail) CanStartMatch() error {
	switch d.Status {
	case CampaignPaused:
		return ErrCampaignPaused
	case CampaignFinished:
		return Refuse("The campaign is finished.")
	}
	return nil
}

// CanPlay is nil while the campaign takes changes to its matches and companies (a paused campaign does; CanStartMatch is stricter), or
// ErrRefused when it is finished (the ranking is final). Matches (scenario, result, end, reopen), the live mode and the aftermath apply it.
func (d CampaignDetail) CanPlay() error {
	if d.Status == CampaignFinished {
		return Refuse("The campaign is finished.")
	}
	return nil
}
