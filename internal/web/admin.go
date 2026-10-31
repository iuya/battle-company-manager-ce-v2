package web

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The administration section (docs/admin.md): Users and Invitations for administrators and superusers (requireAdmin), Rules files
// and the recovery links for superusers only (requireSuperuser); a 404 for everybody else, under one sub-navigation. Every action
// is an htmx form that swaps its own row or card.

type adminBase struct {
	pageBase
	Section string // "users", "invitations" or "rulesets": the current tab
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
	Viewer    core.User // who looks at the page: decides which controls the row offers
	Self      bool
	Nonce     string // differs every time the row is drawn: a double click repeats it (answered once), a deliberate second request does not
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
	// Recovery link card (superusers): the last request's outcome, and the one-time message of a link just made.
	RecoveryOpen bool
	RecoveryMsg  string
	RecoveryErr  bool
	Link         *shownLink
}

// CanOrganiser says the Organiser switch is the viewer's to use on this account: administrators and superusers, on plain
// users and organisers (the service decides again).
func (r adminUserRow) CanOrganiser() bool {
	return !r.U.Closed && r.Viewer.MayAdminister() && (r.U.Role == core.RoleUser || r.CanAdministrator())
}

// CanAdministrator says the Administrator switch is the viewer's to use: superusers, on plain users, organisers and administrators.
func (r adminUserRow) CanAdministrator() bool {
	return !r.U.Closed && r.Viewer.IsSuperuser() && (r.U.Role == core.RoleUser || r.U.Role == core.RoleAdmin)
}

// HasForm is whether the row offers the permission form at all.
func (r adminUserRow) HasForm() bool { return r.CanOrganiser() || r.CanAdministrator() }

// PermissionNote is the muted line of an open account the viewer cannot change.
func (r adminUserRow) PermissionNote() string {
	switch {
	case r.U.Closed || r.HasForm():
		return ""
	case r.U.Role == core.RoleSuperuser && r.Viewer.IsSuperuser():
		return "Set from the console (make grant)."
	case r.U.Role == core.RoleSuperuser:
		return "Superusers are set from the console."
	default:
		return "Administrators are managed by superusers."
	}
}

// CanRecover says the row offers recovery links: superusers, for any other open account.
func (r adminUserRow) CanRecover() bool { return !r.U.Closed && r.Viewer.IsSuperuser() && !r.Self }

// RecoveryPending is whether the account has a live link.
func (r adminUserRow) RecoveryPending() bool {
	return r.U.Recovery != nil && r.U.Recovery.State == core.RecoveryPending
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

func newUserRow(u core.AdminUser, viewer core.User) adminUserRow {
	return adminUserRow{U: u, Viewer: viewer, Nonce: nonce(), Self: u.ID == viewer.ID, Role: u.Role, CanCreate: u.CanCreateCampaigns}
}

func (s *Server) userList(r *http.Request, q string, offset int) (adminUserList, bool, error) {
	me := *userFrom(r.Context())
	users, total, err := s.auth.ListUsers(r.Context(), me, q, userPageSize, offset)
	if err != nil {
		return adminUserList{}, false, err
	}
	list := adminUserList{Q: q, Total: total}
	for _, u := range users {
		row, err := s.withCandidates(r, newUserRow(u, me))
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

// userRowFor looks the account up for a row to draw again after an action: the email the form carries (display only) narrows the
// list, an unknown id is a 404 (ok false, the answer is written).
func (s *Server) userRowFor(w http.ResponseWriter, r *http.Request, me core.User, id int64) (adminUserRow, bool) {
	rows, _, err := s.auth.ListUsers(r.Context(), me, r.PostFormValue("email"), 200, 0)
	if s.fail(w, r, err) {
		return adminUserRow{}, false
	}
	for _, u := range rows {
		if u.ID != id {
			continue
		}
		row, err := s.withCandidates(r, newUserRow(u, me))
		if s.fail(w, r, err) {
			return adminUserRow{}, false
		}
		return row, true
	}
	s.notFound(w, r)
	return adminUserRow{}, false
}

// handleAdminPermissions saves the two switches of the permission form. The form posts role=admin when the Administrator switch
// is on and nothing otherwise; whatever else arrives (a forged role=superuser) goes to the service, which answers 403 for every
// cell of the permission matrix the actor may not use. The Organiser switch is can_create.
func (s *Server) handleAdminPermissions(w http.ResponseWriter, r *http.Request) {
	me := *userFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w, r)
		return
	}
	role := core.Role(r.PostFormValue("role"))
	if role == "" {
		role = core.RoleUser
	}
	can := r.PostFormValue("can_create") == "on" && role == core.RoleUser
	err = s.auth.SetUserPermissions(r.Context(), me, id, role, can)
	if errors.Is(err, core.ErrForbidden) || (err != nil && !errors.Is(err, core.ErrRefused) && !errors.Is(err, core.ErrInvalidUser)) {
		s.fail(w, r, err)
		return
	}
	row, ok := s.userRowFor(w, r, me, id)
	if !ok {
		return
	}
	if err != nil { // a refusal of the state (closed account, last superuser) or an unknown role: the form stays open with what was sent
		row.Role, row.CanCreate, row.Open, row.Err = role, can, true, reason(err)
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_users", "admin_user_row", row)
		return
	}
	row.Notice = "Saved"
	s.render.partial(w, http.StatusOK, "admin_users", "admin_user_row", row)
}

// nonce is a value that differs between two draws of a form.
func nonce() string { return strconv.FormatInt(time.Now().UnixNano(), 36) }

// shownLink is a link that was just made: the message to copy, shown once (docs/auth.md, "Single-use links").
type shownLink struct {
	Message string
	Expires time.Time
}

// absoluteURL is a path of this site as a full address, for the messages an administrator copies (the scheme follows the proxy).
func absoluteURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}

