package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

// threeCustomFiles are a vanilla file, a glossary and a Battle Companies file that only fit together: the Battle Companies file
// names a unit that exists only in the custom vanilla file and a rule that exists only in the custom glossary.
func threeCustomFiles(t *testing.T) (vanilla, glossary, bc []byte) {
	t.Helper()
	vanilla, _, glossary = v2Files(t)
	vanilla = []byte(strings.Replace(string(vanilla), `"name": "Warrior of Arnor"`, `"name": "Zzz Vanilla Unit"`, 1))
	var g map[string]any
	if err := json.Unmarshal(glossary, &g); err != nil {
		t.Fatal(err)
	}
	g["terms"] = append(g["terms"].([]any), map[string]any{"name": "Zzz Glossary Rule", "category": "special_rule", "definition": "Only in the uploaded glossary."})
	glossary, _ = json.Marshal(g)
	raw, err := os.ReadFile("../../examples/customizing-rulesets/custom-company.json")
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ReplaceAll(string(raw), "Warrior of Arnor", "Zzz Vanilla Unit")
	if !strings.Contains(s, "\"Greenway Lore\"\n          ]") {
		t.Fatal("fixture not as expected")
	}
	s = strings.Replace(s, "\"Greenway Lore\"\n          ]", "\"Greenway Lore\", \"Zzz Glossary Rule\"\n          ]", 1)
	return vanilla, glossary, []byte(s)
}

func hasIssue(rep RulesetReport, file, sub string) bool {
	return slices.ContainsFunc(rep.Issues(), func(i RulesetIssue) bool { return i.File == file && !i.Warn && strings.Contains(i.Message, sub) })
}

// TestCheckSetChecksTheBattleCompaniesFileAgainstTheSuppliedFiles: the unit and the rule exist only in the uploaded files, so the
// file passes with them and fails with the shipped ones; the checks write nothing.
func TestCheckSetChecksTheBattleCompaniesFileAgainstTheSuppliedFiles(t *testing.T) {
	ctx := context.Background()
	rs, repo := newTestRulesets(t)
	withDefaults(t, repo) // a strict mock: any Insert fails the test
	vanilla, glossary, bc := threeCustomFiles(t)

	rep, err := rs.CheckSet(ctx, RulesetSet{Vanilla: vanilla, Glossary: glossary, BattleCompanies: bc})
	if err != nil || rep.Errors() != 0 {
		t.Fatalf("with the uploaded vanilla and glossary: %v %v", err, rep.Issues())
	}
	for name, set := range map[string]RulesetSet{
		"shipped vanilla and glossary":       {BattleCompanies: bc},
		"uploaded glossary, shipped vanilla": {Glossary: glossary, BattleCompanies: bc},
	} {
		rep, err = rs.CheckSet(ctx, set)
		if !errors.Is(err, ErrInvalidRuleset) || !hasIssue(rep, "battle_companies.json", "Zzz Vanilla Unit") {
			t.Errorf("%s: %v %v", name, err, rep.Issues())
		}
	}
	rep, err = rs.CheckSet(ctx, RulesetSet{Vanilla: vanilla, BattleCompanies: bc})
	if !errors.Is(err, ErrInvalidRuleset) || !hasIssue(rep, "battle_companies.json", "Zzz Glossary Rule") || hasIssue(rep, "battle_companies.json", "Zzz Vanilla Unit") {
		t.Errorf("uploaded vanilla, shipped glossary: %v %v", err, rep.Issues())
	}
}

// TestCheckSetOneReportGroupedByFile: every file's problems come back together, in the order vanilla, glossary, Battle Companies.
func TestCheckSetOneReportGroupedByFile(t *testing.T) {
	rs, repo := newTestRulesets(t)
	withDefaults(t, repo)
	rep, err := rs.CheckSet(context.Background(), RulesetSet{Vanilla: []byte(`{"appVersion":1}`), Glossary: []byte(`{"name":"G"}`), BattleCompanies: []byte(`{"schemaVersion":2,"name":"BC","version":"1","extends":"V","limits":"no"}`)})
	if !errors.Is(err, ErrInvalidRuleset) {
		t.Fatalf("err %v", err)
	}
	var files []string
	for _, i := range rep.Issues() {
		if !slices.Contains(files, i.File) {
			files = append(files, i.File)
		}
	}
	if !slices.Equal(files, []string{"vanilla.json", "glossary.json", "battle_companies.json"}) {
		t.Errorf("files %v", files)
	}
}

