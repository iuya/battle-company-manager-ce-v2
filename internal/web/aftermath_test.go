package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func endedMatch() core.Match {
	m := editableMatch()
	m.Ended = true
	m.Result = core.MatchResult{Decided: true, Winner: 1}
	return m
}

func twoAftermaths(a, b core.AftermathStatus) []core.AftermathRef {
	return []core.AftermathRef{
		{MatchID: 3, MatchNumber: 3, CompanyID: 1, CompanyName: "Rangers", Status: a},
		{MatchID: 3, MatchNumber: 3, CompanyID: 2, CompanyName: "Raiders", Status: b},
	}
}

func TestAftermathCardIsHiddenWhileTheMatchIsOpen(t *testing.T) {
	f := matchWrite(t, editableMatch())
	if body := f.do(f.signedIn(httpGet(testMatchPath))).Body.String(); strings.Contains(body, `id="aftermath"`) {
		t.Error("an open match has no Aftermath card")
	}
}

func TestAftermathCardForAPlayer(t *testing.T) {
	f := matchWrite(t, endedMatch())
	f.aftermaths = twoAftermaths(core.AftermathPending, core.AftermathInjuries)
	body := f.do(f.signedIn(httpGet(testMatchPath))).Body.String()
	for _, want := range []string{`id="aftermath"`, "Rangers", "Raiders", ">Pending<", ">In progress<", "/campaigns/7/matches/3/aftermath/1", "/campaigns/7/matches/3/aftermath/2"} {
		if !strings.Contains(body, want) {
			t.Errorf("card lacks %q", want)
		}
	}
	// frodo owns Rangers (Start) but not Raiders (View); no Waive for a player
	card := body[strings.Index(body, `id="aftermath"`):]
	if !strings.Contains(card, `/aftermath/1/begin"`) || !strings.Contains(card, `/aftermath/2">View<`) {
		t.Errorf("Start for the owner and View for the others:\n%s", card)
	}
	if strings.Contains(card, "Waive") {
		t.Error("a player cannot waive")
	}
}

func TestAftermathCardForTheOrganiserOffersWaiveAndContinue(t *testing.T) {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = gandalf.ID
	f.camp.EXPECT().Get(gomock.Any(), gandalf.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(endedMatch(), nil)
	f.aftermaths = twoAftermaths(core.AftermathLevelUps, core.AftermathDone)
	f.camp.EXPECT().Aftermath(gomock.Any(), gandalf, int64(7), int64(3), int64(2)).Return(core.Aftermath{
		Status: core.AftermathDone, Casualties: []core.Casualty{{ResultKey: "dead"}, {ResultKey: "injured"}},
		Preview: core.AftermathPreview{Experience: []core.UnitGain{{Total: 3}, {Total: 2}}, Influence: []core.AmountLine{{Amount: 2}, {Amount: 2}}}}, nil)
	card := f.do(f.signedInAs(gandalf, httpGet(testMatchPath))).Body.String()
	card = card[strings.Index(card, `id="aftermath"`):]
	for _, want := range []string{`/aftermath/1">Continue<`, `/aftermath/2">View<`, ">Done<", "2 casualties (1 dead) · &#43;5 Experience · &#43;4 Influence", `/aftermath/1/waive`, `name="note"`} {
		if !strings.Contains(card, want) {
			t.Errorf("organiser card lacks %q", want)
		}
	}
	if strings.Contains(card, `/aftermath/2/waive`) {
		t.Error("a finished aftermath cannot be waived")
	}
}

func TestAftermathCardShowsAnErrorInsteadOfFailingThePage(t *testing.T) {
	f := matchWrite(t, endedMatch())
	f.aftermathErr = errors.New("db down")
	rec := f.do(f.signedIn(httpGet(testMatchPath)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "The aftermath could not be loaded") {
		t.Errorf("%d\n%s", rec.Code, rec.Body.String())
	}
}

func waivePost(f *fixture, u core.User, company string, note string) *httptest.ResponseRecorder {
	r := form(testMatchPath+"/aftermath/"+company+"/waive", url.Values{"note": {note}})
	r.Header.Set("HX-Request", "true")
	return f.do(f.signedInAs(u, r))
}

func waiveFixture(t *testing.T) *fixture {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = gandalf.ID
	f.camp.EXPECT().Get(gomock.Any(), gomock.Any(), int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(endedMatch(), nil).AnyTimes()
	return f
}

func TestWaiveAftermathIsForbiddenToPlayers(t *testing.T) {
	if rec := waivePost(waiveFixture(t), frodo, "2", "gone"); rec.Code != http.StatusForbidden {
		t.Errorf("a player: %d", rec.Code)
	}
}

func TestWaiveAftermath(t *testing.T) {
	f := waiveFixture(t)
	if rec := waivePost(f, gandalf, "2", "  "); rec.Code != 422 || !strings.Contains(rec.Body.String(), "1 to 200 characters") {
		t.Errorf("no note: %d", rec.Code)
	}
	f.camp.EXPECT().WaiveAftermath(gomock.Any(), gandalf, int64(7), int64(3), int64(2), "player is gone").Return(core.Aftermath{Status: core.AftermathWaived}, nil)
	if rec := waivePost(f, gandalf, "2", " player is gone "); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Aftermath waived.") {
		t.Errorf("waived: %d\n%s", rec.Code, rec.Body.String())
	}
	f.camp.EXPECT().WaiveAftermath(gomock.Any(), gandalf, int64(7), int64(3), int64(1), "x").Return(core.Aftermath{}, core.Refuse("Already finished"))
	if rec := waivePost(f, gandalf, "1", "x"); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Already finished") {
		t.Errorf("refused: %d", rec.Code)
	}
	f.camp.EXPECT().WaiveAftermath(gomock.Any(), gandalf, int64(7), int64(3), int64(9), "x").Return(core.Aftermath{}, core.ErrNotFound)
	if rec := waivePost(f, gandalf, "9", "x"); rec.Code != 404 {
		t.Errorf("unknown company: %d", rec.Code)
	}
}

// --- the aftermath page ---

const amPath = testMatchPath + "/aftermath/1"

var amRules = core.AftermathRules{
	InjuryDice: core.DiceSpec{Count: 2, Sides: 6},
	Gains:      []core.XPGain{{Key: "participated", Label: "Took part", Amount: 1}, {Key: "mvp", Label: "MVP", Amount: 1}, {Key: "slain", Label: "Slain a hero", Amount: 1, Repeatable: true}},
	Bonuses:    []core.OddsBonus{{Key: "influence", Name: "Influence", Rating: 15, InfluenceWin: 2}, {Key: "toolkit", Name: "Tool Kit", Rating: 30}},
}

func amUnits() []core.CompanyUnit {
	return []core.CompanyUnit{{ID: 10, Name: "Aldor", Type: "Warrior of Rohan"}, {ID: 11, Name: "Brand", Type: "Warrior of Rohan"}, {ID: 12, Name: "Cirion", Type: "Warrior of Rohan", Injured: true}}
}

func amState(status core.AftermathStatus, access core.AftermathAccess) core.Aftermath {
	return core.Aftermath{MatchID: 3, CompanyID: 1, Status: status, Revision: 2, Outcome: "win", Access: access, Absent: []int64{12},
		Input:   core.AftermathInput{Models: []core.ModelSheet{{UnitID: 10, Outcome: core.OutcomeSurvived, Gains: map[string]int{"participated": 1}}, {UnitID: 11, Outcome: core.OutcomeCasualty, Gains: map[string]int{"participated": 1}}}},
		Preview: core.AftermathPreview{Experience: []core.UnitGain{{UnitID: 10, Total: 1}}, Influence: []core.AmountLine{{Amount: 2}}, Rolls: 1}}
}

// amFixture serves the ended match; frodo owns company 1.
func amFixture(t *testing.T) *fixture {
	f := matchWrite(t, endedMatch())
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(amRules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: amUnits()}, nil).AnyTimes()
	return f
}

func TestAftermathPageEditorSeesTheSheetForm(t *testing.T) {
	f := amFixture(t)
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathPending, core.AftermathEdit), nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{"Aldor", "Brand", `name="unit" value="10"`, `name="outcome-10" value="survived"`, `class="outcome-toggle" data-tone="danger"`, `name="took-10"`, `name="gain-10-mvp"`, `name="gain-10-slain"`, `name="odds" value="toolkit"`, "No effect on the aftermath.",
		`hx-post="/campaigns/7/matches/3/aftermath/1/sheet?auto=1"`, `/aftermath/1/start`, `name="revision" value="2"`, "Missed this game", "Cirion", `aria-current="step"`} {
		if !strings.Contains(body, want) {
			t.Errorf("sheet lacks %q", want)
		}
	}
	if strings.Contains(body, `name="outcome-12"`) {
		t.Error("an absent unit is not on the sheet")
	}
	if strings.Contains(body, `name="relic"`) {
		t.Error("no Relic chip unless the result ticks the Relic")
	}
}

