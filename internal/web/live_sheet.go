package web

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// liveSheet is the window of one live action (docs/live-match.md "Unit card in the Field zone", "Wounded a unit").
type liveSheet struct {
	Campaign int64
	MatchID  int64
	Company  int64
	Kind     string // menu (the list), fate, broken, retreat, died, mount, wound
	Unit     core.CompanyUnit
	Live     core.LiveUnit
	Groups   []woundGroup // wound: who can be wounded, enemies first, then friends
	Gains    map[string]int
	// Recorded are the unit's entries of this kind still in the log (wound, fate, broken): the milestone it earned and the way to undo it.
	Recorded []core.LiveEntry

	// The courage sheet and Item used: the match's turn, whether the company is Broken, who may be offered Stand Fast, the dice rolled for the
	// test and why a roll was refused.
	Turn            int
	Broken          bool
	Hero            bool // may call Stand Fast (a Hero that is not a Wanderer; the service refuses the rest with a sentence)
	StandFastCalled bool // a Hero called Stand Fast this turn: the Warriors' hint
	ManualRolls     bool
	CourageDice     core.DiceSpec // the dice of the Courage test (campaign.breakPoint.whenBroken.courageTest.dice)
	Roll            *core.CourageRoll
	RollErr         string
}

// ItemsToUse are the loaned items the unit can still mark as used (not spent, not for the aftermath).
func (v liveSheet) ItemsToUse() (out []core.LoanedItem) {
	for _, it := range v.Live.Loaned {
		if !it.Used && !it.AfterGame {
			out = append(out, it)
		}
	}
	return out
}

// DiceText is the dice of the roll ("3 + 4").
func (v liveSheet) DiceText() string {
	var parts []string
	for _, d := range v.Roll.Dice {
		parts = append(parts, strconv.Itoa(d))
	}
	return strings.Join(parts, " + ")
}

// temporaryUnit is the card of the temporary Wanderer of Against the Odds as a roster unit: it is in no roster.
func temporaryUnit(w *core.TemporaryWanderer) core.CompanyUnit {
	return core.CompanyUnit{ID: w.UnitID, Name: w.Name, Type: "Temporary Wanderer", Role: core.RoleHero, Kind: core.KindInfantry, Points: w.Rating, Wanderer: true}
}

// woundGroup is the units of one company that can be wounded.
type woundGroup struct {
	Company string
	Side    string
	Friends bool // same side as the attacker: the app does not know alliances, so they are listed after a divider
	Targets []woundTarget
}

type woundTarget struct {
	core.CompanyUnit
	Live core.LiveUnit
	Kill bool // a Hero or Monster: slaying it earns the attacker experience
	// Applies are the rules of the attacker's side that name a keyword of this enemy unit (Hatred (Orc) applies).
	Applies []core.RuleMatch
}

// HasEnemies and HasFriends say which parts of the target list exist.
func (v liveSheet) HasEnemies() bool {
	return slices.ContainsFunc(v.Groups, func(g woundGroup) bool { return !g.Friends })
}
func (v liveSheet) HasFriends() bool {
	return slices.ContainsFunc(v.Groups, func(g woundGroup) bool { return g.Friends })
}

// handleLiveSheet answers the window of an action for a unit of {company}; only somebody who may change the company gets it.
func (s *Server) handleLiveSheet(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !slices.Contains([]string{"menu", "fate", "broken", "retreat", "died", "mount", "wound", "courage", "item"}, kind) {
		s.notFound(w, r)
		return
	}
	sheet, ok := s.liveSheetFor(w, r, kind)
	if !ok {
		return
	}
	s.render.partial(w, http.StatusOK, "campaign_my_company", "live_sheet", sheet)
}

