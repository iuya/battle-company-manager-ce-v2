package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// campaignBase is embedded in the data of every page inside a campaign: the header with the tabs needs it.
type campaignBase struct {
	pageBase
	Campaign core.CampaignDetail
	Tab      string // overview, companies, rankings or matches; empty for pages below a tab
}

// idParam reads a numeric path value; ok is false (and a 404 sent) when it is not a number.
func (s *Server) idParam(w http.ResponseWriter, r *http.Request, name string) (id int64, ok bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		s.notFound(w, r)
		return 0, false
	}
	return id, true
}

// loadCampaign resolves {id} for the signed-in user; it answers 404 (or 500) itself when it cannot.
func (s *Server) loadCampaign(w http.ResponseWriter, r *http.Request, tab string) (campaignBase, bool) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return campaignBase{}, false
	}
	c, err := s.campaigns.Get(r.Context(), userFrom(r.Context()).ID, id)
	if s.fail(w, r, err) {
		return campaignBase{}, false
	}
	return campaignBase{pageBase: s.base(r), Campaign: c, Tab: tab}, true
}

// fail answers 404 for core.ErrNotFound and 500 for anything else; it reports whether err was non-nil.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrNotFound):
		s.notFound(w, r)
	default:
		s.serverError(w, r, err)
	}
	return true
}

// CanInvite is whether the viewer may make an invite link: the organiser and administrators, until the campaign is finished.
func (b campaignBase) CanInvite() bool {
	u := b.User
	return u != nil && b.Campaign.Status != core.CampaignFinished &&
		core.CanManageCampaign(*u, b.Campaign.OrganiserID)
}

type overviewView struct {
	campaignBase
	Recent []core.Match
	// NewOrganisers are the players the viewer could hand the campaign over to (empty when they may not hand over).
	NewOrganisers []handoverCandidate
}

type companiesView struct {
	campaignBase
	Companies []core.CampaignCompany
}

type rankingsView struct {
	campaignBase
	Rows []core.RankedCompany
}

type matchesView struct {
	campaignBase
	Matches   []core.Match
	Companies int // how many companies the campaign has: a match needs at least two
}

// minMatchCompanies is how many companies a campaign needs before a match can be opened.
const minMatchCompanies = 2

// CanOpenMatch is whether the page offers New match: the campaign is active (CanStartMatch) and has companies to play it.
func (v matchesView) CanOpenMatch() bool {
	return v.Campaign.CanStartMatch() == nil && v.Companies >= minMatchCompanies
}

// handleCampaign serves the four tabs of a campaign; each loads only what it shows.
func (s *Server) handleCampaign(page, tab string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base, ok := s.loadCampaign(w, r, tab)
		if !ok {
			return
		}
		ctx, id := r.Context(), base.Campaign.ID
		var view any = base // tabs with nothing of their own to load (the armory)
		switch tab {
		case "overview":
			matches, err := s.campaigns.Matches(ctx, id)
			if s.fail(w, r, err) {
				return
			}
			over := overviewView{campaignBase: base, Recent: matches[:min(len(matches), 3)]}
			if base.CanInvite() {
				if over.NewOrganisers, err = s.handoverCandidates(r, base.Campaign.ID, base.Campaign.OrganiserID); s.fail(w, r, err) {
					return
				}
			}
			view = over
		case "companies", "rankings":
			list, err := s.campaigns.Companies(ctx, id)
			if s.fail(w, r, err) {
				return
			}
			if tab == "companies" {
				view = companiesView{base, list}
			} else {
				view = rankingsView{base, core.Rank(list)}
			}
		case "matches":
			matches, err := s.campaigns.Matches(ctx, id)
			if s.fail(w, r, err) {
				return
			}
			list, err := s.campaigns.Companies(ctx, id)
			if s.fail(w, r, err) {
				return
			}
			view = matchesView{base, matches, len(list)}
		}
		s.render.page(w, r, http.StatusOK, page, view)
	}
}

