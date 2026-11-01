package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The windows of the My Company page (recruiting, buying equipment) are fetched when they open and their forms answer
// with the whole page again, with a notice saying what happened.

type specialView struct {
	core.SpecialRecruit
	Reason string // empty when the company can recruit it now
}

type recruitPanel struct {
	Campaign   core.CampaignDetail
	Co         int64 // the company the window acts on (the one of the URL)
	Company    core.CompanyDetail
	Reason     string // why a new recruit is not possible now; empty when it is
	Special    []specialView
	Roll       *core.RecruitRoll // the recruit in progress, nil when none
	Error      string
	Rendezvous bool // the company holds an unused Rendezvous: the first roll offers +1 / -1
	Animate    bool // the roll was just made by the server: show the dice rolling to it before revealing it
}

// recruitPanelFor builds the recruit window for the signed-in user's company.
func (s *Server) recruitPanelFor(r *http.Request, c core.CampaignDetail, companyID int64, errMsg string, animate bool) (recruitPanel, error) {
	user := *userFrom(r.Context())
	detail, err := s.campaigns.CompanyDetail(r.Context(), c.ID, companyID)
	if err != nil {
		return recruitPanel{}, err
	}
	roll, err := s.campaigns.RecruitState(r.Context(), user, c.ID, companyID)
	if err != nil {
		return recruitPanel{}, err
	}
	_, rendezvous := core.UnusedPerk(detail.Perks, core.PerkRendezvous)
	panel := recruitPanel{Campaign: c, Co: companyID, Company: detail, Roll: roll, Error: errMsg, Animate: animate && roll != nil, Rendezvous: rendezvous}
	_, panel.Reason = detail.CanRecruit(detail.Recruit.Cost)
	for _, sp := range detail.Recruit.Special {
		_, why := detail.CanRecruit(sp.Cost)
		if why == "" && sp.Blocked != "" {
			why = "Cannot join: " + sp.Blocked + "."
		}
		panel.Special = append(panel.Special, specialView{SpecialRecruit: sp, Reason: why})
	}
	return panel, nil
}

// showRecruitPanel answers a step of the recruit with the window again (the player stays in it).
func (s *Server) showRecruitPanel(w http.ResponseWriter, r *http.Request, status int, errMsg string, animate bool) {
	base, ok := s.loadOwn(w, r, "my-company")
	if !ok {
		return
	}
	// a recruit waiting for its options comes first: reopening the window continues it
	pending, err := s.campaigns.PendingChoice(r.Context(), *userFrom(r.Context()), base.Campaign.ID, base.Current.ID)
	if s.fail(w, r, err) {
		return
	}
	if pending != nil {
		s.render.partial(w, status, "campaign_my_company", "choice_panel", map[string]any{"Campaign": base.Campaign, "Co": base.Current.ID, "Choice": pending})
		return
	}
	// so does a Ranger of the North waiting for the officers to be arranged
	if officers, err := s.campaigns.PendingOfficers(r.Context(), *userFrom(r.Context()), base.Campaign.ID, base.Current.ID); s.fail(w, r, err) {
		return
	} else if officers != nil {
		s.render.partial(w, status, "campaign_my_company", "officers_panel", map[string]any{"Campaign": base.Campaign, "Co": base.Current.ID, "Panel": officers, "Error": errMsg})
		return
	}
	panel, err := s.recruitPanelFor(r, base.Campaign, base.Current.ID, errMsg, animate)
	if s.fail(w, r, err) {
		return
	}
	s.render.partials(w, status, renderPart{"campaign_my_company", "recruit_panel", panel}, renderPart{"campaign_my_company", "influence_oob", panel})
}

func (s *Server) handleRecruitPanel(w http.ResponseWriter, r *http.Request) {
	s.showRecruitPanel(w, r, http.StatusOK, "", false)
}

