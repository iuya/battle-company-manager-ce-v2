package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var (
	stewardS = core.User{ID: 8, Email: "steward@x.me", DisplayName: "Steward", Role: core.RoleAdmin}
	secretT  = strings.Repeat("A", 43)
)

func (f *fixture) asUser(r *http.Request, u core.User) *http.Request {
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(u, time.Time{}, nil).AnyTimes()
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	return r
}

func rowsOf(ids ...core.AdminUser) []core.AdminUser { return ids }

func target(id int64, name string, role core.Role, can bool) core.AdminUser {
	return core.AdminUser{User: core.User{ID: id, Email: name + "@x.me", DisplayName: name, Role: role, CanCreateCampaigns: can}, CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}
}

// TestUsersControlsPerActorAndTarget plays the page for every actor and target level: the controls shown are exactly the ones the
// permission matrix lets the actor use, and no Superuser option exists anywhere.
func TestUsersControlsPerActorAndTarget(t *testing.T) {
	targets := []core.AdminUser{target(21, "plain", core.RoleUser, false), target(22, "organiser", core.RoleUser, true), target(23, "administrator", core.RoleAdmin, false), target(24, "superuser", core.RoleSuperuser, false)}
	for _, actor := range []core.User{stewardS, superG} {
		f := newFixture(t, Config{})
		f.auth.EXPECT().ListUsers(gomock.Any(), actor, "", userPageSize, 0).Return(targets, 4, nil)
		body := f.do(f.asUser(httptest.NewRequest("GET", "/admin/users", nil), actor)).Body.String()
		has := func(sub string) bool { return strings.Contains(body, sub) }
		for _, id := range []string{"21", "22"} {
			if !has(`hx-post="/admin/users/` + id + `/permissions"`) {
				t.Errorf("%s: no permission form for %s", actor.Role, id)
			}
		}
		if has(`value="superuser"`) || has("Superuser</span>") {
			t.Errorf("%s: a Superuser option is offered", actor.Role)
		}
		if has(`hx-post="/admin/users/24/permissions"`) {
			t.Errorf("%s: a form for the superuser account", actor.Role)
		}
		if actor.IsSuperuser() {
			if !has(`hx-post="/admin/users/23/permissions"`) || strings.Count(body, `name="role" value="admin"`) != 3 {
				t.Errorf("superuser: Administrator switch missing on plain/organiser/administrator rows (%d)", strings.Count(body, `name="role" value="admin"`))
			}
			if !has("Set from the console (make grant).") || !has("Recovery link") {
				t.Error("superuser: console note or recovery card missing")
			}
		} else {
			if has(`name="role" value="admin"`) || has("Recovery link") || has(`/recovery`) {
				t.Error("administrator: sees the Administrator switch or recovery links")
			}
			if has(`hx-post="/admin/users/23/permissions"`) || !has("Administrators are managed by superusers.") || !has("Superusers are set from the console.") {
				t.Error("administrator: controls on an administrator, or notes missing")
			}
			if strings.Contains(body, "Rules files") {
				t.Error("administrator: Rules files tab")
			}
		}
		if !has("Invitations") {
			t.Errorf("%s: no Invitations tab", actor.Role)
		}
	}
}

