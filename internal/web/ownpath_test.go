package web

import (
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"go.uber.org/mock/gomock"
)

func TestTheOverviewSaysOwnPathUnitsAreWarriorsThatBecomeHeroesOnASix(t *testing.T) {
	for _, on := range []bool{true, false} {
		f := newFixture(t, Config{})
		d := campaignDetail()
		d.Settings.CustomOnTheirOwnPath = on
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
		f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
		body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil))).Body.String()
		has := strings.Contains(body, "warriors that earn experience and level up; on a roll of 6 they become heroes")
		if has != on || strings.Contains(body, "they start as heroes") {
			t.Errorf("setting %v: overview text wrong (has=%v)", on, has)
		}
		if !on && !strings.Contains(body, "earn no experience and never level up") {
			t.Error("without the setting those units earn no experience")
		}
	}
}

func TestAnOwnPathSixIsAFinalResultWithoutAPathWindow(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	d.Settings.ManualRolls = true
	d.Settings.CustomOnTheirOwnPath = true
	detail.Roster[0].Type, detail.Roster[0].XP = "Wood Elf Sentinel", 5
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().RollLevelUp(gomock.Any(), frodo, int64(7), int64(5), []int{6, 0}, false).Return(core.LevelRoll{Kind: "warrior", D6: 6, Result: "heroInTheMaking", Done: true,
		Summary: "Hador becomes a hero on the Path of Their Own Making."}, nil)
	body := f.do(f.signedIn(form("/campaigns/7/my-company/units/5/levelup/roll", url.Values{"roll": {"6"}}))).Body.String()
	for _, want := range []string{"Hador becomes a hero on the Path of Their Own Making.", "You rolled a", `data-dice="6"`, "Done"} {
		if !strings.Contains(body, want) {
			t.Errorf("the final result lacks %q", want)
		}
	}
	for _, no := range []string{"levelup/choose", "Re-roll", "Forgo", "Choose its path"} {
		if strings.Contains(body, no) {
			t.Errorf("a final result has no %q", no)
		}
	}
}

func TestAnOwnPathWarriorUnderTheSettingShowsExperienceAndLevelUp(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	d.Settings.CustomOnTheirOwnPath = true
	detail.Roster[0].Type, detail.Roster[0].XP, detail.Roster[0].Level, detail.Roster[0].Role = "Wood Elf Sentinel", 7, 0, core.RoleWarrior
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	if !strings.Contains(body, "/ 5 XP") || !strings.Contains(body, "Level Up ready!") || strings.Contains(body, "no experience") {
		t.Error("the card of a warrior on its own path shows XP and a Level up button until it rolls a 6")
	}
}

func TestAnOfficerIsShownAsTheUnitItBecomes(t *testing.T) {
	p := core.CompanyProfile{
		Name:       "Arnor",
		Start:      []core.StartUnit{{Count: 1, Unit: "Ranger of Arnor", Wargear: []string{"bow", "spear"}}},
		Promotions: []core.HeroPromotion{{From: "Ranger of Arnor", OnResult: "heroInTheMaking", Becomes: "Ranger of the North", Drop: []string{"spear"}}},
	}
	rows := startRows(p, nil)
	if len(rows) != 1 || rows[0].Becomes != "Ranger of the North" || strings.Join(rows[0].BecomesGear, ",") != "bow" {
		t.Errorf("the officer is the promoted unit with the wargear it keeps: %+v", rows)
	}
}

func TestTheCreationPreviewOfArnorShowsTheRangerOfTheNorthAndTheOwnPathBadgeFollowsTheSetting(t *testing.T) {
	content, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Skip("no rules file")
	}
	book, err := core.ParseBattleCompanies(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, custom := range []bool{false, true} {
		f := newFixture(t, Config{})
		d := campaignDetail()
		d.Settings.CustomOnTheirOwnPath = custom
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(book.Profiles(), nil).AnyTimes()
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
		arnor := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Arnor", nil))).Body.String()
		for _, want := range []string{`data-becomes="Ranger of the North"`, "becomes a Ranger of the North as an officer"} {
			if !strings.Contains(arnor, want) {
				t.Errorf("the Arnor preview lacks %q", want)
			}
		}
		lorien := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Lothlorien", nil))).Body.String()
		got := strings.Contains(lorien, "warriors that earn experience in this campaign")
		if got != custom || (!custom && !strings.Contains(lorien, "do not become heroes and earn no experience")) {
			t.Errorf("custom=%v: the own-path wording is wrong (earns=%v)", custom, got)
		}
	}
}
