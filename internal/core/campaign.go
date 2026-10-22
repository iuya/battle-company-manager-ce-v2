package core

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	// ErrInvalidCampaign, ErrInvalidCompany and ErrInvalidMatch carry (joined) the reason an input was rejected.
	ErrInvalidCampaign = errors.New("invalid campaign")
	ErrInvalidCompany  = errors.New("invalid company")
	ErrInvalidMatch    = errors.New("invalid match")
	// ErrHasCompany is returned when a user already has a company in the campaign (only one is allowed).
	ErrHasCompany = errors.New("you already have a company in this campaign")
)

type CampaignStatus string

const (
	CampaignActive   CampaignStatus = "active"
	CampaignPaused   CampaignStatus = "paused"
	CampaignFinished CampaignStatus = "finished"
)

// CampaignInput is what an organiser fills in to create a campaign.
type CampaignInput struct {
	Name        string
	Description string
	// Vanilla, BattleCompanies and Glossary are the hashes of the rules files the campaign will use; empty means the
	// app's default file. The files must already be stored (see Rulesets in the web layer).
	Vanilla, BattleCompanies, Glossary string
	Settings                           CampaignSettings
}

// CampaignSettings are the options a campaign plays with. They are part of the contract between the web layer and any backend:
// the organiser chooses them when creating the campaign (CampaignInput.Settings), everybody reads them (CampaignDetail.Settings),
// nobody changes them afterwards, and the backend, not the page, enforces what they allow. The zero value is every option off.
type CampaignSettings struct {
	// HeroesDropProfileGear is the optional rule "You don't have my sword": heroes may discard any wargear, even what is
	// built into their base profile, without their rating changing.
	HeroesDropProfileGear bool
	// ShakeTheTable lets players throw the dice in their browser and press the space bar to shake the table. The browser
	// then decides the roll (client authoritative), so it can be cheated: an organiser switches it on for a group that trusts
	// each other. Off, the server rolls and the animation only shows its result.
	ShakeTheTable bool
	// ManualRolls lets players type in a roll they made with physical dice ("I rolled a 4"). Off, every roll panel has only
	// a Roll button and the server rolls. Like ShakeTheTable it takes the player's word for the result.
	ManualRolls bool
	// CustomOnTheirOwnPath gives the units that are On Their Own Path the homebrew "Path of Their Own Making": they start on it
	// as heroes with 0 Might, 0 Will and 1 Fate, earn experience and level up. Off, they never level up.
	CustomOnTheirOwnPath bool
}

// RollRefusal says why a roll the player brought along is not taken, or "" when it is: a result typed in needs ManualRolls and
// one the browser threw needs ShakeTheTable. A backend applies it to every roll it receives.
func (s CampaignSettings) RollRefusal(entered, thrown bool) string {
	switch {
	case thrown && !s.ShakeTheTable:
		return "This campaign does not let players throw the dice in their browser."
	case entered && !thrown && !s.ManualRolls:
		return "This campaign does not let players enter their own rolls: use the Roll button."
	}
	return ""
}

// Validate trims the input and returns it, or an error wrapping ErrInvalidCampaign that is fit to show to the user.
func (in CampaignInput) Validate() (CampaignInput, error) {
	in.Name, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > 60 {
		return in, errors.Join(ErrInvalidCampaign, errors.New("the name must be between 1 and 60 characters"))
	}
	if utf8.RuneCountInString(in.Description) > 280 {
		return in, errors.Join(ErrInvalidCampaign, errors.New("the description can have at most 280 characters"))
	}
	return in, nil
}

// CampaignSummary is a campaign as one of its players sees it in their list.
type CampaignSummary struct {
	ID          int64
	Name        string
	Description string
	Status      CampaignStatus
	Round       int              // the round being played; 0 before the first one
	Rounds      int              // planned rounds; 0 when the campaign is open ended
	Matches     int              // matches played so far
	Company     *CampaignCompany // the user's company, nil until they create one
}

