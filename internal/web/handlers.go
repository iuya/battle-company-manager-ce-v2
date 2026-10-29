package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

type homeView struct {
	pageBase
	Campaigns    []core.CampaignSummary
	ShowFinished bool
	// Newcomer is an account that is in no campaign at all (finished ones count): it gets a plain welcome and no toggle.
	Newcomer bool
}

// navView is the data of the sidebar's campaign list (the template "nav_campaigns" reads Campaigns, as homeView's does).
type navView struct {
	pageBase
	Campaigns []core.CampaignNav
}

const finishedCookie = "bcm-finished" // "1" when the user wants finished campaigns listed (main page and sidebar)

func showFinished(r *http.Request) bool {
	c, err := r.Cookie(finishedCookie)
	return err == nil && c.Value == "1"
}

// handleHome is the main page: the signed-in user's active campaigns.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	with := showFinished(r)
	list, err := s.campaigns.List(r.Context(), userFrom(r.Context()).ID, with)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	newcomer := false
	if len(list) == 0 && !with { // nothing active: is there anything finished to show?
		all, err := s.campaigns.List(r.Context(), userFrom(r.Context()).ID, true)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		newcomer = len(all) == 0
	}
	base := s.bare(r)
	base.NavInline, base.NavOwn = true, true // the sidebar's campaign list is this page's list: no second request, no second query
	s.render.page(w, r, http.StatusOK, "home", homeView{pageBase: base, Campaigns: list, ShowFinished: with, Newcomer: newcomer})
}

// handleNavCampaigns returns the items of the sidebar's campaigns sub-menu. The sidebar fetches it once per full page
// load, so no other page has to know about it.
func (s *Server) handleNavCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := s.campaigns.NavCampaigns(r.Context(), userFrom(r.Context()).ID, showFinished(r))
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render.partial(w, http.StatusOK, "home", "nav_campaigns", navView{pageBase: s.bare(r), Campaigns: list})
}

// pickerView is one "which rules file?" choice of the new-campaign form.
type pickerView struct {
	Kind    core.RulesetKind
	Label   string
	Default core.RulesetInfo
}

type newCampaignView struct {
	pageBase
	Input   core.CampaignInput
	Pickers []pickerView
	Options []core.CampaignOption // the settings the rules file offers
	Error   string
	Report  *reportView // every problem of the rejected rules files
}

func (s *Server) newCampaignView(r *http.Request, in core.CampaignInput, errMsg string, report *reportView) (newCampaignView, error) {
	view := newCampaignView{pageBase: s.base(r), Input: in, Error: errMsg, Report: report}
	var err error
	if view.Options, err = s.rulesets.CampaignOptions(r.Context()); err != nil {
		return view, err
	}
	for _, kind := range []core.RulesetKind{core.KindVanilla, core.KindBattleCompanies, core.KindGlossary} {
		def, err := s.rulesets.Default(r.Context(), kind)
		if err != nil {
			return view, err
		}
		view.Pickers = append(view.Pickers, pickerView{Kind: kind, Label: kind.Label(), Default: def})
	}
	return view, nil
}

func (s *Server) handleNewCampaignPage(w http.ResponseWriter, r *http.Request) {
	if !userFrom(r.Context()).MayCreateCampaigns() {
		s.refuse(w, r, http.StatusForbidden, "you are not allowed to create campaigns")
		return
	}
	view, err := s.newCampaignView(r, core.CampaignInput{}, "", nil)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render.page(w, r, http.StatusOK, "campaign_new", view)
}

const maxUpload = 16 << 20 // the biggest rules file is about 1 MB

func (s *Server) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	if !user.MayCreateCampaigns() {
		s.refuse(w, r, http.StatusForbidden, "you are not allowed to create campaigns")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		s.refuse(w, r, http.StatusRequestEntityTooLarge, "the request is too large or malformed")
		return
	}
	in := core.CampaignInput{Name: r.PostFormValue("name"), Description: r.PostFormValue("description")}
	options, err := s.rulesets.CampaignOptions(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, o := range options { // the form has one checkbox per option the rules file offers
		in.Settings.Set(o.Key, r.PostFormValue("option-"+o.Key) != "")
	}
	if v := strings.TrimSpace(r.PostFormValue("companies-per-player")); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 {
			n = -1 // not a number, or below 1: Validate refuses it with the allowed range (an absent field keeps the default)
		}
		in.Settings.CompaniesPerPlayer = n
	}
	err = func() error {
		if _, err := in.Validate(); err != nil { // before storing any upload
			return err
		}
		hashes, err := s.rulesetChoices(r, core.KindVanilla, core.KindBattleCompanies, core.KindGlossary)
		if err != nil {
			return err
		}
		in.Vanilla, in.BattleCompanies, in.Glossary = hashes[0], hashes[1], hashes[2]
		return nil
	}()
	if err == nil {
		_, err = s.recent.create([]any{"campaign", user.ID, in}, func() (int64, error) {
			_, err := s.campaigns.Create(r.Context(), *user, in)
			return 0, err
		})
	}
	switch {
	case errors.Is(err, core.ErrInvalidCampaign) || errors.Is(err, core.ErrInvalidRuleset):
		rep := rulesetIssues(err)
		if rep != nil {
			rep.Note = "Nothing was stored and no campaign was created."
		}
		view, verr := s.newCampaignView(r, in, reason(err), rep)
		if verr != nil {
			s.serverError(w, r, verr)
			return
		}
		s.render.page(w, r, http.StatusUnprocessableEntity, "campaign_new", view)
	case err != nil:
		s.serverError(w, r, err)
	default:
		redirect(w, r, "/")
	}
}

