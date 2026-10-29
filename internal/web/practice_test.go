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

// Practice matches (docs/practice-match-spec.md): the flag comes from the creation form only, the badge and banner mark the match, and
// the aftermath is the short "nothing to roll" state.

func practiceMatch(ended bool) core.Match {
	m := editableMatch()
	m.Practice, m.Ended = true, ended
	if ended {
		m.Result = core.MatchResult{Decided: true, Winner: 1, Sides: make([]core.SideResult, 2)}
	}
	return m
}

func practiceAftermath(access core.AftermathAccess) core.Aftermath {
	return core.Aftermath{MatchID: 3, CompanyID: 1, Status: core.AftermathDone, Revision: 1, Practice: true, Access: access, Outcome: "win",
		Lines: []core.AftermathLine{{Kind: "note", Text: core.PracticeNothingToRoll}}}
}

func TestCreateMatchPracticeComesFromTheFormOnly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		values   url.Values
		practice bool
	}{
		{"ticked", url.Values{"practice": {"1"}}, true},
		{"off by default", url.Values{}, false},
		{"anything but 1 is off", url.Values{"practice": {"yes"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
			f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1}, {ID: 2}}, nil)
			want := core.NewMatch{Sides: []core.NewSide{{Companies: []int64{1}}, {Companies: []int64{2}}}, Practice: tc.practice}
			f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), want).Return(core.Match{ID: 12, Practice: tc.practice}, nil)
			v := url.Values{"sides": {"2"}, "side-1": {"1"}, "side-2": {"2"}}
			for k, x := range tc.values {
				v[k] = x
			}
			if rec := f.do(f.signedIn(form("/campaigns/7/matches", v))); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/matches/12" {
				t.Errorf("got %d -> %q", rec.Code, rec.Header().Get("Location"))
			}
		})
	}
}

func TestCreateMatchFormKeepsThePracticeBoxWhenRefused(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}, nil)
	f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), gomock.Any()).Return(core.Match{}, core.Refuse("A cannot play yet."))
	rec := f.do(f.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {"1"}, "side-2": {"2"}, "practice": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "practice: true") {
		t.Errorf("got %d: the box stays ticked", rec.Code)
	}
}

func TestNewMatchFormHasThePracticeBox(t *testing.T) {
	for path, ticked := range map[string]bool{"/campaigns/7/matches/new": false, "/campaigns/7/matches/new?practice=1": true} {
		f := newFixture(t, Config{})
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
		f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}, nil)
		body := f.do(f.signedIn(httptest.NewRequest("GET", path, nil))).Body.String()
		for _, want := range []string{`name="practice" value="1"`, "Practice match", "No experience, influence, rewards or injuries. The result is only recorded.", "data-make-practice", "cannot be changed later"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %q", path, want)
			}
		}
		if got := strings.Contains(body, "practice: true"); got != ticked {
			t.Errorf("%s: ticked = %v, want %v", path, got, ticked)
		}
	}
}

func TestMatchesListShowsTheBadgeAndTheFilter(t *testing.T) {
	list := func(f *fixture) {
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
		f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(twoCompanies, nil).AnyTimes()
		real, practice := editableMatch(), practiceMatch(true)
		real.ID, real.Number, practice.ID, practice.Number = 1, 1, 2, 2
		f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return([]core.Match{practice, real}, nil)
	}
	f := newFixture(t, Config{})
	list(f)
	body := f.do(f.signedIn(httpGet("/campaigns/7/matches"))).Body.String()
	if strings.Count(body, "data-practice-badge") != 1 || !strings.Contains(body, `href="/campaigns/7/matches/1"`) || !strings.Contains(body, `data-filter-practice`) || !strings.Contains(body, `ml-1 text-muted">1</span>`) {
		t.Errorf("the list shows both, one badge and the chip with its count:\n%s", body)
	}
	f = newFixture(t, Config{})
	list(f)
	body = f.do(f.signedIn(httpGet("/campaigns/7/matches?practice=1"))).Body.String()
	if strings.Contains(body, `href="/campaigns/7/matches/1"`) || !strings.Contains(body, `href="/campaigns/7/matches/2"`) {
		t.Errorf("the Practice filter lists only practice matches:\n%s", body)
	}
}

func TestOverviewShowsThePracticeCountApart(t *testing.T) {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.Matches, d.PracticeMatches = 4, 2
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
	body := f.do(f.signedIn(httpGet("/campaigns/7"))).Body.String()
	if !strings.Contains(body, "2 practice matches") {
		t.Errorf("Overview lacks the practice line")
	}
	f = newFixture(t, Config{})
	d.PracticeMatches = 0
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
	if body := f.do(f.signedIn(httpGet("/campaigns/7"))).Body.String(); strings.Contains(body, "data-practice-count") {
		t.Error("no line without a practice match")
	}
}

