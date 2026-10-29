package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// Several companies per player (docs/multi-company-spec.md): the company is in the URL, one helper authorises it, and the chip switcher
// appears only for a player with two or more.

func ownTwo(cap int) core.CampaignDetail {
	d := campaignDetail()
	d.Companies = []core.CampaignCompany{{ID: 1, Name: "Rangers", Faction: "Gondor"}, {ID: 4, Name: "Wardens", Faction: "Arnor"}}
	d.Settings.CompaniesPerPlayer = cap
	return d
}

func TestTheCompanyOfTheURLMustBeTheViewersOwn(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		form               url.Values
	}{
		{"the page of another player's company", "GET", "/campaigns/7/my-company/9", nil},
		{"a company that is not a number", "GET", "/campaigns/7/my-company/x", nil},
		{"a recruit window", "GET", "/campaigns/7/my-company/9/recruit", nil},
		{"a roll", "POST", "/campaigns/7/my-company/9/recruit/roll", url.Values{"roll": {"4"}}},
		{"a rename", "POST", "/campaigns/7/my-company/9/rename", url.Values{"name": {"x"}}},
		{"a unit's level-up window", "GET", "/campaigns/7/my-company/9/units/5/levelup", nil},
		{"the armory of another's company", "GET", "/campaigns/7/armory?company=9", nil},
		{"buying for another's company", "POST", "/campaigns/7/armory/buy", url.Values{"company": {"9"}, "unit": {"5"}, "item": {"Map"}}},
		{"buying with no company named", "POST", "/campaigns/7/armory/buy", url.Values{"unit": {"5"}, "item": {"Map"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(ownTwo(3), nil).AnyTimes()
			// no service call may happen: a stranger learns nothing and nothing is changed
			var r *http.Request
			if tc.method == "GET" {
				r = httptest.NewRequest("GET", tc.path, nil)
			} else {
				r = form(tc.path, tc.form)
			}
			if rec := f.do(f.signedIn(r)); rec.Code != http.StatusNotFound {
				t.Errorf("%s %s: got %d, want 404", tc.method, tc.path, rec.Code)
			}
		})
	}
}

func TestTheAliasOpensTheFirstCompanyOrOffersToCreateOne(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(ownTwo(3), nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil)))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/my-company/1" {
		t.Errorf("the alias: %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
	f = newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	rec = f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Create my company") {
		t.Errorf("no company: %d", rec.Code)
	}
	// the armory tab without a company goes to the first one
	f = newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(ownTwo(3), nil)
	rec = f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/armory", nil)))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/armory?company=1" {
		t.Errorf("the armory alias: %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestEachOwnCompanyOfTheURLIsTheOneTheServicesAreAskedAbout(t *testing.T) {
	f := newFixture(t, Config{})
	d := ownTwo(3)
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	// the second company (id 4) is the one read and rolled for, never the first
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(4)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(4)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(4)).Return(nil, nil).AnyTimes()
	_, detail := mineDetail()
	detail.CampaignCompany = d.Companies[1]
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(4)).Return(detail, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().RollRecruitWithPerk(gomock.Any(), frodo, int64(7), int64(4), 4, false, 0).Return(nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/4/recruit/roll", url.Values{"roll": {"4"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/campaigns/7/my-company/4/recruit/") {
		t.Errorf("the roll acts on company 4 and its forms stay on it: %d\n%s", rec.Code, rec.Body.String())
	}
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/4", nil))).Body.String()
	for _, want := range []string{"/campaigns/7/my-company/4/recruit", "/campaigns/7/my-company/4/rename", "/campaigns/7/my-company/4/units/5/levelup"} {
		if !strings.Contains(body, want) && want != "/campaigns/7/my-company/4/units/5/levelup" {
			t.Errorf("the page of company 4 lacks %q", want)
		}
	}
	if strings.Contains(body, "/my-company/1/") {
		t.Error("nothing on the page of company 4 acts on company 1")
	}
}