// handleRollRecruit makes the roll (the player's own, or random) and shows it in the window to be confirmed or adjusted.
func (s *Server) handleRollRecruit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	roll, _ := strconv.Atoi(r.PostFormValue("roll")) // empty or 0: random
	thrown := r.PostFormValue("thrown") != ""        // the browser threw the dice and reports the result
	perk, _ := strconv.Atoi(r.PostFormValue("perk")) // the Rendezvous bonus: -1, 0 or +1
	err := s.campaigns.RollRecruitWithPerk(r.Context(), *userFrom(r.Context()), id, cid, roll, thrown, perk)
	if errors.Is(err, core.ErrRefused) || errors.Is(err, core.ErrInvalidRoll) {
		s.showRecruitPanel(w, r, http.StatusUnprocessableEntity, reason(err), false)
		return
	}
	if s.fail(w, r, err) {
		return
	}
	// only a roll the server made is animated: a thrown or entered roll was already seen by the player
	s.showRecruitPanel(w, r, http.StatusOK, "", roll == 0 && !thrown)
}

// handleConfirmRecruit settles the recruit with the player's adjustment. It ends the recruit (the whole page answers, the
// window closes) unless the roll led to the special chart, which keeps the window open for the second roll.
func (s *Server) handleConfirmRecruit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	adjust, _ := strconv.Atoi(r.PostFormValue("adjust"))
	res, err := s.campaigns.ConfirmRecruit(r.Context(), *userFrom(r.Context()), id, cid, adjust, r.PostFormValue("pick"))
	s.afterConfirm(w, r, res, err)
}

// handleChooseLower settles a recruit whose result could not join with the lower result the player chose.
func (s *Server) handleChooseLower(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	index, err := strconv.Atoi(r.PostFormValue("result"))
	if err != nil {
		s.showRecruitPanel(w, r, http.StatusUnprocessableEntity, "Choose one of the results.", false)
		return
	}
	res, err := s.campaigns.ChooseLower(r.Context(), *userFrom(r.Context()), id, cid, index)
	s.afterConfirm(w, r, res, err)
}

// afterConfirm answers a step that settles a recruit: it may need another roll or a lower result (the window stays), or be over.
func (s *Server) afterConfirm(w http.ResponseWriter, r *http.Request, res core.RecruitResult, err error) {
	switch {
	case errors.Is(err, core.ErrRefused):
		s.showRecruitPanel(w, r, http.StatusUnprocessableEntity, reason(err), false)
	case s.fail(w, r, err):
	case res.Officers != nil: // the Ranger of the North was chosen: the window turns into the officer panel
		s.showRecruitPanel(w, r, http.StatusOK, "", false)
	case res.Next || res.Lower:
		s.showRecruitPanel(w, r, http.StatusOK, "", false)
	default:
		w.Header().Set("HX-Retarget", "#main") // the recruit is over: the page answers and the window goes away
		w.Header().Set("HX-Reswap", "innerHTML show:window:top")
		s.afterAction(w, r, res.Summary, nil)
	}
}

// handleRecruit buys one of the company's special units directly (the form names it).
func (s *Server) handleRecruit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	res, err := s.campaigns.RecruitSpecial(r.Context(), *userFrom(r.Context()), id, cid, r.PostFormValue("special"))
	s.afterAction(w, r, res.Summary, err)
}

// handleChooseOptions finishes a "choice of options" recruit with the option the player picked (empty: the base profile).
func (s *Server) handleChooseOptions(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	res, err := s.campaigns.ChooseOptions(r.Context(), *userFrom(r.Context()), id, cid, r.PostFormValue("option"))
	s.afterAction(w, r, res.Summary, err)
}

type armouryPanel struct {
	Campaign  core.CampaignDetail
	Co        int64
	Unit      core.CompanyUnit
	Slots     core.Slots
	Items     []armouryItemView
	Influence int
	Kind      string           // "wargear" or "equipment": which half of the armoury the window shows; empty shows both
	Creatures []creatureChoice // what an officer can also take: creatures
}

