package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// levelPanel is the level-up window of a unit: the roll form, then the results to choose from, then what happened.
type levelPanel struct {
	Campaign core.CampaignDetail
	Co       int64 // the company the window acts on (the one of the URL)
	Unit     core.CompanyUnit
	Roll     *core.LevelRoll // nil before the roll
	Offers   []offerView     // what the roll offers, each choice with its description
	Upgrade  *offerView      // the company hero upgrades, taken instead of either result
	Unusable []pairView      // results of the hero's two rolls that give it nothing it can take
	Dice     string          // the dice to show rolling, comma separated
	Groups   string          // how they are grouped in lanes (a hero's two rolls): "2,2"
	Labels   string          // the lanes' names
	Animate  bool            // the roll was made by the app: show the dice first
	Pending  int             // level-up rolls the unit still has waiting
	Rerolls  int             // Against the Odds re-rolls the company has
	Error    string
	Fresh    bool         // the roll was made just now: a celebration plays only then, reopening the window shows it still
	Heroic   bool         // a warrior's Hero in the Making waiting to be accepted
	Forgo    []choiceView // what it can take instead of becoming a hero (the promotions)
	Plan     string       // the roll's core.LevelPlan as JSON, for the marks of the window
	// CloseAfter is how many milliseconds after the reveal the window closes by itself: a warrior's no change with no roll left.
	CloseAfter int
	// Spec is the dice of this unit's roll (a warrior's, or the hero's path chart) and Pairs how many results a hero rolls: the form,
	// the dice tray and the heading are sized by them (campaign.progression, data.paths[].dice).
	Spec  core.DiceSpec
	Pairs int
}

func (s *Server) showLevelPanel(w http.ResponseWriter, r *http.Request, status int, unitID int64, roll *core.LevelRoll, animate bool, errMsg string) {
	base, ok := s.loadOwn(w, r, "my-company")
	if !ok {
		return
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, base.Current.ID)
	if s.fail(w, r, err) {
		return
	}
	panel := levelPanel{Campaign: base.Campaign, Co: base.Current.ID, Roll: roll, Animate: animate, Error: errMsg, Fresh: roll != nil}
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
	panel.Pending, panel.Rerolls = panel.Unit.PendingLevelUps(), detail.Rerolls
	panel.Spec, panel.Pairs = detail.RuleDice.Level.Warrior, 1
	if panel.Unit.IsHero() {
		panel.Spec, panel.Pairs = detail.RuleDice.Level.Hero, detail.RuleDice.Level.Pairs
		if panel.Unit.Path != nil {
			panel.Spec = detail.RuleDice.Level.HeroFor(panel.Unit.Path.Name)
		}
	}
	glossary, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID) // to describe a rule, an action or a power with links to its terms
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if roll == nil {
		if roll, err = s.campaigns.LevelUpState(r.Context(), *userFrom(r.Context()), base.Campaign.ID, base.Current.ID, unitID); s.fail(w, r, err) {
			return
		}
		panel.Roll = roll
	}
	if panel.Roll != nil {
		panel.Offers = offerViews(glossary, panel.Roll.Offers)
		if panel.Roll.Upgrade != nil {
			up := offerViews(glossary, []core.LevelOffer{*panel.Roll.Upgrade})[0]
			panel.Upgrade = &up
		}
		if panel.Roll.Plan != nil {
			plan, err := json.Marshal(panel.Roll.Plan)
			if s.fail(w, r, err) {
				return
			}
			panel.Plan = string(plan)
		}
		if panel.Roll.Done && panel.Roll.Result == "noChange" && panel.Pending == 0 {
			panel.CloseAfter = noChangeCloseMillis
		}
		if !panel.Roll.Done && panel.Roll.Stage == "" && panel.Roll.Result == "heroInTheMaking" && len(panel.Offers) == 1 {
			panel.Heroic = true
			for _, c := range panel.Offers[0].Choices {
				if c.Key != "hero" {
					panel.Forgo = append(panel.Forgo, c)
				}
			}
		}
		if len(panel.Roll.Offers) > 0 { // a result nothing of which can be taken is shown too, shadowed
			for i, pair := range panel.Roll.Pairs {
				if !pair.Usable {
					panel.Unusable = append(panel.Unusable, pairView{Pair: i + 1, Sum: pair.Sum, Segments: linkText(glossary, pair.Text)})
				}
			}
		}
		var dice, groups, labels []string
		if panel.Roll.D6 > 0 {
			dice = append(dice, strconv.Itoa(panel.Roll.D6))
		}
		for i, pair := range panel.Roll.Pairs { // each of a hero's rolls gets its own lane
			if len(pair.Dice) == 0 {
				continue
			}
			for _, d := range pair.Dice {
				dice = append(dice, strconv.Itoa(d))
			}
			groups = append(groups, strconv.Itoa(len(pair.Dice)))
			labels = append(labels, rollLabel(i+1))
		}
		panel.Dice, panel.Groups, panel.Labels = strings.Join(dice, ","), strings.Join(groups, ","), strings.Join(labels, ",")
	}
	s.render.partial(w, status, "campaign_my_company", "level_panel", panel)
}

