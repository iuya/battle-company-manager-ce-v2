package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The end checks of a running match (docs/live-match.md "End checks"): who got the objective, the MVP votes and the reminder before
// End match. A sheet on a company page and a card on the match page show the same body; every write answers with that body again.

// endView is the body of the end checks, with the message of the last write.
type endView struct {
	Campaign int64
	MatchID  int64
	Company  int64 // the company page the sheet was opened from, 0 on the match page
	Card     bool  // the match page's card, not the sheet
	Checks   core.EndChecks
	Msg      string
	Error    bool
	EntryID  int64 // the entry to undo; 0 for none
	Revision int   // End reminder: the revision the End match form was rendered with
}

// Players is whether the viewer can set anything here (the others see the tally only).
func (v endView) Players() bool { return len(v.Checks.Objective) > 0 || len(v.Checks.MVP) > 0 }

// Open tells the card what to show when the match has ended or nothing is open.
func (v endView) Tally(companyID int64) *core.MVPTally {
	for i := range v.Checks.Tally {
		if v.Checks.Tally[i].CompanyID == companyID {
			return &v.Checks.Tally[i]
		}
	}
	return nil
}

func (s *Server) endContext(w http.ResponseWriter, r *http.Request) (endView, bool) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return endView{}, false
	}
	matchID, ok := s.idParam(w, r, "match")
	if !ok {
		return endView{}, false
	}
	company, _ := strconv.ParseInt(r.URL.Query().Get("company"), 10, 64)
	if company == 0 {
		company = formID(r, "page")
	}
	return endView{Campaign: id, MatchID: matchID, Company: company, Card: r.URL.Query().Get("card") == "1" || r.PostFormValue("view") == "card"}, true
}

func (v endView) templateName() string {
	if v.Card {
		return "end_checks_card"
	}
	return "end_checks_sheet"
}

// handleEndChecks answers the sheet (?company=) or the card (?card=1). The card of a match that is not open is empty.
func (s *Server) handleEndChecks(w http.ResponseWriter, r *http.Request) {
	v, ok := s.endContext(w, r)
	if !ok {
		return
	}
	checks, err := s.campaigns.EndChecks(r.Context(), *userFrom(r.Context()), v.Campaign, v.MatchID)
	if errors.Is(err, core.ErrNotFound) && v.Card {
		w.WriteHeader(http.StatusOK)
		return
	}
	if s.fail(w, r, err) {
		return
	}
	v.Checks = checks
	s.render.partial(w, http.StatusOK, "match", v.templateName(), v)
}

// handleEndReminder answers the confirmation of End match: what is still open, with Go back and End anyway.
func (s *Server) handleEndReminder(w http.ResponseWriter, r *http.Request) {
	v, ok := s.endContext(w, r)
	if !ok {
		return
	}
	checks, err := s.campaigns.EndChecks(r.Context(), *userFrom(r.Context()), v.Campaign, v.MatchID)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	v.Checks = checks
	v.Revision, _ = strconv.Atoi(r.URL.Query().Get("revision"))
	s.render.partial(w, http.StatusOK, "match", "end_reminder", v)
}

func (s *Server) handleEndObjective(w http.ResponseWriter, r *http.Request) {
	s.endWrite(w, r, func(company int64) core.LiveAction {
		return core.LiveAction{Kind: core.LiveObjective, UnitID: formID(r, "unit"), On: r.PostFormValue("on") == "1"}
	})
}

func (s *Server) handleEndMVP(w http.ResponseWriter, r *http.Request) {
	s.endWrite(w, r, func(company int64) core.LiveAction {
		return core.LiveAction{Kind: core.LiveMVPVote, TargetCompanyID: company, UnitID: formID(r, "unit")}
	})
}

// endWrite records the action for {company} and answers the end checks again with the message (the refusal when it was refused).
func (s *Server) endWrite(w http.ResponseWriter, r *http.Request, action func(company int64) core.LiveAction) {
	v, ok := s.endContext(w, r)
	if !ok {
		return
	}
	company, ok := s.idParam(w, r, "company")
	if !ok {
		return
	}
	user := *userFrom(r.Context())
	status := http.StatusOK
	_, entry, err := s.campaigns.Apply(r.Context(), user, v.Campaign, v.MatchID, company, action(company))
	switch {
	case err == nil:
		v.Msg, v.EntryID = entry.Text, entry.ID
	case errors.Is(err, core.ErrNotFound):
		s.notFound(w, r)
		return
	case errors.Is(err, core.ErrRefused), errors.Is(err, core.ErrInvalidLive):
		status, v.Msg, v.Error = http.StatusUnprocessableEntity, reason(err), true
	default:
		s.serverError(w, r, err)
		return
	}
	s.endAnswer(w, r, v, status)
}

// endAnswer loads the checks and renders the body; a viewer who has nothing to set gets 403 instead of a refusal.
func (s *Server) endAnswer(w http.ResponseWriter, r *http.Request, v endView, status int) {
	checks, err := s.campaigns.EndChecks(r.Context(), *userFrom(r.Context()), v.Campaign, v.MatchID)
	if s.fail(w, r, err) {
		return
	}
	v.Checks = checks
	if status == http.StatusUnprocessableEntity && !v.Players() {
		s.refuse(w, r, http.StatusForbidden, v.Msg)
		return
	}
	s.render.partial(w, status, "match", v.templateName(), v)
}

// answerEndUndo is the answer to an Undo pressed in the end checks.
func (s *Server) answerEndUndo(w http.ResponseWriter, r *http.Request, id, matchID, companyID int64, card bool, undoErr error) {
	v := endView{Campaign: id, MatchID: matchID, Company: companyID, Card: card, Msg: "Undone."}
	status := http.StatusOK
	switch {
	case undoErr == nil:
	case errors.Is(undoErr, core.ErrNotFound):
		v.Msg = "That entry is gone already."
	case errors.Is(undoErr, core.ErrRefused):
		status, v.Msg, v.Error = http.StatusUnprocessableEntity, reason(undoErr), true
	default:
		s.serverError(w, r, undoErr)
		return
	}
	s.endAnswer(w, r, v, status)
}