type newCompanyView struct {
	campaignBase
	Profiles []core.CompanyProfile // every company the player can choose, for the list
	Info     *companyInfoView      // the company shown now (the one chosen, or the one a failed creation was for)
	Input    core.CompanyInput
	Error    string
}

func (s *Server) handleNewCompanyPage(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	if base.Campaign.Company != nil { // one company per player: go and manage it
		redirect(w, r, "/campaigns/"+itoa(int(base.Campaign.ID))+"/my-company")
		return
	}
	view, ok := s.newCompanyPage(w, r, base, core.CompanyInput{Faction: r.URL.Query().Get("c")}, "")
	if !ok {
		return
	}
	s.render.page(w, r, http.StatusOK, "company_new", view)
}

func (s *Server) handleCreateCompany(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	in := core.CompanyInput{Name: r.PostFormValue("name"), Faction: r.PostFormValue("faction"), Heroes: startHeroes(r)}
	_, err := s.campaigns.CreateCompany(r.Context(), *userFrom(r.Context()), base.Campaign.ID, in)
	switch {
	case errors.Is(err, core.ErrInvalidCompany) || errors.Is(err, core.ErrHasCompany):
		view, ok := s.newCompanyPage(w, r, base, in, reason(err))
		if !ok {
			return
		}
		s.render.page(w, r, http.StatusUnprocessableEntity, "company_new", view)
	case s.fail(w, r, err):
	default:
		redirect(w, r, "/campaigns/"+itoa(int(base.Campaign.ID))+"/my-company")
	}
}

// newCompanyPage builds the company browser: the list of companies and, when one is chosen, its page.
func (s *Server) newCompanyPage(w http.ResponseWriter, r *http.Request, base campaignBase, in core.CompanyInput, errMsg string) (newCompanyView, bool) {
	profiles, err := s.campaigns.CompanyProfiles(r.Context(), base.Campaign.ID)
	if s.fail(w, r, err) {
		return newCompanyView{}, false
	}
	view := newCompanyView{campaignBase: base, Profiles: profiles, Input: in, Error: errMsg}
	for _, p := range profiles {
		if p.Name == in.Faction {
			info, err := s.companyInfo(r, base.Campaign, p, in.Name, in.Heroes, errMsg)
			if s.fail(w, r, err) {
				return newCompanyView{}, false
			}
			view.Info = &info
		}
	}
	return view, true
}

// handleCompany shows another player's company: the My Company page, read only. A player's own company lives under its tab.
func (s *Server) handleCompany(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "companies")
	if !ok {
		return
	}
	cid, ok := s.idParam(w, r, "company")
	if !ok {
		return
	}
	co, err := s.campaigns.Company(r.Context(), base.Campaign.ID, cid)
	if s.fail(w, r, err) {
		return
	}
	if base.Campaign.Company != nil && base.Campaign.Company.ID == co.ID {
		http.Redirect(w, r, "/campaigns/"+itoa(int(base.Campaign.ID))+"/my-company", http.StatusSeeOther)
		return
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, co.ID)
	if s.fail(w, r, err) {
		return
	}
	g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID)
	if s.fail(w, r, err) {
		return
	}
	view := myCompanyView{campaignBase: base, Company: detail, HasCompany: true, ReadOnly: true, Units: unitViews(g, detail.Roster)}
	if view.Live, err = s.liveFor(r, base, detail, view.Units); s.fail(w, r, err) {
		return
	}
	view.PerksUnused, view.PerksUsed = splitPerks(perkViews(detail.Perks, detail.Roster))
	if view.Grants, err = s.grantsCard(r, base, co.ID, true); s.fail(w, r, err) { // the organiser's award form stays: it is not the company's to change
		return
	}
	s.render.page(w, r, http.StatusOK, "campaign_my_company", view)
}

// segView is a piece of a description: plain text, or a mention of a glossary term with what its preview shows.
type segView struct {
	Text, TermID, TipTitle, TipText string
}

