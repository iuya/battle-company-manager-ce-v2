package web

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// validatorPost posts the given files (field -> content; "" leaves the field out) to the validator as htmx does.
func validatorPost(f *fixture, files map[string]string, htmx bool) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for field, content := range files {
		if content == "" {
			continue
		}
		fw, _ := mw.CreateFormFile(field, field+".json")
		_, _ = fw.Write([]byte(content))
	}
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/tools/validate", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r = f.signedIn(r)
	if htmx {
		r = htmxReq(r, "validator-result")
	}
	return f.do(r)
}

func expectShippedDefaults(f *fixture) {
	f.rs.EXPECT().Default(gomock.Any(), core.KindVanilla).Return(core.RulesetInfo{Kind: core.KindVanilla, Name: "MESBG 2024", Version: "2", Hash: "aaaaaaaaaaaa"}, nil).AnyTimes()
	f.rs.EXPECT().Default(gomock.Any(), core.KindGlossary).Return(core.RulesetInfo{Kind: core.KindGlossary, Name: "MESBG Glossary", Version: "1", Hash: "bbbbbbbbbbbb"}, nil).AnyTimes()
}

func TestValidatorPageIsForAnySignedInPlayer(t *testing.T) {
	f := newFixture(t, Config{})
	expectShippedDefaults(f)
	rec := f.do(httptest.NewRequest("GET", "/tools/validate", nil))
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Errorf("signed out: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	rec = f.do(f.signedIn(httptest.NewRequest("GET", "/tools/validate", nil)))
	body := rec.Body.String()
	for _, want := range []string{"Check rules files", `name="battle_companies"`, `name="vanilla"`, `name="glossary"`, "shipped one (MESBG 2024 v2)", "shipped one (MESBG Glossary v1)", `id="validator-result"`, `aria-live="polite"`} {
		if rec.Code != 200 || !strings.Contains(body, want) {
			t.Errorf("lacks %q (%d)", want, rec.Code)
		}
	}
	if rec := f.do(httptest.NewRequest("POST", "/tools/validate", nil)); rec.Code != http.StatusSeeOther {
		t.Errorf("signed out POST: %d", rec.Code)
	}
}

// TestValidatorChecksTheThreeFilesTogetherAndStoresNothing: the mock has no Store, StoreAs or StoreAllAs expectation, so any write fails the test.
func TestValidatorChecksTheThreeFilesTogetherAndStoresNothing(t *testing.T) {
	f := newFixture(t, Config{})
	set := core.RulesetSet{Vanilla: []byte("V"), Glossary: []byte("G"), BattleCompanies: []byte("B")}
	f.rs.EXPECT().CheckSet(gomock.Any(), set).Return(*core.NewRulesetReport([]core.RulesetIssue{{File: "glossary.json", Message: "term is never used", Warn: true}}, 0), nil)
	rec := validatorPost(f, map[string]string{"vanilla": "V", "glossary": "G", "battle_companies": "B"}, true)
	body := rec.Body.String()
	for _, want := range []string{"No problems found, 1 warning", "Nothing was stored and no campaign was created.", "Vanilla rules data: your file", "Battle Companies rules data: your file", "alert-success"} {
		if rec.Code != 200 || !strings.Contains(body, want) {
			t.Errorf("lacks %q (%d): %s", want, rec.Code, body)
		}
	}
	if strings.Contains(body, "<html") {
		t.Error("an htmx check answers the fragment only")
	}

	// only the Battle Companies file: the other two are the shipped ones
	f.rs.EXPECT().CheckSet(gomock.Any(), core.RulesetSet{BattleCompanies: []byte("B")}).Return(core.RulesetReport{}, nil)
	rec = validatorPost(f, map[string]string{"battle_companies": "B"}, true)
	if body := rec.Body.String(); rec.Code != 200 || !strings.Contains(body, "Vanilla rules data: the shipped one") || !strings.Contains(body, "Glossary: the shipped one") {
		t.Errorf("defaults: %d %s", rec.Code, body)
	}
}

func TestValidatorShowsEveryProblemAs422(t *testing.T) {
	f := newFixture(t, Config{})
	rep := core.NewRulesetReport(issuesOf(230), 5)
	f.rs.EXPECT().CheckSet(gomock.Any(), core.RulesetSet{BattleCompanies: []byte("B")}).Return(*rep, rep)
	rec := validatorPost(f, map[string]string{"battle_companies": "B"}, true)
	body := rec.Body.String()
	for _, want := range []string{"230 problems found", "units[198].name", "Closest match", "and 35 more not listed", "Nothing was stored and no campaign was created."} {
		if !strings.Contains(body, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if rec.Code != http.StatusUnprocessableEntity || strings.Count(body, `class="badge badge-danger"`) != 200 {
		t.Errorf("422 and 200 rows expected: %d", rec.Code)
	}
}

func TestValidatorRefusesWithoutABattleCompaniesFile(t *testing.T) {
	f := newFixture(t, Config{})
	rec := validatorPost(f, map[string]string{"vanilla": "V"}, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose your Battle Companies file") {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestValidatorRefusesAFileOver16MB(t *testing.T) {
	f := newFixture(t, Config{})
	rec := validatorPost(f, map[string]string{"battle_companies": strings.Repeat(" ", maxUpload+1)}, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "at most 16 MB") {
		t.Errorf("%d %.200s", rec.Code, rec.Body)
	}
}

func TestValidatorPlainRejectionKeepsItsReason(t *testing.T) {
	f := newFixture(t, Config{})
	f.rs.EXPECT().CheckSet(gomock.Any(), gomock.Any()).Return(core.RulesetReport{}, errors.Join(core.ErrInvalidRuleset, errors.New("not valid JSON")))
	rec := validatorPost(f, map[string]string{"battle_companies": "{"}, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "Not valid JSON") {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestValidatorWithoutScriptAnswersTheWholePage(t *testing.T) {
	f := newFixture(t, Config{})
	expectShippedDefaults(f)
	f.rs.EXPECT().CheckSet(gomock.Any(), gomock.Any()).Return(core.RulesetReport{}, nil)
	rec := validatorPost(f, map[string]string{"battle_companies": "B"}, false)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "<html") || !strings.Contains(body, "No problems found") || !strings.Contains(body, "Check files") {
		t.Errorf("%d %.300s", rec.Code, body)
	}
}

func TestValidatorLimitsPerAccountAndGlobally(t *testing.T) {
	f := newFixture(t, Config{})
	now := time.Now()
	f.srv.validator.now = func() time.Time { return now }
	f.rs.EXPECT().CheckSet(gomock.Any(), gomock.Any()).Return(core.RulesetReport{}, nil).Times(validatorPerMinute)
	for i := 0; i < validatorPerMinute; i++ {
		if rec := validatorPost(f, map[string]string{"battle_companies": "B"}, true); rec.Code != 200 {
			t.Fatalf("check %d: %d", i, rec.Code)
		}
	}
	rec := validatorPost(f, map[string]string{"battle_companies": "B"}, true)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "times in the last minute") || rec.Header().Get("Retry-After") == "" {
		t.Errorf("per account: %d %q", rec.Code, rec.Body)
	}
	now = now.Add(61 * time.Second) // the window moves on
	f.rs.EXPECT().CheckSet(gomock.Any(), gomock.Any()).Return(core.RulesetReport{}, nil)
	if rec := validatorPost(f, map[string]string{"battle_companies": "B"}, true); rec.Code != 200 {
		t.Errorf("after a minute: %d", rec.Code)
	}

	// two checks running: a third waits for nobody, it is turned away
	now = now.Add(61 * time.Second)
	for i := 0; i < validatorSlots; i++ {
		if !f.srv.validator.acquire() {
			t.Fatal("slot")
		}
	}
	rec = validatorPost(f, map[string]string{"battle_companies": "B"}, true)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "busy") {
		t.Errorf("global cap: %d %q", rec.Code, rec.Body)
	}
	f.srv.validator.release()
	f.rs.EXPECT().CheckSet(gomock.Any(), gomock.Any()).Return(core.RulesetReport{}, nil)
	if rec := validatorPost(f, map[string]string{"battle_companies": "B"}, true); rec.Code != 200 {
		t.Errorf("a slot is free again: %d", rec.Code)
	}
	if len(f.srv.validator.slots) != validatorSlots-1 {
		t.Errorf("the slot of a finished check is given back: %d", len(f.srv.validator.slots))
	}
}
