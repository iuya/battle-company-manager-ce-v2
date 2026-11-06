package core

import (
	"context"
	"os"
	"testing"

	"go.uber.org/mock/gomock"
)

// BenchmarkRegisterDefaults is what every start of the server does with the three shipped files once the database holds them: hash
// them, check their shape and find them stored.
//
// go test ./internal/core -run xxx -bench RegisterDefaults -benchmem -count 6
func BenchmarkRegisterDefaults(b *testing.B) {
	read := func(p string) []byte {
		raw, err := os.ReadFile(p)
		if err != nil {
			b.Skip("definitions not available: ", err)
		}
		return raw
	}
	vanilla := read("../../definitions/vanilla/data2024.json")
	bc := read("../../definitions/battle_companies/battle_companies.json")
	glossary := read("../../definitions/glossary/mesbg_glossary.json")
	schemas := map[RulesetKind][]byte{
		KindVanilla:  read("../../definitions/vanilla/data2024.schema.json"),
		KindGlossary: read("../../definitions/glossary/mesbg_glossary.schema.json"),
	}
	repo := NewMockrulesetRepository(gomock.NewController(b))
	rs, err := NewRulesets(repo, schemas)
	if err != nil {
		b.Fatal(err)
	}
	for kind, content := range map[RulesetKind][]byte{KindVanilla: vanilla, KindBattleCompanies: bc, KindGlossary: glossary} {
		hash, err := RulesetHash(content)
		if err != nil {
			b.Fatal(err)
		}
		info := RulesetInfo{Kind: kind, Hash: hash, Default: true}
		repo.EXPECT().Info(gomock.Any(), hash).Return(info, nil).AnyTimes()
		repo.EXPECT().Default(gomock.Any(), kind).Return(info, nil).AnyTimes()
	}
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := rs.RegisterDefaults(ctx, vanilla, bc, glossary); err != nil {
			b.Fatal(err)
		}
	}
}
