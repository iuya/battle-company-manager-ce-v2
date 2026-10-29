package core

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// v2Files returns the shipped vanilla and glossary files and a minimal valid v2 file.
func v2Files(t *testing.T) (vanilla, bc, glossary []byte) {
	t.Helper()
	var err error
	if vanilla, err = os.ReadFile("../../definitions/vanilla/data2024.json"); err != nil {
		t.Fatal(err)
	}
	if glossary, err = os.ReadFile("../../definitions/glossary/mesbg_glossary.json"); err != nil {
		t.Fatal(err)
	}
	if bc, err = os.ReadFile("lean/testdata/valid/minimal.json"); err != nil {
		t.Fatal(err)
	}
	return
}

func TestLoadRuleSetDispatchesOnSchemaVersion(t *testing.T) {
	vanilla, bc2, glossary := v2Files(t)
	t.Run("the shipped file: a rule set that carries its document", func(t *testing.T) {
		rs, err := LoadRuleSet(vanilla, readDef(t, "battle_companies/battle_companies.json"), glossary, nil)
		if err != nil || rs.Doc == nil || rs.BC == nil || rs.Vanilla == nil {
			t.Fatalf("rs %v err %v", rs, err)
		}
	})
	t.Run("a file of the old format is refused with one clear issue", func(t *testing.T) {
		for _, old := range []string{`{"name":"x","version":"1","extends":"y","data":{}}`, `{"schemaVersion":1,"name":"x"}`} {
			rs, err := LoadRuleSet(vanilla, []byte(old), glossary, nil)
			var rep *RulesetReport
			if rs != nil || !errors.As(err, &rep) || !errors.Is(err, ErrInvalidRuleset) || len(rep.Issues()) != 1 || !strings.Contains(rep.Issues()[0].Message, "schemaVersion 1 is not supported") {
				t.Fatalf("%s: rs %v err %v", old, rs, err)
			}
		}
	})
	t.Run("invalid v2 file: the whole report", func(t *testing.T) {
		bad := strings.Replace(string(bc2), `"maxModels": 15`, `"maxModels": 99`, 1)
		bad = strings.Replace(bad, `"leaders": 1`, `"leaders": 9`, 1)
		rs, err := LoadRuleSet(vanilla, []byte(bad), glossary, nil)
		var rep *RulesetReport
		if rs != nil || !errors.As(err, &rep) || !errors.Is(err, ErrInvalidRuleset) {
			t.Fatalf("rs %v err %v", rs, err)
		}
		if rep.Errors() != 2 || len(rep.Issues()) != 2 {
			t.Fatalf("issues: %v", rep.Issues())
		}
		first := rep.Issues()[0]
		if first.File != "battle_companies.json" || first.Path != "campaign.company.maxModels" || !strings.Contains(first.Message, "99 is out of range 1..50") {
			t.Errorf("first issue %+v", first)
		}
	})
	t.Run("another version", func(t *testing.T) {
		_, err := LoadRuleSet(vanilla, []byte(`{"schemaVersion":3}`), glossary, nil)
		var rep *RulesetReport
		if !errors.As(err, &rep) || !strings.Contains(rep.Issues()[0].Message, "schemaVersion 3 is not supported") {
			t.Fatalf("err %v", err)
		}
	})
	t.Run("not an object is a one-issue report", func(t *testing.T) {
		_, err := LoadRuleSet(vanilla, []byte(`[1]`), glossary, nil)
		var rep *RulesetReport
		if !errors.As(err, &rep) || len(rep.Issues()) != 1 {
			t.Fatalf("err %v", err)
		}
	})
}

// TestRulesBuildDispatchesOnSchemaVersion: the cached loader behaves like LoadRuleSet for a stored v2 file.
func TestRulesBuildDispatchesOnSchemaVersion(t *testing.T) {
	vanilla, bc2, glossary := v2Files(t)
	files := map[string]StoredRuleset{
		"v": {RulesetInfo: RulesetInfo{Kind: KindVanilla, Hash: "v"}, Content: vanilla},
		"b": {RulesetInfo: RulesetInfo{Kind: KindBattleCompanies, Hash: "b"}, Content: bc2},
		"g": {RulesetInfo: RulesetInfo{Kind: KindGlossary, Hash: "g"}, Content: glossary},
	}
	rules := NewRules(fileRepo{files: files}, nil)
	set, err := rules.For(context.Background(), RulesetHashes{Vanilla: "v", BattleCompanies: "b", Glossary: "g"})
	if err != nil || set.Doc == nil || set.Hashes.BattleCompanies != "b" {
		t.Fatalf("set %v err %v", set, err)
	}
	files["b"] = StoredRuleset{RulesetInfo: files["b"].RulesetInfo, Content: []byte(`{"schemaVersion":2}`)}
	_, err = NewRules(fileRepo{files: files}, nil).For(context.Background(), RulesetHashes{Vanilla: "v", BattleCompanies: "b", Glossary: "g"})
	var rep *RulesetReport
	if !errors.As(err, &rep) {
		t.Fatalf("err %v", err)
	}
}

func TestRulesetReport(t *testing.T) {
	rep := newRulesetReport(leanReport())
	if rep.Errors() != 4 || len(rep.Issues()) != 5 || len(rep.Warnings()) != 1 || rep.Truncated() != 7 {
		t.Fatalf("%+v", rep)
	}
	if !errors.Is(rep, ErrInvalidRuleset) {
		t.Error("a report is an invalid ruleset error")
	}
	msg := rep.Error()
	if !strings.HasPrefix(msg, "the rules file has 4 problems (and 7 more not listed); f.json: a: m1; ") || !strings.HasSuffix(msg, "; ...") || strings.Contains(msg, "warn") {
		t.Errorf("summary: %s", msg)
	}
	if got := rep.Issues()[0].String(); got != "f.json: a: m1; closest: \"Spear\"\n  hint: h1" {
		t.Errorf("issue text %q", got)
	}
	if got := rep.Issues()[4].String(); !strings.HasPrefix(got, "warn: ") {
		t.Errorf("warning text %q", got)
	}
	one := newRulesetReport(leanReportN(1))
	if !strings.HasPrefix(one.Error(), "the rules file has 1 problem; ") {
		t.Errorf("singular: %s", one.Error())
	}
}
