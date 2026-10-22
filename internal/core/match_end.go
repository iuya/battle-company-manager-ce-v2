package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// The match ends only when every participant confirms (docs/matches.md, "Ending a match: confirmation by all participants"). The
// participants are the owners of the companies in the match, one confirmation per company.

// maxOverrideNote is the longest note of an override (characters).
const maxOverrideNote = 200

// EndConfirmation is one company's part of a pending end.
type EndConfirmation struct {
	CompanyID, OwnerID int64
	CompanyName        string
	Confirmed          bool
	By                 string    // display name of who confirmed (the owner, or the override author)
	On                 time.Time // zero while waiting
	Override           bool      // confirmed by the organiser or administrator override
}

// EndProposal is the pending end; Match.EndProposal is nil when there is none.
type EndProposal struct {
	ProposedBy   string
	ProposedByID int64
	ProposedOn   time.Time
	Companies    []EndConfirmation // one per company of the match, the match's order
	// Notice says why the confirmations were reset ("Hador changed the result; confirm again"); empty when they were not.
	Notice string
}

// Waiting are the companies that have not confirmed.
func (p EndProposal) Waiting() []EndConfirmation {
	var out []EndConfirmation
	for _, c := range p.Companies {
		if !c.Confirmed {
			out = append(out, c)
		}
	}
	return out
}

func (p EndProposal) confirmedCount() int { return len(p.Companies) - len(p.Waiting()) }

// WaitingOn says whether the user owns a company that still has to confirm.
func (p EndProposal) WaitingOn(userID int64) bool {
	return slices.ContainsFunc(p.Waiting(), func(c EndConfirmation) bool { return c.OwnerID == userID && userID != 0 })
}

// ConfirmedBy says whether the user owns a company that confirmed on their own (not by an override).
func (p EndProposal) ConfirmedBy(userID int64) bool {
	return slices.ContainsFunc(p.Companies, func(c EndConfirmation) bool { return c.Confirmed && !c.Override && c.OwnerID == userID && userID != 0 })
}

// EndAccess is what the viewer may do now; the page only hides controls with it, the service enforces it.
type EndAccess struct {
	CanPropose, CanConfirm, CanWithdraw, CanCancel, CanOverride bool
	Reason                                                      string // why a control is missing, when it helps
}

// EndAccessFor works out what the user may do about ending the match. companies are the companies of the match (their owners are the
// participants). A viewer, a finished or ended match give nothing.
func EndAccessFor(u User, m Match, companies []CampaignCompany, organiserID int64) EndAccess {
	if m.Ended {
		return EndAccess{Reason: "This match has ended."}
	}
	manager := CanManageCampaign(u, organiserID)
	participant := slices.ContainsFunc(companies, func(c CampaignCompany) bool { return c.OwnerID == u.ID && u.ID != 0 })
	if !manager && !participant {
		return EndAccess{Reason: "Only the players in this match can change it."}
	}
	p := m.EndProposal
	if p == nil {
		return EndAccess{CanPropose: true}
	}
	return EndAccess{
		CanConfirm:  p.WaitingOn(u.ID),
		CanWithdraw: p.ConfirmedBy(u.ID),
		CanCancel:   true,
		CanOverride: manager && len(p.Waiting()) > 0,
	}
}

// matchCompanies lists the companies of every side, in the match's order.
func matchCompanies(m Match) []CampaignCompany {
	var out []CampaignCompany
	for _, s := range m.Sides {
		out = append(out, s.Companies...)
	}
	return out
}

// resetEnd drops every confirmation of a pending end except the proposer's, after a change of the scenario, the result, the roles or the
// Against the Odds selection; by is who changed it. It does nothing when no end is pending.
func (m *Match) resetEnd(by string, what string) {
	p := m.EndProposal
	if p == nil {
		return
	}
	next := *p
	next.Companies = slices.Clone(p.Companies)
	dropped := false
	for i, c := range next.Companies {
		if c.Confirmed && c.OwnerID != p.ProposedByID {
			next.Companies[i] = EndConfirmation{CompanyID: c.CompanyID, OwnerID: c.OwnerID, CompanyName: c.CompanyName}
			dropped = true
		}
	}
	if dropped {
		next.Notice = fmt.Sprintf("%s changed the %s; confirm again", by, what)
	}
	m.EndProposal = &next
}

// finishEnd ends the match: the proposal goes, the detail of the history says who confirmed.
func (m *Match) finishEnd(detail string) {
	now := time.Now()
	m.Ended, m.EndedOn = true, &now
	m.EndProposal, m.EndDetail = nil, detail
}

func companyNames(list []EndConfirmation) string {
	var names []string
	for _, c := range list {
		names = append(names, c.CompanyName)
	}
	return strings.Join(names, ", ")
}

