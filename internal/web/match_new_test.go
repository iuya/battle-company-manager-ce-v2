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

// The new match form (docs/matches.md, "New match"): the scenario the server rolled, Roll again / Choose another / One-off,
// the user's own companies marked, and the Against the Odds block for the underdog's owner only.

func newMatchFixture(t *testing.T) *fixture {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.Companies = []core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID}, {ID: 2, Name: "Raiders", OwnerID: 50}}, nil).AnyTimes()
	return f
}

func TestNewMatchFormShowsTheRolledScenarioAndMarksOwnCompanies(t *testing.T) {
	f := newMatchFixture(t)
	seq := []int{1, 2}
	f.srv.die = func(int) int { n := seq[0]; seq = seq[1:]; return n }
	body := f.do(f.signedIn(httpGet("/campaigns/7/matches/new"))).Body.String()
	for _, want := range []string{"Chance Encounter", "Rolled 1 (table 1) and 2", `name="scenario-kind" value="rulebook"`, `name="scenario-name" value="Chance Encounter"`,
		"/matches/new/scenario-roll", "/matches/new/scenario?mode=pick", "/matches/new/scenario?mode=custom", `&#34;mine&#34;:true`, `data-sides="[{&#34;ids&#34;:[1]},{&#34;ids&#34;:[]}]"`} {
		if !strings.Contains(body, want) {
			t.Errorf("form lacks %q", want)
		}
	}
	for _, bad := range []string{"side-name", "role-", "Attacker", "Defender", "data-mine-chip"} {
		if strings.Contains(body, bad) {
			t.Errorf("the form has no %q", bad)
		}
	}
}

func TestNewMatchFormMarksCompaniesThatCannotPlay(t *testing.T) {
	f := newMatchFixture(t)
	f.unavailable = map[int64]string{1: "Already in Match 6."}
	body := f.do(f.signedIn(httpGet("/campaigns/7/matches/new"))).Body.String()
	if !strings.Contains(body, `&#34;unavailable&#34;:&#34;Already in Match 6.&#34;`) {
		t.Error("the form lacks the reason of the busy company")
	}
	if !strings.Contains(body, `data-sides="[{&#34;ids&#34;:[]},{&#34;ids&#34;:[]}]"`) {
		t.Error("a company that cannot play does not start on side 1")
	}
}

func TestNewMatchScenarioRollAgainChooseAnotherAndOneOff(t *testing.T) {
	f := newMatchFixture(t)
	seq := []int{5, 1}
	f.srv.die = func(int) int { n := seq[0]; seq = seq[1:]; return n }
	r := f.signedIn(httptest.NewRequest("POST", "/campaigns/7/matches/new/scenario-roll", nil))
	r.Header.Set("HX-Request", "true")
	if body := f.do(r).Body.String(); !strings.Contains(body, `name="scenario-name" value="Rescue"`) || strings.Contains(body, "<html") {
		t.Errorf("roll again answers the card with the new scenario:\n%s", body)
	}
	body := htmxGet(f, "/campaigns/7/matches/new/scenario?mode=pick").Body.String()
	for _, want := range []string{"Seize Ground", "Chance Encounter", "Rescue", "/matches/new/scenario?name=Rescue"} {
		if !strings.Contains(body, want) {
			t.Errorf("the list lacks %q", want)
		}
	}
	if body := htmxGet(f, "/campaigns/7/matches/new/scenario?name=Rescue").Body.String(); !strings.Contains(body, `name="scenario-name" value="Rescue"`) || !strings.Contains(body, "Chosen") {
		t.Errorf("one press sets it:\n%s", body)
	}
	if body := htmxGet(f, "/campaigns/7/matches/new/scenario?mode=custom").Body.String(); !strings.Contains(body, `name="scenario-kind" value="custom"`) || !strings.Contains(body, `name="scenario-victory"`) {
		t.Errorf("one-off fields:\n%s", body)
	}
	if rec := htmxGet(f, "/campaigns/7/matches/new/scenario?mode=bogus"); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown mode: %d", rec.Code)
	}
}