func TestAftermathPageIsReadOnlyForOthersAndLockedForOpenMatches(t *testing.T) {
	f := amFixture(t)
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathPending, core.AftermathNone), nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	if strings.Contains(body, `name="outcome-10"`) || strings.Contains(body, "Save sheet") || !strings.Contains(body, "can read this aftermath but not change it") || !strings.Contains(body, "Casualty") {
		t.Error("a viewer gets the sheet read only")
	}
	f = matchWrite(t, endedMatch())
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(core.Aftermath{}, core.Refuse("End the match first."))
	if rec := f.do(f.signedIn(httpGet(amPath))); rec.Code != 200 || !strings.Contains(rec.Body.String(), "End the match first.") {
		t.Errorf("open match: %d", rec.Code)
	}
	f = matchWrite(t, endedMatch())
	if rec := f.do(f.signedIn(httpGet(testMatchPath + "/aftermath/99"))); rec.Code != 404 {
		t.Errorf("a company that is not in the match: %d", rec.Code)
	}
}

func sheetForm(extra url.Values) url.Values {
	v := url.Values{"revision": {"2"}, "unit": {"10", "11"}, "outcome-10": {"survived"}, "took-10": {"1"}, "gain-10-mvp": {"1"}, "gain-10-slain": {"2"}, "outcome-11": {"casualty"}, "odds": {"influence"}}
	for k, x := range extra {
		v[k] = x
	}
	return v
}

func amPost(f *fixture, path string, v url.Values) *httptest.ResponseRecorder {
	r := form(amPath+path, v)
	r.Header.Set("HX-Request", "true")
	return f.do(f.signedIn(r))
}

func TestSaveAndStartTheSheet(t *testing.T) {
	f := amFixture(t)
	want := core.AftermathInput{AgainstTheOdds: []string{"influence"}, Models: []core.ModelSheet{
		{UnitID: 10, Outcome: core.OutcomeSurvived, Gains: map[string]int{"participated": 1, "mvp": 1, "slain": 2}},
		{UnitID: 11, Outcome: core.OutcomeCasualty, Gains: map[string]int{}}}}
	f.camp.EXPECT().SaveAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, want).Return(amState(core.AftermathSheet, core.AftermathEdit), nil)
	if rec := amPost(f, "/sheet", sheetForm(nil)); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Sheet saved.") || !strings.Contains(rec.Body.String(), `id="aftermath-page"`) {
		t.Errorf("save: %d\n%s", rec.Code, rec.Body.String())
	}
	f.camp.EXPECT().StartAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, want).Return(amState(core.AftermathInjuries, core.AftermathEdit), nil)
	rec := amPost(f, "/start", sheetForm(nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Aftermath started") || !strings.Contains(rec.Body.String(), "Edit the sheet again") {
		t.Errorf("start: %d\n%s", rec.Code, rec.Body.String())
	}
}

// Start aftermath from the match page: the prefilled sheet starts at once and the page opens on its first step.
func TestBeginAftermathStartsWithThePrefilledSheet(t *testing.T) {
	f := amFixture(t)
	a := amState(core.AftermathPending, core.AftermathEdit)
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(a, nil)
	f.camp.EXPECT().StartAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, a.Input).Return(amState(core.AftermathInjuries, core.AftermathEdit), nil)
	if rec := amPost(f, "/begin", nil); rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") != amPath {
		t.Errorf("%d %q\n%s", rec.Code, rec.Header().Get("HX-Redirect"), rec.Body.String())
	}
}

// The sheet opens instead, with nothing started, when the player has something to say first. A rejected sheet and a second press land on the page too.
func TestBeginAftermathOpensThePageWithoutStarting(t *testing.T) {
	tied := amState(core.AftermathPending, core.AftermathEdit)
	tied.MVP = &core.MVPHint{Tied: []int64{10, 11}}
	blank := amState(core.AftermathPending, core.AftermathEdit)
	blank.Input.Models[1].Outcome = core.OutcomeSurvived
	for name, a := range map[string]core.Aftermath{"nothing recorded": blank, "tied MVP votes": tied, "already started": amState(core.AftermathInjuries, core.AftermathEdit), "saved sheet": amState(core.AftermathSheet, core.AftermathEdit)} {
		t.Run(name, func(t *testing.T) {
			f := amFixture(t)
			f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(a, nil)
			if rec := amPost(f, "/begin", nil); rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") != amPath {
				t.Errorf("%d %q", rec.Code, rec.Header().Get("HX-Redirect"))
			}
		})
	}
	t.Run("the sheet is refused", func(t *testing.T) {
		f := amFixture(t)
		f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathPending, core.AftermathEdit), nil)
		f.camp.EXPECT().StartAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, gomock.Any()).Return(core.Aftermath{}, errors.Join(core.ErrInvalidAftermath, errors.New("say which unit holds the Relic")))
		if rec := amPost(f, "/begin", nil); rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") != amPath {
			t.Errorf("%d %q", rec.Code, rec.Header().Get("HX-Redirect"))
		}
	})
	t.Run("a reader cannot start it", func(t *testing.T) {
		f := amFixture(t)
		f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathPending, core.AftermathNone), nil)
		if rec := amPost(f, "/begin", nil); rec.Code != http.StatusForbidden {
			t.Errorf("%d", rec.Code)
		}
	})
}

func TestSheetErrorsComeBesideTheUnitAndKeepWhatWasTyped(t *testing.T) {
	f := amFixture(t)
	f.camp.EXPECT().SaveAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, gomock.Any()).Return(core.Aftermath{},
		errors.Join(core.ErrInvalidAftermath, errors.New("Brand is a casualty and cannot hold the Relic"), errors.New("Only one MVP")))
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathSheet, core.AftermathEdit), nil)
	rec := amPost(f, "/sheet", sheetForm(url.Values{"outcome-11": {"fled"}}))
	body := rec.Body.String()
	if rec.Code != 422 || !strings.Contains(body, "Brand is a casualty and cannot hold the Relic") || !strings.Contains(body, "Only one MVP") {
		t.Fatalf("%d\n%s", rec.Code, body)
	}
	if !strings.Contains(body, `value="fled" x-model="outcome" checked`) && !strings.Contains(body, `outcome: 'fled'`) {
		t.Error("the typed outcome is kept")
	}
}

func TestSheetStaleAndRefused(t *testing.T) {
	f := amFixture(t)
	f.camp.EXPECT().SaveAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, gomock.Any()).Return(core.Aftermath{}, core.ErrAftermathChanged)
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathSheet, core.AftermathEdit), nil)
	if rec := amPost(f, "/sheet", sheetForm(nil)); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Somebody changed this aftermath") {
		t.Errorf("stale: %d", rec.Code)
	}
	f.camp.EXPECT().SaveAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, gomock.Any()).Return(core.Aftermath{}, core.Refuse("The sheet is locked."))
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathInjuries, core.AftermathEdit), nil)
	if rec := amPost(f, "/sheet", sheetForm(nil)); rec.Code != 422 || !strings.Contains(rec.Body.String(), "The sheet is locked.") {
		t.Errorf("refused: %d", rec.Code)
	}
	f.camp.EXPECT().SaveAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, gomock.Any()).Return(core.Aftermath{}, core.Refuse("Not your company."))
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathPending, core.AftermathNone), nil)
	if rec := amPost(f, "/sheet", sheetForm(nil)); rec.Code != http.StatusForbidden {
		t.Errorf("not an editor: %d", rec.Code)
	}
}

func TestReopenSheet(t *testing.T) {
	f := amFixture(t)
	f.camp.EXPECT().ReopenSheet(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2).Return(amState(core.AftermathSheet, core.AftermathEdit), nil)
	if rec := amPost(f, "/reopen-sheet", url.Values{"revision": {"2"}}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "The sheet is open again.") || !strings.Contains(rec.Body.String(), "Start aftermath") {
		t.Errorf("%d\n%s", rec.Code, rec.Body.String())
	}
}

// --- the Injuries step ---

func injuriesState(cas ...core.Casualty) core.Aftermath {
	a := amState(core.AftermathInjuries, core.AftermathEdit)
	a.Casualties, a.Rerolls = cas, 2
	return a
}

func TestInjuriesStepShowsRollsResultsAndDecisions(t *testing.T) {
	f := amFixture(t)
	rules := amRules
	rules.Warrior = []core.InjuryRow{{Min: 2, Max: 6, Key: "dead", Name: "Dead", Kind: core.CasualtyDead}, {Min: 7, Max: 12, Key: "ok", Name: "Full Recovery", Kind: core.CasualtyRecovered}}
	f2 := matchWrite(t, endedMatch())
	_ = f
	f = f2
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(rules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: amUnits()}, nil).AnyTimes()
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(injuriesState(
		core.Casualty{UnitID: 10, Name: "Aldor"},
		core.Casualty{UnitID: 11, Name: "Brand", Rolled: true, Dice: []int{1, 2}, Total: 3, ResultKey: "dead", ResultTxt: "Dead", Kind: core.CasualtyDead},
		core.Casualty{UnitID: 12, Name: "Cirion", Hero: true, Rolled: true, Dice: []int{3, 3}, Total: 6, ResultTxt: "Just a Scratch", Kind: core.CasualtyScratch,
			Decision: &core.CasualtyDecision{Kind: "scratch"}}), nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{`hx-post="/campaigns/7/matches/3/aftermath/1/roll"`, `name="unit" value="10"`, "Roll 2D6", "Dead", "goes to the cemetery", `class="die-face" data-n="2"`, `data-tone="danger"`, `data-tone="warn"`, `data-tip-chart="injury-chart-11"`,
		`name="kind" value="scratch"`, `value="reroll"`, "Re-roll · Against the Odds", "<strong>2</strong> re-roll", `aria-current="true"`} {
		if !strings.Contains(body, want) {
			t.Errorf("injuries step lacks %q", want)
		}
	}
	for id, pending := range map[string]bool{"casualty-10": true, "casualty-11": false, "casualty-12": true} { // to roll, nothing to do, a question
		i := strings.Index(body, `id="`+id+`"`)
		if open := strings.LastIndex(body[:i], "<article"); strings.Contains(body[open:i], "unit-pending") != pending {
			t.Errorf("%s: unit-pending should be %t", id, pending)
		}
	}
	if strings.Contains(body, "Roll all") || strings.Contains(body, `value="continue"`) {
		t.Error("one casualty is unrolled (no Roll all) and one question is open")
	}
}

