package web

import (
	"html"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"go.uber.org/mock/gomock"
)

func TestTheCreationFormGivesTheLeaderAndSergeants(t *testing.T) {
	form := url.Values{"name": {"x"}, "leader": {"2"}, "sergeant": {"4", "1"}, "leader-x": {"7"}}
	r := httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()
	got := startHeroes(r)
	want := []core.StartHero{{Index: 1, Role: core.RoleSergeant}, {Index: 2, Role: core.RoleLeader}, {Index: 4, Role: core.RoleSergeant}}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("the leader and the sergeants are read: %+v", got)
	}
}

func TestARecruitThatCannotJoinOffersLowerResults(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	lower := &core.RecruitLower{Roll: 5, Reason: "an Elf, but the company has all it may.", Rows: []core.LowerRow{
		{Index: 0, Chart: "Reinforcements", Roll: "4", Text: "Warrior of Rohan"},
		{Index: 1, Chart: "Reinforcements", Roll: "3", Text: "Galadhrim Warrior", Blocked: "at most 4 Elf models"}}}
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(&core.RecruitRoll{Stage: "lower", Rolled: true, Roll: 5, Lower: lower}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/recruit", nil))).Body.String()
	for _, want := range []string{"Choose a lower result", "recruit/lower", `name="result" value="0"`, "Cannot join: at most 4 Elf models"} {
		if !strings.Contains(body, want) {
			t.Errorf("the window lacks %q", want)
		}
	}
	f.camp.EXPECT().ChooseLower(gomock.Any(), frodo, int64(7), 0).Return(core.RecruitResult{Summary: "A Warrior of Rohan joins."}, nil)
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	if rec := f.do(f.signedIn(form("/campaigns/7/my-company/recruit/lower", url.Values{"result": {"0"}}))); rec.Code != 200 || !strings.Contains(rec.Body.String(), "A Warrior of Rohan joins.") {
		t.Errorf("the chosen result settles the recruit: %d", rec.Code)
	}
}

func TestAChoiceResultAsksForPicksAndASpecialRollOffersTheMissingMember(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roll  *core.RecruitRoll
		wants []string
		not   []string
	}{
		{"choose any 2", &core.RecruitRoll{Stage: "lower", Rolled: true, Roll: 6, Lower: &core.RecruitLower{Roll: 6, Picks: 2, Reason: "Choose any 2 Hobbits from above: choose from the chart.",
			Rows: []core.LowerRow{{Index: 0, Chart: "Reinforcements", Roll: "4", Text: "Hobbit Archer"}}}},
			[]string{"Choose 2 results", "one at a time", "Take this result (2 to go)"}, []string{"Choose a lower result"}},
		{"choose one", &core.RecruitRoll{Stage: "lower", Rolled: true, Roll: 6, Lower: &core.RecruitLower{Roll: 6, Picks: 1, Reason: "Choose any: choose from the chart.",
			Rows: []core.LowerRow{{Index: 0, Chart: "Reinforcements", Roll: "4", Text: "Hobbit Archer"}}}},
			[]string{"Choose a result", "joins the company"}, []string{"Choose a lower result", "to go)"}},
		{"missing member", &core.RecruitRoll{Stage: "special", Rolled: true, Roll: 3, Budget: 3, Replace: "Vault Warden"},
			[]string{`name="pick" value="member"`, "Take the missing Vault Warden instead"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			d, detail := mineDetail()
			f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
			f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
			f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
			f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
			f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
			f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(tc.roll, nil)
			body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/recruit", nil))).Body.String()
			for _, w := range tc.wants {
				if !strings.Contains(body, w) {
					t.Errorf("the window lacks %q", w)
				}
			}
			for _, w := range tc.not {
				if strings.Contains(body, w) {
					t.Errorf("the window has %q", w)
				}
			}
		})
	}
}

func TestTheCemeteryIsASectionOfItsOwn(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Cemetery = []core.FallenUnit{{Unit: core.CompanyUnit{ID: 201, Name: "Hurin", Type: "Warrior of Minas Tirith", Points: 33}, Fate: "Dead", Match: "Match 2"}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	for _, want := range []string{"Cemetery", "1 fallen", "Hurin", "Match 2", `data-unit-id="201"`, "my-company/cemetery"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	f.camp.EXPECT().SaveCemeteryOrder(gomock.Any(), frodo, int64(7), []int64{201}).Return(nil)
	if rec := f.do(f.signedIn(form("/campaigns/7/my-company/cemetery", url.Values{"order": {"201"}}))); rec.Code != 200 {
		t.Errorf("saving the order: %d", rec.Code)
	}
}

func TestEveryCompanyPageRenders(t *testing.T) {
	content, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Skip("no rules file")
	}
	book, err := core.ParseBattleCompanies(content)
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, Config{})
	profiles := book.Profiles()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(profiles, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	for _, p := range profiles {
		rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c="+url.QueryEscape(p.Name), nil)))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), html.EscapeString(p.Name)) {
			t.Errorf("%s: %d", p.Name, rec.Code)
		}
	}
}

