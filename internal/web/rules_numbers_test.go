package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"go.uber.org/mock/gomock"
)

// The numbers of the rules (campaign.company.leaders, .sergeants, campaign.progression.threshold) are printed from the views, never typed in a
// template.

func TestTheCreationPageDrawsAsManyOfficerPlacesAsTheRulesGive(t *testing.T) {
	for _, tt := range []struct {
		leaders, sergeants int
		text               string
	}{{1, 2, "drag one leader and two sergeants"}, {1, 3, "drag one leader and three sergeants"}, {2, 1, "drag two leaders and one sergeant"}} {
		f := newFixture(t, Config{})
		profiles := []core.CompanyProfile{{BCCompany: core.BCCompany{Name: "Minas Tirith", Alignment: "good", MaxModels: 15, ReinforcementCost: 3}, Leaders: tt.leaders, Sergeants: tt.sergeants,
			Start: []core.StartUnit{{Count: 2, Unit: "Warrior of Minas Tirith"}}}}
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
		f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(profiles, nil).AnyTimes()
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
		body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Minas+Tirith", nil))).Body.String()
		if got := strings.Count(body, `data-slot="leader"`); got != tt.leaders {
			t.Errorf("%d leaders: %d places", tt.leaders, got)
		}
		if got := strings.Count(body, `data-slot="sergeant"`); got != tt.sergeants {
			t.Errorf("%d sergeants: %d places", tt.sergeants, got)
		}
		if !strings.Contains(body, tt.text) || !strings.Contains(body, `data-leaders="`+string(rune('0'+tt.leaders))+`"`) || !strings.Contains(body, `data-sergeants-needed="`+string(rune('0'+tt.sergeants))+`"`) {
			t.Errorf("the page says %q and carries the numbers for the script", tt.text)
		}
	}
}

func TestTheExperienceBarPrintsTheRulesThreshold(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].Type, detail.Roster[0].XP, detail.Roster[0].LevelEvery, detail.Roster[0].Level, detail.Roster[0].Role = "Wood Elf Sentinel", 7, 3, 0, core.RoleWarrior
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	if !strings.Contains(body, "7 / 3 XP") || strings.Contains(body, "/ 5 XP") || !strings.Contains(body, `aria-valuemax="3"`) || !strings.Contains(body, "width: 100%") || !strings.Contains(body, "×2") {
		t.Errorf("the bar says 7 / 3 XP, full, with 2 rolls waiting")
	}
}

// The field treatment is sized from the dice of the rules (hero[attempt].roll.dice), not from a D6.
func TestTheTreatmentWindowFollowsTheDiceOfTheRules(t *testing.T) {
	for _, tt := range []struct {
		spec core.DiceSpec
		want []string
		not  []string
	}{
		{core.NdM(1, 6), []string{`data-sides="6"`, `data-per-lane="1"`, `<option value="6">I rolled a 6</option>`, "a roll of 8+."}, []string{`value="7"`, " on D6"}},
		{core.NdM(2, 6), []string{`data-sides="6"`, `data-per-lane="2"`, `<option value="2">I rolled 2</option>`, `<option value="12">I rolled 12</option>`, "a roll of 8+ on 2D6."}, []string{`value="1"`, `value="13"`}},
	} {
		f := newFixture(t, Config{})
		d, detail := mineDetail()
		d.Settings.ManualRolls = true
		detail.Roster[0].Role = core.RoleHero
		detail.Roster[0].Injuries = []core.UnitInjury{{Key: "armWound", Name: "Arm wound", Count: 1}}
		detail.Injuries = core.InjuryRules{Attempt: core.TreatMethod{Key: "attempt", Cost: 1, Target: 8, TreatDice: tt.spec, Healable: []string{"armWound"}, PerPoint: 1, Ceiling: tt.spec.Max()}}
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
		f.camp.EXPECT().TreatmentState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil).AnyTimes()
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
		body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/treat", nil))).Body.String()
		for _, want := range tt.want {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the treatment window lacks %q", tt.spec, want)
			}
		}
		for _, not := range tt.not {
			if strings.Contains(body, not) {
				t.Errorf("%s: the treatment window has %q", tt.spec, not)
			}
		}
	}
}

// Slice P5: the Against the Odds re-roll figures and the experience of the end checks come from the rules.
func TestTheRerollCardPrintsTheBonusOfTheRules(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Rerolls = 1
	detail.RuleDice.RerollRating, detail.RuleDice.RerollCount = 40, 3
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	if !strings.Contains(body, "it costs 40 rating): 3 re-rolls") || strings.Contains(body, "15 rating") {
		t.Errorf("the re-roll card says 40 rating and 3 re-rolls")
	}
}

func TestTheEndChecksPrintTheExperienceOfTheRules(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	ec := endChecksFixture()
	ec.ObjectiveXP, ec.MVPXP = 2, 3
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(ec, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/end?company=1", nil))).Body.String()
	if !strings.Contains(body, "The objective is worth 2 experience, the MVP 3.") || strings.Contains(body, "worth 1 experience") {
		t.Errorf("the end checks say 2 and 3 experience")
	}
}
