package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestManageCardIsForManagersAndShowsTheEndingSteps(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	body := f.do(f.signedInAs(gandalf, httpGet("/campaigns/7"))).Body.String()
	for _, want := range []string{`id="manage"`, "Pause campaign", "End campaign…", "Step 1 of 3", "Step 2 of 3", "Step 3 of 3", "End the campaign for good", `data-name="Siege"`, "Only an administrator can reactivate"} {
		if !strings.Contains(body, want) {
			t.Errorf("manage card lacks %q", want)
		}
	}
	if strings.Contains(body, "Reactivate campaign") {
		t.Error("the organiser cannot reactivate")
	}
	g := organiserFixture(t, frodo, core.CampaignActive)
	if strings.Contains(g.do(g.signedIn(httpGet("/campaigns/7"))).Body.String(), `id="manage"`) {
		t.Error("a player has no manage card")
	}
}

func TestManageCardOfAFinishedCampaign(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignFinished)
	body := f.do(f.signedInAs(gandalf, httpGet("/campaigns/7"))).Body.String()
	if !strings.Contains(body, "only an administrator can reactivate it") || strings.Contains(body, "Reactivate campaign") || strings.Contains(body, "End campaign…") {
		t.Error("organiser of an ended campaign: final")
	}
	g := organiserFixture(t, sauron, core.CampaignFinished)
	if body := g.do(g.signedInAs(sauron, httpGet("/campaigns/7"))).Body.String(); !strings.Contains(body, "Reactivate campaign") {
		t.Error("an administrator can reactivate")
	}
	p := organiserFixture(t, gandalf, core.CampaignPaused)
	if body := p.do(p.signedInAs(gandalf, httpGet("/campaigns/7"))).Body.String(); !strings.Contains(body, "Resume campaign") || strings.Contains(body, "Invite players") {
		t.Error("paused: resume, and no new invitation")
	}
}

func TestSetCampaignStatusRedirectsOrShowsTheRefusal(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().SetCampaignStatus(gomock.Any(), gandalf, int64(7), core.CampaignFinished, "Siege").Return(nil)
	r := f.signedInAs(gandalf, form("/campaigns/7/status", url.Values{"to": {"finished"}, "confirm": {"Siege"}}))
	r.Header.Set("HX-Request", "true")
	if rec := f.do(r); rec.Header().Get("HX-Redirect") != "/campaigns/7" {
		t.Errorf("redirect: %d %v", rec.Code, rec.Header())
	}
	f.camp.EXPECT().SetCampaignStatus(gomock.Any(), gandalf, int64(7), core.CampaignFinished, "x").Return(core.Refuse("Type the name of the campaign to end it."))
	rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/status", url.Values{"to": {"finished"}, "confirm": {"x"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Type the name of the campaign") {
		t.Errorf("refusal: %d", rec.Code)
	}
	g := organiserFixture(t, frodo, core.CampaignActive)
	if rec := g.do(g.signedIn(form("/campaigns/7/status", url.Values{"to": {"paused"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("a player: %d", rec.Code)
	}
}

func TestInviteInvalidateAndRegenerate(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().InvalidateInvites(gomock.Any(), gandalf, int64(7)).Return(2, nil)
	rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/invites/invalidate", nil)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "2 links stopped working") {
		t.Errorf("invalidate: %d %.200s", rec.Code, rec.Body.String())
	}
	f.camp.EXPECT().RegenerateInvite(gomock.Any(), gandalf, int64(7)).Return(core.Invite{Token: "new-tok"}, nil)
	r := f.signedInAs(gandalf, form("/campaigns/7/invites/regenerate", nil))
	r.Host = "bc.example"
	rec = f.do(r)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/join/new-tok") || !strings.Contains(rec.Body.String(), "earlier links stopped") {
		t.Errorf("regenerate: %d", rec.Code)
	}
	f.camp.EXPECT().RegenerateInvite(gomock.Any(), gandalf, int64(7)).Return(core.Invite{}, core.ErrCampaignPaused)
	if rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/invites/regenerate", url.Values{"again": {"1"}}))); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "paused") {
		t.Errorf("paused: %d", rec.Code)
	}
}

func TestJoinPageOfAPausedCampaignAndNewMatchWhilePaused(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Preview(gomock.Any(), frodo, "tok").Return(core.InvitePreview{CampaignID: 7, CampaignName: "Siege", Paused: true}, nil)
	body := f.do(f.signedIn(httpGet("/join/tok"))).Body.String()
	if !strings.Contains(body, "is paused") || strings.Contains(body, "Join the campaign</button>") {
		t.Error("a paused campaign cannot be joined")
	}
	f.camp.EXPECT().Join(gomock.Any(), frodo, "tok").Return(int64(0), core.ErrCampaignPaused)
	if rec := f.do(f.signedIn(form("/join/tok", nil))); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "paused") {
		t.Errorf("join refused: %d", rec.Code)
	}
	g := organiserFixture(t, gandalf, core.CampaignPaused)
	if body := g.do(g.signedInAs(gandalf, httpGet("/campaigns/7/matches"))).Body.String(); strings.Contains(body, "New match") || !strings.Contains(body, "paused") {
		t.Error("no New match while paused")
	}
}