func TestTheSwitcherAppearsOnlyWithTwoCompanies(t *testing.T) {
	_, detail := mineDetail()
	cases := []struct {
		name      string
		d         core.CampaignDetail
		chips     bool
		newChip   bool
		capNote   string
		wantTitle string
	}{
		{"no company", campaignDetail(), false, false, "", ""},
		{"one company, cap 1", func() core.CampaignDetail { d, _ := mineDetail(); return d }(), false, false, "", ""},
		{"one company, cap 3", func() core.CampaignDetail { d, _ := mineDetail(); d.Settings.CompaniesPerPlayer = 3; return d }(), false, false, "", ""},
		{"two companies, below the cap", ownTwo(3), true, true, "", "Rangers · "},
		{"two companies, at the cap", ownTwo(2), true, false, "All 2 companies created", ""},
		{"two companies, a lowered cap", ownTwo(1), true, false, "All 2 companies created", ""},
		{"two companies, finished", func() core.CampaignDetail { d := ownTwo(3); d.Status = core.CampaignFinished; return d }(), true, false, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(tc.d, nil).AnyTimes()
			if len(tc.d.Companies) == 0 {
				body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
				if strings.Contains(body, `aria-label="Your companies"`) {
					t.Error("no company, no switcher")
				}
				return
			}
			id := tc.d.Companies[0].ID
			dd := detail
			dd.CampaignCompany = tc.d.Companies[0]
			f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), id).Return(dd, nil).AnyTimes()
			f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), id).Return(nil, nil).AnyTimes()
			f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), id).Return(nil, nil).AnyTimes()
			f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), id).Return(nil, nil).AnyTimes()
			f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
			body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/"+itoa(int(id)), nil))).Body.String()
			if got := strings.Contains(body, `aria-label="Your companies"`); got != tc.chips {
				t.Fatalf("switcher shown = %v, want %v", got, tc.chips)
			}
			if !tc.chips {
				return
			}
			if !strings.Contains(body, `href="/campaigns/7/my-company/4"`) || !strings.Contains(body, `aria-current="true"`) {
				t.Error("one chip per company, the current one marked")
			}
			if got := strings.Contains(body, "New company"); got != tc.newChip {
				t.Errorf("+ New company chip = %v, want %v", got, tc.newChip)
			}
			if tc.capNote != "" && !strings.Contains(body, tc.capNote) {
				t.Errorf("lacks %q", tc.capNote)
			}
			if tc.wantTitle != "" && !strings.Contains(body, "<title>"+tc.wantTitle) {
				t.Errorf("with several companies the title names the company")
			}
		})
	}
}

func TestTheOverviewListsEveryCompanyAndTheLimit(t *testing.T) {
	cases := []struct {
		name string
		d    core.CampaignDetail
		want []string
		not  []string
	}{
		{"cap 1, one company: as before", func() core.CampaignDetail { d, _ := mineDetail(); return d }(),
			[]string{"Your company", "/campaigns/7/my-company/1", "Manage company", "Each player has one company in this campaign."}, []string{"Create another company", "Your companies", "companies this campaign allows"}},
		{"cap 3, two companies", ownTwo(3), []string{"Your companies", "/campaigns/7/my-company/4", "Create another company", "Each player can have up to 3 companies"}, []string{"allows."}},
		{"at the cap", ownTwo(2), []string{"You have all 2 companies this campaign allows."}, []string{"Create another company"}},
		{"above a lowered cap", func() core.CampaignDetail { d := ownTwo(1); return d }(), []string{}, []string{"Create another company"}},
		{"none, cap 2", func() core.CampaignDetail { d := campaignDetail(); d.Settings.CompaniesPerPlayer = 2; return d }(), []string{"Choose your company", "up to 2 companies"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(tc.d, nil)
			f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
			body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil))).Body.String()
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Errorf("lacks %q", w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(body, n) {
					t.Errorf("must not have %q", n)
				}
			}
		})
	}
}

