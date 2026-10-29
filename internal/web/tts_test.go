package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"go.uber.org/mock/gomock"
)

func ttsFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t, Config{})
	d, _ := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Company(gomock.Any(), int64(7), int64(2)).Return(core.CampaignCompany{ID: 2, Name: "Riders", Faction: "Rohan"}, nil).AnyTimes()
	return f
}

var ttsSample = core.TTSExport{
	Text:   "(Captain of Dale: Shield)\n    (3x Warrior of Rohan: Shield)\n",
	Models: 4, ExtraFigures: 2, BreakPercent: 50, BreakPoint: "2",
	Notes: []core.TTSNote{
		{UnitID: 5, Kind: core.TTSShownAs, Text: "Swordsman is shown as Captain of Dale."},
		{UnitID: 6, Kind: core.TTSNoModel, Text: "Hador has no figure in the mod and is not in the list."},
		{UnitID: 7, Kind: core.TTSApproximate, Text: "Aldor: \"Shield\" is the closest figure."},
	},
}

var ttsNamesSample = core.TTSNamesPayload{Version: 1, Company: "Riders", Rules: "short", Size: 9650, Figures: []core.TTSFigure{
	{Key: "captain of dale: shield", Line: "Captain of Dale: Shield", Unit: 5, Name: "[b]Aldor[/b] (9)", Desc: "[i]Leader[/i]"},
	{Key: "warrior of rohan: shield", Line: "Warrior of Rohan: Shield", Unit: 6, Name: "[b]Eadric[/b] (8)", Desc: "x"},
}}

// ttsDefaultAuras is the selection of a dialog opened with no query: the switch on, the default groups on (docs/aura-ranges.md section 9).
var ttsDefaultAuras = core.AuraSelection{Draw: true, Declared: true, Conditional: true}

func (f *fixture) expectNames(rules string) {
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), gomock.Any(), core.TTSNamesOptions{Rules: rules, Auras: ttsDefaultAuras}).Return(ttsNamesSample, nil)
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), gomock.Any(), gomock.Any(), ttsDefaultAuras).Return(core.AuraSummary{}, nil).AnyTimes()
}

func TestTTSDialogShowsTheListTheCountsAndTheNotes(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, core.TTSOptions{FootVersions: true, MountFigures: true}).Return(ttsSample, nil)
	f.expectNames("none")
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil)))
	body := rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "<html") || !strings.Contains(rec.Header().Get("Cache-Control"), "private") {
		t.Fatalf("a private fragment: %d %v", rec.Code, rec.Header())
	}
	for _, want := range []string{"Army list for Tabletop Simulator", "(Captain of Dale: Shield)", "(3x Warrior of Rohan: Shield)", `id="tts-text"`, "readonly", "Copy to clipboard", `aria-live="polite"`,
		"<strong>4 models</strong> + 2 extra figures", "Break Point: <strong>2</strong> (50% of 4)", "will report <strong class=\"text-fg\">6 models</strong>",
		"Shown as", "Swordsman is shown as Captain of Dale.", "Not in the list", "Closest model", "Spawn Army", "ArmyList",
		`name="foot" value="1" class="peer sr-only" checked`, `name="mounts" value="1" class="peer sr-only" checked`, `hx-get="/campaigns/7/companies/2/tts"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the dialog lacks %q", want)
		}
	}
	for _, banned := range []string{"Changed rules file", "Second warband", "None of these units"} {
		if strings.Contains(body, banned) {
			t.Errorf("the dialog should not say %q", banned)
		}
	}
}

func TestTTSSwitchesComeFromTheQuery(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, core.TTSOptions{FootVersions: false, MountFigures: true}).Return(ttsSample, nil)
	f.expectNames("none")
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?foot=0&mounts=1", nil))).Body.String()
	if strings.Contains(body, `name="foot" value="1" class="peer sr-only" checked`) || !strings.Contains(body, `name="mounts" value="1" class="peer sr-only" checked`) {
		t.Error("the foot switch is off, the mounts switch on")
	}
}

func TestTTSLiveVariantPassesTheLiveStateAndShowsTheSecondWarband(t *testing.T) {
	f := ttsFixture(t)
	units := []core.LiveUnit{{UnitID: 5, Zone: core.ZoneField}, {UnitID: 6, Zone: core.ZoneCasualty}}
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(2)).Return(core.LiveCompany{CompanyID: 2, MatchID: 3, Units: units, Absent: []int64{8}}, nil)
	ex := ttsSample
	ex.Notes = append(slicesClone(ex.Notes), core.TTSNote{Kind: core.TTSFallen, Text: "The units that are wounded or out of the fight (Aldor) are in a second warband after the main one."})
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), &core.TTSLive{Units: units, Absent: []int64{8}}, core.TTSOptions{FootVersions: true, MountFigures: true}).Return(ex, nil)
	f.expectNames("none")
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?live=1", nil))).Body.String()
	for _, want := range []string{"Second warband", "are in a second warband after the main one", "as it is in the match now", `hx-get="/campaigns/7/companies/2/tts?live=1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the live dialog lacks %q", want)
		}
	}
}

