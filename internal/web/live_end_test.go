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

func endChecksFixture() core.EndChecks {
	return core.EndChecks{
		Objective: []core.CompanyObjectives{{CompanyID: 1, Name: "Rangers", Units: []core.EndCheckUnit{
			{UnitID: 5, Title: "Hador", Type: "Warrior", Zone: core.ZoneField, Has: true},
			{UnitID: 6, Title: "Aldor", Type: "Warrior", Zone: core.ZoneRetreated}}}},
		MVP: []core.MVPBallot{{CompanyID: 2, Name: "Raiders", Current: 21, Units: []core.EndCheckUnit{
			{UnitID: 20, Title: "Grashnak", Type: "Captain", Wounds: core.LiveStat{Current: 2}},
			{UnitID: 21, Title: "Snaga", Type: "Orc", Current: true}}}},
		Tally:   []core.MVPTally{{CompanyID: 2, Name: "Raiders", Lines: []core.TallyLine{{UnitID: 21, Title: "Snaga", Votes: 2}}}},
		Pending: []string{"No unit of Rangers has the objective yet", "you have not voted for Raiders"},
	}
}

func TestEndChecksSheetListsOwnUnitsAndBallots(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(endChecksFixture(), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/end?company=1", nil))).Body.String()
	for _, want := range []string{"End checks", "Got the objective · Rangers", "Hador", `aria-checked="true"`, "retreated", "/matches/3/live/1/objective",
		"Select MVP · Raiders", "Your vote", "No MVP", "/matches/3/live/2/mvp", "Snaga", "2 votes", "Close"} {
		if !strings.Contains(body, want) {
			t.Errorf("sheet lacks %q", want)
		}
	}
	if strings.Contains(body, "<html") {
		t.Error("a fragment")
	}
}

func TestEndChecksCardIsEmptyOnceTheMatchIsNotOpen(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(core.EndChecks{}, core.ErrNotFound)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/end?card=1", nil)))
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "" {
		t.Errorf("%d %q", rec.Code, rec.Body.String())
	}
}

func TestEndChecksViewerSeesOnlyTheTally(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveNone)
	ec := endChecksFixture()
	ec.Objective, ec.MVP = nil, nil
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(ec, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/end?card=1", nil))).Body.String()
	if !strings.Contains(body, "Only the players of this match") || !strings.Contains(body, "Snaga") || strings.Contains(body, `role="switch"`) || strings.Contains(body, `role="radio"`) {
		t.Errorf("viewer card: %.400s", body)
	}
}

func TestEndObjectiveAndMVPApplyTheActionsAndOfferUndo(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), core.LiveAction{Kind: core.LiveObjective, UnitID: 6, On: true}).
		Return(core.LiveCompany{}, core.LiveEntry{ID: 50, Text: "Aldor got the objective"}, nil)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(endChecksFixture(), nil).Times(2)
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/objective", url.Values{"unit": {"6"}, "on": {"1"}, "view": {"card"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Aldor got the objective") || !strings.Contains(body, "/live/log/50/undo") || !strings.Contains(body, "endcard") {
		t.Errorf("objective: %d %.300s", rec.Code, body)
	}
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(2), core.LiveAction{Kind: core.LiveMVPVote, TargetCompanyID: 2, UnitID: 0}).
		Return(core.LiveCompany{}, core.LiveEntry{ID: 51, Text: "Frodo withdrew the MVP vote for Raiders"}, nil)
	rec = f.do(f.signedIn(form("/campaigns/7/matches/3/live/2/mvp", url.Values{"unit": {"0"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "withdrew the MVP vote") {
		t.Errorf("mvp: %d", rec.Code)
	}
}

func TestEndChecksRefusalIs422ForPlayersAnd403ForViewers(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	f.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), gomock.Any()).Return(core.LiveCompany{}, core.LiveEntry{}, core.Refuse("Aldor has the objective already."))
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(endChecksFixture(), nil)
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/live/1/objective", url.Values{"unit": {"6"}, "on": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "already") {
		t.Errorf("player: %d", rec.Code)
	}
	g := liveOwnerSetup(t, core.LiveNone)
	g.camp.EXPECT().Apply(gomock.Any(), frodo, int64(7), int64(3), int64(1), gomock.Any()).Return(core.LiveCompany{}, core.LiveEntry{}, core.Refuse("Only the owner of Rangers can change it."))
	g.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(core.EndChecks{}, nil)
	if rec := g.do(g.signedIn(form("/campaigns/7/matches/3/live/1/objective", url.Values{"unit": {"6"}, "on": {"1"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("viewer: %d", rec.Code)
	}
}

func TestEndReminderListsWhatIsOpenAndNeverBlocks(t *testing.T) {
	f := liveOwnerSetup(t, core.LiveEdit)
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(endChecksFixture(), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches/3/live/end/reminder?revision=4", nil))).Body.String()
	for _, want := range []string{"Propose to end this match?", "No unit of Rangers has the objective yet", "you have not voted for Raiders", "Go back", "Propose anyway", `name="revision" value="4"`, "/matches/3/end"} {
		if !strings.Contains(body, want) {
			t.Errorf("reminder lacks %q", want)
		}
	}
}