func TestRollCasualty(t *testing.T) {
	f := amFixture(t)
	st := injuriesState(core.Casualty{UnitID: 10, Name: "Aldor", Rolled: true, Dice: []int{2, 3}, Total: 5, ResultTxt: "Dead", Kind: core.CasualtyDead})
	f.camp.EXPECT().RollCasualty(gomock.Any(), frodo, int64(7), int64(3), int64(1), int64(10), nil, false).Return(st, nil)
	rec := amPost(f, "/roll", url.Values{"unit": {"10"}, "roll": {"0"}, "roll2": {"0"}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-animate="true"`) {
		t.Errorf("the server rolled, so the dice are shown rolling: %d", rec.Code)
	}
	f.camp.EXPECT().RollCasualty(gomock.Any(), frodo, int64(7), int64(3), int64(1), int64(10), []int{2, 3}, true).Return(st, nil)
	rec = amPost(f, "/roll", url.Values{"unit": {"10"}, "roll": {"2"}, "roll2": {"3"}, "thrown": {"1"}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-animate="false"`) {
		t.Errorf("typed or thrown dice are not animated: %d", rec.Code)
	}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(injuriesState(core.Casualty{UnitID: 10, Name: "Aldor"}), nil)
	if rec := amPost(f, "/roll", url.Values{"unit": {"10"}, "roll": {"2"}, "roll2": {"0"}}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "all the dice") {
		t.Errorf("one typed die: %d", rec.Code)
	}
	f.camp.EXPECT().RollCasualty(gomock.Any(), frodo, int64(7), int64(3), int64(1), int64(10), []int{2, 3}, false).Return(core.Aftermath{}, core.Refuse("Players may not type their own rolls here."))
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(injuriesState(core.Casualty{UnitID: 10, Name: "Aldor"}), nil)
	if rec := amPost(f, "/roll", url.Values{"unit": {"10"}, "roll": {"2"}, "roll2": {"3"}}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "may not type") {
		t.Errorf("RollRefusal: %d", rec.Code)
	}
	f.camp.EXPECT().RollCasualty(gomock.Any(), frodo, int64(7), int64(3), int64(1), int64(0), nil, false).Return(st, nil)
	if rec := amPost(f, "/roll", url.Values{"unrolled": {"10,11"}}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-animate="true"`) {
		t.Errorf("roll all: %d", rec.Code)
	}
}

func TestRerollDecideAndContinue(t *testing.T) {
	f := amFixture(t)
	st := injuriesState(core.Casualty{UnitID: 10, Name: "Aldor", Rolled: true, Dice: []int{2, 3}, Total: 5, ResultTxt: "Injured", Kind: core.CasualtyInjured})
	f.camp.EXPECT().RerollCasualty(gomock.Any(), frodo, int64(7), int64(3), int64(1), int64(10), gomock.Any(), false).Return(st, nil)
	if rec := amPost(f, "/reroll", url.Values{"unit": {"10"}}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-animate="true"`) || !strings.Contains(rec.Body.String(), `name="kind" value="continue"`) {
		t.Errorf("reroll: %d (the Continue button appears once everything is rolled and re-rolls are left)", rec.Code)
	}
	f.camp.EXPECT().RerollCasualty(gomock.Any(), frodo, int64(7), int64(3), int64(1), int64(10), []int{4, 5}, false).Return(st, nil)
	if rec := amPost(f, "/reroll", url.Values{"unit": {"10"}, "roll": {"4"}, "roll2": {"5"}}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-animate="false"`) {
		t.Errorf("a typed re-roll is passed on and not animated: %d", rec.Code)
	}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	if rec := amPost(f, "/reroll", url.Values{"unit": {"10"}, "roll": {"4"}, "roll2": {"0"}}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "all the dice") {
		t.Errorf("one typed die: %d", rec.Code)
	}
	f.camp.EXPECT().Decide(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.AftermathAnswer{UnitID: 10, Kind: "bonus", Answer: "3"}).Return(st, nil)
	if rec := amPost(f, "/decide", url.Values{"kind": {"bonus"}, "unit": {"10"}, "answer": {"3"}}); rec.Code != 200 {
		t.Errorf("bonus die: %d", rec.Code)
	}
	f.camp.EXPECT().Decide(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.AftermathAnswer{UnitID: 10, Kind: "scratch", Answer: "keep"}).Return(st, nil)
	if rec := amPost(f, "/decide", url.Values{"kind": {"scratch"}, "unit": {"10"}, "answer": {"keep"}}); rec.Code != 200 {
		t.Errorf("decide: %d", rec.Code)
	}
	f.camp.EXPECT().Decide(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.AftermathAnswer{Kind: "continue"}).Return(amState(core.AftermathLevelUps, core.AftermathEdit), nil)
	if rec := amPost(f, "/decide", url.Values{"kind": {"continue"}}); rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="aftermath-step-3"`) {
		t.Errorf("continue: %d (a reward choice is open: the Rewards step)", rec.Code)
	}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	if rec := amPost(f, "/decide", url.Values{"kind": {"scratch"}, "unit": {"10"}}); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose an answer") {
		t.Errorf("no answer: %d", rec.Code)
	}
}

func TestSuccessionQuestions(t *testing.T) {
	f := amFixture(t)
	st := injuriesState(core.Casualty{UnitID: 10, Name: "Aldor", Rolled: true, Dice: []int{1, 1}, Total: 2, ResultTxt: "Dead", Kind: core.CasualtyDead, Final: true})
	st.Succession = &core.Succession{NeedLeader: true, LeaderOptions: []int64{11}}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{`name="kind" value="succession"`, `name="answer" value="11"`, "Which one takes over as Leader"} {
		if !strings.Contains(body, want) {
			t.Errorf("succession lacks %q", want)
		}
	}
	f.camp.EXPECT().Decide(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.AftermathAnswer{Kind: "sergeant", Answer: ""}).Return(st, nil)
	if rec := amPost(f, "/decide", url.Values{"kind": {"sergeant"}}); rec.Code != 200 {
		t.Errorf("skipping the sergeant: %d", rec.Code)
	}
}

// Every question is answered by one press: the options are the buttons, there is nothing to tick and confirm.
func TestDecisionsAnswerWithOneClick(t *testing.T) {
	dead := core.Casualty{UnitID: 10, Name: "Aldor", Rolled: true, Dice: []int{1, 1}, Total: 2, ResultTxt: "Dead", Kind: core.CasualtyDead, Final: true}
	decided := func(d core.CasualtyDecision) core.Aftermath {
		return injuriesState(core.Casualty{UnitID: 10, Name: "Aldor", Hero: true, Rolled: true, Dice: []int{3, 3}, Total: 6, ResultTxt: "Result", Decision: &d})
	}
	succession := func(s core.Succession) core.Aftermath {
		a := injuriesState(dead)
		a.Succession = &s
		return a
	}
	for _, tc := range []struct {
		name  string
		state core.Aftermath
		want  []string
	}{
		{"scratch", decided(core.CasualtyDecision{Kind: "scratch"}), []string{`name="answer" value="reroll"`, `name="answer" value="keep"`}},
		{"heal", decided(core.CasualtyDecision{Kind: "heal", Options: []core.DecisionOption{{Key: "armWound", Label: "Heal the Arm Wound"}}}), []string{`name="answer" value="armWound"`, `name="answer" value=""`}},
		{"bonus", decided(core.CasualtyDecision{Kind: "bonus"}), []string{`name="answer" value="0"`}},
		{"leader", succession(core.Succession{NeedLeader: true, LeaderOptions: []int64{11}}), []string{`name="answer" value="11"`}},
		{"sergeant", succession(core.Succession{Sergeants: 2, SergeantOptions: []int64{11}}), []string{`name="answer" value="11"`, `name="answer" value=""`}},
		{"fallback", succession(core.Succession{NeedLeader: true, Fallback: []int64{11, 12}}), []string{`name="answer" value="11"`, `name="answer" value="12"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := amFixture(t)
			f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(tc.state, nil)
			body := f.do(f.signedIn(httpGet(amPath))).Body.String()
			for _, want := range tc.want {
				if !strings.Contains(body, "<button "+want) {
					t.Errorf("no button %q", want)
				}
			}
			for _, gone := range []string{`type="radio"`, ">Decide</button>", ">Choose</button>", `id="bonus-`} {
				if strings.Contains(body, gone) {
					t.Errorf("a two-step control is left: %q", gone)
				}
			}
		})
	}
}