func slicesClone(n []core.TTSNote) []core.TTSNote { return append([]core.TTSNote(nil), n...) }

func TestTTSLiveVariantOfACompanyWithoutOpenMatchIsThePlainList(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(2)).Return(core.LiveCompany{}, core.ErrNotFound)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	f.expectNames("none")
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?live=1", nil)))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "match now") {
		t.Errorf("the match ended meanwhile: %d", rec.Code)
	}
}

func TestTTSDialogWithoutAnyModelOffersNoCopy(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(core.TTSExport{BreakPercent: 50, BreakPoint: "0",
		Notes: []core.TTSNote{{Kind: core.TTSNoModel, Text: "Hador has no figure in the mod and is not in the list."}}}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil))).Body.String()
	if !strings.Contains(body, "None of these units has a standard model") || strings.Contains(body, "Copy to clipboard") || strings.Contains(body, "<textarea") || !strings.Contains(body, "Hador has no figure") {
		t.Errorf("no copy, but the notes: %.400s", body)
	}
}

func TestTTSDialogWarnsOfChangedRules(t *testing.T) {
	f := newFixture(t, Config{})
	d, _ := mineDetail()
	d.Rulesets = []core.RulesetInfo{{Kind: core.KindVanilla, Hash: "v", Default: true}, {Kind: core.KindBattleCompanies, Hash: "custom"}, {Kind: core.KindGlossary, Hash: "g", Default: true}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Company(gomock.Any(), int64(7), int64(2)).Return(core.CampaignCompany{ID: 2, Name: "Riders"}, nil)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	f.expectNames("none")
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil))).Body.String()
	if !strings.Contains(body, "uses a changed rules file") || !strings.Contains(body, "unit names may not exist in the mod") {
		t.Error("custom rules are flagged")
	}
	for _, c := range []struct {
		name string
		in   []core.RulesetInfo
		want bool
	}{
		{"shipped", []core.RulesetInfo{{Kind: core.KindVanilla, Default: true}, {Kind: core.KindBattleCompanies, Default: true}, {Kind: core.KindGlossary}}, false},
		{"another vanilla", []core.RulesetInfo{{Kind: core.KindVanilla}, {Kind: core.KindBattleCompanies, Default: true}}, true},
	} {
		if got := customRules(core.CampaignDetail{Rulesets: c.in}); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

func TestTTSDialogUnknownCompanyAndStrangerAre404(t *testing.T) {
	f := newFixture(t, Config{})
	d, _ := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Company(gomock.Any(), int64(7), int64(99)).Return(core.CampaignCompany{}, core.ErrNotFound)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/99/tts", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("unknown company: %d", rec.Code)
	}
	g := newFixture(t, Config{})
	g.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(8)).Return(core.CampaignDetail{}, core.ErrNotFound)
	if rec := g.do(g.signedIn(httptest.NewRequest("GET", "/campaigns/8/companies/2/tts", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("not a member: %d", rec.Code)
	}
	h := newFixture(t, Config{})
	if rec := h.do(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil)); rec.Code != http.StatusSeeOther {
		t.Errorf("signed out goes to the login: %d", rec.Code)
	}
}