func TestNewMatchScenarioListCancelKeepsTheRolledScenario(t *testing.T) {
	f := newMatchFixture(t)
	body := htmxGet(f, "/campaigns/7/matches/new/scenario?mode=pick&scenario-kind=rulebook&scenario-name=Chance+Encounter&scenario-die=1").Body.String()
	for _, want := range []string{"data-cancel-pick", `name="scenario-die" value="1"`, `name="scenario-name" value="Chance Encounter"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the list lacks %q:\n%s", want, body)
		}
	}
	body = htmxGet(f, "/campaigns/7/matches/new/scenario?scenario-kind=rulebook&scenario-name=Chance+Encounter&scenario-die=1").Body.String()
	if !strings.Contains(body, `data-scenario="Chance Encounter"`) || !strings.Contains(body, "Rolled 1 (table 1) and 2") {
		t.Errorf("Cancel shows the rolled scenario again:\n%s", body)
	}
	if body := htmxGet(f, "/campaigns/7/matches/new/scenario?mode=pick").Body.String(); strings.Contains(body, "data-cancel-pick") {
		t.Errorf("nothing to go back to, no Cancel")
	}
}

func TestCreateMatchCarriesTheScenarioAndAgainstTheOdds(t *testing.T) {
	f := newMatchFixture(t)
	want := core.NewMatch{Sides: []core.NewSide{{Companies: []int64{1}}, {Companies: []int64{2}}},
		Scenario: &core.ScenarioInput{Kind: core.ScenarioRulebook, Name: "Rescue"},
		ATO:      &core.NewMatchATO{Selection: core.ATOSelection{Bonuses: []string{"reroll", "ambush"}, Ambush: "Rescue"}}}
	f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), want).Return(core.Match{ID: 12}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {"1"}, "side-2": {"2"},
		"scenario-kind": {"rulebook"}, "scenario-name": {"Rescue"}, "ato": {"reroll", "ambush", ""}})))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/matches/12" {
		t.Errorf("got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}

	g := newMatchFixture(t)
	custom := core.NewMatch{Sides: want.Sides, Scenario: &core.ScenarioInput{Kind: core.ScenarioCustom, Name: "Bridge", Victory: "Hold it"}}
	g.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), custom).Return(core.Match{ID: 13}, nil)
	rec = g.do(g.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {"1"}, "side-2": {"2"},
		"scenario-kind": {"custom"}, "scenario-name": {"Bridge"}, "scenario-victory": {"Hold it"}})))
	if rec.Code != http.StatusSeeOther {
		t.Errorf("one-off: %d", rec.Code)
	}
}

func atoPreview(access core.ATOAccess, eligible bool) core.ATO {
	return core.ATO{Eligible: eligible, Access: access, CompanyName: "Rangers", OwnRating: 100, OppName: "Raiders", OppRating: 140, Gap: 40,
		Options: []core.ATOOption{
			{Key: "reroll", Name: "Reroll", Rating: 5}, {Key: "influence", Name: "Influence", Rating: 10}, {Key: "toolKit", Name: "Tool Kit", Rating: 15},
			{Key: "wanderer", Name: "Wanderer", Rating: 45, OverBudget: true}, {Key: "ambush", Name: "Ambush!", Rating: 60, OverBudget: true}}}
}

func TestNewMatchAgainstTheOddsBlock(t *testing.T) {
	const path = "/campaigns/7/matches/new/ato?a=1&b=2"
	f := newMatchFixture(t)
	f.camp.EXPECT().PreviewAgainstTheOdds(gomock.Any(), frodo, int64(7), []core.NewSide{{Companies: []int64{1}}, {Companies: []int64{2}}}, false).Return(atoPreview(core.ATOEdit, true), nil)
	body := htmxGet(f, path).Body.String()
	for _, want := range []string{`id="new-ato-card"`, `name="ato" value="reroll"`, `name="ato" value="influence"`, "Tool Kit", "Add on the match page"} {
		if !strings.Contains(body, want) {
			t.Errorf("block lacks %q", want)
		}
	}
	for _, bad := range []string{"Wanderer", "Ambush!", `value="toolKit"`} {
		if strings.Contains(body, bad) {
			t.Errorf("not affordable or completed later: no %q", bad)
		}
	}
	for name, card := range map[string]core.ATO{"not the underdog's owner": atoPreview(core.ATONone, true), "equal ratings": atoPreview(core.ATOEdit, false)} {
		g := newMatchFixture(t)
		g.camp.EXPECT().PreviewAgainstTheOdds(gomock.Any(), frodo, int64(7), gomock.Any(), false).Return(card, nil)
		if body := htmxGet(g, path).Body.String(); strings.Contains(body, "new-ato-card") {
			t.Errorf("%s: no block", name)
		}
	}
	h := newMatchFixture(t) // no preview is asked for when a side has no single company
	if body := htmxGet(h, "/campaigns/7/matches/new/ato?a=1&b=0").Body.String(); strings.Contains(body, "new-ato-card") {
		t.Error("a missing company: no block")
	}
}

func TestYourTurnListsWhatWaitsOnTheViewer(t *testing.T) {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.Companies = []core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(twoCompanies, nil).AnyTimes()
	waiting := pendingEnd()
	waiting.ID, waiting.Number = 25, 25
	waiting.EndProposal.Companies[0].Confirmed, waiting.EndProposal.Companies[1].Confirmed = false, true
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return([]core.Match{waiting, editableMatch()}, nil).AnyTimes()
	f.pending = map[int64][]core.AftermathRef{1: {{MatchID: 3, MatchNumber: 3, CompanyID: 1, Status: core.AftermathPending}}}
	for _, path := range []string{"/campaigns/7/matches", "/campaigns/7"} {
		body := f.do(f.signedIn(httpGet(path))).Body.String()
		for _, want := range []string{`id="your-turn"`, "Vote the MVP and confirm the end of Match 25", `href="/campaigns/7/matches/25"`, "Start the aftermath of Match 3", `href="/campaigns/7/matches/3/aftermath/1"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %q", path, want)
			}
		}
	}
	g := newFixture(t, Config{})
	g.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	g.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(twoCompanies, nil).AnyTimes()
	g.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return([]core.Match{editableMatch()}, nil)
	if body := g.do(g.signedIn(httpGet("/campaigns/7/matches"))).Body.String(); strings.Contains(body, `id="your-turn"`) {
		t.Error("nothing waits: no Your turn list")
	}
}
