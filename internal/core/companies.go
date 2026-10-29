package core

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

// maxUnitName is the longest name a player can give a unit.
const maxUnitName = 40

// CompanyService creates companies and manages what a player decides about them without spending anything: names and the
// order of the formation. The company is a single aggregate: every change is a companyRepository.Update.
type CompanyService struct {
	companyDeps
	aftermaths aftermathRepository // optional: perks and history of the aftermaths on the company's detail
}

// NewCompanyService builds the service. rules is the loader (*Rules).
func NewCompanyService(campaigns campaignRepository, companies companyRepository, rules ruleSource) *CompanyService {
	return &CompanyService{companyDeps: companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: RandomDice{}}}
}

// WithAftermaths lets CompanyDetail show the company's perks and the history of its aftermaths.
func (s *CompanyService) WithAftermaths(r aftermathRepository) *CompanyService {
	s.aftermaths = r
	return s
}

// CompanyProfiles are the companies a player can choose in the campaign, each with everything about it.
func (s *CompanyService) CompanyProfiles(ctx context.Context, campaignID int64) ([]CompanyProfile, error) {
	rs, _, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	profiles := rs.BC.Profiles()
	for i := range profiles {
		profiles[i] = rs.withPreviewKeywords(profiles[i].WithPreviews(rs.Vanilla))
	}
	return profiles, nil
}

// CreateCompany makes the user's company in the campaign: the starting units of the chosen company with the leader and the two
// sergeants the player picked (the company's suggestion when none), its Gold and no influence. ErrCompanyLimit when the user owns
// as many companies as the campaign allows (CampaignSettings.CompanyCap), an ErrInvalidCompany when another company of theirs has
// the name (case ignored); an error wrapping ErrInvalidCompany for a bad name, an unknown company or a bad choice of officers; ErrNotFound
// when the user is not in the campaign.
func (s *CompanyService) CreateCompany(ctx context.Context, user User, campaignID int64, in CompanyInput) (CampaignCompany, error) {
	in, err := in.Validate()
	if err != nil {
		return CampaignCompany{}, err
	}
	camp, err := s.campaigns.Get(ctx, user.ID, campaignID)
	if err != nil {
		return CampaignCompany{}, err
	}
	if err := camp.CanPlay(); err != nil {
		return CampaignCompany{}, err
	}
	if len(camp.Companies) >= camp.Settings.CompanyCap() { // the repository checks again, atomically
		return CampaignCompany{}, ErrCompanyLimit
	}
	if slices.ContainsFunc(camp.Companies, func(c CampaignCompany) bool { return strings.EqualFold(c.Name, in.Name) }) {
		return CampaignCompany{}, ErrCompanyNameTaken
	}
	rs, _, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return CampaignCompany{}, err
	}
	profile, ok := rs.BC.Profile(in.Faction)
	if !ok {
		return CampaignCompany{}, errors.Join(ErrInvalidCompany, errors.New("that company is not in the campaign's rules"))
	}
	heroes, err := profile.ValidateStart(in.Heroes)
	if err != nil {
		return CampaignCompany{}, err
	}
	units := StartingUnits(profile, heroes)
	for i := range units {
		units[i].ID = 0 // the repository numbers the units
	}
	co, err := s.companies.Create(ctx, campaignID, user, NewCompany{Name: in.Name, Faction: profile.Name, Influence: rs.BC.Recruiting().StartingInfluence, Gold: profile.StartGold, Units: units, Cap: camp.Settings.CompanyCap()})
	if err != nil {
		return co, err
	}
	return s.rateOne(ctx, campaignID, co)
}

func (s *CompanyService) rateOne(ctx context.Context, campaignID int64, co CampaignCompany) (CampaignCompany, error) {
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return co, err
	}
	return s.rated(ctx, campaignID, rs, settings, co)
}

// CompanyDetail is a company with its resources, limits and units in the company's order. Anyone in the campaign may look.
func (s *CompanyService) CompanyDetail(ctx context.Context, campaignID, companyID int64) (CompanyDetail, error) {
	d, _, err := s.detail(ctx, campaignID, companyID)
	if err != nil {
		return d, err
	}
	// The state read does not count matches; the campaign's company row does (the stats cards and the rankings agree).
	if co, err := s.campaigns.Company(ctx, campaignID, companyID); err == nil {
		d.Wins, d.Draws, d.Losses = co.Wins, co.Draws, co.Losses
	}
	if s.aftermaths == nil {
		return d, nil
	}
	if d.Perks, err = s.aftermaths.Perks(ctx, campaignID, companyID); err != nil {
		return d, err
	}
	d.History, err = s.aftermaths.History(ctx, campaignID, companyID, historyLines)
	return d, err
}

// historyLines is how many lines of the aftermath history a company's detail carries.
const historyLines = 20

// RenameCompany gives the user's company a new name (1 to 60 characters).
func (s *CompanyService) RenameCompany(ctx context.Context, user User, campaignID, companyID int64, name string) error {
	in, err := CompanyInput{Name: name, Faction: "x"}.Validate()
	if err != nil {
		return Refuse("The name must be between 1 and 60 characters.")
	}
	camp, id, err := s.ownedCampaignToChange(ctx, user, campaignID, companyID)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(camp.Companies, func(c CampaignCompany) bool { return c.ID != id && strings.EqualFold(c.Name, in.Name) }) {
		return ErrCompanyNameTaken
	}
	return s.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		st.Company.Name = in.Name
		return nil
	})
}

// RenameUnit gives a unit the name the player chose (empty: back to its type); at most 40 characters.
func (s *CompanyService) RenameUnit(ctx context.Context, user User, campaignID, companyID, unitID int64, name string) error {
	name = strings.Join(strings.Fields(name), " ")
	if utf8.RuneCountInString(name) > maxUnitName {
		return Refuse("A name is at most 40 characters.")
	}
	id, err := s.ownCompanyToChange(ctx, user, campaignID, companyID)
	if err != nil {
		return err
	}
	return s.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		i := slices.IndexFunc(st.Units, func(u UnitState) bool { return u.ID == unitID })
		if i < 0 {
			return ErrNotFound
		}
		st.Units[i].Name = name
		return nil
	})
}

// SaveFormation stores the order the player put their units in. The order has to list every unit of the company exactly once:
// an empty order, a repeated id, a missing unit or an id that is not on the roster is refused.
func (s *CompanyService) SaveFormation(ctx context.Context, user User, campaignID, companyID int64, order []int64) error {
	id, err := s.ownCompanyToChange(ctx, user, campaignID, companyID)
	if err != nil {
		return err
	}
	return s.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		known := map[int64]bool{}
		for _, u := range st.Units {
			known[u.ID] = true
		}
		clean, err := cleanOrder(order, known, "in the company")
		if err != nil {
			return err
		}
		switch {
		case len(order) == 0:
			return Refuse("The formation lists no units.")
		case len(clean) != len(order):
			return Refuse("The formation lists a unit more than once.")
		case len(clean) != len(st.Units):
			return Refuse("The formation has to list all %d units of the company once; it lists %d. Reload the page and try again.", len(st.Units), len(clean))
		}
		st.Order = clean
		return nil
	})
}

// cleanOrder drops repeated ids and refuses unknown ones.
func cleanOrder(order []int64, known map[int64]bool, where string) ([]int64, error) {
	seen := map[int64]bool{}
	clean := []int64{}
	for _, id := range order {
		if !known[id] {
			return nil, Refuse("Unit %d is not %s.", id, where)
		}
		if !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	return clean, nil
}
