package web

import (
	"errors"
	"net/http"
	"slices"
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
	Missed     []liveCard // injured at the start: they take no part
	EndPending bool       // an end of the match is proposed: Start turn is refused
	End        endBanner  // the pending end, for the sticky banner
	Practice   bool       // a practice match: the badge and the banner in the match bar
}

// Broken is whether the company is Broken (its cards show the courage chips).
func (v liveView) Broken() bool { return v.Break.Broken }

// CourageChip is the chip text of a Broken company's unit whose courage is resolved this turn: how it was resolved.
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

// CourageDue is whether a unit of a Broken company still has to test this turn.
func (v liveView) CourageDue(l core.LiveUnit) bool {
	return v.Break.Broken && l.Zone == core.ZoneField && l.CourageTurn != v.Turn
}

// NextTurn is the number "Start turn N" would start.
func (v liveView) NextTurn() int { return v.Turn + 1 }

// MeterPct is how far the casualties are towards the Break Point, held to 100.
func (v liveView) MeterPct() int {
	if v.Break.Threshold100 <= 0 {
		return 0
	}
	return min(100, v.Break.Casualties*10000/v.Break.Threshold100)
}

// FailedCourage lists the units that fled a courage test: casualties for the Break Point.
func (v liveView) FailedCourage() (out []liveCard) {
	for _, c := range v.Retreated {
		if c.L.Reason == "Failed courage" {
			out = append(out, c)
		}
	}
	return out
}

// ToTest lists the Field units of a Broken company that have not resolved their courage this turn first, then the others.
func (v liveView) ToTest() (t struct{ Due, Done []liveCard }) {
	for _, c := range v.Field {
		if v.CourageDue(c.L) {
			t.Due = append(t.Due, c)
		} else {
			t.Done = append(t.Done, c)
		}
	}
	return t
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
		v.EndPending = match.EndProposal != nil
		v.Practice = match.Practice
		v.End = s.endBannerFor(r, base, match, lc.CompanyID)
	}
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
	view, ok := s.liveFragmentView(w, r, base, detail)
	if !ok {
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