func TestPermissionsForgedAndRefused(t *testing.T) {
	f := newFixture(t, Config{})
	// the matrix refuses: 403 with the reason, for a forged role=admin by an administrator and role=superuser by anybody
	f.auth.EXPECT().SetUserPermissions(gomock.Any(), stewardS, int64(21), core.RoleAdmin, false).Return(core.Forbid("Only a superuser can change an administrator"))
	rec := f.do(htmxReq(f.asUser(form("/admin/users/21/permissions", url.Values{"role": {"admin"}, "email": {"plain@x.me"}}), stewardS), "user-21"))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Only a superuser can change an administrator") {
		t.Errorf("admin forging role=admin: %d %s", rec.Code, rec.Body)
	}
	f = newFixture(t, Config{})
	f.auth.EXPECT().SetUserPermissions(gomock.Any(), superG, int64(21), core.RoleSuperuser, false).Return(core.Forbid("Superusers are set from the console (make grant)"))
	rec = f.do(htmxReq(f.asSuper(form("/admin/users/21/permissions", url.Values{"role": {"superuser"}, "email": {"plain@x.me"}})), "user-21"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("forged superuser: %d", rec.Code)
	}
	// an administrator makes an organiser: role defaults to user
	f = newFixture(t, Config{})
	f.auth.EXPECT().SetUserPermissions(gomock.Any(), stewardS, int64(21), core.RoleUser, true).Return(nil)
	f.auth.EXPECT().ListUsers(gomock.Any(), stewardS, "plain@x.me", 200, 0).Return(rowsOf(target(21, "plain", core.RoleUser, true)), 1, nil)
	rec = f.do(htmxReq(f.asUser(form("/admin/users/21/permissions", url.Values{"can_create": {"on"}, "email": {"plain@x.me"}}), stewardS), "user-21"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Saved") {
		t.Errorf("organiser switch: %d %s", rec.Code, rec.Body)
	}
}

func TestRecoveryCardGenerateAndRevoke(t *testing.T) {
	f := newFixture(t, Config{})
	exp := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	pending := target(21, "plain", core.RoleUser, false)
	pending.Recovery = &core.RecoveryStatus{State: core.RecoveryPending, ExpiresAt: exp, CreatedByName: "Gandalf"}
	f.rec.EXPECT().Generate(gomock.Any(), superG, int64(21), 72*time.Hour).Return(core.RecoveryLink{ID: 1, UserID: 21, Token: secretT, ExpiresAt: exp}, nil)
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "plain@x.me", 200, 0).Return(rowsOf(pending), 1, nil)
	rec := f.do(htmxReq(f.asSuper(form("/admin/users/21/recovery", url.Values{"hours": {"72"}, "email": {"plain@x.me"}})), "user-21"))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "/recover/"+secretT) || rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(body, "Open this link to set a new password") {
		t.Errorf("generate: %d %s", rec.Code, body)
	}
	if !strings.Contains(body, `data-recovery-state="pending"`) || !strings.Contains(body, "Revoke link") {
		t.Error("the pending state and the Revoke button are shown")
	}
	// the page that lists users never carries the token
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "", userPageSize, 0).Return(rowsOf(pending), 1, nil)
	if page := f.do(f.asSuper(httptest.NewRequest("GET", "/admin/users", nil))).Body.String(); strings.Contains(page, secretT) {
		t.Error("the list shows the token")
	}
	// refusals: own account, none pending
	f.rec.EXPECT().Generate(gomock.Any(), superG, int64(9), time.Duration(0)).Return(core.RecoveryLink{}, core.Refuse("Use the Account page to change your own password."))
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "", 200, 0).Return(rowsOf(target(9, "me", core.RoleSuperuser, false)), 1, nil)
	rec = f.do(htmxReq(f.asSuper(form("/admin/users/9/recovery", nil)), "user-9"))
	if rec.Code != 422 {
		t.Errorf("own account: %d %s", rec.Code, rec.Body)
	}
	f.rec.EXPECT().Revoke(gomock.Any(), superG, int64(21)).Return(nil)
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "plain@x.me", 200, 0).Return(rowsOf(target(21, "plain", core.RoleUser, false)), 1, nil)
	rec = f.do(htmxReq(f.asSuper(form("/admin/users/21/recovery/revoke", url.Values{"email": {"plain@x.me"}})), "user-21"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "The link is revoked.") {
		t.Errorf("revoke: %d %s", rec.Code, rec.Body)
	}
	f.rec.EXPECT().Revoke(gomock.Any(), superG, int64(22)).Return(core.Refuse("There is no pending link for this account."))
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "", 200, 0).Return(rowsOf(target(22, "x", core.RoleUser, false)), 1, nil)
	if rec = f.do(htmxReq(f.asSuper(form("/admin/users/22/recovery/revoke", nil)), "user-22")); rec.Code != 422 {
		t.Errorf("revoke none: %d", rec.Code)
	}
	// a forged call by an administrator is a 404 (the route is not theirs); a service refusal of a non-superuser is a 403
	f.rec.EXPECT().Generate(gomock.Any(), superG, int64(30), time.Duration(0)).Return(core.RecoveryLink{}, core.Forbid("Only a superuser can make recovery links"))
	if rec = f.do(htmxReq(f.asSuper(form("/admin/users/30/recovery", nil)), "user-30")); rec.Code != 403 {
		t.Errorf("forbidden: %d", rec.Code)
	}
}

