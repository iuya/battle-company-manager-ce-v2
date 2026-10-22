package web

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var superG = core.User{ID: 9, Email: "gandalf@istari.me", DisplayName: "Gandalf", Role: core.RoleSuperuser}

func (f *fixture) asSuper(r *http.Request) *http.Request {
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(superG, time.Time{}, nil).AnyTimes()
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	return r
}

func htmxReq(r *http.Request, target string) *http.Request {
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Target", target)
	return r
}

func TestAdminIsSuperuserOnly(t *testing.T) {
	routes := [][2]string{{"GET", "/admin"}, {"GET", "/admin/users"}, {"POST", "/admin/users/1/permissions"}, {"POST", "/admin/users/1/handover"}, {"GET", "/admin/rulesets"},
		{"POST", "/admin/rulesets"}, {"POST", "/admin/rulesets/prune"}, {"GET", "/admin/rulesets/h/download"}, {"POST", "/admin/rulesets/h/default"}, {"POST", "/admin/rulesets/h/delete"}}
	for _, role := range []core.Role{core.RoleUser, core.RoleAdmin} {
		f := newFixture(t, Config{})
		u := frodo
		u.Role = role
		f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(u, time.Time{}, nil).AnyTimes()
		for _, rt := range routes {
			r := httptest.NewRequest(rt[0], rt[1], nil)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
			if rec := f.do(r); rec.Code != http.StatusNotFound {
				t.Errorf("%s %s as %s: %d, want 404", rt[0], rt[1], role, rec.Code)
			}
		}
	}
}