func TestMyCompanyPageHasTheCopyButtonLiveOnesAskForTheLiveList(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	for _, want := range []string{`id="tts-open"`, `hx-get="/campaigns/7/companies/1/tts"`, `aria-label="Copy as an army list for Tabletop Simulator"`, "Paste it into a Notebook tab named ArmyList and press Spawn Army", "closest standard MESBG model"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	live := liveOwnerSetup(t, core.LiveEdit)
	body = live.do(live.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	if !strings.Contains(body, `hx-get="/campaigns/7/companies/1/tts?live=1"`) {
		t.Error("in a match the button asks for the live list")
	}
}

func TestEmptyCompanyHasNoCopyButton(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster = nil
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	if body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String(); strings.Contains(body, `id="tts-open"`) {
		t.Error("an empty company has nothing to copy")
	}
}

func TestTTSDialogShowsTheNamesBlock(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	f.expectNames("none")
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil))).Body.String()
	for _, want := range []string{"Name and describe the figures", `id="tts-names-text"`, "Copy names", `data-tts-copy="tts-names-text"`, `id="tts-names-status"`, `aria-live="polite"`,
		"2 figures", "9.4 KB of 24 KB", `name="rules" value="none" class="h-4 w-4 accent-[var(--accent)]" checked`, `name="rules" value="short"`, `name="rules" value="full"`,
		`href="/tts/helper.zip"`, `hx-boost="false"`, "Download the helper", "Saved Objects", "~/Library/Tabletop Simulator/Saves/Saved Objects", `Documents\My Games\Tabletop Simulator\Saves\Saved Objects`, "Linux (not verified)",
		"ArmyNames", "Apply names", "Re-apply", "drag", "Show the Lua (fallback, unverified)", `id="tts-lua-text"`, "Copy the Lua", "function onLoad",
		`hx-select-oob="#tts-names-result:outerHTML,#tts-auras:outerHTML"`, `id="tts-auras"`, "This company has no aura ranges."} {
		if !strings.Contains(body, want) {
			t.Errorf("the dialog lacks %q", want)
		}
	}
	// the JSON in the area is the payload's, unescaped once by the browser
	i := strings.Index(body, `id="tts-names-text"`)
	area := body[i : i+strings.Index(body[i:], "</textarea>")]
	area = area[strings.Index(area, ">")+1:]
	area = strings.NewReplacer("&#34;", `"`, "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#39;", "'").Replace(area)
	var p struct {
		V       int
		Company string
		Figures []map[string]any
	}
	if err := json.Unmarshal([]byte(area), &p); err != nil || p.V != 1 || len(p.Figures) != 2 || p.Company != "Riders" {
		t.Errorf("the area holds the ArmyNames JSON: %v %.200s", err, area)
	}
	for _, banned := range []string{"Too large", "Rule texts left out", "Descriptions shortened"} {
		if strings.Contains(body, banned) {
			t.Errorf("no note %q expected", banned)
		}
	}
}

func TestTTSRuleSwitchComesFromTheQueryAndAnUnknownValueIsNone(t *testing.T) {
	for q, want := range map[string]string{"rules=none": "none", "rules=full": "full", "rules=short": "short", "rules=bogus": "none", "": "none"} {
		f := ttsFixture(t)
		f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
		f.expectNames(want)
		body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?"+q, nil))).Body.String()
		if !strings.Contains(body, `name="rules" value="`+want+`" class="h-4 w-4 accent-[var(--accent)]" checked`) {
			t.Errorf("%q: the %s radio is checked", q, want)
		}
	}
}

func TestTTSNamesNotesAreShown(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	big := ttsNamesSample
	big.Rules, big.Size = "none", core.TTSNamesMaxBytes+2048
	big.Notes = []core.TTSNote{
		{Kind: core.TTSRulesLowered, Text: "Rule texts left out to fit."},
		{Kind: core.TTSDescriptionCut, Text: "2 descriptions lost lines to fit 900 characters."},
		{Kind: core.TTSTooLarge, Text: "The names are 26 KB, over the 24 KB the Notebook is meant to take: send fewer figures."},
	}
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), gomock.Any(), core.TTSNamesOptions{Rules: "full", Auras: ttsDefaultAuras}).Return(big, nil)
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), nil, gomock.Any(), ttsDefaultAuras).Return(core.AuraSummary{}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?rules=full", nil))).Body.String()
	for _, want := range []string{"Too large", "send fewer figures", "Rule texts left out to fit", "Descriptions shortened", "2 descriptions lost lines", "26.0 KB of 24 KB", "rule texts: <strong>none</strong>", "alert-warn"} {
		if !strings.Contains(body, want) {
			t.Errorf("the notes lack %q", want)
		}
	}
}

func TestTTSLiveVariantNamesUseTheLiveExport(t *testing.T) {
	f := ttsFixture(t)
	units := []core.LiveUnit{{UnitID: 5, Zone: core.ZoneCasualty}}
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(2)).Return(core.LiveCompany{CompanyID: 2, MatchID: 3, Units: units}, nil)
	ex := ttsSample
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), &core.TTSLive{Units: units}, gomock.Any()).Return(ex, nil)
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), ex, core.TTSNamesOptions{Rules: "none", Auras: ttsDefaultAuras}).Return(ttsNamesSample, nil)
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), &core.TTSLive{Units: units}, gomock.Any(), ttsDefaultAuras).Return(core.AuraSummary{}, nil)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?live=1", nil))); rec.Code != 200 {
		t.Errorf("live names: %d", rec.Code)
	}
}

