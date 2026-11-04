package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The new match form (docs/matches.md, "New match"): two sides with the company search only,
// one options block (Practice, + Add side, All vs all), the scenario the server already rolled and, for a 1 v 1 whose lower rated company
// is the creator's, the Against the Odds bonuses that need no detail. Nothing is written until Create match.

// newMatchView feeds the form (static/js/match_form.js): the campaign's companies to search in and the sides as they currently are,
// both as JSON in data attributes, the scenario card and, after a failed create, the Against the Odds block as it was ticked.
type newMatchView struct {
	campaignBase
	CompaniesJSON string
	SidesJSON     string
	FFA           bool // all vs all: the first "side" is just the pool of companies, each of which fights alone
	Error         string
	Busy          *core.OpenMatchRef // the open match a refused company is in, to link to
	Companies     int                // how many companies the campaign has; under two there is nobody to match
	Practice      bool               // the "Practice match" box is ticked (set on the form only; nothing can change it afterwards)
	Scenario      newScenarioView
	ATO           *newATOView // the Against the Odds block drawn with the page (after a failed create); nil: the form loads it
	MaxSides      int
}

// TooFew is whether the campaign has too few companies for a match.
func (v newMatchView) TooFew() bool { return v.Companies < minMatchCompanies }

// newScenarioView is the scenario card of the form. Mode "" shows the scenario (with the dice when the server rolled it), "pick" the
// rulebook's list by table (one press sets it), "custom" the one-off fields. The card's hidden inputs carry the choice to Create match.
type newScenarioView struct {
	CampaignID int64
	Mode       string
	Info       *core.ScenarioInfo // the rulebook scenario on show; nil for none or a one-off
	Custom     core.ScenarioInput // the one-off fields
	Dice       [2]int             // the server's roll; zero when the scenario was chosen
	Table      int
	Groups     []scenarioGroup
	Ambush     bool // Against the Odds Ambush! is ticked: the scenario is a free choice of the rulebook's
	Err        map[string]string
	Note       string
}

// RollText says where the scenario on show comes from: the dice the server rolled, or the table and roll it sits at when it was chosen.
func (v newScenarioView) RollText() string {
	switch {
	case v.Info == nil:
		return ""
	case v.Dice[0] > 0:
		return "Rolled " + itoa(v.Dice[0]) + " (table " + itoa(v.Table) + ") and " + itoa(v.Dice[1])
	case v.Info.Table > 0 && v.Info.Roll > 0:
		return "Chosen · table " + itoa(v.Info.Table) + ", second die " + itoa(v.Info.Roll)
	}
	return "Chosen"
}

// newATOView is the Against the Odds block of the form: the card for the two companies, only for the owner of the lower rated one.
type newATOView struct {
	Card  core.ATO
	Taken map[string]bool
}

// Options are the bonuses the gap affords (the others are not offered at all); Tool Kit and the Wanderer are completed on the match page.
func (v newATOView) Options() (out []core.ATOOption) {
	for _, o := range v.Card.Options {
		if !o.OverBudget {
			out = append(out, o)
		}
	}
	return out
}

// Later says whether a bonus needs details only the match page asks for (kits and their items, which Wanderer).
func (v newATOView) Later(key string) bool { return key == "toolKit" || key == "wanderer" }

func newMatchForm(base campaignBase, companies []core.CampaignCompany, unavailable map[int64]string, sides []core.NewSide, ffa bool, errMsg string) (newMatchView, error) {
	list := make([]map[string]any, len(companies))
	for i, c := range companies {
		list[i] = map[string]any{"id": c.ID, "name": c.Name, "owner": c.Owner, "ownerId": c.OwnerID, "faction": c.FactionName(), "mine": base.Campaign.Owns(c.ID), "unavailable": unavailable[c.ID]}
	}
	state := make([]map[string]any, len(sides))
	for i, sd := range sides {
		ids := sd.Companies
		if ids == nil {
			ids = []int64{}
		}
		state[i] = map[string]any{"ids": ids}
	}
	cj, err := json.Marshal(list)
	if err != nil {
		return newMatchView{}, err
	}
	sj, err := json.Marshal(state)
	if err != nil {
		return newMatchView{}, err
	}
	return newMatchView{campaignBase: base, CompaniesJSON: string(cj), SidesJSON: string(sj), FFA: ffa, Error: errMsg, Companies: len(companies),
		Scenario: newScenarioView{CampaignID: base.Campaign.ID}, MaxSides: core.MaxSides}, nil
}

