package core

import (
	"context"
	"errors"
	"slices"
	"time"
)

// Matches records what happens at the table: matches, their scenario and result, ending and reopening, and the awards the
// organiser hands out. It implements those methods of web.campaigns (docs/matches.md) over the match repository; the rules
// (Match.Access, CanEnd, ValidateResult, the revision) are applied here, inside matchRepository.Update where the match is locked.
// Core.Campaigns / the wiring compose it next to the other services.
type Matches struct {
	companyDeps
	matches matchRepository

	// AftermathStarted, when set, tells whether any company has started the War's Aftermath of a match; ReopenMatch is refused
	// once one has. The aftermath layer provides it when it is wired; nil means "never".
	AftermathStarted func(ctx context.Context, campaignID, matchID int64) (bool, error)
	// CompanyAftermaths, when set, lists the aftermaths a company still has to finish; a company with any cannot start a match
	// (its roster actions stay open meanwhile). LiveInit, when set, stores the starting values of the
	// live mode right after the match is created; its failure is ignored, the first read initialises (LiveService).
	CompanyAftermaths func(ctx context.Context, campaignID, companyID int64) ([]AftermathRef, error)
	LiveInit          func(ctx context.Context, campaignID, matchID int64) error

	// EndNotify, when set, is told about every change of a pending end (propose, confirm, withdraw, cancel, override, the end itself) after
	// it is stored; the wiring points it at the live log and the hub (docs/live-match.md, "Ending a match while live").
	EndNotify func(ctx context.Context, ch EndChange)
	// MissingVotes, when set, lists the MVP votes still missing before a competitive match can end: those of the owner ownerID (0: of
	// every owner of the match). The wiring points it at the live mode (LiveService.MissingVotes); nil means no vote is required.
	MissingVotes func(ctx context.Context, campaignID, matchID, ownerID int64) ([]MissingVote, error)
	// GameStarted, when set, says whether the game of an open match has begun (a live entry exists): the scenario is then fixed.
	GameStarted func(ctx context.Context, campaignID, matchID int64) (bool, error)

	ato atoRepository // optional (WithATO): the Against the Odds selection
}

// NewMatches builds the service. rules is the loader (*Rules).
func NewMatches(campaigns campaignRepository, companies companyRepository, matches matchRepository, rules ruleSource) *Matches {
	return &Matches{
		companyDeps: companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: RandomDice{}},
		matches:     matches,
	}
}

// Matches lists the campaign's matches, newest first, with the companies rated.
func (s *Matches) Matches(ctx context.Context, id int64) ([]Match, error) {
	if _, _, err := s.campaigns.Hashes(ctx, id); err != nil {
		return nil, err
	}
	list, err := s.matches.List(ctx, id)
	if err != nil {
		return nil, err
	}
	rate, err := s.rater(ctx, id, nil, nil)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if err := rate(&list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// Match returns one match with its companies rated and its history.
func (s *Matches) Match(ctx context.Context, id, matchID int64) (Match, error) {
	m, err := s.matches.Get(ctx, id, matchID)
	if err != nil {
		return m, err
	}
	return s.complete(ctx, id, m)
}

// CreateMatch starts an open match with the scenario the form showed and the Against the Odds bonuses the creator took (both optional).
// An error wrapping ErrInvalidMatch when the sides, the scenario or the bonuses are rejected, ErrRefused when the campaign is finished,
// a company has an officer that has not chosen its path or the creator does not own the lower rated company they took bonuses for,
// ErrNotFound when the user is not in the campaign.
func (s *Matches) CreateMatch(ctx context.Context, user User, id int64, in NewMatch) (Match, error) {
	sides, err := ValidateSides(in.Sides)
	if err != nil {
		return Match{}, err
	}
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return Match{}, err
	}
	if err := camp.CanStartMatch(); err != nil {
		return Match{}, err
	}
	for _, side := range sides { // a company with an officer that has no path yet cannot play
		for _, cid := range side.Companies {
			d, _, err := s.detail(ctx, id, cid)
			if err != nil {
				return Match{}, err
			}
			if d.OfficersPending {
				return Match{}, Refuse("%s cannot play yet: arrange the officers first.", d.Name)
			}
			for _, u := range d.Roster {
				if u.PathPending {
					return Match{}, Refuse("%s cannot play yet: %s has to choose a path first.", d.Name, u.Title())
				}
			}
			if s.CompanyAftermaths != nil {
				refs, err := s.CompanyAftermaths(ctx, id, cid)
				if err != nil {
					return Match{}, err
				}
				if len(refs) > 0 {
					return Match{}, Refuse("%s cannot play yet: %s", d.Name, LockMessage(refs))
				}
			}
		}
	}
	create := NewMatch{Sides: sides, Practice: in.Practice}
	if in.Scenario != nil {
		sc, err := s.validScenario(ctx, id, *in.Scenario)
		if err != nil {
			return Match{}, err
		}
		create.Scenario = &sc
	}
	if in.ATO != nil && !in.ATO.Selection.Empty() {
		ato, err := s.newMatchATO(ctx, user, camp, id, sides, in.Practice, in.ATO.Selection)
		if err != nil {
			return Match{}, err
		}
		if ato.Selection.Ambush != "" { // Ambush!: the scenario of the match is the one the bonus chose
			create.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: ato.Selection.Ambush}
		}
		create.ATO = &ato
	}
	m, err := s.matches.Create(ctx, id, user, create) // refuses with *CompanyBusyError when a company is in another open match
	if err != nil {
		return m, err
	}
	if s.LiveInit != nil {
		_ = s.LiveInit(ctx, id, m.ID) // the live data is also made on the first read
	}
	return s.complete(ctx, id, m)
}

