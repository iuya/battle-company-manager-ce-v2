package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// campaignBase is embedded in the data of every page inside a campaign: the header with the tabs needs it.
type campaignBase struct {
	pageBase
	Campaign core.CampaignDetail
	Tab      string // overview, my-company, armory, rankings, matches or glossary; empty for pages below a tab
	// Current is the viewer's company an owner page acts on (see loadOwn); nil on the other pages.
	Current *core.CampaignCompany
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
	s.rememberVisit(w, r, c, 0)
	return campaignBase{pageBase: s.base(r), Campaign: c, Tab: tab}, true
}

// fail answers 404 for core.ErrNotFound, 403 for core.ErrForbidden (with its reason) and 500 for anything else; it reports whether err was non-nil.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, core.ErrNotFound):
		s.notFound(w, r)
	case errors.Is(err, core.ErrForbidden):
		s.refuse(w, r, http.StatusForbidden, reason(err))
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
	Recent   []core.Match
	YourTurn []yourTurn // what waits on the viewer (docs/frontend-campaigns.md, "Matches")
	// NewOrganisers are the players the viewer could hand the campaign over to (empty when they may not hand over).
	NewOrganisers []handoverCandidate
}

type rankingsView struct {
	campaignBase
	Rows []core.RankedCompany
}

type matchesView struct {
	campaignBase
	Matches   []core.Match // what the list shows: every match, or those of the filter
	Companies int          // how many companies the campaign has: a match needs at least two
	// Filter is the chip on (?filter=mine, open or ended; "" for every match).
	Filter   string
	YourTurn []yourTurn
}

// yourTurn is one line of the Your turn list: a match that waits on the viewer.
type yourTurn struct {
	Text string // "Confirm the end of Match 25", "Start the aftermath of Match 3"
	URL  string
	Post bool   // URL is an action to send (Start the aftermath), not a page to open
	Who  string // the viewer's company it is about, when they have several
}

// yourTurns lists what waits on the viewer: the ends they must confirm (with their MVP votes) and the aftermaths of their companies that
// are pending or in progress.
func (s *Server) yourTurns(r *http.Request, base campaignBase, matches []core.Match) ([]yourTurn, error) {
	u := userFrom(r.Context())
	if u == nil {
		return nil, nil
	}
	id := base.Campaign.ID
	var out []yourTurn
	for _, m := range matches {
		if !m.Ended && m.EndProposal != nil && m.EndProposal.WaitingOn(u.ID) && base.Campaign.Status != core.CampaignFinished {
			text := "Confirm the end of Match " + itoa(m.Number)
			if !m.Practice {
				text = "Vote the MVP and confirm the end of Match " + itoa(m.Number)
			}
			out = append(out, yourTurn{Text: text, URL: matchPath(id, m.ID)})
		}
	}
	several := len(base.Campaign.Companies) > 1
	for _, co := range base.Campaign.Companies {
		refs, err := s.campaigns.PendingAftermaths(r.Context(), id, co.ID)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			t := yourTurn{Text: "Continue the aftermath of Match " + itoa(ref.MatchNumber), URL: matchPath(id, ref.MatchID) + "/aftermath/" + itoa(int(co.ID))}
			if ref.Status == core.AftermathPending {
				t.Text, t.URL, t.Post = "Start the aftermath of Match "+itoa(ref.MatchNumber), t.URL+"/begin", true
			}
			if several {
				t.Who = co.Name
			}
			out = append(out, t)
		}
	}
	return out, nil
}

