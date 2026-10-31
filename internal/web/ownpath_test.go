package web

import (
	"net/http/httptest"
	"net/url"
	"regexp"
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
	detail.Roster[0].Type, detail.Roster[0].XP, detail.Roster[0].LevelEvery = "Wood Elf Sentinel", 5, 5
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().RollLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), []int{6}, false).Return(core.LevelRoll{Kind: "warrior", D6: 6, Result: "heroInTheMaking", Done: true,
		Summary: "Hador becomes a hero on the Path of Their Own Making."}, nil)
	body := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/roll", url.Values{"roll": {"6"}}))).Body.String()
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
	detail.Roster[0].Type, detail.Roster[0].XP, detail.Roster[0].LevelEvery, detail.Roster[0].Level, detail.Roster[0].Role = "Wood Elf Sentinel", 7, 5, 0, core.RoleWarrior
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	if !strings.Contains(body, "/ 5 XP") || !strings.Contains(body, "Level Up ready!") || strings.Contains(body, "no experience") {
		t.Error("the card of a warrior on its own path shows XP and a Level up button until it rolls a 6")
	}
}

func TestTheCreationPreviewKeepsTheOfficersAsTheyAreAndTheOwnPathBadgeFollowsTheSetting(t *testing.T) {
	book := shippedRules(t).BC
	for _, custom := range []bool{false, true} {
		f := newFixture(t, Config{})
		d := campaignDetail()
		d.Settings.CustomOnTheirOwnPath = custom
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(book.Profiles(), nil).AnyTimes()
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
		arnor := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Arnor", nil))).Body.String()
		for _, not := range []string{"data-becomes", "as an officer"} { // an officer keeps its profile: the hero promotion is a level-up result
			if strings.Contains(arnor, not) {
				t.Errorf("the Arnor preview shows a promotion of the officers: %q", not)
			}
		}
		if !strings.Contains(arnor, "Ranger of Arnor") {
			t.Error("the Arnor preview lists the Rangers of Arnor")
		}
		lorien := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Lothlorien", nil))).Body.String()
		got := strings.Contains(lorien, "warriors that earn experience in this campaign")
		if got != custom || (!custom && !strings.Contains(lorien, "do not become heroes and earn no experience")) {
			t.Errorf("custom=%v: the own-path wording is wrong (earns=%v)", custom, got)
		}
	}
}

var glossaryHref = regexp.MustCompile(`/campaigns/7/glossary#t-([a-z0-9-]+)`)

// Every glossary link of the creation preview opens an article that says something, and the own-path note links to the path the
// campaign plays (issue 35: it linked the vanilla end-of-path term, which has no text of its own to show beside empty tables).
func TestEveryGlossaryLinkOfTheCreationPreviewOpensAnArticle(t *testing.T) {
	rs := shippedRules(t)
	gl := rs.GlossaryWithUnits()
	for _, custom := range []bool{false, true} {
		f := newFixture(t, Config{})
		d := campaignDetail()
		d.Settings.CustomOnTheirOwnPath = custom
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(rs.BC.Profiles(), nil).AnyTimes()
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(gl, nil).AnyTimes()
		for _, p := range rs.BC.Profiles() {
			body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c="+url.QueryEscape(p.Name), nil))).Body.String()
			for _, m := range glossaryHref.FindAllStringSubmatch(body, -1) {
				term, ok := gl.Term(m[1])
				if !ok {
					t.Errorf("%s (custom=%v): the link #t-%s has no article", p.Name, custom, m[1])
				} else if len(term.Paragraphs) == 0 && (term.Path == nil || len(term.Path.Limits)+len(term.Path.Rows) == 0) {
					t.Errorf("%s (custom=%v): the article %s is empty", p.Name, custom, m[1])
				}
			}
			if len(p.OnTheirOwnPath) == 0 {
				continue
			}
			want := "on-their-own-path"
			if custom {
				want = "path-of-their-own-making"
			}
			if !strings.Contains(body, "/campaigns/7/glossary#t-"+want+`"`) {
				t.Errorf("%s (custom=%v): the own-path note does not link #t-%s", p.Name, custom, want)
			}
		}
	}
}

// An end-of-path term has a text and no limits or chart: the glossary draws no empty tables under it.
func TestAnEndOfPathArticleHasNoEmptyTables(t *testing.T) {
	rs := shippedRules(t)
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(rs.GlossaryWithUnits(), nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/glossary", nil))).Body.String()
	article := func(id string) string {
		i := strings.Index(body, `<article id="t-`+id+`"`)
		if i < 0 {
			t.Fatalf("no article %s", id)
		}
		return body[i : i+strings.Index(body[i:], "</article>")]
	}
	if a := article("on-their-own-path"); strings.Contains(a, "Level-up chart") || strings.Contains(a, "Characteristic limits") || !strings.Contains(a, "never levels up") {
		t.Error("On Their Own Path draws empty tables or lacks its text")
	}
	if a := article("path-of-their-own-making"); !strings.Contains(a, "Level-up chart") || !strings.Contains(a, "Characteristic limits") {
		t.Error("the Path of Their Own Making keeps its limits and chart")
	}
}
