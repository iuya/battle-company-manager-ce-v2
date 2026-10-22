package core

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// Matches records what happens at the table: matches, their scenario and result, ending and reopening, and the awards the
// organiser hands out. It implements those methods of web.campaigns (docs/matches.md) over the match repository; the rules
// (Match.Access, CanEnd, ValidateResult, the revision) are applied here, inside matchRepository.Update where the match is locked.
// Core.Campaigns / the wiring compose it next to the other services.
type Matches struct {
	companyDeps
	matches  matchRepository
	rulesets rulesetRepository

	// AftermathStarted, when set, tells whether any company has started the War's Aftermath of a match; ReopenMatch is refused
	// once one has. The aftermath layer provides it when it is wired; nil means "never".
	AftermathStarted func(ctx context.Context, campaignID, matchID int64) (bool, error)
	// CompanyAftermaths, when set, lists the aftermaths a company still has to finish; a company with any cannot start a match
	// (REVIEW pib.24: no; the lock would stop its roster actions anyway). LiveInit, when set, stores the starting values of the
	// live mode right after the match is created; its failure is ignored, the first read initialises (LiveService).
	CompanyAftermaths func(ctx context.Context, campaignID, companyID int64) ([]AftermathRef, error)
	LiveInit          func(ctx context.Context, campaignID, matchID int64) error

	// EndNotify, when set, is told about every change of a pending end (propose, confirm, withdraw, cancel, override, the end itself) after
	// it is stored; the wiring points it at the live log and the hub (docs/live-match.md, "Ending a match while live").
	EndNotify func(ctx context.Context, ch EndChange)

	ato atoRepository // optional (WithATO): the Against the Odds selection

	mu        sync.Mutex
	scenarios map[string][]ScenarioInfo // by Battle Companies file hash; files never change
	atoCache  map[string]ATORules       // the same for the Against the Odds rules
}

// NewMatches builds the service. rules is the loader (*Rules).
func NewMatches(campaigns campaignRepository, companies companyRepository, matches matchRepository, rulesets rulesetRepository, rules ruleSource) *Matches {
	return &Matches{
		companyDeps: companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: RandomDice{}},
		matches:     matches, rulesets: rulesets, scenarios: map[string][]ScenarioInfo{},
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
	rate, err := s.rater(ctx, id)
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

// CreateMatch starts an open match. An error wrapping ErrInvalidMatch when the sides are rejected, ErrRefused when the
// campaign is finished or a company has an officer that has not chosen its path, ErrNotFound when the user is not in the campaign.
func (s *Matches) CreateMatch(ctx context.Context, user User, id int64, sides []NewSide) (Match, error) {
	sides, err := ValidateSides(sides)
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
	m, err := s.matches.Create(ctx, id, user, sides) // refuses with *CompanyBusyError when a company is in another open match
	if err != nil {
		return m, err
	}
	if s.LiveInit != nil {
		_ = s.LiveInit(ctx, id, m.ID) // the live data is also made on the first read
	}
	return s.complete(ctx, id, m)
}

// Scenarios lists what the picker offers: the scenarios of the campaign's Battle Companies file.
func (s *Matches) Scenarios(ctx context.Context, id int64) ([]ScenarioInfo, error) {
	h, _, err := s.campaigns.Hashes(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.scenarioList(ctx, h.BattleCompanies)
}

func (s *Matches) scenarioList(ctx context.Context, hash string) ([]ScenarioInfo, error) {
	s.mu.Lock()
	list, ok := s.scenarios[hash]
	s.mu.Unlock()
	if ok {
		return list, nil
	}
	file, err := s.rulesets.Get(ctx, hash)
	if err != nil {
		return nil, err
	}
	if list, err = ParseScenarios(file.Content); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.scenarios[hash] = list
	s.mu.Unlock()
	return list, nil
}

// SetScenario chooses or replaces the scenario; the objectives reported belonged to the old one and are cleared.
func (s *Matches) SetScenario(ctx context.Context, user User, id, matchID int64, revision int, in ScenarioInput) (Match, error) {
	in, err := in.Validate()
	if err != nil {
		return Match{}, err
	}
	if in.Kind == ScenarioRulebook {
		list, err := s.Scenarios(ctx, id)
		if err != nil {
			return Match{}, err
		}
		if !slices.ContainsFunc(list, func(sc ScenarioInfo) bool { return sc.Name == in.Name }) {
			return Match{}, errors.Join(ErrInvalidMatch, errors.New("the campaign's rules have no scenario called "+in.Name))
		}
	}
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		m.Scenario = &in
		clearObjectives(m)
		m.resetEnd(user.DisplayName, "scenario")
		return nil
	})
}