// unavailableCompanies asks why each company of the list cannot play right now.
func (s *Server) unavailableCompanies(r *http.Request, id int64, list []core.CampaignCompany) (map[int64]string, error) {
	ids := make([]int64, len(list))
	for i, c := range list {
		ids[i] = c.ID
	}
	return s.campaigns.UnavailableCompanies(r.Context(), id, ids)
}

// newMatchCtx loads the campaign of the form and its companies; it answers itself when the campaign is unknown or cannot start a match.
func (s *Server) newMatchCtx(w http.ResponseWriter, r *http.Request) (campaignBase, []core.CampaignCompany, bool) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return base, nil, false
	}
	if err := base.Campaign.CanStartMatch(); err != nil {
		s.refuse(w, r, http.StatusForbidden, reason(err))
		return base, nil, false
	}
	list, err := s.campaigns.Companies(r.Context(), base.Campaign.ID)
	if s.fail(w, r, err) {
		return base, nil, false
	}
	return base, list, true
}

func (s *Server) handleNewMatchPage(w http.ResponseWriter, r *http.Request) {
	base, list, ok := s.newMatchCtx(w, r)
	if !ok {
		return
	}
	unavailable, err := s.unavailableCompanies(r, base.Campaign.ID, list)
	if s.fail(w, r, err) {
		return
	}
	sides := []core.NewSide{{}, {}}
	for _, c := range base.Campaign.Companies { // you are most likely playing; with several companies the first that can
		if _, no := unavailable[c.ID]; !no {
			sides[0].Companies = []int64{c.ID}
			break
		}
	}
	view, err := newMatchForm(base, list, unavailable, sides, false, "")
	if s.fail(w, r, err) {
		return
	}
	if view.Scenario, err = s.rolledScenario(r, base.Campaign.ID); s.fail(w, r, err) {
		return
	}
	view.Practice = r.URL.Query().Get("practice") == "1"
	s.render.page(w, r, http.StatusOK, "match_new", view)
}

// rolledScenario is the card with the scenario the server rolled (the form asks nothing).
func (s *Server) rolledScenario(r *http.Request, id int64) (newScenarioView, error) {
	v := newScenarioView{CampaignID: id}
	list, err := s.campaigns.Scenarios(r.Context(), id)
	if err != nil {
		return v, err
	}
	sel, err := s.campaigns.ScenarioSelection(r.Context(), id)
	if err != nil {
		return v, err
	}
	if sel.Die.Sides <= 0 || len(list) == 0 {
		v.Note = "The rules of this campaign list no scenarios to roll for: describe a one-off one, or leave it for the match page."
		return v, nil
	}
	if sc, d1, d2, ok := s.rollScenario(sel, list); ok {
		v.Info, v.Dice, v.Table = &sc, [2]int{d1, d2}, sel.Table(d1)
		return v, nil
	}
	v.Note = "The dice found no scenario of this campaign's rules: choose one."
	return v, nil
}

// scenarioFromForm reads the scenario the card's hidden inputs (or the one-off fields) carry; nil when none.
func scenarioFromForm(r *http.Request) *core.ScenarioInput {
	switch kind := core.ScenarioKind(r.FormValue("scenario-kind")); kind {
	case core.ScenarioRulebook:
		if name := r.FormValue("scenario-name"); name != "" {
			return &core.ScenarioInput{Kind: kind, Name: name}
		}
	case core.ScenarioCustom:
		return &core.ScenarioInput{Kind: kind, Name: r.FormValue("scenario-name"), Description: r.FormValue("scenario-description"), Victory: r.FormValue("scenario-victory")}
	}
	return nil
}