// validScenario checks a scenario of the new match form (a rulebook name must be in the campaign's rules file).
func (s *Matches) validScenario(ctx context.Context, id int64, in ScenarioInput) (ScenarioInput, error) {
	in, err := in.Validate()
	if err != nil {
		return in, err
	}
	if in.Kind == ScenarioRulebook {
		list, err := s.Scenarios(ctx, id)
		if err != nil {
			return in, err
		}
		if !slices.ContainsFunc(list, func(sc ScenarioInfo) bool { return sc.Name == in.Name }) {
			return in, errors.Join(ErrInvalidMatch, errors.New("the campaign's rules have no scenario called "+in.Name))
		}
	}
	return in, nil
}

// Scenarios lists what the picker offers: the scenarios of the campaign's Battle Companies file.
func (s *Matches) Scenarios(ctx context.Context, id int64) ([]ScenarioInfo, error) {
	h, _, err := s.campaigns.Hashes(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.scenarioList(ctx, h)
}

// ScenarioSelection is how the campaign's rules pick a scenario by dice: the size of the two dice and the table ranges of the first.
func (s *Matches) ScenarioSelection(ctx context.Context, id int64) (ScenarioSelection, error) {
	h, _, err := s.campaigns.Hashes(ctx, id)
	if err != nil {
		return ScenarioSelection{}, err
	}
	rs, err := s.rules.For(ctx, h)
	if err != nil {
		return ScenarioSelection{}, err
	}
	return rs.BC.Selection(), nil
}

func (s *Matches) scenarioList(ctx context.Context, h RulesetHashes) ([]ScenarioInfo, error) {
	rs, err := s.rules.For(ctx, h)
	if err != nil {
		return nil, err
	}
	return rs.BC.Scenarios(), nil
}

// SetScenario chooses or replaces the scenario; the objectives reported belonged to the old one and are cleared.
func (s *Matches) SetScenario(ctx context.Context, user User, id, matchID int64, revision int, in ScenarioInput) (Match, error) {
	in, err := s.validScenario(ctx, id, in)
	if err != nil {
		return Match{}, err
	}
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if err := s.scenarioOpen(ctx, id, m); err != nil {
			return err
		}
		m.Scenario = &in
		clearObjectives(m)
		m.resetEnd(user.DisplayName, "scenario")
		return nil
	})
}