func TestAdminRedirectsToUsers(t *testing.T) {
	f := newFixture(t, Config{})
	rec := f.do(f.asSuper(httptest.NewRequest("GET", "/admin", nil)))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/users" {
		t.Errorf("%d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

func adminUsers() []core.AdminUser {
	return []core.AdminUser{
		{User: core.User{ID: 9, Email: superG.Email, DisplayName: "Gandalf", Role: core.RoleSuperuser}, Campaigns: 1, CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		{User: core.User{ID: 1, Email: frodo.Email, DisplayName: "Frodo", Role: core.RoleUser, CanCreateCampaigns: true}, Campaigns: 2, CreatedAt: time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)},
		{User: core.User{ID: 5, DisplayName: "Former player", Role: core.RoleUser}, Closed: true},
	}
}

func TestAdminUsersPage(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "", userPageSize, 0).Return(adminUsers(), 3, nil)
	body := f.do(f.asSuper(httptest.NewRequest("GET", "/admin/users", nil))).Body.String()
	for _, want := range []string{"Users", "Rules files", `aria-current="page"`, "frodo@shire.me", "Can create campaigns", "2 campaigns", "2 Jan 2006"[:0] + "3 Feb 2026", "Closed", "Edit permissions", `hx-post="/admin/users/1/permissions"`, "3 accounts", `id="user-q"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(body, `hx-post="/admin/users/5/permissions"`) {
		t.Error("a closed account has no controls")
	}
	if strings.Contains(body, "Show more") {
		t.Error("everything fits on one page")
	}
}

func TestAdminUsersSearchAndPaging(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "fro", userPageSize, 0).Return(adminUsers()[1:2], 30, nil)
	body := f.do(htmxReq(f.asSuper(httptest.NewRequest("GET", "/admin/users?q=fro", nil)), "user-rows")).Body.String()
	if strings.Contains(body, "<html") || !strings.Contains(body, "Frodo") || !strings.Contains(body, "30 accounts match") || !strings.Contains(body, "offset=1") {
		t.Errorf("search partial:\n%s", body)
	}
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "", userPageSize, 25).Return(adminUsers()[1:2], 26, nil)
	body = f.do(htmxReq(f.asSuper(httptest.NewRequest("GET", "/admin/users?offset=25", nil)), "user-more")).Body.String()
	if strings.Contains(body, "26 accounts") || !strings.Contains(body, "Frodo") || strings.Contains(body, "Show more") {
		t.Errorf("next page is rows only:\n%s", body)
	}
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "zzz", userPageSize, 0).Return(nil, 0, nil)
	body = f.do(htmxReq(f.asSuper(httptest.NewRequest("GET", "/admin/users?q=zzz", nil)), "user-rows")).Body.String()
	if !strings.Contains(body, "No accounts match") {
		t.Errorf("empty: %s", body)
	}
}

func TestAdminPermissions(t *testing.T) {
	f := newFixture(t, Config{})
	post := func(v url.Values) *httptest.ResponseRecorder {
		return f.do(htmxReq(f.asSuper(form("/admin/users/1/permissions", v)), "user-1"))
	}
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, frodo.Email, 200, 0).Return(adminUsers()[1:2], 1, nil).Times(3)

	f.auth.EXPECT().SetUserPermissions(gomock.Any(), superG, int64(1), core.RoleUser, true).Return(nil)
	rec := post(url.Values{"role": {"user"}, "can_create": {"on"}, "email": {frodo.Email}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Saved") {
		t.Errorf("save: %d %s", rec.Code, rec.Body)
	}
	// the creation flag means nothing for an admin: it is not sent on
	f.auth.EXPECT().SetUserPermissions(gomock.Any(), superG, int64(1), core.RoleAdmin, false).Return(nil)
	post(url.Values{"role": {"admin"}, "can_create": {"on"}, "email": {frodo.Email}})

	f.auth.EXPECT().SetUserPermissions(gomock.Any(), superG, int64(1), core.RoleUser, false).Return(core.Refuse("This is the last superuser; make someone else a superuser first"))
	rec = post(url.Values{"role": {"user"}, "email": {frodo.Email}})
	body := rec.Body.String()
	if rec.Code != 422 || !strings.Contains(body, "last superuser") || !strings.Contains(body, `<details class="mt-3" open>`) {
		t.Errorf("refused: %d %s", rec.Code, body)
	}
}

func TestAdminPermissionsUnknownUser(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().SetUserPermissions(gomock.Any(), superG, int64(77), core.RoleUser, false).Return(core.ErrNotFound)
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "x", 200, 0).Return(nil, 0, nil)
	if rec := f.do(f.asSuper(form("/admin/users/77/permissions", url.Values{"role": {"user"}, "email": {"x"}}))); rec.Code != 404 {
		t.Errorf("%d", rec.Code)
	}
}

func usage() []core.RulesetUsage {
	return []core.RulesetUsage{
		{RulesetInfo: core.RulesetInfo{Kind: core.KindVanilla, Hash: "aaaaaaaa11111111", Name: "MESBG 2024", Version: "2", Default: true}, Size: 2048, CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			Campaigns: []core.CampaignRef{{ID: 4, Name: "Minas Tirith"}}},
		{RulesetInfo: core.RulesetInfo{Kind: core.KindBattleCompanies, Hash: "bbbbbbbb22222222", Name: "BC", Version: "1"}, Size: 100, UploadedBy: &core.User{DisplayName: "Frodo"}, CreatedAt: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)},
	}
}

func TestAdminRulesetsPage(t *testing.T) {
	f := newFixture(t, Config{})
	f.rs.EXPECT().ListWithUsage(gomock.Any(), superG).Return(usage(), nil)
	body := f.do(f.asSuper(httptest.NewRequest("GET", "/admin/rulesets", nil))).Body.String()
	for _, want := range []string{"Vanilla rules data", "Battle Companies rules data", "Glossary", "None stored", "Shipped with the app", "Frodo", "Default", "aaaaaaaa", `title="aaaaaaaa11111111"`, "2 KB", "Used by 1 campaign", `href="/campaigns/4"`,
		"Used by no campaign", `hx-post="/admin/rulesets/bbbbbbbb22222222/delete"`, `hx-post="/admin/rulesets/bbbbbbbb22222222/default"`, "Delete 1 unused file", `download="BC-1.json"`, `hx-post="/admin/rulesets"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, not := range []string{`/admin/rulesets/aaaaaaaa11111111/delete`, `/admin/rulesets/aaaaaaaa11111111/default`} {
		if strings.Contains(body, not) {
			t.Errorf("the used default file offers %q", not)
		}
	}
}

func TestAdminRulesetActions(t *testing.T) {
	f := newFixture(t, Config{})
	f.rs.EXPECT().ListWithUsage(gomock.Any(), superG).Return(usage(), nil).AnyTimes()
	do := func(path string) *httptest.ResponseRecorder {
		return f.do(htmxReq(f.asSuper(form(path, nil)), "ruleset-panel"))
	}
	f.rs.EXPECT().SetDefault(gomock.Any(), superG, "bbbbbbbb22222222").Return(nil)
	if rec := do("/admin/rulesets/bbbbbbbb22222222/default"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "New campaigns will use this file") || !strings.Contains(rec.Body.String(), "ring-2") {
		t.Errorf("default: %d %s", rec.Code, rec.Body)
	}
	f.rs.EXPECT().Delete(gomock.Any(), superG, "bbbbbbbb22222222").Return(core.Refuse("A campaign uses this file"))
	if rec := do("/admin/rulesets/bbbbbbbb22222222/delete"); rec.Code != 422 || !strings.Contains(rec.Body.String(), "A campaign uses this file") {
		t.Errorf("refused delete: %d %s", rec.Code, rec.Body)
	}
	f.rs.EXPECT().Delete(gomock.Any(), superG, "bbbbbbbb22222222").Return(nil)
	if rec := do("/admin/rulesets/bbbbbbbb22222222/delete"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "The file is deleted.") {
		t.Errorf("delete: %d %s", rec.Code, rec.Body)
	}
	f.rs.EXPECT().Delete(gomock.Any(), superG, "nope").Return(core.ErrNotFound)
	if rec := do("/admin/rulesets/nope/delete"); rec.Code != 404 {
		t.Errorf("unknown: %d", rec.Code)
	}
	f.rs.EXPECT().PruneUnused(gomock.Any(), superG).Return(3, nil)
	if rec := do("/admin/rulesets/prune"); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Deleted 3 unused files.") {
		t.Errorf("prune: %d %s", rec.Code, rec.Body)
	}
}

func TestAdminRulesetDownload(t *testing.T) {
	f := newFixture(t, Config{})
	f.rs.EXPECT().Download(gomock.Any(), superG, "h").Return(core.RulesetInfo{Name: "BC", Version: "1"}, []byte(`{"a":1}`), nil)
	rec := f.do(f.asSuper(httptest.NewRequest("GET", "/admin/rulesets/h/download", nil)))
	if rec.Code != 200 || rec.Body.String() != `{"a":1}` || !strings.Contains(rec.Header().Get("Content-Disposition"), `filename=BC-1.json`) {
		t.Errorf("%d %q %q", rec.Code, rec.Body, rec.Header().Get("Content-Disposition"))
	}
}

func upload(f *fixture, kind, content string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("kind", kind)
	if content != "" {
		fw, _ := mw.CreateFormFile("file", "x.json")
		_, _ = fw.Write([]byte(content))
	}
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/admin/rulesets", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return f.do(htmxReq(f.asSuper(r), "ruleset-panel"))
}

func TestAdminRulesetUpload(t *testing.T) {
	f := newFixture(t, Config{})
	f.rs.EXPECT().ListWithUsage(gomock.Any(), superG).Return(usage(), nil).AnyTimes()
	const bc = `{"name":"BC2","version":"2","extends":"V"}`
	hash, _ := core.RulesetHash([]byte(bc))

	// new: stored with the uploader recorded
	f.rs.EXPECT().Find(gomock.Any(), core.KindBattleCompanies, hash, "BC2", "2").Return(core.RulesetInfo{}, core.ErrNotFound)
	f.rs.EXPECT().StoreAs(gomock.Any(), superG, core.KindBattleCompanies, hash, []byte(bc)).Return(core.RulesetInfo{Kind: core.KindBattleCompanies, Hash: hash, Name: "BC2", Version: "2"}, nil)
	if rec := upload(f, "battle_companies", bc); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Stored BC2 2.") {
		t.Errorf("new: %d %s", rec.Code, rec.Body)
	}
	// known: nothing stored, the existing card is highlighted
	f.rs.EXPECT().Find(gomock.Any(), core.KindBattleCompanies, hash, "BC2", "2").Return(core.RulesetInfo{Hash: "bbbbbbbb22222222"}, nil)
	if rec := upload(f, "battle_companies", bc); rec.Code != 200 || !strings.Contains(rec.Body.String(), "already stored") || !strings.Contains(rec.Body.String(), "ring-2") {
		t.Errorf("known: %d %s", rec.Code, rec.Body)
	}
	// rejected by the schema
	f.rs.EXPECT().Find(gomock.Any(), core.KindBattleCompanies, hash, "BC2", "2").Return(core.RulesetInfo{}, core.ErrNotFound)
	f.rs.EXPECT().StoreAs(gomock.Any(), superG, core.KindBattleCompanies, hash, gomock.Any()).Return(core.RulesetInfo{}, errors.Join(core.ErrInvalidRuleset, errors.New("limits: wrong type")))
	if rec := upload(f, "battle_companies", bc); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Limits: wrong type") {
		t.Errorf("invalid: %d %s", rec.Code, rec.Body)
	}
	// not JSON, no file, no kind
	if rec := upload(f, "glossary", "{"); rec.Code != 422 || !strings.Contains(rec.Body.String(), "not valid JSON") {
		t.Errorf("not json: %d %s", rec.Code, rec.Body)
	}
	if rec := upload(f, "glossary", ""); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose a .json file") {
		t.Errorf("no file: %d %s", rec.Code, rec.Body)
	}
	if rec := upload(f, "", bc); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose the kind") {
		t.Errorf("no kind: %d %s", rec.Code, rec.Body)
	}
}

