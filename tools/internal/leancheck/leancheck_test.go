package leancheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/conceptgraph"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/jsonx"
	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
)

// Tests of the checks (company texts, aura rings, rule targets, Tabletop Simulator models). The glossary nodes come from the app's loader.

var (
	raws  [3][]byte
	index *conceptgraph.Index
)

func TestMain(m *testing.M) {
	root := repo.Root()
	for i, p := range []string{"definitions/vanilla/data2024.json", "definitions/battle_companies/battle_companies.json", "definitions/glossary/mesbg_glossary.json"} {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			panic(err)
		}
		raws[i] = b
	}
	nodes, err := conceptgraph.MakeNodes(raws)
	if err != nil {
		panic(err)
	}
	index = conceptgraph.NewIndex(nodes)
	os.Exit(m.Run())
}

// fresh decodes the shipped files anew, so that a test may change them.
func fresh(t *testing.T) (bc, van jsonx.Obj) {
	t.Helper()
	b, err := jsonx.Decode(raws[1])
	if err != nil {
		t.Fatal(err)
	}
	v, err := jsonx.Decode(raws[0])
	if err != nil {
		t.Fatal(err)
	}
	return jsonx.O(b), jsonx.O(v)
}

func named(list any, name string) jsonx.Obj {
	for _, x := range jsonx.A(list) {
		if jsonx.S(jsonx.O(x)["name"]) == name {
			return jsonx.O(x)
		}
	}
	return nil
}

func ring(t *testing.T, bc jsonx.Obj, owner, label string) jsonx.Obj {
	t.Helper()
	for _, part := range []string{"rules", "gear"} {
		if x := named(bc[part], owner); x != nil {
			for _, a := range jsonx.A(x["auras"]) {
				if jsonx.S(jsonx.O(a)["label"]) == label {
					return jsonx.O(a)
				}
			}
		}
	}
	t.Fatalf("no ring %q of %q", label, owner)
	return nil
}

func anyContains(errs []string, parts ...string) bool {
	for _, e := range errs {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(e, p)
		}
		if ok {
			return true
		}
	}
	return false
}

func num(n int) json.Number { return jsonx.Number(itoa(n)) }

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// ---- company texts ----

func TestShippedCompanyTextsAreClean(t *testing.T) {
	bc, _ := fresh(t)
	if e := CompanyTextErrors(bc); len(e) != 0 {
		t.Fatalf("%v", e[:min(5, len(e))])
	}
}

func TestArnorEpigraphIsFlavourAndTheRuleIsWhole(t *testing.T) {
	bc, _ := fresh(t)
	a := named(bc["companies"], "Arnor")
	if q := jsonx.S(jsonx.O(a["epigraph"])["quote"]); q != "Leave all that can be spared behind. We travel light." {
		t.Fatal(q)
	}
	rules := jsonx.Strs(a["specialRules"])
	if len(rules) != 1 || !strings.HasPrefix(rules[0], "Should a Ranger of Arnor Warrior") || !strings.HasSuffix(rules[0], "Ranger of Arnor profile.") {
		t.Fatal(rules)
	}
}

func TestLegacyNamesAreTheVanillaUnits(t *testing.T) {
	bc, _ := fresh(t)
	text, _ := json.Marshal(bc["companies"])
	for _, w := range []string{"High Elf", "Giant Mirkwood", "Grimhammer", "Warrior of Erebor", "Iron Hills Dwarf", "Mirkwood Cavalry", "Abrakhân Guard", "Rhûnish"} {
		if strings.Contains(string(text), w) {
			t.Errorf("%q is still in the companies", w)
		}
	}
}

func TestGluedEpigraphIsFound(t *testing.T) {
	bc, _ := fresh(t)
	a := named(bc["companies"], "Arnor")
	a["specialRules"] = append(jsonx.Arr{`"Leave all that can be spared behind. We travel light." - As Arnor fell, the Dunedain grew wilder.`}, jsonx.A(a["specialRules"])...)
	if !anyContains(CompanyTextErrors(bc), "epigraph") {
		t.Fatal("a glued epigraph is not found")
	}
}

func TestRepeatedAndNearDuplicateRulesAreFound(t *testing.T) {
	bc, _ := fresh(t)
	f := named(bc["companies"], "Fiefdoms")
	f["specialRules"] = append(jsonx.A(f["specialRules"]), jsonx.A(f["specialRules"])[0])
	if !anyContains(CompanyTextErrors(bc), "repeat the same text") {
		t.Fatal("a repeated rule is not found")
	}
	bc, _ = fresh(t)
	f = named(bc["companies"], "Fiefdoms")
	f["specialRules"] = append(jsonx.A(f["specialRules"]), "Friendly Dol Amroth models treat the Leader as Prince Imrahil for the purposes of their Pride of Dol Amroth special rule")
	if !anyContains(CompanyTextErrors(bc), "repeat the same text") {
		t.Fatal("a near duplicate is not found")
	}
}