// ClearScenario removes the scenario and the objectives reported.
func (s *Matches) ClearScenario(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
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
				if scenarios, err = s.scenarioList(ctx, h.BattleCompanies); err != nil {
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
	scenarios, err := s.Scenarios(ctx, id)
	if err != nil {
		return Match{}, err
	}
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if err := checkEnd(m, scenarios); err != nil {
			return err
		}
		m.finishEnd("")
		return nil
	})
}

// SetSideRoles tags the sides of an open match, one tag per side in order ("", "attacker" or "defender"; at most one side each). It can be
// done before the scenario is chosen; the tags only matter for a scenario with roles.
func (s *Matches) SetSideRoles(ctx context.Context, user User, id, matchID int64, revision int, roles []string) (Match, error) {
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if m.Ended {
			return Refuse("The teams' roles can only change while the match is open.")
		}
		if len(roles) != len(m.Sides) {
			return errors.Join(ErrInvalidMatch, errors.New("one role for each side"))
		}
		seen := map[string]bool{}
		for _, r := range roles {
			if !ValidRole(r) {
				return errors.Join(ErrInvalidMatch, errors.New("a side is Attacker or Defender, or neither"))
			}
			if r != "" && seen[r] {
				return errors.Join(ErrInvalidMatch, errors.New("only one side can be the "+r))
			}
			seen[r] = true
		}
		m.Sides = slices.Clone(m.Sides)
		for i := range m.Sides {
			m.Sides[i].Role = roles[i]
		}
		m.resetEnd(user.DisplayName, "teams' roles")
		return nil
	})
}

// SwapSideRoles swaps Attacker and Defender between the two tagged sides (a side without a tag takes the other's: with one tag the
// other side gets the opposite role).
func (s *Matches) SwapSideRoles(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	return s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if m.Ended {
			return Refuse("The teams' roles can only change while the match is open.")
		}
		if len(m.Sides) != 2 {
			return Refuse("Swapping roles is for a match of two teams.")
		}
		opposite := map[string]string{"": "", RoleAttacker: RoleDefender, RoleDefender: RoleAttacker}
		a, b := m.Sides[0].Role, m.Sides[1].Role
		if a == "" && b == "" {
			return Refuse("Choose the roles first.")
		}
		if a == "" || b == "" { // one tag: the other side takes the opposite
			if a == "" {
				a = opposite[b]
			} else {
				b = opposite[a]
			}
		}
		m.Sides = slices.Clone(m.Sides)
		m.Sides[0].Role, m.Sides[1].Role = b, a
		m.resetEnd(user.DisplayName, "teams' roles")
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
		if s.AftermathStarted != nil {
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
	rate, err := s.rater(ctx, id)
	if err != nil {
		return m, err
	}
	return m, rate(&m)
}

// rater returns a function that fills the rating figures of the companies of a match (each company is read once).
func (s *Matches) rater(ctx context.Context, id int64) (func(*Match) error, error) {
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
					if figures, err = s.figures(ctx, id, rs, settings); err != nil {
						return err
					}
				}
				if r, ok := figures[co.ID]; ok {
					co.Points, co.FullPoints, co.Units = r.Points, r.FullPoints, r.Units
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
