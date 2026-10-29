package web

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var frodo = core.User{ID: 1, Email: "frodo@shire.me", DisplayName: "Frodo"}

type fixture struct {
	srv  *Server
	camp *Mockcampaigns
	rs   *Mockrulesets
	gl   *Mockglossaries
	auth *Mockauth
	// selection is how the rules pick a scenario by dice (the default: the shipped rules, testSelection).
	selection core.ScenarioSelection
	// grants is what the company pages read as the company's history of awards (the default: none).
	grants []core.Grant
	// aftermaths is what the match page's Aftermath card reads (the default: none).
	aftermaths   []core.AftermathRef
	aftermathErr error
	// liveDefaults is what the result form reads from the live match (the default: no live data).
	liveDefaults *core.LiveMatchDefaults
	// nav is what the sidebar's campaign list reads (the default: none); navErr makes it fail.
	nav          []core.CampaignNav
	navErr       error
	cardsStubbed bool // lazyCards ran
	navFinished  bool // the withFinished the last NavCampaigns call was given
}

func newFixture(t *testing.T, cfg Config) *fixture {
	ctrl := gomock.NewController(t)
	f := &fixture{selection: testSelection, camp: NewMockcampaigns(ctrl), rs: NewMockrulesets(ctrl), gl: NewMockglossaries(ctrl), auth: NewMockauth(ctrl)}
	f.rs.EXPECT().CampaignOptions(gomock.Any()).Return([]core.CampaignOption{
		{Key: "heroesDropProfileGear", Scope: "rules", Label: "Heroes may discard any wargear", Description: "Even what is built into their profile."},
		{Key: "shakeTheTable", Scope: "app", Label: "Shake the table", Description: "Throw the dice in the browser."},
		{Key: "manualRolls", Scope: "app", Label: "Players may enter their own rolls", Description: "Type the result of a roll."}}, nil).AnyTimes()
	f.camp.EXPECT().Grants(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, int64, int64) ([]core.Grant, error) { return f.grants, nil }).AnyTimes()
	f.camp.EXPECT().AftermathStatuses(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, int64, int64) ([]core.AftermathRef, error) { return f.aftermaths, f.aftermathErr }).AnyTimes()
	f.camp.EXPECT().MatchDefaults(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, int64, int64) (core.LiveMatchDefaults, bool, error) {
		if f.liveDefaults == nil {
			return core.LiveMatchDefaults{}, false, nil
		}
		return *f.liveDefaults, true, nil
	}).AnyTimes()
	f.camp.EXPECT().NavCampaigns(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, with bool) ([]core.CampaignNav, error) {
		f.navFinished = with
		return f.nav, f.navErr
	}).AnyTimes()
	f.camp.EXPECT().ScenarioSelection(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, int64) (core.ScenarioSelection, error) { return f.selection, nil }).AnyTimes()
	var err error
	if f.srv, err = NewServer(cfg, f.camp, f.rs, f.gl, f.auth); err != nil {
		t.Fatal(err)
	}
	return f
}

// signedIn makes the "tok" session cookie resolve to frodo.
func (f *fixture) signedIn(r *http.Request) *http.Request {
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(frodo, time.Time{}, nil).AnyTimes()
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	return r
}

var matchPageRe = regexp.MustCompile(`^/campaigns/\d+/matches/\d+$`)

