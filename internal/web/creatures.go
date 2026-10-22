package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// handleBuyCreature buys a creature for the officer picked in the armory (or in a unit's equipment window).
func (s *Server) handleBuyCreature(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	owner, _ := strconv.ParseInt(r.PostFormValue("unit"), 10, 64)
	name := r.PostFormValue("name")
	err := s.campaigns.BuyCreature(r.Context(), *userFrom(r.Context()), id, owner, name, r.PostFormValue("pay"))
	if r.PostFormValue("from") == "company" { // bought from a unit's window: back to the company page
		s.afterAction(w, r, "Bought "+name+".", err)
		return
	}
	s.afterArmory(w, r, "Bought "+name+".", err)
}

func (s *Server) handleHireWanderer(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	name := r.PostFormValue("name")
	err := s.campaigns.HireWanderer(r.Context(), *userFrom(r.Context()), id, name, r.PostFormValue("pay"))
	s.afterArmory(w, r, name+" joins the company.", err)
}

// afterArmory shows the Armory again with what happened (or why the rules refused).
func (s *Server) afterArmory(w http.ResponseWriter, r *http.Request, done string, err error) {
	switch {
	case errors.Is(err, core.ErrRefused):
		s.armoryPage(w, r, http.StatusUnprocessableEntity, reason(err), true)
	case s.fail(w, r, err):
	default:
		s.armoryPage(w, r, http.StatusOK, done, false)
	}
}

func (s *Server) handleDismissWanderer(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	err := s.campaigns.DismissWanderer(r.Context(), *userFrom(r.Context()), id, unitID)
	s.afterAction(w, r, "The wanderer leaves the company.", err)
}

// packPanel is the window of the Baggage Pony: what it carries and what each member could store, take or swap.
type packPanel struct {
	Campaign core.CampaignDetail
	Company  core.CompanyDetail
	Pack     core.Pack
	Members  []packMember
	Notice   string
	Error    string
}

// packMember is a company member and the items it could store (the ones that can be dropped and are not mounts).
type packMember struct {
	ID    int64
	Title string
	Items []string
}

func (s *Server) showPackPanel(w http.ResponseWriter, r *http.Request, status int, notice, errMsg string) {
	base, ok := s.loadCampaign(w, r, "my-company")
	if !ok {
		return
	}
	if base.Campaign.Company == nil {
		s.notFound(w, r)
		return
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, base.Campaign.Company.ID)
	if s.fail(w, r, err) {
		return
	}
	if detail.Pack == nil {
		s.notFound(w, r)
		return
	}
	panel := packPanel{Campaign: base.Campaign, Company: detail, Pack: *detail.Pack, Notice: notice, Error: errMsg}
	for _, u := range detail.Roster {
		m := packMember{ID: u.ID, Title: u.Title()}
		for _, e := range u.Equipment {
			if !e.Upgrade && e.Drop == "" && e.Class != core.EquipMount {
				m.Items = append(m.Items, e.Name)
			}
		}
		panel.Members = append(panel.Members, m)
	}
	s.render.partial(w, status, "campaign_my_company", "pack_panel", panel)
}

func (s *Server) handlePackPanel(w http.ResponseWriter, r *http.Request) {
	s.showPackPanel(w, r, http.StatusOK, "", "")
}

func (s *Server) handleCarry(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	unitID, _ := strconv.ParseInt(r.PostFormValue("unit"), 10, 64)
	a := core.CarryAction{Kind: r.PostFormValue("kind"), Item: r.PostFormValue("item"), Take: r.PostFormValue("take")}
	err := s.campaigns.Carry(r.Context(), *userFrom(r.Context()), id, unitID, a)
	switch {
	case errors.Is(err, core.ErrRefused):
		s.showPackPanel(w, r, http.StatusUnprocessableEntity, "", reason(err))
	case s.fail(w, r, err):
	default:
		s.showPackPanel(w, r, http.StatusOK, "Done: "+a.Kind+".", "")
	}
}
