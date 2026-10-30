package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// liveToast is the message after a live action: what was recorded, with the way back, or why it was refused.
type liveToast struct {
	Text     string
	Error    bool
	EntryID  int64 // the entry to undo; 0 for none
	CanUndo  bool
	MatchID  int64
	Company  int64 // the page's company, sent along with the undo
	Campaign int64
}

func formInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue(name)))
	return n
}

func formID(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(r.PostFormValue(name)), 10, 64)
	return n
}

// liveActionFrom reads the action a form posts (docs/live-match.md "Go contracts": the fields by kind).
func liveActionFrom(r *http.Request) (core.LiveAction, error) {
	a := core.LiveAction{Kind: core.LiveKind(r.PostFormValue("kind")), UnitID: formID(r, "unit"), Note: strings.TrimSpace(r.PostFormValue("note"))}
	switch a.Kind {
	case core.LiveStatChange:
		a.Stat = r.PostFormValue("stat")
		if v := strings.TrimSpace(r.PostFormValue("set")); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				return a, core.Refuse("Enter a whole number.")
			}
			a.Set = &n
		} else {
			a.Delta = formInt(r, "delta")
		}
	case core.LiveFate:
		a.Count = max(0, formInt(r, "count")) // 0: the milestone, no Fate spent
	case core.LiveRetreat:
		a.Why = r.PostFormValue("why")
	case core.LiveMount:
		a.Mounted = r.PostFormValue("mounted") == "1"
	case core.LiveWound:
		a.Targets = []core.WoundTarget{{UnitID: formID(r, "target"), Part: r.PostFormValue("part"), Caused: formInt(r, "caused"), SavedByFate: formInt(r, "saved")}}
	case core.LiveBroken:
		a.StandFast = r.PostFormValue("standFast") == "1"
	case core.LiveTurn:
		a.Turn = formInt(r, "turn")
	case core.LiveBreakAdjust:
		a.Field, a.Delta = r.PostFormValue("field"), formInt(r, "delta")
	case core.LiveCourageSkip:
		a.Reason = r.PostFormValue("reason")
	case core.LiveAllIsLost:
		a.Flee = r.PostFormValue("flee") == "1"
	case core.LiveObjective:
		a.On = r.PostFormValue("on") == "1"
	case core.LiveItemUsed:
		a.Item, a.Index = r.PostFormValue("item"), formInt(r, "index")
	case core.LiveDied, core.LiveReturn:
	default:
		return a, core.Refuse("Unknown action.")
	}
	return a, nil
}

// handleLiveAction records an action of a unit of {company} in the open match and answers the company's zones (the card, the
// zones and a toast with Undo). Not wrapped in once(): +1 pressed twice means two entries.
func (s *Server) handleLiveAction(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	matchID, ok := s.idParam(w, r, "match")
	if !ok {
		return
	}
	companyID, ok := s.idParam(w, r, "company")
	if !ok {
		return
	}
	user := *userFrom(r.Context())
	toast := liveToast{MatchID: matchID, Company: companyID, Campaign: id}
	action, err := liveActionFrom(r)
	if err == nil {
		var entry core.LiveEntry
		_, entry, err = s.campaigns.Apply(r.Context(), user, id, matchID, companyID, action)
		if err == nil {
			toast.Text, toast.EntryID, toast.CanUndo = entry.Text, entry.ID, entry.CanUndo || entry.ID != 0
		}
	}
	s.liveAnswer(w, r, id, companyID, toast, err)
}

// handleLiveUndo removes an entry of the log; {company} is the page the viewer is on.
func (s *Server) handleLiveUndo(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	matchID, ok := s.idParam(w, r, "match")
	if !ok {
		return
	}
	entryID, ok := s.idParam(w, r, "entry")
	if !ok {
		return
	}
	companyID := formID(r, "company")
	toast := liveToast{MatchID: matchID, Company: companyID, Campaign: id}
	_, err := s.campaigns.Undo(r.Context(), *userFrom(r.Context()), id, matchID, entryID, companyID)
	if r.PostFormValue("from") == "secret" {
		s.answerSecretUndo(w, r, id, matchID, err)
		return
	}
	if from := r.PostFormValue("from"); from == "end" || from == "endcard" {
		s.answerEndUndo(w, r, id, matchID, companyID, from == "endcard", err)
		return
	}
	if from := r.PostFormValue("from"); from == "log" || from == "card" { // pressed in the log: it answers with the log
		s.answerLogUndo(w, r, id, matchID, companyID, from == "card", err)
		return
	}
	if err == nil {
		toast.Text = "Undone."
	}
	s.liveAnswer(w, r, id, companyID, toast, err)
}

// liveAnswer renders the zones of the page's company with the toast: the refusal (422, the state is shown as it is), or the
// message of what was done. A viewer who may not write gets 403, a gone entry or match 404.
func (s *Server) liveAnswer(w http.ResponseWriter, r *http.Request, id, companyID int64, toast liveToast, err error) {
	status := http.StatusOK
	switch {
	case err == nil:
	case errors.Is(err, core.ErrNotFound):
		s.notFound(w, r)
		return
	case errors.Is(err, core.ErrRefused), errors.Is(err, core.ErrInvalidLive):
		if lc, lerr := s.campaigns.Live(r.Context(), *userFrom(r.Context()), id, companyID); lerr == nil && lc.Access == core.LiveNone {
			s.refuse(w, r, http.StatusForbidden, reason(err))
			return
		}
		status, toast = http.StatusUnprocessableEntity, liveToast{Text: reason(err), Error: true, MatchID: toast.MatchID, Company: toast.Company, Campaign: toast.Campaign}
	default:
		s.serverError(w, r, err)
		return
	}
	base, ok := s.loadCampaign(w, r, "my-company")
	if !ok {
		return
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), id, companyID)
	if s.fail(w, r, err) {
		return
	}
	view, ok := s.liveFragmentView(w, r, base, detail)
	if !ok {
		return
	}
	view.Toast = &toast
	s.render.partial(w, status, "campaign_my_company", "live_response", view)
}
