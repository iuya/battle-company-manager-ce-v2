package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Handing a campaign over (docs/auth.md): the organiser, or an administrator, makes another player the organiser from the
// campaign overview. The players offered are all the members except the organiser (Campaigns.Members), also the ones
// without a company yet.

type handoverCandidate struct {
	ID   int64
	Name string
}

type handoverCard struct {
	CampaignID int64
	Players    []handoverCandidate
	Error      string
}

func (s *Server) handoverCandidates(r *http.Request, id, organiser int64) ([]handoverCandidate, error) {
	members, err := s.campaigns.Members(r.Context(), id)
	if err != nil {
		return nil, err
	}
	var out []handoverCandidate
	for _, m := range members {
		if m.Organiser || m.UserID == organiser {
			continue
		}
		name := m.Name
		if len(m.Companies) > 0 {
			names := make([]string, len(m.Companies))
			for i, c := range m.Companies {
				names[i] = c.Name
			}
			name += " (" + strings.Join(names, ", ") + ")"
		} else {
			name += " (no company yet)"
		}
		out = append(out, handoverCandidate{m.UserID, name})
	}
	return out, nil
}

func (s *Server) handleHandOver(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "overview")
	if !ok {
		return
	}
	if !base.CanInvite() { // the same people: the organiser and administrators, until the campaign is finished
		s.refuse(w, r, http.StatusForbidden, "Only the organiser or an administrator can hand the campaign over.")
		return
	}
	card := handoverCard{CampaignID: base.Campaign.ID}
	to, err := strconv.ParseInt(r.PostFormValue("organiser"), 10, 64)
	if err == nil {
		err = s.campaigns.HandOver(r.Context(), *userFrom(r.Context()), base.Campaign.ID, to)
	} else {
		err = core.Refuse("Choose the player who takes over")
	}
	if errors.Is(err, core.ErrRefused) {
		card.Error = reason(err)
		var lerr error
		if card.Players, lerr = s.handoverCandidates(r, base.Campaign.ID, base.Campaign.OrganiserID); s.fail(w, r, lerr) {
			return
		}
		s.render.partial(w, http.StatusUnprocessableEntity, "campaign_overview", "handover_card", card)
		return
	}
	if s.fail(w, r, err) {
		return
	}
	// The viewer may no longer have the organiser's controls: draw the whole page again.
	redirect(w, r, "/campaigns/"+strconv.FormatInt(base.Campaign.ID, 10))
}