// lazyCards makes the cards the full match page draws inline fail (registered after the test's own expectations, so those win), which
// leaves their slots lazy: the tests of other things need not stub them.
func (f *fixture) lazyCards() {
	if f.cardsStubbed {
		return
	}
	f.cardsStubbed = true
	errNo := errors.New("card not stubbed")
	f.camp.EXPECT().AgainstTheOdds(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(core.ATO{}, errNo).AnyTimes()
	f.camp.EXPECT().SecretObjectives(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(core.SecretView{}, errNo).AnyTimes()
	f.camp.EXPECT().EndChecks(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(core.EndChecks{}, errNo).AnyTimes()
	f.camp.EXPECT().Log(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errNo).AnyTimes()
}

func (f *fixture) do(r *http.Request) *httptest.ResponseRecorder {
	if r.Method == http.MethodGet && matchPageRe.MatchString(r.URL.Path) && r.Header.Get("HX-Request") == "" {
		f.lazyCards()
	}
	rec := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(rec, r)
	return rec
}

func form(path string, values url.Values) *http.Request {
	r := httptest.NewRequest("POST", path, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func TestProtectedPagesRedirectToLogin(t *testing.T) {
	f := newFixture(t, Config{})
	for path, want := range map[string]string{"/": "/login", "/campaigns/new": "/login?next=%2Fcampaigns%2Fnew", "/glossary": "/login?next=%2Fglossary",
		"/account": "/login?next=%2Faccount", "/admin": "/login?next=%2Fadmin", "/campaigns/7/matches?x=1": "/login?next=%2Fcampaigns%2F7%2Fmatches%3Fx%3D1"} {
		rec := f.do(httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
			t.Errorf("GET %s: got %d -> %q", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	// htmx navigations ask for a full page load, so the header is drawn for a signed-out visitor
	req := httptest.NewRequest("GET", "/glossary", nil)
	req.Header.Set("HX-Request", "true")
	rec := f.do(req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") != "/login" {
		t.Errorf("htmx: got %d, HX-Redirect=%q", rec.Code, rec.Header().Get("HX-Redirect"))
	}
	// an htmx request remembers the page the browser shows, only when it is on this site
	req = httptest.NewRequest("GET", "/nav/campaigns", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-URL", "http://"+req.Host+"/campaigns/7/matches/3")
	if got := f.do(req).Header().Get("HX-Redirect"); got != "/login?next=%2Fcampaigns%2F7%2Fmatches%2F3" {
		t.Errorf("htmx deep link: %q", got)
	}
	req.Header.Set("HX-Current-URL", "http://evil.example/campaigns/7")
	if got := f.do(req).Header().Get("HX-Redirect"); got != "/login" {
		t.Errorf("another site's address must not be remembered: %q", got)
	}
	// a POST cannot be replayed after signing in
	if rec := f.do(form("/campaigns/7/invite", url.Values{})); rec.Header().Get("Location") != "/login" {
		t.Errorf("POST: %q", rec.Header().Get("Location"))
	}
}

func TestStaleSessionCookieIsCleared(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().Authenticate(gomock.Any(), "old").Return(core.User{}, time.Time{}, core.ErrNoSession)
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "old"})
	rec := f.do(req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("got %d, cookie %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
}

func TestHome(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return(nil, nil)
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, true).Return(nil, nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/", nil)))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Welcome, Frodo") || !strings.Contains(body, "Sign out") {
		t.Fatalf("got %d\n%s", rec.Code, body)
	}
	if strings.Contains(body, "Show finished campaigns") || strings.Contains(body, "Welcome back") {
		t.Error("an account in no campaign is welcomed plainly and has nothing to filter")
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-cache" {
		t.Errorf("pages depend on the user: Cache-Control = %q", got)
	}
}

func TestHomeKeepsTheToggleWhenOnlyFinishedCampaignsExist(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return(nil, nil)
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, true).Return([]core.CampaignSummary{{ID: 1, Name: "Over", Status: core.CampaignFinished}}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/", nil))).Body.String()
	if !strings.Contains(body, "Welcome back, Frodo") || !strings.Contains(body, "Show finished campaigns") {
		t.Error("somebody whose campaigns are all finished can still show them")
	}
}

func TestLoginPage(t *testing.T) {
	f := newFixture(t, Config{})
	rec := f.do(httptest.NewRequest("GET", "/login", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `name="password"`) || strings.Contains(body, "Sign out") {
		t.Fatalf("got %d\n%s", rec.Code, body)
	}
	// signed-in users have nothing to do here
	rec = f.do(f.signedIn(httptest.NewRequest("GET", "/login", nil)))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLogin_Success(t *testing.T) {
	f := newFixture(t, Config{SecureCookies: true})
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	f.auth.EXPECT().Login(gomock.Any(), "frodo@shire.me", "a long enough password").Return("tok", expires, frodo, nil)

	rec := f.do(form("/login", url.Values{"email": {"frodo@shire.me"}, "password": {"a long enough password"}}))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value != "tok" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie not hardened: %+v", cookie)
	}
}

func TestLogin_HTMXSuccessRedirectsWithAFullPageLoad(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().Login(gomock.Any(), gomock.Any(), gomock.Any()).Return("tok", time.Now().Add(time.Hour), frodo, nil)
	req := form("/login", url.Values{"email": {"f@shire.me"}, "password": {"x"}})
	req.Header.Set("HX-Request", "true")
	rec := f.do(req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") != "/" {
		t.Fatalf("got %d, HX-Redirect=%q", rec.Code, rec.Header().Get("HX-Redirect"))
	}
}

func TestLogin_WrongCredentials(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().Login(gomock.Any(), "frodo@shire.me", "nope").Return("", time.Time{}, core.User{}, core.ErrInvalidCredentials)
	rec := f.do(form("/login", url.Values{"email": {"frodo@shire.me"}, "password": {"nope"}}))
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "Invalid email or password.") {
		t.Fatalf("got %d\n%s", rec.Code, body)
	}
	if !strings.Contains(body, `value="frodo@shire.me"`) || strings.Contains(body, "nope") {
		t.Fatal("the email is kept, the password never comes back")
	}
	if rec.Header().Get("Set-Cookie") != "" {
		t.Fatal("no cookie on a failed login")
	}
}

func TestLogin_ServerError(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().Login(gomock.Any(), gomock.Any(), gomock.Any()).Return("", time.Time{}, core.User{}, errors.New("db down"))
	if rec := f.do(form("/login", url.Values{"email": {"a@b.co"}, "password": {"x"}})); rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestLogin_IsThrottledAfterRepeatedFailures(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().Login(gomock.Any(), "frodo@shire.me", gomock.Any()).
		Return("", time.Time{}, core.User{}, core.ErrInvalidCredentials).Times(maxFailsPerAccount)
	attempt := func() int {
		return f.do(form("/login", url.Values{"email": {"frodo@shire.me"}, "password": {"guess"}})).Code
	}
	for i := 0; i < maxFailsPerAccount; i++ {
		if code := attempt(); code != http.StatusUnprocessableEntity {
			t.Fatalf("attempt %d: got %d", i+1, code)
		}
	}
	// the next one is refused without even checking the password (no further Login call is expected)
	if code := attempt(); code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", code)
	}
}

func TestLogout(t *testing.T) {
	f := newFixture(t, Config{})
	f.auth.EXPECT().Logout(gomock.Any(), "tok").Return(nil)
	req := f.signedIn(form("/logout", nil))
	rec := f.do(req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("got %d -> %q, cookie %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
	}
}

func TestCrossSitePostsAreRejected(t *testing.T) {
	f := newFixture(t, Config{})
	req := form("/login", url.Values{"email": {"frodo@shire.me"}, "password": {"x"}})
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if rec := f.do(req); rec.Code != http.StatusForbidden { // no Login call expected
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestHomePage_FullVsHTMX(t *testing.T) {
	f := newFixture(t, Config{})
	list := []core.CampaignSummary{{ID: 1, Name: "Rohan Rising", Status: core.CampaignActive}}
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return(list, nil).Times(2)

	full := f.do(f.signedIn(httptest.NewRequest("GET", "/", nil)))
	if !strings.Contains(full.Body.String(), "<html") || !strings.Contains(full.Body.String(), "Rohan Rising") {
		t.Fatalf("full page should include layout and data:\n%s", full.Body)
	}

	req := f.signedIn(httptest.NewRequest("GET", "/", nil))
	req.Header.Set("HX-Request", "true")
	part := f.do(req)
	if !strings.HasPrefix(part.Body.String(), "<title>Campaigns") { // htmx lifts it into document.title
		t.Fatalf("htmx response should start with the <title> element:\n%s", part.Body)
	}
	if strings.Contains(part.Body.String(), "<html") || !strings.Contains(part.Body.String(), "Rohan Rising") {
		t.Fatalf("htmx response should be the content only:\n%s", part.Body)
	}
}

func TestFooterDisclaimer(t *testing.T) {
	f := newFixture(t, Config{})
	if body := f.do(httptest.NewRequest("GET", "/login", nil)).Body.String(); !strings.Contains(body, "Not affiliated with Games Workshop") {
		t.Error("login page lacks the disclaimer")
	}
}

func TestStyleGuide(t *testing.T) {
	on := newFixture(t, Config{StyleGuide: true})
	rec := on.do(httptest.NewRequest("GET", "/styleguide", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Forest &amp; Gold") {
		t.Fatalf("style guide should list the palettes: %d\n%s", rec.Code, rec.Body)
	}

	off := newFixture(t, Config{})
	if rec := off.do(httptest.NewRequest("GET", "/styleguide", nil)); rec.Code != http.StatusNotFound {
		t.Fatalf("style guide should be disabled by default config, got %d", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	f := newFixture(t, Config{})
	h := f.do(httptest.NewRequest("GET", "/login", nil)).Header()
	if h.Get("X-Frame-Options") != "DENY" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security headers: %v", h)
	}
}

func TestHomeListsCampaigns(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return([]core.CampaignSummary{
		{ID: 1, Name: "Siege of Gondor", Status: core.CampaignActive, Round: 2, Rounds: 8, Matches: 3,
			Companies: []core.CampaignCompany{{Name: "Rangers", Faction: "Gondor"}}},
	}, nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/", nil)))
	body := rec.Body.String()
	for _, want := range []string{"Siege of Gondor", "Round 2 of 8", "3 matches", "Rangers"} {
		if rec.Code != 200 || !strings.Contains(body, want) {
			t.Errorf("GET /: want %q in a 200 page, got %d", want, rec.Code)
		}
	}
	if strings.Contains(body, "New campaign") || strings.Contains(body, "Administration") {
		t.Error("a plain user must not see the create button or the administration link")
	}
}

func TestHomeShowsTheFactionUnderItsLabel(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return([]core.CampaignSummary{
		{ID: 1, Name: "Golden Wood", Status: core.CampaignActive, Rounds: 8,
			Companies: []core.CampaignCompany{{Name: "Galadhrim", Faction: "Lothlorien", FactionLabel: "Lothlórien"}}},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/", nil))).Body.String()
	if !strings.Contains(body, "Lothlórien") || strings.Contains(body, "(Lothlorien)") {
		t.Errorf("the faction is shown as its label: %s", body)
	}
}

func TestHomeShowsAdminThings(t *testing.T) {
	f := newFixture(t, Config{})
	admin := core.User{ID: 2, DisplayName: "Gandalf", Role: core.RoleSuperuser}
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(admin, time.Time{}, nil)
	f.camp.EXPECT().List(gomock.Any(), admin.ID, false).Return(nil, nil)
	f.camp.EXPECT().List(gomock.Any(), admin.ID, true).Return(nil, nil)
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
	body := f.do(r).Body.String()
	for _, want := range []string{"New campaign", "Administration", "No active campaigns"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestAdminIsHiddenFromOthers(t *testing.T) {
	f := newFixture(t, Config{})
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/admin", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

// maker is allowed to create campaigns; do sends requests as them.
func campaignMaker(f *fixture) (core.User, func(*http.Request) *httptest.ResponseRecorder) {
	maker := core.User{ID: 3, DisplayName: "Elrond", CanCreateCampaigns: true}
	f.auth.EXPECT().Authenticate(gomock.Any(), "tok").Return(maker, time.Time{}, nil).AnyTimes()
	return maker, func(r *http.Request) *httptest.ResponseRecorder {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "tok"})
		return f.do(r)
	}
}

// multipartForm builds the new-campaign form as the browser sends it: fields plus optional files.
func multipartForm(t *testing.T, fields map[string]string, files map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	for k, v := range files {
		fw, _ := mw.CreateFormFile(k, "rules.json")
		_, _ = fw.Write([]byte(v))
	}
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/campaigns", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func (f *fixture) defaultsShown() {
	f.rs.EXPECT().Default(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, k core.RulesetKind) (core.RulesetInfo, error) {
		return core.RulesetInfo{Kind: k, Name: "Default " + string(k), Version: "1", Default: true}, nil
	}).AnyTimes()
}

func TestCreateCampaign(t *testing.T) {
	f := newFixture(t, Config{})
	maker, do := campaignMaker(f)
	f.defaultsShown()

	in := core.CampaignInput{Name: "Rivendell Cup", Description: "Friendly"}
	f.camp.EXPECT().Create(gomock.Any(), maker, in).Return(core.CampaignSummary{ID: 9}, nil)
	rec := do(form("/campaigns", url.Values{"name": {"Rivendell Cup"}, "description": {"Friendly"}}))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns" {
		t.Errorf("success: got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = do(form("/campaigns", url.Values{"name": {" "}})) // rejected before anything is stored or created
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "The name must be between") {
		t.Errorf("invalid: got %d", rec.Code)
	}
}

func TestCreateCampaignWithRulesFiles(t *testing.T) {
	f := newFixture(t, Config{})
	maker, do := campaignMaker(f)
	f.defaultsShown()
	const vanilla = `{"game":"Mine","appVersion":3}`

	// A new vanilla file is uploaded and stored; a Battle Companies file the server already knows is not.
	f.rs.EXPECT().StoreAs(gomock.Any(), gomock.Any(), core.KindVanilla, "h-van", []byte(vanilla)).Return(core.RulesetInfo{Kind: core.KindVanilla, Hash: "h-van"}, nil)
	f.rs.EXPECT().Lookup(gomock.Any(), "h-bc").Return(core.RulesetInfo{Kind: core.KindBattleCompanies, Hash: "h-bc"}, nil)
	f.camp.EXPECT().Create(gomock.Any(), maker, core.CampaignInput{Name: "Own rules", Vanilla: "h-van", BattleCompanies: "h-bc"}).
		Return(core.CampaignSummary{ID: 1}, nil)
	rec := do(multipartForm(t, map[string]string{
		"name": "Own rules", "vanilla-mode": "file", "vanilla-hash": "h-van",
		"battle_companies-mode": "file", "battle_companies-hash": "h-bc",
	}, map[string]string{"vanilla-file": vanilla}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
}

func TestCreateCampaignRejectsUnknownOrWrongRulesFiles(t *testing.T) {
	f := newFixture(t, Config{})
	_, do := campaignMaker(f)
	f.defaultsShown()
	f.rs.EXPECT().Lookup(gomock.Any(), "nope").Return(core.RulesetInfo{}, core.ErrNotFound)
	rec := do(multipartForm(t, map[string]string{"name": "X", "vanilla-mode": "file", "vanilla-hash": "nope"}, nil))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Choose a file") {
		t.Errorf("unknown hash without a file: got %d", rec.Code)
	}

	f.rs.EXPECT().Lookup(gomock.Any(), "bc").Return(core.RulesetInfo{Kind: core.KindBattleCompanies, Hash: "bc"}, nil)
	rec = do(multipartForm(t, map[string]string{"name": "X", "vanilla-mode": "file", "vanilla-hash": "bc"}, nil))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "is not a Vanilla rules data file") {
		t.Errorf("wrong kind: got %d", rec.Code)
	}

	f.rs.EXPECT().StoreAs(gomock.Any(), gomock.Any(), core.KindVanilla, "h", gomock.Any()).
		Return(core.RulesetInfo{}, errors.Join(core.ErrInvalidRuleset, errors.New("the file does not match the hash it was announced with")))
	rec = do(multipartForm(t, map[string]string{"name": "X", "vanilla-mode": "file", "vanilla-hash": "h"}, map[string]string{"vanilla-file": "{}"}))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "does not match the hash") {
		t.Errorf("tampered upload: got %d", rec.Code)
	}
}

func TestRulesetLookup(t *testing.T) {
	f := newFixture(t, Config{})
	f.rs.EXPECT().Find(gomock.Any(), core.KindVanilla, "abc", "", "").Return(core.RulesetInfo{Kind: core.KindVanilla, Hash: "stored", Name: "N", Version: "2"}, nil)
	f.rs.EXPECT().Find(gomock.Any(), core.KindBattleCompanies, "zzz", "", "").Return(core.RulesetInfo{}, core.ErrNotFound)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/rulesets/abc?kind=vanilla&name=MESBG+2024&version=2", nil)))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":"N"`) || !strings.Contains(rec.Body.String(), `"hash":"stored"`) {
		t.Errorf("known (the answer carries the stored file's hash): %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/rulesets/zzz?kind=battle_companies", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("unknown: %d", rec.Code)
	}
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/rulesets/abc?kind=bogus", nil))); rec.Code != http.StatusBadRequest {
		t.Errorf("bad kind: %d", rec.Code)
	}
	if rec := f.do(httptest.NewRequest("GET", "/rulesets/abc?kind=vanilla", nil)); rec.Code != http.StatusSeeOther {
		t.Errorf("signed out: %d", rec.Code)
	}
}

func TestCampaignOverviewShowsRulesets(t *testing.T) {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.Rulesets = []core.RulesetInfo{
		{Kind: core.KindVanilla, Hash: "0123456789abcdef", Name: "MESBG 2024", Version: "2", Default: true},
		{Kind: core.KindBattleCompanies, Hash: "fedcba9876543210", Name: "BC playtest", Version: "1.0"},
		{Kind: core.KindGlossary, Hash: "aaaaaaaabbbbbbbb", Name: "Glossary 1", Version: "1.0", Default: true},
	}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil))).Body.String()
	for _, want := range []string{"MESBG 2024", "BC playtest", "v1.0", "01234567", "Default", "Glossary 1"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// Every campaign setting is listed on the overview with its state: with all on, there is one "On" badge per field of core.CampaignSettings.
func TestCampaignOverviewListsEverySettingWithItsState(t *testing.T) {
	all, none := core.CampaignSettings{}, core.CampaignSettings{}
	v := reflect.ValueOf(&all).Elem()
	toggles := 0 // CompaniesPerPlayer is a number, shown by its own control, not an On/Off badge
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.Bool {
			v.Field(i).SetBool(true)
			toggles++
		}
	}
	for settings, badge := range map[*core.CampaignSettings]string{&all: ">On</span>", &none: ">Off</span>"} {
		f := newFixture(t, Config{})
		d := campaignDetail()
		d.Settings = *settings
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
		f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
		body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil))).Body.String()
		section := body[strings.Index(body, `id="settings-title"`):]
		section = section[:strings.Index(section, "</section>")]
		if got := strings.Count(section, badge); got != toggles {
			t.Errorf("%d settings shown with %s, want one per setting (%d)", got, badge, toggles)
		}
	}
}

func TestHomeHasClientSideSearch(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return([]core.CampaignSummary{{ID: 1, Name: "Siege of Gondor", Status: core.CampaignActive}}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/", nil))).Body.String()
	if !strings.Contains(body, `x-data="listSearch"`) || !strings.Contains(body, `data-name="Siege of Gondor"`) {
		t.Error("the main page should filter its campaigns in the browser")
	}
}

func TestCreateCampaignNeedsPermission(t *testing.T) {
	f := newFixture(t, Config{}) // frodo is a plain user
	for _, r := range []*http.Request{httptest.NewRequest("GET", "/campaigns/new", nil), form("/campaigns", url.Values{"name": {"x"}})} {
		if rec := f.do(f.signedIn(r)); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: got %d, want 403", r.Method, r.URL.Path, rec.Code)
		}
	}
}

func campaignDetail() core.CampaignDetail {
	return core.CampaignDetail{
		CampaignSummary: core.CampaignSummary{ID: 7, Name: "Siege", Status: core.CampaignActive},
		Organiser:       "Gandalf", Players: 2,
	}
}

func TestCampaignOverviewWithoutCompanyOffersToCreateOne(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil)))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Create my company") || !strings.Contains(body, "/campaigns/7/rankings") {
		t.Fatalf("got %d\n%s", rec.Code, body)
	}
}

func TestCampaignOverviewWithCompanyOffersToManageIt(t *testing.T) {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.Companies = []core.CampaignCompany{{ID: 3, Name: "Rangers", Faction: "Gondor"}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return(nil, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7", nil))).Body.String()
	if !strings.Contains(body, `href="/campaigns/7/my-company/3" class="btn btn-primary">Manage company`) || strings.Contains(body, "Create my company") {
		t.Errorf("expected manage, not create:\n%s", body)
	}
}

func TestUnknownCampaignIs404(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(9)).Return(core.CampaignDetail{}, core.ErrNotFound)
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/9", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("got %d", rec.Code)
	}
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/abc/rankings", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("non-numeric id: got %d", rec.Code)
	}
}

func TestRankingsAreSorted(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{
		{ID: 1, Name: "Small", Points: 100}, {ID: 2, Name: "Big", Points: 400, Wins: 2, Draws: 1, Losses: 0},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/rankings", nil))).Body.String()
	if big, small := strings.Index(body, ">Big<"), strings.Index(body, ">Small<"); big < 0 || small < 0 || big > small {
		t.Errorf("Big should come first (big=%d small=%d)", big, small)
	}
	for _, want := range []string{`tone-win">2<`, `tone-draw">1<`, `tone-loss">0<`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing coloured win/draw/loss %q", want)
		}
	}
}

func TestOthersCompanyCannotBeManaged(t *testing.T) {
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.Companies = []core.CampaignCompany{{ID: 1, Name: "Mine", Faction: "Gondor", Owner: "Frodo"}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).Times(3)
	f.camp.EXPECT().Company(gomock.Any(), int64(7), int64(1)).Return(d.Companies[0], nil)
	f.camp.EXPECT().Company(gomock.Any(), int64(7), int64(2)).Return(core.CampaignCompany{ID: 2, Name: "Theirs", Owner: "Sam"}, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(core.CompanyDetail{CampaignCompany: d.Companies[0], Influence: 4, Gold: 9,
		Limits: []core.CompanyLimit{{Label: "Bows", Used: 1, Max: 2}},
		Roster: []core.CompanyUnit{
			{ID: 1, Name: "Beregond", Type: "Captain", Role: core.RoleLeader, Kind: core.KindInfantry, Points: 70, Level: 0, XP: 11, LevelEvery: 5,
				Stats:     []core.Attribute{{Abbr: "Mv", Name: "Movement", Value: 6, Trail: `"`}, {Abbr: "FV", Name: "Fight", Value: 5, Boost: 1}, {Abbr: "M", Name: "Might", Value: 3, HeroOnly: true}},
				Equipment: []core.Equipment{{Name: "Sword", Class: core.EquipMelee}, {Name: "Bow", Class: core.EquipBow, Bought: true, Stats: `24" · S2`, Description: "Can shoot."}, {Name: "Elven Cloak", Class: core.EquipOther, Description: "Stalk Unseen."}, {Name: "Helmingas", Class: core.EquipOther, Upgrade: true, Description: "Guards of the king."}},
				Rules:     []core.UnitRule{{Name: "Bodyguard", Description: "Protects the Leader.", Source: core.RuleFromUnit}, {Name: "Wither", Description: "Bad things.", Source: core.RuleFromUnit, Spell: true, Casting: 3, Range: 12}},
				Path: &core.UnitPath{Name: "Path of the Warlord",
					HeroicActions: []core.HeroicAction{{Name: "Heroic Strike", Source: core.ActionFromUnit}, {Name: "Heroic March", Source: core.ActionBought}},
					Rules:         []core.UnitRule{{Name: "Rally the Troops", Description: "Cheers.", Source: core.RuleFromPath}},
					Powers:        []core.UnitRule{{Name: "Black Dart", Description: "Hits.", Source: core.RuleFromPath, Spell: true, Casting: 3, Range: 12}}}},
			{ID: 2, Type: "Warrior", Role: core.RoleWarrior, Kind: core.KindCavalry, Points: 8},
		}}, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(2)).Return(core.CompanyDetail{CampaignCompany: core.CampaignCompany{ID: 2, Name: "Theirs", Owner: "Sam"}, Roster: []core.CompanyUnit{{ID: 9, Name: "Sam's Captain", Type: "Captain", Role: core.RoleLeader, Points: 60}}}, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()

	// Your own company has its tab: its own page just sends you there.
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/1", nil)))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/my-company/1" {
		t.Errorf("own company page: %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	for _, want := range []string{`aria-current="page">My Company`, "Influence", "Gold", "Bows", "1 / 2", "Beregond", "unit-leader", "insignia-leader", "+ Bow", `24&#34; · S2`, "has-tip", "Bodyguard", "rule-spell", "3+", "FV", "path-box", "Path of the Warlord", "Heroic Strike", "+ Heroic March", "Rally the Troops", "Black Dart", "learned through the path", `title="Level 0"`, "11 / 5 XP", "Level Up ready!", "×2", "unit-corner", "70", `data-tip-title="Elven Cloak"`, `data-tip-text="Stalk Unseen."`, `data-tip-title="Helmingas" data-tip-meta="Unit upgrade" data-tip-text="Guards of the king."`} {
		if !strings.Contains(body, want) {
			t.Errorf("My Company tab lacks %q", want)
		}
	}
	body = f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/companies/2", nil))).Body.String()
	for _, want := range []string{"Viewing <strong>Theirs</strong>", "Read only", "Sam&#39;s Captain", `id="perks"`, `id="aftermath-history"`} {
		if !strings.Contains(body, want) {
			t.Errorf("another company's page lacks %q", want)
		}
	}
	if strings.Contains(body, "/use\"") {
		t.Error("others cannot mark perks used")
	}
	for _, banned := range []string{"Recruit ·", "Change formation", "/rename", "Your company", "/armoury", "Buy equipment"} {
		if strings.Contains(body, banned) {
			t.Errorf("another company's page is read only, but has %q", banned)
		}
	}
}

func TestMyCompanyTabWithoutCompanyOffersToCreateOne(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company", nil))).Body.String()
	if !strings.Contains(body, "Create my company") {
		t.Error("no company yet: offer to create one")
	}
}

func TestArmoryTab(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/armory?company=1", nil))).Body.String()
	if !strings.Contains(body, `aria-current="page">Armory`) {
		t.Error("the Armory tab should be current")
	}
}

func TestCreateCompanyOnlyOnce(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return([]core.CompanyProfile{{BCCompany: core.BCCompany{Name: "Gondor", Alignment: "good"}}}, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	in := core.CompanyInput{Name: "Rangers", Faction: "Gondor"}
	f.camp.EXPECT().CreateCompany(gomock.Any(), frodo, int64(7), in).Return(core.CampaignCompany{}, core.ErrCompanyLimit)
	rec := f.do(f.signedIn(form("/campaigns/7/company", url.Values{"name": {"Rangers"}, "faction": {"Gondor"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "You already have a company in this campaign.") {
		t.Errorf("got %d", rec.Code)
	}

	d := campaignDetail()
	d.Companies = []core.CampaignCompany{{ID: 5}}
	f2 := newFixture(t, Config{})
	f2.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	rec = f2.do(f2.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/new", nil)))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/my-company" {
		t.Errorf("a player at the cap is sent to the alias: %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestCreateMatchBuildsSidesFromTheForm(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}, nil)
	want := []core.NewSide{{Name: "Gondor", Companies: []int64{1, 3}}, {Companies: []int64{2}}, {Name: "Spies"}}
	f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), core.NewMatch{Sides: want}).Return(core.Match{ID: 12}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/matches", url.Values{
		"sides": {"3"}, "side-name-1": {"Gondor"}, "side-1": {"1", "3", "99"}, // 99 is not in the campaign: ignored
		"side-2": {"2"}, "side-name-3": {"Spies"},
	})))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/campaigns/7/matches/12" {
		t.Errorf("got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestMatchListColoursTheOutcome(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(twoCompanies, nil).AnyTimes()
	sides := func(a, b string) []core.MatchSide {
		return []core.MatchSide{{Name: "Side 1", Companies: []core.CampaignCompany{{Name: a}}}, {Name: "Side 2", Companies: []core.CampaignCompany{{Name: b}}}}
	}
	f.camp.EXPECT().Matches(gomock.Any(), int64(7)).Return([]core.Match{
		{ID: 1, Ended: true, Winner: 1, Sides: sides("Victor", "Loser")},
		{ID: 2, Ended: true, Winner: 0, Sides: sides("Tie1", "Tie2")},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/matches", nil))).Body.String()
	for _, want := range []string{`tone-win" title="Side 1">Victor`, `tone-loss" title="Side 2">Loser`, `tone-draw" title="Side 1">Tie1`, `tone-draw" title="Side 2">Tie2`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestNavCampaigns(t *testing.T) {
	f := newFixture(t, Config{})
	f.nav = []core.CampaignNav{{ID: 4, Name: "Rohan Rising"}}
	rec := f.do(f.signedIn(httptest.NewRequest("GET", "/nav/campaigns", nil)))
	if body := rec.Body.String(); rec.Code != 200 || !strings.Contains(body, `href="/campaigns/4"`) || strings.Contains(body, "<html") {
		t.Errorf("got %d: %s", rec.Code, body)
	}
}

func TestCreateMatchInvalid(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}, nil)
	f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), gomock.Any()).
		Return(core.Match{}, errors.Join(core.ErrInvalidMatch, errors.New("every side needs at least one company")))
	rec := f.do(f.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "every side needs") {
		t.Errorf("got %d", rec.Code)
	}
}

func TestCreateMatchRefusedShowsTheReason(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}, nil)
	f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), gomock.Any()).
		Return(core.Match{}, core.Refuse("A cannot play yet: Rider has to choose a path first."))
	rec := f.do(f.signedIn(form("/campaigns/7/matches", url.Values{"sides": {"2"}, "side-1": {"1"}, "side-2": {"2"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "A cannot play yet") {
		t.Errorf("got %d", rec.Code)
	}
}

func TestFinishedCampaignsFollowTheCookie(t *testing.T) {
	f := newFixture(t, Config{})
	list := []core.CampaignSummary{
		{ID: 1, Name: "Live", Status: core.CampaignActive},
		{ID: 2, Name: "Over", Status: core.CampaignFinished},
	}
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, true).Return(list, nil)
	f.nav = []core.CampaignNav{{ID: 1, Name: "Live", Status: core.CampaignActive}, {ID: 2, Name: "Over", Status: core.CampaignFinished}}
	for _, path := range []string{"/", "/nav/campaigns"} {
		r := f.signedIn(httptest.NewRequest("GET", path, nil))
		r.AddCookie(&http.Cookie{Name: finishedCookie, Value: "1"})
		body := f.do(r).Body.String()
		if !strings.Contains(body, "Over") || strings.Index(body, "Live") > strings.Index(body, "Over") {
			t.Errorf("GET %s: finished campaigns should be listed after the active ones", path)
		}
		if path == "/nav/campaigns" && (!strings.Contains(body, "!text-muted") || !f.navFinished) {
			t.Error("finished campaigns are greyed out in the sidebar and asked for")
		}
	}
}

func TestCreateMatchAllVsAll(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return([]core.CampaignCompany{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 3, Name: "C"}}, nil)
	want := []core.NewSide{{Name: "C", Companies: []int64{3}}, {Name: "A", Companies: []int64{1}}}
	f.camp.EXPECT().CreateMatch(gomock.Any(), frodo, int64(7), core.NewMatch{Sides: want}).Return(core.Match{ID: 5}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/matches", url.Values{"mode": {"ffa"}, "sides": {"2"}, "side-1": {"3", "1"}})))
	if rec.Code != http.StatusSeeOther {
		t.Errorf("got %d", rec.Code)
	}
}

func TestNavCampaignsHaveTheirPages(t *testing.T) {
	f := newFixture(t, Config{})
	f.nav = []core.CampaignNav{{ID: 4, Name: "Rohan Rising"}}
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/nav/campaigns", nil))).Body.String()
	for _, want := range []string{`href="/campaigns/4"`, `href="/campaigns/4/companies"`, `href="/campaigns/4/rankings"`, `href="/campaigns/4/matches"`, "inCampaign(4)"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestNavCampaignLinksTargetTheMainPanel(t *testing.T) {
	f := newFixture(t, Config{})
	f.nav = []core.CampaignNav{{ID: 4, Name: "Rohan Rising"}}
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/nav/campaigns", nil))).Body.String()
	if !strings.Contains(body, `hx-target="#main"`) {
		t.Errorf("links inherit the list's own hx-target unless they set #main:\n%s", body)
	}
}

func TestHistoryRestoreGetsTheFullPage(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return(nil, nil)
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, true).Return(nil, nil)
	r := f.signedIn(httptest.NewRequest("GET", "/", nil))
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-History-Restore-Request", "true")
	if body := f.do(r).Body.String(); !strings.Contains(body, "<html") {
		t.Errorf("going back swaps the whole body, so the layout is needed:\n%.200s", body)
	}
}

func sampleGlossary() core.Glossary {
	return core.Glossary{
		Info: core.RulesetInfo{Kind: core.KindGlossary, Name: "MESBG Glossary", Version: "1.0", Hash: "abcdef0123456789"},
		Terms: []core.GlossaryTerm{
			{ID: "stand-fast", Name: "Stand Fast", Category: "heroic", Aliases: []string{"Stand Fasts"}, Pages: []int{5},
				Paragraphs: [][]core.Segment{{{Text: "Ignore "}, {Text: "Break Tests", TermID: "break-test"}, {Text: "."}}},
				See:        []core.TermRef{{ID: "break-test", Name: "Break Test"}}},
			{ID: "break-test", Name: "Break Test", Category: "morale", Paragraphs: [][]core.Segment{{{Text: "Roll a dice."}}}},
		},
	}
}

func TestGlossaryPages(t *testing.T) {
	f := newFixture(t, Config{})
	f.rs.EXPECT().Default(gomock.Any(), core.KindGlossary).Return(core.RulesetInfo{Kind: core.KindGlossary, Name: "MESBG Glossary", Version: "1.0", Hash: "abcdef0123456789"}, nil).AnyTimes()
	f.gl.EXPECT().Default(gomock.Any()).Return(sampleGlossary(), nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/glossary", nil))).Body.String()
	for _, want := range []string{`id="t-stand-fast"`, `data-aliases="Stand Fasts"`, `<a href="#t-break-test">Break Tests</a>`, "MESBG Glossary", "v1.0", "abcdef01", "2 terms", `x-data="glossary"`, `data-chip="heroic"`, `data-chip="morale"`, `aria-label="Filter by category"`} {
		if !strings.Contains(body, want) {
			t.Errorf("glossary page lacks %q", want)
		}
	}

	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(sampleGlossary(), nil)
	body = f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/glossary", nil))).Body.String()
	if !strings.Contains(body, `aria-current="page">Glossary`) || !strings.Contains(body, `id="t-break-test"`) {
		t.Error("the campaign's Glossary tab should be current and list the terms")
	}
}

func mineDetail() (core.CampaignDetail, core.CompanyDetail) {
	d := campaignDetail()
	d.Companies = []core.CampaignCompany{{ID: 1, Name: "Rangers", Faction: "Gondor"}}
	return d, core.CompanyDetail{CampaignCompany: d.Companies[0], Influence: 4, Gold: 1, MaxUnits: 15,
		Recruit:  core.RecruitInfo{Rules: recruitRules, Cost: 3, Chart: []core.RecruitRow{{Min: 2, Max: 2, Text: "Warrior of Minas Tirith (Shield)"}, {Min: 6, Max: 6, Text: "Roll on the special chart"}}, SpecialChart: []core.RecruitRow{{Min: 1, Max: 3, Text: "Citadel Guard"}}, Notes: []string{"A rare unit the company cannot have is replaced by a lower result."}, Special: []core.SpecialRecruit{{Unit: "Cave Troll", Cost: 15, Rare: 1}}},
		Roster:   []core.CompanyUnit{{ID: 5, Name: "Hador", Type: "Warrior", Role: core.RoleSergeant, Kind: core.KindInfantry, Points: 8}},
		RuleDice: testRuleDice}
}

// testRuleDice are the dice of the shipped rules file: warriors roll a d6, heroes two results of 2d6, a Broken model tests Courage on 2d6.
var testRuleDice = core.RuleDice{Level: core.LevelDice{Warrior: core.DiceSpec{Count: 1, Sides: 6}, Hero: core.DiceSpec{Count: 2, Sides: 6}, Pairs: 2}, Courage: core.DiceSpec{Count: 2, Sides: 6}}

func TestRecruitWindowShowsBothChartsApart(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/recruit", nil))).Body.String()
	for _, want := range []string{"3 influence per roll", "Reinforcements", "Warrior of Minas Tirith (Shield)", "Special chart", "Citadel Guard", "rare unit the company cannot have",
		"Cave Troll", "15 influence", "That costs 15 influence and the company has 4"} {
		if !strings.Contains(body, want) {
			t.Errorf("recruit window lacks %q", want)
		}
	}
	if strings.Contains(body, "<html") {
		t.Error("the window is a fragment")
	}
}

func TestARecruitIsRolledThenConfirmedInTheWindow(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	d.Settings.ManualRolls = true
	detail.Influence = 6
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()

	// 1. the roll: the window stays and shows the result with its adjustment buttons
	f.camp.EXPECT().RollRecruitWithPerk(gomock.Any(), frodo, int64(7), int64(1), 4, false, 0).Return(nil)
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(&core.RecruitRoll{Stage: "chart", Rolled: true, Roll: 4, Budget: 3, Adjust: upToThree}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/recruit/roll", url.Values{"roll": {"4"}})))
	body := rec.Body.String()
	for _, want := range []string{"You rolled a", "Raise the roll", "Confirm", "recruit/confirm", `name="adjust"`,
		`id="company-influence" hx-swap-oob="true">6</p>`} { // the page's balance follows the roll at once (the price leaves it when rolling)
		if !strings.Contains(body, want) {
			t.Errorf("the rolled window lacks %q", want)
		}
	}
	if rec.Code != 200 || strings.Contains(body, "<html") || rec.Header().Get("HX-Retarget") != "" {
		t.Errorf("an intermediate step answers with the window only: %d", rec.Code)
	}

	// 2. a result that leads to the special chart keeps the window open
	f.camp.EXPECT().ConfirmRecruit(gomock.Any(), frodo, int64(7), int64(1), 1, "").Return(core.RecruitResult{Roll: 6, Summary: "special", Next: true}, nil)
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(&core.RecruitRoll{Stage: "special", Roll: 0, Budget: 2, Adjust: upToThree}, nil)
	rec = f.do(f.signedIn(form("/campaigns/7/my-company/1/recruit/confirm", url.Values{"adjust": {"1"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "special chart") || !strings.Contains(rec.Body.String(), "Roll on the special chart") || rec.Header().Get("HX-Retarget") != "" {
		t.Errorf("the special chart asks for a second roll in the same window: %d", rec.Code)
	}

	// 3. the final confirm ends the recruit: the whole page answers and the window goes away
	f.camp.EXPECT().ConfirmRecruit(gomock.Any(), frodo, int64(7), int64(1), 0, "").Return(core.RecruitResult{Roll: 2, Summary: "You rolled a 2: a Warrior joins."}, nil)
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	rec = f.do(f.signedIn(form("/campaigns/7/my-company/1/recruit/confirm", url.Values{"adjust": {"0"}})))
	if rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#main" || !strings.Contains(rec.Body.String(), "You rolled a 2: a Warrior joins.") {
		t.Errorf("the end of a recruit answers with the page: %d, retarget %q", rec.Code, rec.Header().Get("HX-Retarget"))
	}
}

func TestARecruitInProgressReopensItsWindow(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).Times(2)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(&core.RecruitRoll{Stage: "chart", Rolled: true, Roll: 2, Budget: 3, Adjust: upToThree}, nil).Times(2)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	if !strings.Contains(body, `data-modal="true"`) || !strings.Contains(body, "You rolled a") {
		t.Error("a recruit in progress is not lost on reload: its window opens on it")
	}
}

func TestRefusedPurchasesShowTheReason(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).Times(2)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	f.camp.EXPECT().BuyEquipment(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Map", gomock.Any()).Return(core.Refuse("Map: no free small slot"))
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/equipment", url.Values{"item": {"Map"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Map: no free small slot") || !strings.Contains(rec.Body.String(), "alert-danger") {
		t.Errorf("got %d", rec.Code)
	}
}

func TestArmouryWindow(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().Armoury(gomock.Any(), int64(7), int64(1), int64(5)).Return([]core.ArmouryItem{
		{GearOffer: core.GearOffer{Name: "Badge of Courage", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 2, Size: "small"}, PayInfluence: true, PayGold: true, GoldPrice: 1},
		{GearOffer: core.GearOffer{Name: "Map", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 1, Size: "small", Restrictions: []string{"hero"}}, Reason: "Heroes only"},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/armoury", nil))).Body.String()
	for _, want := range []string{"Equipment for Hador", "Buy · 2 influence", "Buy · 1 gold", "Heroes only", "small slot", `name="item" value="Badge of Courage"`, "Armory tab", `type="search"`, "Find an item", `data-find="Badge of Courage `, `data-find="Map `} {
		if !strings.Contains(body, want) {
			t.Errorf("armoury window lacks %q", want)
		}
	}
	if strings.Contains(body, `value="Map"`) {
		t.Error("an item that cannot be bought has no buy button")
	}
}

func TestAChoiceOfOptionsOpensItsWindow(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).Times(3)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).Times(2)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).Times(2)
	pending := &core.RecruitChoice{Unit: "Warrior of Minas Tirith", Options: []core.ChoiceOption{{Name: "Shield and Spear", Points: 2}, {Name: "Bow", Points: 1}}}
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(pending, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	for _, want := range []string{`data-modal="true"`, "Choose its options", "Shield and Spear", "+2 pts", "Its base profile"} {
		if !strings.Contains(body, want) {
			t.Errorf("the choice window lacks %q", want)
		}
	}
	f.camp.EXPECT().ChooseOptions(gomock.Any(), frodo, int64(7), int64(1), "Bow").Return(core.RecruitResult{Summary: "A Warrior of Minas Tirith joins the company with Bow."}, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/recruit/choose", url.Values{"option": {"Bow"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "joins the company with Bow") || strings.Contains(rec.Body.String(), `data-modal="true"`) {
		t.Errorf("choosing finishes the recruit: %d", rec.Code)
	}
}

func TestDroppingEquipment(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).Times(2)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	f.camp.EXPECT().DropEquipment(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Bow").Return(nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/equipment/drop", url.Values{"item": {"Bow"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Dropped Bow.") {
		t.Errorf("got %d", rec.Code)
	}
}

func TestExpandedEquipmentCanBeDropped(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].Equipment = []core.Equipment{{Name: "Heavy Armour", Class: core.EquipArmour}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	if !strings.Contains(body, `/units/5/equipment/drop`) || !strings.Contains(body, `value="Heavy Armour"`) || !strings.Contains(body, "hx-confirm") {
		t.Error("a unit can drop what the rules allow, from its expanded card")
	}
}

func TestEquipmentThatCannotBeDroppedSaysWhy(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].Equipment = []core.Equipment{{Name: "Heavy Armour", Class: core.EquipArmour, Profile: true, Drop: "Part of its profile"}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	if strings.Contains(body, `/units/5/equipment/drop`) || !strings.Contains(body, "Part of its profile") {
		t.Error("an item the rules do not let the unit drop has no button, only the reason")
	}
}

func TestOptionalRuleIsChosenWhenCreatingACampaign(t *testing.T) {
	f := newFixture(t, Config{})
	maker, do := campaignMaker(f)
	f.defaultsShown()
	f.camp.EXPECT().Create(gomock.Any(), maker, core.CampaignInput{Name: "House rules", Settings: core.CampaignSettings{HeroesDropProfileGear: true}}).Return(core.CampaignSummary{ID: 1}, nil)
	if rec := do(form("/campaigns", url.Values{"name": {"House rules"}, "option-heroesDropProfileGear": {"on"}})); rec.Code != http.StatusSeeOther {
		t.Errorf("got %d", rec.Code)
	}
	rec := do(httptest.NewRequest("GET", "/campaigns/new", nil))
	if !strings.Contains(rec.Body.String(), "Heroes may discard any wargear") || !strings.Contains(rec.Body.String(), `name="option-heroesDropProfileGear"`) {
		t.Error("the form offers the optional rule")
	}
}

func TestInjuredUnitsAreMarkedAndNotCounted(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].Injured = true
	detail.InjuredUnits = 1
	detail.Points = 40
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	for _, want := range []string{"Out next match", "unit-injured", "1 injured, not counted"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestArmoryTabListsEverythingWithTheUnitsThatCanTakeIt(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryWanderers(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryEntries(gomock.Any(), int64(7), int64(1)).Return([]core.ArmouryEntry{
		{GearOffer: core.GearOffer{Name: "Badge of Courage", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 2, Size: "small", Grants: []string{"Fearless"}},
			Units: []core.UnitOption{{ID: 5, Title: "Hador", Type: "Warrior", Role: core.RoleSergeant}}},
		{GearOffer: core.GearOffer{Name: "Barding", Classes: []core.EquipmentClass{core.EquipArmour}, Cost: 2, Size: "large", Restrictions: []string{"cavalry"}},
			Units: []core.UnitOption{{ID: 5, Title: "Hador", Type: "Warrior", Role: core.RoleSergeant, Reason: "Cavalry only", Blocker: core.BlockedByRule}}},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/armory?company=1", nil))).Body.String()
	for _, want := range []string{"Badge of Courage", "Barding", "Buy for…", "No unit can take it", "Cavalry only", `/armory/buy`, `name="item" value="Badge of Courage"`, "Gives:", "Fearless", `x-data="armory"`} {
		if !strings.Contains(body, want) {
			t.Errorf("armory lacks %q", want)
		}
	}
	if strings.Contains(body, `name="item" value="Barding"`) {
		t.Error("a unit that cannot take an item has no buy button")
	}
}

func TestArmoryOfALockedCompanyExplainsAndDisablesTheBuyButtons(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Aftermaths = []core.AftermathRef{{MatchID: 3, MatchNumber: 3, CompanyID: 1, Status: core.AftermathPending}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryWanderers(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryEntries(gomock.Any(), int64(7), int64(1)).Return([]core.ArmouryEntry{
		{GearOffer: core.GearOffer{Name: "Badge of Courage", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 2, Size: "small"},
			Units: []core.UnitOption{{ID: 5, Title: "Hador", Type: "Warrior", Role: core.RoleSergeant}}},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/armory?company=1", nil))).Body.String()
	for _, want := range []string{`id="armory-lock"`, "The company is locked.", "Finish the aftermath of Match 3 first.", `disabled title="Finish the aftermath of Match 3 first."`} {
		if !strings.Contains(body, want) {
			t.Errorf("a locked armory lacks %q", want)
		}
	}
}

func TestBuyingFromTheArmory(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).Times(2)
	f.camp.EXPECT().ArmouryEntries(gomock.Any(), int64(7), int64(1)).Return(nil, nil).Times(2)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryWanderers(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().BuyEquipment(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Map", gomock.Any()).Return(nil)
	rec := f.do(f.signedIn(form("/campaigns/7/armory/buy", url.Values{"company": {"1"}, "unit": {"5"}, "item": {"Map"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Bought Map.") {
		t.Errorf("got %d", rec.Code)
	}
	f.camp.EXPECT().BuyEquipment(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Seeing Stone", gomock.Any()).Return(core.Refuse("Seeing Stone: heroes only"))
	rec = f.do(f.signedIn(form("/campaigns/7/armory/buy", url.Values{"company": {"1"}, "unit": {"5"}, "item": {"Seeing Stone"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "heroes only") {
		t.Errorf("refused: %d", rec.Code)
	}
}

func TestArmoryTabAsksForTheWeaponOfAWeaponModifier(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster = []core.CompanyUnit{{ID: 5, Name: "Hador", Type: "Warrior", PoisonableWeapons: []string{"Bow", "Sword"}}, {ID: 6, Name: "Bare", Type: "Warrior"}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).Times(3)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).Times(2)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryWanderers(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().ArmouryEntries(gomock.Any(), int64(7), int64(1)).Return([]core.ArmouryEntry{
		{GearOffer: core.GearOffer{Name: "Envenom Weapon (X)", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 2, Size: "small", WeaponModifier: true},
			Units: []core.UnitOption{{ID: 5, Title: "Hador", Type: "Warrior", PayInfluence: true}, {ID: 6, Title: "Bare", Type: "Warrior", PayInfluence: true}}},
	}, nil).Times(2)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/armory?company=1", nil))).Body.String()
	for _, want := range []string{`name="weapon"`, `<option value="Bow"`, `<option value="Sword"`, "Choose a weapon", "No weapon left for Envenom Weapon (X)"} {
		if !strings.Contains(body, want) {
			t.Errorf("armory lacks %q", want)
		}
	}
	if strings.Count(body, `name="weapon"`) != 1 {
		t.Error("only the unit with a weapon gets a chooser; the other gets the reason")
	}
	f.camp.EXPECT().BuyEquipmentFor(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Envenom Weapon (X)", gomock.Any(), "Bow").Return(nil)
	rec := f.do(f.signedIn(form("/campaigns/7/armory/buy", url.Values{"company": {"1"}, "unit": {"5"}, "item": {"Envenom Weapon (X)"}, "weapon": {"Bow"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Bought Envenom Weapon (X).") {
		t.Errorf("got %d", rec.Code)
	}
}

func TestSavingTheFormation(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).Times(2)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	f.camp.EXPECT().SaveFormation(gomock.Any(), frodo, int64(7), int64(1), []int64{5, 2, 9}).Return(nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/formation", url.Values{"order": {"5, 2,x,9"}})))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Formation saved.") {
		t.Errorf("got %d", rec.Code)
	}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).Times(2)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	f.camp.EXPECT().SaveFormation(gomock.Any(), frodo, int64(7), int64(1), []int64{42}).Return(core.Refuse("Unit 42 is not in the company."))
	if rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/formation", url.Values{"order": {"42"}}))); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "not in the company") {
		t.Errorf("refused: %d", rec.Code)
	}
}

func TestUnitCardsCanBeDraggedWhenChangingTheFormation(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
	for _, want := range []string{"Change formation", `data-unit-id="5"`, "unit-handle", "Save formation", `name="order"`, "my-company/1/formation", "js/app.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestAThrownRollNeedsTheShakeTheTableSetting(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()

	// the page tells the backend the browser threw the dice; the backend refuses when the campaign does not allow it
	f.camp.EXPECT().RollRecruitWithPerk(gomock.Any(), frodo, int64(7), int64(1), 5, true, 0).Return(core.Refuse("%s", core.CampaignSettings{}.RollRefusal(true, true)))
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/recruit/roll", url.Values{"roll": {"5"}, "thrown": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "does not let players throw") {
		t.Errorf("a thrown roll is refused when the organiser did not allow it: %d", rec.Code)
	}
}

func TestReopeningTheRecruitWindowContinuesAPendingChoice(t *testing.T) {
	f := newFixture(t, Config{})
	d, _ := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(&core.RecruitChoice{Unit: "Warrior of Minas Tirith", Options: []core.ChoiceOption{{Name: "Bow", Points: 1}}}, nil)
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/recruit", nil))).Body.String()
	if !strings.Contains(body, "Choose its options") || strings.Contains(body, "influence per roll") {
		t.Errorf("the window offers the pending choice, not a new recruit")
	}
}

func TestTreatingAHeroShowsTheResultInTheWindow(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Influence = 3
	detail.Injuries = core.InjuryRules{Attempt: core.TreatMethod{Key: "attempt", Cost: 1, Target: 5, Ceiling: 6, PerPoint: 1, Healable: []string{"armWound"}}}
	detail.Roster[0].Role = core.RoleHero
	detail.Roster[0].Injuries = []core.UnitInjury{{Key: "armWound", Name: "Arm Wound", Count: 1}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().TreatmentState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().TreatInjury(gomock.Any(), frodo, int64(7), int64(1), int64(5), core.Treatment{Method: "attempt", Injury: "armWound", Roll: 0}).
		Return(core.TreatResult{Rolled: true, Roll: 3, Total: 4, Needed: 5, Summary: "Rolled 4 (5 needed): the treatment failed and the influence is lost."}, nil)

	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/treat", url.Values{"method": {"attempt"}, "injury": {"armWound"}})))
	body := rec.Body.String()
	for _, want := range []string{"Treat Hador", "the treatment failed", `data-animate="true"`, `name="injury"`, "Arm Wound", "Done"} {
		if !strings.Contains(body, want) {
			t.Errorf("the window lacks %q", want)
		}
	}
	if rec.Code != 200 || strings.Contains(body, "<html") {
		t.Errorf("a hero's treatment answers with the window only: %d", rec.Code)
	}

	// a roll thrown in the browser is passed on as thrown; the backend refuses it when the organiser did not allow it
	f.camp.EXPECT().TreatInjury(gomock.Any(), frodo, int64(7), int64(1), int64(5), core.Treatment{Method: "attempt", Injury: "armWound", Roll: 6, Thrown: true}).
		Return(core.TreatResult{}, core.Refuse("%s", core.CampaignSettings{}.RollRefusal(true, true)))
	rec = f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/treat", url.Values{"method": {"attempt"}, "injury": {"armWound"}, "roll": {"6"}, "thrown": {"1"}})))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "does not let players throw") {
		t.Errorf("a thrown roll needs the setting: %d", rec.Code)
	}
}

func TestAHeroWithOnlyOneWayToTreatGetsASingleColumn(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Influence = 3
	detail.Injuries = core.InjuryRules{Attempt: core.TreatMethod{Key: "attempt", Cost: 1, Target: 5, Ceiling: 6, PerPoint: 1, Healable: []string{"justAScratch"}}}
	detail.Roster[0].Role = core.RoleHero
	detail.Roster[0].Injuries = []core.UnitInjury{{Key: "justAScratch", Name: "T'is Just a Scratch", Count: 1}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().TreatmentState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/treat", nil))).Body.String()
	if !strings.Contains(body, "Field treatment") || strings.Contains(body, "md:grid-cols-[1fr_auto_1fr]") {
		t.Errorf("one way to treat takes the whole width")
	}
}

func TestTheLevelUpWindowRollsAndOffersTheResults(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	d.Settings.ManualRolls = true
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	detail.Roster[0].XP, detail.Roster[0].LevelEvery = 10, 5
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().LevelUpState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/levelup", nil))).Body.String()
	for _, want := range []string{"Level up Hador", "2</strong> rolls waiting", "levelup/roll", "Roll for me"} {
		if !strings.Contains(body, want) {
			t.Errorf("the roll form lacks %q", want)
		}
	}
	f.camp.EXPECT().RollLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), nil, false).Return(core.LevelRoll{Kind: "warrior", D6: 6, Result: "heroInTheMaking",
		Offers: []core.LevelOffer{{Roll: 6, Text: "A hero", Choices: []core.LevelChoice{{Key: "path:Path of the Duelist", Label: "Become a hero: Path of the Duelist", Group: "Choose its path"}}}}}, nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/roll", url.Values{"roll": {"0"}})))
	body = rec.Body.String()
	for _, want := range []string{`data-animate="true"`, `data-dice="6"`, "Become a hero: Path of the Duelist", `value="path:Path of the Duelist"`, "levelup/choose"} {
		if !strings.Contains(body, want) {
			t.Errorf("the offers lack %q", want)
		}
	}
	f.camp.EXPECT().ChooseLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), []string{"path:Path of the Duelist"}).Return(core.LevelRoll{Kind: "warrior", Done: true, Summary: "Hador becomes a hero."}, nil)
	body = f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/choose", url.Values{"choice": {"path:Path of the Duelist"}}))).Body.String()
	if !strings.Contains(body, "Hador becomes a hero.") || !strings.Contains(body, "Done") {
		t.Errorf("the result is shown: %s", body)
	}
}

func TestTheSorcererPowerStepDoesNotSayToTakeTheOtherRoll(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].XP, detail.Roster[0].LevelEvery = 10, 5
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().LevelUpState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil).AnyTimes()
	offer := core.LevelOffer{Text: "A Sorcerer adds a magical power.", Choices: []core.LevelChoice{{Key: "power:Immobilise", Label: "Immobilise"}}}
	step := func(stage string) core.LevelRoll {
		return core.LevelRoll{Kind: "warrior", Again: true, Stage: stage, Offers: []core.LevelOffer{offer}}
	}
	const wording = "says to take the other roll too"
	f.camp.EXPECT().ChooseLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), []string{"path:Path of the Sorcerer"}).Return(step("startingPower"), nil)
	body := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/choose", url.Values{"choice": {"path:Path of the Sorcerer"}}))).Body.String()
	if !strings.Contains(body, "Immobilise") || strings.Contains(body, wording) {
		t.Errorf("the first power is offered without the other-roll wording: %s", body)
	}
	f.camp.EXPECT().ChooseLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), []string{"stat:Might:2"}).Return(step(""), nil)
	body = f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/levelup/choose", url.Values{"choice": {"stat:Might:2"}}))).Body.String()
	if !strings.Contains(body, wording) {
		t.Errorf("a hero result that asks for the other roll still says so: %s", body)
	}
}

func TestTypedRollsNeedTheCampaignSetting(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	detail.Roster[0].XP, detail.Roster[0].LevelEvery = 10, 5
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().LevelUpState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().TreatmentState(gomock.Any(), frodo, int64(7), int64(1), int64(5)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()

	// the roll form has only a Roll button
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/levelup", nil))).Body.String()
	if strings.Contains(body, "I rolled") || strings.Contains(body, "Roll for me") || !strings.Contains(body, ">Roll</button>") {
		t.Errorf("only a Roll button: %s", body)
	}
	// a typed roll reaches the backend, which refuses it
	msg := core.CampaignSettings{}.RollRefusal(true, false)
	f.camp.EXPECT().RollRecruitWithPerk(gomock.Any(), frodo, int64(7), int64(1), 4, false, 0).Return(core.Refuse("%s", msg))
	f.camp.EXPECT().TreatInjury(gomock.Any(), frodo, int64(7), int64(1), int64(5), core.Treatment{Method: "attempt", Injury: "armWound", Roll: 6}).Return(core.TreatResult{}, core.Refuse("%s", msg))
	f.camp.EXPECT().RollLevelUp(gomock.Any(), frodo, int64(7), int64(1), int64(5), []int{6}, false).Return(core.LevelRoll{}, core.Refuse("%s", msg))
	for _, c := range []struct {
		path string
		form url.Values
	}{
		{"/campaigns/7/my-company/1/recruit/roll", url.Values{"roll": {"4"}}},
		{"/campaigns/7/my-company/1/units/5/treat", url.Values{"method": {"attempt"}, "injury": {"armWound"}, "roll": {"6"}}},
		{"/campaigns/7/my-company/1/units/5/levelup/roll", url.Values{"roll": {"6"}}},
	} {
		rec := f.do(f.signedIn(form(c.path, c.form)))
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "does not let players enter their own rolls") {
			t.Errorf("%s: %d", c.path, rec.Code)
		}
	}
}

func TestTheDiceBundleIsServed(t *testing.T) {
	f := newFixture(t, Config{})
	rec := f.do(httptest.NewRequest("GET", "/static/js/dice3d.js", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Dice3D") {
		t.Errorf("the 3D dice bundle is part of the static files: %d", rec.Code)
	}
}

func TestRenamingAUnitReturnsToThePage(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().RenameUnit(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Hador the Bold").Return(nil)
	rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/rename", url.Values{"name": {"Hador the Bold"}})))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Renamed.") || !strings.Contains(body, `hx-post="/campaigns/7/my-company/1/units/5/rename"`) {
		t.Errorf("the page shows the rename form of every unit: %d", rec.Code)
	}
}

func TestCompanyInfoShowsTheEpigraphAboveTheCompanyRule(t *testing.T) {
	f := newFixture(t, Config{})
	profiles := []core.CompanyProfile{
		{BCCompany: core.BCCompany{Name: "Arnor", Alignment: "good", MaxModels: 15, ReinforcementCost: 3}, Rules: []string{"Rule"}, Substitutions: []string{"Own rule sentence."},
			Epigraph: &core.Epigraph{Quote: "We travel light.", Text: "The Dunedain grew wilder."}},
		{BCCompany: core.BCCompany{Name: "Mordor", Alignment: "evil", MaxModels: 15, ReinforcementCost: 3}, Substitutions: []string{"Other rule sentence."}},
	}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(profiles, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	get := func(c string) string {
		return f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c="+c, nil))).Body.String()
	}
	a := get("Arnor")
	q, r := strings.Index(a, "We travel light."), strings.Index(a, "Own rule sentence.")
	if q < 0 || r < 0 || q > r || !strings.Contains(a, "The Dunedain grew wilder.") || !strings.Contains(a, "data-epigraph") {
		t.Errorf("the epigraph comes before the company rule: %d %d", q, r)
	}
	if m := get("Mordor"); strings.Contains(m, "data-epigraph") {
		t.Error("a company without an epigraph draws none")
	}
}

func TestTheCompanyBrowserListsEveryCompanyAndShowsOne(t *testing.T) {
	f := newFixture(t, Config{})
	profiles := []core.CompanyProfile{
		{BCCompany: core.BCCompany{Name: "Minas Tirith", Alignment: "good", MaxModels: 15, ReinforcementCost: 3}, Description: "The White City.", Oath: core.Oath{Text: "By the White Tower I take this company. Let it be called {name}, now and always.", Vow: "I solemnly vow"},
			Start:        []core.StartUnit{{Count: 2, Unit: "Warrior of Minas Tirith", Wargear: []string{"shield"}}},
			Paths:        []core.PathProgression{{Name: "Path of the Warlord"}, {Name: "Path of the Sorcerer", LimitPerCompany: 1}},
			HeroUpgrades: []core.HeroUpgrade{{Name: "The Charge of Gondor", Rule: "The Charge of Gondor", Text: "Charge!"}},
			Previews:     map[string]core.UnitPreview{core.PreviewKey("Warrior of Minas Tirith", []string{"shield"}): {ID: "pv1", Name: "Warrior of Minas Tirith", Points: 8}}},
		{BCCompany: core.BCCompany{Name: "Mordor", Alignment: "evil", MaxModels: 15, ReinforcementCost: 3}},
	}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil).AnyTimes()
	f.camp.EXPECT().CompanyProfiles(gomock.Any(), int64(7)).Return(profiles, nil).AnyTimes()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/new", nil))).Body.String()
	for _, want := range []string{"Minas Tirith", "Mordor", "Pick a company from the list", "company/info?c=Mordor"} {
		if !strings.Contains(body, want) {
			t.Errorf("the list lacks %q", want)
		}
	}
	part := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Minas+Tirith", nil)))
	body = part.Body.String()
	for _, want := range []string{"The White City.", "Starting company", `data-preview="pv1"`, `id="pv1"`, "The Charge of Gondor", "Charge!", `name="faction" value="Minas Tirith"`, "Take command", `id="take-command"`, `name="leader"`, "data-index=\"1\"", "I solemnly vow", "Are you sure?"} {
		if !strings.Contains(body, want) {
			t.Errorf("the company page lacks %q", want)
		}
	}
	if part.Code != 200 || strings.Contains(body, "<html") {
		t.Errorf("a fragment: %d", part.Code)
	}
	if !strings.Contains(body, `<span class="eq-chip">Shield</span>`) || strings.Contains(body, ">shield<") {
		t.Errorf("wargear names start with a capital wherever they are shown")
	}
	if rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/company/info?c=Nowhere", nil))); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown company: %d", rec.Code)
	}
}

func matchFixture(t *testing.T, user core.User, m core.Match) (string, *fixture) {
	t.Helper()
	f := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = 99
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().Match(gomock.Any(), int64(7), m.ID).Return(m, nil)
	return "/campaigns/7/matches/" + itoa(int(m.ID)), f
}

func TestMatchPageWithoutScenario(t *testing.T) {
	m := core.Match{ID: 3, Number: 3, Sides: []core.MatchSide{
		{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers", OwnerID: frodo.ID}}},
		{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders", OwnerID: 50}}},
	}}
	path, f := matchFixture(t, frodo, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", path, nil))).Body.String()
	for _, want := range []string{"No scenario yet.", "Who won?", "Open"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	// a viewer outside the match reads the other empty state
	f2 := newFixture(t, Config{})
	d := campaignDetail()
	d.OrganiserID = 99
	f2.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	m.Sides[0].Companies[0].OwnerID = 51
	f2.camp.EXPECT().Match(gomock.Any(), int64(7), int64(3)).Return(m, nil)
	if body := f2.do(f2.signedIn(httptest.NewRequest("GET", path, nil))).Body.String(); !strings.Contains(body, "The players have not chosen a scenario yet.") {
		t.Errorf("read only empty state missing")
	}
}

func TestMatchPageShowsScenarioAndResult(t *testing.T) {
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	vp := 7
	m := core.Match{ID: 4, Number: 4, Ended: true, Winner: 2, EndedOn: &now,
		Scenario: &core.MatchScenario{Kind: core.ScenarioRulebook, Name: "Seize Ground"},
		Sides: []core.MatchSide{
			{Name: "Gondor", Companies: []core.CampaignCompany{{ID: 1, Name: "Rangers"}}},
			{Name: "Mordor", Companies: []core.CampaignCompany{{ID: 2, Name: "Raiders"}}},
		},
		Result: core.MatchResult{Decided: true, Winner: 2, Note: "<b>close</b>", Sides: []core.SideResult{{}, {VictoryPoints: &vp}},
			Companies: []core.CompanyResult{{CompanyID: 1, Fled: true}}}}
	path, f := matchFixture(t, frodo, m)
	f.camp.EXPECT().Scenarios(gomock.Any(), int64(7)).Return([]core.ScenarioInfo{{Name: "Seize Ground", Scoring: []string{"Hold the objectives"}}}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", path, nil))).Body.String()
	for _, want := range []string{"Seize Ground", "How it is scored", "Hold the objectives", "Ended 3 October 2026", `>Fled</span>`, "&lt;b&gt;close&lt;/b&gt;", `<span class="tone-win">Mordor</span> won`, "This match has ended."} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "<b>close</b>") {
		t.Error("the note must be escaped")
	}
}

func TestMatchPageEscapesAOneOffScenario(t *testing.T) {
	m := core.Match{ID: 5, Number: 5, Scenario: &core.MatchScenario{Kind: core.ScenarioCustom, Name: "Ambush <i>", Description: "line one\nline <two>", Victory: "last <script>"}}
	path, f := matchFixture(t, frodo, m)
	body := f.do(f.signedIn(httptest.NewRequest("GET", path, nil))).Body.String()
	for _, want := range []string{"One-off", "Ambush &lt;i&gt;", "line &lt;two&gt;", "last &lt;script&gt;", "whitespace-pre-line"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "<script>") && strings.Contains(body, "last <script>") {
		t.Error("victory text must be escaped")
	}
}

func TestSentenceStartsWithACapital(t *testing.T) {
	for in, want := range map[string]string{"the name must be": "The name must be", "Already fine": "Already fine", "": "", "écrit": "Écrit", "1 to 60": "1 to 60"} {
		if got := sentence(in); got != want {
			t.Errorf("sentence(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStaticDirectoriesAreNotListed(t *testing.T) {
	f := newFixture(t, Config{})
	for _, path := range []string{"/static/", "/static/js/", "/static/fonts/"} {
		if rec := f.do(httptest.NewRequest("GET", path, nil)); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d", path, rec.Code)
		}
	}
	if rec := f.do(httptest.NewRequest("GET", "/static/js/alpine.min.js", nil)); rec.Code != http.StatusOK {
		t.Errorf("a file: %d", rec.Code)
	}
}

func TestArmouryWindowSplitsWargearFromEquipmentAndHidesWhatARuleForbids(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return(nil, nil).AnyTimes()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
	f.camp.EXPECT().Armoury(gomock.Any(), int64(7), int64(1), int64(5)).Return([]core.ArmouryItem{
		{GearOffer: core.GearOffer{Name: "Badge of Courage", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 2, Size: "small"}, PayInfluence: true},
		{GearOffer: core.GearOffer{Name: "Armour", Classes: []core.EquipmentClass{core.EquipArmour}, Cost: 2, ArmourStage: 1}, PayInfluence: true},
		{GearOffer: core.GearOffer{Name: "Heavy Armour", Classes: []core.EquipmentClass{core.EquipArmour}, Cost: 3, ArmourStage: 2}, Reason: "Armour is upgraded one stage at a time", Blocker: core.BlockedByRule},
		{GearOffer: core.GearOffer{Name: "Banner", Cost: 3, Size: "large"}, Reason: "Not enough influence"},
	}, nil).AnyTimes()
	get := func(q string) string {
		return f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/units/5/armoury"+q, nil))).Body.String()
	}
	wargear := get("?kind=wargear")
	for _, want := range []string{"Wargear for Hador", `value="Armour"`, "armour upgrade"} {
		if !strings.Contains(wargear, want) {
			t.Errorf("wargear window lacks %q", want)
		}
	}
	for _, banned := range []string{"Badge of Courage", "Banner", "Heavy Armour", "Large slots"} {
		if strings.Contains(wargear, banned) {
			t.Errorf("wargear window has %q", banned)
		}
	}
	equipment := get("?kind=equipment")
	for _, want := range []string{"Equipment for Hador", "Badge of Courage", "Banner", "Not enough influence", "Large slots"} {
		if !strings.Contains(equipment, want) {
			t.Errorf("equipment window lacks %q", want)
		}
	}
	if strings.Contains(equipment, `"Armour"`) || strings.Contains(equipment, "Heavy Armour") {
		t.Error("equipment window lists wargear or a forbidden upgrade")
	}
}

func TestStatTipExplainsGearChanges(t *testing.T) {
	mods := []core.StatMod{{Stat: "defence", Delta: 1, From: "Shield"}, {Stat: "defence", From: "Shield", Note: "cancelled by a bow"}, {Stat: "strength", Delta: -1, From: "Pike", Note: "duel"}}
	if got, want := statTip("Defence", mods), "Defence: +1 from Shield; Shield: cancelled by a bow"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got := statTip("Fight", mods); got != "Fight" {
		t.Errorf("an unaffected stat has only its name: %q", got)
	}
}

func TestArmoryFiltersAndTooltips(t *testing.T) {
	f := newFixture(t, Config{})
	d, detail := mineDetail()
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil)
	f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
	f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
	f.camp.EXPECT().ArmouryEntries(gomock.Any(), int64(7), int64(1)).Return([]core.ArmouryEntry{
		{GearOffer: core.GearOffer{Name: "Badge of Courage", Classes: []core.EquipmentClass{core.EquipOther}, Cost: 2, Size: "small", Description: "Be brave."},
			Units: []core.UnitOption{{ID: 5, Title: "Hador", Type: "Warrior"}}},
		{GearOffer: core.GearOffer{Name: "Goat", Classes: []core.EquipmentClass{core.EquipMount}, Cost: 4},
			Units: []core.UnitOption{{ID: 5, Title: "Hador", Type: "Warrior", Reason: "No such option", Blocker: core.BlockedByRule}}},
		{GearOffer: core.GearOffer{Name: "Sword", Classes: []core.EquipmentClass{core.EquipMelee}, Cost: 1},
			Units: []core.UnitOption{{ID: 5, Title: "Hador", Type: "Warrior", Reason: "Costs 1", Blocker: core.BlockedByInfluence}}},
	}, nil)
	f.camp.EXPECT().ArmouryCreatures(gomock.Any(), int64(7), int64(1)).Return([]core.ArmouryCreature{
		{CreatureOffer: core.CreatureOffer{Name: "Hunting Dog", Cost: 1, Rules: []string{"Fearful"}}}}, nil)
	f.camp.EXPECT().ArmouryWanderers(gomock.Any(), int64(7), int64(1)).Return([]core.ArmouryWanderer{
		{WandererOffer: core.WandererOffer{Name: "Wandering Swordsman", Cost: 9, Wargear: []string{"Badge of Courage"}, Rules: []string{"Fearful"}}},
		{WandererOffer: core.WandererOffer{Name: "Kin of Another Folk", Cost: 9}, Reason: "The company cannot take Kin of Another Folk: evil", Blocker: core.BlockedByRule},
	}, nil)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/armory?company=1", nil))).Body.String()
	for _, want := range []string{
		`x-data="armory"`, `role="switch"`, `data-main-chip="wargear"`, `data-main-chip="wanderers"`, `data-sub-chip="mount"`,
		`data-main="equipment" data-sub="other" data-avail="1"`, `data-main="wargear" data-sub="mount" data-avail="0"`, `data-main="wargear" data-sub="melee" data-avail="1"`,
		`data-main="wanderers" data-avail="1"`, `data-main="wanderers" data-avail="0"`,
		`data-tip-title="Badge of Courage"`, `data-tip-text="Be brave."`, `{&#34;creatures&#34;:[],`, `&#34;wanderers&#34;:[]`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("armory lacks %q", want)
		}
	}
}

// A full page carries the sidebar's campaign list (no /nav/campaigns round trip); an htmx content swap does not, and a
// failing list falls back to the lazy fetch.
func TestFullPagesCarryTheSidebarList(t *testing.T) {
	f := newFixture(t, Config{})
	f.nav = []core.CampaignNav{{ID: 4, Name: "Rohan Rising"}}
	get := func() *httptest.ResponseRecorder {
		return f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/new", nil)))
	}
	body := get().Body.String()
	if !strings.Contains(body, `href="/campaigns/4"`) || strings.Contains(body, `hx-trigger="load, campaigns-changed`) {
		t.Errorf("sidebar not inline: %s", body)
	}
	req := f.signedIn(httptest.NewRequest("GET", "/campaigns/new", nil))
	req.Header.Set("HX-Request", "true")
	if body := f.do(req).Body.String(); strings.Contains(body, "Rohan Rising") {
		t.Errorf("htmx swap carries the sidebar: %s", body)
	}
	f.navErr = errors.New("db down")
	body = get().Body.String()
	if strings.Contains(body, "Rohan Rising") || !strings.Contains(body, `hx-trigger="load, campaigns-changed`) {
		t.Errorf("no lazy fallback: %s", body)
	}
}

func TestBuyingAndChoosingTheWeaponOfAWeaponModifier(t *testing.T) {
	t.Run("buy with a weapon", func(t *testing.T) {
		f := newFixture(t, Config{})
		d, detail := mineDetail()
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
		f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
		f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
		f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
		f.camp.EXPECT().BuyEquipmentFor(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Envenom Weapon (X)", "", "Bow").Return(nil)
		rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/equipment", url.Values{"item": {"Envenom Weapon (X)"}, "weapon": {"Bow"}})))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Bought Envenom Weapon (X).") {
			t.Errorf("got %d", rec.Code)
		}
	})
	t.Run("choose a legacy item's weapon", func(t *testing.T) {
		f := newFixture(t, Config{})
		d, detail := mineDetail()
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
		f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
		f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
		f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
		f.camp.EXPECT().ChooseModifierWeapon(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Envenom Weapon (X)", "Bow").Return(nil)
		rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/equipment/weapon", url.Values{"item": {"Envenom Weapon (X)"}, "weapon": {"Bow"}})))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "applies to the Bow") {
			t.Errorf("got %d", rec.Code)
		}
	})
	t.Run("the choice is final: a second choice and a purchase without a weapon are refused with the message", func(t *testing.T) {
		f := newFixture(t, Config{})
		d, detail := mineDetail()
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).Times(2)
		f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).Times(2)
		f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
		f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).Times(2)
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).Times(2)
		f.camp.EXPECT().ChooseModifierWeapon(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Envenom Weapon (Bow)", "Spear").
			Return(core.Refuse("Envenom Weapon (Bow) already has its weapon (Bow): the choice is final."))
		rec := f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/equipment/weapon", url.Values{"item": {"Envenom Weapon (Bow)"}, "weapon": {"Spear"}})))
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "the choice is final") {
			t.Errorf("second choice: %d", rec.Code)
		}
		f.camp.EXPECT().BuyEquipment(gomock.Any(), frodo, int64(7), int64(1), int64(5), "Envenom Weapon (X)", gomock.Any()).
			Return(core.Refuse("Envenom Weapon (X) needs the weapon it is for: choose one of Bow, Hand weapon."))
		rec = f.do(f.signedIn(form("/campaigns/7/my-company/1/units/5/equipment", url.Values{"item": {"Envenom Weapon (X)"}})))
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "choose one of Bow, Hand weapon") {
			t.Errorf("no weapon: %d", rec.Code)
		}
	})
	t.Run("a chosen weapon has no chooser and no Change weapon, whether or not the weapon is carried", func(t *testing.T) {
		f := newFixture(t, Config{})
		d, detail := mineDetail()
		detail.Roster[0].Equipment = []core.Equipment{{Name: "Envenom Weapon (Bow)", Class: core.EquipOther, Size: "small", Bought: true, Modifies: "Bow"}}
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
		f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
		f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
		f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
		body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
		if strings.Contains(body, `/units/5/equipment/weapon`) || strings.Contains(body, "Change weapon") || strings.Contains(body, "Choose weapon") {
			t.Error("the choice is final: nothing to choose or change")
		}
		if !strings.Contains(body, "Bow · envenomed") {
			t.Error("the item shows its weapon")
		}
	})
	t.Run("the card offers the chooser", func(t *testing.T) {
		f := newFixture(t, Config{})
		d, detail := mineDetail()
		detail.Roster[0].Equipment = []core.Equipment{{Name: "Envenom Weapon (X)", Class: core.EquipOther, Size: "small", Bought: true, Choices: []string{"Bow", "Hand weapon"}}}
		f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
		f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil)
		f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
		f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
		f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil)
		f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil)
		body := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1", nil))).Body.String()
		if !strings.Contains(body, `/units/5/equipment/weapon`) || !strings.Contains(body, `<option value="Hand weapon"`) || !strings.Contains(body, "Choose weapon") {
			t.Error("the card lets the player choose the weapon")
		}
	})
}

// Several uploaded files are stored all or nothing: a campaign form with a good glossary and a bad Battle Companies file stores
// neither and creates no campaign; the answer is the 422 form again with the reason.
func TestCreateCampaignStoresUploadsAllOrNothing(t *testing.T) {
	f := newFixture(t, Config{})
	maker, do := campaignMaker(f)
	f.defaultsShown()
	const bc, glossary = `{"name":"BC"}`, `{"name":"G","version":"1","terms":[]}`
	uploads := []core.RulesetUpload{
		{Kind: core.KindBattleCompanies, Hash: "h-bc", Content: []byte(bc)},
		{Kind: core.KindGlossary, Hash: "h-gl", Content: []byte(glossary)},
	}
	form := map[string]string{"name": "Own rules", "battle_companies-mode": "file", "battle_companies-hash": "h-bc", "glossary-mode": "file", "glossary-hash": "h-gl"}
	files := map[string]string{"battle_companies-file": bc, "glossary-file": glossary}

	f.rs.EXPECT().StoreAllAs(gomock.Any(), maker, uploads).Return(nil, errors.Join(core.ErrInvalidRuleset, errors.New("limits: wrong type")))
	rec := do(multipartForm(t, form, files)) // no Create expectation: the strict mock fails if the campaign is created
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Limits: wrong type") {
		t.Fatalf("a bad file: got %d: %s", rec.Code, rec.Body)
	}

	f.rs.EXPECT().StoreAllAs(gomock.Any(), maker, uploads).Return([]core.RulesetInfo{{Kind: core.KindBattleCompanies, Hash: "h-bc"}, {Kind: core.KindGlossary, Hash: "h-gl"}}, nil)
	f.camp.EXPECT().Create(gomock.Any(), maker, core.CampaignInput{Name: "Own rules", BattleCompanies: "h-bc", Glossary: "h-gl"}).Return(core.CampaignSummary{ID: 2}, nil)
	if rec := do(multipartForm(t, form, files)); rec.Code != http.StatusSeeOther {
		t.Fatalf("good files: got %d: %s", rec.Code, rec.Body)
	}
}

// The recruiting parameters of the shipped file, for the fakes of the windows (the real ones come from the rules file).
var (
	upToThree    = core.AdjustRules{Max: 3, CostPerPoint: 1, Direction: core.AdjustUp, Die: true}
	recruitRules = core.RecruitRules{Adjust: upToThree, Sides: 6, Rendezvous: core.PerkRange{Low: -1, High: 1}}
)

// The window prints the adjustment from the rules it is given: the price, the points, the directions and the buttons.
func TestTheRecruitWindowPrintsTheAdjustmentFromTheRules(t *testing.T) {
	for _, tc := range []struct {
		name         string
		adjust       core.AdjustRules
		budget, down int
		want, absent []string
	}{
		{"shipped: raise only", upToThree, 3, 0, []string{"Raise the roll", "1 influence per point, up to 3 more", ">+3<", "As rolled"}, []string{">-1<", "Adjust the roll"}},
		{"cap 5, 2 per point, both ways", core.AdjustRules{Max: 5, CostPerPoint: 2, Direction: core.AdjustBoth, Die: true}, 2, 3,
			[]string{"Adjust the roll", "2 influence per point, up to 2 up or 3 down", ">+5<", ">-5<", ">-1<", "As rolled"}, []string{"Raise the roll"}},
		{"lower only", core.AdjustRules{Max: 2, CostPerPoint: 1, Direction: core.AdjustDown, Die: true}, 0, 2,
			[]string{"Lower the roll", ">-2<", "up to 2 down"}, []string{">+1<"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			d, detail := mineDetail()
			detail.Influence = 6
			detail.Recruit.Rules = core.RecruitRules{Adjust: tc.adjust, Sides: 6, Rendezvous: core.PerkRange{Low: -1, High: 1}}
			f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(d, nil).AnyTimes()
			f.camp.EXPECT().CompanyDetail(gomock.Any(), int64(7), int64(1)).Return(detail, nil).AnyTimes()
			f.camp.EXPECT().PendingChoice(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
			f.camp.EXPECT().PendingOfficers(gomock.Any(), frodo, int64(7), int64(1)).Return(nil, nil).AnyTimes()
			f.gl.EXPECT().ForCampaign(gomock.Any(), int64(7)).Return(core.Glossary{}, nil).AnyTimes()
			f.camp.EXPECT().RecruitState(gomock.Any(), frodo, int64(7), int64(1)).Return(&core.RecruitRoll{Stage: "chart", Rolled: true, Roll: 3, Budget: tc.budget, BudgetDown: tc.down, Adjust: tc.adjust, Sides: 6}, nil)
			rec := f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/my-company/1/recruit", nil)))
			body := rec.Body.String()
			for _, w := range tc.want {
				if !strings.Contains(body, w) {
					t.Errorf("the window lacks %q", w)
				}
			}
			for _, w := range tc.absent {
				if strings.Contains(body, w) {
					t.Errorf("the window has %q", w)
				}
			}
		})
	}
}
