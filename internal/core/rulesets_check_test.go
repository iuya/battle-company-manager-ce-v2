package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

func asReport(t *testing.T, err error) *RulesetReport {
	t.Helper()
	var rep *RulesetReport
	if !errors.As(err, &rep) {
		t.Fatalf("error %v is not a *RulesetReport", err)
	}
	return rep
}

// TestRulesetsCheckNeverTouchesTheRepository: a dry run writes nothing (the strict mock fails on any Insert); a Battle Companies file reads
// the default vanilla file and glossary to be checked against.
func TestRulesetsCheckNeverTouchesTheRepository(t *testing.T) {
	rs, repo := newTestRulesets(t)
	withDefaults(t, repo)
	ctx := context.Background()
	rep, err := rs.Check(ctx, KindBattleCompanies, []byte(bcFile))
	if err != nil || len(rep.Issues()) != 0 || rep.Errors() != 0 {
		t.Fatalf("a valid file: %v %+v", err, rep)
	}
	rep, err = rs.Check(ctx, KindBattleCompanies, []byte(`{"schemaVersion":2,"name":"BC","version":"1","extends":"V","limits":"no"}`))
	if !errors.Is(err, ErrInvalidRuleset) {
		t.Fatalf("err %v", err)
	}
	if rep.Errors() < 1 || !slices.ContainsFunc(rep.Issues(), func(i RulesetIssue) bool { return i.File == "battle_companies.json" && i.Path == "limits" }) {
		t.Errorf("issue %+v", rep.Issues())
	}
	if asReport(t, err).Error() != rep.Error() || !strings.Contains(err.Error(), "limits") {
		t.Errorf("the error is the report: %v", err)
	}
}

func TestRulesetsCheckPreflightFailuresAreOneIssueReports(t *testing.T) {
	rs, _ := newTestRulesets(t)
	ctx := context.Background()
	for _, c := range []struct {
		name    string
		kind    RulesetKind
		content string
		want    string
		file    string
	}{
		{"not json", KindBattleCompanies, "{nope", "not valid JSON", "battle_companies.json"},
		{"wrong document", KindGlossary, bcFile, "does not look like a Glossary", "glossary.json"},
		{"unknown kind", "nope", bcFile, "unknown kind", "battle_companies.json"},
		{"too large", KindVanilla, strings.Repeat(" ", MaxRulesetSize+1), "larger than 16 MB", "vanilla.json"},
		{"no version", KindBattleCompanies, `{"name":"x","extends":"y"}`, "no version", "battle_companies.json"},
	} {
		t.Run(c.name, func(t *testing.T) {
			rep, err := rs.Check(ctx, c.kind, []byte(c.content))
			if !errors.Is(err, ErrInvalidRuleset) || len(rep.Issues()) != 1 || !strings.Contains(rep.Issues()[0].Message, c.want) || rep.Issues()[0].File != c.file {
				t.Errorf("err %v report %+v", err, rep.Issues())
			}
		})
	}
}

// TestSchemaFailuresAreAllListed: a file with many schema failures lists every one (the old text named five and counted the rest).
func TestSchemaFailuresAreAllListed(t *testing.T) {
	var props []string
	var bad []string
	for i := 0; i < 12; i++ {
		props = append(props, fmt.Sprintf(`"p%d":{"type":"integer"}`, i))
		bad = append(bad, fmt.Sprintf(`"p%d":"x"`, i))
	}
	schemas := map[RulesetKind][]byte{
		KindVanilla:         []byte(`{"type":"object","properties":{` + strings.Join(props, ",") + `,"list":{"type":"array","items":{"type":"integer"}}}}`),
		KindBattleCompanies: testSchemas[KindBattleCompanies],
		KindGlossary:        testSchemas[KindGlossary],
	}
	rs, err := NewRulesets(NewMockrulesetRepository(gomock.NewController(t)), schemas)
	if err != nil {
		t.Fatal(err)
	}
	content := `{"game":"G","appVersion":1,` + strings.Join(bad, ",") + `,"list":[1,"a",3,"b"]}`
	rep, cerr := rs.Check(context.Background(), KindVanilla, []byte(content))
	if !errors.Is(cerr, ErrInvalidRuleset) || rep.Errors() != 14 {
		t.Fatalf("%d errors: %v", rep.Errors(), cerr)
	}
	paths := map[string]bool{}
	for _, i := range rep.Issues() {
		paths[i.Path] = true
	}
	if !paths["p0"] || !paths["p11"] || !paths["list[1]"] || !paths["list[3]"] {
		t.Errorf("paths %v", paths)
	}
	if !strings.Contains(cerr.Error(), "and 9 more") || strings.Count(cerr.Error(), "; ") != 5 {
		t.Errorf("the one-line text keeps its five and the count: %v", cerr)
	}
}

func TestSchemaPath(t *testing.T) {
	for in, want := range map[string][]string{"": nil, "data": {"data"}, "data.heroes[3].name": {"data", "heroes", "3", "name"}, `a["b c"][0]`: {"a", "b c", "0"}} {
		if got := schemaPath(want); got != in {
			t.Errorf("schemaPath(%v) = %q, want %q", want, got, in)
		}
	}
}

