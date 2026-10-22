package web

import (
	"context"
	"errors"
	"io/fs"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestConfigResolve(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		wantSecure bool
		wantErr    bool
	}{
		{"production defaults to secure", Config{AppEnv: "production", CSPMode: "report"}, true, false},
		{"development defaults to plain", Config{AppEnv: "development", CSPMode: "report"}, false, false},
		{"production explicit off is refused", Config{AppEnv: "production", SecureCookiesEnv: "false", CSPMode: "off"}, false, true},
		{"production explicit off with opt-out", Config{AppEnv: "production", SecureCookiesEnv: "false", AllowInsecureCookies: true, CSPMode: "off"}, false, false},
		{"production explicit on", Config{AppEnv: "production", SecureCookiesEnv: "true", CSPMode: "report"}, true, false},
		{"development explicit off needs no opt-out", Config{AppEnv: "development", SecureCookiesEnv: "false", CSPMode: "report"}, false, false},
		{"development explicit on", Config{AppEnv: "development", SecureCookiesEnv: "true", CSPMode: "enforce"}, true, false},
		{"unknown env", Config{AppEnv: "staging", CSPMode: "report"}, false, true},
		{"unknown cookies", Config{AppEnv: "production", SecureCookiesEnv: "maybe", CSPMode: "report"}, false, true},
		{"unknown csp", Config{AppEnv: "production", CSPMode: "loud"}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Resolve()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Resolve() = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.cfg.SecureCookies != tt.wantSecure {
				t.Errorf("SecureCookies = %v, want %v", tt.cfg.SecureCookies, tt.wantSecure)
			}
		})
	}
}

func TestConfigWarnings(t *testing.T) {
	if w := (Config{AppEnv: "development"}).Warnings(); len(w) != 0 {
		t.Errorf("development warns: %v", w)
	}
	w := Config{AppEnv: "production", CSPMode: "off"}.Warnings()
	if len(w) != 3 {
		t.Errorf("insecure production should warn 3 times (cookies, proxy, csp), got %v", w)
	}
	if w := (Config{AppEnv: "production", SecureCookies: true, TrustProxy: true, CSPMode: "report"}).Warnings(); len(w) != 0 {
		t.Errorf("hardened production warns: %v", w)
	}
}

func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		ready  func(context.Context) error
		status int
		body   string
	}{
		{"healthz needs no database", "/healthz", func(context.Context) error { return errors.New("down") }, 200, "ok"},
		{"readyz without a check", "/readyz", nil, 200, "ok"},
		{"readyz with a healthy database", "/readyz", func(context.Context) error { return nil }, 200, "ok"},
		{"readyz hides the reason", "/readyz", func(context.Context) error { return errors.New("password authentication failed") }, 503, "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			f.srv.SetReadiness(tt.ready)
			rec := f.do(httptest.NewRequest("GET", tt.path, nil)) // no session, no mock expectations: it must not touch the auth service
			if rec.Code != tt.status || !strings.HasPrefix(rec.Body.String(), tt.body) || strings.Contains(rec.Body.String(), "password") {
				t.Errorf("%s = %d %q, want %d %q", tt.path, rec.Code, rec.Body.String(), tt.status, tt.body)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("health answers must not be cached")
			}
		})
	}
}