func TestAnOfficerChoosesItsPathInAWindow(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	offer := core.PathOffer{Unit: core.CompanyUnit{ID: 5, Name: "Hador", Role: core.RoleSergeant}, Paths: []core.PathOption{
		{Name: "Path of the Soldier", Description: "Hardened fighters.", Summary: "Adds no Heroic Action. Heroic Combat Mastery."}, {Name: "Path of the Sorcerer", StartPower: true, Blocked: "the company already has a hero on it (at most 1)"}},
		Powers: []core.LevelChoice{{Key: "power:Aura of Command", Label: "Aura of Command"}}}
	f.camp.EXPECT().PathOptions(gomock.Any(), frodo, int64(7), int64(5)).Return(offer, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/units/5/path", nil))).Body.String()
	for _, want := range []string{"Choose a path", "Path of the Soldier", `name="path"`, "Not available: the company already has a hero", "First magical power", "Hardened fighters.", `title="Adds no Heroic Action. Heroic Combat Mastery."`} {
		if !strings.Contains(body, want) {
			t.Errorf("the window lacks %q", want)
		}
	}
	f.camp.EXPECT().ChoosePath(gomock.Any(), frodo, int64(7), int64(5), "Path of the Soldier", "").Return(nil)
	if rec := f.do(f.signedIn(form("/campaigns/7/my-company/units/5/path", url.Values{"path": {"Path of the Soldier"}}))); rec.Code != 200 || !strings.Contains(rec.Body.String(), "The path is chosen.") {
		t.Errorf("choosing: %d", rec.Code)
	}
	f.camp.EXPECT().RenameCompany(gomock.Any(), frodo, int64(7), "The Ruinwatch").Return(nil)
	if rec := f.do(f.signedIn(form("/campaigns/7/my-company/rename", url.Values{"name": {"The Ruinwatch"}}))); rec.Code != 200 {
		t.Errorf("renaming: %d", rec.Code)
	}
}

func TestAnOptionTheAppDoesNotKnowIsShownStruckThroughAndDisabled(t *testing.T) {
	f := newFixture(t, Config{})
	rec := httptest.NewRecorder()
	f.srv.render.partial(rec, 200, "campaign_new", "option_group", map[string]any{"Title": "Optional rules", "Scope": "rules", "Settings": core.CampaignSettings{},
		"Options": []core.CampaignOption{{Key: "madeUp", Scope: "rules", Label: "A made-up rule", Description: "Nobody knows it.", Unsupported: true}, {Key: "manualRolls", Scope: "rules", Label: "Known rule", Description: "Works."}}})
	body := rec.Body.String()
	for _, want := range []string{"unknown setting", "line-through", "A made-up rule", `name="option-manualRolls"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the group lacks %q", want)
		}
	}
	if strings.Contains(body, `name="option-madeUp"`) {
		t.Error("an unknown option cannot be switched on")
	}
}

func TestSmallShieldNoteIsShownOnTheCard(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].StatMods = []core.StatMod{{Stat: "defence", From: "Small Shield", Note: "+1 Defence against strikes only"}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	body := f.do(f.signedIn(httpGet("/campaigns/7/my-company"))).Body.String()
	if !strings.Contains(body, "Small Shield: &#43;1 Defence against strikes only") {
		t.Errorf("the note is on the card: %d %v", len(body), strings.Contains(body, "Hador"))
	}
}

func TestMyCompanyOfAFinishedCampaignIsFrozen(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	d.Status = core.CampaignFinished
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	body := f.do(f.signedIn(httpGet("/campaigns/7/my-company"))).Body.String()
	if !strings.Contains(body, "is frozen as it was") || strings.Contains(body, "Rename the company") {
		t.Error("an ended campaign freezes the company page")
	}
}

func TestTheShiresRangerStartsLockedAsLeader(t *testing.T) {
	p := core.CompanyProfile{
		Name:              "The Shire",
		Start:             []core.StartUnit{{Count: 1, Unit: "Ranger of the North", Role: core.RoleLeader}, {Count: 3, Unit: "Hobbit Militia"}},
		RangerReplacement: &core.RangerReplacement{Unit: "Ranger of the North", Max: 1},
	}
	// even if a post leaves him out, he is shown as the Leader and locked
	rows := startRows(p, []core.StartHero{{Index: 1, Role: core.RoleLeader}, {Index: 2, Role: core.RoleSergeant}, {Index: 3, Role: core.RoleSergeant}})
	if !rows[0].Locked || rows[0].Role != core.RoleLeader {
		t.Errorf("the Ranger is the locked Leader: %+v", rows[0])
	}
	if rows[1].Locked || rows[1].Role != core.RoleWarrior {
		t.Errorf("only the Ranger is locked: %+v", rows[1])
	}
}
