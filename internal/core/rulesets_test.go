package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
)

var testSchemas = map[RulesetKind][]byte{
	KindVanilla:         []byte(`{"type":"object","required":["game","appVersion"]}`),
	KindBattleCompanies: []byte(`{"$id":"bc.schema.json","type":"object","required":["name","version","extends"],"properties":{"limits":{"type":"array"}}}`),
	KindGlossary:        []byte(`{"type":"object","required":["name","version","terms"]}`),
}

const (
	vanillaFile  = `{"game":"MESBG 2024","appVersion":2}`
	glossaryFile = `{"name":"G","version":"1","terms":[]}`
)

// bcFile is a small valid lean Battle Companies file (the fixture of the lean package), compact.
var bcFile = func() string {
	raw, err := os.ReadFile("lean/testdata/valid/minimal.json")
	if err != nil {
		panic(err)
	}
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		panic(err)
	}
	return b.String()
}()

// withDefaults lets the repository answer with the shipped vanilla file and glossary as the defaults a Battle Companies file is checked
// against.
func withDefaults(t *testing.T, repo *MockrulesetRepository) {
	t.Helper()
	for kind, f := range map[RulesetKind]string{KindVanilla: "vanilla/data2024.json", KindGlossary: "glossary/mesbg_glossary.json"} {
		raw := readDef(t, f)
		hash := "default-" + string(kind)
		repo.EXPECT().Default(gomock.Any(), kind).Return(RulesetInfo{Kind: kind, Hash: hash, Default: true}, nil).AnyTimes()
		repo.EXPECT().Get(gomock.Any(), hash).Return(StoredRuleset{RulesetInfo: RulesetInfo{Kind: kind, Hash: hash}, Content: raw}, nil).AnyTimes()
	}
}

func newTestRulesets(t *testing.T) (*Rulesets, *MockrulesetRepository) {
	t.Helper()
	repo := NewMockrulesetRepository(gomock.NewController(t))
	rs, err := NewRulesets(repo, testSchemas)
	if err != nil {
		t.Fatal(err)
	}
	return rs, repo
}

