package web

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// issuesOf makes n error issues spread over the vanilla and the Battle Companies file.
func issuesOf(n int) []core.RulesetIssue {
	var out []core.RulesetIssue
	for i := 0; i < n; i++ {
		file := "vanilla.json"
		if i%2 == 1 {
			file = "battle-companies.json"
		}
		out = append(out, core.RulesetIssue{File: file, Path: fmt.Sprintf("units[%d].name", i), Message: fmt.Sprintf("unknown unit %d", i), Hint: "Use a name from the vanilla file.", Closest: "Warrior of Minas Tirith"})
	}
	return out
}

func checkUpload(f *fixture, kind, content string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("kind", kind)
	if content != "" {
		fw, _ := mw.CreateFormFile("file", "x.json")
		_, _ = fw.Write([]byte(content))
	}
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/admin/rulesets/check", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return f.do(htmxReq(f.asSuper(r), "rs-check-result"))
}

func TestRulesetIssuesViewGroupsByFileAndCaps(t *testing.T) {
	if rulesetIssues(errors.Join(core.ErrInvalidRuleset, errors.New("x"))) != nil || rulesetIssues(nil) != nil {
		t.Error("a plain error has no report")
	}
	v := rulesetIssues(errors.Join(errors.New("wrapped"), core.NewRulesetReport(issuesOf(250), 7)))
	if v == nil || v.Errors != 250 || v.More != 57 {
		t.Fatalf("view: %+v", v)
	}
	shown := 0
	for _, g := range v.Files {
		shown += len(g.Issues)
	}
	if len(v.Files) != 2 || shown != 200 {
		t.Errorf("files %d shown %d", len(v.Files), shown)
	}
}

func TestAdminCheckThisFile(t *testing.T) {
	f := newFixture(t, Config{})
	// no problem: a warning is shown, nothing is stored (the mock has no StoreAs expectation)
	warn := core.NewRulesetReport([]core.RulesetIssue{{File: "glossary.json", Path: "terms[2]", Message: "term is never used", Warn: true}}, 0)
	f.rs.EXPECT().Check(gomock.Any(), core.KindGlossary, []byte("{}")).Return(*warn, nil)
	rec := checkUpload(f, "glossary", "{}")
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "No problems found, 1 warning") || !strings.Contains(body, "term is never used") || !strings.Contains(body, "alert-success") {
		t.Errorf("ok: %d %s", rec.Code, body)
	}

	// every issue is listed with file, path, hint and closest match; the 200 cap counts the rest
	rep := core.NewRulesetReport(issuesOf(230), 5)
	f.rs.EXPECT().Check(gomock.Any(), core.KindBattleCompanies, []byte("{}")).Return(*rep, rep)
	rec = checkUpload(f, "battle_companies", "{}")
	body = rec.Body.String()
	for _, want := range []string{"230 problems found", "battle-companies.json", "units[198].name", "Use a name from the vanilla file.", "Closest match", "Warrior of Minas Tirith", "and 35 more not listed", "Nothing was stored."} {
		if !strings.Contains(body, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if rec.Code != http.StatusUnprocessableEntity || strings.Contains(body, "units[200].name") || strings.Count(body, `class="badge badge-danger"`) != 200 {
		t.Errorf("422 and exactly 200 rows expected: %d", rec.Code)
	}

	// no file, no kind
	if rec := checkUpload(f, "glossary", ""); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose a .json file") {
		t.Errorf("no file: %d %s", rec.Code, rec.Body)
	}
	if rec := checkUpload(f, "", "{}"); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Choose the kind") {
		t.Errorf("no kind: %d %s", rec.Code, rec.Body)
	}
	// a plain rejection without a report keeps its reason
	f.rs.EXPECT().Check(gomock.Any(), core.KindGlossary, []byte("{")).Return(core.RulesetReport{}, errors.Join(core.ErrInvalidRuleset, errors.New("not valid JSON")))
	if rec := checkUpload(f, "glossary", "{"); rec.Code != 422 || !strings.Contains(rec.Body.String(), "Not valid JSON") {
		t.Errorf("plain: %d %s", rec.Code, rec.Body)
	}
}

func TestAdminUploadShowsTheWholeReport(t *testing.T) {
	f := newFixture(t, Config{})
	f.rs.EXPECT().ListWithUsage(gomock.Any(), superG).Return(usage(), nil).AnyTimes()
	const bc = `{"name":"BC2","version":"2","extends":"V"}`
	hash, _ := core.RulesetHash([]byte(bc))
	f.rs.EXPECT().Find(gomock.Any(), core.KindBattleCompanies, hash, "BC2", "2").Return(core.RulesetInfo{}, core.ErrNotFound)
	f.rs.EXPECT().StoreAs(gomock.Any(), superG, core.KindBattleCompanies, hash, gomock.Any()).Return(core.RulesetInfo{}, core.NewRulesetReport(issuesOf(3), 0))
	rec := upload(f, "battle_companies", bc)
	body := rec.Body.String()
	if rec.Code != 422 || !strings.Contains(body, "3 problems found") || !strings.Contains(body, "units[2].name") || !strings.Contains(body, "Check this file") {
		t.Errorf("%d %s", rec.Code, body)
	}
}

func TestCreateCampaignShowsEveryProblemAndCreatesNothing(t *testing.T) {
	f := newFixture(t, Config{})
	_, do := campaignMaker(f)
	f.defaultsShown()
	// both files are checked before anything is stored; the two reports come back merged (no StoreAs, StoreAllAs or Create call)
	f.rs.EXPECT().Lookup(gomock.Any(), "h-bc").Return(core.RulesetInfo{}, core.ErrNotFound).AnyTimes()
	bad := core.NewRulesetReport(issuesOf(2), 0)
	f.rs.EXPECT().StoreAllAs(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, bad)
	rec := do(multipartForm(t, map[string]string{
		"name": "Own rules", "vanilla-mode": "file", "vanilla-hash": "h-van", "battle_companies-mode": "file", "battle_companies-hash": "h-bc2",
	}, map[string]string{"vanilla-file": "{}", "battle_companies-file": "{}"}))
	body := rec.Body.String()
	if rec.Code != 422 || !strings.Contains(body, "2 problems found") || !strings.Contains(body, "no campaign was created") || !strings.Contains(body, `value="Own rules"`) {
		t.Errorf("%d %s", rec.Code, body)
	}
}
