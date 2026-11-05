package web

import (
	"html"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// levelFixture is a window with a unit that has the rolls given waiting.
func levelFixture(t *testing.T, xp int) *fixture {
	t.Helper()
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].XP, detail.Roster[0].LevelEvery = xp, 5
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	return f
}

func levelRoll(t *testing.T, f *fixture, roll core.LevelRoll) string {
	t.Helper()
	f.camp.EXPECT().RollLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), nil, false).Return(roll, nil)
	return html.UnescapeString(f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/roll", url.Values{"roll": {"0"}}))).Body.String())
}

func TestTheLevelUpWindowHasButtonsNotRadios(t *testing.T) {
	f := levelFixture(t, 5)
	gain := &core.StatGain{Stat: "Fight", Gain: 1, Taken: 1, Limit: 3, Detail: "Fight 4 → 5, path limit +3, peak 9"}
	body := levelRoll(t, f, core.LevelRoll{Kind: "hero", Offers: []core.LevelOffer{
		{Pair: 1, Roll: 8, Choices: []core.LevelChoice{{Key: "stat:fight:-1", Label: "Fight 4 → 5", Stat: gain}}},
		{Pair: 2, Roll: 2, Choices: []core.LevelChoice{{Key: "rule:Spy", Label: "Spy", Hint: "Sees all."}}},
	}})
	for _, no := range []string{`type="radio"`, `>Choose</button>`, "Apply both</button>\n    {{"} {
		if strings.Contains(body, no) {
			t.Errorf("the window has %q", no)
		}
	}
	for _, want := range []string{
		`value="stat:fight:-1"`, "Fight +1", `data-tip-text="Fight 4 → 5, path limit +3, peak 9"`,
		`class="level-dot-taken"`, `class="level-dot-gain"`, `class="level-dot-free"`,
		"Special rule", `class="level-info"`, `data-concept="Spy"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the window lacks %q", want)
		}
	}
	if strings.Count(body, "level-dot-") != 3 {
		t.Errorf("one dot per step of the limit: %d", strings.Count(body, "level-dot-"))
	}
}

func TestTheLevelUpWindowCarriesThePlan(t *testing.T) {
	f := levelFixture(t, 5)
	body := levelRoll(t, f, core.LevelRoll{Kind: "hero", Plan: &core.LevelPlan{Marks: [2]string{"stat:courage:1", ""}, Conflicts: map[string]string{"a|b": "no"}, Alone: []string{}},
		Offers: []core.LevelOffer{
			{Pair: 1, Roll: 5, Choices: []core.LevelChoice{{Key: "stat:courage:1", Label: "Courage +1", RollAgain: true}}},
			{Pair: 2, Roll: 8, Choices: []core.LevelChoice{{Key: "stat:fight:-1", Label: "Fight +1"}}},
		}, Upgrade: &core.LevelOffer{Choices: []core.LevelChoice{{Key: "upgrade:Pipe", Label: "Pipe"}}}})
	for _, want := range []string{`data-plan="{`, "stat:courage:1", "+ other roll", `data-again`, `value="upgrade:Pipe"`, `data-offer="-1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the window lacks %q", want)
		}
	}
}

func TestChoosingBothKeysSendsTheRollAgainOneFirst(t *testing.T) {
	f := levelFixture(t, 5)
	f.camp.EXPECT().ChooseLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), []string{"stat:courage:1", "stat:fight:-1"}).Return(core.LevelRoll{Kind: "hero", Done: true, Summary: "Both."}, nil)
	body := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/choose", url.Values{"choice": {"stat:courage:1"}, "also": {"stat:fight:-1"}}))).Body.String()
	if !strings.Contains(body, "Both.") {
		t.Errorf("the result: %s", body)
	}
}

