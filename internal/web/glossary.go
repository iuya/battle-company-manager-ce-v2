package web

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

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
	s.render.partial(w, http.StatusOK, "campaign_glossary", "glossary_units", newUnitCardsView(g, id))
}

// unitCardsView is the unit cards of a campaign's glossary with their wargear, rules, options and text linked like the unit cards of the company page.
type unitCardsView struct {
	CID   int64
	Units []unitCardView
}

// unitCardView is a base unit card with its names turned into chips that carry the tooltip and the helper popup, and its own text linked.
type unitCardView struct {
	core.BaseUnit
	Wargear []chipView
	Rules   []chipView
	Options []optionChipView
	Members []unitCardView // the cards of the units a bundle is made of (Vault Warden Team), each with its own profile and points
	Text    []segView      // the company's own words, with the glossary terms and the other units linked; never the unit's own name
}

// chipView is a name on a card (a rule, a piece of wargear) with what the glossary says about it: Tip is empty for a name the glossary does not define.
type chipView struct {
	Name, Meta, Tip string
}

type optionChipView struct {
	Chip   chipView
	Points int
}

func newUnitCardsView(g core.Glossary, cid int64) unitCardsView {
	v := unitCardsView{CID: cid, Units: make([]unitCardView, len(g.Units))}
	for i, u := range g.Units {
		c := unitCardView{BaseUnit: u}
		for _, n := range u.Preview.Wargear {
			c.Wargear = append(c.Wargear, mountChip(termChip(g, n), u.Mounts))
		}
		for _, n := range u.Preview.Rules {
			c.Rules = append(c.Rules, termChip(g, n))
		}
		for _, o := range u.Options {
			c.Options = append(c.Options, optionChipView{Chip: mountChip(termChip(g, o.Name), u.Mounts), Points: o.Points})
		}
		if u.Text != "" {
			c.Text = viewSegments(g, g.LinkExcept(u.Text, u.Name), 320)
		}
		v.Units[i] = c
	}
	byID := map[string]unitCardView{}
	for _, c := range v.Units {
		byID[c.ID] = c
	}
	for i, c := range v.Units {
		for _, m := range c.Includes {
			if mc, ok := byID[core.UnitAnchor(m)]; ok {
				v.Units[i].Members = append(v.Units[i].Members, mc)
			}
		}
	}
	return v
}

// termChip is a name with the glossary term it stands for (the way a rule names it, parameter and all), as the tooltip of a chip.
func termChip(g core.Glossary, name string) chipView {
	c := chipView{Name: name}
	if t, ok := g.Find(name); ok {
		c.Meta, c.Tip = t.Category, t.Summary(320)
	}
	return c
}

// mountChip adds a mount's stat block and special rules to the tooltip of its chip, as the company page's tooltip of the mount shows them.
func mountChip(c chipView, mounts []core.BaseMount) chipView {
	for _, m := range mounts {
		if !strings.EqualFold(m.Name, c.Name) {
			continue
		}
		var lines []string
		if len(m.Stats) > 0 {
			parts := make([]string, len(m.Stats))
			for i, a := range m.Stats {
				parts[i] = a.Abbr + " " + strconv.Itoa(a.Value) + a.Trail
			}
			lines = append(lines, strings.Join(parts, " · "))
		}
		if len(m.Rules) > 0 {
			lines = append(lines, "Special rules: "+strings.Join(m.Rules, ", "))
		}
		if c.Tip != "" {
			lines = append(lines, c.Tip)
		}
		c.Meta, c.Tip = "Mount", strings.Join(lines, "\n\n")
	}
	return c
}
