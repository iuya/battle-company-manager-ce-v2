package web

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/core/ttshelper"
)

// The copy button of a company and its dialog (docs/tabletop-simulator.md): the company as the army list the Tabletop Simulator
// mod "MESBG FTC - Version 0.4" spawns from its ArmyList notebook tab.

// ttsTip is the tooltip of the button (also its explanation in the dialog).
const ttsTip = "Copy this company as an army list for Tabletop Simulator. Each unit becomes the closest standard MESBG model, because the " +
	"Tabletop Simulator mod can only spawn standard models. Paste it into a Notebook tab named ArmyList and press Spawn Army. " +
	"Names, paths, injuries and special rules are not carried over."

// ttsNoteGroups are the kinds of note in the order the dialog shows them, with their headings.
var ttsNoteGroups = []struct{ Kind, Title, Hint string }{
	{core.TTSNoModel, "Not in the list", "These units have no model in the mod."},
	{core.TTSOptionDropped, "Left out", "The model has no option for these."},
	{core.TTSShownAs, "Shown as", "The mod has no model of its own for these, so a stand-in is used."},
	{core.TTSApproximate, "Closest model", "The mod has no model with exactly this wargear."},
	{core.TTSHasStates, "Dismounting", "Change the figure's state in Tabletop Simulator to dismount it."},
	{core.TTSExtraFigure, "Extra figures", "Added by the switches above."},
	{core.TTSFallen, "Second warband", "A live match: the wounded and the fallen are listed apart."},
}

// ttsNoteGroup is the notes of one kind.
type ttsNoteGroup struct {
	Title, Hint string
	Notes       []string
}

// ttsView is the dialog's content.
type ttsView struct {
	CampaignID int64
	CompanyID  int64
	Name       string
	Export     core.TTSExport
	Foot       bool // switch: the on-foot line of riders
	Mounts     bool // switch: the mounts that fight as figures of their own
	Live       bool // the list is of the company in its open match
	Custom     bool // the campaign has a changed rules file: names may not exist in the mod
	Groups     []ttsNoteGroup

	// Level B: the names and descriptions of the figures.
	Rules      string // switch: the rule texts in the names (core.TTSRulesNone, TTSRulesShort or TTSRulesFull), as the player chose
	Names      core.TTSNamesPayload
	NamesJSON  string
	NameGroups []ttsNoteGroup
	Lua        string // the helper's script, for the fallback

	// Aura rings (docs/aura-ranges.md): the switches of the Auras section and what the company has.
	AuraSel core.AuraSelection
	Auras   core.AuraSummary
}

// auraKindOrder is the kinds of data.auras in the order of the dialog; the query lists the ones that are on.
var auraKindOrder = []core.AuraKind{"banner", "command", "support", "enemy", "utility"}

// auraSwitches are the group switches of the Auras section in the order the dialog shows them (the group "always" has none: it is on with
// the master switch). Default is the owner's choice (docs/aura-ranges.md section 9): declared and conditional on, the others off.
var auraSwitches = []struct {
	Group, Param, Label, Hint string
	Default                   bool
}{
	{core.AuraGroupDeclared, "adeclared", "Rules that need an action", "Heroic Actions, Active rules and once-per-game rules.", true},
	{core.AuraGroupConditional, "aconditional", "Rules that need an event", "Rules that start working after something happened in the game.", true},
	{core.AuraGroupCast, "acast", "Spells in effect", "Magical Powers of the bearer while they are in effect.", false},
	{core.AuraGroupTarget, "atarget", "Targeting ranges", "The radius only limits the choice of one model.", false},
	{core.AuraGroupLarge, "alarge", "Large rings, over 12 inches", "They fill the table, like the war horn's 24 inches.", false},
	{core.AuraGroupOptional, "aoptional", "Optional rings", "Light of a Torch: correct, but not a rule that decides who is affected.", false},
}