// checkEnd is CanEnd plus the rule that a scenario with an Attacker and a Defender needs both teams tagged.
func checkEnd(m *Match, scenarios []ScenarioInfo) error {
	if err := m.CanEnd(); err != nil {
		return err
	}
	if m.Scenario != nil && m.Scenario.Kind == ScenarioRulebook && len(m.Sides) == 2 {
		if i := slices.IndexFunc(scenarios, func(sc ScenarioInfo) bool { return sc.Name == m.Scenario.Name }); i >= 0 && scenarios[i].HasRoles() {
			if m.Sides[0].Role == "" || m.Sides[1].Role == "" {
				return errors.Join(ErrInvalidMatch, errors.New("Choose which team attacks and which defends"))
			}
		}
	}
	return nil
}

// EndChange is what happened to a pending end, for the live log and the hub (EndNotify).
type EndChange struct {
	Kind        string // endProposed, endConfirmed, endWithdrawn, endCancelled, endOverride, ended
	By          User
	CampaignID  int64
	MatchID     int64
	CompanyIDs  []int64 // the companies it concerns
	Text        string  // "Hador proposed to end the match", "Ending cancelled: Aldor wounded Grashnak"
	CompanyName string
}

func (s *Matches) notifyEnd(ctx context.Context, ch EndChange) {
	if s.EndNotify != nil {
		s.EndNotify(ctx, ch)
	}
}

// ProposeEnd starts the end: CanEnd runs at once (ErrRefused / ErrInvalidMatch with its reasons), the proposer's own companies confirm,
// and when nobody is left waiting the match ends at once.
func (s *Matches) ProposeEnd(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	scenarios, err := s.Scenarios(ctx, id)
	if err != nil {
		return Match{}, err
	}
	var change EndChange
	m, err := s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if m.EndProposal != nil {
			return Refuse("The end of this match has already been proposed.")
		}
		if err := checkEnd(m, scenarios); err != nil {
			return err
		}
		now := time.Now()
		p := EndProposal{ProposedBy: user.DisplayName, ProposedByID: user.ID, ProposedOn: now}
		for _, c := range matchCompanies(*m) {
			e := EndConfirmation{CompanyID: c.ID, OwnerID: c.OwnerID, CompanyName: c.Name}
			if c.OwnerID == user.ID && user.ID != 0 {
				e.Confirmed, e.By, e.On = true, user.DisplayName, now
			}
			p.Companies = append(p.Companies, e)
		}
		change = EndChange{Kind: "endProposed", By: user, CampaignID: id, MatchID: matchID, Text: user.DisplayName + " proposed to end the match"}
		if len(p.Waiting()) == 0 {
			m.finishEnd("confirmed by " + companyNames(p.Companies))
			change.Kind = "ended"
			return nil
		}
		m.EndProposal = &p
		return nil
	})
	if err == nil {
		s.notifyEnd(ctx, change)
	}
	return m, err
}

// ConfirmEnd confirms the companies of the user that wait; the last confirmation ends the match.
func (s *Matches) ConfirmEnd(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	scenarios, err := s.Scenarios(ctx, id)
	if err != nil {
		return Match{}, err
	}
	var change EndChange
	m, err := s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		p := m.EndProposal
		if p == nil {
			return Refuse("Nobody has proposed to end this match.")
		}
		if !p.WaitingOn(user.ID) {
			return Refuse("You have no company waiting to confirm.")
		}
		next := *p
		next.Companies = slices.Clone(p.Companies)
		now := time.Now()
		var mine []int64
		for i, c := range next.Companies {
			if !c.Confirmed && c.OwnerID == user.ID {
				next.Companies[i].Confirmed, next.Companies[i].By, next.Companies[i].On = true, user.DisplayName, now
				mine = append(mine, c.CompanyID)
			}
		}
		next.Notice = ""
		change = EndChange{Kind: "endConfirmed", By: user, CampaignID: id, MatchID: matchID, CompanyIDs: mine, Text: user.DisplayName + " confirmed the end"}
		if len(next.Waiting()) == 0 {
			if err := checkEnd(m, scenarios); err != nil {
				return err
			}
			m.finishEnd("confirmed by " + companyNames(next.Companies))
			change.Kind = "ended"
			return nil
		}
		m.EndProposal = &next
		return nil
	})
	if err == nil {
		s.notifyEnd(ctx, change)
	}
	return m, err
}