// v2Rulesets is a Rulesets whose repository holds the shipped vanilla file and glossary as defaults.
func v2Rulesets(t *testing.T, ctx context.Context) (*Rulesets, *MockrulesetRepository, []byte) {
	t.Helper()
	vanilla, bc, glossary := v2Files(t)
	rs, repo := newTestRulesets(t)
	repo.EXPECT().Default(ctx, KindVanilla).Return(RulesetInfo{Kind: KindVanilla, Hash: "v"}, nil).AnyTimes()
	repo.EXPECT().Default(ctx, KindGlossary).Return(RulesetInfo{Kind: KindGlossary, Hash: "g"}, nil).AnyTimes()
	repo.EXPECT().Get(ctx, "v").Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindVanilla, Hash: "v"}, Content: vanilla}, nil).AnyTimes()
	repo.EXPECT().Get(ctx, "g").Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: KindGlossary, Hash: "g"}, Content: glossary}, nil).AnyTimes()
	return rs, repo, bc
}

func TestRulesetsCheckAndStoreV2(t *testing.T) {
	ctx := context.Background()
	t.Run("a valid v2 file checks clean and is stored", func(t *testing.T) {
		rs, repo, bc := v2Rulesets(t, ctx)
		hash := mustHash2(string(bc))
		rep, err := rs.Check(ctx, KindBattleCompanies, bc)
		if err != nil || len(rep.Issues()) != 0 {
			t.Fatalf("check: %v %v", err, rep.Issues())
		}
		repo.EXPECT().Info(ctx, hash).Return(RulesetInfo{}, ErrNotFound)
		repo.EXPECT().Insert(ctx, gomock.Any()).Return(nil)
		info, err := rs.StoreAs(ctx, User{ID: 1}, KindBattleCompanies, hash, bc)
		if err != nil || info.Hash != hash || info.Kind != KindBattleCompanies {
			t.Fatalf("store: %v %+v", err, info)
		}
	})
	t.Run("a file of the old format is refused with the clear issue", func(t *testing.T) {
		rs, _, _ := v2Rulesets(t, ctx)
		old := []byte(`{"name":"x","version":"1","extends":"y"}`)
		rep, err := rs.Check(ctx, KindBattleCompanies, old)
		if !errors.Is(err, ErrInvalidRuleset) || len(rep.Issues()) != 1 || !strings.Contains(rep.Issues()[0].Message, "schemaVersion 1 is not supported") {
			t.Fatalf("check: %v %v", err, rep.Issues())
		}
	})
	t.Run("an invalid v2 file returns every issue and stores nothing", func(t *testing.T) {
		rs, repo, bc := v2Rulesets(t, ctx)
		bad := []byte(strings.Replace(strings.Replace(string(bc), `"maxModels": 15`, `"maxModels": 99`, 1), `"leaders": 1`, `"leaders": 9`, 1))
		hash := mustHash2(string(bad))
		rep, err := rs.Check(ctx, KindBattleCompanies, bad)
		if !errors.Is(err, ErrInvalidRuleset) || rep.Errors() != 2 {
			t.Fatalf("check: %v %d", err, rep.Errors())
		}
		repo.EXPECT().Info(ctx, hash).Return(RulesetInfo{}, ErrNotFound)
		_, err = rs.StoreAs(ctx, User{ID: 1}, KindBattleCompanies, hash, bad)
		if got := asReport(t, err); got.Errors() != 2 {
			t.Errorf("store: %v", got.Issues())
		}
	})
	t.Run("warnings are reported and do not block Check", func(t *testing.T) {
		rs, _, bc := v2Rulesets(t, ctx)
		var doc map[string]any
		if err := json.Unmarshal(bc, &doc); err != nil {
			t.Fatal(err)
		}
		doc["vanilla"] = map[string]any{"pins": []any{map[string]any{"kind": "hero", "name": "Nobody At All", "occurrence": 1}}}
		warn, _ := json.Marshal(doc)
		rep, err := rs.Check(ctx, KindBattleCompanies, warn)
		if err != nil || rep.Errors() != 0 || len(rep.Warnings()) != 1 || !rep.Issues()[0].Warn {
			t.Fatalf("%v %+v", err, rep.Issues())
		}
	})
	t.Run("without default files the report says so", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		repo.EXPECT().Default(ctx, KindVanilla).Return(RulesetInfo{}, ErrNotFound)
		_, bc2, _ := v2Files(t)
		rep, err := rs.Check(ctx, KindBattleCompanies, bc2)
		if err == nil || !strings.Contains(rep.Issues()[0].Message, "default vanilla and glossary files") {
			t.Fatalf("%v %+v", err, rep.Issues())
		}
	})
}

