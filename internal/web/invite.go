package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Invite links (docs/architecture.md): the organiser makes a link on the campaign page, anybody signed in who opens it and
// confirms becomes a member and can create their company.

type inviteCard struct {
	CampaignID int64
	Link       string
	Expires    time.Time
	Error      string
	Active     int    // the links that still work (they cannot be listed: only counted)
	Paused     bool   // the campaign is paused: no new link, but old ones can be invalidated
	Notice     string // what the last request did
}

// handleInvite makes a new link and answers the card that shows it (the secret is shown once: only its hash is kept).
func (s *Server) handleInvite(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "overview")
	if !ok {
		return
	}
	card := inviteCard{CampaignID: base.Campaign.ID, Active: base.Campaign.ActiveInvites, Paused: base.Campaign.Status == core.CampaignPaused}
	if !base.CanInvite() {
		s.refuse(w, r, http.StatusForbidden, "Only the organiser of the campaign can invite players.")
		return
	}
	inv, err := s.campaigns.Invite(r.Context(), *userFrom(r.Context()), base.Campaign.ID)
	switch {
	case errors.Is(err, core.ErrRefused):
		card.Error = reason(err)
		s.render.partial(w, http.StatusUnprocessableEntity, "campaign_overview", "invite_card", card)
		return
	case s.fail(w, r, err):
		return
	}
	card.Link, card.Expires, card.Active = inviteURL(r, inv.Token), inv.ExpiresAt, card.Active+1
	s.render.partial(w, http.StatusOK, "campaign_overview", "invite_card", card)
}

type joinView struct {
	pageBase
	Token   string
	Error   string
	Preview core.InvitePreview
}

const invalidInvite = "This invitation link is not valid any more: it has expired, was used up or never existed. Ask the organiser for a new one."

// handleJoinPage asks a signed-in visitor to confirm joining (opening a link never changes anything); anybody else is sent to
// sign in first and comes back here.
func (s *Server) handleJoinPage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if userFrom(r.Context()) == nil {
		redirect(w, r, "/login?next="+url.QueryEscape("/join/"+url.PathEscape(token)))
		return
	}
	view := joinView{pageBase: s.base(r), Token: token}
	preview, err := s.campaigns.Preview(r.Context(), *userFrom(r.Context()), token)
	switch {
	case errors.Is(err, core.ErrNotFound):
		s.render.page(w, r, http.StatusNotFound, "join", joinView{pageBase: view.pageBase, Token: token, Error: invalidInvite})
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	view.Preview = preview
	s.render.page(w, r, http.StatusOK, "join", view)
}

// handleJoin redeems the link. An unknown, expired or used-up one is shown as such.
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	id, err := s.campaigns.Join(r.Context(), *userFrom(r.Context()), token)
	switch {
	case errors.Is(err, core.ErrNotFound):
		s.render.page(w, r, http.StatusNotFound, "join", joinView{pageBase: s.base(r), Token: token, Error: invalidInvite})
		return
	case errors.Is(err, core.ErrRefused):
		s.render.page(w, r, http.StatusUnprocessableEntity, "join", joinView{pageBase: s.base(r), Token: token, Error: reason(err)})
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	redirect(w, r, "/campaigns/"+strconv.FormatInt(id, 10))
}