// mine says whether the viewer owns a company of the match.
func mine(base campaignBase, m core.Match) bool {
	return slices.ContainsFunc(m.AllCompanies(), func(c core.CampaignCompany) bool { return base.Campaign.Owns(c.ID) })
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
			if over.YourTurn, err = s.yourTurns(r, base, matches); s.fail(w, r, err) {
				return
			}
			if base.CanInvite() {
				if over.NewOrganisers, err = s.handoverCandidates(r, base.Campaign.ID, base.Campaign.OrganiserID); s.fail(w, r, err) {
					return
				}
			}
			view = over
		case "rankings":
			list, err := s.campaigns.Companies(ctx, id)
			if s.fail(w, r, err) {
				return
			}
			view = rankingsView{base, core.Rank(list)}
		case "matches":
			matches, err := s.campaigns.Matches(ctx, id)
			if s.fail(w, r, err) {
				return
			}
			list, err := s.campaigns.Companies(ctx, id)
			if s.fail(w, r, err) {
				return
			}
			mv := matchesView{campaignBase: base, Matches: matches, Companies: len(list), Filter: r.URL.Query().Get("filter")}
			if mv.YourTurn, err = s.yourTurns(r, base, matches); s.fail(w, r, err) {
				return
			}
			drop := map[string]func(core.Match) bool{
				"mine":  func(m core.Match) bool { return !mine(base, m) },
				"open":  func(m core.Match) bool { return m.Ended },
				"ended": func(m core.Match) bool { return !m.Ended },
			}[mv.Filter]
			if drop != nil {
				mv.Matches = slices.DeleteFunc(slices.Clone(matches), drop)
			} else {
				mv.Filter = ""
			}
			view = mv
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
	if !base.Campaign.CanCreateCompany() { // at the cap (one company per player by default): go and manage it
		redirect(w, r, campaignPath(base.Campaign.ID)+"/my-company")
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
	created, err := s.campaigns.CreateCompany(r.Context(), *userFrom(r.Context()), base.Campaign.ID, in)
	switch {
	case errors.Is(err, core.ErrInvalidCompany) || errors.Is(err, core.ErrCompanyLimit):
		msg := reason(err)
		if errors.Is(err, core.ErrCompanyLimit) {
			msg = "You already have a company in this campaign."
			if n := base.Campaign.Settings.CompanyCap(); n > 1 {
				msg = fmt.Sprintf("You already have the %d companies this campaign allows.", n)
			}
		}
		view, ok := s.newCompanyPage(w, r, base, in, msg)
		if !ok {
			return
		}
		s.render.page(w, r, http.StatusUnprocessableEntity, "company_new", view)
	case s.fail(w, r, err):
	default:
		redirect(w, r, ownPath(base.Campaign.ID, created.ID))
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
	base, ok := s.loadCampaign(w, r, "rankings") // the companies are listed in the Rankings
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
	if base.Campaign.Owns(co.ID) {
		http.Redirect(w, r, ownPath(base.Campaign.ID, co.ID), http.StatusSeeOther)
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
	if view.Grants, err = s.grantsCard(r, base, co.ID, true); s.fail(w, r, err) { // the organiser's award form: CanGrant decides
		return
	}
	s.render.page(w, r, http.StatusOK, "campaign_my_company", view)
}

// segView is a piece of a description: plain text, or a mention of a glossary term with what its preview shows.
type segView struct {
	Text, TermID, TipTitle, TipText string
	// UnitID is the anchor of a base unit's card in the campaign glossary when the text names the unit (core.UnitAnchor); the tooltip
	// is then the unit's profile in short (TipMeta: types and points; TipText: the profile line, wargear and rules).
	UnitID, TipMeta string
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
	TipText string     // the glossary's text of the path, for the tooltip of its name
	TipPath string     // and its limits and chart (tipPath)
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

// linkText cuts text into pieces, linking the glossary terms and the base units it mentions.
func linkText(g core.Glossary, text string) []segView { return segViews(g, text, 320) }

// segViews is linkText with the length of a term's tooltip chosen. A unit the glossary has no card for (it cannot happen for a text cut by
// the same glossary) is left as plain text rather than linking to nothing.
func segViews(g core.Glossary, text string, tip int) []segView {
	return viewSegments(g, g.Link(text), tip)
}

// itemText is linkText for the description of an item: the item's own names (its name and its other spellings) are never linked, and
// the names of weapons are linked in any case ("a lance").
func itemText(g core.Glossary, text string, names ...string) []segView {
	return viewSegments(g, g.LinkExcept(text, names...), 320)
}

func viewSegments(g core.Glossary, segs []core.Segment, tip int) []segView {
	var out []segView
	for _, seg := range segs {
		v := segView{Text: seg.Text, TermID: seg.TermID}
		if term, ok := g.Term(seg.TermID); ok {
			v.TipTitle, v.TipText = term.Name, term.Summary(tip)
		}
		if u := g.UnitCard(seg.UnitID); seg.UnitID != "" && u != nil {
			v.UnitID, v.TipTitle, v.TipMeta, v.TipText = u.ID, u.Name, u.TipMeta(), u.TipText()
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
			v.Items = append(v.Items, itemView{Equipment: e, Segments: itemText(g, e.Description, append([]string{e.Name}, e.Spellings...)...), TipStats: tipStats(e.MountStats)})
		}
		for _, r := range u.Rules {
			v.Rules = append(v.Rules, ruleView{UnitRule: r, Segments: linkText(g, r.Description)})
		}
		if u.Path != nil {
			pv := &pathView{Name: u.Path.Name}
			if term, ok := g.Find(u.Path.Name); ok {
				pv.TipText, pv.TipPath = term.Text(), tipPath(term.Path)
			}
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

// myCompanyPage renders the My Company tab for the company of the URL, optionally with a notice about what was just done.
func (s *Server) myCompanyPage(w http.ResponseWriter, r *http.Request, status int, notice string, noticeErr bool) {
	base, ok := s.loadOwn(w, r, "my-company")
	if !ok {
		return
	}
	mine, user := *base.Current, *userFrom(r.Context())
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, mine.ID)
	if s.fail(w, r, err) {
		return
	}
	g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID) // descriptions link to the campaign's glossary
	if s.fail(w, r, err) {
		return
	}
	view := myCompanyView{campaignBase: base, Notice: notice, NoticeError: noticeErr, Company: detail, HasCompany: true, Units: unitViews(g, detail.Roster)}
	view.Frozen = base.Campaign.Status == core.CampaignFinished
	view.ReadOnly = view.Frozen // an ended campaign is frozen: the page shows everything and offers nothing
	if view.Live, err = s.liveFor(r, base, detail, view.Units); s.fail(w, r, err) {
		return
	}
	view.PerksUnused, view.PerksUsed = splitPerks(perkViews(detail.Perks, detail.Roster))
	if view.Grants, err = s.grantsCard(r, base, mine.ID, true); s.fail(w, r, err) { // the organiser and administrators award their own company too (CanGrant decides)
		return
	}
	if view.Pending, err = s.campaigns.PendingChoice(r.Context(), user, base.Campaign.ID, mine.ID); s.fail(w, r, err) {
		return
	}
	if view.Officers, err = s.campaigns.PendingOfficers(r.Context(), user, base.Campaign.ID, mine.ID); s.fail(w, r, err) {
		return
	}
	if roll, err := s.campaigns.RecruitState(r.Context(), user, base.Campaign.ID, mine.ID); s.fail(w, r, err) {
		return
	} else if roll != nil && !view.Frozen {
		panel, err := s.recruitPanelFor(r, base.Campaign, mine.ID, "", false)
		if s.fail(w, r, err) {
			return
		}
		view.Recruiting = &panel
	}
	s.render.page(w, r, status, "campaign_my_company", view)
}

// handleMyCompany is the My Company tab of the company in the URL.
func (s *Server) handleMyCompany(w http.ResponseWriter, r *http.Request) {
	s.myCompanyPage(w, r, http.StatusOK, "", false)
}

// handleMyCompanyAlias is /campaigns/{id}/my-company without a company, the URL of bookmarks, the sidebar and the tab: it sends the
// viewer to the company they opened last (the bcm-last cookie), else their first, oldest company, and renders the page
// that offers to create one when they have none.
func (s *Server) handleMyCompanyAlias(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "my-company")
	if !ok {
		return
	}
	if id, ok := lastCompany(r, base.Campaign); ok { // the one opened last, else the oldest
		if boostedLink(r) { // a click: answer with the company's page now, a redirect would cost the browser one more round trip
			target := r.Clone(r.Context())
			target.SetPathValue("company", itoa(int(id)))
			w.Header().Set("HX-Push-Url", ownPath(base.Campaign.ID, id))
			s.handleMyCompany(w, target)
			return
		}
		http.Redirect(w, r, ownPath(base.Campaign.ID, id), http.StatusSeeOther)
		return
	}
	s.render.page(w, r, http.StatusOK, "campaign_my_company", myCompanyView{campaignBase: base})
}

// boostedLink reports whether the request is a click on a boosted link or form (not the restore of a history entry). An address that
// redirects can answer such a request with the page it leads to and name the final address in HX-Push-Url: the address bar and the
// history get the same entry as after the redirect, without the extra round trip. A request for the whole page keeps the redirect.
func boostedLink(r *http.Request) bool {
	return r.Header.Get("HX-Boosted") == "true" && r.Header.Get("HX-History-Restore-Request") != "true"
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

// Own is the company of the viewer the page acts on: set on the owner routes (/my-company/{company}, the armory) by loadOwn, nil on every
// other page.
func (b campaignBase) Own() *core.CampaignCompany { return b.Current }

// Switcher is whether the company chips show: the viewer owns two or more companies of the campaign (docs/frontend-campaigns.md, "Company switcher").
func (b campaignBase) Switcher() bool { return len(b.Campaign.Companies) > 1 }

// OwnPath is the URL root of the owner pages of the company the page acts on: /campaigns/{id}/my-company/{company}.
func (b campaignBase) OwnPath() string {
	if b.Current == nil {
		return campaignPath(b.Campaign.ID) + "/my-company"
	}
	return ownPath(b.Campaign.ID, b.Current.ID)
}

// SwitchPath is where the company chip of one of the viewer's companies leads: the same tab, that company.
func (b campaignBase) SwitchPath(companyID int64) string {
	if b.Tab == "armory" {
		return b.ArmoryPath(companyID)
	}
	return ownPath(b.Campaign.ID, companyID)
}

// ArmoryPath is the URL of the armory of one of the viewer's companies.
func (b campaignBase) ArmoryPath(companyID int64) string {
	return campaignPath(b.Campaign.ID) + "/armory?company=" + itoa(int(companyID))
}

// ownPath is the URL root of the owner pages of one company.
func campaignPath(campaignID int64) string { return "/campaigns/" + itoa(int(campaignID)) }

func matchPath(campaignID, matchID int64) string {
	return campaignPath(campaignID) + "/matches/" + itoa(int(matchID))
}

func companyPath(campaignID, companyID int64) string {
	return campaignPath(campaignID) + "/companies/" + itoa(int(companyID))
}

func ownPath(campaignID, companyID int64) string {
	return campaignPath(campaignID) + "/my-company/" + itoa(int(companyID))
}

// ownCompany is THE helper of every owner route: it reads {company} (or the form value "company" of the armory) and returns it when it is one of the viewer's own companies of the
// campaign. It answers 404 itself for anything else (not a number, another player's company, a company of another campaign), so a stranger
// and an organiser probing an id learn nothing. No handler guesses a company.
func (s *Server) ownCompany(w http.ResponseWriter, r *http.Request, d core.CampaignDetail) (core.CampaignCompany, bool) {
	raw := r.PathValue("company")
	if raw == "" {
		raw = r.FormValue("company") // the armory names its company in the query (page) or a hidden field (buying)
	}
	cid, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		s.notFound(w, r)
		return core.CampaignCompany{}, false
	}
	co, own := d.Own(cid)
	if !own {
		s.notFound(w, r)
		return core.CampaignCompany{}, false
	}
	return co, true
}

// loadOwn is loadCampaign for the pages of one of the viewer's companies: base.Current is the company of the URL.
func (s *Server) loadOwn(w http.ResponseWriter, r *http.Request, tab string) (campaignBase, bool) {
	base, ok := s.loadCampaign(w, r, tab)
	if !ok {
		return campaignBase{}, false
	}
	co, ok := s.ownCompany(w, r, base.Campaign)
	if !ok {
		return campaignBase{}, false
	}
	base.Current = &co
	s.rememberVisit(w, r, base.Campaign, co.ID)
	return base, true
}

// ownCompanyID is the company id of the URL for an action that needs nothing else of the campaign: it loads the campaign to authorise
// the company through ownCompany.
func (s *Server) ownCompanyID(w http.ResponseWriter, r *http.Request, campaignID int64) (int64, bool) {
	d, err := s.campaigns.Get(r.Context(), userFrom(r.Context()).ID, campaignID)
	if s.fail(w, r, err) {
		return 0, false
	}
	co, ok := s.ownCompany(w, r, d)
	return co.ID, ok
}