// itemView and ruleView are equipment and special rules with their descriptions cut into segView pieces.
type itemView struct {
	core.Equipment
	Struck   string // in a live match, why the item counts no more ("needs a mount"); empty while it does
	Segments []segView
	TipStats string // a mount's stat block as JSON for the tooltip: [["MV","10\""],...]
}

type ruleView struct {
	core.UnitRule
	Struck   string // in a live match, why the rule counts no more ("the mount is lost"); empty while it does
	Segments []segView
}

// actionView is a heroic action; its rules text comes from the glossary term of the same name.
type actionView struct {
	core.HeroicAction
	TermID   string
	Summary  string // a short text for the hover preview
	Segments []segView
}

// pathView is a hero's path with its heroic actions and the rules it gave.
type pathView struct {
	Name    string
	Actions []actionView
	Rules   []ruleView
	Powers  []ruleView // magical powers learned through the path
}

// unitView is a unit as the company page draws it.
type unitView struct {
	core.CompanyUnit
	CreatureStats string // its creature's stat block as JSON for the tooltip
	CreatureTip   string // and its rules
	Items         []itemView
	Rules         []ruleView
	Path          *pathView // heroes only
}

// linkText cuts text into pieces, linking the glossary terms it mentions.
func linkText(g core.Glossary, text string) []segView {
	var out []segView
	for _, seg := range g.Link(text) {
		v := segView{Text: seg.Text, TermID: seg.TermID}
		if term, ok := g.Term(seg.TermID); ok {
			v.TipTitle, v.TipText = term.Name, term.Summary(320)
		}
		out = append(out, v)
	}
	return out
}

func unitViews(g core.Glossary, units []core.CompanyUnit) []unitView {
	out := make([]unitView, len(units))
	for i, u := range units {
		v := unitView{CompanyUnit: u}
		if u.Creature != nil {
			v.CreatureStats = tipStats(u.Creature.Stats)
			v.CreatureTip = creatureTip(*u.Creature)
		}
		for _, e := range u.Equipment {
			v.Items = append(v.Items, itemView{Equipment: e, Segments: linkText(g, e.Description), TipStats: tipStats(e.MountStats)})
		}
		for _, r := range u.Rules {
			v.Rules = append(v.Rules, ruleView{UnitRule: r, Segments: linkText(g, r.Description)})
		}
		if u.Path != nil {
			pv := &pathView{Name: u.Path.Name}
			for _, a := range u.Path.HeroicActions {
				av := actionView{HeroicAction: a}
				if term, ok := g.Find(a.Name); ok {
					av.TermID, av.Summary = term.ID, term.Summary(320)
					av.Segments = []segView{{Text: term.Summary(700)}}
				}
				pv.Actions = append(pv.Actions, av)
			}
			for _, r := range u.Path.Rules {
				pv.Rules = append(pv.Rules, ruleView{UnitRule: r, Segments: linkText(g, r.Description)})
			}
			for _, r := range u.Path.Powers {
				pv.Powers = append(pv.Powers, ruleView{UnitRule: r, Segments: linkText(g, r.Description)})
			}
			v.Path = pv
		}
		out[i] = v
	}
	return out
}

// myCompanyView is the "My Company" tab: the viewer's company (zero until they create one).
type myCompanyView struct {
	campaignBase
	Company     core.CompanyDetail
	HasCompany  bool
	Units       []unitView
	Notice      string // what the last purchase or recruit did (or why it was refused)
	NoticeError bool
	Pending     *core.RecruitChoice // a recruit waiting for the player to choose its options
	Officers    *core.OfficerPanel  // a Ranger of the North waiting for the officers to be arranged: the window opens by itself on it
	Recruiting  *recruitPanel       // a recruit in progress: the window opens by itself on it
	Grants      grantsCard          // the company's history of awards
	PerksUnused []perkView          // scenario perks waiting to be used
	PerksUsed   []perkView
	ReadOnly    bool       // somebody else's company (or a finished campaign): everything is shown, nothing can be changed
	Frozen      bool       // the campaign has ended: the read only notice says so
	OOB         bool       // the zones are sent out of band (an answer that is about something else)
	Toast       *liveToast // what the last live action did (the live response only)
	Live        *liveView  // the company is in an open match: its live state, zones and banner (nil otherwise)
}

