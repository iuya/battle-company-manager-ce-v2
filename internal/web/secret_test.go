package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var fogScenario = core.ScenarioInfo{Name: "Fog of War", Table: 1, Roll: 3, ScoringObjectives: []core.ScoringObjective{
	{Key: "ownHeroSurvives", Side: "both", Secret: true, Instruction: "Note down one of your own Heroes.", Nomination: &core.ObjectiveNomination{From: "ownHeroes", AllowLeader: "onlyHero"},
		Options: []core.ScoringTier{{Text: "Alive.", Points: 1}, {Text: "Unwounded.", Points: 3}}},
	{Key: "terrainPiece", Side: "both", Secret: true, Instruction: "Note down a terrain piece.", Nomination: &core.ObjectiveNomination{From: "terrain", Where: "opponentHalf"},
		Options: []core.ScoringTier{{Text: "More models.", Points: 1}}},
	{Key: "kill", Side: "both", Instruction: "Public objective."},
}}

func secretMatch() core.Match {
	m := editableMatch()
	m.Scenario = &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Fog of War"}
	return m
}

func secretFixture(t *testing.T) *fixture {
	f := matchWrite(t, secretMatch())
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{fogScenario}, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Name: "Rangers", Roster: []core.CompanyUnit{
		{ID: 5, Name: "Faramir", Type: "Captain", Role: core.RoleLeader}, {ID: 6, Name: "Hador", Type: "Ranger", Role: core.RoleSergeant}, {ID: 7, Name: "Boy", Type: "Warrior", Role: core.RoleWarrior}}}, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(2)).Return(core.CompanyDetail{Name: "Raiders", Roster: []core.CompanyUnit{{ID: 20, Name: "Grashnak", Type: "Orc Captain", Role: core.RoleLeader}}}, nil).AnyTimes()
	return f
}

func TestSecretCardOfAPlayerListsNominationsAndTiers(t *testing.T) {
	f := secretFixture(t)
	f.camp.EXPECT().SecretObjectives(gomock.Any(), frodo, int64(7), int64(3)).Return(core.SecretView{
		Lines:  []core.SecretLine{{CompanyID: 1, Key: "ownHeroSurvives", Nominee: "Hador", Tier: 2, Points: 3}},
		Others: []string{"Raiders"}}, nil)
	body := f.do(f.signedIn(httpGet(matchPath + "/secret"))).Body.String()
	for _, want := range []string{"Secret objectives", "ownHeroSurvives", "terrainPiece", "Hador</option>", `value="6" selected`, "3 VP", `value="2" checked`, "Noted already: Raiders", "ruined tower"} {
		if !strings.Contains(body, want) {
			t.Errorf("card lacks %q", want)
		}
	}
	if strings.Contains(body, "Faramir</option>") {
		t.Error("the Leader may not be noted down while a Sergeant is a Hero")
	}
	if strings.Contains(body, "Public objective") {
		t.Error("only secret objectives")
	}
}

func TestSecretWriteAppliesTheActionAndIsRefusedForViewers(t *testing.T) {
	f := secretFixture(t)
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveSecretNominate, ObjectiveKey: "ownHeroSurvives", UnitID: 6}).
		Return(core.LiveCompany{}, core.LiveEntry{ID: 60}, nil)
	f.camp.EXPECT().SecretObjectives(gomock.Any(), frodo, int64(7), int64(3)).Return(core.SecretView{}, nil).AnyTimes()
	rec := f.do(f.signedIn(form(matchPath+"/secret/1", url.Values{"kind": {"nominate"}, "key": {"ownHeroSurvives"}, "unit": {"6"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Only you can see it") || !strings.Contains(rec.Body.String(), "/live/log/60/undo") {
		t.Errorf("nominate: %d", rec.Code)
	}
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveSecretDone, ObjectiveKey: "ownHeroSurvives", Tier: 2}).
		Return(core.LiveCompany{}, core.LiveEntry{}, core.Refuse("Choose one of the tiers of this objective."))
	rec = f.do(f.signedIn(form(matchPath+"/secret/1", url.Values{"kind": {"done"}, "key": {"ownHeroSurvives"}, "tier": {"2"}})))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose one of the tiers") {
		t.Errorf("refused: %d", rec.Code)
	}
}

func TestSecretObjectivesAreRevealedAndPrefillTheResultAfterTheEnd(t *testing.T) {
	m := secretMatch()
	m.Ended = true
	f := matchWrite(t, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{fogScenario}, nil).AnyTimes()
	line := core.SecretLine{CompanyID: 1, Company: "Rangers", Key: "ownHeroSurvives", Label: "Own hero survives", Text: "A very long rule text", Nominee: "Hador", Tier: 2, Points: 3}
	f.camp.EXPECT().SecretObjectives(gomock.Any(), frodo, int64(7), int64(3)).Return(core.SecretView{Revealed: true, Lines: []core.SecretLine{line}}, nil).AnyTimes()
	body := f.do(f.signedIn(httpGet(matchPath + "/secret"))).Body.String()
	if !strings.Contains(body, "revealed") || !strings.Contains(body, "Noted: Hador") || !strings.Contains(body, "Scored 3 VP") || !strings.Contains(body, "Own hero survives") || strings.Contains(body, "A very long rule text") {
		t.Errorf("reveal: %.300s", body)
	}
	_ = http.StatusOK
}