// liveSheetFor gathers what the window of a unit needs; it answers the refusals itself.
func (s *Server) liveSheetFor(w http.ResponseWriter, r *http.Request, kind string) (liveSheet, bool) {
	var none liveSheet
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return none, false
	}
	matchID, ok := s.idParam(w, r, "match")
	if !ok {
		return none, false
	}
	companyID, ok := s.idParam(w, r, "company")
	if !ok {
		return none, false
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return none, false
	}
	user := *userFrom(r.Context())
	lc, err := s.campaigns.Live(r.Context(), user, id, companyID)
	if s.fail(w, r, err) {
		return none, false
	}
	if lc.MatchID != matchID {
		s.notFound(w, r)
		return none, false
	}
	if lc.Access != core.LiveEdit {
		s.refuse(w, r, http.StatusForbidden, "Only the owner of this company can change it.")
		return none, false
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), id, companyID)
	if s.fail(w, r, err) {
		return none, false
	}
	sheet := liveSheet{Campaign: id, MatchID: matchID, Company: companyID, Kind: kind, Turn: lc.Turn, Broken: lc.Break.Broken}
	for _, u := range detail.Roster {
		if u.ID == unitID {
			sheet.Unit = u
		}
	}
	if lc.Wanderer != nil && unitID == lc.Wanderer.UnitID {
		sheet.Unit = temporaryUnit(lc.Wanderer)
	}
	for _, u := range lc.Units {
		if u.UnitID == unitID {
			sheet.Live = u
		}
		if lc.Turn > 0 && u.StandFastTurn == lc.Turn {
			sheet.StandFastCalled = true
		}
	}
	if sheet.Unit.ID == 0 || sheet.Live.UnitID == 0 {
		s.notFound(w, r)
		return none, false
	}
	sheet.CourageDice = detail.RuleDice.Courage
	sheet.Hero = sheet.Unit.IsHero() && !sheet.Unit.Wanderer
	sheet.Gains = sheet.Live.Gains
	if kind == "courage" {
		base, ok := s.loadCampaign(w, r, "my-company")
		if !ok {
			return none, false
		}
		sheet.ManualRolls = base.Campaign.Settings.ManualRolls
	}
	if kind == "wound" || kind == "fate" || kind == "broken" {
		if sheet.Recorded, err = s.campaigns.UnitLog(r.Context(), user, id, matchID, unitID, core.LiveKind(kind)); s.fail(w, r, err) {
			return none, false
		}
	}
	if kind == "wound" {
		if sheet.Groups, err = s.woundTargets(r, id, matchID, companyID, unitID); s.fail(w, r, err) {
			return none, false
		}
	}
	return sheet, true
}

// handleCourageRoll rolls the Courage test of a unit (nothing is written) and answers the courage window with the result.
func (s *Server) handleCourageRoll(w http.ResponseWriter, r *http.Request) {
	sheet, ok := s.liveSheetFor(w, r, "courage")
	if !ok {
		return
	}
	entered := enteredRolls(r)
	status := http.StatusOK
	roll, err := s.campaigns.RollCourage(r.Context(), *userFrom(r.Context()), sheet.Campaign, sheet.MatchID, sheet.Unit.ID, entered, false)
	switch {
	case err == nil:
		sheet.Roll = &roll
	case errors.Is(err, core.ErrRefused), errors.Is(err, core.ErrInvalidLive):
		status, sheet.RollErr = http.StatusUnprocessableEntity, reason(err)
	default:
		s.fail(w, r, err)
		return
	}
	s.render.partial(w, status, "campaign_my_company", "live_sheet", sheet)
}

// woundTargets lists the units in the Field of every company of the match, except the attacker: the other sides first.
func (s *Server) woundTargets(r *http.Request, id, matchID, companyID, attacker int64) ([]woundGroup, error) {
	m, err := s.campaigns.Match(r.Context(), id, matchID)
	if err != nil {
		return nil, err
	}
	own := -1
	for i, side := range m.Sides {
		if slices.ContainsFunc(side.Companies, func(c core.CampaignCompany) bool { return c.ID == companyID }) {
			own = i
		}
	}
	user := *userFrom(r.Context())
	var enemies, friends []woundGroup
	var mine []core.TargetUnit // the Field units of the attacker's side, the attacker included: their rules are what the markers show
	for i, side := range m.Sides {
		for _, co := range side.Companies {
			lc, err := s.campaigns.Live(r.Context(), user, id, co.ID)
			if err != nil {
				return nil, err
			}
			detail, err := s.campaigns.CompanyDetail(r.Context(), id, co.ID)
			if err != nil {
				return nil, err
			}
			g := woundGroup{Company: co.Name, Side: side.Label(), Friends: i == own}
			if g.Friends {
				mine = append(mine, targetUnits(detail.Roster, lc)...)
			}
			for _, lu := range lc.Units {
				if lu.Zone != core.ZoneField || lu.UnitID == attacker {
					continue
				}
				if lu.Temporary && lc.Wanderer != nil {
					g.Targets = append(g.Targets, woundTarget{CompanyUnit: temporaryUnit(lc.Wanderer), Live: lu, Kill: true})
				} else if k := slices.IndexFunc(detail.Roster, func(u core.CompanyUnit) bool { return u.ID == lu.UnitID }); k >= 0 {
					u := detail.Roster[k]
					g.Targets = append(g.Targets, woundTarget{CompanyUnit: u, Live: lu, Kill: u.IsHero() || u.Kind == core.KindMonster})
				}
			}
			if len(g.Targets) == 0 {
				continue
			}
			if g.Friends {
				friends = append(friends, g)
			} else {
				enemies = append(enemies, g)
			}
		}
	}
	for gi := range enemies {
		var theirs []core.TargetUnit
		for _, t := range enemies[gi].Targets {
			if tu, ok := core.NewTargetUnit(t.CompanyUnit, t.Live); ok {
				theirs = append(theirs, tu)
			}
		}
		found := core.RuleMatches(mine, theirs)
		for ti := range enemies[gi].Targets {
			enemies[gi].Targets[ti].Applies = found[enemies[gi].Targets[ti].ID]
		}
	}
	return append(enemies, friends...), nil
}
