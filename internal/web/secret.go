package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The secret objectives card of the match page (docs/matches.md "Secret objectives"): a player nominates and reports their own while the
// match is open; the service shows nobody else's entries, and after End match everybody's.

type heroOption struct {
	ID       int64
	Label    string
	Selected bool
}

// secretObjective is one secret objective of a company the viewer plays.
type secretObjective struct {
	core.ScoringObjective
	Nominee string // what the viewer noted down; empty for nothing
	Tier    int    // reported tier, 0 for none
	Heroes  []heroOption
}

type secretCompany struct {
	ID         int64
	Name       string
	Objectives []secretObjective
}

type secretView struct {
	Campaign int64
	MatchID  int64
	Ended    bool
	Mine     []secretCompany
	Revealed []core.SecretLine // after the match: everybody's
	Others   []string          // companies that have secret entries the viewer cannot see
	Msg      string
	Error    bool
	EntryID  int64
}

// secretObjectivesOf are the secret scoring objectives of the match's scenario that a company with this tag scores.
func secretObjectivesOf(info *core.ScenarioInfo, role string) []core.ScoringObjective {
	if info == nil {
		return nil
	}
	var out []core.ScoringObjective
	for _, o := range info.ObjectivesFor(role) {
		if o.Secret {
			out = append(out, o)
		}
	}
	return out
}

// hasSecret is whether the scenario has secret objectives (the card is shown only then).
func hasSecret(info *core.ScenarioInfo) bool { return len(secretObjectivesOf(info, "")) > 0 }

// ShowSecret is whether the match page has the secret objectives card.
func (v matchView) ShowSecret() bool {
	return hasSecret(v.Info) && (v.Campaign.Status != core.CampaignFinished || v.Match.Ended) // a frozen open match takes no notes
}

// secretCard builds the card for the viewer.
func (s *Server) secretCard(r *http.Request, id int64, m core.Match) (secretView, error) {
	user := *userFrom(r.Context())
	v := secretView{Campaign: id, MatchID: m.ID, Ended: m.Ended}
	view, err := s.campaigns.SecretObjectives(r.Context(), user, id, m.ID)
	if err != nil {
		return v, err
	}
	v.Others = view.Others
	if view.Revealed {
		v.Revealed = view.Lines
		return v, nil
	}
	var info *core.ScenarioInfo
	if m.Scenario != nil && m.Scenario.Kind == core.ScenarioRulebook {
		list, err := s.campaigns.Scenarios(r.Context(), id)
		if err != nil {
			return v, err
		}
		for i := range list {
			if list[i].Name == m.Scenario.Name {
				info = &list[i]
			}
		}
	}
	details := map[int64]core.CompanyDetail{}
	for _, side := range m.Sides {
		for _, co := range side.Companies {
			d, err := s.campaigns.CompanyDetail(r.Context(), id, co.ID)
			if err != nil {
				return v, err
			}
			details[co.ID] = d
		}
	}
	for i, side := range m.Sides {
		for _, co := range side.Companies {
			if co.OwnerID != user.ID {
				continue
			}
			sc := secretCompany{ID: co.ID, Name: co.Name}
			for _, o := range secretObjectivesOf(info, side.Role) {
				so := secretObjective{ScoringObjective: o}
				for _, l := range view.Lines {
					if l.CompanyID == co.ID && l.Key == o.Key {
						so.Nominee, so.Tier = l.Nominee, l.Tier
					}
				}
				if n := o.Nomination; n != nil && n.From != "terrain" {
					for j, other := range m.Sides {
						for _, oc := range other.Companies {
							if (n.From == "ownHeroes") != (oc.ID == co.ID) || (n.From == "enemyHeroes" && j == i) {
								continue
							}
							so.Heroes = append(so.Heroes, heroOptions(details[oc.ID], n, n.From == "enemyHeroes", so.Nominee)...)
						}
					}
				}
				sc.Objectives = append(sc.Objectives, so)
			}
			v.Mine = append(v.Mine, sc)
		}
	}
	return v, nil
}