// rulesetPick is the form's answer for one kind of rules file: the hash of a file the server already has (or "" for the app's
// default), or an upload that is not stored yet.
type rulesetPick struct {
	hash   string
	upload *core.RulesetUpload
}

// pickRuleset reads the form's answer for one kind without writing anything. The browser announces the hash of the file it picked
// and uploads the file only when the server did not know the hash; the server checks both cases.
func (s *Server) pickRuleset(r *http.Request, kind core.RulesetKind) (rulesetPick, error) {
	prefix := string(kind)
	if r.PostFormValue(prefix+"-mode") != "file" {
		return rulesetPick{}, nil
	}
	hash := r.PostFormValue(prefix + "-hash")
	file, _, err := r.FormFile(prefix + "-file")
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) { // not uploaded: it must be a file the server already has
		info, err := s.rulesets.Lookup(r.Context(), hash)
		switch {
		case errors.Is(err, core.ErrNotFound):
			return rulesetPick{}, errors.Join(core.ErrInvalidRuleset, errors.New("Choose a file for the "+kind.Label()))
		case err != nil:
			return rulesetPick{}, err
		case info.Kind != kind:
			return rulesetPick{}, errors.Join(core.ErrInvalidRuleset, errors.New("That file is not a "+kind.Label()+" file"))
		}
		return rulesetPick{hash: info.Hash}, nil
	}
	if err != nil {
		return rulesetPick{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return rulesetPick{}, err
	}
	return rulesetPick{upload: &core.RulesetUpload{Kind: kind, Hash: hash, Content: content}}, nil
}

// rulesetChoices turns the form's answers for several kinds into the hashes to store with the campaign ("" is the app's default).
// Uploads are all or nothing: every answer is read and every uploaded file checked before any file is stored, so a bad file leaves
// none of the others behind and no campaign is created.
func (s *Server) rulesetChoices(r *http.Request, kinds ...core.RulesetKind) ([]string, error) {
	picks := make([]rulesetPick, len(kinds))
	var errs []error
	for i, kind := range kinds {
		p, err := s.pickRuleset(r, kind)
		if err != nil {
			if !errors.Is(err, core.ErrInvalidRuleset) {
				return nil, err
			}
			errs = append(errs, err)
			continue
		}
		picks[i] = p
	}
	if len(errs) > 0 {
		return nil, core.JoinRulesetReports(errs...)
	}
	var uploads []core.RulesetUpload
	var at []int // index in kinds of each upload
	for i, p := range picks {
		if p.upload != nil {
			uploads = append(uploads, *p.upload)
			at = append(at, i)
		}
	}
	user := *userFrom(r.Context())
	switch len(uploads) {
	case 0:
	case 1: // one file is validated and stored by StoreAs itself
		info, err := s.rulesets.StoreAs(r.Context(), user, uploads[0].Kind, uploads[0].Hash, uploads[0].Content)
		if err != nil {
			return nil, err
		}
		picks[at[0]].hash = info.Hash
	default:
		infos, err := s.rulesets.StoreAllAs(r.Context(), user, uploads)
		if err != nil {
			return nil, err
		}
		for j, info := range infos {
			picks[at[j]].hash = info.Hash
		}
	}
	hashes := make([]string, len(kinds))
	for i, p := range picks {
		hashes[i] = p.hash
	}
	return hashes, nil
}

// rulesetChoice is rulesetChoices for one kind.
func (s *Server) rulesetChoice(r *http.Request, kind core.RulesetKind) (string, error) {
	hashes, err := s.rulesetChoices(r, kind)
	if err != nil {
		return "", err
	}
	return hashes[0], nil
}

// handleRulesetLookup tells the browser whether this file (hash, plus kind, name and version as query parameters) is already
// stored, so it can skip the upload. The answer carries the stored file's own hash, which the form then sends.
func (s *Server) handleRulesetLookup(w http.ResponseWriter, r *http.Request) {
	kind := core.RulesetKind(r.URL.Query().Get("kind"))
	if !kind.Valid() {
		s.refuse(w, r, http.StatusBadRequest, "unknown kind")
		return
	}
	info, err := s.rulesets.Find(r.Context(), kind, r.PathValue("hash"), "", "") // a file is its hash: name and version identify nothing
	if s.fail(w, r, err) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-cache")
	_ = json.NewEncoder(w).Encode(map[string]any{"hash": info.Hash, "kind": info.Kind, "name": info.Name, "version": info.Version, "default": info.Default})
}

// reason is the human part of an error built with errors.Join(sentinel, reason).
func reason(err error) string {
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		if errs := u.Unwrap(); len(errs) > 1 {
			return sentence(errs[len(errs)-1].Error())
		}
	}
	return sentence(err.Error())
}

// sentence starts a message with a capital: the core words its validation errors without one, and the pages show them as
// sentences of their own.
func sentence(msg string) string {
	r, n := utf8.DecodeRuneInString(msg)
	if n == 0 || !unicode.IsLower(r) {
		return msg
	}
	return string(unicode.ToUpper(r)) + msg[n:]
}
