package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type vendored struct {
	File, Name, Version, License, Source, SHA256 string
}

func loadVendor(t *testing.T) []vendored {
	t.Helper()
	raw, err := os.ReadFile("vendor.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Files []vendored }
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m.Files
}

// The committed third-party scripts are the ones the manifest names: an edit without a manifest update (or the reverse) fails here.
// With VENDOR_ONLINE=1 (make vendor-check ONLINE=1) every fetchable source is downloaded and compared too.
func TestVendoredJSMatchesManifest(t *testing.T) {
	listed := map[string]bool{}
	for _, v := range loadVendor(t) {
		listed[v.File] = true
		if v.Name == "" || v.Version == "" || v.License == "" || v.Source == "" || len(v.SHA256) != 64 {
			t.Errorf("%s: incomplete manifest entry %+v", v.File, v)
		}
		got, err := os.ReadFile("static/js/" + strings.TrimPrefix(v.File, "static/js/"))
		if err != nil {
			t.Errorf("%s: %v", v.File, err)
			continue
		}
		sum := sha256.Sum256(got)
		if hex.EncodeToString(sum[:]) != v.SHA256 {
			t.Errorf("%s: sha256 is %x, the manifest says %s (update vendor.json and docs/vendored-js.md)", v.File, sum, v.SHA256)
		}
		if os.Getenv("VENDOR_ONLINE") == "" || !strings.HasPrefix(v.Source, "https://") {
			continue
		}
		c := http.Client{Timeout: 30 * time.Second}
		resp, err := c.Get(v.Source)
		if err != nil {
			t.Errorf("%s: fetching %s: %v", v.File, v.Source, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		up := sha256.Sum256(body)
		if resp.StatusCode != 200 || hex.EncodeToString(up[:]) != v.SHA256 {
			t.Errorf("%s: %s answers %d with sha256 %x, the committed file has %s", v.File, v.Source, resp.StatusCode, up, v.SHA256)
		}
	}
	// every script under static/js that is not our own is listed: the minified ones and the bundle
	entries, err := os.ReadDir("static/js")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if (strings.HasSuffix(e.Name(), ".min.js") || e.Name() == "dice3d.js") && !listed["static/js/"+e.Name()] {
			t.Errorf("%s is vendored but not in vendor.json", e.Name())
		}
	}
}