// CampaignCompany is a battle company inside a campaign. A user has at most one per campaign.
type CampaignCompany struct {
	ID      int64
	Name    string
	Faction string
	OwnerID int64
	Owner   string // the player's display name
	Points  int    // sum of the point costs of its units that are not injured: this is what the ranking sorts by
	// FullPoints is what Points would be if nobody were injured; zero means the same as Points. The front shows "Points (FullPoints)".
	FullPoints int
	Units      int
	Wins       int
	Draws      int
	Losses     int
}

func (c CampaignCompany) Played() int { return c.Wins + c.Draws + c.Losses }

// CampaignDetail is a campaign as one of its players sees it on its own page.
type CampaignDetail struct {
	CampaignSummary // Company is the viewer's company
	Organiser       string
	OrganiserID     int64
	Players         int
	Rulesets        []RulesetInfo // the vanilla, Battle Companies and glossary files this campaign is played with
	Settings        CampaignSettings
	// What the Manage campaign dialogs say before the organiser pauses, ends or invalidates: invitation links that still work,
	// matches not ended, and companies with an aftermath to finish.
	ActiveInvites, OpenMatches, PendingAftermaths int
}

// CampaignMember is a member of a campaign, with or without a company.
type CampaignMember struct {
	UserID      int64
	Name        string
	Organiser   bool
	CompanyID   int64 // 0: the member has no company yet
	CompanyName string
}

// CanManage says whether u manages this campaign: an administrator or the organiser.
func (d CampaignDetail) CanManage(u User) bool { return CanManageCampaign(u, d.OrganiserID) }

// CanManageCampaign says whether the user manages the campaign whose organiser is organiserID: administrators, superusers and the
// organiser (the creator unless handed over). Within the campaign the organiser can do what an administrator can; the Administration
// section stays with administrators and superusers.
func CanManageCampaign(u User, organiserID int64) bool {
	return u.Role == RoleAdmin || u.Role == RoleSuperuser || (organiserID != 0 && u.ID == organiserID)
}

// CompanyInput is what a player fills in to create their company.
type CompanyInput struct {
	Name    string
	Faction string
	Heroes  []StartHero // the leader and the two sergeants among the starting units; empty: the company's suggestion
}

func (in CompanyInput) Validate() (CompanyInput, error) {
	in.Name, in.Faction = strings.TrimSpace(in.Name), strings.TrimSpace(in.Faction)
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > 60 {
		return in, errors.Join(ErrInvalidCompany, errors.New("the name must be between 1 and 60 characters"))
	}
	if in.Faction == "" {
		return in, errors.Join(ErrInvalidCompany, errors.New("choose a faction"))
	}
	return in, nil
}

// RankedCompany is a row of the rankings.
type RankedCompany struct {
	Rank int
	CampaignCompany
}

