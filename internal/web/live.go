package web

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// liveView is a company page while its match is open: the banner, and the units sorted into the zones (docs/live-match.md).
type liveView struct {
	core.LiveCompany
	CampaignID int64
	Scenario   string // the match's scenario, empty until chosen
	Edit       bool   // the viewer may change this company's live state
	Field      []liveCard
	Retreated  []liveCard
	Casualties []liveCard
	Missed     []liveCard             // injured at the start: they take no part
	End        endBanner              // the pending end and the Broken companies, for the sticky banner
	Practice   bool                   // a practice match: the badge and the banner in the match bar
	Companies  []core.CampaignCompany // every company of the match, for its chips (this one is marked)
	Own        map[int64]bool         // the viewer's companies among them: their pages are My Company
}

// PagePath is the page of a company of the match: My Company for the viewer's own, the company page in match mode otherwise.
func (v liveView) PagePath(companyID int64) string {
	if v.Own[companyID] {
		return ownPath(v.CampaignID, companyID)
	}
	return companyPath(v.CampaignID, companyID)
}

// Broken is whether the company is Broken (its cards show the courage chips).
func (v liveView) Broken() bool { return v.Break.Broken }

// CourageChip is the chip text of a Broken company's unit: how its last courage test was resolved (there are no turns: the players
// know when it tests again).
func (v liveView) CourageChip(l core.LiveUnit) string {
	switch l.CourageHow {
	case "passed":
		return "Tested: passed"
	case "failed":
		return "Tested: failed"
	case "standFast", "heroicResolve", "chariot", "scenario":
		return "Auto-pass"
	case "engaged", "cannotActivate":
		return "Exempt"
	}
	return "Courage resolved"
}

// CourageDue is whether a unit of a Broken company has not tested its courage yet in this match.
func (v liveView) CourageDue(l core.LiveUnit) bool {
	return v.Break.Broken && l.Zone == core.ZoneField && l.CourageHow == ""
}

// CourageTest is whether the card offers the Courage test: every unit of a Broken company in the Field (it tests again each turn; the
// players know when).
func (v liveView) CourageTest(l core.LiveUnit) bool {
	return v.Break.Broken && l.Zone == core.ZoneField
}

// breakCell is one starting model on the bar of the Break Point panel: fallen, gone from the table or still in play.
type breakCell struct{ Fallen, Left bool }

// BreakCells is the bar of the Break Point panel, one cell per starting model: the casualties fill it from the left and the models that
// left the board take the far end, so the fill always reads against the Break mark.
func (v liveView) BreakCells() []breakCell {
	b := v.Break
	out := make([]breakCell, b.StartingModels)
	for i := range out {
		out[i] = breakCell{Fallen: i < b.Casualties, Left: i >= b.StartingModels-b.LeftTable && i >= b.Casualties}
	}
	return out
}

// breakMark is where a mark sits on the bar, as a CSS percentage: the boundary before the cell of the model that crosses it, so the fill
// passing a mark means that milestone has been reached.
func (v liveView) breakMark(cells int) string {
	return strconv.FormatFloat(float64(min(max(cells, 0), v.Break.StartingModels))*100/float64(v.Break.StartingModels), 'f', 2, 64)
}

// BreakMark is the position of the casualty that breaks the company.
func (v liveView) BreakMark() string { return v.breakMark(v.Break.BreakAt() - 1) }

// EndMark is the position of the casualty that reaches the scenario's end.
func (v liveView) EndMark() string { return v.breakMark(v.Break.EndAt() - 1) }

// MarksMeet says the Break and the end fall on the same model (a very small company): the two marks then share the line.
func (v liveView) MarksMeet() bool { return v.BreakMark() == v.EndMark() }

// FailedCourage lists the units that fled a courage test: casualties for the Break Point.
func (v liveView) FailedCourage() (out []liveCard) {
	for _, c := range v.Retreated {
		if c.L.Reason == "Failed courage" {
			out = append(out, c)
		}
	}
	return out
}

// liveCard is a unit with its live state (zero L for a unit that is not in the match).
type liveCard struct {
	U       unitView
	L       core.LiveUnit
	Applies []core.RuleMatch // an enemy unit: the rules of the viewer's side that name one of its keywords (docs/unit-keywords.md)
}

// targetUnits are the units of a company still in the Field, as the rule matching sees them (core.NewTargetUnit).
func targetUnits(roster []core.CompanyUnit, lc core.LiveCompany) (out []core.TargetUnit) {
	live := map[int64]core.LiveUnit{}
	for _, l := range lc.Units {
		live[l.UnitID] = l
	}
	for _, u := range roster {
		if t, ok := core.NewTargetUnit(u, live[u.ID]); ok {
			out = append(out, t)
		}
	}
	return out
}

