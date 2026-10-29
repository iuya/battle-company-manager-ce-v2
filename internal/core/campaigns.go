package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// ruleSource gives the rules a campaign is played with. *Rules satisfies it.
type ruleSource interface {
	For(ctx context.Context, h RulesetHashes) (*RuleSet, error)
}

// companyDeps is what every service that works on companies shares: the repositories and the rules loader. The helpers below
// are the common first steps of a use case (find the campaign's rules, build a company's detail, find the user's company).
type companyDeps struct {
	campaigns campaignRepository
	companies companyRepository
	rules     ruleSource
	dice      Dice
}

// ruleset returns the rules and the settings of a campaign.
func (d companyDeps) ruleset(ctx context.Context, campaignID int64) (*RuleSet, CampaignSettings, error) {
	h, settings, err := d.campaigns.Hashes(ctx, campaignID)
	if err != nil {
		return nil, settings, err
	}
	rs, err := d.rules.For(ctx, h)
	return rs, settings, err
}

// detail builds the page data of a company (ErrNotFound when the campaign has no such company).
func (d companyDeps) detail(ctx context.Context, campaignID, companyID int64) (CompanyDetail, *RuleSet, error) {
	rs, settings, err := d.ruleset(ctx, campaignID)
	if err != nil {
		return CompanyDetail{}, nil, err
	}
	st, err := d.companies.Read(ctx, campaignID, companyID)
	if err != nil {
		return CompanyDetail{}, nil, err
	}
	return BuildDetail(rs, settings, st), rs, nil
}

// ownCompany returns the id of the company companyID when the user owns it in the campaign: the one helper that resolves and
// authorises every owner action. ErrNotFound when the campaign is unknown to them or the company is not theirs (another player's,
// another campaign's, or the organiser probing: a stranger learns nothing).
func (d companyDeps) ownCompany(ctx context.Context, user User, campaignID, companyID int64) (int64, error) {
	camp, err := d.campaigns.Get(ctx, user.ID, campaignID)
	if err != nil {
		return 0, err
	}
	co, ok := camp.Own(companyID)
	if !ok {
		return 0, ErrNotFound
	}
	return co.ID, nil
}

// ownCompanyToChange is ownCompany for a use case that changes the company: a finished campaign is frozen (matches, companies and
// grants stay as they were until an administrator reactivates it), so every write is refused with the same reason.
func (d companyDeps) ownCompanyToChange(ctx context.Context, user User, campaignID, companyID int64) (int64, error) {
	_, id, err := d.ownedCampaignToChange(ctx, user, campaignID, companyID)
	return id, err
}

// ownedCampaignToChange is ownCompanyToChange that also returns the campaign as the user sees it (their other companies included).
func (d companyDeps) ownedCampaignToChange(ctx context.Context, user User, campaignID, companyID int64) (CampaignDetail, int64, error) {
	camp, err := d.campaigns.Get(ctx, user.ID, campaignID)
	if err != nil {
		return camp, 0, err
	}
	co, ok := camp.Own(companyID)
	if !ok {
		return camp, 0, ErrNotFound
	}
	if err := camp.CanPlay(); err != nil {
		return camp, 0, err
	}
	return camp, co.ID, nil
}

// rated fills the rating figures of a company from its rules: the repositories cannot, prices live in the rules files.
func (d companyDeps) rated(ctx context.Context, campaignID int64, rs *RuleSet, settings CampaignSettings, co CampaignCompany) (CampaignCompany, error) {
	st, err := d.companies.Read(ctx, campaignID, co.ID)
	if err != nil {
		return co, err
	}
	b := companyFigures(rs, settings, st)
	co.Points, co.FullPoints, co.Units, co.FactionLabel = b.Points, b.FullPoints, b.Units, b.FactionLabel
	return co, nil
}

// rosterOfReader is implemented by a repository that can read the rosters of some companies by id, whichever campaigns they are in, in one
// statement. A page that rates a few companies (the player's own on the campaign list, the companies of one match) then does not read, build
// and discard the whole campaign.
type rosterOfReader interface {
	ReadRosterOf(ctx context.Context, companyIDs []int64) ([]CompanyState, error)
}

// roster reads the states of the companies ids names (all of the campaign when ids is empty).
func (d companyDeps) roster(ctx context.Context, campaignID int64, ids []int64) ([]CompanyState, error) {
	if len(ids) > 0 {
		if r, ok := d.companies.(rosterOfReader); ok {
			sts, err := r.ReadRosterOf(ctx, ids)
			if !errors.Is(err, errRosterOfUnsupported) {
				return sts, err
			}
		}
	}
	return d.companies.ReadRoster(ctx, campaignID)
}

