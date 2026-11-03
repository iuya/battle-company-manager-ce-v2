package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// pendingEnd is readyToEnd with Rangers (frodo) confirmed and Raiders waiting.
func pendingEnd() core.Match {
	m := readyToEnd()
	on := time.Date(2026, 10, 20, 18, 30, 0, 0, time.UTC)
	m.EndProposal = &core.EndProposal{ProposedBy: "Frodo", ProposedByID: frodo.ID, ProposedOn: on, Companies: []core.EndConfirmation{
		{CompanyID: 1, OwnerID: frodo.ID, CompanyName: "Rangers", Confirmed: true, By: "Frodo", On: on},
		{CompanyID: 2, OwnerID: 50, CompanyName: "Raiders"},
	}}
	return m
}

func TestPendingEndCardForTheProposer(t *testing.T) {
	f := matchWrite(t, pendingEnd())
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"Frodo proposes to end: Rangers won", "Rangers", "Raiders waiting", "Withdraw my confirmation", "Keep playing",
		matchPath + "/end/withdraw", matchPath + "/end/cancel", `id="end-card-slot"`, `data-phase="ending"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"data-confirm-end", "data-end-match", "End without confirmation", "/live/end/reminder"} {
		if strings.Contains(body, bad) {
			t.Errorf("shows %q", bad)
		}
	}
}

func TestPendingEndCardForTheWaitingOwner(t *testing.T) {
	m := pendingEnd()
	m.EndProposal.Companies[0].OwnerID, m.EndProposal.ProposedByID = 50, 50
	m.EndProposal.Companies[1].OwnerID = frodo.ID
	m.Sides[0].Companies[0].OwnerID, m.Sides[1].Companies[0].OwnerID = 50, frodo.ID
	m.EndProposal.Notice = "Hador changed the result; confirm again"
	f := matchWrite(t, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"data-confirm-end", "Keep playing", matchPath + "/end/sheet", "Hador changed the result; confirm again"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "Withdraw my confirmation") {
		t.Error("a waiting owner has nothing to withdraw")
	}
}

func TestPendingEndCardForAViewerHasNoButtons(t *testing.T) {
	m := pendingEnd()
	m.Sides[0].Companies[0].OwnerID, m.Sides[1].Companies[0].OwnerID = 51, 52
	f := matchWrite(t, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	if !strings.Contains(body, "proposes to end") || !strings.Contains(body, "waiting") {
		t.Error("a viewer sees the pending state")
	}
	for _, bad := range []string{"data-confirm-end", "/end/withdraw", "/end/cancel", "/end/override", "Keep playing"} {
		if strings.Contains(body, bad) {
			t.Errorf("a viewer sees %q", bad)
		}
	}
}

func TestOrganiserSeesTheOverrideWithItsWarning(t *testing.T) {
	m := pendingEnd()
	m.Sides[0].Companies[0].OwnerID, m.Sides[1].Companies[0].OwnerID = 51, 52
	m.EndProposal.Companies[0].OwnerID, m.EndProposal.Companies[1].OwnerID = 51, 52
	f := organiserMatch(t, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String()
	for _, want := range []string{"End without confirmation", "Raiders has not confirmed that the game is over. The result and the aftermath will be fixed for everyone.", `name="note"`, matchPath + "/end/override", "Keep playing"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestEndAnswers(t *testing.T) {
	for _, tt := range []struct {
		path, notice string
		expect       func(f *fixture, done core.Match)
	}{
		{"withdraw", "You took back your confirmation.", func(f *fixture, done core.Match) {
			done.Ended = false
			f.camp.EXPECT().WithdrawEnd(gomock.Any(), frodo, int64(7), int64(3), 4).Return(done, nil)
		}},
		{"cancel", "Ending cancelled: the match goes on.", func(f *fixture, done core.Match) {
			done.Ended, done.EndProposal = false, nil
			f.camp.EXPECT().CancelEnd(gomock.Any(), frodo, int64(7), int64(3), 4).Return(done, nil)
		}},
	} {
		t.Run(tt.path, func(t *testing.T) {
			f := matchWrite(t, pendingEnd())
			now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
			done := readyToEnd()
			done.Ended, done.EndedOn, done.Revision = true, &now, 5
			tt.expect(f, done)
			rec := f.do(f.signedIn(form(matchPath+"/end/"+tt.path, url.Values{"revision": {"4"}})))
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tt.notice) {
				t.Errorf("%d\n%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func organiserMatch(t *testing.T, m core.Match) *fixture {
	t.Helper()
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = frodo.ID
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Match(gomock.Any(), int64(7), m.ID).Return(m, nil).AnyTimes()
	return f
}

func TestOverrideNeedsANoteAndKeepsTheDialogOpen(t *testing.T) {
	m := pendingEnd()
	m.Sides[1].Companies[0].OwnerID = 52
	f := organiserMatch(t, m)
	f.camp.EXPECT().OverrideEnd(gomock.Any(), frodo, int64(7), int64(3), 4, "").Return(core.Match{}, errors.Join(core.ErrInvalidMatch, errors.New("the note must be between 1 and 200 characters")))
	rec := f.do(f.signedIn(form(matchPath+"/end/override", url.Values{"revision": {"4"}, "note": {"  "}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "The note must be between 1 and 200 characters") {
		t.Errorf("%d\n%s", rec.Code, rec.Body.String())
	}
}

func TestOverrideEnds(t *testing.T) {
	f := matchWrite(t, pendingEnd())
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	done := readyToEnd()
	done.Ended, done.EndedOn = true, &now
	f.camp.EXPECT().OverrideEnd(gomock.Any(), frodo, int64(7), int64(3), 4, "Raiders left the table").Return(done, nil)
	rec := f.do(f.signedIn(form(matchPath+"/end/override", url.Values{"revision": {"4"}, "note": {" Raiders left the table "}})))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ended without the confirmation") {
		t.Errorf("%d", rec.Code)
	}
}

func TestEndRefusedAndStale(t *testing.T) {
	f := matchWrite(t, pendingEnd())
	f.camp.EXPECT().EndChecks(gomock.Any(), frodo, int64(7), int64(3)).Return(core.EndChecks{}, core.ErrNotFound)
	f.camp.EXPECT().ConfirmEnd(gomock.Any(), frodo, int64(7), int64(3), 4).Return(core.Match{}, core.Refuse("You have no company waiting to confirm."))
	rec := f.do(f.signedIn(form(matchPath+"/end/confirm", url.Values{"revision": {"4"}, "winner": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "You have no company waiting to confirm.") {
		t.Errorf("refused: %d", rec.Code)
	}
	f = matchWrite(t, pendingEnd())
	f.camp.EXPECT().CancelEnd(gomock.Any(), frodo, int64(7), int64(3), 3).Return(core.Match{}, core.ErrMatchChanged)
	rec = f.do(f.signedIn(form(matchPath+"/end/cancel", url.Values{"revision": {"3"}})))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "changed this match while you were editing") {
		t.Errorf("stale: %d", rec.Code)
	}
}

func TestEndCardRefreshSendsTheHeaderButtonOutOfBand(t *testing.T) {
	f := matchWrite(t, pendingEnd())
	body := htmxGet(f, matchPath+"/end/card").Body.String()
	if !strings.Contains(body, "proposes to end") || !strings.Contains(body, `hx-swap-oob="true"`) || strings.Contains(body, "data-end-match") {
		t.Errorf("pending:\n%s", body)
	}
	f = matchWrite(t, readyToEnd())
	body = htmxGet(f, matchPath+"/end/card").Body.String()
	if strings.Contains(body, "proposes to end") || !strings.Contains(body, "data-end-match") {
		t.Errorf("cancelled:\n%s", body)
	}
}

func TestHistoryShowsWhoConfirmedTheEnd(t *testing.T) {
	m := readyToEnd()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	m.Ended, m.EndedOn = true, &now
	m.History = []core.MatchEvent{{Kind: "ended", By: "Frodo", On: now, Detail: "overriding Raiders: left the table"}}
	f := matchWrite(t, m)
	if body := f.do(f.signedIn(httptest.NewRequest("GET", matchPath, nil))).Body.String(); !strings.Contains(body, "overriding Raiders: left the table") {
		t.Error("the history shows the detail")
	}
}

func pendingSides() []core.MatchSide {
	return []core.MatchSide{
		{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID}}},
		{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders", OwnerID: 50}}},
	}
}

func pendingOnMatch(m *core.Match) {
	p := pendingEnd()
	m.EndProposal, m.Result = p.EndProposal, p.Result
}

func TestLiveBannerShowsThePendingEndOnTheCompanyPage(t *testing.T) {
	f := liveSetupMatch(t, core.LiveEdit, nil, pendingOnMatch, pendingSides()...)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	for _, want := range []string{`id="live-end"`, "Frodo proposes to end: Rangers won", "Waiting for Raiders", "Withdraw", "Keep playing", "/campaigns/7/matches/3/end/withdraw", `name="view" value="banner"`,
		`/campaigns/7/matches/3/live/end/banner?company=1`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"turn-blocked", "Start turn", "data-end-match"} {
		if strings.Contains(body, bad) {
			t.Errorf("a pending end: no %q", bad)
		}
	}
}

func TestLiveBannerIsEmptyWithoutAPendingEnd(t *testing.T) {
	f := liveSetupMatch(t, core.LiveEdit, nil, nil, pendingSides()...)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	if strings.Contains(body, "proposes to end") || !strings.Contains(body, "data-end-match") {
		t.Error("nothing pending: End match is offered")
	}
}

func TestLiveBannerAnswerRefreshesThePageWhenTheMatchEnded(t *testing.T) {
	f := liveSetupMatch(t, core.LiveEdit, nil, pendingOnMatch, pendingSides()...)
	now := time.Now()
	f.camp.EXPECT().CancelEnd(gomock.Any(), frodo, int64(7), int64(3), 6).Return(core.Match{ID: 3, Ended: true, EndedOn: &now}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/end/cancel", url.Values{"revision": {"6"}, "view": {"banner"}, "company": {"1"}})))
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Error("an ended match reloads the page")
	}
}

func TestLiveBannerRefusalIsShownInTheBanner(t *testing.T) {
	f := liveSetupMatch(t, core.LiveEdit, nil, pendingOnMatch, pendingSides()...)
	f.camp.EXPECT().CancelEnd(gomock.Any(), frodo, int64(7), int64(3), 6).Return(core.Match{}, core.Refuse("Nobody has proposed to end this match."))
	rec := f.do(f.signedIn(form("/campaigns/7/matches/3/end/cancel", url.Values{"revision": {"6"}, "view": {"banner"}, "company": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Nobody has proposed to end this match.") {
		t.Errorf("%d\n%s", rec.Code, rec.Body.String())
	}
}

func TestLiveBannerOfAWaitingOwnerOpensTheConfirmSheet(t *testing.T) {
	f := liveSetupMatch(t, core.LiveEdit, nil, func(m *core.Match) {
		pendingOnMatch(m)
		m.EndProposal.Companies[0].OwnerID, m.EndProposal.Companies[1].OwnerID = 50, frodo.ID
		m.EndProposal.Companies[0].Confirmed, m.EndProposal.Companies[1].Confirmed = true, false
		m.Sides[0].Companies[0].OwnerID, m.Sides[1].Companies[0].OwnerID = 50, frodo.ID
	}, pendingSides()...)
	body := htmxGet(f, "/campaigns/7/matches/3/live/end/banner?company=1").Body.String()
	if !strings.Contains(body, "data-confirm-end") || !strings.Contains(body, "/end/sheet?company=1") {
		t.Errorf("banner:\n%s", body)
	}
}

func TestLiveBannerNamesEveryBrokenCompany(t *testing.T) {
	f := liveSetupMatch(t, core.LiveEdit, nil, nil, pendingSides()...)
	f.breaks = map[int64]core.LiveBreak{2: {Broken: true, Casualties: 8, StartingModels: 15}, 1: {Casualties: 2, StartingModels: 12}}
	body := htmxGet(f, "/campaigns/7/matches/3/live/end/banner?company=1").Body.String()
	if !strings.Contains(body, "Raiders is broken (8 of 15 casualties)") || strings.Contains(body, "Rangers is broken") {
		t.Errorf("broken notice:\n%s", body)
	}
}