func TestNoChangeClosesByItselfOnlyWithNoRollLeft(t *testing.T) {
	noChange := core.LevelRoll{Kind: "warrior", D6: 2, Result: "noChange", Done: true, Summary: "Hador rolled a 2: no change."}
	t.Run("no rolls left", func(t *testing.T) {
		body := levelRoll(t, levelFixture(t, 0), noChange)
		for _, want := range []string{`data-close-after="2000"`, "alert-danger", "No change", ">Done</button>"} {
			if !strings.Contains(body, want) {
				t.Errorf("lacks %q", want)
			}
		}
		if strings.Contains(body, "Next level up") {
			t.Error("no next roll")
		}
	})
	t.Run("rolls left", func(t *testing.T) {
		body := levelRoll(t, levelFixture(t, 5), noChange)
		if !strings.Contains(body, `data-close-after="0"`) || !strings.Contains(body, "Next level up") || !strings.Contains(body, "alert-danger") {
			t.Errorf("it stays open with the next roll: %s", body)
		}
	})
	t.Run("no promotion is not this case", func(t *testing.T) {
		body := levelRoll(t, levelFixture(t, 0), core.LevelRoll{Kind: "warrior", D6: 4, Result: "promoted", Done: true, Summary: "Hador has no promotion in the company."})
		if !strings.Contains(body, `data-close-after="0"`) || strings.Contains(body, "alert-danger") || !strings.Contains(body, "has no promotion") {
			t.Errorf("its text and its Done button: %s", body)
		}
	})
}

func heroInTheMaking() core.LevelRoll {
	return core.LevelRoll{Kind: "warrior", D6: 6, Result: "heroInTheMaking", Offers: []core.LevelOffer{{Roll: 6, Choices: []core.LevelChoice{
		{Key: "hero", Label: "Become a hero"}, {Key: "promo:0:0", Label: "Forgo the Hero's Journey: Citadel Guard", Group: "Take the promotion instead"}}}}}
}

func TestAHeroInTheMakingCelebratesAndFinishesWithoutAPath(t *testing.T) {
	f := levelFixture(t, 5)
	body := levelRoll(t, f, heroInTheMaking())
	for _, want := range []string{"Hero in the Making!", "Hador", `class="confetti"`, `role="status"`, "Accept and close", "Choose path now", "Forgo the Hero's Journey: Citadel Guard", `value="promo:0:0"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen lacks %q", want)
		}
	}
	if strings.Contains(body, `value="hero"`) {
		t.Error("accepting has its own buttons")
	}
	// reopening shows the screen again, without the confetti
	roll := heroInTheMaking()
	f.camp.EXPECT().LevelUpState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(&roll, nil)
	body = f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/levelup", nil))).Body.String()
	if !strings.Contains(body, "Hero in the Making!") || strings.Contains(body, `class="confetti"`) {
		t.Errorf("no confetti the second time: %s", body)
	}
	// accepting closes the window
	f.camp.EXPECT().ChooseLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), []string{"hero"}).Return(core.LevelRoll{Kind: "warrior", Done: true, Summary: "Hador becomes a hero."}, nil)
	body = f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/choose", url.Values{"choice": {"hero"}, "then": {"close"}}))).Body.String()
	if !strings.Contains(body, "Hador becomes a hero.") || !strings.Contains(body, `data-close-after="1"`) || !strings.Contains(body, `data-back="/campaigns/7/my-company/1"`) {
		t.Errorf("it closes by itself: %s", body)
	}
}

func TestChoosePathNowOpensThePathWindow(t *testing.T) {
	f := levelFixture(t, 5)
	f.camp.EXPECT().ChooseLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), []string{"hero"}).Return(core.LevelRoll{Kind: "warrior", Done: true, Summary: "Hador becomes a hero."}, nil)
	f.camp.EXPECT().PathOptions(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(core.PathOffer{Unit: core.CompanyUnit{ID: 5, Name: "Hador"}, Paths: []core.PathOption{{Name: "Path of the Duelist"}}}, nil)
	body := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/choose", url.Values{"choice": {"hero"}, "then": {"path"}}))).Body.String()
	if !strings.Contains(body, "Choose a path") || !strings.Contains(body, "Path of the Duelist") {
		t.Errorf("the path window: %s", body)
	}
}

func TestAHeroWithNoPathIsSentToThePathWindow(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].XP, detail.Roster[0].LevelEvery, detail.Roster[0].PathPending = 5, 5, true
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().LevelUpState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/levelup", nil))).Body.String()
	if !strings.Contains(body, "/units/5/path") || strings.Contains(body, "levelup/roll") {
		t.Errorf("it chooses its path before it can roll: %s", body)
	}
}

func TestChoosingAPathFromTheAftermathGoesBackToIt(t *testing.T) {
	f := levelFixture(t, 5)
	f.camp.EXPECT().ChoosePath(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Path of the Duelist", "").Return(nil)
	req := form("/campaigns/7/my-company/1/units/5/path", url.Values{"path": {"Path of the Duelist"}})
	req.Header.Set("HX-Current-URL", "http://x/campaigns/7/matches/3/aftermath/1")
	rec := f.do(f.signedIn(req))
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("the page it was opened from: %v", rec.Header())
	}
}