// ttsAuraSelection reads the Auras section of the query: the master switch is on unless it says 0, akinds lists the kinds that are on
// (absent: all), and each group switch has its default.
func ttsAuraSelection(r *http.Request) core.AuraSelection {
	q := r.URL.Query()
	sel := core.AuraSelection{Draw: ttsSwitch(r, "auras")}
	if q.Has("akinds") {
		sel.Kinds = map[core.AuraKind]bool{}
		for _, k := range strings.Split(q.Get("akinds"), ",") {
			if k = strings.TrimSpace(k); k != "" {
				sel.Kinds[core.AuraKind(k)] = true
			}
		}
	}
	for _, g := range auraSwitches {
		on := g.Default
		if v := q.Get(g.Param); v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				on = b
			}
		}
		switch g.Group {
		case core.AuraGroupDeclared:
			sel.Declared = on
		case core.AuraGroupConditional:
			sel.Conditional = on
		case core.AuraGroupCast:
			sel.Cast = on
		case core.AuraGroupTarget:
			sel.Target = on
		case core.AuraGroupLarge:
			sel.Large = on
		case core.AuraGroupOptional:
			sel.Optional = on
		}
	}
	return sel
}

func (v ttsView) groupOn(g string) bool {
	switch g {
	case core.AuraGroupDeclared:
		return v.AuraSel.Declared
	case core.AuraGroupConditional:
		return v.AuraSel.Conditional
	case core.AuraGroupCast:
		return v.AuraSel.Cast
	case core.AuraGroupTarget:
		return v.AuraSel.Target
	case core.AuraGroupLarge:
		return v.AuraSel.Large
	case core.AuraGroupOptional:
		return v.AuraSel.Optional
	}
	return true
}

// AurasOn is whether the master switch is on.
func (v ttsView) AurasOn() bool { return v.AuraSel.Draw }

// HasAuraKinds is whether the company has any aura ring at all (in any group).
func (v ttsView) HasAuraKinds() bool { return len(v.Auras.Kinds) > 0 }

// auraKindRow is one row of the Auras section: a kind the company has.
type auraKindRow struct {
	ID, Label, Letter, Colour, Text string
	On                              bool
	Rings                           int
	Radii                           string // `3", 6"`
	Thick                           bool   // drawn with the thicker line (the enemy kind): a cue that does not rely on colour
}

// AuraKinds are the rows of the kinds the company has.
func (v ttsView) AuraKinds() []auraKindRow {
	var out []auraKindRow
	for _, k := range v.Auras.Kinds {
		out = append(out, auraKindRow{ID: string(k.Kind), Label: k.Label, Letter: strings.ToUpper(string(k.Kind)[:1]), Colour: k.Colour, Text: k.Text, On: k.On, Rings: k.Rings,
			Radii: auraRadii(k.Radii), Thick: k.Kind == "enemy"})
	}
	return out
}

// auraRadii writes radii as `3", 6", 12"`.
func auraRadii(rs []float64) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = auraInches(r) + `"`
	}
	return strings.Join(parts, ", ")
}

// auraInches writes a radius without a needless fraction: 6, 4.5.
func auraInches(r float64) string { return strconv.FormatFloat(r, 'f', -1, 64) }

// auraSwitchRow is a group switch with what it does.
type auraSwitchRow struct {
	Name, Label, Hint, Count string
	On                       bool
}

// AuraGroups are the group switches that matter to this company: those with at least one ring (the count says how many they add or remove).
func (v ttsView) AuraGroups() []auraSwitchRow {
	var out []auraSwitchRow
	for _, g := range auraSwitches {
		n := v.Auras.Counts[g.Group]
		if n == 0 {
			continue
		}
		on := v.groupOn(g.Group)
		c := "adds " + counted(n, "ring")
		if on {
			c = counted(n, "ring")
		}
		out = append(out, auraSwitchRow{Name: g.Param, Label: g.Label, Hint: g.Hint, Count: c, On: on})
	}
	return out
}

// counted writes "1 ring", "3 rings".
func counted(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return itoa(n) + " " + word + "s"
}

// AuraTotals is the line under the rows: "7 rings on 5 figures".
func (v ttsView) AuraTotals() string {
	return counted(v.Auras.Rings, "ring") + " on " + counted(v.Auras.Figures, "figure")
}