// noStore marks an answer that carries a secret: nothing keeps a copy of it.
func noStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "no-store") }

// recoveryTTL reads the expiry select of the recovery card: a number of hours, 0 for the service's default.
// A missing value is 0 (the default); anything else that is not a number of hours, or is negative, is a negative duration the
// service refuses with its range message. Values are clamped before the multiplication so a huge number cannot wrap around.
func recoveryTTL(r *http.Request) time.Duration {
	v := strings.TrimSpace(r.PostFormValue("hours"))
	if v == "" {
		return 0
	}
	h, err := strconv.Atoi(v)
	if err != nil || h < 0 {
		return -time.Hour
	}
	return time.Duration(min(h, maxClampUnits)) * time.Hour
}

// maxClampUnits bounds a number of hours or days from a form before it becomes a time.Duration (far above any allowed range).
const maxClampUnits = 100_000

// handleAdminRecovery makes a recovery link for an account and answers the row with the message to hand over (superusers only).
func (s *Server) handleAdminRecovery(w http.ResponseWriter, r *http.Request) {
	me := *userFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w, r)
		return
	}
	link, err := s.recovery.Generate(r.Context(), me, id, recoveryTTL(r))
	s.recoveryAnswer(w, r, me, id, err, func(row *adminUserRow) {
		row.Link = &shownLink{Message: recoveryMessage(r, link), Expires: link.ExpiresAt}
		noStore(w)
	})
}

// handleAdminRecoveryRevoke revokes the account's pending link (superusers only).
func (s *Server) handleAdminRecoveryRevoke(w http.ResponseWriter, r *http.Request) {
	me := *userFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w, r)
		return
	}
	err = s.recovery.Revoke(r.Context(), me, id)
	s.recoveryAnswer(w, r, me, id, err, func(row *adminUserRow) { row.RecoveryMsg = "The link is revoked." })
}

// recoveryAnswer draws the row again after a recovery action: a refusal (own account, closed account, no pending link, expiry out of
// range) is a 422 with its reason, the matrix of who may is a 403, success runs ok on the row.
func (s *Server) recoveryAnswer(w http.ResponseWriter, r *http.Request, me core.User, id int64, err error, ok func(*adminUserRow)) {
	refused := errors.Is(err, core.ErrRefused) || errors.Is(err, core.ErrInvalidInput)
	if err != nil && !refused {
		s.fail(w, r, err)
		return
	}
	row, found := s.userRowFor(w, r, me, id)
	if !found {
		return
	}
	row.RecoveryOpen = true
	if refused {
		row.RecoveryMsg, row.RecoveryErr = reason(err), true
		s.render.partial(w, http.StatusUnprocessableEntity, "admin_users", "admin_user_row", row)
		return
	}
	ok(&row)
	s.render.partial(w, http.StatusOK, "admin_users", "admin_user_row", row)
}

// recoveryMessage is the text a superuser pastes to the person: the link and when it stops working.
func recoveryMessage(r *http.Request, l core.RecoveryLink) string {
	return "Open this link to set a new password: " + absoluteURL(r, "/recover/"+url.PathEscape(l.Token)) +
		"\nIt works once and expires on " + l.ExpiresAt.In(locationFrom(r.Context())).Format("2 January 2006, 15:04") + "."
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
	row, ok := s.userRowFor(w, r, me, id)
	if !ok {
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
}
