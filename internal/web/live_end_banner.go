package web

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The part of the sticky live banner that every change of the match refreshes (docs/live-match.md "Ending a match while live"): the
// companies that are Broken ("Minas Morgul is broken (8 of 15 casualties)") and the pending end with Confirm, which opens the Confirm
// sheet. Its other buttons post to the match's end routes with view=banner and company=<page's company>, and those answer the banner again.

// endBanner is what the banner shows; zero shows nothing.
type endBanner struct {
	Campaign, MatchID, Company int64
	Revision                   int
	Pending                    bool
	Proposal                   *core.EndProposal
	Result                     string // what the pending end reports: "Rohan won", "Draw"
	Access                     core.EndAccess
	Broken                     []string // "Minas Morgul is broken (8 of 15 casualties)", for every Broken company of the match
	Notice                     string
	NoticeErr                  bool
	OverrideNote, OverrideErr  string
}

// WaitingNames lists the companies that have not confirmed, "Gorgoroth, Rangers".
func (b endBanner) WaitingNames() string {
	var names []string
	for _, c := range b.Proposal.Waiting() {
		names = append(names, c.CompanyName)
	}
	return strings.Join(names, ", ")
}

func (s *Server) endBannerFor(r *http.Request, base campaignBase, m core.Match, company int64) endBanner {
	b := endBanner{Campaign: base.Campaign.ID, MatchID: m.ID, Company: company, Revision: m.Revision}
	if m.EndProposal != nil && !m.Ended {
		b.Pending, b.Proposal, b.Access, b.Result = true, m.EndProposal, endAccess(r, base, m), proposedResult(m)
	}
	if !m.Ended {
		if breaks, err := s.campaigns.MatchBreaks(r.Context(), base.Campaign.ID, m.ID); err == nil { // an extra: a failed read shows none
			for _, co := range m.AllCompanies() {
				if br, ok := breaks[co.ID]; ok && br.Broken {
					b.Broken = append(b.Broken, fmt.Sprintf("%s is broken (%d of %d casualties)", co.Name, br.Casualties, br.StartingModels))
				}
			}
		}
	}
	return b
}

// handleEndBanner answers the banner for the page's refresh (GET ...?company=N).
func (s *Server) handleEndBanner(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	s.answerEndBanner(w, r, http.StatusOK, base, m, queryID(r, "company"), "", false, "", "")
}

func (s *Server) answerEndBanner(w http.ResponseWriter, r *http.Request, status int, base campaignBase, m core.Match, company int64, notice string, isErr bool, note, noteErr string) {
	if m.Ended { // the page becomes the one of an ended match
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
		return
	}
	b := s.endBannerFor(r, base, m, company)
	b.Notice, b.NoticeErr, b.OverrideNote, b.OverrideErr = notice, isErr, note, noteErr
	s.render.partial(w, status, "campaign_my_company", "end_banner", b)
}

// bannerFailed answers a failed write of the banner: the current banner with the reason.
func (s *Server) bannerFailed(w http.ResponseWriter, r *http.Request, base campaignBase, m core.Match, company int64, err error) {
	switch {
	case errors.Is(err, core.ErrMatchChanged):
		cur, merr := s.campaigns.Match(r.Context(), base.Campaign.ID, m.ID)
		if s.fail(w, r, merr) {
			return
		}
		s.answerEndBanner(w, r, http.StatusOK, base, cur, company, "Somebody changed this match meanwhile, so this is its current state.", true, "", "")
	case errors.Is(err, core.ErrRefused):
		if matchAccess(base, m, *userFrom(r.Context())) == core.MatchNoAccess {
			s.refuse(w, r, http.StatusForbidden, reason(err))
			return
		}
		s.answerEndBanner(w, r, http.StatusUnprocessableEntity, base, m, company, reason(err), true, "", "")
	default:
		s.fail(w, r, err)
	}
}

// Fields are the hidden inputs that make the match routes answer the banner.
func (b endBanner) Fields() template.HTML {
	return template.HTML(fmt.Sprintf(`<input type="hidden" name="view" value="banner"><input type="hidden" name="company" value="%d">`, b.Company))
}
