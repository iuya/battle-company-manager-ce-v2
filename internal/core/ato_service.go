package core

import (
	"context"
	"fmt"
	"slices"
)

// WithATO gives Matches the store of the Against the Odds selection; without it the card is not offered (AgainstTheOdds says so).
func (s *Matches) WithATO(r atoRepository) *Matches { s.ato = r; return s }

func (s *Matches) atoRules(ctx context.Context, hash string) (ATORules, error) {
	s.mu.Lock()
	r, ok := s.atoCache[hash]
	s.mu.Unlock()
	if ok {
		return r, nil
	}
	file, err := s.rulesets.Get(ctx, hash)
	if err != nil {
		return r, err
	}
	if r, err = ParseATO(file.Content); err != nil {
		return r, err
	}
	s.mu.Lock()
	if s.atoCache == nil {
		s.atoCache = map[string]ATORules{}
	}
	s.atoCache[hash] = r
	s.mu.Unlock()
	return r, nil
}

// atoSides says who can take bonuses in the match: the lower rated company (side index and company), its opponent and the gap. why is
// the sentence for a match that has no card.
func atoSides(m Match) (lower, higher CampaignCompany, why string) {
	if len(m.Sides) != 2 || len(m.Sides[0].Companies) != 1 || len(m.Sides[1].Companies) != 1 {
		return lower, higher, "Against the Odds is only available in two-company matches."
	}
	a, b := m.Sides[0].Companies[0], m.Sides[1].Companies[0]
	switch {
	case a.Points == b.Points:
		return lower, higher, fmt.Sprintf("Against the Odds: nobody. Both companies are rated %d.", a.Points)
	case a.Points < b.Points:
		return a, b, ""
	}
	return b, a, ""
}

// AgainstTheOdds loads the card for the viewer. ErrNotFound: the campaign or the match is unknown, or the viewer is not in the
// campaign. A match without a card (not two companies, equal ratings) is ATO.Eligible false with Why, never an error.
func (s *Matches) AgainstTheOdds(ctx context.Context, user User, id, matchID int64) (ATO, error) {
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return ATO{}, err
	}
	m, err := s.Match(ctx, id, matchID)
	if err != nil {
		return ATO{}, err
	}
	return s.atoCard(ctx, user, camp, id, m)
}

func (s *Matches) atoCard(ctx context.Context, user User, camp CampaignDetail, id int64, m Match) (ATO, error) {
	card := ATO{MatchID: m.ID, Revision: m.Revision}
	lower, higher, why := atoSides(m)
	if why != "" || s.ato == nil {
		card.Why = why
		if s.ato == nil && why == "" {
			card.Why = "Against the Odds is not available."
		}
		return card, nil
	}
	h, _, err := s.campaigns.Hashes(ctx, id)
	if err != nil {
		return card, err
	}
	rules, err := s.atoRules(ctx, h.BattleCompanies)
	if err != nil {
		return card, err
	}
	scenarios, err := s.scenarioList(ctx, h.BattleCompanies)
	if err != nil {
		return card, err
	}
	rec, err := s.ato.Get(ctx, id, m.ID)
	if err != nil {
		return card, err
	}
	d, _, err := s.detail(ctx, id, lower.ID)
	if err != nil {
		return card, err
	}
	card.Eligible, card.CompanyID, card.CompanyName = true, lower.ID, lower.Name
	card.OwnRating, card.OppRating, card.OppName, card.Gap = lower.Points, higher.Points, higher.Name, higher.Points-lower.Points
	card.Selection = rec.Selection
	card.Spent = rec.Selection.Spent(rules)
	hasWanderer := slices.ContainsFunc(d.Roster, func(u CompanyUnit) bool { return u.Wanderer })
	left := card.Gap - card.Spent
	for _, b := range rules.Bonuses {
		o := ATOOption{Key: b.Key, Name: b.Name, Text: b.Text, Rating: b.Rating, Taken: rec.Selection.Has(b.Key), AppliesIn: b.AppliesIn}
		switch {
		case o.Taken:
		case b.Key == atoWanderer && hasWanderer:
			o.Disabled, o.Reason = true, "The company already has a Wanderer."
		case b.Rating > left || (!rules.Inclusive && b.Rating == left):
			o.Disabled, o.Reason = true, fmt.Sprintf("Needs %d, %d left", b.Rating, max(left, 0))
		}
		card.Options = append(card.Options, o)
	}
	for _, k := range rules.Kits {
		card.Kits = append(card.Kits, ATOKit{Key: k.Key, Name: k.Name, Items: k.Items, Taken: slices.Contains(rec.Selection.Kits, k.Key)})
	}
	card.Wanderers = rules.Wanderers
	for _, u := range d.Roster {
		if !Absent(u) {
			card.Models = append(card.Models, ATOModel{UnitID: u.ID, Name: u.Title(), Type: u.Type, Hero: u.IsHero()})
		}
	}
	_ = scenarios
	started, err := s.atoStarted(ctx, id, m.ID)
	if err != nil {
		return card, err
	}
	if started {
		card.Locked, card.LockedWhy = true, "The game has started."
	}
	card.Access = ATOAccessFor(user, m, lower, camp.OrganiserID)
	if card.Locked || camp.CanPlay() != nil {
		card.Access = ATONone
	}
	return card, nil
}