// scenarioCardFor is the card that shows the scenario in (a rulebook name, a one-off or none).
func (s *Server) scenarioCardFor(r *http.Request, id int64, in *core.ScenarioInput, mode string, ambush bool) (newScenarioView, error) {
	v := newScenarioView{CampaignID: id, Mode: mode, Ambush: ambush}
	list, err := s.campaigns.Scenarios(r.Context(), id)
	if err != nil {
		return v, err
	}
	if in != nil {
		switch in.Kind {
		case core.ScenarioRulebook:
			v.Info = core.FindScenario(list, in.Name)
		case core.ScenarioCustom:
			v.Custom = *in
		}
	}
	if ambush && v.Info == nil && mode == "" {
		v.Mode = "pick"
	}
	if v.Mode == "pick" {
		v.Groups = groupScenarios(list)
	}
	return v, nil
}

// handleNewScenarioCard answers the scenario card of the form: ?mode=pick (the list) or custom (the one-off fields), or a rulebook
// scenario by ?name= (what a press in the list chose). The scenario the card held comes along (hx-include) so Cancel keeps it.
func (s *Server) handleNewScenarioCard(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	mode := r.FormValue("mode")
	if mode != "" && mode != "pick" && mode != "custom" {
		s.notFound(w, r)
		return
	}
	in := scenarioFromForm(r)
	chosen := r.URL.Query().Get("name")
	if chosen != "" {
		in = &core.ScenarioInput{Kind: core.ScenarioRulebook, Name: chosen}
	}
	if mode == "custom" && (in == nil || in.Kind != core.ScenarioCustom) {
		in = &core.ScenarioInput{Kind: core.ScenarioCustom}
	}
	v, err := s.scenarioCardFor(r, base.Campaign.ID, in, mode, r.FormValue("ambush") == "1")
	if s.fail(w, r, err) {
		return
	}
	if chosen == "" {
		if err := s.keepDice(r, base.Campaign.ID, &v); s.fail(w, r, err) {
			return
		}
	}
	s.render.partial(w, http.StatusOK, "match_new", "new_scenario", v)
}

// keepDice puts back the server's roll the card carried (scenario-die, the first die) when it still points at the scenario on show,
// so Cancel in the list shows "Rolled ..." again rather than "Chosen".
func (s *Server) keepDice(r *http.Request, id int64, v *newScenarioView) error {
	d, _ := strconv.Atoi(r.FormValue("scenario-die"))
	if d <= 0 || v.Info == nil {
		return nil
	}
	sel, err := s.campaigns.ScenarioSelection(r.Context(), id)
	if err != nil {
		return err
	}
	if sel.Valid(d) && sel.Table(d) == v.Info.Table {
		v.Dice, v.Table = [2]int{d, v.Info.Roll}, v.Info.Table
	}
	return nil
}

// handleNewScenarioRoll rolls again (both dice, no question) and answers the card.
func (s *Server) handleNewScenarioRoll(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	v, err := s.rolledScenario(r, base.Campaign.ID)
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, http.StatusOK, "match_new", "new_scenario", v)
}

// handleNewATO answers the Against the Odds block for the companies a and b (one each side): empty unless the viewer owns the lower
// rated one and there is a gap (docs/matches.md, "New match").
func (s *Server) handleNewATO(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	v, err := s.newATO(r, base, []core.NewSide{{Companies: []int64{queryID(r, "a")}}, {Companies: []int64{queryID(r, "b")}}}, r.URL.Query().Get("practice") == "1", nil)
	if s.fail(w, r, err) {
		return
	}
	if v == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	s.render.partial(w, http.StatusOK, "match_new", "new_ato", *v)
}

