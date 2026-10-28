package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const leanTestdata = "../../internal/core/lean/testdata/"

func runCheck(t *testing.T, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(append([]string{"-vanilla", "../../definitions/vanilla/data2024.json", "-glossary", "../../definitions/glossary/mesbg_glossary.json"}, args...), &o, &e)
	return code, o.String(), e.String()
}

func TestRuleCheck(t *testing.T) {
	t.Run("a valid file exits 0", func(t *testing.T) {
		code, out, _ := runCheck(t, leanTestdata+"valid/company.json")
		if code != 0 || out != "company.json: 0 errors, 0 warnings\n" {
			t.Errorf("code %d out %q", code, out)
		}
	})
	t.Run("errors exit 1 and every one is printed", func(t *testing.T) {
		code, out, _ := runCheck(t, leanTestdata+"invalid/many_mistakes.json")
		if code != 1 || !strings.HasSuffix(out, "many_mistakes.json: 12 errors, 0 warnings\n") || !strings.Contains(out, `closest: "Ranger of Arnor"`) || !strings.Contains(out, "  hint: ") {
			t.Errorf("code %d out %q", code, out)
		}
	})
	t.Run("warnings alone exit 0", func(t *testing.T) {
		code, out, _ := runCheck(t, leanTestdata+"invalid/vanilla_pins.json")
		if code != 0 || !strings.HasPrefix(out, "warn: ") || !strings.HasSuffix(out, "0 errors, 2 warnings\n") {
			t.Errorf("code %d out %q", code, out)
		}
	})
	t.Run("the old format is pointed at its validator", func(t *testing.T) {
		code, out, _ := runCheck(t, "../../definitions/battle_companies/battle_companies.json")
		if code != 1 || !strings.Contains(out, "tools/validate_definitions.py") {
			t.Errorf("code %d out %q", code, out)
		}
	})
	t.Run("a file that is not JSON", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "x.json")
		_ = os.WriteFile(p, []byte(`{"schemaVersion": 2,`), 0o644)
		code, out, _ := runCheck(t, p)
		if code != 1 || !strings.Contains(out, "not valid JSON at line 1") {
			t.Errorf("code %d out %q", code, out)
		}
	})
	t.Run("usage and unreadable files exit 2", func(t *testing.T) {
		if code, _, errOut := runCheck(t); code != 2 || !strings.Contains(errOut, "usage") {
			t.Errorf("no argument: %d %q", code, errOut)
		}
		if code, _, errOut := runCheck(t, "/nonexistent.json"); code != 2 || !strings.Contains(errOut, "no such file") {
			t.Errorf("missing file: %d %q", code, errOut)
		}
		var o, e bytes.Buffer
		if code := run([]string{"-vanilla", "/nonexistent", "x.json"}, &o, &e); code != 2 {
			t.Errorf("missing vanilla: %d", code)
		}
		if code := run([]string{"-nope"}, &o, &e); code != 2 {
			t.Errorf("bad flag: %d", code)
		}
	})
}