// atoStarted says whether a live entry exists for the match.
func (s *Matches) atoStarted(ctx context.Context, id, matchID int64) (bool, error) {
	return s.ato.GameStarted(ctx, id, matchID)
}

// SaveAgainstTheOdds replaces the lower company's selection (an empty one clears it) and, for Ambush!, sets the match's scenario and
// remembers the one it had; dropping Ambush! puts that one back. ErrRefused: not the lower company's owner, the organiser or an administrator,
// the game has started, the match has no card; ErrMatchChanged for a stale revision; ErrInvalidMatch (joined, fit to show) for a selection the
// rules refuse. The match revision goes up by one.
func (s *Matches) SaveAgainstTheOdds(ctx context.Context, user User, id, matchID int64, revision int, sel ATOSelection) (ATO, error) {
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return ATO{}, err
	}
	if err := camp.CanPlay(); err != nil {
		return ATO{}, err
	}
	m, err := s.Match(ctx, id, matchID)
	if err != nil {
		return ATO{}, err
	}
	card, err := s.atoCard(ctx, user, camp, id, m)
	if err != nil {
		return ATO{}, err
	}
	if !card.Eligible {
		return ATO{}, Refuse("%s", card.Why)
	}
	h, _, err := s.campaigns.Hashes(ctx, id)
	if err != nil {
		return ATO{}, err
	}
	rules, err := s.atoRules(ctx, h.BattleCompanies)
	if err != nil {
		return ATO{}, err
	}
	scenarios, err := s.scenarioList(ctx, h.BattleCompanies)
	if err != nil {
		return ATO{}, err
	}
	names := make([]string, len(scenarios))
	for i, sc := range scenarios {
		names[i] = sc.Name
	}
	d, _, err := s.detail(ctx, id, card.CompanyID)
	if err != nil {
		return ATO{}, err
	}
	clean, err := sel.Validate(rules, d.Roster, card.Gap, names)
	if err != nil {
		return ATO{}, err
	}
	lower, _, _ := atoSides(m)
	_, err = s.ato.Update(ctx, id, matchID, user, func(st *ATOState) error {
		switch {
		case ATOAccessFor(user, st.Match, lower, camp.OrganiserID) != ATOEdit:
			return Refuse("Only the owner of %s, the organiser of the campaign or an administrator can change this.", lower.Name)
		case st.Started:
			return Refuse("The game has started: Against the Odds can no longer change.")
		case revision != st.Match.Revision:
			return ErrMatchChanged
		}
		old := st.Record
		switch {
		case clean.Ambush != "":
			if old.Selection.Ambush == "" { // the scenario it replaces is remembered once
				old.HadPrevious = st.Match.Scenario != nil
				old.Previous = nil
				if st.Match.Scenario != nil {
					cp := *st.Match.Scenario
					old.Previous = &cp
				}
			}
			if st.Match.Scenario == nil || st.Match.Scenario.Kind != ScenarioRulebook || st.Match.Scenario.Name != clean.Ambush {
				st.Match.Scenario = &MatchScenario{Kind: ScenarioRulebook, Name: clean.Ambush}
				clearObjectives(&st.Match)
			}
		case old.Selection.Ambush != "":
			cur := st.Match.Scenario
			if cur != nil && cur.Kind == ScenarioRulebook && cur.Name == old.Selection.Ambush { // still the one Ambush! chose: put the old one back
				st.Match.Scenario = old.Previous
				clearObjectives(&st.Match)
			}
			old.Previous, old.HadPrevious = nil, false
		}
		old.MatchID, old.CompanyID, old.Selection = matchID, lower.ID, clean
		st.Record = old
		st.Match.resetEnd(user.DisplayName, "Against the Odds selection")
		return nil
	})
	if err != nil {
		return ATO{}, err
	}
	return s.AgainstTheOdds(ctx, user, id, matchID)
}