// viewerSide is the companies of the match that stand on the viewer's side, the viewer's own among them; nil when the viewer has no company
// in the match or the company asked about is on that side too (nothing of one's own side is an enemy).
func viewerSide(base campaignBase, m core.Match, companyID int64) []core.CampaignCompany {
	mine := base.Campaign.Companies // the company the page acts on, or else any of the viewer's own
	if base.Current != nil {
		mine = []core.CampaignCompany{*base.Current}
	}
	if len(mine) == 0 {
		return nil
	}
	var viewer, other []core.CampaignCompany
	for _, side := range m.Sides {
		hasMine, hasOther := false, false
		for _, c := range side.Companies {
			hasMine = hasMine || slices.ContainsFunc(mine, func(o core.CampaignCompany) bool { return o.ID == c.ID })
			hasOther = hasOther || c.ID == companyID
		}
		if hasMine {
			viewer = side.Companies
		}
		if hasOther {
			other = side.Companies
		}
	}
	if viewer == nil || other == nil || slices.ContainsFunc(viewer, func(c core.CampaignCompany) bool { return c.ID == companyID }) {
		return nil
	}
	return viewer
}

// enemyMatches are the rules of the viewer's side that name keywords of the units of an enemy company, by unit; empty when the viewer is
// not on the other side, when the rule set has no rule targets, or when a read fails (the markers are an extra, never an error).
func (s *Server) enemyMatches(r *http.Request, base campaignBase, m core.Match, companyID int64, theirs []core.CompanyUnit, lc core.LiveCompany) map[int64][]core.RuleMatch {
	side := viewerSide(base, m, companyID)
	if side == nil {
		return nil
	}
	var mine []core.TargetUnit
	user := *userFrom(r.Context())
	for _, co := range side {
		friend, err := s.campaigns.Live(r.Context(), user, base.Campaign.ID, co.ID)
		if err != nil {
			return nil
		}
		detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, co.ID)
		if err != nil {
			return nil
		}
		mine = append(mine, targetUnits(detail.Roster, friend)...)
	}
	return core.RuleMatches(mine, targetUnits(theirs, lc))
}

