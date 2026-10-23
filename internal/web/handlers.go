package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
}

func (s *Server) newCampaignView(r *http.Request, in core.CampaignInput, errMsg string) (newCampaignView, error) {
	view := newCampaignView{pageBase: s.base(r), Input: in, Error: errMsg}
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
	view, err := s.newCampaignView(r, core.CampaignInput{}, "")
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
	err = func() error {
		if _, err := in.Validate(); err != nil { // before storing any upload
			return err
		}
		var err error
		if in.Vanilla, err = s.rulesetChoice(r, core.KindVanilla); err != nil {
			return err
		}
		if in.BattleCompanies, err = s.rulesetChoice(r, core.KindBattleCompanies); err != nil {
			return err
		}
		in.Glossary, err = s.rulesetChoice(r, core.KindGlossary)
		return err
	}()
	if err == nil {
		_, err = s.recent.create([]any{"campaign", user.ID, in}, func() (int64, error) {
			_, err := s.campaigns.Create(r.Context(), *user, in)
			return 0, err
		})
	}
	switch {
	case errors.Is(err, core.ErrInvalidCampaign) || errors.Is(err, core.ErrInvalidRuleset):
		view, verr := s.newCampaignView(r, in, reason(err))
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

// rulesetChoice turns the form's answer for one kind of rules file into the hash to store with the campaign ("" is the
// app's default). The browser announces the hash of the file it picked and uploads the file only when the server did
// not know the hash; the server checks both cases.
func (s *Server) rulesetChoice(r *http.Request, kind core.RulesetKind) (string, error) {
	prefix := string(kind)
	if r.PostFormValue(prefix+"-mode") != "file" {
		return "", nil
	}
	hash := r.PostFormValue(prefix + "-hash")
	file, _, err := r.FormFile(prefix + "-file")
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) { // not uploaded: it must be a file the server already has
		info, err := s.rulesets.Lookup(r.Context(), hash)
		switch {
		case errors.Is(err, core.ErrNotFound):
			return "", errors.Join(core.ErrInvalidRuleset, errors.New("Choose a file for the "+kind.Label()))
		case err != nil:
			return "", err
		case info.Kind != kind:
			return "", errors.Join(core.ErrInvalidRuleset, errors.New("That file is not a "+kind.Label()+" file"))
		}
		return info.Hash, nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	info, err := s.rulesets.StoreAs(r.Context(), *userFrom(r.Context()), kind, hash, content)
	return info.Hash, err
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
