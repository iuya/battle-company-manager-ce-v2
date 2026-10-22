package web

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// companyInfoView is a company's page in the browser shown when a player creates theirs.
type companyInfoView struct {
	Campaign core.CampaignDetail
	Profile  core.CompanyProfile
	Tips     map[string]string // what a rule, an item or a upgrade says, by name, for the tooltips
	Name     string            // the name typed in the creation form, kept when it is shown again
	Start    []startRow        // the starting models, with the role chosen (or suggested) for each
	Notes    core.PageNotes    // the page's notes, each sentence once
	Error    string
	glossary core.Glossary
}

// Linked cuts a text into pieces, linking the glossary terms it mentions.
func (v companyInfoView) Linked(text string) []segView {
	var out []segView
	for _, seg := range v.glossary.Link(text) {
		piece := segView{Text: seg.Text, TermID: seg.TermID}
		if term, ok := v.glossary.Term(seg.TermID); ok {
			piece.TipTitle, piece.TipText = term.Name, term.Summary(tipLength)
		}
		out = append(out, piece)
	}
	return out
}

// tipLength is how much of a term a hover preview shows: enough for a rule, not for a chapter.
const tipLength = 900

// Named links a whole name (a rule, a path, an item) to its glossary term when it has one.
func (v companyInfoView) Named(name string) []segView {
	if term, ok := v.glossary.Find(name); ok {
		return []segView{{Text: name, TermID: term.ID, TipTitle: term.Name, TipText: term.Summary(tipLength)}}
	}
	return []segView{{Text: name}}
}

// startRow is a model of the starting company in the creation form: who may lead, and the role chosen (or suggested) for it.
type startRow struct {
	Index     int
	Unit      string
	Wargear   []string
	Eligible  bool
	Role      core.UnitRole // warrior, leader or sergeant
	Suggested bool          // the role is the company's own suggestion, not the player's choice
	Locked    bool          // the company's fixed Leader (the Shire's Ranger of the North): it starts as Leader and cannot be moved
}

// Leader, Sergeants and Pool split the starting models by the place they start in on the page.
func (v companyInfoView) Leader() []startRow { return v.with(core.RoleLeader) }
func (v companyInfoView) Pool() []startRow   { return v.with(core.RoleWarrior) }
func (v companyInfoView) Sergeant(n int) []startRow {
	if s := v.with(core.RoleSergeant); n < len(s) {
		return s[n : n+1]
	}
	return nil
}

func (v companyInfoView) with(role core.UnitRole) []startRow {
	var out []startRow
	for _, r := range v.Start {
		if r.Role == role {
			out = append(out, r)
		}
	}
	return out
}

// startRows lists the starting models with the roles chosen; with nothing chosen yet they carry the company's suggestion.
func startRows(p core.CompanyProfile, chosen []core.StartHero) []startRow {
	suggested := len(chosen) == 0
	if suggested {
		chosen = p.DefaultHeroes()
	}
	var rows []startRow
	for i, u := range p.StartList() {
		row := startRow{Index: i, Unit: u.Unit, Wargear: u.Wargear, Eligible: p.CanLead(u.Unit), Role: core.RoleWarrior}
		row.Locked = u.Role == core.RoleLeader && p.RangerReplacement != nil && strings.EqualFold(u.Unit, p.RangerReplacement.Unit)
		for _, h := range chosen {
			if h.Index == i {
				row.Role, row.Suggested = h.Role, suggested
			}
		}
		if row.Locked { // whatever was posted, the page shows him in the Leader place
			row.Role, row.Suggested = core.RoleLeader, suggested
		}
		rows = append(rows, row)
	}
	if slices.ContainsFunc(rows, func(r startRow) bool { return r.Locked }) { // one Leader place: nobody else holds it
		for i := range rows {
			if !rows[i].Locked && rows[i].Role == core.RoleLeader {
				rows[i].Role = core.RoleWarrior
			}
		}
	}
	return rows
}

// startHeroes reads the leader and the sergeants chosen in the creation form (fields leader and sergeant: the numbers of the
// starting models).
func startHeroes(r *http.Request) []core.StartHero {
	var out []core.StartHero
	if i, err := strconv.Atoi(r.PostFormValue("leader")); err == nil {
		out = append(out, core.StartHero{Index: i, Role: core.RoleLeader})
	}
	for _, v := range r.PostForm["sergeant"] {
		if i, err := strconv.Atoi(v); err == nil {
			out = append(out, core.StartHero{Index: i, Role: core.RoleSergeant})
		}
	}
	slices.SortFunc(out, func(a, b core.StartHero) int { return a.Index - b.Index })
	return out
}

// companyInfo builds the page of one company for a campaign.
func (s *Server) companyInfo(r *http.Request, c core.CampaignDetail, profile core.CompanyProfile, name string, chosen []core.StartHero, errMsg string) (companyInfoView, error) {
	view := companyInfoView{Campaign: c, Profile: profile, Name: name, Start: startRows(profile, chosen), Notes: profile.PageNotes(), Error: errMsg, Tips: map[string]string{}}
	g, err := s.glossary.ForCampaign(r.Context(), c.ID) // rules are explained by the campaign's glossary
	if err != nil && err != core.ErrNotFound {
		return view, err
	}
	view.glossary = g
	tip := func(rule string) {
		if rule == "" || view.Tips[rule] != "" {
			return
		}
		if term, ok := g.Find(rule); ok {
			view.Tips[rule] = term.Summary(520)
		}
	}
	for _, h := range profile.HeroUpgrades {
		tip(h.Rule)
		tip(h.Name)
		if h.Text != "" {
			view.Tips[h.Name] = h.Text
		}
	}
	for _, gr := range profile.Granted {
		tip(gr.Rule)
	}
	for _, a := range profile.Aliases {
		for _, rule := range a.AddRules {
			tip(rule)
		}
	}
	for _, ex := range profile.Exchanges {
		tip(ex.Add)
		tip(ex.Remove)
	}
	for _, w := range profile.UniqueWargear {
		view.Tips[w.Name] = w.Description
	}
	return view, nil
}

// handleCompanyInfo answers the company page of the browser (a fragment, loaded when a company is picked from the list).
func (s *Server) handleCompanyInfo(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	profiles, err := s.campaigns.CompanyProfiles(r.Context(), base.Campaign.ID)
	if s.fail(w, r, err) {
		return
	}
	want := strings.TrimSpace(r.URL.Query().Get("c"))
	for _, p := range profiles {
		if p.Name == want {
			view, err := s.companyInfo(r, base.Campaign, p, r.URL.Query().Get("name"), nil, "")
			if s.fail(w, r, err) {
				return
			}
			s.render.partial(w, http.StatusOK, "company_new", "company_info", view)
			return
		}
	}
	s.notFound(w, r)
}