// armouryItemView is an item of the buy window with its description cut into text and glossary links (the item's own name is never one).
type armouryItemView struct {
	core.ArmouryItem
	Segments []segView
}

// creatureChoice is a creature and whether this officer can take it.
type creatureChoice struct {
	core.ArmouryCreature
	Reason string
}

func (s *Server) handleArmouryPanel(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadOwn(w, r, "my-company")
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, base.Current.ID)
	if s.fail(w, r, err) {
		return
	}
	items, err := s.campaigns.Armoury(r.Context(), base.Campaign.ID, base.Current.ID, unitID)
	if s.fail(w, r, err) {
		return
	}
	g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	panel := armouryPanel{Campaign: base.Campaign, Co: base.Current.ID, Influence: detail.Influence}
	if k := r.URL.Query().Get("kind"); k == "wargear" || k == "equipment" {
		panel.Kind = k
	}
	for _, it := range items {
		// What a rule forbids is not shown; the rest is, with the reason when it cannot be bought now.
		if it.Offered() && (panel.Kind == "" || (panel.Kind == "equipment") == (it.Size != "")) {
			panel.Items = append(panel.Items, armouryItemView{ArmouryItem: it, Segments: itemText(g, it.Description, append([]string{it.Name}, it.AlsoCalled...)...)})
		}
	}
	for _, u := range detail.Roster {
		if u.ID == unitID {
			panel.Unit, panel.Slots = u, u.Slots()
		}
	}
	creatures, err := s.campaigns.ArmouryCreatures(r.Context(), base.Campaign.ID, base.Current.ID)
	if s.fail(w, r, err) {
		return
	}
	for _, c := range creatures {
		if panel.Kind == "equipment" {
			break
		}
		for _, o := range c.Owners {
			if o.ID == unitID {
				panel.Creatures = append(panel.Creatures, creatureChoice{ArmouryCreature: c, Reason: o.Reason})
			}
		}
	}
	s.render.partial(w, http.StatusOK, "campaign_my_company", "armoury_panel", panel)
}

// buyEquipment buys the item with the form's payment and, for a weapon modifier (Envenom Weapon), the weapon the player chose:
// the buy window and the Armory tab ask for it the same way (the shared weapon_chooser).
func (s *Server) buyEquipment(r *http.Request, id, cid, unitID int64, item string) error {
	u := *userFrom(r.Context())
	if weapon := r.PostFormValue("weapon"); weapon != "" {
		return s.campaigns.BuyEquipmentFor(r.Context(), u, id, cid, unitID, item, r.PostFormValue("pay"), weapon)
	}
	return s.campaigns.BuyEquipment(r.Context(), u, id, cid, unitID, item, r.PostFormValue("pay"))
}

func (s *Server) handleBuyEquipment(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	item := r.PostFormValue("item")
	err := s.buyEquipment(r, id, cid, unitID, item)
	s.afterAction(w, r, "Bought "+item+".", err)
}

// handleChooseModifierWeapon chooses the weapon a weapon modifier of the unit without one is for; the choice is final, core refuses a second.
func (s *Server) handleChooseModifierWeapon(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	item, weapon := r.PostFormValue("item"), r.PostFormValue("weapon")
	err := s.campaigns.ChooseModifierWeapon(r.Context(), *userFrom(r.Context()), id, cid, unitID, item, weapon)
	s.afterAction(w, r, "Poisoned Attacks now applies to the "+weapon+".", err)
}

func (s *Server) handleDropEquipment(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	item := r.PostFormValue("item")
	err := s.campaigns.DropEquipment(r.Context(), *userFrom(r.Context()), id, cid, unitID, item)
	s.afterAction(w, r, "Dropped "+item+".", err)
}

