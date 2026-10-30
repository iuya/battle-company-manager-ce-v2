package core

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"os"
	"testing"
)

// BenchmarkJSON compares encoding/json (v1 API) with encoding/json/v2 for the workloads of the application, on the same inputs:
//
//	go test ./internal/core -run xxx -bench JSON -benchmem -count 10
//
// v2 is run with the v1 behaviour options (json.DefaultOptionsV1: case-insensitive names, nil as null, v1 omitempty) so that what is
// measured is the engine, and with its own defaults for comparison. The results are in docs/performance.md.
func BenchmarkJSON(b *testing.B) {
	v1 := json.DefaultOptionsV1()
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

	run := func(name string, f func()) {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				f()
			}
		})
	}

	// the vanilla file into the typed document the book is built from (maps of raw values)
	run("decode-vanilla/v1", func() {
		var d vanillaDoc
		if err := json.Unmarshal(vanilla, &d); err != nil {
			b.Fatal(err)
		}
	})
	run("decode-vanilla/v2-v1options", func() {
		var d vanillaDoc
		if err := jsonv2.Unmarshal(vanilla, &d, v1); err != nil {
			b.Fatal(err)
		}
	})
	run("decode-vanilla/v2", func() {
		var d vanillaDoc
		if err := jsonv2.Unmarshal(vanilla, &d); err != nil {
			b.Fatal(err)
		}
	})

	// the glossary file into its term list
	type term struct {
		Name       string   `json:"name"`
		Category   string   `json:"category"`
		Aliases    []string `json:"aliases"`
		Definition string   `json:"definition"`
		See        []string `json:"see"`
		Sources    []struct {
			RulebookPage int `json:"rulebookPage"`
		} `json:"sources"`
	}
	run("decode-glossary/v1", func() {
		var d struct {
			Terms []term `json:"terms"`
		}
		if err := json.Unmarshal(glossary, &d); err != nil {
			b.Fatal(err)
		}
	})
	run("decode-glossary/v2-v1options", func() {
		var d struct {
			Terms []term `json:"terms"`
		}
		if err := jsonv2.Unmarshal(glossary, &d, v1); err != nil {
			b.Fatal(err)
		}
	})
	run("decode-glossary/v2", func() {
		var d struct {
			Terms []term `json:"terms"`
		}
		if err := jsonv2.Unmarshal(glossary, &d); err != nil {
			b.Fatal(err)
		}
	})

	// the Battle Companies file into a generic tree (what the lean decoder builds with the token API)
	run("decode-bc-any/v1", func() {
		var d any
		if err := json.Unmarshal(bc, &d); err != nil {
			b.Fatal(err)
		}
	})
	run("decode-bc-any/v2", func() {
		var d any
		if err := jsonv2.Unmarshal(bc, &d); err != nil {
			b.Fatal(err)
		}
	})

	// the Tabletop Simulator payload of a real company
	rs := shippedRules(b)
	bcCo := ttsCompanies(rs)[0]
	var roster []CompanyUnit
	for _, e := range ttsEntries(rs, bcCo) {
		if len(roster) < 12 {
			u := ttsOne(rs, bcCo, e).Roster[0]
			u.ID = int64(len(roster) + 1)
			roster = append(roster, u)
		}
	}
	d := CompanyDetail{CampaignCompany: CampaignCompany{Name: bcCo.Name, Faction: bcCo.Name}, Roster: roster}
	ex := rs.TTSList(d, TTSOptions{FootVersions: true, MountFigures: true})
	payload := rs.TTSNames(d, ex, TTSNamesOptions{Rules: TTSRulesShort, Auras: rs.DefaultAuraSelection()})
	want, err := payload.JSON()
	if err != nil {
		b.Fatal(err)
	}
	encV2 := func(opts ...jsonv2.Options) []byte {
		out, err := jsonv2.Marshal(payload, opts...)
		if err != nil {
			b.Fatal(err)
		}
		return out
	}
	b.Logf("TTS payload %d bytes; v2 with v1 options identical: %v; v2 defaults identical: %v", len(want), bytes.Equal(want, encV2(v1)), bytes.Equal(want, encV2()))
	run("encode-tts/v1", func() {
		if _, err := payload.JSON(); err != nil {
			b.Fatal(err)
		}
	})
	run("encode-tts/v2-v1options", func() { encV2(v1) })
	run("encode-tts/v2", func() { encV2() })

	// small payloads: a campaign's settings and an event
	settings := CampaignSettings{HeroesDropProfileGear: true, ManualRolls: true}
	settingsJSON, _ := json.Marshal(settings)
	event := map[string]any{"type": "live:unit", "match": 42, "seq": 17, "unit": 9, "zone": "casualty", "text": "Beregond falls"}
	run("small-roundtrip/v1", func() {
		out, _ := json.Marshal(event)
		var s CampaignSettings
		_ = json.Unmarshal(settingsJSON, &s)
		_ = out
	})
	run("small-roundtrip/v2-v1options", func() {
		out, _ := jsonv2.Marshal(event, v1)
		var s CampaignSettings
		_ = jsonv2.Unmarshal(settingsJSON, &s, v1)
		_ = out
	})
}