// noChangeCloseMillis is how long a warrior's no change stays on screen after the dice stop.
const noChangeCloseMillis = 2000

// rollLabel names the i-th result of a hero's level-up: "First roll", "Second roll", "Roll 3".
func rollLabel(i int) string {
	switch i {
	case 1:
		return "First roll"
	case 2:
		return "Second roll"
	}
	return "Roll " + strconv.Itoa(i)
}

func (s *Server) handleLevelUpPanel(w http.ResponseWriter, r *http.Request) {
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	s.showLevelPanel(w, r, http.StatusOK, unitID, nil, false, "")
}

// handleLevelUpRoll makes the roll: the player's own dice (a D6 for a warrior, the sums of two pairs for a hero) or the app's.
func (s *Server) handleLevelUpRoll(w http.ResponseWriter, r *http.Request) {
	id, cid, unitID, ok := s.ownUnitParams(w, r)
	if !ok {
		return
	}
	entered := enteredRolls(r)
	own := entered != nil
	thrown := r.PostFormValue("thrown") != "" // the browser threw the dice (Shake the table) and reports the result
	roll, err := s.campaigns.RollLevelUp(r.Context(), *userFrom(r.Context()), id, cid, unitID, entered, thrown)
	switch {
	case errors.Is(err, core.ErrRefused):
		s.showLevelPanel(w, r, http.StatusUnprocessableEntity, unitID, nil, false, reason(err))
	case s.fail(w, r, err):
	default:
		s.showLevelPanel(w, r, http.StatusOK, unitID, &roll, !own, "")
	}
}

