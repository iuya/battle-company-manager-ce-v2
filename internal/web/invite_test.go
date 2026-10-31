package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func organiserFixture(t *testing.T, u core.User, status core.CampaignStatus) *fixture {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID, d.Status = gandalf.ID, status
	f.camp.EXPECT().Get(gomock.Any(), u.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, OwnerID: gandalf.ID, Owner: "Gandalf"}, {ID: 2, OwnerID: frodo.ID, Owner: "Frodo"}}, nil).AnyTimes()
	f.camp.EXPECT().Members(gomock.Any(), int64(7)).Return([]core.CampaignMember{{UserID: gandalf.ID, Name: "Gandalf", Organiser: true, Companies: []core.MemberCompany{{ID: 1, Name: "Grey"}}}, {UserID: frodo.ID, Name: "Frodo", Companies: []core.MemberCompany{{ID: 2, Name: "Shire"}}}, {UserID: 5, Name: "Pippin"}}, nil).AnyTimes()
	return f
}

func TestInviteButtonIsForOrganiserAndAdministrators(t *testing.T) {
	for _, u := range []core.User{gandalf, sauron} {
		f := organiserFixture(t, u, core.CampaignActive)
		if body := f.do(f.signedInAs(u, httpGet("/campaigns/7"))).Body.String(); !strings.Contains(body, `hx-post="/campaigns/7/invite"`) || !strings.Contains(body, "Invite players") {
			t.Errorf("%s should see the invite button", u.DisplayName)
		}
	}
	f := organiserFixture(t, frodo, core.CampaignActive)
	if body := f.do(f.signedIn(httpGet("/campaigns/7"))).Body.String(); strings.Contains(body, "Invite players") {
		t.Error("a player does not see the invite button")
	}
	f = organiserFixture(t, gandalf, core.CampaignFinished)
	if body := f.do(f.signedInAs(gandalf, httpGet("/campaigns/7"))).Body.String(); strings.Contains(body, "Invite players") {
		t.Error("no invites for a finished campaign")
	}
}

func TestMakingAnInviteShowsTheLinkOnce(t *testing.T) {
	f := organiserFixture(t, gandalf, core.CampaignActive)
	exp := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	f.camp.EXPECT().Invite(gomock.Any(), gandalf, int64(7)).Return(core.Invite{Token: "tok-123", ExpiresAt: exp}, nil)
	r := f.signedInAs(gandalf, form("/campaigns/7/invite", nil))
	r.Host = "bc.example"
	r.Header.Set("X-Forwarded-Proto", "https")
	rec := f.do(r)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `value="https://bc.example/join/tok-123"`) || !strings.Contains(body, "27 October 2026") || !strings.Contains(body, "Make another link") {
		t.Errorf("%d\n%s", rec.Code, body)
	}
	f = organiserFixture(t, frodo, core.CampaignActive)
	if rec := f.do(f.signedIn(form("/campaigns/7/invite", nil))); rec.Code != http.StatusForbidden {
		t.Errorf("a player: %d", rec.Code)
	}
}

func TestJoinPage(t *testing.T) {
	f := newFixture(t, Config{})
	// signed out: sign in first, and come back
	rec := f.do(httptest.NewRequest("GET", "/join/tok-123", nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?next=%2Fjoin%2Ftok-123" {
		t.Errorf("signed out: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// signed in: a confirmation, nothing changes yet
	f.camp.EXPECT().Preview(gomock.Any(), frodo, "tok-123").Return(core.InvitePreview{CampaignID: 7, CampaignName: "Rohan Rising"}, nil)
	if body := f.do(f.signedIn(httpGet("/join/tok-123"))).Body.String(); !strings.Contains(body, `action="/join/tok-123"`) || !strings.Contains(body, "Join the campaign") || !strings.Contains(body, "Rohan Rising") {
		t.Error("the page asks for confirmation")
	}
	f.camp.EXPECT().Join(gomock.Any(), frodo, "tok-123").Return(int64(7), nil)
	rec = f.do(f.signedIn(form("/join/tok-123", nil)))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/my-company" {
		t.Errorf("joined: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	f.camp.EXPECT().Join(gomock.Any(), frodo, "old").Return(int64(0), core.ErrNotFound)
	rec = f.do(f.signedIn(form("/join/old", nil)))
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "not valid any more") {
		t.Errorf("invalid link: %d", rec.Code)
	}
}

func TestJoinPageForAMemberAndForAnInvalidLink(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Preview(gomock.Any(), frodo, "tok").Return(core.InvitePreview{CampaignID: 7, CampaignName: "Rohan Rising", Member: true}, nil)
	body := f.do(f.signedIn(httpGet("/join/tok"))).Body.String()
	if !strings.Contains(body, "already in") || !strings.Contains(body, `href="/campaigns/7/my-company"`) || strings.Contains(body, "Join the campaign") {
		t.Errorf("a member is told so and gets a link: %s", body)
	}
	f.camp.EXPECT().Preview(gomock.Any(), frodo, "old").Return(core.InvitePreview{}, core.ErrNotFound)
	rec := f.do(f.signedIn(httpGet("/join/old")))
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "not valid any more") || strings.Contains(rec.Body.String(), "Join the campaign") {
		t.Errorf("an invalid link is shown as such before confirming: %d", rec.Code)
	}
}

func TestLoginGoesBackToTheInviteButNeverElsewhere(t *testing.T) {
	f := newFixture(t, Config{})
	if body := f.do(httpGet("/login?next=%2Fjoin%2Ftok")).Body.String(); !strings.Contains(body, `name="next" value="/join/tok"`) {
		t.Error("the login form carries the next page")
	}
	for _, bad := range []string{"https://evil.example/", "//evil.example", "/\\evil", "javascript:alert(1)"} {
		if got := safeNext(bad); got != "" {
			t.Errorf("safeNext(%q) = %q", bad, got)
		}
	}
	f.auth.EXPECT().Login(gomock.Any(), "frodo@shire.me", "pw").Return("tok", time.Now().Add(time.Hour), frodo, nil)
	r := form("/login", map[string][]string{"email": {"frodo@shire.me"}, "password": {"pw"}, "next": {"/join/tok-123"}})
	if rec := f.do(r); rec.Header().Get("Location") != "/join/tok-123" {
		t.Errorf("after signing in: %q", rec.Header().Get("Location"))
	}
}
