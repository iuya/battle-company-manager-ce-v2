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
	i := slices.IndexFunc(detail.Roster, func(u core.CompanyUnit) bool { return u.ID == unitID })
	if i < 0 {
		s.notFound(w, r)
		return
	}
	unit := detail.Roster[i]
	panel := levelPanel{Campaign: base.Campaign, Co: base.Current.ID, Unit: unit, Roll: roll, Animate: animate, Error: errMsg, Fresh: roll != nil,
		Pending: unit.PendingLevelUps(), Rerolls: detail.Rerolls}
	panel.Spec, panel.Pairs = levelDice(unit, detail.RuleDice.Level)
	glossary, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID) // to describe a rule, an action or a power with links to its terms
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if roll == nil {
		if panel.Roll, err = s.campaigns.LevelUpState(r.Context(), *userFrom(r.Context()), base.Campaign.ID, base.Current.ID, unitID); s.fail(w, r, err) {
			return
		}
	}
	if panel.Roll != nil && s.fail(w, r, panel.fillRoll(glossary)) {
		return
	}
	s.render.partial(w, status, "campaign_my_company", "level_panel", panel)
}

// levelDice is the dice of a unit's level-up roll and how many results it rolls: a warrior's D6, or a hero's roll on its path's chart.
func levelDice(u core.CompanyUnit, dice core.LevelDice) (core.DiceSpec, int) {
	if !u.IsHero() {
		return dice.Warrior, 1
	}
	if u.Path != nil {
		return dice.HeroFor(u.Path.Name), dice.Pairs
	}
	return dice.Hero, dice.Pairs
}

// fillRoll sets what the window shows of its roll.
func (p *levelPanel) fillRoll(g core.Glossary) error {
	roll := p.Roll
	p.Offers = offerViews(g, roll.Offers)
	if roll.Upgrade != nil {
		up := offerViews(g, []core.LevelOffer{*roll.Upgrade})[0]
		p.Upgrade = &up
	}
	if roll.Plan != nil {
		plan, err := json.Marshal(roll.Plan)
		if err != nil {
			return err
		}
		p.Plan = string(plan)
	}
	if roll.Done && roll.Result == core.WarriorResultNoChange && p.Pending == 0 {
		p.CloseAfter = noChangeCloseMillis
	}
	if !roll.Done && roll.Stage == "" && roll.Result == core.WarriorResultHeroInTheMaking && len(p.Offers) == 1 {
		p.Heroic = true
		for _, c := range p.Offers[0].Choices {
			if c.Key != core.HeroChoiceKey {
				p.Forgo = append(p.Forgo, c)
			}
		}
	}
	p.Unusable = unusablePairs(g, *roll)
	p.Dice, p.Groups, p.Labels = rollDice(*roll)
	return nil
}

// unusablePairs is the results of a hero's rolls it cannot take anything from: shown too, shadowed, while there is something to choose.
func unusablePairs(g core.Glossary, roll core.LevelRoll) []pairView {
	if len(roll.Offers) == 0 {
		return nil
	}
	var out []pairView
	for i, pair := range roll.Pairs {
		if !pair.Usable {
			out = append(out, pairView{Pair: i + 1, Sum: pair.Sum, Segments: linkText(g, pair.Text)})
		}
	}
	return out
}

// rollDice is the dice to show rolling, how they are grouped in lanes and the lanes' names, comma separated: each of a hero's rolls
// gets its own lane.
func rollDice(roll core.LevelRoll) (dice, groups, labels string) {
	var d, g, l []string
	if roll.D6 > 0 {
		d = append(d, strconv.Itoa(roll.D6))
	}
	for i, pair := range roll.Pairs {
		if len(pair.Dice) == 0 {
			continue
		}
		for _, n := range pair.Dice {
			d = append(d, strconv.Itoa(n))
		}
		g = append(g, strconv.Itoa(len(pair.Dice)))
		l = append(l, rollLabel(i+1))
	}
	return strings.Join(d, ","), strings.Join(g, ","), strings.Join(l, ",")
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