// With typed dice the player still gives the dice of the roll and the re-roll: these are inputs, not answers.
func TestTypedDiceStayInputs(t *testing.T) {
	f := rollSetup(t, endedMatch(), core.CampaignSettings{ManualRolls: true})
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(amRules, nil).AnyTimes()
	st := injuriesState(core.Casualty{UnitID: 10, Name: "Aldor"}, core.Casualty{UnitID: 11, Name: "Brand", Rolled: true, Dice: []int{4, 4}, Total: 8, ResultTxt: "Full Recovery", Kind: core.CasualtyRecovered})
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{`<select id="d1-10" name="roll"`, `<select id="d2-10" name="roll2"`, `<select id="rr1-11" name="roll"`, `<select id="rr2-11" name="roll2"`, "Roll for me", "I rolled a 6"} {
		if !strings.Contains(body, want) {
			t.Errorf("the typed dice lack %q", want)
		}
	}
}

// A casualty and a candidate show who they are at a glance; a casualty already buried keeps its card, from the cemetery.
func TestUnitCardsTellWhoTheUnitIs(t *testing.T) {
	sergeant := core.CompanyUnit{ID: 11, Name: "Brand", Type: "Captain of Rohan", Role: core.RoleSergeant, Points: 57, Level: 2, XP: 13, LevelEvery: 5, Path: &core.UnitPath{Name: "Path of the Warrior"},
		Injuries: []core.UnitInjury{{Key: "armWound", Name: "Arm Wound", Count: 2}}, Stats: []core.Attribute{{Abbr: "Fi", Name: "Fight", Value: 4, Trail: "/4+"}, {Name: "Might", Value: 3, HeroOnly: true}}}
	fallen := core.CompanyUnit{ID: 10, Type: "Captain of Rohan", Role: core.RoleLeader, Points: 83}
	f := matchWrite(t, endedMatch())
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(amRules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: []core.CompanyUnit{sergeant}, Cemetery: []core.FallenUnit{{Unit: fallen, Fate: "Dead"}}}, nil).AnyTimes()
	st := injuriesState(core.Casualty{UnitID: 10, Name: "Aldor (Leader)", Hero: true, Rolled: true, Dice: []int{1, 1}, Total: 2, ResultTxt: "Dead", Kind: core.CasualtyDead, Final: true})
	st.Succession = &core.Succession{NeedLeader: true, LeaderOptions: []int64{11}}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{"Brand (Sergeant)", "Captain of Rohan", `<span class="font-semibold text-fg">57</span> pts`, ">Sergeant</span>", "path-k-", "Arm Wound ×2", "unit-sergeant", "data-hero-inline", `<span class="sr-only">Might</span>`, `class="xp-bar" style="--seg: 5"`, `aria-valuenow="3"`, "3 / 5 XP", // the candidate
		"Aldor (Leader)", `<span class="font-semibold text-fg">83</span> pts`, "unit-leader"} { // the buried casualty
		if !strings.Contains(body, want) {
			t.Errorf("the cards lack %q", want)
		}
	}
	if strings.Contains(body, "statline") || strings.Contains(body, "<dt>Fi</dt>") {
		t.Error("the aftermath shows Might, Will and Fate in one line, and no profile")
	}
}

// While the casualties are rolled the rest of the company follows them, each with what it earned; afterwards it is everybody, the casualties first.
func TestEveryUnitShowsWhatTheMatchDidToIt(t *testing.T) {
	rules := amRules
	rules.Gains = append(slices.Clone(rules.Gains), core.XPGain{Key: core.GainObjective, Label: "Carrying an objective", Amount: 1})
	roster := append(amUnits(), core.CompanyUnit{ID: 13, Name: "Dernhelm", Type: "Warrior of Rohan"}, core.CompanyUnit{ID: 14, Name: "Elfhelm", Type: "Warrior of Rohan"})
	serve := func(t *testing.T, a core.Aftermath) string {
		f := matchWrite(t, endedMatch())
		f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(rules, nil).AnyTimes()
		f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: roster}, nil).AnyTimes()
		f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(a, nil)
		return f.do(f.signedIn(httpGet(amPath))).Body.String()
	}
	state := func(status core.AftermathStatus) core.Aftermath {
		a := amState(status, core.AftermathEdit)
		a.Casualties = []core.Casualty{{UnitID: 11, Name: "Brand", Rolled: true, Dice: []int{6, 6}, Total: 12, ResultTxt: "Dead", Kind: core.CasualtyDead, Final: true}}
		a.Input.Models = []core.ModelSheet{
			{UnitID: 14, Outcome: core.OutcomeSurvived, Gains: map[string]int{}},
			{UnitID: 13, Outcome: core.OutcomeFled, Gains: map[string]int{"participated": 1, "slain": 2, core.GainObjective: 1}},
			{UnitID: 11, Outcome: core.OutcomeCasualty, Gains: map[string]int{"participated": 1}},
			{UnitID: 10, Outcome: core.OutcomeSurvived, Gains: map[string]int{"participated": 1}},
		}
		a.Preview.Experience = []core.UnitGain{{UnitID: 13, Total: 4}, {UnitID: 11, Total: 1}, {UnitID: 10, Total: 1}}
		return a
	}
	result := func(t *testing.T, body string, id int) string {
		t.Helper()
		_, after, ok := strings.Cut(body, fmt.Sprintf(`data-unit-result="%d"`, id))
		if !ok {
			t.Fatalf("no result card of unit %d", id)
		}
		card, _, _ := strings.Cut(after, "</article>")
		return card
	}

	body := serve(t, state(core.AftermathInjuries))
	if strings.Contains(body, `data-unit-result="11"`) {
		t.Error("while rolling, a casualty has its own card only")
	}
	if !strings.Contains(body, `id="casualty-11"`) || strings.Index(body, `id="casualty-11"`) > strings.Index(body, `data-unit-result="13"`) {
		t.Error("the casualty comes before the others")
	}
	if strings.Index(body, `data-unit-result="10"`) > strings.Index(body, `data-unit-result="13"`) {
		t.Error("the units follow the company's order, not the order of the sheet")
	}
	fled := result(t, body, 13)
	for _, want := range []string{"Fled", `xp-match-text">+4<`, "Objective", "Slain", `class="milestone-count"`, ">2</span>"} {
		if !strings.Contains(fled, want) {
			t.Errorf("the unit that fled lacks %q:\n%s", want, fled)
		}
	}
	if survived := result(t, body, 10); !strings.Contains(survived, "Survived") || !strings.Contains(survived, `xp-match-text">+1<`) || strings.Contains(survived, `data-on="true"`) || strings.Count(survived, `data-on="false"`) != 3 {
		t.Errorf("a unit that earned no milestone still shows them all, unlit:\n%s", survived)
	}
	if strings.Contains(body, "<button type=\"button\" class=\"milestone\"") {
		t.Error("the milestones of the results cannot be pressed")
	}
	absent := result(t, body, 12)
	if !strings.Contains(absent, "Missed this game") || !strings.Contains(absent, "Recovered: can play the next match") {
		t.Errorf("the unit that missed the game:\n%s", absent)
	}
	if strings.Index(body, `data-unit-result="12"`) < strings.Index(body, `data-unit-result="14"`) || strings.Index(body, `data-unit-result="14"`) < strings.Index(body, `data-unit-result="10"`) {
		t.Error("who did not play comes last")
	}
	if c := result(t, body, 14); !strings.Contains(c, "Did not take part") || !strings.Contains(c, "Can play the next match") {
		t.Errorf("the unit that stayed out:\n%s", c)
	}

	done := state(core.AftermathDone)
	done.Lines = []core.AftermathLine{{Kind: "experience", UnitID: 13, Amount: 5}}
	body = serve(t, done)
	if strings.Index(body, `data-unit-result="11"`) > strings.Index(body, `data-unit-result="13"`) || !strings.Contains(body, `data-unit-result="13"`) {
		t.Error("afterwards the casualties come first")
	}
	if c := result(t, body, 11); !strings.Contains(c, "Dead") || strings.Contains(c, "xp-match-text") {
		t.Errorf("the fallen unit:\n%s", c)
	}
	if c := result(t, body, 13); !strings.Contains(c, `xp-match-text">+5<`) {
		t.Errorf("the experience given is the record's:\n%s", c)
	}

	waived := state(core.AftermathWaived)
	if strings.Contains(serve(t, waived), "data-unit-result") {
		t.Error("a waived aftermath did nothing to anybody")
	}
}

// A save of the sheet answers the experience of every unit too: what the match gives it, over what it had. Given, the bar draws it the same way.
func TestTheSheetSaveUpdatesTheExperienceOfEachUnit(t *testing.T) {
	roster := amUnits()
	roster[0].XP, roster[0].LevelEvery = 6, 5
	f := matchWrite(t, endedMatch())
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(amRules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: roster}, nil).AnyTimes()
	a := amState(core.AftermathSheet, core.AftermathEdit)
	a.Preview.Experience = []core.UnitGain{{UnitID: 10, Total: 3}}
	f.camp.EXPECT().SaveAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, gomock.Any()).Return(a, nil)
	body := amPost(f, "/sheet?auto=1", sheetForm(nil)).Body.String()
	for _, want := range []string{`id="aftermath-bar"`, `id="sheet-xp-10" hx-swap-oob="true"`, `id="sheet-xp-11" hx-swap-oob="true"`, `xp-match-text" title="Earned this match: added in the aftermath">+3<`, "6 / 5 XP"} {
		if !strings.Contains(body, want) {
			t.Errorf("the save lacks %q:\n%s", want, body)
		}
	}
	v := aftermathView{A: core.Aftermath{Status: core.AftermathDone, Lines: []core.AftermathLine{{Kind: "experience", UnitID: 10, Amount: 3}}}}
	if f := v.Foot(roster[0]); f.MatchXP != 3 || f.U.XP != 3 {
		t.Errorf("experience already given: %+v", f)
	}
}

