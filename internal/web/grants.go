package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The Grants card of a company page (docs/matches.md, "Organiser and scenario rewards"): the history of what the organiser
// awarded, newest first, and, for the organiser and administrators, the form that awards something.

// grantForm is what the form shows: the raw text of the fields, so a rejected form keeps what was typed.
type grantForm struct {
	Gold, Influence, Experience, Unit, Note string
}

type grantsCard struct {
	CampaignID, CompanyID int64
	Grants                []core.Grant
	CanGrant              bool
	Roster                []core.CompanyUnit // the units that can get experience; only loaded for the form
	Gold, Influence       int                // what the company has now, shown beside the form
	Names                 map[int64]string   // unit titles by id, for the history
	Form                  grantForm
	Errors                map[string]string // by field; "" for the form
	Notice                string
	NoticeErr             bool
}

// UnitName is the title of the unit an experience grant went to, empty when unknown.
func (c grantsCard) UnitName(id int64) string { return c.Names[id] }

// grantsCard loads the card of a company for the viewer; form says whether the page offers the form to those who may use it. The roster is read only when the form or the history needs a unit's name.
func (s *Server) grantsCard(r *http.Request, base campaignBase, companyID int64, form bool) (grantsCard, error) {
	c := grantsCard{CampaignID: base.Campaign.ID, CompanyID: companyID, CanGrant: form && base.Campaign.CanGrant(*userFrom(r.Context())) == nil}
	var err error
	if c.Grants, err = s.campaigns.Grants(r.Context(), base.Campaign.ID, companyID); err != nil {
		return c, err
	}
	needUnits := c.CanGrant
	for _, g := range c.Grants {
		needUnits = needUnits || g.UnitID != 0
	}
	if needUnits {
		detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, companyID)
		if err != nil {
			return c, err
		}
		c.Names = map[int64]string{}
		for _, u := range detail.Roster {
			c.Names[u.ID] = u.Title()
		}
		if c.CanGrant {
			c.Roster, c.Gold, c.Influence = detail.Roster, detail.Gold, detail.Influence
		}
	}
	return c, nil
}

func (s *Server) handleGrantReward(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	companyID, ok := s.idParam(w, r, "company")
	if !ok {
		return
	}
	user := *userFrom(r.Context())
	if err := base.Campaign.CanGrant(user); err != nil {
		s.refuse(w, r, http.StatusForbidden, reason(err))
		return
	}
	f := grantForm{Gold: r.PostFormValue("gold"), Influence: r.PostFormValue("influence"), Experience: r.PostFormValue("experience"), Unit: r.PostFormValue("unit"), Note: r.PostFormValue("note")}
	in := core.GrantInput{Note: f.Note}
	errs := map[string]string{}
	num := func(field, label, text string, dst *int) {
		if text = strings.TrimSpace(text); text == "" {
			return
		}
		n, err := strconv.Atoi(text)
		if err != nil {
			errs[field] = label + " must be a whole number"
			return
		}
		*dst = n
	}
	num("gold", "Gold", f.Gold, &in.Gold)
	num("influence", "Influence", f.Influence, &in.Influence)
	num("experience", "Experience", f.Experience, &in.Experience)
	in.UnitID, _ = strconv.ParseInt(f.Unit, 10, 64)

	card := func(status int, notice string, mutate func(*grantsCard)) {
		c, err := s.grantsCard(r, base, companyID, true)
		if s.fail(w, r, err) {
			return
		}
		c.Notice = notice
		if mutate != nil {
			mutate(&c)
		}
		s.render.partial(w, status, "campaign_my_company", "grants_card", c)
	}
	rejected := func(status int, notice string) {
		card(status, notice, func(c *grantsCard) { c.Form, c.Errors, c.NoticeErr = f, errs, notice != "" })
	}
	if len(errs) > 0 {
		rejected(http.StatusUnprocessableEntity, "")
		return
	}
	if _, err := s.campaigns.GrantReward(r.Context(), user, base.Campaign.ID, companyID, in); err != nil {
		switch {
		case errors.Is(err, core.ErrInvalidCompany):
			for _, msg := range reasons(err) {
				field := ""
				switch {
				case strings.HasPrefix(msg, "the note"):
					field = "note"
				case strings.HasPrefix(msg, "Gold"):
					field = "gold"
				case strings.HasPrefix(msg, "Influence"):
					field = "influence"
				case strings.HasPrefix(msg, "Experience"):
					field = "experience"
				case strings.HasPrefix(msg, "choose the unit"):
					field = "unit"
				}
				if errs[field] != "" {
					msg = errs[field] + ". " + msg
				}
				errs[field] = msg
			}
			rejected(http.StatusUnprocessableEntity, "")
		case errors.Is(err, core.ErrRefused):
			rejected(http.StatusUnprocessableEntity, reason(err))
		default:
			s.fail(w, r, err)
		}
		return
	}
	card(http.StatusOK, "Reward granted.", nil)
}
