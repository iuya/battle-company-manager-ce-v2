package core

import (
	"log/slog"
	"os"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// BenchmarkLoadRuleSet measures what a rule set costs to build from the three shipped files (done once per content hash):
//
//	LoadRuleSet   core.LoadRuleSet(vanilla, battle_companies, glossary): the whole load
//	Decode        lean.Decode alone on the Battle Companies file against a prepared vanilla book and glossary
//
// go test ./internal/core -run xxx -bench LoadRuleSet -benchmem -count 10
func BenchmarkLoadRuleSet(b *testing.B) {
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
	log := slog.New(slog.DiscardHandler)

	b.Run("LoadRuleSet", func(b *testing.B) {
		b.SetBytes(int64(len(vanilla) + len(bc) + len(glossary)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := LoadRuleSet(vanilla, bc, glossary, log); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Decode", func(b *testing.B) {
		vb, err := ParseVanillaWith(vanilla, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		hash, err := RulesetHash(glossary)
		if err != nil {
			b.Fatal(err)
		}
		gl, err := BuildGlossary(RulesetInfo{Kind: KindGlossary, Hash: hash}, glossary)
		if err != nil {
			b.Fatal(err)
		}
		lower := newVanillaLower(vb, gl)
		b.SetBytes(int64(len(bc)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if doc, _, rep := lean.Decode("battle_companies.json", bc, lower); doc == nil || !rep.OK() {
				b.Fatal(rep)
			}
		}
	})
}