// The sheet draws each gain as a milestone of one press, Fate only for a unit that has Fate, and repeatable gains count.
func TestSheetOffersMilestonesOfOnePress(t *testing.T) {
	v := aftermathView{Rules: core.AftermathRules{Gains: []core.XPGain{{Key: "participated", Label: "Took part"}, {Key: core.GainWoundedEnemy, Label: "Wounding at least one enemy"}, {Key: core.GainFatePrevented, Label: "Fate"}, {Key: "custom", Label: "A gain of a custom ruleset"}}}}
	warrior := core.CompanyUnit{ID: 1}
	got := v.SheetMilestones(warrior, core.ModelSheet{Gains: map[string]int{core.GainWoundedEnemy: 1}})
	if len(got) != 2 || got[0].M.Label != "Wounded" || got[0].M.Tip != "Wounding at least one enemy" || got[0].Count != 1 || got[1].M.Icon != gainLookOther || got[1].M.Label != "A gain of a custom ruleset" {
		t.Errorf("a warrior: %+v", got)
	}
	if got := v.SheetMilestones(warrior, core.ModelSheet{Gains: map[string]int{core.GainFatePrevented: 1}}); len(got) != 3 {
		t.Errorf("a Fate the sheet already gives stays: %+v", got)
	}
	hero := core.CompanyUnit{ID: 2, Stats: []core.Attribute{{Name: "Fate", Value: 1, HeroOnly: true}}}
	if !hero.HasHeroStats() {
		t.Fatal("the test's hero has no hero stats")
	}
	if got := v.SheetMilestones(hero, core.ModelSheet{}); len(got) != 3 {
		t.Errorf("a hero: %+v", got)
	}
}

func TestNobodyLeftToLeadIsSaid(t *testing.T) {
	f := amFixture(t)
	st := injuriesState(core.Casualty{UnitID: 10, Name: "Aldor", Rolled: true, Dice: []int{1, 1}, Total: 2, ResultTxt: "Dead", Kind: core.CasualtyDead, Final: true})
	st.NoOneToLead = true
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	if body := f.do(f.signedIn(httpGet(amPath))).Body.String(); !strings.Contains(body, "Nobody is left to lead the company") {
		t.Error("the page does not say that nobody is left to lead")
	}
}

// --- the Level-ups and Finish steps ---

func ownerFixture(t *testing.T, units []core.CompanyUnit) *fixture {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = 99
	d.Companies = []core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(endedMatch(), nil).AnyTimes()
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(amRules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: units}, nil).AnyTimes()
	return f
}

func levelUnits() []core.CompanyUnit {
	us := amUnits()
	us[0].XP, us[0].LevelEvery = 12, 5
	return us
}

func TestLevelUpsStepListsWaitingRollsAndOpensTheWindow(t *testing.T) {
	f := ownerFixture(t, levelUnits())
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathLevelUps, core.AftermathEdit), nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{"Aldor", "roll", `hx-get="/campaigns/7/my-company/1/units/10/levelup"`, `id="modal-body"`, `data-step="Level-ups"`, `data-step="Rewards"`} {
		if !strings.Contains(body, want) {
			t.Errorf("level-ups step lacks %q", want)
		}
	}
}

func TestLevelUpsStepForAReaderHasNoContinueButtonAndTheSummaryCountsWaitingRolls(t *testing.T) {
	f := ownerFixture(t, levelUnits())
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathLevelUps, core.AftermathNone), nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	if strings.Contains(body, "Finish aftermath") || strings.Contains(body, "Start aftermath") {
		t.Error("a reader gets no button to move the aftermath on")
	}
	g := ownerFixture(t, levelUnits())
	g.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathDone, core.AftermathEdit), nil)
	if body := g.do(g.signedIn(httpGet(amPath))).Body.String(); !strings.Contains(body, "1 level-up ready") {
		t.Errorf("the finished summary must say that a roll is waiting")
	}
}

// A finished aftermath has no stepper and no step cards: the summary, the counts, two collapsed details and the way out; level-ups waiting and
// the Injuries block stay in view, in that order, above the company.
func TestFinishedAftermathIsCompact(t *testing.T) {
	serve := func(st core.Aftermath) string {
		f := ownerFixture(t, levelUnits())
		f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
		return f.do(f.signedIn(httpGet(amPath))).Body.String()
	}
	gone := func(t *testing.T, body string) {
		t.Helper()
		for _, not := range []string{`aria-label="Steps"`, `data-step=`, `id="aftermath-sheet"`, "1. Sheet", "!text-xl\">Done<"} {
			if strings.Contains(body, not) {
				t.Errorf("a finished aftermath has no %q", not)
			}
		}
		for _, want := range []string{"data-sheet-counts>2 took part · 1 casualty<", "Everything that changed", "The sheet", ">Back to the match<", `id="modal-body"`} {
			if !strings.Contains(body, want) {
				t.Errorf("a finished aftermath lacks %q", want)
			}
		}
		if strings.LastIndex(body, "</details>") > strings.Index(body, "data-done-links") {
			t.Error("the links stay outside the collapsed details")
		}
		if strings.Contains(body, "data-done-changes open") || strings.Contains(body, "data-done-sheet open") {
			t.Error("the details start collapsed")
		}
	}

	done := amState(core.AftermathDone, core.AftermathEdit)
	done.Casualties = []core.Casualty{{UnitID: 11, Name: "Boromir", ResultKey: "injured", ResultTxt: "Injured", Rolled: true, Final: true}}
	body := serve(done)
	gone(t, body)
	for _, want := range []string{"data-change-summary>1 level-up ready<", ">My Company<", `/my-company/1/units/10/levelup"`, "2 rolls waiting", "Boromir"} {
		if !strings.Contains(body, want) {
			t.Errorf("the finished page lacks %q\n%s", want, body)
		}
	}
	top, levels, injuries, company := strings.Index(body, `id="aftermath-done"`), strings.Index(body, `id="aftermath-step-2"`), strings.Index(body, `id="aftermath-step-1"`), strings.Index(body, `id="aftermath-company"`)
	if top < 0 || !(top < levels && levels < injuries && injuries < company) {
		t.Errorf("the order is summary, Level-ups, Injuries, company: %d %d %d %d", top, levels, injuries, company)
	}

	reader := done
	reader.Access = core.AftermathNone
	body = serve(reader)
	gone(t, body)
	if !strings.Contains(body, "You can read this aftermath but not change it.") {
		t.Errorf("a reader is told it is read only:\n%s", body)
	}

	waived := amState(core.AftermathWaived, core.AftermathEdit)
	waived.WaivedNote = "The player is gone"
	body = serve(waived)
	for _, want := range []string{"Waived: nothing was applied.", "data-waived-note>The player is gone<", "The sheet", ">Back to the match<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the waived page lacks %q", want)
		}
	}
	for _, not := range []string{"data-change-summary", "Everything that changed", `id="aftermath-step-2"`, `aria-label="Steps"`} {
		if strings.Contains(body, not) {
			t.Errorf("the waived page has %q", not)
		}
	}
}

func TestFinishStepOffersFreeEquipmentAndRefusesWhileOpen(t *testing.T) {
	f := ownerFixture(t, amUnits())
	st := amState(core.AftermathLevelUps, core.AftermathEdit)
	st.Preview.Rewards = []core.RewardApply{{Key: "spoils", Name: "To the Victor the Spoils", Kind: "freeEquipment", MaxValue: 2, Decision: true}, {Key: "cargo", Name: "Cargo", Kind: "influence", Amount: 1}}
	st.Pending = st.Preview.Rewards[:1]
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	f.camp.EXPECT().FreeEquipmentOffers(gomock.Any(), int64(7), int64(1), 2).Return([]core.FreeEquipment{{GearOffer: core.GearOffer{Name: "Shield", Cost: 1, Size: "small", Description: itemWarSpear}, Units: []core.UnitOption{{ID: 10, Title: "Aldor"}}}}, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(shippedRules(t).GlossaryWithUnits(), nil).AnyTimes()
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	if got := strings.Join(linkTextsIn(body, "while mounted and a"), ","); got != "lance,spear" {
		t.Errorf("the offered item's description links: %q", got)
	}
	for _, want := range []string{"To the Victor the Spoils", "Choose one item worth at most 2 Influence", `value="Shield"`, `name="answer" value="Shield|10"`, `name="key" value="spoils"`, "The aftermath finishes as soon as the choice is made.", "+1 Influence"} {
		if !strings.Contains(body, want) {
			t.Errorf("finish lacks %q", want)
		}
	}

	f.camp.EXPECT().Decide(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.AftermathAnswer{Kind: "reward", Key: "spoils", Answer: "Shield|10"}).Return(st, nil)
	f.camp.EXPECT().FreeEquipmentOffers(gomock.Any(), int64(7), int64(1), 2).Return(nil, nil)
	if rec := amPost(f, "/decide", url.Values{"kind": {"reward"}, "key": {"spoils"}, "answer": {"Shield|10"}}); rec.Code != 200 {
		t.Errorf("reward decision: %d", rec.Code)
	}

	f.camp.EXPECT().FinishAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2).Return(core.Aftermath{}, core.Refuse("Choose the free equipment first."))
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	f.camp.EXPECT().FreeEquipmentOffers(gomock.Any(), int64(7), int64(1), 2).Return(nil, nil)
	rec := amPost(f, "/finish", url.Values{"revision": {"2"}})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose the free equipment first.") {
		t.Errorf("refused: %d", rec.Code)
	}
}