// figuresOf rates states by company id, only the Points, FullPoints, Units and FactionLabel fields filled. want, when not nil, picks the
// companies to build.
func figuresOf(states []CompanyState, rs *RuleSet, settings CampaignSettings, want func(companyID int64) bool) map[int64]CampaignCompany {
	out := make(map[int64]CampaignCompany, len(states))
	for _, st := range states {
		if want != nil && !want(st.Company.ID) { // building a company is the cost: only those asked for
			continue
		}
		out[st.Company.ID] = companyFigures(rs, settings, st)
	}
	return out
}

// figures rates the companies of the campaign with a single read of the rosters (rated costs a read per company); the result is by
// company id. want, when not nil, picks the companies to build; ids, when not empty, are the only ones that are read at all (a superset of
// what want accepts).
func (d companyDeps) figures(ctx context.Context, campaignID int64, rs *RuleSet, settings CampaignSettings, want func(companyID int64) bool, ids []int64) (map[int64]CampaignCompany, error) {
	states, err := d.roster(ctx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	return figuresOf(states, rs, settings, want), nil
}

// ratedAll fills the rating figures of the companies given (of one campaign) from one read of their rosters. whole says that the list is
// every company of the campaign: the campaign's roster is then read as a whole, which the other readers of one request share.
func (d companyDeps) ratedAll(ctx context.Context, campaignID int64, rs *RuleSet, settings CampaignSettings, list []CampaignCompany, whole bool) error {
	var ids []int64
	if !whole {
		ids = make([]int64, len(list))
		for i := range list {
			ids[i] = list[i].ID
		}
	}
	f, err := d.figures(ctx, campaignID, rs, settings, nil, ids)
	if err != nil {
		return err
	}
	return fill(f, list)
}

func fill(f map[int64]CampaignCompany, list []CampaignCompany) error {
	for i := range list {
		b, ok := f[list[i].ID]
		if !ok {
			return ErrNotFound
		}
		list[i].Points, list[i].FullPoints, list[i].Units, list[i].FactionLabel = b.Points, b.FullPoints, b.Units, b.FactionLabel
	}
	return nil
}

// Campaigns lists, creates and shows campaigns and their companies. It implements those methods of web.campaigns over the
// campaign repository; the rating figures of the companies come from the company aggregate and the rules.
type Campaigns struct {
	companyDeps
	rulesets rulesetRepository
	log      *slog.Logger
}

// NewCampaigns builds the service. rules is the loader (*Rules).
func NewCampaigns(campaigns campaignRepository, companies companyRepository, rulesets rulesetRepository, rules ruleSource, log *slog.Logger) *Campaigns {
	if log == nil {
		log = slog.Default()
	}
	return &Campaigns{companyDeps: companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: RandomDice{}}, rulesets: rulesets, log: log}
}

// List returns the user's campaigns, unfinished first, each group newest first, with the user's company rated.
func (c *Campaigns) List(ctx context.Context, userID int64, withFinished bool) ([]CampaignSummary, error) {
	list, err := c.campaigns.List(ctx, userID, withFinished)
	if err != nil {
		return nil, err
	}
	// the player's own companies of every campaign are read in one statement when the repository can
	var all []int64
	for i := range list {
		for _, co := range list[i].Companies {
			all = append(all, co.ID)
		}
	}
	var states map[int64]CompanyState
	if r, ok := c.companies.(rosterOfReader); ok && len(all) > 0 {
		if sts, err := r.ReadRosterOf(ctx, all); errors.Is(err, errRosterOfUnsupported) {
			states = nil
		} else if err != nil {
			c.log.Warn("cannot read the companies' rosters", "err", err) // each campaign then tries on its own
		} else {
			states = make(map[int64]CompanyState, len(sts))
			for _, st := range sts {
				states[st.Company.ID] = st
			}
		}
	}
	for i := range list {
		if len(list[i].Companies) == 0 {
			continue
		}
		rs, settings, err := c.ruleset(ctx, list[i].ID)
		if err == nil && states != nil {
			own := make([]CompanyState, 0, len(list[i].Companies))
			for _, co := range list[i].Companies {
				if st, ok := states[co.ID]; ok {
					own = append(own, st)
				}
			}
			err = fill(figuresOf(own, rs, settings, nil), list[i].Companies)
		} else if err == nil {
			err = c.ratedAll(ctx, list[i].ID, rs, settings, list[i].Companies, false)
		}
		if err != nil { // one campaign whose rules cannot be loaded must not hide the others
			c.log.Warn("cannot rate the companies", "campaign", list[i].ID, "err", err)
		}
	}
	return list, nil
}

