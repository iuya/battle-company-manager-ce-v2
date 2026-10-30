package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

func getStatic(f *fixture, target string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", target, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return f.do(r)
}

func TestStaticVersionedURLIsImmutableAndUnversionedRevalidates(t *testing.T) {
	f := newFixture(t, Config{})
	rec := getStatic(f, staticURL("js/htmx.min.js"), nil)
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("versioned: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
		t.Errorf("content type %q", rec.Header().Get("Content-Type"))
	}
	// a stale version is served, but not for a year
	if rec := getStatic(f, "/static/js/htmx.min.js?v=0000000000", nil); rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("stale version: %q", rec.Header().Get("Cache-Control"))
	}
	un := getStatic(f, "/static/js/htmx.min.js", nil)
	etag := un.Header().Get("ETag")
	if un.Header().Get("Cache-Control") != "no-cache" || etag == "" {
		t.Fatalf("unversioned: %q etag %q", un.Header().Get("Cache-Control"), etag)
	}
	if rec := getStatic(f, "/static/js/htmx.min.js", map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: %d", rec.Code)
	}
	if rec := getStatic(f, "/static/nothing.js", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing: %d", rec.Code)
	}
	f2 := newFixture(t, Config{StaticMaxAge: 3600e9})
	if rec := getStatic(f2, "/static/js/htmx.min.js", nil); rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Errorf("STATIC_MAX_AGE: %q", rec.Header().Get("Cache-Control"))
	}
}

func TestStaticGzipForTextFilesOnly(t *testing.T) {
	f := newFixture(t, Config{})
	plain := getStatic(f, "/static/app.css", nil)
	if plain.Header().Get("Content-Encoding") != "" || plain.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("no Accept-Encoding: %q vary %q", plain.Header().Get("Content-Encoding"), plain.Header().Get("Vary"))
	}
	gz := getStatic(f, "/static/app.css", map[string]string{"Accept-Encoding": "br, gzip;q=0.8"})
	if gz.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip: %q", gz.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if !bytes.Equal(got, plain.Body.Bytes()) || gz.Body.Len() >= plain.Body.Len() {
		t.Errorf("gzip body differs or is not smaller: %d vs %d", gz.Body.Len(), plain.Body.Len())
	}
	if gz.Header().Get("ETag") == plain.Header().Get("ETag") {
		t.Error("the gzip and plain bodies share an ETag")
	}
	if rec := getStatic(f, "/static/app.css", map[string]string{"Accept-Encoding": "gzip;q=0"}); rec.Header().Get("Content-Encoding") != "" {
		t.Error("gzip;q=0 refuses gzip")
	}
	if rec := getStatic(f, "/static/fonts/cormorant-garamond-latin-700.woff2", map[string]string{"Accept-Encoding": "gzip"}); rec.Header().Get("Content-Encoding") != "" || rec.Header().Get("Content-Type") != "font/woff2" {
		t.Errorf("font: %q %q", rec.Header().Get("Content-Encoding"), rec.Header().Get("Content-Type"))
	}
}

func TestStylesheetFilesCarryTheirVersion(t *testing.T) {
	f := newFixture(t, Config{})
	css := getStatic(f, "/static/app.css", nil).Body.String()
	want := "url(\"" + staticURL("fonts/cormorant-garamond-latin-700.woff2") + "\")"
	if !strings.Contains(css, want) || !strings.Contains(css, "url(\""+staticURL("img/ring-text.svg")+"\")") {
		t.Errorf("app.css does not load its fonts and images by versioned address (want %s)", want)
	}
}

func TestPagesLinkStaticFilesByVersionedAddress(t *testing.T) {
	f := newFixture(t, Config{})
	body := f.do(httptest.NewRequest("GET", "/login", nil)).Body.String()
	for _, p := range []string{"app.css", "js/app.js", "js/theme-init.js", "js/tz.js", "favicon.svg", "icon.svg"} {
		if !strings.Contains(body, staticURL(p)) || !strings.Contains(staticURL(p), "?v=") {
			t.Errorf("the login page does not use %s", staticURL(p))
		}
	}
	if strings.Contains(body, `"/static/app.css"`) {
		t.Error("an unversioned stylesheet address")
	}
}

// The heading font is preloaded under the address the stylesheet loads it from (otherwise the browser fetches it twice), so it is on
// the page before the first paint and the headings do not change height when it arrives.
func TestLayoutPreloadsTheHeadingFontUnderTheStylesheetsAddress(t *testing.T) {
	f := newFixture(t, Config{})
	body := f.do(httptest.NewRequest("GET", "/login", nil)).Body.String()
	font := staticURL("fonts/cormorant-garamond-latin-700.woff2")
	if want := `<link rel="preload" href="` + font + `" as="font" type="font/woff2" crossorigin>`; !strings.Contains(body, want) {
		t.Errorf("no font preload %s", want)
	}
	if !strings.Contains(getStatic(f, "/static/app.css", nil).Body.String(), `url("`+font+`")`) {
		t.Error("the stylesheet loads the heading font from another address than the preload")
	}
}

func TestScriptBundleHoldsEveryPageScriptInOrder(t *testing.T) {
	files := staticFiles()
	b := string(files[bundleName].data)
	at := -1
	for _, p := range bundleScripts {
		src := string(files[p].data)
		i := strings.Index(b, src)
		if i < 0 {
			t.Fatalf("the bundle lacks %s", p)
		}
		if i < at {
			t.Errorf("%s is out of order", p)
		}
		at = i
	}
	if !strings.Contains(b, "Sortable 1.15.6") || !strings.HasPrefix(string(files[bundleScripts[len(bundleScripts)-1]].data), "(()=>{") {
		t.Error("the vendored libraries are part of the bundle")
	}
	// The layout names the bundle and none of the files in it.
	f := newFixture(t, Config{})
	body := f.do(httptest.NewRequest("GET", "/login", nil)).Body.String()
	for _, p := range bundleScripts {
		if strings.Contains(body, `src="/static/`+p) {
			t.Errorf("the page still loads %s on its own", p)
		}
	}
	if strings.Count(body, "<script src=") != 3 {
		t.Errorf("want theme-init, tz and the bundle, found %d scripts", strings.Count(body, "<script src="))
	}
}

func TestHomeDrawsTheSidebarCampaignsInPlace(t *testing.T) {
	f := newFixture(t, Config{})
	f.camp.EXPECT().List(gomock.Any(), frodo.ID, false).Return([]core.CampaignSummary{{ID: 4, Name: "Rohan Rising"}}, nil).Times(1)
	body := f.do(f.signedIn(httptest.NewRequest("GET", "/", nil))).Body.String()
	if !strings.Contains(body, "inCampaign(4)") || !strings.Contains(body, `hx-trigger="campaigns-changed from:body"`) || strings.Contains(body, `hx-trigger="load,`) {
		t.Error("the main page carries the sidebar's list and does not fetch it again")
	}
	f.nav = []core.CampaignNav{{ID: 4, Name: "Rohan Rising"}}
	f.camp.EXPECT().Get(gomock.Any(), frodo.ID, int64(7)).Return(campaignDetail(), nil)
	f.camp.EXPECT().Companies(gomock.Any(), int64(7)).Return(nil, nil).AnyTimes()
	body = f.do(f.signedIn(httptest.NewRequest("GET", "/campaigns/7/rankings", nil))).Body.String()
	if !strings.Contains(body, "inCampaign(4)") || !strings.Contains(body, `hx-trigger="campaigns-changed from:body"`) || strings.Contains(body, `hx-trigger="load,`) {
		t.Error("other full pages carry the sidebar's list too")
	}
}