func TestRecoveryStatesAreWorded(t *testing.T) {
	f := newFixture(t, Config{})
	at := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	mk := func(id int64, st core.RecoveryStatus) core.AdminUser {
		u := target(id, "u", core.RoleUser, false)
		u.Recovery = &st
		return u
	}
	list := []core.AdminUser{
		mk(31, core.RecoveryStatus{State: core.RecoveryUsed, UsedAt: &at}),
		mk(32, core.RecoveryStatus{State: core.RecoveryExpired, ExpiresAt: at}),
		mk(33, core.RecoveryStatus{State: core.RecoveryRevoked, RevokedAt: &at, RevokedByName: "Gandalf"}),
		mk(34, core.RecoveryStatus{State: core.RecoveryInvalidated, InvalidatedAt: &at, Reason: core.RecoveryByLogin}),
		mk(35, core.RecoveryStatus{State: core.RecoveryInvalidated, InvalidatedAt: &at, Reason: core.RecoveryByAccess}),
	}
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "", userPageSize, 0).Return(list, 5, nil)
	body := f.do(f.asSuper(httptest.NewRequest("GET", "/admin/users", nil))).Body.String()
	for _, want := range []string{"Link used on 4 March 2026", "Link expired on", "Link revoked by Gandalf", "the person signed in.", "the person opened the app."} {
		if !strings.Contains(body, want) {
			t.Errorf("lacks %q", want)
		}
	}
}

func invitation(id int64, state core.InvitationState) core.AccountInvitation {
	return core.AccountInvitation{ID: id, Kind: core.InvitationNamed, Email: "new@x.me", MaxUses: 1, CreatedByName: "Gandalf", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), State: state}
}

func TestInvitationsPageAndFilter(t *testing.T) {
	f := newFixture(t, Config{})
	used := invitation(2, core.InvitationUsed)
	used.Uses, used.Accounts = 1, []core.InvitationAccount{{UserID: 5, DisplayName: "Newbie"}}
	f.inv.EXPECT().List(gomock.Any(), stewardS, core.InvitationFilter{}, invitationPageSize, 0).Return([]core.AccountInvitation{invitation(1, core.InvitationPending), used}, 2, nil)
	body := f.do(f.asUser(httptest.NewRequest("GET", "/admin/invitations", nil), stewardS)).Body.String()
	for _, want := range []string{"Create invitation", "Named (one person)", "Anonymous (anyone with the link)", "new@x.me", `hx-post="/admin/invitations/1/revoke"`, "Newbie", "used 1 of 1", `data-state="used"`, "Created by Gandalf"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(body, `hx-post="/admin/invitations/2/revoke"`) {
		t.Error("a used invitation offers Revoke")
	}
	f.inv.EXPECT().List(gomock.Any(), stewardS, core.InvitationFilter{State: core.InvitationPending}, invitationPageSize, 0).Return([]core.AccountInvitation{invitation(1, core.InvitationPending)}, 1, nil)
	part := f.do(htmxReq(f.asUser(httptest.NewRequest("GET", "/admin/invitations?state=pending", nil), stewardS), "invitation-list")).Body.String()
	if strings.Contains(part, "<html") || !strings.Contains(part, "1 invitation") {
		t.Errorf("filter partial: %s", part)
	}
}

func TestInvitationCreate(t *testing.T) {
	f := newFixture(t, Config{})
	exp := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	created := core.CreatedAccountInvitation{AccountInvitation: core.AccountInvitation{ID: 3, Kind: core.InvitationNamed, Email: "a<b@x.me", MaxUses: 1, ExpiresAt: exp}, Token: secretT}
	f.inv.EXPECT().Create(gomock.Any(), stewardS, core.NewAccountInvitation{Kind: core.InvitationNamed, Email: "a<b@x.me", CanCreateCampaigns: true, Expiry: 3 * 24 * time.Hour}).Return(created, nil)
	rec := f.do(htmxReq(f.asUser(form("/admin/invitations", url.Values{"kind": {"named"}, "email": {"a<b@x.me"}, "days": {"3"}, "can_create": {"on"}}), stewardS), "invitation-create"))
	body := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("HX-Trigger") != "invitations-changed" {
		t.Errorf("create: %d %v", rec.Code, rec.Header())
	}
	if !strings.Contains(body, "/register/"+secretT+"?email=a%3Cb%40x.me") || strings.Contains(body, "a<b@x.me") || !strings.Contains(body, "It works once and expires on 7 November 2026") {
		t.Errorf("message: %s", body)
	}
	if strings.Contains(body, "<script") {
		t.Error("inline script")
	}
	// anonymous
	f.inv.EXPECT().Create(gomock.Any(), stewardS, core.NewAccountInvitation{Kind: core.InvitationAnonymous, MaxUses: 5, Expiry: 7 * 24 * time.Hour}).
		Return(core.CreatedAccountInvitation{AccountInvitation: core.AccountInvitation{Kind: core.InvitationAnonymous, MaxUses: 5, ExpiresAt: exp}, Token: secretT}, nil)
	rec = f.do(htmxReq(f.asUser(form("/admin/invitations", url.Values{"kind": {"anonymous"}, "max_uses": {"5"}, "days": {"7"}}), stewardS), "invitation-create"))
	if !strings.Contains(rec.Body.String(), "It works for up to 5 people") || strings.Contains(rec.Body.String(), "?email=") {
		t.Errorf("anonymous: %s", rec.Body)
	}
	// refusals keep the form
	f.inv.EXPECT().Create(gomock.Any(), stewardS, gomock.Any()).Return(core.CreatedAccountInvitation{}, core.Refuse("There is already an account with this address."))
	rec = f.do(htmxReq(f.asUser(form("/admin/invitations", url.Values{"kind": {"named"}, "email": {"old@x.me"}, "days": {"7"}}), stewardS), "invitation-create"))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "already an account") || !strings.Contains(rec.Body.String(), `value="old@x.me"`) {
		t.Errorf("refused: %d %s", rec.Code, rec.Body)
	}
	rec = f.do(htmxReq(f.asUser(form("/admin/invitations", url.Values{"kind": {"anonymous"}, "max_uses": {"many"}, "days": {"7"}}), stewardS), "invitation-create"))
	if rec.Code != 422 {
		t.Errorf("bad number: %d", rec.Code)
	}
	f.inv.EXPECT().Create(gomock.Any(), stewardS, gomock.Any()).Return(core.CreatedAccountInvitation{}, errors.Join(core.ErrInvalidInput, errors.New("the invitation must expire within 30 days")))
	if rec = f.do(htmxReq(f.asUser(form("/admin/invitations", url.Values{"kind": {"named"}, "email": {"x@x.me"}, "days": {"99"}}), stewardS), "invitation-create")); rec.Code != 422 || !strings.Contains(rec.Body.String(), "30 days") {
		t.Errorf("range: %d %s", rec.Code, rec.Body)
	}
}

