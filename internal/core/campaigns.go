package core

import (
	"context"
	"errors"
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

// myCompany returns the id of the company the user owns in the campaign; ErrNotFound when the campaign is unknown to them
// or they have no company in it (so a stranger learns nothing).
func (d companyDeps) myCompany(ctx context.Context, user User, campaignID int64) (int64, error) {
	camp, err := d.campaigns.Get(ctx, user.ID, campaignID)
	if err != nil {
		return 0, err
	}
	if camp.Company == nil {
		return 0, ErrNotFound
	}
	return camp.Company.ID, nil
}

// myCompanyToChange is myCompany for a use case that changes the company: a finished campaign is frozen (matches, companies and
// grants stay as they were until an administrator reactivates it), so every write is refused with the same reason.
func (d companyDeps) myCompanyToChange(ctx context.Context, user User, campaignID int64) (int64, error) {
	camp, err := d.campaigns.Get(ctx, user.ID, campaignID)
	if err != nil {
		return 0, err
	}
	if camp.Company == nil {
		return 0, ErrNotFound
	}
	if err := camp.CanPlay(); err != nil {
		return 0, err
	}
	return camp.Company.ID, nil
}

// rated fills the rating figures of a company from its rules: the repositories cannot, prices live in the rules files.
func (d companyDeps) rated(ctx context.Context, campaignID int64, rs *RuleSet, settings CampaignSettings, co CampaignCompany) (CampaignCompany, error) {
	st, err := d.companies.Read(ctx, campaignID, co.ID)
	if err != nil {
		return co, err
	}
	b := BuildDetail(rs, settings, st).CampaignCompany
	co.Points, co.FullPoints, co.Units = b.Points, b.FullPoints, b.Units
	return co, nil
}

// figures rates every company of the campaign with a single read of the rosters (rated costs a read per company); the result
// is by company id, only the Points, FullPoints and Units fields filled.
func (d companyDeps) figures(ctx context.Context, campaignID int64, rs *RuleSet, settings CampaignSettings) (map[int64]CampaignCompany, error) {
	states, err := d.companies.ReadRoster(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]CampaignCompany, len(states))
	for _, st := range states {
		out[st.Company.ID] = BuildDetail(rs, settings, st).CampaignCompany
	}
	return out, nil
}

// ratedAll fills the rating figures of the companies given (of one campaign) from one read of the campaign's rosters.
func (d companyDeps) ratedAll(ctx context.Context, campaignID int64, rs *RuleSet, settings CampaignSettings, list []CampaignCompany) error {
	f, err := d.figures(ctx, campaignID, rs, settings)
	if err != nil {
		return err
	}
	for i := range list {
		b, ok := f[list[i].ID]
		if !ok {
			return ErrNotFound
		}
		list[i].Points, list[i].FullPoints, list[i].Units = b.Points, b.FullPoints, b.Units
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
	for i := range list {
		co := list[i].Company
		if co == nil {
			continue
		}
		rs, settings, err := c.ruleset(ctx, list[i].ID)
		if err == nil {
			var rated CampaignCompany
			if rated, err = c.rated(ctx, list[i].ID, rs, settings, *co); err == nil {
				list[i].Company = &rated
			}
		}
		if err != nil { // one campaign whose rules cannot be loaded must not hide the others
			c.log.Warn("cannot rate the company", "campaign", list[i].ID, "company", co.ID, "err", err)
		}
	}
	return list, nil
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
	if err != nil || d.Company == nil {
		return d, err
	}
	rs, settings, err := c.ruleset(ctx, id)
	if err != nil {
		return d, err
	}
	rated, err := c.rated(ctx, id, rs, settings, *d.Company)
	if err != nil {
		return d, err
	}
	d.Company = &rated
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
	if err := c.ratedAll(ctx, id, rs, settings, list); err != nil {
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

// Settings returns the settings the campaign is played with; they never change after creation.
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
// reactivate a finished campaign (user decision 2026-10-21, mtu.26: an ended campaign is ended for good for the organiser). Ending needs
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