func TestFinishedAftermathShowsTheSummaryToEveryone(t *testing.T) {
	f := ownerFixture(t, amUnits())
	st := amState(core.AftermathDone, core.AftermathEdit)
	st.Lines = []core.AftermathLine{{Kind: "influence", Text: "+2 Influence for a Win", Amount: 2}, {Kind: "experience", Text: "Aldor: +3 Experience", UnitID: 10, Amount: 3}}
	f.camp.EXPECT().FinishAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2).Return(st, nil)
	rec := amPost(f, "/finish", url.Values{"revision": {"2"}})
	body := rec.Body.String()
	for _, want := range []string{"Aftermath finished.", "The company", "&#43;2 Influence for a Win", "Aldor: &#43;3 Experience", `href="/campaigns/7/my-company/1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("summary lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Finish aftermath</button>") {
		t.Error("a finished aftermath has no Finish button")
	}
}

// --- My Company: the banner and the lock ---

func myCompanyWith(t *testing.T, refs []core.AftermathRef) string {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Aftermaths = refs
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	return f.do(f.signedIn(httpGet("/campaigns/7/my-company/1"))).Body.String()
}

func TestMyCompanyBannerAndLock(t *testing.T) {
	body := myCompanyWith(t, []core.AftermathRef{{MatchID: 3, MatchNumber: 3, CompanyID: 1, Status: core.AftermathPending}, {MatchID: 4, MatchNumber: 4, CompanyID: 1, Status: core.AftermathLevelUps}})
	for _, want := range []string{`id="aftermath-banner"`, "/campaigns/7/matches/3/aftermath/1", "Match 3: Start the aftermath", "Match 4: Continue the aftermath",
		`data-lock="Finish the aftermaths of Matches 3 and 4 first."`, `@click.capture="lockClick($event)"`, `data-lock class="btn btn-primary"`} {
		if !strings.Contains(body, want) {
			t.Errorf("a locked company lacks %q", want)
		}
	}
	body = myCompanyWith(t, nil)
	if strings.Contains(body, "aftermath-banner") || !strings.Contains(body, `data-lock=""`) {
		t.Error("no banner and an empty lock reason without an aftermath")
	}
}

// --- perks and history ---

func TestMyCompanyShowsPerksAndHistoryAndMarksOneUsed(t *testing.T) {
	used := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Perks = []core.Perk{{ID: 5, MatchID: 3, Name: "Rendezvous", Text: "+1 to the first reinforcement roll", When: "nextReinforcementRoll"},
		{ID: 6, MatchID: 2, Name: "Vantage Point", UsedOn: &used}}
	detail.History = []core.AftermathLine{{Kind: "influence", Text: "+2 Influence for a Win", Amount: 2, Match: 4, MatchID: 44, On: time.Date(2026, 10, 3, 20, 0, 0, 0, time.UTC)}, {Kind: "experience", Text: "Aldor: +1 Experience", Match: 4, MatchID: 44, On: time.Date(2026, 10, 3, 20, 0, 0, 0, time.UTC)}, {Kind: "injury", Text: "Brand: Injured", Match: 3}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	body := f.do(f.signedIn(httpGet("/campaigns/7/my-company/1"))).Body.String()
	for _, want := range []string{`id="perks"`, "Rendezvous", "On the next reinforcement roll", `hx-post="/campaigns/7/my-company/1/perks/5/use"`, "Used (1)", "Vantage Point", "1 October 2026", `id="aftermath-history"`, "Influence for a Win", `<a class="underline" href="/campaigns/7/matches/44">Match 4</a>`, "· 3 Oct 2026", "<h3 class=\"!text-base\">Match 3</h3>"} {
		if !strings.Contains(body, want) {
			t.Errorf("My Company lacks %q", want)
		}
	}
	if strings.Contains(body, "perks/6/use") {
		t.Error("a used perk has no button")
	}
	f.camp.EXPECT().UsePerk(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/perks/5/use", nil)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Perk marked as used.") {
		t.Errorf("mark used: %d", rec.Code)
	}
	f.camp.EXPECT().UsePerk(gomock.Any(), frodo, int64(7), int64(1), int64(6)).Return(core.Refuse("Only the owner of the company can use its perks."))
	if rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/perks/6/use", nil))); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Only the owner") {
		t.Errorf("refused: %d", rec.Code)
	}
}

func TestPerksCardEmptyStateAndOthersCompany(t *testing.T) {
	body := myCompanyWith(t, nil)
	if !strings.Contains(body, "No perks waiting") || !strings.Contains(body, "Nothing yet. What the War") {
		t.Error("empty states of the perks and history cards")
	}
}

func TestInjuriesSummaryNotesWhatTheResultDid(t *testing.T) {
	v := aftermathView{Rules: core.AftermathRules{Hero: []core.InjuryRow{{Key: "armWound", Name: "Arm Wound"}}}}
	for _, c := range []struct {
		cas  core.Casualty
		want string
	}{
		{core.Casualty{Kind: core.CasualtyDead}, "goes to the cemetery"},
		{core.Casualty{Kind: core.CasualtyRetired, Count: 2}, "forced to retire by its injury number 2, goes to the cemetery"},
		{core.Casualty{Kind: core.CasualtyInjured}, "misses the next game"},
		{core.Casualty{Kind: core.CasualtyHeal, InjuryKey: "armWound"}, "healed one Arm Wound"},
		{core.Casualty{Kind: core.CasualtyHeal}, "nothing healed"},
		{core.Casualty{Kind: core.CasualtyRecovered}, ""},
	} {
		if got := v.Note(c.cas); got != c.want {
			t.Errorf("%s: %q, want %q", c.cas.Kind, got, c.want)
		}
	}
}

func TestSheetPreviewListsTheScenarioRewards(t *testing.T) {
	f := amFixture(t)
	st := amState(core.AftermathPending, core.AftermathEdit)
	st.Preview.Rewards = []core.RewardApply{{Key: "survivalOfTheFittest", Name: "Survival of the Fittest", Kind: "experience", Amount: 1, UnitID: 10}, {Key: "cargo", Name: "Cargo", Kind: "influence", Amount: 1}, {Key: "spoils", Name: "To the Victor", Kind: "freeEquipment", MaxValue: 2, Decision: true}}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{"Survival of the Fittest: &#43;1 Experience to Aldor", "Cargo: &#43;1 Influence", "To the Victor: a free item worth up to 2 Influence, chosen at Finish"} {
		if !strings.Contains(body, want) {
			t.Errorf("the preview lacks %q", want)
		}
	}
}

// bonusRules is amRules with Lesson Learned (a D3 on the warrior chart) and Wounds of a Hero (a D6 on the hero chart), as the rules file has them.
var bonusRules = func() core.AftermathRules {
	r := amRules
	r.Warrior = []core.InjuryRow{{Min: 2, Max: 12, Key: "lessonLearned", Name: "Lesson Learned", Kind: core.CasualtyRecovered, BonusDice: "d3", BonusSpec: core.NdM(1, 3), BonusKind: "experience"}}
	r.Hero = []core.InjuryRow{{Min: 2, Max: 12, Key: "woundsOfAHero", Name: "Wounds of a Hero", Kind: core.CasualtyRecovered, BonusDice: "d6", BonusSpec: core.NdM(1, 6), BonusKind: "influence"}}
	return r
}()

// bonusFixture is the aftermath page of a campaign with the settings, whose rules have the two bonus dice.
func bonusFixture(t *testing.T, settings core.CampaignSettings) *fixture {
	t.Helper()
	f := rollSetup(t, endedMatch(), settings)
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(bonusRules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: amUnits()}, nil).AnyTimes()
	return f
}

// bonusPage renders the Injuries step with the casualty's bonus question open.
func bonusPage(t *testing.T, settings core.CampaignSettings, c core.Casualty) string {
	t.Helper()
	f := bonusFixture(t, settings)
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(injuriesState(c), nil)
	return f.do(f.signedIn(httpGet(amPath))).Body.String()
}

func lessonLearned(decision *core.CasualtyDecision) core.Casualty {
	return core.Casualty{UnitID: 10, Name: "Aldor", Rolled: true, Dice: []int{6, 6}, Total: 12, ResultKey: "lessonLearned", ResultTxt: "Lesson Learned", Kind: core.CasualtyRecovered, Decision: decision}
}

func TestBonusDieIsAskedForWhenDiceAreTyped(t *testing.T) {
	body := bonusPage(t, core.CampaignSettings{ManualRolls: true}, lessonLearned(&core.CasualtyDecision{Kind: "bonus"}))
	for _, want := range []string{`name="kind" value="bonus"`, `name="answer" value="0"`, `name="answer" value="6"`, "I rolled a ", "Roll D3", "On a D3 a die of 1-2 reads 1"} {
		if !strings.Contains(body, want) {
			t.Errorf("the bonus question lacks %q", want)
		}
	}
	if strings.Contains(body, "throwBonus") {
		t.Error("without Shake the table the browser throws nothing")
	}
}

// With Shake the table the bonus die is a roll panel: the dice of the rules file are thrown in the browser and sent as thrown=1.
func TestBonusDieIsThrownWhereTheTableShakes(t *testing.T) {
	body := bonusPage(t, core.CampaignSettings{ShakeTheTable: true}, lessonLearned(&core.CasualtyDecision{Kind: "bonus"}))
	for _, want := range []string{`x-ref="bonusForm" data-sides="3" data-count="1"`, `name="thrown"`, `x-ref="bonusAnswer"`, "throwBonus()", "Roll D3", "space bar"} {
		if !strings.Contains(body, want) {
			t.Errorf("the thrown bonus die lacks %q", want)
		}
	}
	if strings.Contains(body, `name="answer" value="6"`) || strings.Contains(body, "I rolled a ") {
		t.Error("typed number buttons are for ManualRolls only")
	}
	hero := bonusPage(t, core.CampaignSettings{ShakeTheTable: true, ManualRolls: true}, core.Casualty{UnitID: 10, Name: "Aldor", Hero: true, Rolled: true, Dice: []int{6, 6}, Total: 12,
		ResultKey: "woundsOfAHero", ResultTxt: "Wounds of a Hero", Kind: core.CasualtyRecovered, Decision: &core.CasualtyDecision{Kind: "bonus"}})
	for _, want := range []string{`data-sides="6" data-count="1"`, "Roll D6", `name="answer" value="6"`} {
		if !strings.Contains(hero, want) {
			t.Errorf("Wounds of a Hero (a D6) through the same panel lacks %q", want)
		}
	}
	if strings.Contains(hero, "reads") {
		t.Error("a D6 is typed as it is: no folding hint")
	}
}

func TestBonusDieThrownIsPassedOnAndTheServersIsActedOut(t *testing.T) {
	f := bonusFixture(t, core.CampaignSettings{ShakeTheTable: true})
	done := lessonLearned(nil)
	done.Bonus = 2
	st := injuriesState(done)
	f.camp.EXPECT().Decide(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.AftermathAnswer{UnitID: 10, Kind: "bonus", Answer: "2", Thrown: true}).Return(st, nil)
	body := amPost(f, "/decide", url.Values{"kind": {"bonus"}, "unit": {"10"}, "answer": {"2"}, "thrown": {"1"}}).Body.String()
	if strings.Contains(body, `data-animate="true"`) {
		t.Error("a die the player threw is not acted out again")
	}
	f.camp.EXPECT().Decide(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.AftermathAnswer{UnitID: 10, Kind: "bonus", Answer: "0"}).Return(st, nil)
	body = amPost(f, "/decide", url.Values{"kind": {"bonus"}, "unit": {"10"}, "answer": {"0"}, "thrown": {""}}).Body.String()
	for _, want := range []string{`data-dice="2"`, `data-sides="3"`, `data-animate="true"`, `x-show="revealed" x-cloak><span class="badge badge-gold">`} {
		if !strings.Contains(body, want) {
			t.Errorf("the server's bonus die is not acted out: lacks %q", want)
		}
	}
	// the thrown flag means nothing for the other questions
	f.camp.EXPECT().Decide(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.AftermathAnswer{UnitID: 10, Kind: "scratch", Answer: "keep"}).Return(st, nil)
	amPost(f, "/decide", url.Values{"kind": {"scratch"}, "unit": {"10"}, "answer": {"keep"}, "thrown": {"1"}})
}

// --- Healing Herbs and Wondrous Cram (36y.5) ---

var itemRules = func() core.AftermathRules {
	r := amRules
	r.Warrior = []core.InjuryRow{{Min: 2, Max: 6, Key: "dead", Name: "Dead", Kind: core.CasualtyDead}, {Min: 7, Max: 12, Key: "fullRecovery", Name: "Full Recovery", Kind: core.CasualtyRecovered}}
	r.ItemEffects = map[string]string{"healing herbs": "healingHerbs", "wondrous cram": "wondrousCram"}
	r.Effects = map[string]core.RewardEffect{
		"healingHerbs": {Key: "healingHerbs", Kind: "injuryRollModifier", Modifier: 1, ClampMin: 2, ClampMax: 12, Carrier: "hero", Requires: []string{"participated", "notCasualty"}, Text: "Adds 1 to injury rolls."},
		"wondrousCram": {Key: "wondrousCram", Kind: "injuryRollReplacement", NoRoll: true, Result: "fullRecovery", Requires: []string{"participated", "casualty"}, Text: "Counts as Full Recovery."}}
	return r
}()

func itemFixture(t *testing.T, a core.Aftermath) *fixture {
	f := matchWrite(t, endedMatch())
	units := []core.CompanyUnit{
		{ID: 10, Name: "Aldor", Type: "Captain", Role: core.RoleLeader, Equipment: []core.Equipment{{Name: "Healing Herbs"}, {Name: "Wondrous Cram"}}},
		{ID: 11, Name: "Brand", Type: "Warrior of Rohan", Role: core.RoleWarrior, Equipment: []core.Equipment{{Name: "Healing Herbs"}, {Name: "Wondrous Cram"}}},
		{ID: 12, Name: "Cirion", Type: "Warrior of Rohan", Injured: true}}
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(itemRules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: units}, nil).AnyTimes()
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(a, nil)
	return f
}

func TestSheetOffersTheItemsACarrierCanUse(t *testing.T) {
	a := amState(core.AftermathPending, core.AftermathEdit)
	a.FromMatch = true
	a.Input.Models[0].UsesItem = "healingHerbs"
	a.Preview.Items = []core.AmountLine{{Text: "Healing Herbs: +1 to every injury roll"}}
	f := itemFixture(t, a)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{`name="item-10"`, `name="item-11"`, "Use Healing Herbs (&#43;1 to every injury roll of the company)", "Use Wondrous Cram (skips the injury roll: counts as Full Recovery)",
		"From the match", "Items: Healing Herbs: &#43;1 to every injury roll.", `uses: {&#34;10&#34;:&#34;healingHerbs&#34;}`, "Casualties cannot use Healing Herbs.", "(they do not stack)"} {
		if !strings.Contains(body, want) {
			t.Errorf("sheet lacks %q", want)
		}
	}
	// Brand is not a Hero: no Herbs row for them (one Herbs row for Aldor, one for nobody else), Cirion carries nothing.
	if n := strings.Count(body, "Use Healing Herbs ("); n != 1 {
		t.Errorf("Healing Herbs rows = %d, want 1 (heroes only)", n)
	}
	if n := strings.Count(body, `name="item-`); n != 2 {
		t.Errorf("one item field per unit, got %d", n)
	}
}

