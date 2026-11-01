package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var organiser = core.User{ID: 99, DisplayName: "Gandalf"}

const companyPath = "/campaigns/7/companies/2"

// grantSetup serves company 2 of campaign 7 to viewer; the organiser of the campaign is user 99.
func grantSetup(t *testing.T, viewer core.User, status core.CampaignStatus) *fixture {
	t.Helper()
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID, d.Status = organiser.ID, status
	f.camp.EXPECT().Get(gomock.Any(), viewer.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Company(gomock.Any(), int64(7), int64(2)).Return(core.CampaignCompany{ID: 2, Name: "Theirs", Owner: "Sam"}, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(2)).Return(core.CompanyDetail{Gold: 6, Influence: 3,
		Roster: []core.CompanyUnit{{ID: 40, Name: "Beregond", Type: "Captain"}, {ID: 41, Name: "Dorlas", Type: "Warrior"}}}, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	return f
}

func TestGrantsCardShowsTheHistoryNewestFirstToEveryone(t *testing.T) {
	f := grantSetup(t, frodo, core.CampaignActive)
	on := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	f.grants = []core.Grant{
		{ID: 2, Gold: -2, Note: "Correction", By: "Gandalf", On: on},
		{ID: 1, Gold: 5, Influence: 1, Experience: 2, UnitID: 41, Note: "Won the narrative scenario", By: "Gandalf", On: on},
	}
	body := f.do(f.signedIn(httptest.NewRequest("GET", companyPath, nil))).Body.String()
	for _, want := range []string{"Grants", "-2 Gold", "+5 Gold", "+1 Influence", "+2 Experience · Dorlas", "Won the narrative scenario", "Gandalf · 18 October 2026"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Index(body, "Correction") > strings.Index(body, "Won the narrative") {
		t.Error("the backend's order (newest first) is kept")
	}
	if strings.Contains(body, "Grant reward") {
		t.Error("a player must not see the form")
	}
}

func TestGrantsFormIsForTheOrganiserAndAdministrators(t *testing.T) {
	for _, u := range []core.User{organiser, sauron} {
		f := grantSetup(t, u, core.CampaignActive)
		body := f.do(f.signedInAs(u, httptest.NewRequest("GET", companyPath, nil))).Body.String()
		for _, want := range []string{"Grant reward", `name="gold"`, `name="note"`, `value="41"`, "Beregond", "Dorlas", "<strong class=\"text-fg\">6</strong> Gold", `hx-post="/campaigns/7/companies/2/grants"`, "negative number"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s misses %q", u.DisplayName, want)
			}
		}
	}
	f := grantSetup(t, organiser, core.CampaignFinished)
	if body := f.do(f.signedInAs(organiser, httptest.NewRequest("GET", companyPath, nil))).Body.String(); strings.Contains(body, "Grant reward") {
		t.Error("a finished campaign offers no form")
	}
}

func TestGrantRewardPostsTheInputAndShowsTheCard(t *testing.T) {
	f := grantSetup(t, organiser, core.CampaignActive)
	f.camp.EXPECT().GrantReward(gomock.Any(), organiser, int64(7), int64(2), core.GrantInput{Gold: -3, Influence: 2, Experience: 1, UnitID: 40, Note: "Narrative"}).Return(core.Grant{}, nil)
	rec := f.do(f.signedInAs(organiser, form(companyPath+"/grants", url.Values{"gold": {"-3"}, "influence": {"2"}, "experience": {"1"}, "unit": {"40"}, "note": {"Narrative"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `id="grants"`) || !strings.Contains(body, "Reward granted.") || strings.Contains(body, "<html") {
		t.Errorf("%d %s", rec.Code, body)
	}
}

func TestGrantRewardIsRefusedForOthers(t *testing.T) {
	f := grantSetup(t, frodo, core.CampaignActive) // no GrantReward call is expected
	if rec := f.do(f.signedIn(form(companyPath+"/grants", url.Values{"gold": {"3"}, "note": {"x"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("a player: %d", rec.Code)
	}
	f = grantSetup(t, organiser, core.CampaignFinished)
	if rec := f.do(f.signedInAs(organiser, form(companyPath+"/grants", url.Values{"gold": {"3"}, "note": {"x"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("finished: %d", rec.Code)
	}
}

func TestGrantRewardKeepsTheFormWhenInvalid(t *testing.T) {
	f := grantSetup(t, organiser, core.CampaignActive)
	f.camp.EXPECT().GrantReward(gomock.Any(), organiser, int64(7), int64(2), gomock.Any()).
		Return(core.Grant{}, invalidGrant())
	rec := f.do(f.signedInAs(organiser, form(companyPath+"/grants", url.Values{"gold": {"1"}, "note": {""}})))
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "The note must be between 1 and 200 characters") || !strings.Contains(body, `value="1"`) {
		t.Errorf("%d %s", rec.Code, body)
	}
	// text in a number field never reaches the backend
	rec = f.do(f.signedInAs(organiser, form(companyPath+"/grants", url.Values{"gold": {"lots"}, "note": {"x"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Gold must be a whole number") {
		t.Errorf("text: %d", rec.Code)
	}
	// a refusal of the backend (below zero) is shown on top
	f.camp.EXPECT().GrantReward(gomock.Any(), organiser, int64(7), int64(2), gomock.Any()).Return(core.Grant{}, core.Refuse("The company would be left with less than 0 Gold or Influence."))
	rec = f.do(f.signedInAs(organiser, form(companyPath+"/grants", url.Values{"gold": {"-50"}, "note": {"x"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "less than 0 Gold") || !strings.Contains(rec.Body.String(), "alert-danger") {
		t.Errorf("refused: %d", rec.Code)
	}
}

func invalidGrant() error {
	_, err := core.GrantInput{Gold: 1}.Validate()
	return err
}

func TestGrantRewardSentTwiceIsGrantedOnce(t *testing.T) {
	f := grantSetup(t, organiser, core.CampaignActive)
	f.camp.EXPECT().GrantReward(gomock.Any(), organiser, int64(7), int64(2), core.GrantInput{Gold: 5, Note: "Won the siege"}).Return(core.Grant{}, nil).Times(1)
	var first string
	for i := 0; i < 2; i++ {
		rec := f.do(f.signedInAs(organiser, form(companyPath+"/grants", url.Values{"gold": {"5"}, "note": {"Won the siege"}})))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Reward granted.") {
			t.Fatalf("send %d: %d %s", i, rec.Code, rec.Body.String())
		}
		if i == 0 {
			first = rec.Body.String()
		} else if rec.Body.String() != first {
			t.Error("the second send gets the first answer")
		}
	}
}

func TestValidationMessagesLandUnderTheirField(t *testing.T) {
	_, err := core.GrantInput{Gold: 5000, Influence: -500, Experience: 500}.Validate()
	got := map[string]string{}
	for _, fr := range fieldReasons(err) {
		got[fr.Field] = fr.Msg
	}
	for field, want := range map[string]string{
		"note": "The note must be between 1 and 200 characters", "gold": "Gold must be between -1000 and 1000",
		"influence": "Influence must be between -100 and 100", "experience": "Experience must be between -100 and 100",
		"unit": "Choose the unit that gets the experience",
	} {
		if got[field] != want {
			t.Errorf("grant field %q: %q, want %q", field, got[field], want)
		}
	}
	_, err = core.GrantInput{Note: "x"}.Validate()
	if fr := fieldReasons(err); len(fr) != 1 || fr[0].Field != "" || fr[0].Msg != "Give some Gold, Influence or Experience" {
		t.Errorf("a message about the whole form has no field: %+v", fr)
	}

	_, err = core.ScenarioInput{Kind: core.ScenarioCustom, Description: strings.Repeat("x", 1001), Victory: strings.Repeat("x", 1001)}.Validate()
	sc := scenarioErrors(err)
	for field, want := range map[string]string{
		"name": "The name must be between 1 and 60 characters", "description": "The description can have at most 1000 characters",
		"victory": "How it is won can have at most 1000 characters",
	} {
		if sc[field] != want {
			t.Errorf("scenario field %q: %q, want %q", field, sc[field], want)
		}
	}
}

// ownCompanyPage serves the My Company page of company 1 (the viewer's own) of campaign 7, organised by user 99.
func ownCompanyPage(t *testing.T, viewer core.User, status core.CampaignStatus) string {
	t.Helper()
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	d.OrganiserID, d.Status = organiser.ID, status
	f.camp.EXPECT().Get(gomock.Any(), viewer.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), viewer, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), viewer, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), viewer, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	return f.do(f.signedInAs(viewer, httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
}

// The organiser and administrators award their own company from the Grants card at the bottom of its page; players never see the form.
func TestGrantsFormIsOnTheOwnCompanyPageOfTheOrganiserAndAdministrators(t *testing.T) {
	for _, u := range []core.User{organiser, sauron, {ID: 78, DisplayName: "Root", Role: core.RoleSuperuser}} {
		body := ownCompanyPage(t, u, core.CampaignActive)
		for _, want := range []string{"Grant reward", `hx-post="/campaigns/7/companies/1/grants"`, `name="gold"`, `name="note"`, "Hador"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s misses %q on their own company", u.DisplayName, want)
			}
		}
		if strings.Index(body, `id="grants"`) < strings.Index(body, `id="history`) && strings.Contains(body, `id="history`) {
			t.Error("the Grants card stays at the bottom, after the history")
		}
	}
	if body := ownCompanyPage(t, frodo, core.CampaignActive); !strings.Contains(body, `id="grants"`) || strings.Contains(body, "Grant reward") {
		t.Error("a plain player sees the Grants history but not the form on their own company")
	}
	if body := ownCompanyPage(t, organiser, core.CampaignFinished); strings.Contains(body, "Grant reward") {
		t.Error("a finished campaign offers no form")
	}
}