func TestSecurityHeadersByMode(t *testing.T) {
	tests := []struct {
		name           string
		cfg            Config
		wantHSTS       bool
		csp, cspReport bool
	}{
		{"zero config sends only the basics", Config{}, false, false, false},
		{"report only", Config{CSPMode: "report"}, false, false, true},
		{"enforce with secure cookies", Config{CSPMode: "enforce", SecureCookies: true}, true, true, false},
		{"off", Config{CSPMode: "off", SecureCookies: true}, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, tt.cfg)
			h := f.do(httptest.NewRequest("GET", "/healthz", nil)).Header()
			if h.Get("X-Frame-Options") != "DENY" || h.Get("X-Content-Type-Options") != "nosniff" {
				t.Error("basic headers missing")
			}
			if got := h.Get("Strict-Transport-Security") != ""; got != tt.wantHSTS {
				t.Errorf("HSTS present = %v, want %v", got, tt.wantHSTS)
			}
			if got := h.Get("Content-Security-Policy") != ""; got != tt.csp {
				t.Errorf("CSP present = %v, want %v", got, tt.csp)
			}
			if got := h.Get("Content-Security-Policy-Report-Only") != ""; got != tt.cspReport {
				t.Errorf("CSP report-only present = %v, want %v", got, tt.cspReport)
			}
			for _, v := range []string{h.Get("Content-Security-Policy"), h.Get("Content-Security-Policy-Report-Only")} {
				if v != "" && (!strings.Contains(v, "frame-ancestors 'none'") || !strings.Contains(v, "report-uri /csp-report") || strings.Contains(v, "script-src 'self' 'unsafe-inline'")) {
					t.Errorf("unexpected policy %q", v)
				}
			}
		})
	}
}

func TestCSPReport(t *testing.T) {
	post := func(f *fixture, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/csp-report", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/csp-report")
		r.Header.Set("Sec-Fetch-Site", "cross-site") // reports are not blocked by the CSRF check
		return f.do(r)
	}
	f := newFixture(t, Config{CSPMode: "report"})
	if rec := post(f, `{"csp-report":{"violated-directive":"script-src","blocked-uri":"https://evil.example/x.js"}}`); rec.Code != 204 {
		t.Errorf("valid report = %d, want 204", rec.Code)
	}
	if rec := post(f, strings.Repeat("x", 10_000)); rec.Code != 204 {
		t.Errorf("oversized report = %d, want 204 (read capped, nothing logged)", rec.Code)
	}
	if rec := post(f, "not json"); rec.Code != 204 {
		t.Errorf("garbage = %d, want 204", rec.Code)
	}
	if rec := f.do(httptest.NewRequest("GET", "/csp-report", nil)); rec.Code == 204 {
		t.Error("GET must not be accepted")
	}
}

func TestReportLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &reportLimiter{now: func() time.Time { return now }}
	for i := range maxCSPReportsMin {
		if ok, _ := l.allow(); !ok {
			t.Fatalf("report %d refused inside the limit", i)
		}
	}
	for range 5 {
		if ok, _ := l.allow(); ok {
			t.Fatal("report over the limit allowed")
		}
	}
	now = now.Add(61 * time.Second)
	ok, dropped := l.allow()
	if !ok || dropped != 5 {
		t.Errorf("new window: ok=%v dropped=%d, want true and 5", ok, dropped)
	}
}

func TestCutField(t *testing.T) {
	if got := cutField("a\nb\r\x00c"); got != "a b  c" {
		t.Errorf("control characters kept: %q", got)
	}
	if got := cutField(strings.Repeat("é", 500)); len([]rune(got)) != maxCSPField+3 {
		t.Errorf("long field not cut: %d runes", len([]rune(got)))
	}
	if cutField("<nil>") != "" {
		t.Error("missing field should be empty")
	}
}

// The enforced policy has no 'unsafe-inline' for scripts, so no template may carry an inline script or an event-handler attribute.
func TestTemplatesHaveNoInlineScript(t *testing.T) {
	inlineScript := regexp.MustCompile(`(?i)<script\b[^>]*>`)
	handler := regexp.MustCompile(`(?i)\son[a-z]+\s*=\s*["']`)
	err := fs.WalkDir(assets, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		for _, m := range inlineScript.FindAllString(string(raw), -1) {
			if !strings.Contains(m, "src=") {
				t.Errorf("%s: inline script %s", path, m)
			}
		}
		if m := handler.FindString(string(raw)); m != "" {
			t.Errorf("%s: event handler attribute %q", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
