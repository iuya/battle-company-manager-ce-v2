package web

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The administration section (docs/admin.md): superusers only, a 404 for everybody else (requireSuperuser), two pages
// under one sub-navigation. Every action is an htmx form that swaps its own row or card.

type adminBase struct {
	pageBase
	Section string // "users" or "rulesets": the current tab
}

func (s *Server) adminBase(r *http.Request, section string) adminBase {
	return adminBase{pageBase: s.base(r), Section: section}
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

// ---- Users ----

const userPageSize = 25

type adminUsersView struct {
	adminBase
	Q    string
	List adminUserList
}

// adminUserList is a page of rows and the button for the next one.
type adminUserList struct {
	Q     string
	Rows  []adminUserRow
	Total int
	Next  int // offset of the next page; 0 when there is none
}

// NextURL is the "Show more" request.
func (l adminUserList) NextURL() string {
	return "/admin/users?q=" + url.QueryEscape(l.Q) + "&offset=" + strconv.Itoa(l.Next)
}

// adminUserRow is one account: the figures, the permission form (Role and CanCreate are what it shows, which after a
// refusal are what was submitted) and the notice or error of the last save.
type adminUserRow struct {
	U         core.AdminUser
	Self      bool
	Role      core.Role
	CanCreate bool
	Open      bool // the form is expanded
	Notice    string
	Err       string
	// Candidates are, per campaign the user organises, the players it could be handed to (the owners of its companies).
	Candidates    map[int64][]handoverCandidate
	HandoverOpen  bool
	HandoverMsg   string
	HandoverError bool
}

// withCandidates loads the possible new organisers of the campaigns the row's user organises.
func (s *Server) withCandidates(r *http.Request, row adminUserRow) (adminUserRow, error) {
	for _, c := range row.U.Organises {
		list, err := s.handoverCandidates(r, c.ID, row.U.ID)
		if err != nil {
			return row, err
		}
		if row.Candidates == nil {
			row.Candidates = map[int64][]handoverCandidate{}
		}
		row.Candidates[c.ID] = list
	}
	return row, nil
}

func newUserRow(u core.AdminUser, self int64) adminUserRow {
	return adminUserRow{U: u, Self: u.ID == self, Role: u.Role, CanCreate: u.CanCreateCampaigns || u.Role != core.RoleUser}
}

func (s *Server) userList(r *http.Request, q string, offset int) (adminUserList, bool, error) {
	me := *userFrom(r.Context())
	users, total, err := s.auth.ListUsers(r.Context(), me, q, userPageSize, offset)
	if err != nil {
		return adminUserList{}, false, err
	}
	list := adminUserList{Q: q, Total: total}
	for _, u := range users {
		row, err := s.withCandidates(r, newUserRow(u, me.ID))
		if err != nil {
			return adminUserList{}, false, err
		}
		list.Rows = append(list.Rows, row)
	}
	if next := offset + len(users); len(users) > 0 && next < total {
		list.Next = next
	}
	return list, true, nil
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	list, _, err := s.userList(r, q, max(offset, 0))
	if s.fail(w, r, err) {
		return
	}
	// The search box and "Show more" swap the list alone; everything else is the whole page.
	if r.Header.Get("HX-Target") == "user-rows" || r.Header.Get("HX-Target") == "user-more" {
		w.Header().Add("Vary", "HX-Target")
		name := "admin_user_rows"
		if offset > 0 {
			name = "admin_user_page"
		}
		s.render.partial(w, http.StatusOK, "admin_users", name, list)
		return
	}
	s.render.page(w, r, http.StatusOK, "admin_users", adminUsersView{adminBase: s.adminBase(r, "users"), Q: q, List: list})
}

func (s *Server) handleAdminPermissions(w http.ResponseWriter, r *http.Request) {
	me := *userFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w, r)
		return
	}
	role := core.Role(r.PostFormValue("role"))
	can := r.PostFormValue("can_create") == "on" && role == core.RoleUser
	// The row to draw is looked up by the email the form carries (display only); an unknown id is a 404.
	find := func() (core.AdminUser, bool, error) {
		rows, _, err := s.auth.ListUsers(r.Context(), me, r.PostFormValue("email"), 200, 0)
		if err != nil {
			return core.AdminUser{}, false, err
		}
		for _, u := range rows {
			if u.ID == id {
				return u, true, nil
			}
		}
		return core.AdminUser{}, false, nil
	}
	err = s.auth.SetUserPermissions(r.Context(), me, id, role, can)
	u, ok, ferr := find()
	if ferr != nil {
		s.fail(w, r, ferr)
		return
	}
	if !ok {
		s.notFound(w, r)
		return
	}
	row, rerr := s.withCandidates(r, newUserRow(u, me.ID))
	if s.fail(w, r, rerr) {
		return
	}
	switch {
	case errors.Is(err, core.ErrRefused), errors.Is(err, core.ErrInvalidUser):
		row.Role, row.CanCreate, row.Open, row.Err = role, can || role != core.RoleUser, true, reason(err)
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_users", "admin_user_row", row)
		return
	case s.fail(w, r, err):
		return
	}
	row.Notice = "Saved"
	s.render.partial(w, http.StatusOK, "admin_users", "admin_user_row", row)
}

