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

// The Against the Odds card of the match page (docs/matches.md, "Against the Odds before the game"): the lower rated company
// chooses its bonuses after the scenario roll. The card is a slot of the match page loaded with hx-get; a save answers the whole
// match (the revision moved and Ambush! may have changed the Scenario card) or, when refused, the card alone.

type atoView struct {
	core.ATO
	Campaign int64
	Ended    bool
	Groups   []scenarioGroup // the scenarios Ambush! may choose from
	Msg      string
	Error    bool
}

// Show is whether the card is drawn at all: an open match always when the rules offer bonuses; an ended one only when something was chosen.
func (v atoView) Show() bool {
	if v.Ended {
		return v.Eligible && v.Chosen()
	}
	return v.Eligible || v.Why != ""
}

// Chosen is whether the company took anything.
func (v atoView) Chosen() bool { return len(v.Selection.Bonuses) > 0 }

// Editor is whether the viewer may change the selection.
func (v atoView) Editor() bool { return v.Eligible && v.Access == core.ATOEdit }

// Left is what the saved selection leaves of the gap.
func (v atoView) Left() int { return v.Gap - v.Spent }

// HardReason is why an option cannot be taken for a reason other than the budget (that one is worked out live in the browser).
func (v atoView) HardReason(o core.ATOOption) string {
	if o.Disabled && !strings.HasPrefix(o.Reason, "Needs") {
		return o.Reason
	}
	return ""
}

// ToolKitRating is the Rating of one kit.
func (v atoView) ToolKitRating() int {
	for _, o := range v.Options {
		if o.Key == "toolKit" {
			return o.Rating
		}
	}
	return 0
}