// CampaignNav is a campaign as the sidebar lists it.
type CampaignNav struct {
	ID     int64
	Name   string
	Status CampaignStatus
}

// campaignNavLister is implemented by a repository that can list the sidebar's campaigns in one statement.
type campaignNavLister interface {
	Nav(ctx context.Context, userID int64, withFinished bool) ([]CampaignNav, error)
}

// NavCampaigns lists the user's campaigns for the sidebar (same order as List) without rating anyone's company: the sidebar shows only
// the name and status. A repository with a one-statement Nav answers from it; otherwise the plain list is mapped (still without rules).
func (c *Campaigns) NavCampaigns(ctx context.Context, userID int64, withFinished bool) ([]CampaignNav, error) {
	if nl, ok := c.campaigns.(campaignNavLister); ok {
		return nl.Nav(ctx, userID, withFinished)
	}
	list, err := c.campaigns.List(ctx, userID, withFinished)
	if err != nil {
		return nil, err
	}
	out := make([]CampaignNav, len(list))
	for i, g := range list {
		out[i] = CampaignNav{ID: g.ID, Name: g.Name, Status: g.Status}
	}
	return out, nil
}

// Create makes the user the organiser of a new campaign. Only users with CanCreateCampaigns (or admins) may (ErrRefused). An
// empty hash means the app's default file of that kind; a hash that is not stored or is of another kind is ErrInvalidCampaign.
func (c *Campaigns) Create(ctx context.Context, owner User, in CampaignInput) (CampaignSummary, error) {
	if !owner.MayCreateCampaigns() {
		return CampaignSummary{}, Refuse("You may not create campaigns.")
	}
	in, err := in.Validate()
	if err != nil {
		return CampaignSummary{}, err
	}
	nc := NewCampaign{Name: in.Name, Description: in.Description, Settings: in.Settings}
	for _, pick := range []struct {
		kind RulesetKind
		hash string
		set  *string
	}{{KindVanilla, in.Vanilla, &nc.Vanilla}, {KindBattleCompanies, in.BattleCompanies, &nc.BattleCompanies}, {KindGlossary, in.Glossary, &nc.Glossary}} {
		var info RulesetInfo
		if pick.hash == "" {
			info, err = c.rulesets.Default(ctx, pick.kind)
		} else {
			info, err = c.rulesets.Info(ctx, pick.hash)
		}
		switch {
		case errors.Is(err, ErrNotFound):
			return CampaignSummary{}, errors.Join(ErrInvalidCampaign, errors.New("the "+pick.kind.Label()+" file is not stored"))
		case err != nil:
			return CampaignSummary{}, err
		case info.Kind != pick.kind:
			return CampaignSummary{}, errors.Join(ErrInvalidCampaign, errors.New("that file is not a "+pick.kind.Label()+" file"))
		}
		*pick.set = info.Hash
	}
	return c.campaigns.Create(ctx, owner, nc)
}

// Get returns the campaign as one of its players sees it, with their company rated; ErrNotFound for non-members.
func (c *Campaigns) Get(ctx context.Context, userID, id int64) (CampaignDetail, error) {
	d, err := c.campaigns.Get(ctx, userID, id)
	if err != nil || len(d.Companies) == 0 {
		return d, err
	}
	rs, settings, err := c.ruleset(ctx, id)
	if err != nil {
		return d, err
	}
	if err := c.ratedAll(ctx, id, rs, settings, d.Companies, false); err != nil {
		return d, err
	}
	return d, nil
}