func mustHash(t *testing.T, s string) string {
	t.Helper()
	h, err := RulesetHash([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestNewRulesetsNeedsEverySchema(t *testing.T) {
	repo := NewMockrulesetRepository(gomock.NewController(t))
	if _, err := NewRulesets(repo, map[RulesetKind][]byte{KindVanilla: testSchemas[KindVanilla]}); err == nil {
		t.Error("missing schemas must fail")
	}
	bad := map[RulesetKind][]byte{KindVanilla: []byte("{"), KindBattleCompanies: testSchemas[KindBattleCompanies], KindGlossary: testSchemas[KindGlossary]}
	if _, err := NewRulesets(repo, bad); err == nil {
		t.Error("a broken schema must fail")
	}
}

func TestRulesetsDefaultAndLookup(t *testing.T) {
	rs, repo := newTestRulesets(t)
	ctx := context.Background()
	want := RulesetInfo{Kind: KindGlossary, Hash: "h", Default: true}
	repo.EXPECT().Default(ctx, KindGlossary).Return(want, nil)
	if got, err := rs.Default(ctx, KindGlossary); err != nil || got != want {
		t.Errorf("Default = %v, %v", got, err)
	}
	repo.EXPECT().Info(ctx, "h").Return(want, nil)
	if got, err := rs.Lookup(ctx, "h"); err != nil || got != want {
		t.Errorf("Lookup = %v, %v", got, err)
	}
	repo.EXPECT().Info(ctx, "x").Return(RulesetInfo{}, ErrNotFound)
	if _, err := rs.Lookup(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Lookup unknown = %v", err)
	}
}

func TestRulesetsFind(t *testing.T) {
	ctx := context.Background()
	stored := RulesetInfo{Kind: KindBattleCompanies, Hash: "h", Name: "BC", Version: "1"}
	vanilla := RulesetInfo{Kind: KindVanilla, Hash: "v1", Name: "MESBG", Version: "2"}
	tests := []struct {
		name  string
		kind  RulesetKind
		setup func(*MockrulesetRepository)
		want  RulesetInfo
		err   error
	}{
		{"known hash", KindBattleCompanies, func(m *MockrulesetRepository) { m.EXPECT().Info(ctx, "h").Return(stored, nil) }, stored, nil},
		{"unknown hash of a hash-identified kind", KindBattleCompanies, func(m *MockrulesetRepository) { m.EXPECT().Info(ctx, "h").Return(RulesetInfo{}, ErrNotFound) }, RulesetInfo{}, ErrNotFound},
		{"hash stored under another kind", KindGlossary, func(m *MockrulesetRepository) { m.EXPECT().Info(ctx, "h").Return(stored, nil) }, RulesetInfo{}, ErrNotFound},
		{"vanilla by hash", KindVanilla, func(m *MockrulesetRepository) { m.EXPECT().Info(ctx, "h").Return(vanilla, nil) }, vanilla, nil},
		{"vanilla name and version do not identify a file", KindVanilla, func(m *MockrulesetRepository) {
			m.EXPECT().Info(ctx, "h").Return(RulesetInfo{}, ErrNotFound)
		}, RulesetInfo{}, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs, repo := newTestRulesets(t)
			tt.setup(repo)
			got, err := rs.Find(ctx, tt.kind, "h", "MESBG", "2")
			if got != tt.want || !errors.Is(err, tt.err) && err != tt.err {
				t.Errorf("Find = %v, %v; want %v, %v", got, err, tt.want, tt.err)
			}
		})
	}
	t.Run("repository failure", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		boom := errors.New("db down")
		repo.EXPECT().Info(ctx, "h").Return(RulesetInfo{}, boom)
		if _, err := rs.Find(ctx, KindVanilla, "h", "MESBG", "2"); !errors.Is(err, boom) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestRulesetsStoreStoresCompactedContent(t *testing.T) {
	rs, repo := newTestRulesets(t)
	ctx := context.Background()
	withDefaults(t, repo)
	raw, _ := os.ReadFile("lean/testdata/valid/minimal.json")
	pretty := string(raw)
	hash := mustHash(t, pretty)
	repo.EXPECT().Info(ctx, hash).Return(RulesetInfo{}, ErrNotFound)
	repo.EXPECT().Insert(ctx, StoredRuleset{
		RulesetInfo: RulesetInfo{Kind: KindBattleCompanies, Hash: hash, Name: "Fixture", Version: "1.0"},
		Size:        len(bcFile), Content: []byte(bcFile),
	}).Return(nil)
	info, err := rs.Store(ctx, KindBattleCompanies, hash, []byte(pretty))
	if err != nil || info.Hash != hash || info.Name != "Fixture" || info.Version != "1.0" || info.Default {
		t.Errorf("Store = %+v, %v", info, err)
	}
}

func TestRulesetsStoreAlreadyStoredSkipsInsert(t *testing.T) {
	rs, repo := newTestRulesets(t)
	ctx := context.Background()
	hash := mustHash(t, bcFile)
	known := RulesetInfo{Kind: KindBattleCompanies, Hash: hash, Name: "Fixture", Version: "1.0", Default: true}
	repo.EXPECT().Info(ctx, hash).Return(known, nil)
	if got, err := rs.Store(ctx, KindBattleCompanies, hash, []byte(bcFile)); err != nil || got != known {
		t.Errorf("Store = %+v, %v", got, err)
	}
}

func TestRulesetsStoreRejections(t *testing.T) {
	ctx := context.Background()
	big := `{"schemaVersion":2,"name":"BC","version":"1","extends":"V","pad":"` + strings.Repeat("x", MaxRulesetSize) + `"}`
	many := `{"schemaVersion":2,"name":"BC","version":"1","extends":"V","pad":[0` + strings.Repeat(",0", MaxRulesetValues) + `]}`
	deep := `{"schemaVersion":2,"name":"BC","version":"1","extends":"V","pad":` + strings.Repeat("[", MaxRulesetDepth+1) + strings.Repeat("]", MaxRulesetDepth+1) + `}`
	tests := []struct {
		name    string
		kind    RulesetKind
		content string
		hash    func(string) string // nil: the right hash
		setup   func(*MockrulesetRepository, string)
		want    string // text the reason must contain
	}{
		{"unknown kind", "nope", bcFile, nil, nil, "unknown kind"},
		{"too large", KindBattleCompanies, big, func(string) string { return "h" }, nil, "larger than 16 MB"},
		{"too many values", KindBattleCompanies, many, nil, nil, "more than 1000000 values"},
		{"nested too deep", KindBattleCompanies, deep, nil, nil, "levels deep"},
		{"not json", KindBattleCompanies, "{nope", func(string) string { return "h" }, nil, "not valid JSON"},
		{"hash mismatch", KindBattleCompanies, bcFile, func(string) string { return "deadbeef" }, nil, "does not match the hash"},
		{"wrong kind of document", KindGlossary, bcFile, nil, nil, "does not look like a Glossary"},
		{"lean violation", KindBattleCompanies, `{"schemaVersion":2,"name":"BC","version":"1","extends":"V","limits":"no"}`, nil,
			func(m *MockrulesetRepository, h string) {
				m.EXPECT().Info(ctx, h).Return(RulesetInfo{}, ErrNotFound)
				withDefaults(t, m)
			}, "limits"},
		{"hash stored as another kind", KindBattleCompanies, bcFile, nil,
			func(m *MockrulesetRepository, h string) {
				m.EXPECT().Info(ctx, h).Return(RulesetInfo{Kind: KindVanilla, Hash: h}, nil)
			}, "already stored as a Vanilla"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs, repo := newTestRulesets(t)
			hash := mustHash2(tt.content)
			if tt.hash != nil {
				hash = tt.hash(hash)
			}
			if tt.setup != nil {
				tt.setup(repo, hash)
			}
			_, err := rs.Store(ctx, tt.kind, hash, []byte(tt.content))
			if !errors.Is(err, ErrInvalidRuleset) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want ErrInvalidRuleset containing %q", err, tt.want)
			}
		})
	}
}

func mustHash2(s string) string { h, _ := RulesetHash([]byte(s)); return h }

func TestRulesetsStoreRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("db down")
	hash := mustHash(t, vanillaFile)
	for name, setup := range map[string]func(*MockrulesetRepository){
		"info": func(m *MockrulesetRepository) { m.EXPECT().Info(ctx, hash).Return(RulesetInfo{}, boom) },
		"insert": func(m *MockrulesetRepository) {
			m.EXPECT().Info(ctx, hash).Return(RulesetInfo{}, ErrNotFound)
			m.EXPECT().Insert(ctx, gomock.Any()).Return(boom)
		},
	} {
		t.Run(name, func(t *testing.T) {
			rs, repo := newTestRulesets(t)
			setup(repo)
			if _, err := rs.Store(ctx, KindVanilla, hash, []byte(vanillaFile)); !errors.Is(err, boom) {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestRulesetsRegisterDefaults(t *testing.T) {
	ctx := context.Background()
	files := map[RulesetKind]string{KindVanilla: vanillaFile, KindBattleCompanies: bcFile, KindGlossary: glossaryFile}
	t.Run("first start inserts and marks every default", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		for kind, content := range files {
			h := mustHash(t, content)
			repo.EXPECT().Info(ctx, h).Return(RulesetInfo{}, ErrNotFound)
			repo.EXPECT().Insert(ctx, gomock.Cond(func(x any) bool { return x.(StoredRuleset).Hash == h })).DoAndReturn(func(_ context.Context, s StoredRuleset) error {
				if s.UploadedBy != 0 || s.Kind != kind {
					t.Errorf("stored %+v", s)
				}
				return nil
			})
			repo.EXPECT().Default(ctx, kind).Return(RulesetInfo{}, ErrNotFound)
			repo.EXPECT().SetDefault(ctx, kind, h).Return(nil)
		}
		if err := rs.RegisterDefaults(ctx, []byte(vanillaFile), []byte(bcFile), []byte(glossaryFile)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("second start changes nothing", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		for kind, content := range files {
			h := mustHash(t, content)
			info := RulesetInfo{Kind: kind, Hash: h, Default: true}
			repo.EXPECT().Info(ctx, h).Return(info, nil)
			repo.EXPECT().Default(ctx, kind).Return(info, nil)
		}
		if err := rs.RegisterDefaults(ctx, []byte(vanillaFile), []byte(bcFile), []byte(glossaryFile)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a newer shipped file replaces the default", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		newBC := strings.Replace(bcFile, `"version":"1.0"`, `"version":"2.0"`, 1)
		h := mustHash(t, newBC)
		repo.EXPECT().Info(ctx, h).Return(RulesetInfo{Kind: KindBattleCompanies, Hash: h}, nil)
		repo.EXPECT().Default(ctx, KindBattleCompanies).Return(RulesetInfo{Hash: "old"}, nil)
		repo.EXPECT().SetDefault(ctx, KindBattleCompanies, h).Return(nil)
		other := map[RulesetKind]string{KindVanilla: vanillaFile, KindGlossary: glossaryFile}
		for kind, content := range other {
			hh := mustHash(t, content)
			info := RulesetInfo{Kind: kind, Hash: hh, Default: true}
			repo.EXPECT().Info(ctx, hh).Return(info, nil)
			repo.EXPECT().Default(ctx, kind).Return(info, nil)
		}
		if err := rs.RegisterDefaults(ctx, []byte(vanillaFile), []byte(newBC), []byte(glossaryFile)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("an invalid shipped file fails the start", func(t *testing.T) {
		rs, _ := newTestRulesets(t)
		err := rs.RegisterDefaults(ctx, []byte(`{"nope":1}`), []byte(bcFile), []byte(glossaryFile))
		if !errors.Is(err, ErrInvalidRuleset) || !strings.Contains(err.Error(), "vanilla") {
			t.Errorf("err = %v", err)
		}
	})
}

func TestRulesetsCampaignOptions(t *testing.T) {
	ctx := context.Background()
	rs, repo := newTestRulesets(t)
	content := []byte(`{"campaign":{"options":[]}}`)
	repo.EXPECT().Default(ctx, KindBattleCompanies).Return(RulesetInfo{Hash: "h"}, nil)
	repo.EXPECT().Get(ctx, "h").Return(StoredRuleset{Content: content}, nil)
	if _, err := rs.CampaignOptions(ctx); err != nil {
		t.Errorf("err = %v", err)
	}
	repo.EXPECT().Default(ctx, KindBattleCompanies).Return(RulesetInfo{}, ErrNotFound)
	if _, err := rs.CampaignOptions(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("no default: %v", err)
	}
}

// The schemas that ship with the app accept the files that ship with the app.
func TestRulesetsShippedFilesPassTheirSchemas(t *testing.T) {
	read := func(p string) []byte {
		b, err := readFile("../../definitions/" + p)
		if err != nil {
			t.Skip("definitions not available:", err)
		}
		return b
	}
	schemas := map[RulesetKind][]byte{
		KindVanilla:         read("vanilla/data2024.schema.json"),
		KindBattleCompanies: testSchemas[KindBattleCompanies],
		KindGlossary:        read("glossary/mesbg_glossary.schema.json"),
	}
	repo := NewMockrulesetRepository(gomock.NewController(t))
	withDefaults(t, repo)
	rs, err := NewRulesets(repo, schemas)
	if err != nil {
		t.Fatal(err)
	}
	for kind, f := range map[RulesetKind]string{KindVanilla: "vanilla/data2024.json", KindGlossary: "glossary/mesbg_glossary.json"} {
		if err := rs.validate(kind, read(f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	// the Battle Companies file is checked by the lean decoder against the shipped vanilla file and glossary
	if rep, err := rs.Check(context.Background(), KindBattleCompanies, read("battle_companies/battle_companies.json")); err != nil || rep.Errors() != 0 {
		t.Errorf("the shipped Battle Companies file: %v %v", err, rep.Issues())
	}
}

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }

func TestRulesets_AdminActions(t *testing.T) {
	ctx := context.Background()
	super, admin := User{ID: 1, Role: RoleSuperuser}, User{ID: 2, Role: RoleAdmin}

	t.Run("superuser only", func(t *testing.T) {
		rs, _ := newTestRulesets(t)
		if _, err := rs.ListWithUsage(ctx, admin); !errors.Is(err, ErrRefused) {
			t.Errorf("list: %v", err)
		}
		if err := rs.SetDefault(ctx, admin, "h"); !errors.Is(err, ErrRefused) {
			t.Errorf("default: %v", err)
		}
		if err := rs.Delete(ctx, admin, "h"); !errors.Is(err, ErrRefused) {
			t.Errorf("delete: %v", err)
		}
		if _, err := rs.PruneUnused(ctx, admin); !errors.Is(err, ErrRefused) {
			t.Errorf("prune: %v", err)
		}
	})
	t.Run("list", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		want := []RulesetUsage{{RulesetInfo: RulesetInfo{Hash: "a"}, Campaigns: []CampaignRef{{ID: 1, Name: "C"}}}}
		repo.EXPECT().ListUsage(gomock.Any()).Return(want, nil)
		got, err := rs.ListWithUsage(ctx, super)
		if err != nil || len(got) != 1 || got[0].Campaigns[0].Name != "C" {
			t.Fatalf("%v %v", got, err)
		}
	})
	t.Run("set default uses the kind of the file", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		repo.EXPECT().Info(gomock.Any(), "h").Return(RulesetInfo{Kind: KindGlossary, Hash: "h"}, nil)
		repo.EXPECT().SetDefault(gomock.Any(), KindGlossary, "h").Return(nil)
		if err := rs.SetDefault(ctx, super, "h"); err != nil {
			t.Fatal(err)
		}
		repo.EXPECT().Info(gomock.Any(), "x").Return(RulesetInfo{}, ErrNotFound)
		if err := rs.SetDefault(ctx, super, "x"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("delete passes the repository's verdict through", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		for _, want := range []error{nil, ErrNotFound, Refuse("in use")} {
			repo.EXPECT().Delete(gomock.Any(), "h").Return(want)
			if err := rs.Delete(ctx, super, "h"); !errors.Is(err, want) && err != want {
				t.Errorf("want %v got %v", want, err)
			}
		}
	})
	t.Run("prune", func(t *testing.T) {
		rs, repo := newTestRulesets(t)
		repo.EXPECT().DeleteUnused(gomock.Any()).Return(3, nil)
		if n, err := rs.PruneUnused(ctx, super); err != nil || n != 3 {
			t.Fatalf("%d %v", n, err)
		}
	})
}

func TestRulesets_Download(t *testing.T) {
	ctx := context.Background()
	rs, repo := newTestRulesets(t)
	if _, _, err := rs.Download(ctx, User{ID: 2, Role: RoleAdmin}, "h"); !errors.Is(err, ErrRefused) {
		t.Errorf("admin: %v", err)
	}
	repo.EXPECT().Get(gomock.Any(), "h").Return(StoredRuleset{RulesetInfo: RulesetInfo{Hash: "h", Name: "N"}, Content: []byte("{}")}, nil)
	info, content, err := rs.Download(ctx, User{ID: 1, Role: RoleSuperuser}, "h")
	if err != nil || info.Name != "N" || string(content) != "{}" {
		t.Fatalf("%v %s %v", info, content, err)
	}
	repo.EXPECT().Get(gomock.Any(), "x").Return(StoredRuleset{}, ErrNotFound)
	if _, _, err := rs.Download(ctx, User{ID: 1, Role: RoleSuperuser}, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

// Two files with the same name and version but different content are two files: the second is stored too.
func TestRulesetsSameNameAndVersionCoexist(t *testing.T) {
	ctx := context.Background()
	rs, repo := newTestRulesets(t)
	hash := mustHash(t, vanillaFile)
	repo.EXPECT().Info(ctx, hash).Return(RulesetInfo{}, ErrNotFound)
	repo.EXPECT().Insert(ctx, gomock.Any()).Return(nil)
	if _, err := rs.Store(ctx, KindVanilla, hash, []byte(vanillaFile)); err != nil {
		t.Fatalf("a vanilla file whose name and version are stored under another hash: %v", err)
	}
	repo.EXPECT().Info(ctx, "other").Return(RulesetInfo{}, ErrNotFound)
	if _, err := rs.Find(ctx, KindVanilla, "other", "MESBG 2024", "2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("name and version do not identify a file: %v", err)
	}
}