func (s *Server) handleLevelUpChoose(w http.ResponseWriter, r *http.Request) {
	id, cid, unitID, ok := s.ownUnitParams(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	roll, err := s.campaigns.ChooseLevelUp(r.Context(), *userFrom(r.Context()), id, cid, unitID, append(r.PostForm["choice"], r.PostForm["also"]...))
	switch {
	case errors.Is(err, core.ErrRefused):
		s.showLevelPanel(w, r, http.StatusUnprocessableEntity, unitID, nil, false, reason(err))
	case s.fail(w, r, err):
	case r.PostFormValue("then") == "close": // a Hero in the Making accepted: the window closes by itself and the page behind refreshes
		s.render.partial(w, http.StatusOK, "campaign_my_company", "level_closing", map[string]any{"Campaign": id, "Co": cid, "Summary": roll.Summary})
	case r.PostFormValue("then") == "path": // ... or goes on to the path window
		base, ok := s.loadOwn(w, r, "my-company")
		if ok {
			s.showPathPanel(w, r, base, unitID, http.StatusOK, "")
		}
	default:
		s.showLevelPanel(w, r, http.StatusOK, unitID, &roll, false, "")
	}
}

// handleLevelUpReroll spends an Against the Odds re-roll on a result.
func (s *Server) handleLevelUpReroll(w http.ResponseWriter, r *http.Request) {
	id, cid, unitID, ok := s.ownUnitParams(w, r)
	if !ok {
		return
	}
	which, _ := strconv.Atoi(r.PostFormValue("which"))
	roll, err := s.campaigns.RerollLevelUp(r.Context(), *userFrom(r.Context()), id, cid, unitID, which)
	switch {
	case errors.Is(err, core.ErrRefused):
		s.showLevelPanel(w, r, http.StatusUnprocessableEntity, unitID, nil, false, reason(err))
	case s.fail(w, r, err):
	default:
		s.showLevelPanel(w, r, http.StatusOK, unitID, &roll, true, "")
	}
}

// offerView is an offer of a level-up roll whose choices carry their descriptions.
type offerView struct {
	core.LevelOffer
	Choices []choiceView
}

// choiceView is a choice with what the glossary says about the rule, action, power or path it gives: the summary cut into
// pieces that link to other terms (and preview them on hover), and the full text for the choice's own tooltip.
type choiceView struct {
	core.LevelChoice
	Name     string   // what the button says: the option's name, a stat with its amount ("Fight +1")
	Badge    string   // the kind of ability it gives
	Dots     []string // a stat's progress against its limit: taken, gain, free
	TipTitle string
	TipText  string
	TipPath  string // a path's limits and chart as JSON for the tooltip: {"limits": [["Fight", "+3 / 9"]], "rows": [[2, "..."]]}
}

func offerViews(g core.Glossary, offers []core.LevelOffer) []offerView {
	out := make([]offerView, len(offers))
	for i, o := range offers {
		out[i] = offerView{LevelOffer: o}
		for _, c := range o.Choices {
			out[i].Choices = append(out[i].Choices, newChoiceView(g, c))
		}
	}
	return out
}

func newChoiceView(g core.Glossary, c core.LevelChoice) choiceView {
	v := choiceView{LevelChoice: c, Name: c.Label, Badge: choiceBadge(c.Key)}
	if st := c.Stat; st != nil {
		v.Name = fmt.Sprintf("%s +%d", st.Stat, st.Gain)
		v.Dots = statDots(*st)
		v.TipTitle, v.TipText = v.Name, st.Detail
		return v
	}
	if name := choiceTerm(c.Key); name != "" {
		if term, ok := g.Find(name); ok {
			v.TipTitle, v.TipText = term.Name, term.Text()
			if strings.HasPrefix(c.Key, "path:") {
				v.TipPath = tipPath(term.Path)
			}
			if _, target, ok := strings.Cut(c.Label, " - "); ok && strings.Contains(c.Key, "|unit:") { // X is the unit picked
				v.TipText = strings.ReplaceAll(v.TipText, " X ", " "+target+" ")
			}
		}
	}
	if v.TipText == "" && c.Hint != "" {
		v.TipTitle, v.TipText = c.Label, c.Hint
	}
	return v
}

// statDots is a stat's progress against its limit, one dot per step of improvement; none when nothing limits the stat.
func statDots(st core.StatGain) []string {
	if st.Limit <= 0 {
		return nil
	}
	dots := make([]string, 0, st.Limit)
	for i := range st.Limit {
		switch {
		case i < st.Taken:
			dots = append(dots, "taken")
		case i < st.Taken+st.Gain:
			dots = append(dots, "gain")
		default:
			dots = append(dots, "free")
		}
	}
	return dots
}

// choiceBadge names the kind of ability a choice gives; empty for anything else.
func choiceBadge(key string) string {
	kind, _, _ := strings.Cut(key, ":")
	switch kind {
	case "rule":
		return "Special rule"
	case "minor":
		return "Minor rule"
	case "action":
		return "Heroic action"
	case "power":
		return "Magical power"
	}
	return ""
}

// choiceTerm is the glossary name of what a choice gives (a rule, a minor rule, a heroic action, a power, a path); empty for
// choices that are only numbers.
func choiceTerm(key string) string {
	kind, name, _ := strings.Cut(key, ":")
	name, _, _ = strings.Cut(name, "|") // a rule with a target is described by the rule
	switch kind {
	case "rule", "minor", "action", "power", "cast", "path":
		return name
	}
	return ""
}

// pairView is one of a hero's two rolls that it cannot take anything from.
type pairView struct {
	Pair     int // 1 or 2
	Sum      int
	Segments []segView
}

// tipPath is a path's tables as JSON for the tooltip; empty for anything else.
func tipPath(p *core.PathTable) string {
	if p == nil {
		return ""
	}
	type row struct {
		Roll int    `json:"roll"`
		Text string `json:"text"`
	}
	out := struct {
		Limits [][3]string `json:"limits"`
		Rows   []row       `json:"rows"`
		Dice   string      `json:"dice"`
	}{}
	if p.PathDice.Valid() {
		out.Dice = p.PathDice.Display()
	}
	for _, l := range p.Limits {
		out.Limits = append(out.Limits, [3]string{l.Stat, l.Limit, l.Abbr})
	}
	for _, r := range p.Rows {
		var text strings.Builder
		for _, seg := range r.Segments {
			text.WriteString(seg.Text)
		}
		out.Rows = append(out.Rows, row{Roll: r.Roll, Text: text.String()})
	}
	b, _ := json.Marshal(out)
	return string(b)
}