// ---- Rules files ----

type adminRulesetsView struct {
	adminBase
	Panel adminRulesetPanel
}

// adminRulesetPanel is everything below the sub-navigation: the upload card and the list; it is swapped as a whole after
// any action, because making a default or deleting a file changes other cards too.
type adminRulesetPanel struct {
	Groups    []rulesetGroup
	Unused    int // files "Delete unused" would remove
	Highlight string
	Notice    string
	Err       string
	UploadErr string
	Report    *reportView // every problem of the rejected upload
	Kind      string      // the kind chosen in the upload form
}

type rulesetGroup struct {
	Kind  core.RulesetKind
	Label string
	Files []rulesetCard
}

// rulesetCard is a stored file as the page draws it.
type rulesetCard struct{ core.RulesetUsage }

// KB is the size in kilobytes, at least 1.
func (c rulesetCard) KB() int { return max(1, (c.Size+512)/1024) }

// Deletable says whether the file can go: no campaign uses it and it is not a default.
func (c rulesetCard) Deletable() bool { return len(c.Campaigns) == 0 && !c.Default }

func (s *Server) rulesetPanel(r *http.Request) (adminRulesetPanel, error) {
	files, err := s.rulesets.ListWithUsage(r.Context(), *userFrom(r.Context()))
	if err != nil {
		return adminRulesetPanel{}, err
	}
	p := adminRulesetPanel{Kind: string(core.KindBattleCompanies)}
	for _, k := range []core.RulesetKind{core.KindVanilla, core.KindBattleCompanies, core.KindGlossary} {
		g := rulesetGroup{Kind: k, Label: k.Label()}
		for _, f := range files {
			if f.Kind == k {
				g.Files = append(g.Files, rulesetCard{f})
				if (rulesetCard{f}).Deletable() {
					p.Unused++
				}
			}
		}
		p.Groups = append(p.Groups, g)
	}
	return p, nil
}

func (s *Server) handleAdminRulesets(w http.ResponseWriter, r *http.Request) {
	p, err := s.rulesetPanel(r)
	if s.fail(w, r, err) {
		return
	}
	s.render.page(w, r, http.StatusOK, "admin_rulesets", adminRulesetsView{adminBase: s.adminBase(r, "rulesets"), Panel: p})
}

// panelAnswer redraws the panel with the outcome of an action: a notice, or the reason it was refused (422).
func (s *Server) panelAnswer(w http.ResponseWriter, r *http.Request, status int, mutate func(*adminRulesetPanel)) {
	p, err := s.rulesetPanel(r)
	if s.fail(w, r, err) {
		return
	}
	mutate(&p)
	s.render.partial(w, status, "admin_rulesets", "admin_ruleset_panel", p)
}

func (s *Server) handleAdminRulesetDefault(w http.ResponseWriter, r *http.Request) {
	err := s.rulesets.SetDefault(r.Context(), *userFrom(r.Context()), r.PathValue("hash"))
	if errors.Is(err, core.ErrRefused) {
		s.panelAnswer(w, r, http.StatusUnprocessableEntity, func(p *adminRulesetPanel) { p.Err = reason(err) })
		return
	}
	if s.fail(w, r, err) {
		return
	}
	s.panelAnswer(w, r, http.StatusOK, func(p *adminRulesetPanel) {
		p.Notice, p.Highlight = "New campaigns will use this file. Existing campaigns keep theirs.", r.PathValue("hash")
	})
}

func (s *Server) handleAdminRulesetDelete(w http.ResponseWriter, r *http.Request) {
	err := s.rulesets.Delete(r.Context(), *userFrom(r.Context()), r.PathValue("hash"))
	if errors.Is(err, core.ErrRefused) {
		s.panelAnswer(w, r, http.StatusUnprocessableEntity, func(p *adminRulesetPanel) { p.Err = reason(err) })
		return
	}
	if s.fail(w, r, err) {
		return
	}
	s.panelAnswer(w, r, http.StatusOK, func(p *adminRulesetPanel) { p.Notice = "The file is deleted." })
}