func TestInvitationRevoke(t *testing.T) {
	f := newFixture(t, Config{})
	rev := invitation(1, core.InvitationRevoked)
	f.inv.EXPECT().Revoke(gomock.Any(), stewardS, int64(1)).Return(rev, nil)
	rec := f.do(htmxReq(f.asUser(form("/admin/invitations/1/revoke", nil), stewardS), "invitation-1"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `data-state="revoked"`) || strings.Contains(rec.Body.String(), "/revoke") {
		t.Errorf("revoke: %d %s", rec.Code, rec.Body)
	}
	f.inv.EXPECT().Revoke(gomock.Any(), stewardS, int64(2)).Return(core.AccountInvitation{}, core.Refuse("This invitation is already used."))
	rec = f.do(htmxReq(f.asUser(form("/admin/invitations/2/revoke", nil), stewardS), "invitation-2"))
	if rec.Code != 422 || rec.Header().Get("HX-Retarget") != "#invitation-2-msg" || !strings.Contains(rec.Body.String(), "already used") {
		t.Errorf("refused: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	f.inv.EXPECT().Revoke(gomock.Any(), stewardS, int64(3)).Return(core.AccountInvitation{}, core.ErrNotFound)
	if rec = f.do(htmxReq(f.asUser(form("/admin/invitations/3/revoke", nil), stewardS), "invitation-3")); rec.Code != 404 {
		t.Errorf("unknown: %d", rec.Code)
	}
	f = newFixture(t, Config{})
	f.inv.EXPECT().Revoke(gomock.Any(), superG, int64(4)).Return(core.AccountInvitation{}, core.Forbid("no"))
	if rec = f.do(htmxReq(f.asSuper(form("/admin/invitations/4/revoke", nil)), "invitation-4")); rec.Code != 403 {
		t.Errorf("forbidden: %d", rec.Code)
	}
}

func assertSecretHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	h := rec.Header()
	if h.Get("Cache-Control") != "no-store" || h.Get("Referrer-Policy") != "no-referrer" || h.Get("X-Robots-Tag") != "noindex" {
		t.Errorf("secret page headers: %v", h)
	}
}

func TestRegisterPage(t *testing.T) {
	f := newFixture(t, Config{})
	f.inv.EXPECT().Preview(gomock.Any(), secretT).Return(core.InvitationPreview{Kind: core.InvitationNamed, Email: "stored@x.me"}, nil)
	rec := f.do(httptest.NewRequest("GET", "/register/"+secretT+"?email=forged@evil.me", nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `value="stored@x.me"`) || strings.Contains(body, "forged@evil.me") || strings.Contains(body, `name="email"`) {
		t.Errorf("named: %d %s", rec.Code, body)
	}
	assertSecretHeaders(t, rec)
	f.inv.EXPECT().Preview(gomock.Any(), secretT).Return(core.InvitationPreview{Kind: core.InvitationAnonymous}, nil)
	if body = f.do(httptest.NewRequest("GET", "/register/"+secretT, nil)).Body.String(); !strings.Contains(body, `name="email"`) {
		t.Error("an anonymous invitation asks for the email")
	}
}

// TestRegisterFailuresAreAllTheSame: every cause of a dead link answers the very same page and status.
func TestRegisterFailuresAreAllTheSame(t *testing.T) {
	var gets, posts []*httptest.ResponseRecorder
	for _, token := range []string{"x", secretT, strings.Repeat("B", 43)} { // malformed, then the service says unusable whatever the reason
		f := newFixture(t, Config{})
		f.inv.EXPECT().Preview(gomock.Any(), token).Return(core.InvitationPreview{}, core.ErrLinkUnusable)
		gets = append(gets, f.do(httptest.NewRequest("GET", "/register/"+token, nil)))
		f.inv.EXPECT().Register(gomock.Any(), token, gomock.Any()).Return(core.RegisteredSession{}, core.ErrLinkUnusable)
		posts = append(posts, f.do(form("/register/"+token, url.Values{"email": {"taken@x.me"}, "name": {"N"}, "password": {"longenough1"}, "confirm": {"longenough1"}})))
	}
	norm := func(rec *httptest.ResponseRecorder, tok string) string {
		return strings.ReplaceAll(rec.Body.String(), tok, "")
	}
	for i, rec := range gets {
		if rec.Code != 404 || !strings.Contains(rec.Body.String(), core.TalkToAdministrator) {
			t.Errorf("get %d: %d", i, rec.Code)
		}
		assertSecretHeaders(t, rec)
		if norm(rec, "") != norm(gets[0], "") {
			t.Errorf("get %d differs", i)
		}
	}
	for i, rec := range posts {
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), core.TalkToAdministrator) || rec.Body.String() != posts[0].Body.String() {
			t.Errorf("post %d: %d", i, rec.Code)
		}
	}
}

func TestRegisterSuccessSignsInAndFields(t *testing.T) {
	f := newFixture(t, Config{})
	exp := time.Now().Add(time.Hour)
	f.inv.EXPECT().Register(gomock.Any(), secretT, core.Registration{Email: "me@x.me", DisplayName: "Me", Password: "longenough1"}).
		Return(core.RegisteredSession{Token: "sess", Expires: exp, User: core.User{ID: 3}}, nil)
	rec := f.do(form("/register/"+secretT, url.Values{"email": {"me@x.me"}, "name": {"Me"}, "password": {"longenough1"}, "confirm": {"longenough1"}}))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	var c *http.Cookie
	for _, k := range rec.Result().Cookies() {
		if k.Name == sessionCookie {
			c = k
		}
	}
	if c == nil || c.Value != "sess" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie %+v", c)
	}
	assertSecretHeaders(t, rec)

	// mismatch: the form again, nothing registered (Preview only)
	f = newFixture(t, Config{})
	f.inv.EXPECT().Preview(gomock.Any(), secretT).Return(core.InvitationPreview{Kind: core.InvitationAnonymous}, nil)
	rec = f.do(form("/register/"+secretT, url.Values{"email": {"me@x.me"}, "name": {"Me"}, "password": {"longenough1"}, "confirm": {"other"}}))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "do not match") || !strings.Contains(rec.Body.String(), `value="me@x.me"`) {
		t.Errorf("mismatch: %d", rec.Code)
	}
	// weak password: inline reasons, the link is fine
	f = newFixture(t, Config{})
	f.inv.EXPECT().Register(gomock.Any(), secretT, gomock.Any()).Return(core.RegisteredSession{}, errors.Join(core.ErrInvalidUser, errors.New("the password must be between 10 and 128 characters")))
	f.inv.EXPECT().Preview(gomock.Any(), secretT).Return(core.InvitationPreview{Kind: core.InvitationNamed, Email: "s@x.me"}, nil)
	rec = f.do(form("/register/"+secretT, url.Values{"name": {"Me"}, "password": {"short"}, "confirm": {"short"}}))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "The password must be between 10 and 128") || !strings.Contains(rec.Body.String(), "s@x.me") {
		t.Errorf("weak: %d %s", rec.Code, rec.Body)
	}
	// service failure: 500 without a cause
	f = newFixture(t, Config{})
	f.inv.EXPECT().Register(gomock.Any(), secretT, gomock.Any()).Return(core.RegisteredSession{}, errors.New("db down"))
	if rec = f.do(form("/register/"+secretT, url.Values{"password": {"a"}, "confirm": {"a"}})); rec.Code != 500 || strings.Contains(rec.Body.String(), "db down") {
		t.Errorf("500: %d", rec.Code)
	}
}