// heroOptions lists the Heroes of a company that may be noted down (the Leader only when it has no other Hero).
func heroOptions(d core.CompanyDetail, n *core.ObjectiveNomination, withCompany bool, nominee string) []heroOption {
	heroes := 0
	for _, u := range d.Roster {
		if u.IsHero() && !core.Absent(u) {
			heroes++
		}
	}
	var out []heroOption
	for _, u := range d.Roster {
		if !u.IsHero() || core.Absent(u) || (n.AllowLeader == "onlyHero" && u.Role == core.RoleLeader && heroes > 1) {
			continue
		}
		label := u.Title()
		if withCompany {
			label += " (" + d.Name + ")"
		}
		out = append(out, heroOption{ID: u.ID, Label: label, Selected: nominee != "" && nominee == u.Title()})
	}
	return out
}

func (s *Server) secretCtx(w http.ResponseWriter, r *http.Request) (int64, core.Match, bool) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return 0, core.Match{}, false
	}
	mid, ok := s.idParam(w, r, "match")
	if !ok {
		return 0, core.Match{}, false
	}
	m, err := s.campaigns.Match(r.Context(), id, mid)
	if s.fail(w, r, err) {
		return 0, core.Match{}, false
	}
	return id, m, true
}

// handleSecretCard answers the card.
func (s *Server) handleSecretCard(w http.ResponseWriter, r *http.Request) {
	id, m, ok := s.secretCtx(w, r)
	if !ok {
		return
	}
	v, err := s.secretCard(r, id, m)
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, http.StatusOK, "match", "secret_card", v)
}

// handleSecretWrite nominates (kind=nominate: key, unit or note) or reports (kind=done: key, tier) for {company}.
func (s *Server) handleSecretWrite(w http.ResponseWriter, r *http.Request) {
	id, m, ok := s.secretCtx(w, r)
	if !ok {
		return
	}
	company, ok := s.idParam(w, r, "company")
	if !ok {
		return
	}
	a := core.LiveAction{ObjectiveKey: r.PostFormValue("key")}
	switch r.PostFormValue("kind") {
	case "nominate":
		a.Kind, a.UnitID, a.Note = core.LiveSecretNominate, formID(r, "unit"), strings.TrimSpace(r.PostFormValue("note"))
	case "done":
		a.Kind, a.Tier = core.LiveSecretDone, formInt(r, "tier")
	default:
		s.notFound(w, r)
		return
	}
	user := *userFrom(r.Context())
	status := http.StatusOK
	msg, isErr, entryID := "", false, int64(0)
	_, entry, err := s.campaigns.Apply(r.Context(), user, id, m.ID, company, a)
	switch {
	case err == nil:
		msg, entryID = "Saved. Only you can see it until the match ends.", entry.ID
	case errors.Is(err, core.ErrNotFound):
		s.notFound(w, r)
		return
	case errors.Is(err, core.ErrRefused), errors.Is(err, core.ErrInvalidLive):
		status, msg, isErr = http.StatusUnprocessableEntity, reason(err), true
	default:
		s.serverError(w, r, err)
		return
	}
	s.secretAnswer(w, r, id, m, status, msg, isErr, entryID)
}

func (s *Server) secretAnswer(w http.ResponseWriter, r *http.Request, id int64, m core.Match, status int, msg string, isErr bool, entryID int64) {
	v, err := s.secretCard(r, id, m)
	if s.fail(w, r, err) {
		return
	}
	if status == http.StatusUnprocessableEntity && len(v.Mine) == 0 {
		s.refuse(w, r, http.StatusForbidden, msg)
		return
	}
	v.Msg, v.Error, v.EntryID = msg, isErr, entryID
	s.render.partial(w, status, "match", "secret_card", v)
}

// answerSecretUndo is the answer to an Undo pressed on the card.
func (s *Server) answerSecretUndo(w http.ResponseWriter, r *http.Request, id, matchID int64, undoErr error) {
	m, err := s.campaigns.Match(r.Context(), id, matchID)
	if s.fail(w, r, err) {
		return
	}
	switch {
	case undoErr == nil:
		s.secretAnswer(w, r, id, m, http.StatusOK, "Undone.", false, 0)
	case errors.Is(undoErr, core.ErrNotFound):
		s.secretAnswer(w, r, id, m, http.StatusOK, "That entry is gone already.", true, 0)
	case errors.Is(undoErr, core.ErrRefused):
		s.secretAnswer(w, r, id, m, http.StatusUnprocessableEntity, reason(undoErr), true, 0)
	default:
		s.serverError(w, r, undoErr)
	}
}
