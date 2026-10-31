package web

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

type glossaryView struct {
	pageBase
	Glossary core.Glossary
	Custom   bool       // the glossary is one the visitor picked, not the app's default
	Picker   pickerView // the "use another glossary" choice
	Notice   string     // why the default is shown although another one was picked
	Error    string     // why the picked file was refused
}

// glossaryCookie remembers the glossary the sidebar page shows (the hash of the file), per browser.
const glossaryCookie = "bcm-glossary"

var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *Server) setGlossaryCookie(w http.ResponseWriter, hash string) {
	c := &http.Cookie{Name: glossaryCookie, Value: hash, Path: "/", MaxAge: 31536000, SameSite: http.SameSiteLaxMode, Secure: s.cfg.SecureCookies}
	if hash == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

// glossaryPage renders the sidebar's Glossary: the glossary of the bcm-glossary cookie, or the app's default. An unknown or
// malformed hash falls back to the default with a notice and the cookie is dropped.
func (s *Server) glossaryPage(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	view := glossaryView{pageBase: s.base(r), Error: errMsg}
	def, err := s.rulesets.Default(r.Context(), core.KindGlossary)
	if s.fail(w, r, err) {
		return
	}
	view.Picker = pickerView{Kind: core.KindGlossary, Label: core.KindGlossary.Label(), Default: def}
	if c, cerr := r.Cookie(glossaryCookie); cerr == nil && c.Value != "" {
		if hashRE.MatchString(c.Value) {
			view.Glossary, err = s.glossary.ByHash(r.Context(), c.Value)
		} else {
			err = core.ErrNotFound
		}
		switch {
		case err == nil:
			view.Custom = true
		case errors.Is(err, core.ErrNotFound):
			view.Notice = "The glossary you picked is not available any more, so the app's default is shown."
			s.setGlossaryCookie(w, "")
		default:
			s.serverError(w, r, err)
			return
		}
	}
	if !view.Custom {
		if view.Glossary, err = s.glossary.Default(r.Context()); s.fail(w, r, err) {
			return
		}
	}
	s.render.page(w, r, status, "glossary", view)
}

// handleGlossary is the sidebar's Glossary.
func (s *Server) handleGlossary(w http.ResponseWriter, r *http.Request) {
	s.glossaryPage(w, r, http.StatusOK, "")
}

// handleChooseGlossary keeps the glossary picked with the rules file picker (a known file, or an upload) in the cookie; the
// app's default clears it.
func (s *Server) handleChooseGlossary(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		s.refuse(w, r, http.StatusRequestEntityTooLarge, "the request is too large or malformed")
		return
	}
	hash, err := s.rulesetChoice(r, core.KindGlossary)
	switch {
	case errors.Is(err, core.ErrInvalidRuleset):
		s.glossaryPage(w, r, http.StatusUnprocessableEntity, reason(err))
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	s.setGlossaryCookie(w, hash)
	redirect(w, r, "/glossary")
}

type campaignGlossaryView struct {
	campaignBase
	Glossary core.Glossary
	UnitsURL string // where the unit cards are fetched from the first time they are wanted
}

// handleCampaignGlossary is the Glossary tab of a campaign: the glossary the campaign is played with.
func (s *Server) handleCampaignGlossary(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "glossary")
	if !ok {
		return
	}
	g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID)
	if s.fail(w, r, err) {
		return
	}
	s.render.page(w, r, http.StatusOK, "campaign_glossary", campaignGlossaryView{campaignBase: base, Glossary: g, UnitsURL: fmt.Sprintf("/campaigns/%d/glossary/units", base.Campaign.ID)})
}

// handleCampaignGlossaryUnits is the unit cards of the campaign's glossary, as the fragment the Glossary tab puts where its placeholder is
// (they are 0.5 MB of HTML, so the page leaves them out until they are wanted). Same access as the tab: a member of the campaign.
func (s *Server) handleCampaignGlossaryUnits(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.campaigns.Get(r.Context(), userFrom(r.Context()).ID, id); s.fail(w, r, err) {
		return
	}
	g, err := s.glossary.ForCampaign(r.Context(), id)
	if s.fail(w, r, err) {
		return
	}
	if len(g.Units) == 0 {
		s.notFound(w, r)
		return
	}
	s.render.partial(w, http.StatusOK, "campaign_glossary", "glossary_units", g)
}