func TestRegisterSignedInVisitorIsSentHome(t *testing.T) {
	f := newFixture(t, Config{})
	// no Preview or Register expectation: the token is not touched
	for _, r := range []*http.Request{httptest.NewRequest("GET", "/register/"+secretT, nil), form("/register/"+secretT, nil)} {
		rec := f.do(f.signedIn(r))
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
			t.Errorf("%s: %d %q", r.Method, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestRegisterAndRecoverAreThrottled(t *testing.T) {
	f := newFixture(t, Config{})
	// looking at dead links is cheap and never counts, however often
	f.inv.EXPECT().Preview(gomock.Any(), secretT).Return(core.InvitationPreview{}, core.ErrLinkUnusable).Times(maxFailsPerAddress + 1)
	for range maxFailsPerAddress + 1 {
		if rec := f.do(httptest.NewRequest("GET", "/register/"+secretT, nil)); rec.Code != 404 {
			t.Fatalf("a GET must not be throttled: %d", rec.Code)
		}
	}
	// failed attempts to use a link do
	f.inv.EXPECT().Register(gomock.Any(), secretT, gomock.Any()).Return(core.RegisteredSession{}, core.ErrLinkUnusable).Times(maxFailsPerAccount)
	reg := func() *httptest.ResponseRecorder {
		return f.do(form("/register/"+secretT, url.Values{"email": {"a@example.com"}, "name": {"A"}, "password": {"longenough1"}, "confirm": {"longenough1"}}))
	}
	for range maxFailsPerAccount {
		if rec := reg(); rec.Code != 422 {
			t.Fatalf("%d", rec.Code)
		}
	}
	rec := reg()
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "Too many attempts") {
		t.Errorf("register throttle: %d", rec.Code)
	}
	assertSecretHeaders(t, rec)
	f.rec.EXPECT().SetPassword(gomock.Any(), secretT, gomock.Any()).Return(core.ErrLinkUnusable).Times(maxFailsPerAccount)
	for range maxFailsPerAccount {
		f.do(form("/recover/"+secretT, url.Values{"password": {"longenough1"}, "confirm": {"longenough1"}}))
	}
	if rec = f.do(form("/recover/"+secretT, url.Values{"password": {"a"}, "confirm": {"a"}})); rec.Code != http.StatusTooManyRequests {
		t.Errorf("recover throttle: %d", rec.Code)
	}
}

func TestRecoverPageNeverReadsTheSession(t *testing.T) {
	f := newFixture(t, Config{})
	// the mock has no Authenticate expectation: reading the session would fail the test
	f.rec.EXPECT().Preview(gomock.Any(), secretT).Return(nil)
	req := httptest.NewRequest("GET", "/recover/"+secretT, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec := f.do(req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Set a new password") {
		t.Fatalf("%d", rec.Code)
	}
	assertSecretHeaders(t, rec)
	f.rec.EXPECT().SetPassword(gomock.Any(), secretT, "longenough1").Return(nil)
	req = form("/recover/"+secretT, url.Values{"password": {"longenough1"}, "confirm": {"longenough1"}})
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	rec = f.do(req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?notice=password-set" || len(rec.Result().Cookies()) != 0 {
		t.Errorf("success: %d %v", rec.Code, rec.Header())
	}
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(core.User{}, time.Time{}, core.ErrNoSession).AnyTimes()
	if body := f.do(httptest.NewRequest("GET", "/login?notice=password-set", nil)).Body.String(); !strings.Contains(body, "Your password was changed") {
		t.Error("login notice")
	}
}

func TestRecoverFailures(t *testing.T) {
	f := newFixture(t, Config{})
	f.rec.EXPECT().Preview(gomock.Any(), secretT).Return(core.ErrLinkUnusable)
	get := f.do(httptest.NewRequest("GET", "/recover/"+secretT, nil))
	f.rec.EXPECT().SetPassword(gomock.Any(), secretT, "longenough1").Return(core.ErrLinkUnusable)
	post := f.do(form("/recover/"+secretT, url.Values{"password": {"longenough1"}, "confirm": {"longenough1"}}))
	if get.Code != 404 || post.Code != 422 || !strings.Contains(get.Body.String(), core.TalkToAdministrator) || !strings.Contains(post.Body.String(), core.TalkToAdministrator) {
		t.Errorf("%d %d", get.Code, post.Code)
	}
	f.rec.EXPECT().SetPassword(gomock.Any(), secretT, "short").Return(errors.Join(core.ErrInvalidUser, errors.New("the password must be between 10 and 128 characters")))
	f.rec.EXPECT().Preview(gomock.Any(), secretT).Return(nil)
	rec := f.do(form("/recover/"+secretT, url.Values{"password": {"short"}, "confirm": {"short"}}))
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "between 10 and 128") {
		t.Errorf("weak: %d", rec.Code)
	}
	f.rec.EXPECT().Preview(gomock.Any(), secretT).Return(nil)
	if rec = f.do(form("/recover/"+secretT, url.Values{"password": {"longenough1"}, "confirm": {"x"}})); rec.Code != 422 || !strings.Contains(rec.Body.String(), "do not match") {
		t.Errorf("mismatch: %d", rec.Code)
	}
}

// TestTokenRoutesAreRedactedInLogs: neither a request nor a panic on a token route writes the secret to the log.
func TestTokenRoutesAreRedactedInLogs(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	f := newFixture(t, Config{})
	f.inv.EXPECT().Preview(gomock.Any(), secretT).Return(core.InvitationPreview{}, core.ErrLinkUnusable)
	f.rec.EXPECT().Preview(gomock.Any(), secretT).Return(errors.New("boom " + "db"))
	f.do(httptest.NewRequest("GET", "/register/"+secretT, nil))
	f.do(httptest.NewRequest("GET", "/recover/"+secretT, nil)) // a 500: "request failed" is logged too
	f.camp.EXPECT().Join(gomock.Any(), gomock.Any(), secretT).DoAndReturn(func(context.Context, core.User, string) (int64, error) { panic("boom") })
	func() {
		defer func() { _ = recover() }()
		f.do(f.signedIn(form("/join/"+secretT, nil)))
	}()
	out := buf.String()
	if strings.Contains(out, secretT) {
		t.Errorf("the log holds the secret:\n%s", out)
	}
	for _, want := range []string{"path=/register/{token}", "path=/recover/{token}", "path=/join/{token}"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
}

// TestLogPathRedactsUncleanPaths: a token behind a path the mux would redirect ("//register/x", "/a/../recover/x") is not logged either.
func TestLogPathRedactsUncleanPaths(t *testing.T) {
	for _, p := range []string{"//register/" + secretT, "/x/../recover/" + secretT, "/./join/" + secretT, "/a//join//" + secretT, "/REGISTER/" + secretT} {
		if got := logPath(httptest.NewRequest("GET", p, nil)); strings.Contains(got, secretT) {
			t.Errorf("logPath(%q) = %q holds the secret", p, got)
		}
	}
	if got := logPath(httptest.NewRequest("GET", "/campaigns/3", nil)); got != "/campaigns/3" {
		t.Errorf("an ordinary path changed: %q", got)
	}
}

// TestInvitationMessageSingular: "up to 1 person", not "1 people".
func TestInvitationMessageSingular(t *testing.T) {
	c := core.CreatedAccountInvitation{AccountInvitation: core.AccountInvitation{Kind: core.InvitationAnonymous, MaxUses: 1, ExpiresAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)}, Token: secretT}
	got := invitationMessage(httptest.NewRequest("GET", "/", nil), c)
	if !strings.Contains(got, "up to 1 person and") {
		t.Errorf("message: %q", got)
	}
}