func TestATypedEntryMayRepeatItsRule(t *testing.T) {
	bc, _ := fresh(t)
	f := named(bc["companies"], "Fiefdoms")
	sub := jsonx.O(jsonx.A(f["substitutions"])[0])
	if sub["text"] != jsonx.A(f["specialRules"])[0] {
		t.Fatal("the first substitution no longer repeats the first rule")
	}
	if e := CompanyTextErrors(bc); len(e) != 0 {
		t.Fatal(e)
	}
}

func TestEmptyRuleAndRepeatedEpigraphAreFound(t *testing.T) {
	bc, _ := fresh(t)
	a := named(bc["companies"], "Arnor")
	a["specialRules"] = append(jsonx.A(a["specialRules"]), "  ")
	if !anyContains(CompanyTextErrors(bc), ": empty:") {
		t.Fatal("an empty rule is not found")
	}
	bc, _ = fresh(t)
	a = named(bc["companies"], "Arnor")
	a["specialRules"] = append(jsonx.A(a["specialRules"]), jsonx.S(jsonx.O(a["epigraph"])["text"])+" And more.")
	if !anyContains(CompanyTextErrors(bc), "epigraph is repeated") {
		t.Fatal("a repeated epigraph is not found")
	}
}

func TestFragments(t *testing.T) {
	for _, c := range []struct{ text, why string }{
		{`Models gain "Fearless`, "unbalanced"}, {"and then they fight.", "mid-sentence"}, {"This Hero may take the", "mid-sentence"},
		{"- As Arnor fell, they grew.", "mid-sentence"}, {"Fine rule. Battle Companies: 2nd Community Edition", "page header"},
	} {
		if !anyContains(Artefacts(c.text), c.why) {
			t.Errorf("%q: %v lacks %q", c.text, Artefacts(c.text), c.why)
		}
	}
	if a := Artefacts("A whole rule, with (brackets) and “quotes”."); len(a) != 0 {
		t.Fatal(a)
	}
}

// ---- aura rings ----

func auraErrors(bc, van jsonx.Obj) []string { return AuraErrors(bc, index.Texts, van) }

func TestShippedAuraTableIsClean(t *testing.T) {
	bc, van := fresh(t)
	if e := auraErrors(bc, van); len(e) != 0 {
		t.Fatal(e)
	}
}

func TestAuraVocabularyIsTheSpecifiedOne(t *testing.T) {
	bc, _ := fresh(t)
	a := jsonx.O(bc["auras"])
	for _, g := range jsonx.A(a["groups"]) {
		go_ := jsonx.O(g)
		if want, ok := AuraGroupDefaults[jsonx.S(go_["id"])]; !ok || go_["default"] != want {
			t.Errorf("group %v", go_)
		}
	}
	var kinds []string
	for _, k := range jsonx.A(a["kinds"]) {
		kinds = append(kinds, jsonx.S(jsonx.O(k)["id"]))
		if jsonx.O(k)["default"] != true {
			t.Errorf("kind %v is off by default", k)
		}
	}
	if strings.Join(kinds, ",") != strings.Join(AuraKinds, ",") {
		t.Fatal(kinds)
	}
}

func TestItemsOfTheSpec(t *testing.T) {
	bc, _ := fresh(t)
	for _, c := range []struct {
		owner, label string
		radius       int
		kind, when   string
	}{{"Banner", "", 3, "banner", "always"}, {"Company Standard", "", 3, "banner", "always"}, {"War Horn", "", 24, "command", "always"},
		{"War Horn", `War Horn: Stand Fast +3"`, 9, "command", "declared"}} {
		a := ring(t, bc, c.owner, c.label)
		if jsonx.Float(a["radius"]) != float64(c.radius) || a["kind"] != c.kind || a["when"] != c.when {
			t.Errorf("%s: %v", c.owner, a)
		}
	}
	if g := ring(t, bc, "War Horn", "")["group"]; g != "large" {
		t.Fatal(g)
	}
}

func TestVoiceOfCommandReplacesTheSixInchCommandRings(t *testing.T) {
	bc, _ := fresh(t)
	a := ring(t, bc, "Voice of Command", "")
	if jsonx.Float(a["radius"]) != 12 || a["kind"] != "command" {
		t.Fatal(a)
	}
	if !strings.Contains(jsonx.Repr(a["replaces"]), "Stand Fast") {
		t.Fatal(a["replaces"])
	}
	if got := jsonx.Strs(ring(t, bc, "Stand Fast", "")["mergeLabels"]); strings.Join(got, "|") != "With Me!|Loose" {
		t.Fatal(got)
	}
}