// Rank sorts companies by points (highest first, then name) and numbers them; equal points share a rank
// ("1, 2, 2, 4").
func Rank(companies []CampaignCompany) []RankedCompany {
	sorted := slices.Clone(companies)
	slices.SortFunc(sorted, func(a, b CampaignCompany) int {
		if a.Points != b.Points {
			return b.Points - a.Points
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	out := make([]RankedCompany, len(sorted))
	for i, c := range sorted {
		out[i] = RankedCompany{Rank: i + 1, CampaignCompany: c}
		if i > 0 && c.Points == sorted[i-1].Points {
			out[i].Rank = out[i-1].Rank
		}
	}
	return out
}

const (
	maxSides       = 8
	maxSideNameLen = 40
)

// MatchSide is one side of a match: a name ("Side 1" unless the players chose another) and its companies.
type MatchSide struct {
	Name      string
	Companies []CampaignCompany
	// Role is the tag of the team in a scenario with roles: "", RoleAttacker or RoleDefender. Every company of the side has it. It is
	// kept when the scenario has no roles but is ignored then.
	Role string
}

// The tags of a side in a scenario with an Attacker and a Defender.
const (
	RoleAttacker = "attacker"
	RoleDefender = "defender"
)

// ValidRole says whether s is a side tag (the empty tag is none).
func ValidRole(s string) bool { return s == "" || s == RoleAttacker || s == RoleDefender }

// RoleOf is the tag of the side the company plays on ("" for none or an unknown company).
func (m Match) RoleOf(companyID int64) string {
	for _, s := range m.Sides {
		for _, c := range s.Companies {
			if c.ID == companyID {
				return s.Role
			}
		}
	}
	return ""
}

var defaultSideName = regexp.MustCompile(`^Side [0-9]+$`)

// Label is what the pages call the side: the players' name, or, for a side left at its default name ("Side 2"), its
// companies ("Rangers + Raiders"), which says more than a number.
func (s MatchSide) Label() string {
	if !defaultSideName.MatchString(s.Name) || len(s.Companies) == 0 {
		return s.Name
	}
	names := make([]string, len(s.Companies))
	for i, c := range s.Companies {
		names[i] = c.Name
	}
	return strings.Join(names, " + ")
}

// Match is a game between two or more sides, each with one or more companies.
type Match struct {
	ID       int64
	Number   int // 1, 2, 3... within the campaign
	PlayedOn time.Time
	Ended    bool
	Winner   int // 1-based side that won; 0 is a draw once the match has ended
	Sides    []MatchSide

	Scenario *MatchScenario // nil until chosen
	Result   MatchResult    // zero value while nothing is reported
	Revision int            // bumped by every write; writes carry the one the form was rendered with
	EndedOn  *time.Time
	// History is who ended and reopened the match and when, oldest first (the repository's Events); empty for a match never ended.
	History []MatchEvent
	// EndProposal is the pending end (docs/matches.md, "Ending a match: confirmation by all participants"); nil when there is none. The
	// repository stores it with the match. EndDetail is write-only: the service sets it when it ends the match, and the repository
	// writes it as the Detail of the "ended" event.
	EndProposal *EndProposal
	EndDetail   string
}

// Tone says how side i (0-based) came out: "win", "loss", "draw", or "" while the match is open.
func (m Match) Tone(i int) string {
	switch {
	case !m.Ended:
		return ""
	case m.Winner == 0:
		return "draw"
	case m.Winner == i+1:
		return "win"
	}
	return "loss"
}

// WinnerName is the name of the winning side, empty for an open match or a draw.
func (m Match) WinnerName() string {
	if m.Ended && m.Winner >= 1 && m.Winner <= len(m.Sides) {
		return m.Sides[m.Winner-1].Label()
	}
	return ""
}

// NewSide is a side of a match being created: an optional name and the ids of its companies.
type NewSide struct {
	Name      string
	Companies []int64
	Role      string // optional tag: "", "attacker" or "defender"
}

// ValidateSides trims and names the sides of a new match ("Side N" when blank) and checks them: at least two sides,
// every side with a company, and no company twice (so never on both sides at once).
func ValidateSides(sides []NewSide) ([]NewSide, error) {
	if len(sides) < 2 || len(sides) > maxSides {
		return nil, errors.Join(ErrInvalidMatch, errors.New("a match needs between 2 and 8 sides"))
	}
	out := make([]NewSide, len(sides))
	seen := map[int64]bool{}
	roles := map[string]bool{}
	for i, side := range sides {
		if !ValidRole(side.Role) {
			return nil, errors.Join(ErrInvalidMatch, errors.New("a side is Attacker or Defender, or neither"))
		}
		if side.Role != "" {
			if roles[side.Role] {
				return nil, errors.Join(ErrInvalidMatch, errors.New("only one side can be the "+side.Role))
			}
			roles[side.Role] = true
		}
		side.Name = strings.TrimSpace(side.Name)
		if utf8.RuneCountInString(side.Name) > maxSideNameLen {
			return nil, errors.Join(ErrInvalidMatch, errors.New("a side name can have at most 40 characters"))
		}
		if side.Name == "" {
			side.Name = "Side " + strconv.Itoa(i+1)
		}
		if len(side.Companies) == 0 {
			return nil, errors.Join(ErrInvalidMatch, errors.New(side.Name+" needs at least one company"))
		}
		for _, id := range side.Companies {
			if seen[id] {
				return nil, errors.Join(ErrInvalidMatch, errors.New("a company cannot appear twice in a match"))
			}
			seen[id] = true
		}
		out[i] = side
	}
	return out, nil
}
