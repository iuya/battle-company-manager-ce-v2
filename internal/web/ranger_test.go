package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"go.uber.org/mock/gomock"
)

// The recruit window's swap option and the officer panel come with bead d1z (the core side is built: pick "ranger",
// PendingOfficers, ArrangeOfficers).

func TestCemeteryTagsTheFallenRanger(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.RangerMayReturn, detail.RangerUnit = true, "Ranger of the North"
	detail.Cemetery = []core.FallenUnit{{Unit: core.CompanyUnit{ID: 9, Name: "Halbarad", Type: "Ranger of the North", Kind: core.KindInfantry}, Fate: "Dead", Match: "Match 3"},
		{Unit: core.CompanyUnit{ID: 10, Type: "Hobbit Militia", Kind: core.KindInfantry}, Fate: "Dead"}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	if n := strings.Count(body, "May return with a reinforcement"); n != 1 {
		t.Errorf("only the fallen Ranger is tagged, got %d tags", n)
	}
}

func TestAftermathHintsThatTheRangerMayReturn(t *testing.T) {
	f := matchWrite(t, endedMatch()) // Match 3
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(amRules, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{Roster: amUnits(), RangerMayReturn: true, RangerUnit: "Ranger of the North",
		Cemetery: []core.FallenUnit{{Unit: core.CompanyUnit{ID: 9, Type: "Ranger of the North"}, Fate: "Dead", Match: "Match 3, Seize Ground"}}}, nil).AnyTimes()
	st := amState(core.AftermathDone, core.AftermathEdit)
	st.Casualties = []core.Casualty{{UnitID: 10, Name: "Aldor", Hero: true, ResultKey: "dead", Kind: core.CasualtyDead}}
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(st, nil)
	if body := f.do(f.signedIn(httpGet(amPath))).Body.String(); !strings.Contains(body, "The Ranger of the North fell.") {
		t.Errorf("the Finish summary lacks the hint")
	}
}

func rangerFixture(t *testing.T, officersPending bool) (*fixture, core.CompanyDetail) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.OfficersPending = officersPending
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	return f, detail
}

func rolled(offer *core.RangerOffer) *core.RecruitRoll {
	return &core.RecruitRoll{Stage: "chart", Rolled: true, Roll: 2, Budget: 3, Adjust: upToThree, Ranger: offer}
}

func TestRecruitWindowOffersTheRangerSwap(t *testing.T) {
	f, _ := rangerFixture(t, false)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(rolled(&core.RangerOffer{Unit: "Ranger of the North"}), nil)
	body := f.do(f.signedIn(httpGet("/campaigns/7/my-company/recruit"))).Body.String()
	for _, want := range []string{`id="ranger-offer"`, `name="pick" value="ranger"`, "Swap it for the Ranger of the North (free)", "Leader to start with", "and arrange the officers"} {
		if !strings.Contains(body, want) {
			t.Errorf("window lacks %q", want)
		}
	}
	for _, gone := range []string{"steps down", `name="role"`} {
		if strings.Contains(body, gone) {
			t.Errorf("window still has %q", gone)
		}
	}
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(rolled(nil), nil)
	if body := f.do(f.signedIn(httpGet("/campaigns/7/my-company/recruit"))).Body.String(); strings.Contains(body, "ranger-offer") {
		t.Errorf("no offer, no swap")
	}
}

func officerPanel() *core.OfficerPanel {
	return &core.OfficerPanel{Unit: "Ranger of the North", Leader: core.OfficerCard{ID: core.RangerID, Title: "Ranger of the North", Role: core.RoleLeader},
		Officers: []core.OfficerCard{{ID: 11, Title: "Rosie", Role: core.RoleLeader}, {ID: 12, Title: "Sam", Role: core.RoleSergeant}, {ID: 13, Title: "Merry", Role: core.RoleSergeant}}, Drop: 1,
		Candidates: []core.OfficerCard{{ID: 14, Title: "Pippin", Role: core.RoleHero}}}
}

func TestPendingOfficersFillTheWindow(t *testing.T) {
	f, _ := rangerFixture(t, false)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(officerPanel(), nil)
	body := f.do(f.signedIn(httpGet("/campaigns/7/my-company/recruit"))).Body.String()
	for _, want := range []string{"The Ranger of the North takes command", `data-drop="1"`, `data-keep="true"`, `data-id="0"`, `data-id="11"`, `data-id="13"`, `data-id="14"`, "Appoint", `name="leader"`, "recruit/officers"} {
		if !strings.Contains(body, want) {
			t.Errorf("officer panel lacks %q", want)
		}
	}
	// the Ranger's card has no remove button: only the other three cards and the candidate do
	if n := strings.Count(body, "Not an officer</button>"); n != 4 {
		t.Errorf("remove buttons = %d, want 4 (never the Ranger's)", n)
	}
}

func TestMyCompanyOpensTheOfficerPanelAndShowsTheBanner(t *testing.T) {
	f, _ := rangerFixture(t, true)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(officerPanel(), nil)
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil)
	body := f.do(f.signedIn(httpGet("/campaigns/7/my-company"))).Body.String()
	for _, want := range []string{`id="officers-banner"`, "waiting for its new Leader", `data-modal="true"`, "The Ranger of the North takes command"} {
		if !strings.Contains(body, want) {
			t.Errorf("my company lacks %q", want)
		}
	}
}

func TestConfirmWithTheRangerShowsTheOfficerPanel(t *testing.T) {
	f, _ := rangerFixture(t, false)
	f.camp.EXPECT().ConfirmRecruit(gomock.Any(), frodo, int64(7), 0, "ranger").Return(core.RecruitResult{Roll: 2, Officers: officerPanel()}, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(officerPanel(), nil)
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/recruit/confirm", url.Values{"adjust": {"0"}, "pick": {"ranger"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "The Ranger of the North takes command") {
		t.Errorf("got %d", rec.Code)
	}
}

func TestAppointingTheOfficers(t *testing.T) {
	f, _ := rangerFixture(t, false)
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7)).Return(nil, nil).AnyTimes()
	// the Ranger moved to a Sergeant place
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(nil, nil)
	f.camp.EXPECT().ArrangeOfficers(gomock.Any(), frodo, int64(7), core.OfficerArrangement{Leader: 12, Sergeants: []int64{0, 11}}).
		Return(core.RecruitResult{Summary: "Officers: Sam as Leader."}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/recruit/officers", url.Values{"leader": {"12"}, "sergeant": {"0", "11"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Officers: Sam as Leader.") || rec.Header().Get("HX-Retarget") != "#main" {
		t.Errorf("got %d %q", rec.Code, rec.Header().Get("HX-Retarget"))
	}
	f.camp.EXPECT().ArrangeOfficers(gomock.Any(), frodo, int64(7), gomock.Any()).Return(core.RecruitResult{}, core.Refuse("The Ranger of the North takes command: he cannot be left out."))
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(officerPanel(), nil)
	rec = f.do(f.signedIn(form("/campaigns/7/my-company/recruit/officers", url.Values{"leader": {"11"}, "sergeant": {"12", "13"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "cannot be left out") || !strings.Contains(rec.Body.String(), "takes command") {
		t.Errorf("refused: %d", rec.Code)
	}
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7)).Return(officerPanel(), nil)
	if rec := f.do(f.signedIn(form("/campaigns/7/my-company/recruit/officers", url.Values{"sergeant": {"12"}}))); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("no leader: %d", rec.Code)
	}
}