func TestTTSNamesErrorIsAnError(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), gomock.Any(), gomock.Any(), gomock.Any()).Return(core.AuraSummary{}, nil)
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), gomock.Any(), gomock.Any()).Return(core.TTSNamesPayload{}, core.ErrNotFound)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("names for a vanished company: %d", rec.Code)
	}
}

func TestTTSHelperZip(t *testing.T) {
	f := newFixture(t, Config{})
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/tts/helper.zip", nil)))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	h := rec.Header()
	if h.Get("Content-Type") != "application/zip" || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment;") || !strings.Contains(h.Get("Cache-Control"), "public") || !strings.Contains(h.Get("Cache-Control"), "max-age=") || h.Get("ETag") == "" {
		t.Errorf("headers: %v", h)
	}
	b := rec.Body.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("a valid zip: %v", err)
	}
	var names []string
	for _, e := range zr.File {
		names = append(names, e.Name)
		r, _ := e.Open()
		data, _ := io.ReadAll(r)
		r.Close()
		switch {
		case strings.HasSuffix(e.Name, ".json") && !json.Valid(data):
			t.Errorf("%s is not JSON", e.Name)
		case strings.HasSuffix(e.Name, ".png") && !bytes.HasPrefix(data, []byte("\x89PNG")):
			t.Errorf("%s is not a PNG", e.Name)
		}
	}
	if len(names) != 2 {
		t.Errorf("the json and the png: %v", names)
	}
	// a conditional request with the tag is answered 304
	req := f.signedIn(httptest.NewRequest("GET", "/tts/helper.zip", nil))
	req.Header.Set("If-None-Match", h.Get("ETag"))
	if rec := f.do(req); rec.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: %d", rec.Code)
	}
}

func TestTTSHelperZipNeedsASession(t *testing.T) {
	f := newFixture(t, Config{})
	rec := f.do(httptest.NewRequest("GET", "/tts/helper.zip", nil))
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Errorf("signed out goes to the login: %d %v", rec.Code, rec.Header())
	}
}

// ttsAuraSample is what a company with banners, Heroes and a Torch shows: kinds with counts and radii, and what the switches add.
var ttsAuraSample = core.AuraSummary{
	Rings: 7, Figures: 5, LeftOut: 2,
	Kinds: []core.AuraKindSummary{
		{Kind: "banner", Label: "Banner", Colour: "yellow", Text: "Banners: friendly models re-roll a Duel die", Rings: 1, Radii: []float64{3}, On: true},
		{Kind: "command", Label: "Command", Colour: "orange", Text: "Courage and the Hero's shouts", Rings: 4, Radii: []float64{6, 12}, On: true},
		{Kind: "enemy", Label: "Enemy", Colour: "vermillion", Text: "Penalties against the enemy", Rings: 2, Radii: []float64{6}, On: false},
	},
	Counts: map[string]int{"declared": 3, "conditional": 0, "cast": 1, "target": 0, "large": 1, "optional": 1},
	Groups: map[string]core.AuraGroupDef{
		"declared": {ID: "declared", Label: "Rules that need an action", Text: "Heroic Actions"}, "cast": {ID: "cast", Label: "Spells in effect"},
		"large": {ID: "large", Label: "Large rings, over 15 inches", Text: "Rings fill the table"}, "optional": {ID: "optional", Label: "Optional rings"},
	},
}

func TestTTSAurasSectionShowsKindsCountsGroupsAndLegend(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), nil, core.TTSOptions{FootVersions: true, MountFigures: true}, ttsDefaultAuras).Return(ttsAuraSample, nil)
	v2 := ttsNamesSample
	v2.Version = 2
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), gomock.Any(), core.TTSNamesOptions{Rules: "none", Auras: ttsDefaultAuras}).Return(v2, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil))).Body.String()
	for _, want := range []string{
		`id="tts-auras"`, `name="auras" value="1"`, `id="tts-auras-on"`, "Draw aura rings",
		`id="tts-akind-banner" name="akind" value="banner"`, `id="tts-akind-command" name="akind" value="command"`, "1 ring, radii 3&#34;", "4 rings, radii 6&#34;, 12&#34;",
		"Rules that need an action", "adds 1 ring", "Spells in effect", "Large rings, over 15 inches", "Optional rings", `name="adeclared" value="1" class="peer sr-only" checked`,
		"7 rings on 5 figures", "2 left out", "data-tts-legend", "Command, orange", "Enemy, vermillion, thick line", "aura-ring-thick", "aura-k-banner",
		`id="tts-helper-v2"`, "Download the helper again", "numpad 1 to 3", "B C S E U",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the dialog lacks %q", want)
		}
	}
	// the enemy kind is off: its checkbox is not checked; the group with no rings has no switch
	if i := strings.Index(body, `id="tts-akind-enemy"`); i < 0 || strings.Contains(body[i:i+120], "checked") {
		t.Error("the enemy kind is off, so not checked")
	}
	for _, banned := range []string{`name="aconditional"`, `name="atarget"`, "This company has no aura ranges."} {
		if strings.Contains(body, banned) {
			t.Errorf("the dialog should not show %q", banned)
		}
	}
}

