package web

import (
	"net/http"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The To Wound chart (docs/matches.md, "To Wound chart"): a window on the match page and the live company page, filled from
// GET /campaigns/{id}/to-wound. The chart is the campaign glossary file's reference.toWound (core.Glossary.ToWound); the answer is
// static HTML, so it is fetched once when the page loads and nothing the live stream refetches replaces it.

// twCell is one cell of the chart as the template shows it.
type twCell struct {
	S, D    int
	Roll    string // "4+", "6+/4+", "-": the chart's text
	Show    string // what the narrow cell prints: "4+", "6/4", "-"
	Big     string // the find result: "4+", "6+ then 4+", "Cannot wound"
	Tier    string // 3, 4, 5, 6, split or none: the colour band
	Meaning string // the chart's notation text for this cell
	Label   string // the accessible name: "Strength 3 against Defence 4: 4+, roll one dice; ..."
}

type twRow struct {
	S     int
	Cells []twCell
}

type toWoundView struct {
	CampaignID int64
	Chart      *core.ToWoundChart
	Rows       []twRow
	Min, Max   struct{ S, D int }
	Legend     []twCell // one example cell per band, for the key
}

// toWoundCell words a cell of the chart. notation is the file's own text for the cell forms ("N+", "6+/N+", "-").
func toWoundCell(c core.ToWoundChart, s, d int, roll string) twCell {
	cell := twCell{S: s, D: d, Roll: roll}
	switch {
	case roll == "-":
		cell.Show, cell.Big, cell.Tier = "-", "Cannot wound", "none"
		cell.Meaning = c.Notation["-"]
	case strings.HasPrefix(roll, "6+/"):
		n := strings.TrimSuffix(strings.TrimPrefix(roll, "6+/"), "+")
		cell.Show, cell.Big, cell.Tier = "6/"+n, "6+ then "+n+"+", "split"
		cell.Meaning = strings.ReplaceAll(c.Notation["6+/N+"], "N", n)
	default:
		n := strings.TrimSuffix(roll, "+")
		cell.Show, cell.Big, cell.Tier = roll, roll, n
		cell.Meaning = strings.ReplaceAll(c.Notation["N+"], "N", n)
	}
	cell.Meaning = capitalise(cell.Meaning)
	cell.Label = "Strength " + itoa(s) + " against Defence " + itoa(d) + ": " + cell.Big
	if cell.Meaning != "" {
		cell.Label += ". " + cell.Meaning
	}
	return cell
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func newToWoundView(id int64, c *core.ToWoundChart) toWoundView {
	v := toWoundView{CampaignID: id, Chart: c}
	if c == nil {
		return v
	}
	for si, s := range c.Strengths {
		row := twRow{S: s}
		for di, d := range c.Defences {
			row.Cells = append(row.Cells, toWoundCell(*c, s, d, c.Cells[si][di]))
		}
		v.Rows = append(v.Rows, row)
	}
	v.Min.S, v.Max.S = c.Strengths[0], c.Strengths[len(c.Strengths)-1]
	v.Min.D, v.Max.D = c.Defences[0], c.Defences[len(c.Defences)-1]
	for _, tier := range []string{"3", "4", "5", "6", "split", "none"} { // the key: the first cell of each band, easiest first
		if i := slices.IndexFunc(flatten(v.Rows), func(c twCell) bool { return c.Tier == tier }); i >= 0 {
			v.Legend = append(v.Legend, flatten(v.Rows)[i])
		}
	}
	return v
}

// handleToWound answers the window's body. A campaign whose rules file has no chart gets a short explanation instead of a 404.
func (s *Server) handleToWound(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	g, err := s.glossary.ForCampaign(r.Context(), base.Campaign.ID)
	if s.fail(w, r, err) {
		return
	}
	s.render.partial(w, http.StatusOK, "match", "to_wound_body", newToWoundView(base.Campaign.ID, g.ToWound))
}

func flatten(rows []twRow) []twCell {
	var out []twCell
	for _, r := range rows {
		out = append(out, r.Cells...)
	}
	return out
}