// treatPanel is the window for treating a hero's injuries.
type treatPanel struct {
	Campaign  core.CampaignDetail
	Co        int64
	Unit      core.CompanyUnit
	Influence int
	Rules     core.InjuryRules
	Attempt   []core.UnitInjury // what the attempt can remove now
	Healers   []core.UnitInjury // what the healers can
	Result    *core.TreatResult // what the last treatment did
	Animate   bool              // the roll was made by the server: show the dice rolling to it first
	Error     string
}

func (s *Server) showTreatPanel(w http.ResponseWriter, r *http.Request, status int, unitID int64, result *core.TreatResult, errMsg string, animate bool) {
	base, ok := s.loadOwn(w, r, "my-company")
	if !ok {
		return
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, base.Current.ID)
	if s.fail(w, r, err) {
		return
	}
	panel := treatPanel{Campaign: base.Campaign, Co: base.Current.ID, Influence: detail.Influence, Rules: detail.Injuries, Result: result, Error: errMsg, Animate: animate}
	found := false
	for _, u := range detail.Roster {
		if u.ID == unitID {
			panel.Unit, found = u, true
		}
	}
	if !found {
		s.notFound(w, r)
		return
	}
	if result == nil { // a roll that waits for the decision on extra influence is shown again
		if result, err = s.campaigns.TreatmentState(r.Context(), *userFrom(r.Context()), base.Campaign.ID, base.Current.ID, unitID); s.fail(w, r, err) {
			return
		}
		panel.Result = result
	}
	panel.Attempt = detail.Injuries.Healable(panel.Unit, detail.Injuries.Attempt)
	panel.Healers = detail.Injuries.Healable(panel.Unit, detail.Injuries.Healers)
	s.render.partial(w, status, "campaign_my_company", "treat_panel", panel)
}

func (s *Server) handleTreatPanel(w http.ResponseWriter, r *http.Request) {
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	s.showTreatPanel(w, r, http.StatusOK, unitID, nil, "", false)
}

// handleTreatInjury pays to treat a unit's injuries. A warrior's treatment answers with the page; a hero's with the window,
// showing what the roll gave, so that a failed attempt can be tried again.
func (s *Server) handleTreatInjury(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	t := core.Treatment{Method: r.PostFormValue("method"), Injury: r.PostFormValue("injury")}
	t.Roll, _ = strconv.Atoi(r.PostFormValue("roll"))
	t.Extra, _ = strconv.Atoi(r.PostFormValue("extra"))
	t.Resolve = r.PostFormValue("resolve") != ""
	t.Thrown = r.PostFormValue("thrown") != ""
	thrown := r.PostFormValue("thrown") != ""
	user := *userFrom(r.Context())
	res, err := s.campaigns.TreatInjury(r.Context(), user, id, cid, unitID, t)
	if t.Method == "warrior" {
		s.afterAction(w, r, res.Summary, err)
		return
	}
	switch {
	case errors.Is(err, core.ErrRefused):
		s.showTreatPanel(w, r, http.StatusUnprocessableEntity, unitID, nil, reason(err), false)
	case s.fail(w, r, err):
	default:
		s.showTreatPanel(w, r, http.StatusOK, unitID, &res, "", res.Rolled && !t.Resolve && t.Roll == 0 && !thrown)
	}
}

// handleSaveCemetery stores the order of the fallen units.
func (s *Server) handleSaveCemetery(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	err := s.campaigns.SaveCemeteryOrder(r.Context(), *userFrom(r.Context()), id, cid, idList(r.PostFormValue("order")))
	s.afterAction(w, r, "Cemetery order saved.", err)
}

// idList reads unit ids separated by commas.
func idList(text string) []int64 {
	var out []int64
	for _, part := range strings.Split(text, ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// handleSaveFormation stores the unit order the player chose; the form carries the unit ids, comma separated.
func (s *Server) handleSaveFormation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	var order []int64
	for _, part := range strings.Split(r.PostFormValue("order"), ",") {
		if n, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
			order = append(order, n)
		}
	}
	err := s.campaigns.SaveFormation(r.Context(), *userFrom(r.Context()), id, cid, order)
	s.afterAction(w, r, "Formation saved.", err)
}