func TestAdminUsersListsOrganisedCampaignsAndHandsThemOver(t *testing.T) {
	f := newFixture(t, Config{})
	u := core.AdminUser{User: core.User{ID: 50, Email: "faramir@x.me", DisplayName: "Faramir", Role: core.RoleUser}, Campaigns: 1,
		Organises: []core.OrganisedCampaign{{ID: 7, Name: "Siege", Status: core.CampaignActive, Players: 3}}}
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "", userPageSize, 0).Return([]core.AdminUser{u}, 1, nil)
	f.camp.EXPECT().Members(gomock.Any(), int64(7)).Return([]core.CampaignMember{{UserID: 50, Name: "Faramir", Organiser: true, CompanyID: 1, CompanyName: "Ithilien"}, {UserID: 51, Name: "Eowyn", CompanyID: 2, CompanyName: "Rohan"}}, nil).AnyTimes()
	body := f.do(f.asSuper(httptest.NewRequest("GET", "/admin/users", nil))).Body.String()
	for _, want := range []string{"Organises 1 campaign", "Siege", "3 players", `value="51">Eowyn (Rohan)`, "/admin/users/50/handover"} {
		if !strings.Contains(body, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if strings.Contains(body, `value="50">Faramir`) {
		t.Error("the organiser is not offered")
	}
	f.camp.EXPECT().HandOver(gomock.Any(), superG, int64(7), int64(51)).Return(nil)
	f.auth.EXPECT().ListUsers(gomock.Any(), superG, "faramir@x.me", 200, 0).Return([]core.AdminUser{u}, 1, nil).AnyTimes()
	rec := f.do(f.asSuper(form("/admin/users/50/handover", url.Values{"campaign": {"7"}, "organiser": {"51"}, "email": {"faramir@x.me"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Handed over.") {
		t.Errorf("handover: %d", rec.Code)
	}
	f.camp.EXPECT().HandOver(gomock.Any(), superG, int64(7), int64(52)).Return(core.Refuse("That player is not in the campaign"))
	rec = f.do(f.asSuper(form("/admin/users/50/handover", url.Values{"campaign": {"7"}, "organiser": {"52"}, "email": {"faramir@x.me"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "not in the campaign") {
		t.Errorf("refused: %d", rec.Code)
	}
}