func TestMatchPageMarksAPracticeMatch(t *testing.T) {
	for _, ended := range []bool{false, true} {
		f := matchWrite(t, practiceMatch(ended))
		f.aftermaths = twoAftermaths(core.AftermathDone, core.AftermathDone)
		body := f.do(f.signedIn(httpGet(matchPath))).Body.String()
		for _, want := range []string{"data-practice-badge", "data-practice-banner", "Practice match: no experience, influence, rewards or injuries."} {
			if !strings.Contains(body, want) {
				t.Errorf("ended=%v lacks %q", ended, want)
			}
		}
		if strings.Contains(body, `hx-post="/campaigns/7/matches/3/practice`) {
			t.Error("there is no action to change the flag")
		}
		if ended {
			card := body[strings.Index(body, `id="aftermath"`):]
			if strings.Count(card, "Aftermath: nothing to roll (practice match)") != 2 || strings.Contains(card, ">Done<") || strings.Contains(card, ">Start<") || strings.Contains(card, "Waive") {
				t.Errorf("each company says nothing to roll, with no call to action:\n%s", card)
			}
		}
	}
	f := matchWrite(t, editableMatch())
	if body := f.do(f.signedIn(httpGet(matchPath))).Body.String(); strings.Contains(body, "data-practice") {
		t.Error("a real match has no practice marks")
	}
}

func TestAftermathPageOfAPracticeMatchIsTheShortState(t *testing.T) {
	f := matchWrite(t, practiceMatch(true))
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(practiceAftermath(core.AftermathEdit), nil)
	rec := f.do(f.signedIn(httpGet(amPath)))
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("got %d, not a 404", rec.Code)
	}
	for _, want := range []string{"data-practice-badge", "Practice match: nothing to roll.", "Nothing was awarded and nobody was injured.", `href="/campaigns/7/matches/3"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, not := range []string{`name="outcome-`, "Save sheet", "Waive", "aftermath-step-"} {
		if strings.Contains(body, not) {
			t.Errorf("page has %q: no sheet, rolls or waive", not)
		}
	}
}

func TestAftermathWritesOfAPracticeMatchAreRefused(t *testing.T) {
	refuse := core.ErrPracticeNothingToRoll
	f := matchWrite(t, practiceMatch(true))
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(amRules, nil).AnyTimes()
	f.camp.EXPECT().StartAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), gomock.Any(), gomock.Any()).Return(core.Aftermath{}, refuse)
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(practiceAftermath(core.AftermathEdit), nil)
	rec := amPost(f, "/start", url.Values{"revision": {"1"}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Practice match: nothing to roll.") {
		t.Errorf("an editor is told why: %d\n%s", rec.Code, rec.Body.String())
	}
	f = matchWrite(t, practiceMatch(true))
	f.camp.EXPECT().AftermathRules(gomock.Any(), int64(7)).Return(amRules, nil).AnyTimes()
	f.camp.EXPECT().FinishAftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1), gomock.Any()).Return(core.Aftermath{}, refuse)
	f.camp.EXPECT().Aftermath(gomock.Any(), frodo, int64(7), int64(3), int64(1)).Return(practiceAftermath(core.AftermathNone), nil).AnyTimes()
	if rec := amPost(f, "/finish", url.Values{"revision": {"1"}}); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Practice match: nothing to roll.") {
		t.Errorf("a reader gets the refusal: %d\n%s", rec.Code, rec.Body.String())
	}
}

func TestATOCardShowsTheRewardBonusesDisabledInAPracticeMatch(t *testing.T) {
	f := matchWrite(t, practiceMatch(false))
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return(testScenarios, nil).AnyTimes()
	c := atoCard()
	c.Options[0].Disabled, c.Options[0].Reason = true, core.ATOPracticeReason
	c.Options = append(c.Options, core.ATOOption{Key: "influence", Name: "Influence", Rating: 15, Taken: true, Reason: core.ATOPracticeReason})
	c.Selection = core.ATOSelection{Bonuses: []string{"influence"}}
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), frodo, int64(7), int64(3)).Return(c, nil)
	body := htmxGet(f, matchPath+"/ato").Body.String()
	if strings.Count(body, core.ATOPracticeReason) < 2 || !strings.Contains(body, "data-ato-practice") || !strings.Contains(body, `name="bonus" value="toolKit"`) {
		t.Errorf("reward bonuses show the reason, table effects stay:\n%s", body)
	}
}