func TestAuraMutationsAreErrors(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(t *testing.T, bc jsonx.Obj)
		want   []string
	}{
		{"wrong radius", func(t *testing.T, bc jsonx.Obj) { ring(t, bc, "Furious Tirade", "")["radius"] = num(8) }, []string{"Furious Tirade", "not written"}},
		{"fractional radius", func(t *testing.T, bc jsonx.Obj) { ring(t, bc, "Furious Tirade", "")["radius"] = jsonx.Number("4.5") }, []string{"Furious Tirade", "not written"}},
		{"a sum is written in parts", func(t *testing.T, bc jsonx.Obj) {
			ring(t, bc, "War Horn", `War Horn: Stand Fast +3"`)["radius"] = num(10)
		}, []string{"War Horn"}},
		{"group must follow when", func(t *testing.T, bc jsonx.Obj) { ring(t, bc, "Gatekeeper", "")["group"] = "always" }, []string{"Gatekeeper", "group"}},
		{"group must follow radius", func(t *testing.T, bc jsonx.Obj) { ring(t, bc, "War Horn", "")["group"] = "always" }, []string{"War Horn", "'large'"}},
		{"changed group default", func(t *testing.T, bc jsonx.Obj) {
			named2(bc, "cast")["default"] = true
		}, []string{"groups must be exactly"}},
		{"kind off by default", func(t *testing.T, bc jsonx.Obj) {
			jsonx.O(jsonx.A(jsonx.O(bc["auras"])["kinds"])[0])["default"] = false
		}, []string{"every kind is on by default"}},
		{"kinds out of order", func(t *testing.T, bc jsonx.Obj) {
			k := jsonx.A(jsonx.O(bc["auras"])["kinds"])
			k[0], k[1] = k[1], k[0]
		}, []string{"auras.kinds must be"}},
		{"concept must resolve", func(t *testing.T, bc jsonx.Obj) { ring(t, bc, "Banner", "")["concept"] = "No Such Term" }, []string{"No Such Term"}},
		{"tip too long", func(t *testing.T, bc jsonx.Obj) { ring(t, bc, "Banner", "")["tip"] = strings.Repeat("x", 201) }, []string{"tip is longer"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			bc, van := fresh(t)
			c.change(t, bc)
			if e := auraErrors(bc, van); !anyContains(e, c.want...) {
				t.Fatalf("%v not in %v", c.want, e)
			}
		})
	}
}

func named2(bc jsonx.Obj, group string) jsonx.Obj {
	for _, g := range jsonx.A(jsonx.O(bc["auras"])["groups"]) {
		if jsonx.S(jsonx.O(g)["id"]) == group {
			return jsonx.O(g)
		}
	}
	return nil
}

func TestMissingAurasIsOneError(t *testing.T) {
	bc, van := fresh(t)
	delete(bc, "auras")
	if e := auraErrors(bc, van); len(e) != 1 || e[0] != "auras is missing" {
		t.Fatal(e)
	}
}

func TestKeenSightOfTheFalconAgreesWithKeenSight(t *testing.T) {
	bc, _ := fresh(t)
	if jsonx.Float(ring(t, bc, "Keen Sight", "")["radius"]) != 12 || jsonx.Float(ring(t, bc, "Keen Sight (Falcon)", "")["radius"]) != 12 {
		t.Fatal("the Keen Sight rings are not 12 inches")
	}
}

func TestHelpers(t *testing.T) {
	for _, c := range []struct {
		n, text string
		want    bool
	}{{"3", `within 3" of`, true}, {"3", `within 13" of`, false}, {"3", `within 2.3" of`, false}, {"12", "12” away", true}, {"3", `3" first`, true}} {
		if Has(c.n, c.text) != c.want {
			t.Errorf("Has(%s, %q) != %v", c.n, c.text, c.want)
		}
	}
	for _, c := range []struct {
		when   string
		radius int
		want   string
	}{{"always", 24, "large"}, {"cast", 24, "cast"}, {"declared", 12, "declared"}} {
		if got := GroupOf(jsonx.Obj{"when": c.when, "radius": num(c.radius)}); got != c.want {
			t.Errorf("GroupOf(%s, %d) = %s", c.when, c.radius, got)
		}
	}
	if Fold("Abrakhân’s Guard") != "abrakhan s guard" || Norm("Hello, World!") != "hello world" {
		t.Fatal(Fold("Abrakhân’s Guard"), Norm("Hello, World!"))
	}
}