// MatchStarted says whether the game of the match has begun (a live entry exists); false without GameStarted.
func (s *Matches) MatchStarted(ctx context.Context, id, matchID int64) (bool, error) {
	if s.GameStarted == nil {
		return false, nil
	}
	return s.GameStarted(ctx, id, matchID)
}

// scenarioOpen refuses a change of the scenario of an open match once the game has started (the first live entry), as Against the
// Odds is: what the players set up for stays (docs/simplify-matches-spec.md, D4). An ended match can still be corrected, and a match that
// started without one (older matches) can still get one, since it cannot end without it.
func (s *Matches) scenarioOpen(ctx context.Context, id int64, m *Match) error {
	if m.Ended || m.Scenario == nil || s.GameStarted == nil {
		return nil
	}
	started, err := s.GameStarted(ctx, id, m.ID)
	if err != nil {
		return err
	}
	if started {
		return Refuse("The game has started: the scenario can no longer change.")
	}
	return nil
}

// ClearScenario removes the scenario and the objectives reported.
func (s *Matches) ClearScenario(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if err := s.scenarioOpen(ctx, id, m); err != nil {
			return err
		}
		m.Scenario = nil
		clearObjectives(m)
		m.resetEnd(user.DisplayName, "scenario")
		return nil
	})
}

func clearObjectives(m *Match) {
	m.Result.Companies = slices.Clone(m.Result.Companies)
	for i := range m.Result.Companies {
		m.Result.Companies[i].Objectives = nil
	}
}

// SaveResult replaces the reported result (validated by Match.ValidateResult) and sets Match.Winner.
func (s *Matches) SaveResult(ctx context.Context, user User, id, matchID int64, revision int, r MatchResult) (Match, error) {
	h, _, err := s.campaigns.Hashes(ctx, id)
	if err != nil {
		return Match{}, err
	}
	var scenarios []ScenarioInfo
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		var info *ScenarioInfo
		if m.Scenario != nil && m.Scenario.Kind == ScenarioRulebook {
			if scenarios == nil {
				if scenarios, err = s.scenarioList(ctx, h); err != nil {
					return err
				}
			}
			if i := slices.IndexFunc(scenarios, func(sc ScenarioInfo) bool { return sc.Name == m.Scenario.Name }); i >= 0 {
				info = &scenarios[i]
			}
		}
		r, err := m.ValidateResult(r, info)
		if err != nil {
			return err
		}
		m.Result, m.Winner = r, r.Winner
		m.resetEnd(user.DisplayName, "result")
		return nil
	})
}

// endMatch closes the match at once when Match.CanEnd passes (unexported since 7k7). It is the internal step for an end that needs no confirmation; the pages
// use ProposeEnd, ConfirmEnd and OverrideEnd (docs/matches.md).
func (s *Matches) endMatch(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if err := s.checkEnd(ctx, m, id, 0); err != nil {
			return err
		}
		m.finishEnd("")
		return nil
	})
}

// ReopenMatch is for the managers of the campaign (its organiser and administrators); it is refused once a company has started the
// aftermath.
func (s *Matches) ReopenMatch(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return Match{}, err
	}
	if !camp.CanManage(user) {
		return Match{}, Refuse("Only the organiser of the campaign or an administrator can reopen a match.")
	}
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if !m.Ended {
			return Refuse("This match is not ended.")
		}
		if s.AftermathStarted != nil && !m.Practice { // a practice match has no aftermath to start
			started, err := s.AftermathStarted(ctx, id, matchID)
			if err != nil {
				return err
			}
			if started {
				return Refuse("A company has started the aftermath of this match; it cannot be reopened.")
			}
		}
		m.Ended, m.EndedOn = false, nil
		return nil
	})
}

// edit runs change on the match under the repository's lock after the access rule and the revision check.
func (s *Matches) edit(ctx context.Context, user User, id, matchID int64, revision int, change func(*Match) error) (Match, error) {
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return Match{}, err
	}
	if err := camp.CanPlay(); err != nil {
		return Match{}, err
	}
	m, err := s.matches.Update(ctx, id, matchID, user, func(m *Match) error {
		if m.Access(user, camp.OrganiserID) == MatchNoAccess {
			return Refuse("%s", m.NoAccessReason())
		}
		if revision != m.Revision {
			return ErrMatchChanged
		}
		return change(m)
	})
	if err != nil {
		return m, err
	}
	return s.complete(ctx, id, m)
}

