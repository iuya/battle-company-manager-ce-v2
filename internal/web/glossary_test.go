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

var goodHash = strings.Repeat("ab", 32)

func glossaryFixture(t *testing.T) *fixture {
	f := newFixture(t, Config{})
	f.rs.EXPECT().Default(gomock.Any(), core.KindGlossary).Return(core.RulesetInfo{Kind: core.KindGlossary, Name: "MESBG Glossary", Version: "1.0", Hash: "abcdef0123456789"}, nil).AnyTimes()
	f.gl.EXPECT().Default(gomock.Any()).Return(sampleGlossary(), nil).AnyTimes()
	return f
}

func glossaryWithCookie(f *fixture, value string) *httptest.ResponseRecorder {
	r := httpGet("/glossary")
	if value != "" {
		r.AddCookie(&http.Cookie{Name: glossaryCookie, Value: value})
	}
	return f.do(f.signedIn(r))
}

func TestGlossaryPickerAndCookie(t *testing.T) {
	f := glossaryFixture(t)
	// no cookie: the default, with the picker
	rec := glossaryWithCookie(f, "")
	if body := rec.Body.String(); !strings.Contains(body, "Use another glossary") || !strings.Contains(body, `x-data="rulesetPicker"`) || strings.Contains(body, "Back to the default") {
		t.Error("the default glossary offers the picker only")
	}
	// a known hash: that file, with the way back
	custom := sampleGlossary()
	custom.Info.Name = "My glossary"
	f.gl.EXPECT().ByHash(gomock.Any(), goodHash).Return(custom, nil)
	if body := glossaryWithCookie(f, goodHash).Body.String(); !strings.Contains(body, "My glossary") || !strings.Contains(body, "Back to the default") || !strings.Contains(body, "Not the app") {
		t.Error("a picked glossary says so and offers the way back")
	}
	// an unknown hash falls back to the default with a notice and drops the cookie
	f.gl.EXPECT().ByHash(gomock.Any(), goodHash).Return(core.Glossary{}, core.ErrNotFound)
	rec = glossaryWithCookie(f, goodHash)
	if !strings.Contains(rec.Body.String(), "not available any more") || !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), ";"), "bcm-glossary=;") {
		t.Errorf("unknown hash: %s", rec.Header())
	}
	// a malformed one never reaches the store
	rec = glossaryWithCookie(f, "../etc")
	if !strings.Contains(rec.Body.String(), "not available any more") {
		t.Error("malformed hash")
	}
}

func TestChooseGlossary(t *testing.T) {
	f := glossaryFixture(t)
	post := func(v url.Values) *httptest.ResponseRecorder {
		r := form("/glossary", v)
		return f.do(f.signedIn(r))
	}
	f.rs.EXPECT().Lookup(gomock.Any(), goodHash).Return(core.RulesetInfo{Kind: core.KindGlossary, Hash: goodHash}, nil)
	rec := post(url.Values{"glossary-mode": {"file"}, "glossary-hash": {goodHash}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/glossary" || !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), ";"), "bcm-glossary="+goodHash) {
		t.Errorf("a known file: %d %v", rec.Code, rec.Header())
	}
	rec = post(url.Values{"glossary-mode": {"default"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), ";"), "bcm-glossary=;") {
		t.Errorf("back to the default clears the cookie: %v", rec.Header())
	}
	f.rs.EXPECT().Lookup(gomock.Any(), goodHash).Return(core.RulesetInfo{}, core.ErrNotFound)
	rec = post(url.Values{"glossary-mode": {"file"}, "glossary-hash": {goodHash}})
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Choose a file") {
		t.Errorf("an unknown file: %d", rec.Code)
	}
}