// ---- rule targets ----

func TestShippedRuleTargetsAreClean(t *testing.T) {
	bc, van := fresh(t)
	if e := RuleTargetErrors(bc, van); len(e) != 0 {
		t.Fatal(e)
	}
}

func TestTheFamiliesWithAKeywordParameter(t *testing.T) {
	bc, _ := fresh(t)
	want := map[string]string{"Hatred": "enemy-keyword", "Ancient Enemies": "enemy-keyword", "Terror": "enemy-keyword", "Fury": "friendly-keyword"}
	for n, applies := range want {
		r := named(bc["rules"], n)
		if jsonx.O(r["target"])["appliesTo"] != applies || jsonx.O(r["parameter"])["kind"] != "keyword" {
			t.Errorf("%s: %v", n, r)
		}
	}
	if jsonx.O(named(bc["rules"], "Terror")["bareTarget"])["appliesTo"] != "enemy-any" {
		t.Fatal("Terror's bare target")
	}
}

func TestTextOnlyTargetsHaveNoKeywords(t *testing.T) {
	// Lordly Quality (the bearer's own faction keyword) and Defender (the opposite species keyword) depend on the bearer
	bc, _ := fresh(t)
	for _, n := range []string{"Lordly Quality", "Defender"} {
		target := jsonx.O(named(bc["rules"], n)["target"])
		if _, has := target["keywords"]; target["appliesTo"] != "friendly-keyword" || has || jsonx.S(target["text"]) == "" {
			t.Errorf("%s: %v", n, target)
		}
	}
}

func TestUnknownKeywordIsAnError(t *testing.T) {
	bc, van := fresh(t)
	r := named(bc["rules"], "Gatekeeper")
	if r["target"] == nil {
		r["target"] = jsonx.Obj{}
	}
	jsonx.O(r["target"])["keywords"] = jsonx.Arr{"Infantri"}
	if e := RuleTargetErrors(bc, van); !anyContains(e, "Infantri") {
		t.Fatal(e)
	}
	jsonx.O(r["target"])["keywords"] = jsonx.Arr{"Infantry"}
	if e := RuleTargetErrors(bc, van); len(e) != 0 {
		t.Fatal(e)
	}
}

// ---- Tabletop Simulator ----

func TestStandInsOfTheSpec(t *testing.T) {
	bc, _ := fresh(t)
	got := map[string]string{}
	for _, g := range jsonx.A(bc["gear"]) {
		if tts := jsonx.O(jsonx.O(g)["tts"]); tts != nil {
			got[jsonx.S(jsonx.O(g)["name"])] = jsonx.S(tts["model"])
		}
	}
	want := map[string]string{"Fell Warg": "Fell Warg", "Warg": "Fell Warg", "Baggage Pony": "Bill the Pony", "Falcon": "Crebain", "Hunting Dog": "Fang",
		"Wandering Swordsman": "Captain of Dale", "Wandering Marksman": "Ruffian: Bow", "Wandering Scavenger": "Goblin Mercenary Captain", "Wandering Sage": "Malbeth the Seer"}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q, want %q", k, got[k], v)
		}
	}
}

func TestNoCatalogueNoCheck(t *testing.T) {
	bc, _ := fresh(t)
	if e := TTSErrors(bc, nil); e != nil {
		t.Fatal(e)
	}
}

func TestLocalCatalogue(t *testing.T) {
	bc, _ := fresh(t)
	cat := Catalogue{{"captain of dale", ""}: true, {"ruffian", "bow"}: true}
	e := strings.Join(TTSErrors(bc, cat), "\n")
	if !strings.Contains(e, "Malbeth the Seer") || strings.Contains(e, "Captain of Dale") || strings.Contains(e, "Ruffian: Bow") {
		t.Fatal(e)
	}
}

func TestCatalogueFileIsOptional(t *testing.T) {
	dir := t.TempDir()
	if c, err := TTSCatalogue(filepath.Join(dir, "models.json")); c != nil || err != nil {
		t.Fatal(c, err)
	}
	p := filepath.Join(dir, "models.json")
	if err := os.WriteFile(p, []byte(`["Ruffian: Bow", "Fang:"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := TTSCatalogue(p)
	if err != nil || len(c) != 2 || !c[[2]string{"ruffian", "bow"}] || !c[[2]string{"fang", ""}] {
		t.Fatal(c, err)
	}
	if err := os.WriteFile(p, []byte(`{"models": ["Fang:"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if c, err := TTSCatalogue(p); err != nil || len(c) != 1 {
		t.Fatal(c, err)
	}
}