func TestJoinRulesetReports(t *testing.T) {
	a := &RulesetReport{issues: []RulesetIssue{{File: "a", Message: "1"}}}
	b := &RulesetReport{issues: []RulesetIssue{{File: "b", Message: "2"}, {File: "b", Message: "3", Warn: true}}, truncated: 4}
	plain := errors.Join(ErrInvalidRuleset, errors.New("boom"))
	if JoinRulesetReports(nil, nil) != nil {
		t.Error("no errors, no error")
	}
	if got := JoinRulesetReports(nil, a); got != error(a) {
		t.Errorf("a single report is returned as it is: %v", got)
	}
	got := asReport(t, JoinRulesetReports(a, nil, b))
	if len(got.Issues()) != 3 || got.Errors() != 2 || got.Truncated() != 4 || got.Issues()[0].File != "a" || got.Issues()[2].File != "b" || !errors.Is(got, ErrInvalidRuleset) {
		t.Errorf("merged %+v", got)
	}
	if err := JoinRulesetReports(a, plain); !errors.Is(err, ErrInvalidRuleset) || strings.Contains(err.Error(), "boom") == false {
		t.Errorf("a plain error keeps the plain join: %v", err)
	}
}

func TestRulesetsStoreAllAsIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	user := User{ID: 7}
	good := []RulesetUpload{
		{KindVanilla, mustHash2(vanillaFile), []byte(vanillaFile)},
		{KindBattleCompanies, mustHash2(bcFile), []byte(bcFile)},
		{KindGlossary, mustHash2(glossaryFile), []byte(glossaryFile)},
	}
	notFound := func(repo *MockrulesetRepository, ups ...RulesetUpload) {
		for _, u := range ups {
			repo.EXPECT().Info(ctx, u.Hash).Return(RulesetInfo{}, ErrNotFound)
		}
	}
	t.Run("three good files are all inserted in order", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		withDefaults(t, repo)
		notFound(repo, good...)
		var inserted []RulesetKind
		repo.EXPECT().Insert(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, s StoredRuleset) error {
			if s.UploadedBy != user.ID {
				t.Errorf("uploader %d", s.UploadedBy)
			}
			inserted = append(inserted, s.Kind)
			return nil
		}).Times(3)
		infos, err := rs.StoreAllAs(ctx, user, good)
		if err != nil || len(infos) != 3 || infos[0].Kind != KindVanilla || infos[2].Kind != KindGlossary {
			t.Fatalf("%v %v", infos, err)
		}
		if len(inserted) != 3 {
			t.Errorf("inserted %v", inserted)
		}
	})
	t.Run("two good files and one bad store none", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		withDefaults(t, repo)
		badBC := []byte(`{"schemaVersion":2,"name":"BC","version":"1","extends":"V","limits":"no"}`)
		ups := []RulesetUpload{good[0], {KindBattleCompanies, mustHash2(string(badBC)), badBC}, good[2]}
		notFound(repo, ups...) // and no Insert expectation: the strict mock fails on one
		infos, err := rs.StoreAllAs(ctx, user, ups)
		rep := asReport(t, err)
		if infos != nil || rep.Errors() < 1 || !slices.ContainsFunc(rep.Issues(), func(i RulesetIssue) bool { return i.File == "battle_companies.json" && i.Path == "limits" }) {
			t.Fatalf("%v %+v", infos, rep.Issues())
		}
	})
	t.Run("the problems of several bad files come back together", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		withDefaults(t, repo)
		badBC := []byte(`{"schemaVersion":2,"name":"BC","version":"1","extends":"V","limits":"no"}`)
		badGSchema := []byte(`{"name":"G","version":"1"}`)
		ups := []RulesetUpload{{KindBattleCompanies, mustHash2(string(badBC)), badBC}, {KindGlossary, mustHash2(string(badGSchema)), badGSchema}}
		notFound(repo, ups[0])
		_, err := rs.StoreAllAs(ctx, user, ups)
		rep := asReport(t, err)
		if rep.Errors() < 2 || rep.Issues()[0].File != "battle_companies.json" || rep.Issues()[len(rep.Issues())-1].File != "glossary.json" {
			t.Errorf("%+v", rep.Issues())
		}
	})
	t.Run("a file already stored is not checked again or inserted", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		known := RulesetInfo{Kind: KindVanilla, Hash: good[0].Hash, Name: "V"}
		repo.EXPECT().Info(ctx, good[0].Hash).Return(known, nil)
		infos, err := rs.StoreAllAs(ctx, user, good[:1])
		if err != nil || infos[0] != known {
			t.Fatalf("%v %v", infos, err)
		}
	})
	t.Run("a repository failure is not reported as a bad file", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		boom := errors.New("db down")
		repo.EXPECT().Info(ctx, good[0].Hash).Return(RulesetInfo{}, boom)
		if _, err := rs.StoreAllAs(ctx, user, good[:1]); !errors.Is(err, boom) || errors.Is(err, ErrInvalidRuleset) {
			t.Errorf("err %v", err)
		}
	})
	t.Run("an insert failure after the checks is returned as is", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		boom := errors.New("db down")
		notFound(repo, good[0])
		repo.EXPECT().Insert(ctx, gomock.Any()).Return(boom)
		if _, err := rs.StoreAllAs(ctx, user, good[:1]); !errors.Is(err, boom) {
			t.Errorf("err %v", err)
		}
	})
}