// myCompanyPage renders the My Company tab, optionally with a notice about what was just done.
func (s *Server) myCompanyPage(w http.ResponseWriter, r *http.Request, status int, notice string, noticeErr bool) {
	base, ok := s.loadCampaign(w, r, "my-company")
	if !ok {
		return
	}
	view := myCompanyView{campaignBase: base, Notice: notice, NoticeError: noticeErr}
	if mine := base.Campaign.Company; mine != nil {
		detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, mine.ID)
		if s.fail(w, r, err) {
			return
		}
		g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID) // descriptions link to the campaign's glossary
		if s.fail(w, r, err) {
			return
		}
		view.Company, view.HasCompany, view.Units = detail, true, unitViews(g, detail.Roster)
		view.Frozen = base.Campaign.Status == core.CampaignFinished
		view.ReadOnly = view.Frozen // an ended campaign is frozen: the page shows everything and offers nothing
		if view.Live, err = s.liveFor(r, base, detail, view.Units); s.fail(w, r, err) {
			return
		}
		view.PerksUnused, view.PerksUsed = splitPerks(perkViews(detail.Perks, detail.Roster))
		if view.Grants, err = s.grantsCard(r, base, mine.ID, false); s.fail(w, r, err) { // the form is on the page of the other companies; one does not award one's own
			return
		}
		if view.Pending, err = s.campaigns.PendingChoice(r.Context(), *userFrom(r.Context()), base.Campaign.ID); s.fail(w, r, err) {
			return
		}
		if view.Officers, err = s.campaigns.PendingOfficers(r.Context(), *userFrom(r.Context()), base.Campaign.ID); s.fail(w, r, err) {
			return
		}
		if roll, err := s.campaigns.RecruitState(r.Context(), *userFrom(r.Context()), base.Campaign.ID); s.fail(w, r, err) {
			return
		} else if roll != nil && !view.Frozen {
			panel, err := s.recruitPanelFor(r, base.Campaign, "", false)
			if s.fail(w, r, err) {
				return
			}
			view.Recruiting = &panel
		}
	}
	s.render.page(w, r, status, "campaign_my_company", view)
}

func (s *Server) handleMyCompany(w http.ResponseWriter, r *http.Request) {
	s.myCompanyPage(w, r, http.StatusOK, "", false)
}

// newMatchView feeds the new-match form (static/js/match_form.js): the campaign's companies to search in and the
// sides as they currently are, both as JSON in data attributes.
type newMatchView struct {
	campaignBase
	CompaniesJSON string
	SidesJSON     string
	FFA           bool // all vs all: the first "side" is just the pool of companies, each of which fights alone
	Error         string
	Busy          *core.OpenMatchRef // the open match a refused company is in, to link to
	Companies     int                // how many companies the campaign has; under two there is nobody to match
}

// TooFew is whether the campaign has too few companies for a match.
func (v newMatchView) TooFew() bool { return v.Companies < minMatchCompanies }

func newMatchForm(base campaignBase, companies []core.CampaignCompany, sides []core.NewSide, ffa bool, errMsg string) (newMatchView, error) {
	list := make([]map[string]any, len(companies))
	for i, c := range companies {
		list[i] = map[string]any{"id": c.ID, "name": c.Name, "owner": c.Owner, "faction": c.FactionName()}
	}
	state := make([]map[string]any, len(sides))
	for i, sd := range sides {
		ids := sd.Companies
		if ids == nil {
			ids = []int64{}
		}
		state[i] = map[string]any{"name": sd.Name, "ids": ids, "role": sd.Role}
	}
	cj, err := json.Marshal(list)
	if err != nil {
		return newMatchView{}, err
	}
	sj, err := json.Marshal(state)
	if err != nil {
		return newMatchView{}, err
	}
	return newMatchView{campaignBase: base, CompaniesJSON: string(cj), SidesJSON: string(sj), FFA: ffa, Error: errMsg, Companies: len(companies)}, nil
}