// newATO is the block for the sides, nil when it is not shown (not a 1 v 1, equal ratings, the viewer is not the underdog's owner).
func (s *Server) newATO(r *http.Request, base campaignBase, sides []core.NewSide, practice bool, taken []string) (*newATOView, error) {
	if len(sides) != 2 || len(sides[0].Companies) != 1 || len(sides[1].Companies) != 1 || sides[0].Companies[0] == sides[1].Companies[0] ||
		sides[0].Companies[0] <= 0 || sides[1].Companies[0] <= 0 {
		return nil, nil
	}
	card, err := s.campaigns.PreviewAgainstTheOdds(r.Context(), *userFrom(r.Context()), base.Campaign.ID, sides, practice)
	if errors.Is(err, core.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !card.Eligible || card.Access != core.ATOEdit {
		return nil, nil
	}
	v := &newATOView{Card: card, Taken: map[string]bool{}}
	for _, k := range taken {
		v.Taken[k] = true
	}
	return v, nil
}

func (s *Server) handleCreateMatch(w http.ResponseWriter, r *http.Request) {
	base, list, ok := s.newMatchCtx(w, r)
	if !ok {
		return
	}
	known := map[int64]bool{}
	for _, c := range list {
		known[c.ID] = true
	}
	ffa := r.PostFormValue("mode") == "ffa"
	n, _ := strconv.Atoi(r.PostFormValue("sides"))
	sides := make([]core.NewSide, max(2, min(n, core.MaxSides)))
	for i := range sides {
		for _, v := range r.PostForm["side-"+itoa(i+1)] {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil && known[id] {
				sides[i].Companies = append(sides[i].Companies, id)
			}
		}
	}
	form := sides
	if ffa { // all vs all: the companies of the pool (side 1) each become a side of their own
		sides = nil
		for _, id := range form[0].Companies {
			sides = append(sides, core.NewSide{Companies: []int64{id}})
		}
	}
	user := *userFrom(r.Context())
	practice := r.PostFormValue("practice") == "1"
	scenario := scenarioFromForm(r)
	in := core.NewMatch{Sides: sides, Practice: practice, Scenario: scenario}
	taken := slices.DeleteFunc(slices.Clone(r.PostForm["ato"]), func(k string) bool { return k == "" })
	if len(taken) > 0 && !ffa {
		sel := core.ATOSelection{Bonuses: taken}
		if slices.Contains(taken, "ambush") && scenario != nil && scenario.Kind == core.ScenarioRulebook {
			sel.Ambush = scenario.Name
		}
		in.ATO = &core.NewMatchATO{Selection: sel}
	}
	matchID, err := s.recent.create([]any{"match", user.ID, base.Campaign.ID, sides, practice, scenario, taken}, func() (int64, error) {
		m, err := s.campaigns.CreateMatch(r.Context(), user, base.Campaign.ID, in)
		return m.ID, err
	})
	switch {
	case errors.Is(err, core.ErrInvalidMatch) || errors.Is(err, core.ErrRefused):
		msg := reason(err)
		if errors.Is(err, core.ErrInvalidMatch) {
			msg = strings.Join(reasons(err), " ")
		}
		unavailable, jerr := s.unavailableCompanies(r, base.Campaign.ID, list)
		if s.fail(w, r, jerr) {
			return
		}
		view, jerr := newMatchForm(base, list, unavailable, form, ffa, msg)
		if s.fail(w, r, jerr) {
			return
		}
		view.Practice = practice
		var busy *core.CompanyBusyError
		if errors.As(err, &busy) {
			view.Busy = &busy.OpenMatchRef
		}
		custom := scenario != nil && scenario.Kind == core.ScenarioCustom
		mode := ""
		if custom {
			mode = "custom"
		}
		if view.Scenario, jerr = s.scenarioCardFor(r, base.Campaign.ID, scenario, mode, slices.Contains(taken, "ambush")); s.fail(w, r, jerr) {
			return
		}
		if custom {
			view.Scenario.Err = scenarioErrors(err)
		}
		if !ffa {
			if view.ATO, jerr = s.newATO(r, base, sides, practice, taken); s.fail(w, r, jerr) {
				return
			}
		}
		s.render.page(w, r, http.StatusUnprocessableEntity, "match_new", view)
	case s.fail(w, r, err):
	default:
		redirect(w, r, matchPath(base.Campaign.ID, matchID))
	}
}