// afterAction shows the My Company page again: with the notice when it worked, or the reason when the rules refused.
func (s *Server) afterAction(w http.ResponseWriter, r *http.Request, done string, err error) {
	switch {
	case errors.Is(err, core.ErrRefused):
		s.myCompanyPage(w, r, http.StatusUnprocessableEntity, reason(err), true)
	case s.fail(w, r, err):
	default:
		s.myCompanyPage(w, r, http.StatusOK, done, false)
	}
}

// handleRenameUnit renames a unit; an empty name takes the unit back to its type.
func (s *Server) handleRenameUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	err := s.campaigns.RenameUnit(r.Context(), *userFrom(r.Context()), id, cid, unitID, r.PostFormValue("name"))
	s.afterAction(w, r, "Renamed.", err)
}

// handlePathPanel is the window where a leader or sergeant without a path chooses one.
func (s *Server) handlePathPanel(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadOwn(w, r, "my-company")
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	s.showPathPanel(w, r, base, unitID, http.StatusOK, "")
}

func (s *Server) showPathPanel(w http.ResponseWriter, r *http.Request, base campaignBase, unitID int64, status int, errMsg string) {
	offer, err := s.campaigns.PathOptions(r.Context(), *userFrom(r.Context()), base.Campaign.ID, base.Current.ID, unitID)
	if errors.Is(err, core.ErrRefused) {
		errMsg, err = reason(err), nil
	}
	if s.fail(w, r, err) {
		return
	}
	g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID) // the texts link to the campaign's glossary
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, status, "campaign_my_company", "path_panel", map[string]any{"Campaign": base.Campaign, "Co": base.Current.ID, "Offer": offer, "Cards": pathCards(g, offer.Paths), "Error": errMsg})
}

// pathCard is one path of the carousel with all that is known of it: what PathOffer holds plus the glossary's limits and chart. The chart
// already names every rule and Mastery the path unlocks, so they are not listed again.
type pathCard struct {
	core.PathOption
	Look     pathLookView
	Heroic   []segView // the heroic action it starts with, linked (it is not a row of the chart)
	Desc     []segView
	Limits   []core.PathTableLimit // the most each characteristic of the profile can reach
	Hero     []core.PathTableLimit // the most Might, Will and Fate can reach, shown as on a hero's card
	Dice     string                // the dice its chart is rolled with
	Rows     []pathRowView
	HasChart bool
}

// pathRowView is one result of the chart; Alts are its alternatives (one when the row offers a single thing), each linked.
type pathRowView struct {
	Roll int
	Alts [][]segView
}

func pathCards(g core.Glossary, paths []core.PathOption) []pathCard {
	out := make([]pathCard, len(paths))
	for i, p := range paths {
		c := pathCard{PathOption: p, Look: pathLook(p.Name), Desc: linkText(g, p.Description)}
		if p.StartAction != "" {
			c.Heroic = linkText(g, p.StartAction)
		}
		if term, ok := g.Find(p.Name); ok && term.Path != nil {
			for _, l := range term.Path.Limits {
				if l.Stat == "Might" || l.Stat == "Will" || l.Stat == "Fate" {
					c.Hero = append(c.Hero, l)
				} else {
					c.Limits = append(c.Limits, l)
				}
			}
			if term.Path.PathDice.Valid() {
				c.Dice = term.Path.PathDice.Display()
			}
			for _, row := range term.Path.Rows {
				var text strings.Builder
				for _, seg := range row.Segments {
					text.WriteString(seg.Text)
				}
				rv := pathRowView{Roll: row.Roll}
				for _, alt := range splitAlternatives(text.String()) {
					rv.Alts = append(rv.Alts, linkText(g, alt))
				}
				c.Rows = append(c.Rows, rv)
			}
			c.HasChart = len(c.Rows) > 0
		}
		out[i] = c
	}
	return out
}