// HelperV2 is whether the names carry rings, so an older helper cannot read them.
func (v ttsView) HelperV2() bool { return v.Names.Version >= core.TTSNamesVersionAuras }

// ttsNamesNoteGroups are the kinds of note of the names, in the order the dialog shows them.
var ttsNamesNoteGroups = []struct{ Kind, Title, Hint string }{
	{core.TTSTooLarge, "Too large", "The Notebook may cut the text, and the helper then cannot read it."},
	{core.TTSRulesLowered, "Rule texts left out to fit", "The names would pass the size limit with the rule texts you chose."},
	{core.TTSDescriptionCut, "Descriptions shortened", "A description has a limit, so some lines or a rule text were cut."},
	{core.TTSAurasLeftOut, "Aura rings left out", "A figure draws at most 4 rings and the names carry at most 150, so the rest are not drawn."},
}

// HasNames is whether there are figures to name.
func (v ttsView) HasNames() bool { return len(v.Names.Figures) > 0 && v.NamesJSON != "" }

// NamesRows is the height of the names' text area.
func (v ttsView) NamesRows() int { return 6 }

// NamesSize is the size of the JSON for people: "9.4 KB".
func (v ttsView) NamesSize() string {
	n := v.Names.Size
	if n < 1024 {
		return itoa(n) + " bytes"
	}
	return strconv.FormatFloat(float64(n)/1024, 'f', 1, 64) + " KB"
}

// NamesLimit is the size the generator keeps to, in KB.
func (v ttsView) NamesLimit() int { return core.TTSNamesMaxBytes / 1024 }

// NamesOver is whether the JSON is over the limit even without rule texts.
func (v ttsView) NamesOver() bool { return v.Names.Size > core.TTSNamesMaxBytes }

// RulesIs is whether the switch is at that level (for the radio buttons).
func (v ttsView) RulesIs(level string) bool { return v.Rules == level }

// ttsRules reads the rule-text switch of the query: none (names only) unless it says short or full.
func ttsRules(r *http.Request) string {
	switch v := r.URL.Query().Get("rules"); v {
	case core.TTSRulesShort, core.TTSRulesFull:
		return v
	}
	return core.TTSRulesNone
}

// HasList is whether anything can be pasted.
func (v ttsView) HasList() bool { return strings.TrimSpace(v.Export.Text) != "" }

// Reported is the model count the mod's own message will show: the company's models and every extra figure.
func (v ttsView) Reported() int { return v.Export.Models + v.Export.ExtraFigures }

// Rows is the height of the text area in lines.
func (v ttsView) Rows() int { return min(16, max(4, strings.Count(v.Export.Text, "\n")+2)) }

// Tip is the button's tooltip.
func (v ttsView) Tip() string { return ttsTip }

// URL is the address of the dialog's content.
func (v ttsView) URL() string {
	return "/campaigns/" + itoa(int(v.CampaignID)) + "/companies/" + itoa(int(v.CompanyID)) + "/tts" + ttsQuery(v.Live)
}

func ttsQuery(live bool) string {
	if live {
		return "?live=1"
	}
	return ""
}

// ttsSwitch reads a switch of the query: on unless it says 0.
func ttsSwitch(r *http.Request, name string) bool {
	v := r.URL.Query().Get(name)
	b, err := strconv.ParseBool(v)
	return v == "" || err != nil || b
}

// customRules is whether the campaign is played with a rules file other than the one the app ships with: the unit names of such a
// campaign may not exist in the mod.
func customRules(c core.CampaignDetail) bool {
	for _, f := range c.Rulesets {
		if (f.Kind == core.KindVanilla || f.Kind == core.KindBattleCompanies) && !f.Default {
			return true
		}
	}
	return false
}

// ttsGroups sorts the notes by kind, in the order of the dialog.
func ttsGroups(notes []core.TTSNote) []ttsNoteGroup { return ttsGroupsOf(ttsNoteGroups, notes) }