// TestCheckSetUnreadableCompanionFallsBackWithAWarning: a vanilla file that cannot be parsed is reported itself; the Battle Companies
// file is checked against the shipped one and says so.
func TestCheckSetUnreadableCompanionFallsBackWithAWarning(t *testing.T) {
	rs, repo := newTestRulesets(t)
	withDefaults(t, repo)
	rep, err := rs.CheckSet(context.Background(), RulesetSet{Vanilla: []byte(`{nope`), BattleCompanies: []byte(bcFile)})
	if err == nil || !hasIssue(rep, "vanilla.json", "not valid JSON") || len(rep.Warnings()) != 1 || rep.Errors() != 1 {
		t.Fatalf("nothing said: %v", rep.Issues())
	}
}

// TestCheckSetNeverPanicsOnGarbage: truncated, binary, empty and deeply nested input is an issue, never a panic.
func TestCheckSetNeverPanicsOnGarbage(t *testing.T) {
	rs, repo := newTestRulesets(t)
	withDefaults(t, repo)
	_, _, valid := threeCustomFiles(t)
	inputs := map[string][]byte{
		"empty":       {},
		"binary":      {0x00, 0xff, 0xfe, 0x01, 0x80, 0x7f},
		"truncated":   valid[:len(valid)/2],
		"nested":      []byte(strings.Repeat("[", 100000) + strings.Repeat("]", 100000)),
		"nested obj":  []byte(strings.Repeat(`{"a":`, 50000) + "1" + strings.Repeat("}", 50000)),
		"scalar":      []byte("42"),
		"null":        []byte("null"),
		"bom":         append([]byte("\xef\xbb\xbf"), valid...),
		"wrong utf8":  []byte("{\"name\":\"\xff\xfe\"}"),
		"huge string": []byte(`{"name":"` + strings.Repeat("x", 5<<20) + `"}`),
	}
	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			for _, set := range []RulesetSet{{Vanilla: in}, {Glossary: in}, {BattleCompanies: in}, {Vanilla: in, Glossary: in, BattleCompanies: in}} {
				rep, err := rs.CheckSet(context.Background(), set)
				if err == nil && rep.Errors() > 0 {
					t.Errorf("errors without an error")
				}
			}
		})
	}
}

func FuzzCheckSet(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":2,"name":"x","version":"1","extends":"y"}`))
	f.Add([]byte(`[[[[`))
	f.Add([]byte{0, 1, 2})
	f.Fuzz(func(t *testing.T, in []byte) {
		rs, repo := newTestRulesets(t)
		withDefaults(t, repo)
		_, _ = rs.CheckSet(context.Background(), RulesetSet{BattleCompanies: in})
		_, _ = rs.CheckSet(context.Background(), RulesetSet{Vanilla: in, Glossary: in})
	})
}

// TestStoreAllAsChecksTheBattleCompaniesFileAgainstTheUploadedFiles: the campaign form's all-or-nothing store uses the vanilla file
// and glossary uploaded with the Battle Companies file; without them nothing is inserted.
func TestStoreAllAsChecksTheBattleCompaniesFileAgainstTheUploadedFiles(t *testing.T) {
	ctx := context.Background()
	vanilla, glossary, bc := threeCustomFiles(t)
	up := func(kind RulesetKind, c []byte) RulesetUpload {
		h, err := RulesetHash(c)
		if err != nil {
			t.Fatal(err)
		}
		return RulesetUpload{Kind: kind, Hash: h, Content: c}
	}
	t.Run("all three uploaded: stored", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		withDefaults(t, repo)
		repo.EXPECT().Info(gomock.Any(), gomock.Any()).Return(RulesetInfo{}, ErrNotFound).Times(3)
		repo.EXPECT().Insert(gomock.Any(), gomock.Any()).Return(nil).Times(3)
		infos, err := rs.StoreAllAs(ctx, User{ID: 1}, []RulesetUpload{up(KindBattleCompanies, bc), up(KindVanilla, vanilla), up(KindGlossary, glossary)})
		if err != nil || len(infos) != 3 || infos[0].Kind != KindBattleCompanies {
			t.Fatalf("%v %v", infos, err)
		}
	})
	t.Run("without the vanilla file: refused, nothing inserted", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		withDefaults(t, repo)
		repo.EXPECT().Info(gomock.Any(), gomock.Any()).Return(RulesetInfo{}, ErrNotFound).Times(2)
		_, err := rs.StoreAllAs(ctx, User{ID: 1}, []RulesetUpload{up(KindBattleCompanies, bc), up(KindGlossary, glossary)})
		if !errors.Is(err, ErrInvalidRuleset) {
			t.Fatalf("err %v", err)
		}
	})
}