func (s *Server) handleNewMatchPage(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	if err := base.Campaign.CanStartMatch(); err != nil {
		s.refuse(w, r, http.StatusForbidden, reason(err))
		return
	}
	list, err := s.campaigns.Companies(r.Context(), base.Campaign.ID)
	if s.fail(w, r, err) {
		return
	}
	sides := []core.NewSide{{}, {}}
	if mine := base.Campaign.Company; mine != nil {
		sides[0].Companies = []int64{mine.ID} // you are most likely playing
	}
	view, err := newMatchForm(base, list, sides, false, "")
	if s.fail(w, r, err) {
		return
	}
	s.render.page(w, r, http.StatusOK, "match_new", view)
}

func (s *Server) handleCreateMatch(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	if err := base.Campaign.CanStartMatch(); err != nil {
		s.refuse(w, r, http.StatusForbidden, reason(err))
		return
	}
	list, err := s.campaigns.Companies(r.Context(), base.Campaign.ID)
	if s.fail(w, r, err) {
		return
	}
	known := map[int64]bool{}
	for _, c := range list {
		known[c.ID] = true
	}
	ffa := r.PostFormValue("mode") == "ffa"
	n, _ := strconv.Atoi(r.PostFormValue("sides"))
	sides := make([]core.NewSide, max(2, min(n, 8)))
	for i := range sides {
		sides[i].Name = r.PostFormValue("side-name-" + itoa(i+1))
		if role := r.PostFormValue("side-role-" + itoa(i+1)); core.ValidRole(role) && !ffa {
			sides[i].Role = role
		}
		for _, v := range r.PostForm["side-"+itoa(i+1)] {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil && known[id] {
				sides[i].Companies = append(sides[i].Companies, id)
			}
		}
	}
	form := sides
	if ffa { // all vs all: the companies of the pool (side 1) each become a side of their own
		names := map[int64]string{}
		for _, c := range list {
			names[c.ID] = c.Name
		}
		sides = nil
		for _, id := range form[0].Companies {
			sides = append(sides, core.NewSide{Name: names[id], Companies: []int64{id}})
		}
	}
	user := *userFrom(r.Context())
	matchID, err := s.recent.create([]any{"match", user.ID, base.Campaign.ID, sides}, func() (int64, error) {
		m, err := s.campaigns.CreateMatch(r.Context(), user, base.Campaign.ID, sides)
		return m.ID, err
	})
	switch {
	case errors.Is(err, core.ErrInvalidMatch) || errors.Is(err, core.ErrRefused):
		view, jerr := newMatchForm(base, list, form, ffa, reason(err))
		if s.fail(w, r, jerr) {
			return
		}
		var busy *core.CompanyBusyError
		if errors.As(err, &busy) {
			view.Busy = &busy.OpenMatchRef
		}
		s.render.page(w, r, http.StatusUnprocessableEntity, "match_new", view)
	case s.fail(w, r, err):
	default:
		redirect(w, r, "/campaigns/"+itoa(int(base.Campaign.ID))+"/matches/"+itoa(int(matchID)))
	}
}

// tipStats is a stat block as JSON pairs of label and value for the tooltip; empty when there is none.
func tipStats(stats []core.Attribute) string {
	if len(stats) == 0 {
		return ""
	}
	pairs := make([][2]string, len(stats))
	for i, a := range stats {
		pairs[i] = [2]string{a.Abbr, strconv.Itoa(a.Value) + a.Trail}
	}
	b, _ := json.Marshal(pairs)
	return string(b)
}

// creatureTip is the text of a creature's tooltip: its special rules and notes.
func creatureTip(c core.UnitCreatureRef) string {
	var parts []string
	if len(c.Rules) > 0 {
		parts = append(parts, "Special rules: "+strings.Join(c.Rules, ", "))
	}
	return strings.Join(append(parts, c.Notes...), "\n\n")
}
