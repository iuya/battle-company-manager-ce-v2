package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// levelPanel is the level-up window of a unit: the roll form, then the results to choose from, then what happened.
type levelPanel struct {
	Campaign core.CampaignDetail
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
}

func (s *Server) showLevelPanel(w http.ResponseWriter, r *http.Request, status int, unitID int64, roll *core.LevelRoll, animate bool, errMsg string) {
	base, ok := s.loadCampaign(w, r, "my-company")
	if !ok {
		return
	}
	if base.Campaign.Company == nil {
		s.notFound(w, r)
		return
	}
	detail, err := s.campaigns.CompanyDetail(r.Context(), base.Campaign.ID, base.Campaign.Company.ID)
	if s.fail(w, r, err) {
		return
	}
	panel := levelPanel{Campaign: base.Campaign, Roll: roll, Animate: animate, Error: errMsg}
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
	glossary, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID) // to describe a rule, an action or a power with links to its terms
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	if roll == nil {
		if roll, err = s.campaigns.LevelUpState(r.Context(), *userFrom(r.Context()), base.Campaign.ID, unitID); s.fail(w, r, err) {
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
		names := []string{"First roll", "Second roll"}
		for i, pair := range panel.Roll.Pairs { // each of a hero's rolls gets its own lane
			if len(pair.Dice) == 0 {
				continue
			}
			for _, d := range pair.Dice {
				dice = append(dice, strconv.Itoa(d))
			}
			groups = append(groups, strconv.Itoa(len(pair.Dice)))
			labels = append(labels, names[i])
		}
		panel.Dice, panel.Groups, panel.Labels = strings.Join(dice, ","), strings.Join(groups, ","), strings.Join(labels, ",")
	}
	s.render.partial(w, status, "campaign_my_company", "level_panel", panel)
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
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	var entered []int
	own := false
	for _, f := range []string{"roll", "roll2"} {
		n, _ := strconv.Atoi(r.PostFormValue(f))
		entered = append(entered, n)
		own = own || n > 0
	}
	thrown := r.PostFormValue("thrown") != "" // the browser threw the dice (Shake the table) and reports the result
	roll, err := s.campaigns.RollLevelUp(r.Context(), *userFrom(r.Context()), id, unitID, entered, thrown)
	switch {
	case errors.Is(err, core.ErrRefused):
		s.showLevelPanel(w, r, http.StatusUnprocessableEntity, unitID, nil, false, reason(err))
	case s.fail(w, r, err):
	default:
		s.showLevelPanel(w, r, http.StatusOK, unitID, &roll, !own, "")
	}
}

func (s *Server) handleLevelUpChoose(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	_ = r.ParseForm()
	roll, err := s.campaigns.ChooseLevelUp(r.Context(), *userFrom(r.Context()), id, unitID, append(r.PostForm["choice"], r.PostForm["also"]...))
	switch {
	case errors.Is(err, core.ErrRefused):
		s.showLevelPanel(w, r, http.StatusUnprocessableEntity, unitID, nil, false, reason(err))
	case s.fail(w, r, err):
	default:
		s.showLevelPanel(w, r, http.StatusOK, unitID, &roll, false, "")
	}
}

// handleLevelUpReroll spends an Against the Odds re-roll on a result.
func (s *Server) handleLevelUpReroll(w http.ResponseWriter, r *http.Request) {
	id, ok := s.idParam(w, r, "id")
	if !ok {
		return
	}
	unitID, ok := s.idParam(w, r, "unit")
	if !ok {
		return
	}
	which, _ := strconv.Atoi(r.PostFormValue("which"))
	roll, err := s.campaigns.RerollLevelUp(r.Context(), *userFrom(r.Context()), id, unitID, which)
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
	Segments []segView
	TipTitle string
	TipText  string
	TipPath  string // a path's limits and chart as JSON for the tooltip: {"limits": [["Fight", "+3 / 9"]], "rows": [[2, "..."]]}
}

func offerViews(g core.Glossary, offers []core.LevelOffer) []offerView {
	out := make([]offerView, len(offers))
	for i, o := range offers {
		out[i] = offerView{LevelOffer: o}
		for _, c := range o.Choices {
			v := choiceView{LevelChoice: c}
			if name := choiceTerm(c.Key); name != "" {
				if term, ok := g.Find(name); ok {
					v.TipTitle = term.Name
					if strings.HasPrefix(c.Key, "path:") { // a path is chosen by its name; the tooltip has its whole chart
						v.TipText = term.Text()
						v.TipPath = tipPath(term.Path)
					} else {
						v.TipText = term.Summary(700)
						text := term.Summary(240)
						if _, target, ok := strings.Cut(c.Label, " - "); ok && strings.Contains(c.Key, "|unit:") { // X is the unit picked
							text = strings.ReplaceAll(text, " X ", " "+target+" ")
						}
						v.Segments = linkText(g, text)
					}
				}
			}
			if v.TipText == "" && c.Hint != "" {
				v.TipTitle, v.TipText = c.Label, c.Hint
				v.Segments = linkText(g, c.Hint)
			}
			out[i].Choices = append(out[i].Choices, v)
		}
	}
	return out
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
	}{}
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