func TestTTSAurasQueryBuildsTheSelection(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	sel := core.AuraSelection{Draw: true, Kinds: map[core.AuraKind]bool{"banner": true, "support": true}, Declared: false, Conditional: true, Cast: true, Large: true}
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), nil, gomock.Any(), sel).Return(ttsAuraSample, nil)
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), gomock.Any(), core.TTSNamesOptions{Rules: "none", Auras: sel}).Return(ttsNamesSample, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?akinds=banner,support&adeclared=0&aconditional=1&acast=1&alarge=1&atarget=0", nil))).Body.String()
	if strings.Contains(body, `name="adeclared" value="1" class="peer sr-only" checked`) || !strings.Contains(body, `name="acast" value="1" class="peer sr-only" checked`) {
		t.Error("the group switches come from the query")
	}
	if strings.Contains(body, `id="tts-helper-v2"`) {
		t.Error("a v1 payload needs no new helper")
	}
}

func TestTTSAurasSwitchOffAsksForNoRingsAndHidesTheRows(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	sel := core.AuraSelection{Declared: true, Conditional: true}
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), nil, gomock.Any(), sel).Return(core.AuraSummary{Kinds: ttsAuraSample.Kinds}, nil)
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), gomock.Any(), core.TTSNamesOptions{Rules: "none", Auras: sel}).Return(ttsNamesSample, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?auras=0", nil))).Body.String()
	if !strings.Contains(body, `id="tts-auras-on" name="auras" value="1" class="peer sr-only" >`) || strings.Contains(body, `name="akind"`) || strings.Contains(body, "data-tts-legend") {
		t.Error("with the switch off only the switch is drawn")
	}
}

func TestTTSAurasLiveVariantAsksForTheLiveRings(t *testing.T) {
	f := ttsFixture(t)
	units := []core.LiveUnit{{UnitID: 5, Zone: core.ZoneCasualty}}
	f.camp.EXPECT().Live(gomock.Any(), frodo, int64(7), int64(2)).Return(core.LiveCompany{CompanyID: 2, MatchID: 3, Units: units}, nil)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), &core.TTSLive{Units: units}, gomock.Any()).Return(ttsSample, nil)
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), &core.TTSLive{Units: units}, gomock.Any(), ttsDefaultAuras).Return(ttsAuraSample, nil)
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), gomock.Any(), gomock.Any()).Return(ttsNamesSample, nil)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts?live=1", nil))); rec.Code != 200 {
		t.Errorf("live auras: %d", rec.Code)
	}
}

func TestTTSAurasLeftOutNoteIsShown(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	f.camp.EXPECT().TTSAuras(gomock.Any(), int64(7), int64(2), nil, gomock.Any(), ttsDefaultAuras).Return(ttsAuraSample, nil)
	p := ttsNamesSample
	p.Notes = []core.TTSNote{{Kind: core.TTSAurasLeftOut, Text: "2 aura rings were left out (limit 4 per figure, 150 in all)."}}
	f.camp.EXPECT().TTSNames(gomock.Any(), int64(7), int64(2), gomock.Any(), gomock.Any()).Return(p, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil))).Body.String()
	if !strings.Contains(body, "Aura rings left out") || !strings.Contains(body, "2 aura rings were left out") {
		t.Error("the note of the rings left out is shown")
	}
}

func TestTTSAurasNoRingsSaysSo(t *testing.T) {
	f := ttsFixture(t)
	f.camp.EXPECT().TTSExport(gomock.Any(), int64(7), int64(2), nil, gomock.Any()).Return(ttsSample, nil)
	f.expectNames("none")
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2/tts", nil))).Body.String()
	if !strings.Contains(body, "This company has no aura ranges.") || strings.Contains(body, `name="auras"`) {
		t.Error("a company without rings says so and has no switch")
	}
}