// Companies lists the campaign's companies with their rating figures.
func (c *Campaigns) Companies(ctx context.Context, id int64) ([]CampaignCompany, error) {
	list, err := c.campaigns.Companies(ctx, id)
	if err != nil || len(list) == 0 {
		return list, err
	}
	rs, settings, err := c.ruleset(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := c.ratedAll(ctx, id, rs, settings, list, true); err != nil {
		return nil, err
	}
	return list, nil
}

// Company returns one company of the campaign with its rating figures.
func (c *Campaigns) Company(ctx context.Context, id, companyID int64) (CampaignCompany, error) {
	co, err := c.campaigns.Company(ctx, id, companyID)
	if err != nil {
		return co, err
	}
	rs, settings, err := c.ruleset(ctx, id)
	if err != nil {
		return co, err
	}
	return c.rated(ctx, id, rs, settings, co)
}

// SetCompaniesPerPlayer changes how many companies each player may have (1 to 4) at any time, up or down. The organiser and
// administrators may (ErrRefused otherwise, and for a finished campaign); n out of range is ErrInvalidCampaign. Lowering touches no
// company: players above the new limit keep theirs and cannot create more.
func (c *Campaigns) SetCompaniesPerPlayer(ctx context.Context, actor User, campaignID int64, n int) error {
	if n < 1 || n > MaxCompaniesPerPlayer {
		return errors.Join(ErrInvalidCampaign, fmt.Errorf("a player can have between 1 and %d companies", MaxCompaniesPerPlayer))
	}
	return c.campaigns.UpdateSettings(ctx, campaignID, func(organiserID int64, status CampaignStatus, s CampaignSettings) (CampaignSettings, error) {
		if !CanManageCampaign(actor, organiserID) {
			return s, Refuse("Only the organiser of the campaign or an administrator can change this.")
		}
		if status == CampaignFinished {
			return s, Refuse("The campaign has ended.")
		}
		s.CompaniesPerPlayer = n
		return s, nil
	})
}

// Settings returns the settings the campaign is played with; only CompaniesPerPlayer changes after creation.
func (c *Campaigns) Settings(ctx context.Context, id int64) (CampaignSettings, error) {
	_, settings, err := c.campaigns.Hashes(ctx, id)
	return settings, err
}

// Members lists everyone in the campaign, also the members without a company: the organiser first, then by name. The handover
// form offers the ones who are not the organiser. ErrNotFound for an unknown campaign; who may see it is the caller's decision.
func (c *Campaigns) Members(ctx context.Context, id int64) ([]CampaignMember, error) {
	return c.campaigns.Members(ctx, id)
}

// HandOver makes another member the organiser of the campaign; the previous organiser becomes a player. Only the
// organiser and administrators (admin or superuser) may; the new organiser must be a member. ErrRefused with the reason
// otherwise, ErrNotFound for an unknown campaign.
func (c *Campaigns) HandOver(ctx context.Context, actor User, campaignID, newOrganiserID int64) error {
	return c.campaigns.HandOver(ctx, campaignID, newOrganiserID, func(current int64) error {
		if !CanManageCampaign(actor, current) {
			return Refuse("Only the organiser can hand the campaign over")
		}
		if newOrganiserID == current {
			return Refuse("That player already organises this campaign")
		}
		return nil
	})
}

// SetCampaignStatus pauses, resumes, ends or reactivates a campaign. The organiser and administrators pause (active to paused), resume
// (paused to active) and end (active or paused to finished; allowed with open matches, the page warns first); only administrators
// reactivate a finished campaign (an ended campaign is ended for good for the organiser). Ending needs
// the campaign's name as confirm (the page asks for it after its warnings) and revokes every invitation link; the matches stay exactly
// as they are, frozen. ErrRefused with the reason otherwise.
func (c *Campaigns) SetCampaignStatus(ctx context.Context, actor User, campaignID int64, to CampaignStatus, confirm string) error {
	return c.campaigns.SetStatus(ctx, campaignID, func(organiserID int64, name string, from CampaignStatus) (CampaignStatus, error) {
		if !CanManageCampaign(actor, organiserID) {
			return from, Refuse("Only the organiser of the campaign or an administrator can change its status.")
		}
		switch {
		case to != CampaignActive && to != CampaignPaused && to != CampaignFinished:
			return from, Refuse("Unknown campaign status.")
		case to == from:
			return from, Refuse("The campaign is already %s.", from)
		case from == CampaignFinished && (actor.Role != RoleAdmin && actor.Role != RoleSuperuser):
			return from, Refuse("Only an administrator can reactivate a finished campaign.")
		case from == CampaignFinished && to == CampaignPaused:
			return from, Refuse("Reactivate the campaign first.")
		}
		if to == CampaignFinished && strings.TrimSpace(confirm) != name {
			return from, Refuse("Type the name of the campaign to end it.")
		}
		return to, nil
	})
}