// WithdrawEnd takes the user's own confirmations back; a proposal nobody confirms any more is cancelled.
func (s *Matches) WithdrawEnd(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	var change EndChange
	m, err := s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		p := m.EndProposal
		if p == nil {
			return Refuse("Nobody has proposed to end this match.")
		}
		if !p.ConfirmedBy(user.ID) {
			return Refuse("You have not confirmed the end.")
		}
		next := *p
		next.Companies = slices.Clone(p.Companies)
		var mine []int64
		for i, c := range next.Companies {
			if c.Confirmed && !c.Override && c.OwnerID == user.ID {
				next.Companies[i] = EndConfirmation{CompanyID: c.CompanyID, OwnerID: c.OwnerID, CompanyName: c.CompanyName}
				mine = append(mine, c.CompanyID)
			}
		}
		change = EndChange{Kind: "endWithdrawn", By: user, CampaignID: id, MatchID: matchID, CompanyIDs: mine, Text: user.DisplayName + " withdrew the confirmation"}
		if next.confirmedCount() == 0 {
			m.EndProposal = nil
			change.Kind, change.Text = "endCancelled", user.DisplayName+" withdrew the confirmation: nobody confirms the end now"
			return nil
		}
		m.EndProposal = &next
		return nil
	})
	if err == nil {
		s.notifyEnd(ctx, change)
	}
	return m, err
}

// CancelEnd drops the proposal and every confirmation: a participant owner, the organiser or an administrator ("Keep playing").
func (s *Matches) CancelEnd(ctx context.Context, user User, id, matchID int64, revision int) (Match, error) {
	var change EndChange
	m, err := s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		if m.EndProposal == nil {
			return Refuse("Nobody has proposed to end this match.")
		}
		m.EndProposal = nil
		change = EndChange{Kind: "endCancelled", By: user, CampaignID: id, MatchID: matchID, Text: "Ending cancelled by " + user.DisplayName}
		return nil
	})
	if err == nil {
		s.notifyEnd(ctx, change)
	}
	return m, err
}

// CancelPendingEnd is CancelEnd for the live mode: a game action made while the end is pending cancels it first, whoever acts and
// whatever the revision. It returns false when nothing was pending. why is the log line ("Ending cancelled: Aldor wounded Grashnak").
func (s *Matches) CancelPendingEnd(ctx context.Context, user User, id, matchID int64, why string) (bool, error) {
	cancelled := false
	_, err := s.matches.Update(ctx, id, matchID, user, func(m *Match) error {
		if m.EndProposal == nil || m.Ended {
			return errNothingToStore
		}
		m.EndProposal = nil
		cancelled = true
		return nil
	})
	if errors.Is(err, errNothingToStore) {
		return false, nil
	}
	if err == nil && cancelled {
		s.notifyEnd(ctx, EndChange{Kind: "endCancelled", By: user, CampaignID: id, MatchID: matchID, Text: why})
	}
	return cancelled, err
}

// errNothingToStore makes a matchRepository.Update roll back without storing anything (nothing to change).
var errNothingToStore = errors.New("nothing to store")

// OverrideEnd ends the match without the confirmation of the companies still waiting: the organiser or an administrator, with a note
// of 1-200 characters. Their own waiting companies confirm instead of being overridden.
func (s *Matches) OverrideEnd(ctx context.Context, user User, id, matchID int64, revision int, note string) (Match, error) {
	note = strings.TrimSpace(note)
	if n := utf8.RuneCountInString(note); n == 0 || n > maxOverrideNote {
		return Match{}, errors.Join(ErrInvalidMatch, errors.New("the note must be between 1 and 200 characters"))
	}
	camp, err := s.campaigns.Get(ctx, user.ID, id)
	if err != nil {
		return Match{}, err
	}
	if !camp.CanManage(user) {
		return Match{}, Refuse("Only the organiser of the campaign or an administrator can end a match without its players' confirmation.")
	}
	scenarios, err := s.Scenarios(ctx, id)
	if err != nil {
		return Match{}, err
	}
	var change EndChange
	m, err := s.edit(ctx, user, id, matchID, revision, func(m *Match) error {
		p := m.EndProposal
		if p == nil {
			return Refuse("Propose the end first; the override is for a player who does not confirm.")
		}
		var overridden []EndConfirmation
		next := *p
		next.Companies = slices.Clone(p.Companies)
		now := time.Now()
		for i, c := range next.Companies {
			switch {
			case c.Confirmed:
			case c.OwnerID == user.ID && user.ID != 0:
				next.Companies[i].Confirmed, next.Companies[i].By, next.Companies[i].On = true, user.DisplayName, now
			default:
				overridden = append(overridden, c)
				next.Companies[i].Confirmed, next.Companies[i].By, next.Companies[i].On, next.Companies[i].Override = true, user.DisplayName, now, true
			}
		}
		if len(overridden) == 0 {
			return Refuse("Every company has confirmed already.")
		}
		if err := checkEnd(m, scenarios); err != nil {
			return err
		}
		m.finishEnd(fmt.Sprintf("overriding %s: %s", companyNames(overridden), note))
		var ids []int64
		for _, c := range overridden {
			ids = append(ids, c.CompanyID)
		}
		change = EndChange{Kind: "endOverride", By: user, CampaignID: id, MatchID: matchID, CompanyIDs: ids,
			Text: fmt.Sprintf("%s ended the match overriding %s: %s", user.DisplayName, companyNames(overridden), note)}
		return nil
	})
	if err == nil {
		s.notifyEnd(ctx, change)
	}
	return m, err
}