func TestSheetWithoutItemsShowsNoItemRows(t *testing.T) {
	f := amFixture(t)
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(amState(core.AftermathPending, core.AftermathEdit), nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	if strings.Contains(body, "Use Healing Herbs") || strings.Contains(body, "From the match") {
		t.Error("no items and no match prefill: nothing about them")
	}
}

func TestSheetFormCarriesTheElection(t *testing.T) {
	f := amFixture(t)
	want := core.AftermathInput{AgainstTheOdds: []string{"influence"}, Models: []core.ModelSheet{
		{UnitID: 10, Outcome: core.OutcomeSurvived, Gains: map[string]int{"participated": 1, "mvp": 1, "slain": 2}, UsesItem: "healingHerbs"},
		{UnitID: 11, Outcome: core.OutcomeCasualty, Gains: map[string]int{}, UsesItem: "wondrousCram"}}}
	f.camp.EXPECT().SaveAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), 2, want).Return(amState(core.AftermathSheet, core.AftermathEdit), nil)
	if rec := amPost(f, "/sheet", sheetForm(url.Values{"item-10": {"healingHerbs"}, "item-11": {"wondrousCram"}})); rec.Code != 200 {
		t.Errorf("save: %d", rec.Code)
	}
}

func TestInjuriesStepShowsTheHerbsBannerAndTheCramCard(t *testing.T) {
	a := injuriesState(
		core.Casualty{UnitID: 11, Name: "Brand", Rolled: true, Dice: []int{4, 5}, Total: 10, Modifier: 1, ResultKey: "fullRecovery", ResultTxt: "Full Recovery", Kind: core.CasualtyRecovered},
		core.Casualty{UnitID: 12, Name: "Cirion", Rolled: true, Final: true, Total: 7, Item: "wondrousCram", ResultKey: "fullRecovery", ResultTxt: "Full Recovery", Kind: core.CasualtyRecovered})
	a.Input.Models[0].UsesItem = "healingHerbs"
	f := itemFixture(t, a)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{"Healing Herbs: &#43;1 to every injury roll", "&#43;1 = 10", `>Wondrous Cram</span><span class="sr-only">No roll:`, "Full Recovery"} {
		if !strings.Contains(body, want) {
			t.Errorf("injuries lack %q", want)
		}
	}
	if strings.Count(body, "Roll 2D6") != 0 {
		t.Error("a Cram casualty has no Roll button")
	}
}

func TestReadOnlySheetNamesTheElectedItem(t *testing.T) {
	a := amState(core.AftermathInjuries, core.AftermathNone)
	a.Input.Models[0].UsesItem = "healingHerbs"
	f := itemFixture(t, a)
	if body := f.do(f.signedIn(httpGet(amPath))).Body.String(); !strings.Contains(body, "Uses Healing Herbs") {
		t.Error("the sheet shows who uses the Herbs")
	}
}

