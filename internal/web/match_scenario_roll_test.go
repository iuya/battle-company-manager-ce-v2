package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The scenario roll of the match page (docs/matches.md, "New match"): the 3D dice of the new match card, here on the live match.

// scenarioRollReq is a Roll again request of the match page.
func scenarioRollReq(f *fixture, v url.Values) *httptest.ResponseRecorder {
	v.Set("revision", "4")
	return f.do(f.signedIn(form(testMatchPath+"/scenario/roll", v)))
}

// With Shake the table off the server rolls and the card plays its numbers out: the text and the Vantage Point forms wait for the dice.
func TestRollAgainWithoutShakePlaysTheServersDice(t *testing.T) {
	m := editableMatch()
	f := rollSetup(t, m, core.CampaignSettings{})
	seq := []int{1, 1}
	f.srv.die = func(int) int { n := seq[0]; seq = seq[1:]; return n }
	done := m
	done.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Seize Ground"}
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Seize Ground"}).Return(done, nil)
	body := scenarioRollReq(f, url.Values{}).Body.String()
	for _, want := range []string{`x-data="rollPanel"`, `data-animate="true"`, `data-dice="1,1"`, `data-shake="false"`, `data-sides="6"`, `<canvas x-ref="canvas" class="dice-tray"`, `id="scenario-roll"`, `x-show="revealed"`, `hx-post="/campaigns/7/matches/3/scenario/roll"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "throwNewScenario") {
		t.Error("Shake the table is off: the button posts a form")
	}
}

// A normal page load of an open scenario shows everything at once: no animation.
func TestMatchPageShowsTheScenarioAtOnce(t *testing.T) {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Seize Ground"}
	f := rollSetup(t, m, core.CampaignSettings{ShakeTheTable: true})
	body := f.do(f.signedIn(httptest.NewRequest("GET", testMatchPath, nil))).Body.String()
	for _, want := range []string{`data-animate="false"`, `data-sides="6"`, `data-shake="true"`, `@click="throwNewScenario()"`, `data-roll-target="#match"`, `data-revision="4"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, `id="scenario-roll"`) {
		t.Error("no roll text without a roll")
	}
}

// Under Shake the table the browser's thrown dice are taken as they are and set the scenario the file places on them.
func TestRollAgainTakesTheThrownDiceUnderShakeTheTable(t *testing.T) {
	m := editableMatch()
	f := rollSetup(t, m, core.CampaignSettings{ShakeTheTable: true})
	f.srv.die = func(int) int { t.Error("the server must not roll thrown dice"); return 1 }
	sc, ok := core.ScenarioByRolls(testSelection, testScenarios, 5, 1)
	if !ok || sc.Name != "Rescue" {
		t.Fatalf("fixture: %v %v", sc, ok)
	}
	done := m
	done.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: sc.Name}
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: sc.Name}).Return(done, nil)
	rec := scenarioRollReq(f, url.Values{"roll": {"5"}, "roll2": {"1"}, "thrown": {"1"}})
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Rolled <strong>5</strong> (table 3) and <strong>1</strong>") || strings.Contains(body, `data-animate="true"`) {
		t.Errorf("thrown dice: %d %s", rec.Code, body)
	}
}

func TestRollAgainRefusesThrownDiceWithoutShakeTheTable(t *testing.T) {
	f := rollSetup(t, editableMatch(), core.CampaignSettings{ManualRolls: true})
	rec := scenarioRollReq(f, url.Values{"roll": {"1"}, "roll2": {"1"}, "thrown": {"1"}})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "does not let players throw the dice") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestRollAgainRefusesAnInvalidThrownDie(t *testing.T) {
	f := rollSetup(t, editableMatch(), core.CampaignSettings{ShakeTheTable: true})
	for _, v := range []url.Values{{"roll": {"7"}, "roll2": {"1"}}, {"roll": {"1"}, "roll2": {"0"}}, {"roll": {"1"}}} {
		v.Set("thrown", "1")
		rec := scenarioRollReq(f, v)
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), "A die shows 1 to 6.") {
			t.Errorf("%v: %d %s", v, rec.Code, rec.Body.String())
		}
	}
}

// A thrown pair the file places no scenario on cannot be rolled again by the server: the player chooses.
func TestRollAgainThrownPairWithNoScenario(t *testing.T) {
	f := rollSetup(t, editableMatch(), core.CampaignSettings{ShakeTheTable: true})
	f.srv.die = func(int) int { t.Error("the server must not re-roll"); return 1 }
	rec := scenarioRollReq(f, url.Values{"roll": {"3"}, "roll2": {"1"}, "thrown": {"1"}})
	body := rec.Body.String()
	if rec.Code != 422 || !strings.Contains(body, "place no scenario on that roll: choose one") || !strings.Contains(body, "Rolled <strong>3</strong> (table 2)") {
		t.Errorf("%d %s", rec.Code, body)
	}
}

// The Vantage Point is a modifier after the roll: no animation, and the dice it carries are never taken as thrown ones.
func TestVantagePointAdjustDoesNotAnimateOrThrow(t *testing.T) {
	m := editableMatch()
	f := rollSetup(t, m, core.CampaignSettings{ShakeTheTable: true})
	f.srv.die = func(int) int { t.Error("the perk rolls nothing"); return 1 }
	f.camp.EXPECT().UseVantagePoint(gomock.Any(), frodo, int64(7), int64(3), int64(1), 1, 1).Return(2, "1, Vantage Point +1 = 2", nil)
	done := m
	done.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Chance Encounter"}
	f.camp.EXPECT().SetScenario(gomock.Any(), frodo, int64(7), int64(3), 4, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Chance Encounter"}).Return(done, nil)
	body := scenarioRollReq(f, url.Values{"roll": {"1"}, "roll2": {"1"}, "vantage": {"1"}, "adjust": {"1"}}).Body.String()
	if strings.Contains(body, `data-animate="true"`) || !strings.Contains(body, "Chance Encounter") || !strings.Contains(body, "Vantage Point") {
		t.Errorf("vantage: %s", body)
	}
}
