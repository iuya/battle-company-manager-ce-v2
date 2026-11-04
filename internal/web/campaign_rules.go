package web

import (
	"errors"
	"net/http"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The Rules files card of the overview (docs/admin.md, "Rules files of a campaign"): "Check the update" shows what moving the campaign to
// the app's current rules files would do to every company; the button that moves it appears only when nothing blocks.

// rulesUpdateCard is what the card's report shows. Done is the outcome of a move that just happened; Error the refusal of the last request.
type rulesUpdateCard struct {
	Campaign core.CampaignDetail
	Update   core.RulesUpdate
	Done     bool
	Error    string
}

func (s *Server) rulesUpdateManager(w http.ResponseWriter, r *http.Request) (campaignBase, core.User, bool) {
	base, ok := s.loadCampaign(w, r, "overview")
	if !ok {
		return base, core.User{}, false
	}
	user := *userFrom(r.Context())
	if !core.CanManageCampaign(user, base.Campaign.OrganiserID) {
		s.refuse(w, r, http.StatusForbidden, "Only the organiser of the campaign or an administrator can move it to other rules files.")
		return base, user, false
	}
	return base, user, true
}

// handleRulesUpdateCheck answers the report of "Check the update": a swap of the card, nothing is changed.
func (s *Server) handleRulesUpdateCheck(w http.ResponseWriter, r *http.Request) {
	base, user, ok := s.rulesUpdateManager(w, r)
	if !ok {
		return
	}
	u, err := s.campaigns.RulesUpdatePreview(r.Context(), user, base.Campaign.ID)
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, http.StatusOK, "campaign_overview", "rules_update_report", rulesUpdateCard{Campaign: base.Campaign, Update: u})
}

// handleRulesUpdate moves the campaign. Success reloads the overview (the Rules list shows the new files and the Default badges); a refusal
// shows the report again with its reason.
func (s *Server) handleRulesUpdate(w http.ResponseWriter, r *http.Request) {
	base, user, ok := s.rulesUpdateManager(w, r)
	if !ok {
		return
	}
	err := s.campaigns.UpdateRules(r.Context(), user, base.Campaign.ID, r.PostFormValue("switch_custom") == "on")
	switch {
	case err == nil:
		redirect(w, r, "/campaigns/"+itoa(int(base.Campaign.ID)))
	case errors.Is(err, core.ErrRefused):
		u, perr := s.campaigns.RulesUpdatePreview(r.Context(), user, base.Campaign.ID)
		if s.fail(w, r, perr) {
			return
		}
		s.render.partial(w, http.StatusUnprocessableEntity, "campaign_overview", "rules_update_report", rulesUpdateCard{Campaign: base.Campaign, Update: u, Error: reason(err)})
	default:
		s.fail(w, r, err)
	}
}
