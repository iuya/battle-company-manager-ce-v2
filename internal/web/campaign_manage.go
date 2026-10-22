package web

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The Manage campaign card of the overview (docs/admin.md "Campaign status: pause and end"): pause and resume in one step, ending in
// a series of confirmations, reactivating for administrators; and the invitation controls next to "Invite players".

// manageCard is what the card shows; Error is the refusal of the last request.
type manageCard struct {
	Campaign core.CampaignDetail
	Admin    bool // may reactivate a finished campaign
	Error    string
}

func isAdmin(u core.User) bool { return u.Role == core.RoleAdmin || u.Role == core.RoleSuperuser }

// CanManage is whether the viewer sees the Manage campaign card: the organiser and administrators (a finished campaign too: it says so).
func (b campaignBase) CanManage() bool {
	return b.User != nil && core.CanManageCampaign(*b.User, b.Campaign.OrganiserID)
}

// IsAdmin is whether the viewer is an administrator or superuser.
func (b campaignBase) IsAdmin() bool { return b.User != nil && isAdmin(*b.User) }

// handleCampaignStatus changes the status (to=paused|active|finished, confirm = the name for ending). Success reloads the overview,
// so the badge, the card and the other tabs show the new state; a refusal is shown on the card.
func (s *Server) handleCampaignStatus(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "overview")
	if !ok {
		return
	}
	user := *userFrom(r.Context())
	if !core.CanManageCampaign(user, base.Campaign.OrganiserID) {
		s.refuse(w, r, http.StatusForbidden, "Only the organiser of the campaign or an administrator can change its status.")
		return
	}
	err := s.campaigns.SetCampaignStatus(r.Context(), user, base.Campaign.ID, core.CampaignStatus(r.PostFormValue("to")), r.PostFormValue("confirm"))
	switch {
	case err == nil:
		redirect(w, r, "/campaigns/"+itoa(int(base.Campaign.ID)))
	case errors.Is(err, core.ErrRefused):
		s.render.partial(w, http.StatusUnprocessableEntity, "campaign_overview", "manage_card", manageCard{Campaign: base.Campaign, Admin: isAdmin(user), Error: reason(err)})
	default:
		s.fail(w, r, err)
	}
}

// inviteAnswer renders the invite block again; base is the campaign as loaded before the change.
func (s *Server) inviteAnswer(w http.ResponseWriter, r *http.Request, status int, card inviteCard) {
	s.render.partial(w, status, "campaign_overview", "invite_card", card)
}

func (s *Server) inviteBase(w http.ResponseWriter, r *http.Request) (campaignBase, inviteCard, bool) {
	base, ok := s.loadCampaign(w, r, "overview")
	if !ok {
		return base, inviteCard{}, false
	}
	if !base.CanInvite() {
		s.refuse(w, r, http.StatusForbidden, "Only the organiser of the campaign can manage its invitations.")
		return base, inviteCard{}, false
	}
	return base, inviteCard{CampaignID: base.Campaign.ID, Active: base.Campaign.ActiveInvites, Paused: base.Campaign.Status == core.CampaignPaused}, true
}

// handleInvalidateInvites makes every link stop working; the players who joined stay.
func (s *Server) handleInvalidateInvites(w http.ResponseWriter, r *http.Request) {
	base, card, ok := s.inviteBase(w, r)
	if !ok {
		return
	}
	n, err := s.campaigns.InvalidateInvites(r.Context(), *userFrom(r.Context()), base.Campaign.ID)
	switch {
	case errors.Is(err, core.ErrRefused):
		card.Error = reason(err)
		s.inviteAnswer(w, r, http.StatusUnprocessableEntity, card)
		return
	case s.fail(w, r, err):
		return
	}
	card.Active, card.Notice = 0, "No link works any more."
	if n > 0 {
		card.Notice = itoa(n) + " link" + map[bool]string{true: "", false: "s"}[n == 1] + " stopped working. The players who joined stay."
	}
	s.inviteAnswer(w, r, http.StatusOK, card)
}

// handleRegenerateInvite revokes the links and shows one new link, once.
func (s *Server) handleRegenerateInvite(w http.ResponseWriter, r *http.Request) {
	base, card, ok := s.inviteBase(w, r)
	if !ok {
		return
	}
	inv, err := s.campaigns.RegenerateInvite(r.Context(), *userFrom(r.Context()), base.Campaign.ID)
	switch {
	case errors.Is(err, core.ErrRefused):
		card.Error = reason(err)
		s.inviteAnswer(w, r, http.StatusUnprocessableEntity, card)
		return
	case s.fail(w, r, err):
		return
	}
	card.Active, card.Link, card.Expires = 1, inviteURL(r, inv.Token), inv.ExpiresAt
	card.Notice = "The earlier links stopped working."
	s.inviteAnswer(w, r, http.StatusOK, card)
}

func inviteURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/join/" + url.PathEscape(token)
}