// State is what the form's Alpine component starts with.
func (v atoView) State() string {
	ratings, names := map[string]int{}, map[string]string{}
	for _, o := range v.Options {
		ratings[o.Key], names[o.Key] = o.Rating, o.Name
	}
	st := map[string]any{"gap": v.Gap, "ratings": ratings, "names": names, "bonus": nonNil(v.Selection.Bonuses), "kits": nonNil(v.Selection.Kits),
		"wanderer": v.Selection.Wanderer, "ambush": v.Selection.Ambush}
	b, _ := json.Marshal(st)
	return string(b)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Assigned is the saved assignment of one copy of a kit item.
func (v atoView) Assigned(kit, item string, copy int) core.KitAssignment {
	for _, a := range v.Selection.Items {
		if a.Kit == kit && a.Item == item && a.Index == copy {
			return a
		}
	}
	return core.KitAssignment{}
}

// Models lists who may carry an item.
func (v atoView) Models(heroOnly bool) (out []core.ATOModel) {
	for _, m := range v.ATO.Models {
		if !heroOnly || m.Hero {
			out = append(out, m)
		}
	}
	return out
}

// Copies is 0..n-1 for the copies of a kit item.
func (v atoView) Copies(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// ModelName names a model of the card.
func (v atoView) ModelName(id int64) string {
	for _, m := range v.ATO.Models {
		if m.UnitID == id {
			return m.Name
		}
	}
	return ""
}

// KitName names a kit.
func (v atoView) KitName(key string) string {
	for _, k := range v.Kits {
		if k.Key == key {
			return k.Name
		}
	}
	return key
}

// Summary is the plain text of what the company took ("Hador took: Reroll, Wanderer (Name), Healer's Kit.").
func (v atoView) Summary() string {
	var parts []string
	for _, o := range v.Options {
		if !o.Taken {
			continue
		}
		switch o.Key {
		case "toolKit":
			var kits []string
			for _, k := range v.Selection.Kits {
				kits = append(kits, v.KitName(k))
			}
			parts = append(parts, o.Name+" ("+strings.Join(kits, ", ")+")")
		case "wanderer":
			parts = append(parts, o.Name+" ("+v.Selection.Wanderer+")")
		case "ambush":
			parts = append(parts, o.Name+" (scenario: "+v.Selection.Ambush+")")
		default:
			parts = append(parts, o.Name)
		}
	}
	if len(parts) == 0 {
		return v.CompanyName + " took no bonus."
	}
	return v.CompanyName + " took: " + strings.Join(parts, "; ") + "."
}

func (s *Server) atoCard(r *http.Request, id int64, m core.Match) (atoView, error) {
	a, err := s.campaigns.AgainstTheOdds(r.Context(), *userFrom(r.Context()), id, m.ID)
	if err != nil {
		return atoView{}, err
	}
	v := atoView{ATO: a, Campaign: id, Ended: m.Ended}
	if v.Editor() && slices.ContainsFunc(a.Options, func(o core.ATOOption) bool { return o.Key == "ambush" }) {
		list, err := s.campaigns.Scenarios(r.Context(), id)
		if err != nil {
			return v, err
		}
		v.Groups = groupScenarios(list)
	}
	return v, nil
}

// handleATOCard answers the card.
func (s *Server) handleATOCard(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	id := base.Campaign.ID
	v, err := s.atoCard(r, id, m)
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, http.StatusOK, "match", "ato_card", v)
}

// selectionFromForm reads the card: the ticked bonuses, kits, where each kit item goes (assign-<kit>-<item>-<copy>, detail-...),
// the Wanderer and the Ambush! scenario. Only what belongs to a ticked bonus is read.
func selectionFromForm(r *http.Request, a core.ATO) core.ATOSelection {
	_ = r.ParseForm()
	var sel core.ATOSelection
	sel.Bonuses = slices.Clone(r.PostForm["bonus"])
	if sel.Has("toolKit") {
		sel.Kits = slices.Clone(r.PostForm["kit"])
		for _, k := range a.Kits {
			if !slices.Contains(sel.Kits, k.Key) {
				continue
			}
			for ii, it := range k.Items {
				for c := 0; c < it.Count; c++ {
					name := fmt.Sprintf("%s-%d-%d", k.Key, ii, c)
					unit, _ := strconv.ParseInt(r.PostFormValue("assign-"+name), 10, 64)
					if unit == 0 {
						continue
					}
					sel.Items = append(sel.Items, core.KitAssignment{Kit: k.Key, Item: it.Item, Index: c, UnitID: unit, Detail: strings.TrimSpace(r.PostFormValue("detail-" + name))})
				}
			}
		}
	}
	if sel.Has("wanderer") {
		sel.Wanderer = r.PostFormValue("wanderer")
	}
	if sel.Has("ambush") {
		sel.Ambush = r.PostFormValue("ambush")
	}
	return sel
}

// handleATOSave replaces the selection. Success answers the whole match (htmx retargets #match); a refusal answers the card.
func (s *Server) handleATOSave(w http.ResponseWriter, r *http.Request) {
	base, m, ok := s.matchCtx(w, r)
	if !ok {
		return
	}
	id := base.Campaign.ID
	user := *userFrom(r.Context())
	cur, err := s.campaigns.AgainstTheOdds(r.Context(), user, id, m.ID)
	if s.fail(w, r, err) {
		return
	}
	if cur.Access != core.ATOEdit {
		msg := "Only the lower rated company's player, the organiser and administrators can change Against the Odds."
		if cur.Locked {
			msg = cur.LockedWhy
		} else if !cur.Eligible && cur.Why != "" {
			msg = cur.Why
		}
		s.refuse(w, r, http.StatusForbidden, msg)
		return
	}
	sel := selectionFromForm(r, cur)
	_, err = s.campaigns.SaveAgainstTheOdds(r.Context(), user, id, m.ID, revisionOf(r), sel)
	status, msg := http.StatusOK, ""
	switch {
	case err == nil:
		fresh, merr := s.campaigns.Match(r.Context(), id, m.ID)
		if s.fail(w, r, merr) {
			return
		}
		w.Header().Set("HX-Retarget", "#match")
		w.Header().Set("HX-Reswap", "outerHTML")
		s.answer(w, r, http.StatusOK, base, fresh, "", "Against the Odds saved.", false, nil)
		return
	case errors.Is(err, core.ErrMatchChanged):
		msg = "Somebody changed this match while you were editing, so what you see now is the current state. Check it and make your change again."
	case errors.Is(err, core.ErrRefused):
		status, msg = http.StatusUnprocessableEntity, reason(err)
	case errors.Is(err, core.ErrInvalidMatch):
		status, msg = http.StatusUnprocessableEntity, strings.Join(reasons(err), " ")
	case errors.Is(err, core.ErrNotFound):
		s.notFound(w, r)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	v, cerr := s.atoCard(r, id, m)
	if s.fail(w, r, cerr) {
		return
	}
	if status == http.StatusUnprocessableEntity {
		v.Selection, v.Spent = sel, 0 // keep what was typed
	}
	v.Msg, v.Error = msg, true
	s.render.partial(w, status, "match", "ato_card", v)
}