// liveFor loads the live state of the company when it is in an open match (nil when it is in none: the page is the usual one).
func (s *Server) liveFor(r *http.Request, base campaignBase, detail core.CompanyDetail, units []unitView) (*liveView, error) {
	if detail.OpenMatch == nil {
		return nil, nil
	}
	campaignID := base.Campaign.ID
	lc, err := s.campaigns.Live(r.Context(), *userFrom(r.Context()), campaignID, detail.ID)
	if errors.Is(err, core.ErrNotFound) { // the match ended meanwhile
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v := &liveView{LiveCompany: lc, CampaignID: campaignID, Edit: lc.Access == core.LiveEdit}
	match, matchErr := s.campaigns.Match(r.Context(), campaignID, lc.MatchID)
	if matchErr == nil {
		if match.Scenario != nil {
			v.Scenario = match.Scenario.Name
		}
		v.Practice = match.Practice
		v.End = s.endBannerFor(r, base, match, lc.CompanyID)
		v.Companies, v.Own = match.AllCompanies(), map[int64]bool{}
		for _, co := range v.Companies {
			if base.Campaign.Owns(co.ID) {
				v.Own[co.ID] = true
			}
		}
	}
	v.End.Campaign, v.End.MatchID, v.End.Company, v.End.Edit = campaignID, lc.MatchID, lc.CompanyID, v.Edit // End match is drawn even when the match could not be read
	live := map[int64]core.LiveUnit{}
	for _, u := range lc.Units {
		live[u.UnitID] = u
	}
	var applies map[int64][]core.RuleMatch
	if matchErr == nil && viewerSide(base, match, lc.CompanyID) != nil {
		roster := make([]core.CompanyUnit, len(units))
		for i, u := range units {
			roster[i] = u.CompanyUnit
		}
		applies = s.enemyMatches(r, base, match, lc.CompanyID, roster, lc)
	}
	for _, u := range units {
		u = markUsedItems(u, live[u.ID])
		l, in := live[u.ID]
		if in && l.HasMount && !l.Mounted {
			u = strikeMountGear(u)
		}
		card := liveCard{U: u, L: l, Applies: applies[u.ID]}
		switch {
		case slices.Contains(lc.Absent, u.ID):
			v.Missed = append(v.Missed, card)
		case in && l.Zone == core.ZoneRetreated:
			v.Retreated = append(v.Retreated, card)
		case in && l.Zone == core.ZoneCasualty:
			v.Casualties = append(v.Casualties, card)
		default:
			v.Field = append(v.Field, card)
		}
	}
	if lc.Wanderer != nil { // the temporary Wanderer of Against the Odds is in no roster: its card comes from the selection
		if l, in := live[lc.Wanderer.UnitID]; in {
			card := liveCard{U: unitViews(core.Glossary{}, []core.CompanyUnit{temporaryUnit(lc.Wanderer)})[0], L: l}
			switch l.Zone {
			case core.ZoneRetreated:
				v.Retreated = append(v.Retreated, card)
			case core.ZoneCasualty:
				v.Casualties = append(v.Casualties, card)
			default:
				v.Field = append(v.Field, card)
			}
		}
	}
	return v, nil
}

// markUsedItems strikes through the owned items the unit used in this match.
func markUsedItems(u unitView, l core.LiveUnit) unitView {
	if len(l.UsedItems) == 0 {
		return u
	}
	u.Items = slices.Clone(u.Items)
	for i, it := range u.Items {
		if slices.ContainsFunc(l.UsedItems, func(n string) bool { return strings.EqualFold(n, it.Name) }) {
			u.Items[i].Struck = "used in this match"
		}
	}
	return u
}

// handleLiveFragment answers the zones of a company in a running match: what a change of the match refetches (htmx).
func (s *Server) handleLiveFragment(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "my-company")
	if !ok {
		return
	}
	companyID, ok := s.idParam(w, r, "company")
	if !ok {
		return
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, companyID)
	if s.fail(w, r, err) {
		return
	}
	view, err := s.liveFragmentViewQuiet(r, base, detail)
	if s.fail(w, r, err) {
		return
	}
	if view.Live == nil {
		if r.Header.Get("HX-Request") != "" { // the match ended while the page was open: it becomes the page of an ended match
			w.Header().Set("HX-Refresh", "true")
			w.WriteHeader(http.StatusOK)
			return
		}
		s.notFound(w, r)
		return
	}
	s.render.partial(w, http.StatusOK, "campaign_my_company", "live_zones", view)
}

// liveFragmentView is what the zones need, for the owner's own company or somebody else's (read only); it answers 404 when the
// company is in no open match.
func (s *Server) liveFragmentView(w http.ResponseWriter, r *http.Request, base campaignBase, detail core.CompanyDetail) (myCompanyView, bool) {
	view, err := s.liveFragmentViewQuiet(r, base, detail)
	if s.fail(w, r, err) {
		return myCompanyView{}, false
	}
	if view.Live == nil {
		s.notFound(w, r)
		return myCompanyView{}, false
	}
	return view, true
}

// liveFragmentViewQuiet is liveFragmentView without answering; Live is nil when the company is in no open match.
func (s *Server) liveFragmentViewQuiet(r *http.Request, base campaignBase, detail core.CompanyDetail) (myCompanyView, error) {
	g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID)
	if err != nil {
		return myCompanyView{}, err
	}
	view := myCompanyView{campaignBase: base, Company: detail, HasCompany: true, Units: unitViews(g, detail.Roster)}
	view.ReadOnly = !base.Campaign.Owns(detail.ID)
	view.Live, err = s.liveFor(r, base, detail, view.Units)
	return view, err
}

// strikeMountGear marks what a unit that lost its mount cannot use any more: the mount itself, the gear that needs one, the rules the
// mount gave and the Cavalry keyword. Nothing is removed: the items and rules stay listed, struck through, and come back with the mount.
func strikeMountGear(u unitView) unitView {
	u.Items = slices.Clone(u.Items)
	for i, it := range u.Items {
		switch {
		case it.Class == core.EquipMount:
			u.Items[i].Struck = "the mount is lost"
		case it.RequiresMount:
			u.Items[i].Struck = "needs a mount"
		}
	}
	u.Rules = slices.Clone(u.Rules)
	for i, r := range u.Rules {
		if r.Mount != "" || r.Name == "Cavalry" {
			u.Rules[i].Struck = "needs a mount"
		}
	}
	return u
}

// xpAhead is how much of the experience earned in the match fits in the unit's current level: the rest of the bar at most.
func xpAhead(inLevel, every, match int) int {
	return max(0, min(match, every-inLevel))
}