// splitAlternatives cuts the text of a chart row into the alternatives the data writes: at an upper-case " OR " (the file's own marker), at
// ", or improve " (a second result of an "Improve..." row) and before " Pick an additional " in a row that starts "Choice of" (a minor rule or a
// heroic action). An ordinary lower-case "or" inside a sentence ("Improve Might or Fate by 1") is left alone.
func splitAlternatives(text string) []string {
	var out []string
	for _, a := range strings.Split(text, " OR ") {
		if head, tail, ok := strings.Cut(a, ", or improve "); ok {
			out = append(out, head, "Improve "+tail)
			continue
		}
		out = append(out, a)
	}
	var split []string
	for _, a := range out {
		if strings.HasPrefix(a, "Choice of") {
			if head, tail, ok := strings.Cut(a, " Pick an additional "); ok {
				split = append(split, head, "Pick an additional "+tail)
				continue
			}
		}
		split = append(split, a)
	}
	var clean []string
	for _, a := range split {
		if a = strings.TrimSpace(a); a != "" {
			r, n := utf8.DecodeRuneInString(a)
			clean = append(clean, string(unicode.ToUpper(r))+a[n:]) // an alternative starts a sentence
		}
	}
	if len(clean) == 0 {
		return []string{text}
	}
	return clean
}

// handleChoosePath gives the officer the path the player chose.
func (s *Server) handleChoosePath(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadOwn(w, r, "my-company")
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	err := s.campaigns.ChoosePath(r.Context(), *userFrom(r.Context()), base.Campaign.ID, base.Current.ID, unitID, r.PostFormValue("path"), r.PostFormValue("power"))
	if errors.Is(err, core.ErrRefused) {
		s.showPathPanel(w, r, base, unitID, http.StatusUnprocessableEntity, reason(err))
		return
	}
	w.Header().Set("HX-Retarget", "#main")
	w.Header().Set("HX-Reswap", "innerHTML show:window:top")
	s.afterAction(w, r, "The path is chosen.", err)
}

// handleRenameCompany renames the player's company.
func (s *Server) handleRenameCompany(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	err := s.campaigns.RenameCompany(r.Context(), *userFrom(r.Context()), id, cid, r.PostFormValue("name"))
	s.afterAction(w, r, "The company has a new name.", err)
}

// handleArrangeOfficers appoints the officers when a Ranger of the North takes command: the form names the unit for the Leader place
// ("leader", 0 is the Ranger) and for each Sergeant place ("sergeant", repeated).
func (s *Server) handleArrangeOfficers(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	cid, ok := s.ownCompanyID(w, r, id)
	if !ok {
		return
	}
	leader, err := strconv.ParseInt(r.PostFormValue("leader"), 10, 64)
	if err != nil {
		s.showRecruitPanel(w, r, http.StatusUnprocessableEntity, "Choose a Leader.", false)
		return
	}
	a := core.OfficerArrangement{Leader: leader}
	for _, v := range r.PostForm["sergeant"] {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			s.showRecruitPanel(w, r, http.StatusUnprocessableEntity, "That unit cannot be appointed.", false)
			return
		}
		a.Sergeants = append(a.Sergeants, n)
	}
	res, err := s.campaigns.ArrangeOfficers(r.Context(), *userFrom(r.Context()), id, cid, a)
	if errors.Is(err, core.ErrRefused) {
		s.showRecruitPanel(w, r, http.StatusUnprocessableEntity, reason(err), false)
		return
	}
	if s.fail(w, r, err) {
		return
	}
	w.Header().Set("HX-Retarget", "#main") // the officers are settled: the page answers and the window goes away
	w.Header().Set("HX-Reswap", "innerHTML show:window:top")
	s.afterAction(w, r, res.Summary, nil)
}