func ttsGroupsOf(groups []struct{ Kind, Title, Hint string }, notes []core.TTSNote) []ttsNoteGroup {
	var out []ttsNoteGroup
	for _, g := range groups {
		var texts []string
		for _, n := range notes {
			if n.Kind == g.Kind {
				texts = append(texts, n.Text)
			}
		}
		if len(texts) > 0 {
			out = append(out, ttsNoteGroup{Title: g.Title, Hint: g.Hint, Notes: texts})
		}
	}
	return out
}

// HasCopyButton is whether the page draws the copy button: the company has units.
func (v myCompanyView) HasCopyButton() bool { return len(v.Company.Roster) > 0 }

// TTSURL is the address of the dialog's content for the company on the page (live when it is in an open match).
func (v myCompanyView) TTSURL() string {
	return "/campaigns/" + itoa(int(v.Campaign.ID)) + "/companies/" + itoa(int(v.Company.ID)) + "/tts" + ttsQuery(v.Live != nil)
}

// TTSTip is the button's tooltip.
func (v myCompanyView) TTSTip() string { return ttsTip }

// handleTTS answers the content of the dialog: the company's army list with the switches as the query says. Whoever may see the
// company's page may ask (a member of the campaign); a stranger gets the 404 of the page.
func (s *Server) handleTTS(w http.ResponseWriter, r *http.Request) {
	base, ok := s.loadCampaign(w, r, "")
	if !ok {
		return
	}
	cid, ok := s.idParam(w, r, "company")
	if !ok {
		return
	}
	co, err := s.campaigns.Company(r.Context(), base.Campaign.ID, cid)
	if s.fail(w, r, err) {
		return
	}
	view := ttsView{CampaignID: base.Campaign.ID, CompanyID: co.ID, Name: co.Name, Foot: ttsSwitch(r, "foot"), Mounts: ttsSwitch(r, "mounts"), Custom: customRules(base.Campaign)}
	var live *core.TTSLive
	if r.URL.Query().Get("live") == "1" {
		lc, err := s.campaigns.Live(r.Context(), *userFrom(r.Context()), base.Campaign.ID, co.ID)
		switch {
		case err == nil:
			live, view.Live = &core.TTSLive{Units: lc.Units, Absent: lc.Absent}, true
		case !errors.Is(err, core.ErrNotFound): // the match ended meanwhile: the plain list
			s.fail(w, r, err)
			return
		}
	}
	view.Export, err = s.campaigns.TTSExport(r.Context(), base.Campaign.ID, co.ID, live, core.TTSOptions{FootVersions: view.Foot, MountFigures: view.Mounts})
	if s.fail(w, r, err) {
		return
	}
	view.Groups = ttsGroups(view.Export.Notes)
	view.Rules = ttsRules(r)
	view.AuraSel = ttsAuraSelection(r)
	if view.HasList() {
		view.Auras, err = s.campaigns.TTSAuras(r.Context(), base.Campaign.ID, co.ID, live, core.TTSOptions{FootVersions: view.Foot, MountFigures: view.Mounts}, view.AuraSel)
		if s.fail(w, r, err) {
			return
		}
		view.Names, err = s.campaigns.TTSNames(r.Context(), base.Campaign.ID, co.ID, view.Export, core.TTSNamesOptions{Rules: view.Rules, Auras: view.AuraSel})
		if s.fail(w, r, err) {
			return
		}
		js, err := view.Names.JSON()
		if s.fail(w, r, err) {
			return
		}
		view.NamesJSON = string(js)
		view.NameGroups = ttsGroupsOf(ttsNamesNoteGroups, view.Names.Notes)
	}
	view.Lua = ttshelper.ShowLua()
	s.render.partial(w, http.StatusOK, "campaign_my_company", "tts_dialog", view)
}

// handleTTSHelper serves the helper for level B: a zip with the Saved Object and its icon. The content is constant, so it carries an
// ETag and may be cached. Any signed-in user may download it (it holds no company data).
func (s *Server) handleTTSHelper(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "application/zip")
	h.Set("Content-Disposition", `attachment; filename="bc-names-helper.zip"`)
	h.Set("Cache-Control", "public, max-age=3600")
	h.Set("ETag", ttshelper.ETag())
	http.ServeContent(w, r, "bc-names-helper.zip", time.Time{}, bytes.NewReader(ttshelper.Zip()))
}