func TestOnlyManagersSeeAndUseTheCompaniesPerPlayerControl(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	if body := f.do(f.signedInAs(gandalf, httpGet("/campaigns/7"))).Body.String(); !strings.Contains(body, `id="companies-cap-input"`) || !strings.Contains(body, "Lowering it only stops players creating more companies") {
		t.Error("the organiser sees the control with the rule")
	}
	f = organiserFixture(t, frodo, core.CampaignActive)
	if body := f.do(f.signedIn(httpGet("/campaigns/7"))).Body.String(); strings.Contains(body, "companies-cap-input") || !strings.Contains(body, "Companies per player") {
		t.Error("a player sees the value, not the control")
	}
	f = organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().SetCompaniesPerPlayer(gomock.Any(), gandalf, int64(7), 3).Return(nil)
	rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/settings/companies", url.Values{"companies": {"3"}})))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7" {
		t.Errorf("saving reloads the overview: %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
	f = organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().SetCompaniesPerPlayer(gomock.Any(), gandalf, int64(7), 9).Return(core.ErrInvalidCampaign)
	if rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/settings/companies", url.Values{"companies": {"9"}}))); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `role="alert"`) {
		t.Errorf("an out of range number is shown on the control: %d", rec.Code)
	}
	if rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/settings/companies", url.Values{"companies": {"x"}}))); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Enter a number from 1 to 4.") {
		t.Errorf("not a number: %d", rec.Code)
	}
	f = organiserFixture(t, frodo, core.CampaignActive)
	if rec := f.do(f.signedIn(form("/campaigns/7/settings/companies", url.Values{"companies": {"3"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("a player may not change it: %d", rec.Code)
	}
}

func TestCreatingASecondCompanyAndHittingTheCap(t *testing.T) {
	f := newFixture(t, Config{})
	d, _ := mineDetail()
	d.Settings.CompaniesPerPlayer = 2
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return([]core.CompanyProfile{{BCCompany: core.BCCompany{Name: "Gondor", Alignment: "good"}}}, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	// below the cap the creation page opens for a player who has a company
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/new", nil))); rec.Code != 200 {
		t.Errorf("below the cap the page opens: %d", rec.Code)
	}
	in := core.CompanyInput{Name: "Second", Faction: "Gondor"}
	f.camp.EXPECT().CreateCompany(gomock.Any(), frodo, int64(7), in).Return(core.CampaignCompany{ID: 8}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/company", url.Values{"name": {"Second"}, "faction": {"Gondor"}})))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/my-company/8" {
		t.Errorf("the new company opens: %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
	f.camp.EXPECT().CreateCompany(gomock.Any(), frodo, int64(7), core.CompanyInput{Name: "Third", Faction: "Gondor"}).Return(core.CampaignCompany{}, core.ErrCompanyLimit)
	rec = f.do(f.signedIn(form("/campaigns/7/company", url.Values{"name": {"Third"}, "faction": {"Gondor"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "You already have the 2 companies this campaign allows.") {
		t.Errorf("the cap is refused with its number: %d", rec.Code)
	}
}

func TestTheCreationFormSetsCompaniesPerPlayer(t *testing.T) {
	f := newFixture(t, Config{})
	maker, do := campaignMaker(f)
	f.defaultsShown()
	if body := do(httptest.NewRequest("GET", "/campaigns/new", nil)).Body.String(); !strings.Contains(body, `name="companies-per-player"`) || !strings.Contains(body, `value="1"`) || !strings.Contains(body, `aria-describedby="companies-per-player-help"`) {
		t.Error("the creation form has the number, defaulting to 1, with its help text")
	}
	in := core.CampaignInput{Name: "Two bands", Settings: core.CampaignSettings{CompaniesPerPlayer: 3}}
	f.camp.EXPECT().Create(gomock.Any(), maker, in).Return(core.CampaignSummary{ID: 9}, nil)
	if rec := do(form("/campaigns", url.Values{"name": {"Two bands"}, "companies-per-player": {"3"}})); rec.Code != http.StatusSeeOther {
		t.Errorf("created: %d", rec.Code)
	}
	for _, bad := range []string{"9", "0", "x"} { // refused by the form's own validation, before anything is stored
		if rec := do(form("/campaigns", url.Values{"name": {"X"}, "companies-per-player": {bad}})); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%q: got %d", bad, rec.Code)
		}
	}
}

func TestAMatchOfOnePlayersTwoCompaniesCarriesTheSamePlayerNote(t *testing.T) {
	m := core.Match{ID: 3, Number: 3, Sides: []core.MatchSide{
		{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID, Owner: "Frodo"}, {ID: 4, Name: "Wardens", OwnerID: frodo.ID, Owner: "Frodo"}}},
		{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders", OwnerID: 50, Owner: "Sauron"}}},
	}}
	path, f := matchFixture(t, frodo, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", path, nil))).Body.String()
	if !strings.Contains(body, "Same player: both of these companies are Frodo&#39;s.") {
		t.Errorf("same side note missing:\n%s", body)
	}
	m.Sides[1].Companies = []core.CampaignCompany{{ID: 2, Name: "Raiders", OwnerID: frodo.ID, Owner: "Frodo"}}
	m.Sides[0].Companies = m.Sides[0].Companies[:1]
	path, f = matchFixture(t, frodo, m)
	if body := f.do(f.signedIn(httptest.NewRequest("GET", path, nil))).Body.String(); !strings.Contains(body, "Same player: Frodo plays both sides.") {
		t.Errorf("both sides note missing")
	}
	m.Sides[1].Companies = []core.CampaignCompany{{ID: 2, Name: "Raiders", OwnerID: 50, Owner: "Sauron"}}
	path, f = matchFixture(t, frodo, m)
	if body := f.do(f.signedIn(httptest.NewRequest("GET", path, nil))).Body.String(); strings.Contains(body, "Same player") {
		t.Errorf("no note when every company has its own player")
	}
}