func TestSuccessionFallbackNamesTheWarriorsAndIsMandatory(t *testing.T) {
	f := amFixture(t)
	st := injuriesState(core.Casualty{UnitID: 10, Name: "Aldor", Rolled: true, Dice: []int{1, 1}, Total: 2, ResultTxt: "Dead", Kind: core.CasualtyDead, Final: true})
	st.Succession = &core.Succession{Fallback: []int64{11}}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{`name="kind" value="sergeant"`, "A Sergeant has fallen and the company has no hero left", "Hero in the Making", "This place cannot stay empty", `name="answer" value="11"`, "Make Sergeant", "Experience"} {
		if !strings.Contains(body, want) {
			t.Errorf("fallback lacks %q", want)
		}
	}
	if strings.Contains(body, "Nobody for now") {
		t.Errorf("the Warrior fallback is not optional")
	}
	st.Succession = &core.Succession{NeedLeader: true, Fallback: []int64{11, 12}}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	body = f.do(f.signedIn(httpGet(amPath))).Body.String()
	if !strings.Contains(body, `name="kind" value="succession"`) || !strings.Contains(body, "2 Warriors are tied") || strings.Contains(body, "required checked") {
		t.Errorf("tied fallback: the player chooses")
	}
}

func TestWoundsOfAHeroShowsTheExtraInfluenceRoll(t *testing.T) {
	f := amFixture(t)
	rules := amRules
	rules.Hero = []core.InjuryRow{{Min: 2, Max: 11, Key: "dead", Name: "Dead", Kind: core.CasualtyDead}, {Min: 12, Max: 12, Key: "woundsOfAHero", Name: "Wounds of a Hero", Kind: core.CasualtyHeal, BonusDice: "d6", BonusSpec: core.DiceSpec{Count: 1, Sides: 6}, BonusKind: "influence"}}
	f2 := matchWrite(t, endedMatch())
	_ = f
	f = f2
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(rules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: amUnits()}, nil).AnyTimes()
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(injuriesState(
		core.Casualty{UnitID: 10, Name: "Aldor", Hero: true, Rolled: true, Dice: []int{6, 6}, Total: 12, ResultKey: "woundsOfAHero", ResultTxt: "Wounds of a Hero", Kind: core.CasualtyRecovered, Bonus: 4},
		core.Casualty{UnitID: 11, Name: "Brand", Hero: true, Rolled: true, Dice: []int{6, 6}, Total: 12, ResultKey: "woundsOfAHero", ResultTxt: "Wounds of a Hero", Kind: core.CasualtyHeal,
			Decision: &core.CasualtyDecision{Kind: "heal", Options: []core.DecisionOption{{Key: "arm", Label: "Arm Wound"}}}}), nil)
	body := f.do(f.signedIn(httpGet(amPath))).Body.String()
	for _, want := range []string{"Extra D6 Influence rolled: &#43;4 Influence.", "The extra D6 Influence is rolled once the question below is answered."} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
}

// Each status of the aftermath is one named step, and the stepper shows that step at its own index.
func TestAftermathStepsNameTheCurrentStep(t *testing.T) {
	for _, c := range []struct {
		status                                core.AftermathStatus
		step                                  int
		sheet, injuries, rewards, done, ahead bool
		given                                 bool
	}{
		{status: core.AftermathPending, step: stepSheet, sheet: true, ahead: true},
		{status: core.AftermathSheet, step: stepSheet, sheet: true, ahead: true},
		{status: core.AftermathInjuries, step: stepInjuries, injuries: true},
		{status: core.AftermathLevelUps, step: stepRewards, rewards: true, given: true},
		{status: core.AftermathDone, step: stepDone, done: true, given: true},
		{status: core.AftermathWaived, step: stepDone, done: true, given: true},
	} {
		v := aftermathView{A: core.Aftermath{Status: c.status, Casualties: []core.Casualty{{UnitID: 1}}}}
		if v.Step() != c.step || v.OnSheet() != c.sheet || v.OnInjuries() != c.injuries || v.OnRewards() != c.rewards || v.Closed() != c.done ||
			v.InjuriesAhead() != c.ahead || v.experienceGiven() != c.given {
			t.Errorf("%s: step %d, sheet %v, injuries %v, rewards %v, closed %v, ahead %v, given %v", c.status, v.Step(), v.OnSheet(), v.OnInjuries(), v.OnRewards(), v.Closed(), v.InjuriesAhead(), v.experienceGiven())
		}
		if !slices.ContainsFunc(v.Steps(), func(s aftermathStep) bool { return s.Index == c.step }) {
			t.Errorf("%s: step %d is not in the stepper: %+v", c.status, c.step, v.Steps())
		}
	}
}

// The strip of a rolled casualty and the badge of its unit card come from one classification: only the unit that fell keeps the skull.
func TestCasualtyToneDrivesTheUnitCard(t *testing.T) {
	for _, c := range []struct {
		kind        core.CasualtyKind
		tone, badge string
		icon        string
	}{
		{core.CasualtyDead, "danger", "badge-danger", "death-skull"},
		{core.CasualtyRetired, "danger", "badge-danger", "death-skull"},
		{core.CasualtyInjured, "warn", "badge-warn", ""},
		{core.CasualtyInjury, "warn", "badge-warn", ""},
		{core.CasualtyScratch, "warn", "badge-warn", ""},
		{core.CasualtyHeal, "success", "badge-success", ""},
		{core.CasualtyRecovered, "success", "badge-success", ""},
	} {
		cas := core.Casualty{UnitID: 10, Rolled: true, Kind: c.kind}
		v := aftermathView{A: core.Aftermath{Casualties: []core.Casualty{cas}}}
		if got := v.CasualtyTone(cas); got != c.tone {
			t.Errorf("%s: tone %q, want %q", c.kind, got, c.tone)
		}
		r := v.unitResult(core.ModelSheet{UnitID: 10, Outcome: core.OutcomeCasualty})
		if r.Badge != c.badge || r.Icon != c.icon {
			t.Errorf("%s: badge %q icon %q, want %q %q", c.kind, r.Badge, r.Icon, c.badge, c.icon)
		}
	}
}

func TestSavedSheetColoursEachOutcome(t *testing.T) {
	var v aftermathView
	for o, want := range map[core.ModelOutcome]string{core.OutcomeSurvived: "badge-success", core.OutcomeFled: "badge-warn", core.OutcomeCasualty: "badge-danger"} {
		if got := v.OutcomeBadge(o); got != want {
			t.Errorf("%s: badge %q, want %q", o, got, want)
		}
	}
}

// The experience per level-up comes from the rules, so an empty roster still prints it.
func TestAftermathViewLevelEveryFromTheRules(t *testing.T) {
	if got := (aftermathView{Rules: core.AftermathRules{LevelEvery: 4}}).LevelEvery(); got != 4 {
		t.Errorf("LevelEvery = %d, want 4 with an empty roster", got)
	}
}

// The sheet follows the company's order, whatever order it was saved in; the lists after it put the casualties first.
func TestAftermathListsFollowTheCompanyOrder(t *testing.T) {
	order := companyOrder(core.CompanyDetail{
		Roster:    []core.CompanyUnit{{ID: 2, Role: core.RoleWarrior}, {ID: 1, Role: core.RoleLeader}, {ID: 3, Role: core.RoleWarrior}},
		Cemetery:  []core.FallenUnit{{Unit: core.CompanyUnit{ID: 4, Role: core.RoleSergeant}}},
		Formation: []int64{3, 2, 1},
	})
	if got := []int{order[3], order[2], order[1], order[4]}; !slices.Equal(got, []int{0, 1, 2, 3}) {
		t.Errorf("the saved order leads, the fallen follow: %v", order)
	}

	roster := map[int64]core.CompanyUnit{}
	for _, id := range []int64{1, 2, 3} {
		roster[id] = core.CompanyUnit{ID: id, Name: fmt.Sprintf("Unit %d", 4-id), XP: 100, Level: 0}
	}
	v := aftermathView{Roster: roster, order: map[int64]int{1: 0, 2: 1, 3: 2}, A: core.Aftermath{
		Casualties: []core.Casualty{{UnitID: 3}},
		Absent:     []int64{2, 1},
	}}
	v.fillUnits(core.AftermathInput{Models: []core.ModelSheet{{UnitID: 3}, {UnitID: 1}}}, nil)
	if len(v.Units) != 2 || v.Units[0].Unit.ID != 1 || v.Units[1].Unit.ID != 3 {
		t.Errorf("the sheet stays in the company's order, the casualty included: %+v", v.Units)
	}
	if len(v.Absent) != 2 || v.Absent[0].ID != 1 {
		t.Errorf("who missed the game follows the company's order: %+v", v.Absent)
	}
	ids := []int64{1, 2, 3}
	slices.SortFunc(ids, v.casualtiesFirst)
	if !slices.Equal(ids, []int64{3, 1, 2}) {
		t.Errorf("after the sheet the casualties come first, then the company's order: %v", ids)
	}
}

// Restore default order appears only when the company has a saved order, and forgets it.
func TestRestoreDefaultOrder(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	page := func(formation []int64) string {
		detail.Formation = formation
		f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
		return f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	}
	if strings.Contains(page(nil), "Restore default order") {
		t.Error("a company in the default order has nothing to restore")
	}
	if body := page([]int64{5}); !strings.Contains(body, "Restore default order") || !strings.Contains(body, "my-company/1/formation/reset") {
		t.Error("a saved order can be given up")
	}
	f.camp.EXPECT().ResetFormation(gomock.Any(), frodo, int64(7), int64(1)).Return(nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/formation/reset", url.Values{})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Default order restored.") {
		t.Errorf("got %d", rec.Code)
	}
}