func (s *Server) handleAdminRulesetPrune(w http.ResponseWriter, r *http.Request) {
	n, err := s.rulesets.PruneUnused(r.Context(), *userFrom(r.Context()))
	if errors.Is(err, core.ErrRefused) {
		s.panelAnswer(w, r, http.StatusUnprocessableEntity, func(p *adminRulesetPanel) { p.Err = reason(err) })
		return
	}
	if s.fail(w, r, err) {
		return
	}
	s.panelAnswer(w, r, http.StatusOK, func(p *adminRulesetPanel) {
		switch n {
		case 0:
			p.Notice = "There was nothing to delete."
		case 1:
			p.Notice = "Deleted 1 unused file."
		default:
			p.Notice = fmt.Sprintf("Deleted %d unused files.", n)
		}
	})
}

func (s *Server) handleAdminRulesetDownload(w http.ResponseWriter, r *http.Request) {
	info, content, err := s.rulesets.Download(r.Context(), *userFrom(r.Context()), r.PathValue("hash"))
	if s.fail(w, r, err) {
		return
	}
	name := info.Name
	if info.Version != "" {
		name += "-" + info.Version
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".json"}))
	w.Header().Set("Cache-Control", "private, no-cache")
	_, _ = w.Write(content)
}

// handleAdminRulesetUpload stores a file chosen on the page. The server hashes it itself (the campaign form's picker
// hashes in the browser only to skip uploading a file the server has); the outcome is stored, already stored or a reason.
func (s *Server) handleAdminRulesetUpload(w http.ResponseWriter, r *http.Request) {
	me := *userFrom(r.Context())
	var kind core.RulesetKind
	reject := func(status int, msg string, rep *reportView) {
		s.panelAnswer(w, r, status, func(p *adminRulesetPanel) {
			p.UploadErr, p.Report = msg, rep
			if kind.Valid() {
				p.Kind = string(kind)
			}
		})
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		reject(http.StatusUnprocessableEntity, "Choose a .json file of at most 16 MB.", nil)
		return
	}
	kind = core.RulesetKind(r.PostFormValue("kind"))
	if !kind.Valid() {
		reject(http.StatusUnprocessableEntity, "Choose the kind of file.", nil)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		reject(http.StatusUnprocessableEntity, "Choose a .json file.", nil)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	hash, err := core.RulesetHash(content)
	if err == nil {
		var name, version string
		if name, version, err = core.RulesetMeta(kind, content); err == nil {
			var info core.RulesetInfo
			switch info, err = s.rulesets.Find(r.Context(), kind, hash, name, version); {
			case err == nil:
				s.panelAnswer(w, r, http.StatusOK, func(p *adminRulesetPanel) {
					p.Notice, p.Highlight, p.Kind = "That file is already stored (highlighted below).", info.Hash, string(kind)
				})
				return
			case errors.Is(err, core.ErrNotFound):
				if info, err = s.rulesets.StoreAs(r.Context(), me, kind, hash, content); err == nil {
					s.panelAnswer(w, r, http.StatusOK, func(p *adminRulesetPanel) {
						p.Notice, p.Highlight, p.Kind = "Stored "+info.Name+" "+info.Version+".", info.Hash, string(kind)
					})
					return
				}
			}
		}
	}
	switch {
	case errors.Is(err, core.ErrInvalidRuleset):
		reject(http.StatusUnprocessableEntity, reason(err), rulesetIssues(err))
	default:
		s.fail(w, r, err)
	}
}

// handleAdminHandover hands a campaign the user organises over to another player; the row is drawn again with the outcome.
func (s *Server) handleAdminHandover(w http.ResponseWriter, r *http.Request) {
	me := *userFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w, r)
		return
	}
	campaign, _ := strconv.ParseInt(r.PostFormValue("campaign"), 10, 64)
	to, terr := strconv.ParseInt(r.PostFormValue("organiser"), 10, 64)
	if terr != nil {
		err = core.Refuse("Choose the player who takes over")
	} else {
		err = s.campaigns.HandOver(r.Context(), me, campaign, to)
	}
	if err != nil && !errors.Is(err, core.ErrRefused) {
		s.fail(w, r, err)
		return
	}
	rows, _, lerr := s.auth.ListUsers(r.Context(), me, r.PostFormValue("email"), 200, 0)
	if s.fail(w, r, lerr) {
		return
	}
	for _, u := range rows {
		if u.ID != id {
			continue
		}
		row, rerr := s.withCandidates(r, newUserRow(u, me.ID))
		if s.fail(w, r, rerr) {
			return
		}
		row.HandoverOpen = true
		status := http.StatusOK
		if err != nil {
			status, row.HandoverMsg, row.HandoverError = http.StatusUnprocessableEntity, reason(err), true
		} else {
			row.HandoverMsg = "Handed over."
		}
		s.render.partial(w, status, "admin_users", "admin_user_row", row)
		return
	}
	s.notFound(w, r)
}