// complete fills what the repository cannot: the history and the rating of the companies.
func (s *Matches) complete(ctx context.Context, id int64, m Match) (Match, error) {
	var err error
	if m.History, err = s.matches.Events(ctx, id, m.ID); err != nil {
		return m, err
	}
	inMatch := map[int64]bool{} // one match shows its own companies: the others of the campaign are not built
	for _, side := range m.Sides {
		for _, co := range side.Companies {
			inMatch[co.ID] = true
		}
	}
	inIDs := make([]int64, 0, len(inMatch))
	for companyID := range inMatch {
		inIDs = append(inIDs, companyID)
	}
	slices.Sort(inIDs)
	rate, err := s.rater(ctx, id, func(companyID int64) bool { return inMatch[companyID] }, inIDs)
	if err != nil {
		return m, err
	}
	return m, rate(&m)
}

// rater returns a function that fills the rating figures of the companies of a match (each company is read once).
func (s *Matches) rater(ctx context.Context, id int64, want func(companyID int64) bool, ids []int64) (func(*Match) error, error) {
	rs, settings, err := s.ruleset(ctx, id)
	if err != nil {
		return nil, err
	}
	var figures map[int64]CampaignCompany // the whole campaign, rated with one read, the first time a company is needed
	return func(m *Match) error {
		m.Sides = slices.Clone(m.Sides)
		for i := range m.Sides {
			m.Sides[i].Companies = slices.Clone(m.Sides[i].Companies)
			for k, co := range m.Sides[i].Companies {
				if figures == nil {
					if figures, err = s.figures(ctx, id, rs, settings, want, ids); err != nil {
						return err
					}
				}
				if r, ok := figures[co.ID]; ok {
					co.Points, co.FullPoints, co.Units, co.FactionLabel = r.Points, r.FullPoints, r.Units, r.FactionLabel
					m.Sides[i].Companies[k] = co
				}
			}
		}
		return nil
	}, nil
}

// GrantReward awards a company Gold, Influence and Experience on behalf of the organiser (or an administrator). ErrRefused
// (CampaignDetail.CanGrant) for anyone else and for a result below zero Gold or Influence; ErrInvalidCompany for bad input.
func (s *Matches) GrantReward(ctx context.Context, user User, id, companyID int64, in GrantInput) (Grant, error) {
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return Grant{}, err
	}
	if err := camp.CanGrant(user); err != nil {
		return Grant{}, err
	}
	in, err = in.Validate()
	if err != nil {
		return Grant{}, err
	}
	st, err := s.companies.Read(ctx, id, companyID)
	if err != nil {
		return Grant{}, err
	}
	if in.UnitID != 0 && !slices.ContainsFunc(st.Units, func(u UnitState) bool { return u.ID == in.UnitID }) {
		return Grant{}, Refuse("That unit is not on the company's roster.")
	}
	if st.Gold+in.Gold < 0 || st.Influence+in.Influence < 0 {
		return Grant{}, Refuse("The company would be left with less than 0 Gold or Influence (it has %d Gold and %d Influence).", st.Gold, st.Influence)
	}
	g, err := s.matches.AddGrant(ctx, id, user, Grant{CompanyID: companyID, Kind: GrantOrganiser, Gold: in.Gold, Influence: in.Influence,
		Experience: in.Experience, UnitID: in.UnitID, Note: in.Note, By: user.DisplayName, On: time.Now()})
	if errors.Is(err, ErrNegativeBalance) { // changed since the read above
		return Grant{}, Refuse("The company would be left with less than 0 Gold or Influence.")
	}
	return g, err
}

// Grants is the company's history of awards, newest first.
func (s *Matches) Grants(ctx context.Context, id, companyID int64) ([]Grant, error) {
	if _, err := s.companies.Read(ctx, id, companyID); err != nil {
		return nil, err
	}
	return s.matches.Grants(ctx, id, companyID)
}
