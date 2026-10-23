package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

const liveLogPage = 30

// liveLogView is a page of the match log: the sheet of a company page, or the card of the match page. Newest first.
type liveLogView struct {
	Campaign int64
	MatchID  int64
	Company  int64 // the company page the viewer is on (undo refreshes its zones); 0 on the match page
	Entries  []liveLogRow
	Older    int64 // seq to page from when there are older entries; 0 when this is the last page
	Error    string
	Ended    bool
	Card     bool // the match page's card, not the sheet
	More     bool // rows only (a later page appended to the list)
}

// liveLogRow is an entry with what the page prints around it.
type liveLogRow struct {
	core.LiveEntry
	Company string // the acting company
	Hue     int    // a stable colour per company
	Heading string // set on the first entry of a minute
}

func hueOf(id int64) int { return int(id*47) % 360 }

// logView reads a page of the log of the match (newest first), with the company names and the minute headings.
func (s *Server) logView(r *http.Request, id, matchID, before int64) (liveLogView, error) {
	m, err := s.campaigns.Match(r.Context(), id, matchID)
	if err != nil {
		return liveLogView{}, err
	}
	return s.logViewOf(r, id, m, before)
}

// logViewOf is logView for a match the caller has loaded already.
func (s *Server) logViewOf(r *http.Request, id int64, m core.Match, before int64) (liveLogView, error) {
	user, matchID := *userFrom(r.Context()), m.ID
	entries, err := s.campaigns.Log(r.Context(), user, id, matchID, before, liveLogPage)
	if err != nil {
		return liveLogView{}, err
	}
	names := map[int64]string{}
	for _, side := range m.Sides {
		for _, co := range side.Companies {
			names[co.ID] = co.Name
		}
	}
	v := liveLogView{Campaign: id, MatchID: matchID, Ended: m.Ended}
	var last string
	for _, e := range entries {
		row := liveLogRow{LiveEntry: e, Company: names[e.CompanyID], Hue: hueOf(e.CompanyID)}
		if h := logHeading(e.On, locationFrom(r.Context())); h != last {
			row.Heading, last = h, h
		}
		v.Entries = append(v.Entries, row)
	}
	if len(entries) == liveLogPage {
		v.Older = entries[len(entries)-1].Seq
	}
	return v, nil
}

// logHeading is the minute an entry belongs to ("14:32", with the date when it is not today).
func logHeading(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return ""
	}
	t = t.In(loc)
	if now := time.Now().In(loc); t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return t.Format("15:04")
	}
	return t.Format("2 Jan 15:04")
}

// handleLiveLog answers the match log: the sheet (for a company page), the card of the match page (?card=1) or, with ?before=, the
// next older rows. A member of the campaign may read it, also after the match ended.
func (s *Server) handleLiveLog(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	matchID, ok := s.idParam(w, r, "match")
	if !ok {
		return
	}
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	v, err := s.logView(r, id, matchID, before)
	if s.fail(w, r, err) {
		return
	}
	v.Card = r.URL.Query().Get("card") == "1"
	v.Company, _ = strconv.ParseInt(r.URL.Query().Get("company"), 10, 64)
	v.More = before > 0
	name := "live_log_sheet"
	switch {
	case v.More:
		name = "live_log_rows"
	case v.Card:
		name = "live_log_card"
	}
	s.render.partial(w, http.StatusOK, "match", name, v)
}

// answerLogUndo is the answer to an Undo pressed in the log: the log again (with the reason when it was refused) and, on a company
// page, its zones out of band.
func (s *Server) answerLogUndo(w http.ResponseWriter, r *http.Request, id, matchID, companyID int64, card bool, undoErr error) {
	status := http.StatusOK
	msg := ""
	switch {
	case undoErr == nil:
	case errors.Is(undoErr, core.ErrNotFound):
		msg = "That entry is gone already."
	case errors.Is(undoErr, core.ErrRefused):
		status, msg = http.StatusUnprocessableEntity, reason(undoErr)
	default:
		s.serverError(w, r, undoErr)
		return
	}
	v, err := s.logView(r, id, matchID, 0)
	if s.fail(w, r, err) {
		return
	}
	v.Company, v.Error, v.Card = companyID, msg, card
	name := "live_log_sheet"
	if card {
		name = "live_log_card"
	}
	parts := []renderPart{{"match", name, v}}
	if companyID != 0 && undoErr == nil { // the zones of the company page, swapped out of band
		if base, ok := s.loadCampaign(w, r, "my-company"); ok {
			if detail, err := s.campaigns.CompanyDetail(r.Context(), id, companyID); err == nil {
				if view, err := s.liveFragmentViewQuiet(r, base, detail); err == nil && view.Live != nil {
					view.OOB = true
					parts = append(parts, renderPart{"campaign_my_company", "live_zones", view})
				}
			}
		} else {
			return
		}
	}
	s.render.partials(w, status, parts...)
}
