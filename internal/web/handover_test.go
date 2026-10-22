package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func TestOverviewOffersHandoverToTheOtherPlayers(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	body := f.do(f.signedInAs(gandalf, httpGet("/campaigns/7"))).Body.String()
	if !strings.Contains(body, `hx-post="/campaigns/7/handover"`) || !strings.Contains(body, `<option value="1">Frodo (Shire)</option>`) || !strings.Contains(body, `<option value="5">Pippin (no company yet)</option>`) || strings.Contains(body, `>Gandalf</option>`) {
		t.Errorf("the organiser can choose the other players only:\n%s", body)
	}
	f = organiserFixture(t, frodo, core.CampaignActive)
	if body := f.do(f.signedIn(httpGet("/campaigns/7"))).Body.String(); strings.Contains(body, "Hand over the campaign") {
		t.Error("a player cannot hand the campaign over")
	}
}

func TestHandOver(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	f.camp.EXPECT().HandOver(gomock.Any(), gandalf, int64(7), int64(1)).Return(nil)
	r := f.signedInAs(gandalf, form("/campaigns/7/handover", url.Values{"organiser": {"1"}}))
	r.Header.Set("HX-Request", "true")
	if rec := f.do(r); rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") != "/campaigns/7" {
		t.Errorf("%d %q", rec.Code, rec.Header().Get("HX-Redirect"))
	}
	f.camp.EXPECT().HandOver(gomock.Any(), gandalf, int64(7), int64(1)).Return(core.Refuse("That player is not a member of this campaign"))
	rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/handover", url.Values{"organiser": {"1"}})))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "not a member") || !strings.Contains(rec.Body.String(), "<details open>") {
		t.Errorf("refused: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(f.signedInAs(gandalf, form("/campaigns/7/handover", url.Values{"organiser": {""}}))); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose the player") {
		t.Errorf("nobody chosen: %d", rec.Code)
	}
	f = organiserFixture(t, frodo, core.CampaignActive)
	if rec := f.do(f.signedIn(form("/campaigns/7/handover", url.Values{"organiser": {"2"}}))); rec.Code != http.StatusForbidden {
		t.Errorf("a player: %d", rec.Code)
	}
}
