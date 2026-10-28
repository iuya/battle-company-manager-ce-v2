package core

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The test-only converter oldToLean and the equivalence registry (docs/definitions-lean-design.md section 11, slice 1).
//
// oldToLean turns the shipped old-format Battle Companies file into a schemaVersion 2 document, one section per slice. It is
// never shipped: the curator's generator replaces it as the producer of the real file. Slice 1 converts the header and `campaign`
// with the values the engine uses today (the section 12 parameters that today live in Go constants are written here, once).
//
// To add a section in a later slice: (1) extend oldToLean with the section (a func that returns its lean JSON from the old
// document), (2) add the section's entry to leanEquivalenceSections with a check that the typed value read from the lean document
// equals what the old parser gives, (3) build the consumer from RuleSet.Doc and let the golden snapshot (snapshot_test.go,
// byte-identical) prove the pages did not change. A section whose slice has not come yet stays a t.Skip("slice N").

type obj = map[string]any

func decodeOld(t *testing.T, raw []byte) obj {
	t.Helper()
	var old obj
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	return old
}

func get(o obj, path ...string) any {
	var cur any = o
	for _, p := range path {
		m, ok := cur.(obj)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

func pick(src obj, keys ...string) obj {
	out := obj{}
	for _, k := range keys {
		if v, ok := src[k]; ok {
			out[k] = v
		}
	}
	return out
}

// oldToLean converts header and campaign; sections of later slices are added below as their slices land.
func oldToLean(t *testing.T, raw []byte) []byte {
	t.Helper()
	old := decodeOld(t, raw)
	doc := obj{
		"schemaVersion": 2,
		"name":          old["name"],
		"version":       old["version"],
		"extends":       old["extends"],
		"credit":        old["credit"],
		"campaign":      convertCampaign(t, old),
		"scenarios":     convertScenarios(t, old),
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// convertScenarios turns data.scenarioDetails into the lean scenarios[] list (design 4.7, 6). A scoring objective keeps one
// sentence per tier, once, as options[].text, in the order the sentences are printed (the file lists them so; the loader sorts the
// tiers by points); the instruction of a secret objective is its own field and is not repeated as an option; a "for each"
// objective carries its single sentence as one option worth pointsEach, with count = maxCount. maxPoints, reveal and completion
// are derived (leanMaxPoints; every secret objective reveals at the end and completes by tier). What the lean model has no field
// for is listed in leanGaps (none).
func convertScenarios(t *testing.T, old obj) []any {
	t.Helper()
	var out []any
	list, _ := get(old, "data", "scenarioDetails").([]any)
	for _, raw := range list {
		d := raw.(obj)
		sc := pick(d, "name", "description", "outline", "usesRules", "armies", "layout", "startingPositions", "objectives", "specialRules", "roles", "rewards")
		objs, _ := d["scoringObjectives"].([]any)
		var scoring []any
		for _, o := range objs {
			ob := o.(obj)
			row := pick(ob, "key", "side", "kind", "item", "instruction", "secret", "nomination")
			if c, ok := ob["count"]; ok {
				row["count"] = c
			} else if c, ok := ob["maxCount"]; ok {
				row["count"] = c
			}
			var options []any
			for _, x := range ob["options"].([]any) {
				options = append(options, pick(x.(obj), "text", "points"))
			}
			row["options"] = options
			scoring = append(scoring, row)
		}
		if scoring != nil {
			sc["scoring"] = scoring
		}
		out = append(out, sc)
	}
	return out
}

// leanGaps are the old scenario keys the lean model has no field for: none, TestLeanScenariosGaps proves nothing is left out.
var leanGaps []string

// leanMaxPoints is how the best a player can score on a lean objective is derived (the old file stores maxPoints).
func leanMaxPoints(kind string, count int, options []lean.ScoringOption) int {
	best := 0
	for _, o := range options {
		best = max(best, o.Points)
	}
	switch kind {
	case "eachTiers", "each":
		return count * best
	}
	return best
}

func convertCampaign(t *testing.T, old obj) obj {
	c, _ := old["campaign"].(obj)
	if c == nil {
		t.Fatal("the old file has no campaign")
	}
	rows := func(list any, keys ...string) []any {
		var out []any
		for _, r := range list.([]any) {
			out = append(out, pick(r.(obj), keys...))
		}
		return out
	}
	injuryRows := func(list any) []any {
		var out []any
		for _, r := range list.([]any) {
			row := pick(r.(obj), "roll", "key", "name", "text", "grantsExperience")
			if sc, ok := r.(obj)["statChanges"].([]any); ok {
				var changes []any
				for _, ch := range sc {
					m := ch.(obj)
					changes = append(changes, obj{"stat": m["stat"], "amount": m["delta"]})
				}
				row["statChanges"] = changes
			}
			out = append(out, row)
		}
		return out
	}
	company := c["company"].(obj)
	progression := c["progression"].(obj)
	warrior := progression["warrior"].(obj)
	hero := progression["hero"].(obj)
	adjust := get(c, "recruitment", "adjust").(obj)
	rare := get(c, "recruitment", "rare").(obj)
	var effects []any
	for _, e := range c["aftermathEffects"].([]any) {
		effects = append(effects, pick(e.(obj), "key", "range"))
	}
	var gains []any
	for _, g := range get(c, "experience", "gains").([]any) {
		gains = append(gains, pick(g.(obj), "key", "label", "amount", "repeatable"))
	}
	bp := c["breakPoint"].(obj)
	return obj{
		"company": obj{
			"maxModels": company["maxModels"], "leaders": company["leaders"], "sergeants": company["sergeants"],
			"heroStart":         company["startingHeroStats"],
			"startingInfluence": 0,                           // the shipped old file has no such key: a new company starts with none
			"equipmentSlots":    obj{"small": 1, "large": 1}, // the slots every model has today (docs/engine-parameters-audit.md B6)
		},
		"recruitment": obj{
			"die":    "d6",
			"adjust": obj{"maxPerAttempt": adjust["maxPerAttempt"], "direction": "up", "costPerPoint": adjust["costPerPoint"]},
			"rare":   obj{"injuredCount": rare["injuredUnitsCount"], "deadCount": rare["deadUnitsCount"], "promotedCount": rare["promotedUnitsCount"]},
		},
		"influence": obj{"gains": get(c, "influence", "gains")},
		"progression": obj{
			"threshold": progression["threshold"],
			"warrior": obj{"roll": "d6", "table": rows(warrior["table"], "roll", "key", "name", "text"),
				"noPromotion": warrior["noPromotion"]},
			"hero": obj{"roll": "2d6", "rollsPerProgression": hero["rollsPerProgression"], "choose": hero["choose"]},
		},
		"injuries": obj{
			"roll":    "2d6",
			"warrior": injuryRows(get(c, "injuries", "warrior")),
			"hero":    injuryRows(get(c, "injuries", "hero")),
		},
		"experience": obj{"gains": gains},
		"breakPoint": obj{
			"default":    pick(get(bp, "default").(obj), "percent", "of"),
			"threshold":  obj{"comparison": "gt"}, // the old prose value strictlyGreater
			"whenBroken": obj{"courageTest": obj{"dice": "2d6"}},
		},
		"rating": obj{"hero": obj{"wargear": pick(get(c, "rating", "hero", "wargear").(obj), "threshold")}},
		"scenarioSelection": obj{
			"die":    "d6",
			"tables": rows(get(c, "scenarioSelection", "tables"), "roll", "scenarios"),
		},
		"aftermathEffects": effects,
		"gold":             c["gold"],
		"matchInput":       c["matchInput"],
		"options":          c["options"],
	}
}

// leanEquivalenceSections is the registry: one entry per section of the old file. check receives the old document and the lean
// document read from oldToLean's output and fails when the lean value differs from what the old parser gives today.
var leanEquivalenceSections = []struct {
	name  string
	skip  string // "slice N": the section is converted by that slice
	check func(t *testing.T, old obj, raw []byte, rs *RuleSet)
}{
	{name: "header and campaign", check: checkHeaderAndCampaign},
	{name: "scenarios", check: checkScenarios},
	{name: "rules (keywords, minor and parameterised rules, targets, auras)", skip: "slice 4"},
	{name: "gear (wargear, equipment, creatures, wanderers, annotations)", skip: "slice 5"},
	{name: "paths", skip: "slice 6"},
	{name: "companies", skip: "slice 7"},
	{name: "tts and name resolution", skip: "slice 8"},
}

func TestLeanEquivalence(t *testing.T) {
	raw, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Fatal(err)
	}
	old := decodeOld(t, raw)
	rs := shippedRules(t)
	lean2 := oldToLean(t, raw)
	for _, s := range leanEquivalenceSections {
		t.Run(s.name, func(t *testing.T) {
			if s.skip != "" {
				t.Skip(s.skip)
			}
			s.check(t, old, lean2, rs)
		})
	}
}

func checkHeaderAndCampaign(t *testing.T, old obj, converted []byte, rs *RuleSet) {
	book, gl := v2World(t)
	doc, rep := DecodeBattleCompanies("converted.json", converted, book, gl)
	if doc == nil || len(rep.Issues) != 0 {
		t.Fatalf("the converted shipped campaign must decode with no issue:\n%s", rep)
	}
	if doc.SchemaVersion != 2 || doc.Name != old["name"] || string(doc.Version) != old["version"] || doc.Extends != old["extends"] {
		t.Errorf("header: %+v", doc)
	}
	c := doc.Campaign
	// the typed parameters equal what the engine uses today
	if got := c.Recruitment.Adjust.MaxPerAttempt; got != rs.BC.Recruiting().Adjust.Max {
		t.Errorf("adjust.maxPerAttempt = %d, the engine reads %d", got, rs.BC.Recruiting().Adjust.Max)
	}
	if got := c.Company.StartingInfluence; got != rs.BC.Recruiting().StartingInfluence {
		t.Errorf("startingInfluence = %d", got)
	}
	hb := rs.BC.HeroBonus(nil)
	if h := c.Company.HeroStart; h.Might != hb["might"] || h.Will != hb["will"] || h.Fate != hb["fate"] {
		t.Errorf("heroStart = %+v, the engine's HeroBonus is %v", h, hb)
	}
	if rules := rs.BC.Progression(); rules.Threshold != c.Progression.Threshold {
		t.Errorf("progression threshold: lean %d, old parser %d", c.Progression.Threshold, rules.Threshold)
	}
	if c.Injuries.Roll.String() != "2d6" || c.Recruitment.Die.String() != "d6" || c.Progression.Hero.Roll.String() != "2d6" {
		t.Errorf("dice: %v %v %v", c.Injuries.Roll, c.Recruitment.Die, c.Progression.Hero.Roll)
	}
	if len(c.ScenarioSelection.Tables) != 3 || len(c.Injuries.Warrior) != len(get(old, "campaign", "injuries", "warrior").([]any)) {
		t.Errorf("tables: %d scenario tables, %d warrior injury rows", len(c.ScenarioSelection.Tables), len(c.Injuries.Warrior))
	}
}

func TestOldToLeanIsStable(t *testing.T) {
	raw, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Fatal(err)
	}
	a, b := oldToLean(t, raw), oldToLean(t, raw)
	if string(a) != string(b) {
		t.Error("the converter must be deterministic")
	}
}

// checkScenarios proves the scenarios section: the converted document decodes with no issue, every field the app reads
// round-trips (the text of each scenario, the objectives with their tiers, maxPoints derived), the scoring line composed from the
// lean options equals the old text byte for byte (all three old forms), and each tier sentence is stored once.
func checkScenarios(t *testing.T, old obj, converted []byte, rs *RuleSet) {
	book, gl := v2World(t)
	doc, rep := DecodeBattleCompanies("converted.json", converted, book, gl)
	if doc == nil || len(rep.Issues) != 0 {
		t.Fatalf("the converted scenarios must decode with no issue:\n%s", rep)
	}
	details := get(old, "data", "scenarioDetails").([]any)
	if len(doc.Scenarios) != len(details) || len(doc.Scenarios) != len(old["scenarios"].([]any)) {
		t.Fatalf("%d lean scenarios, %d details, %d names", len(doc.Scenarios), len(details), len(old["scenarios"].([]any)))
	}
	for i, sc := range doc.Scenarios {
		d := details[i].(obj)
		if sc.Name != old["scenarios"].([]any)[i] || sc.Name != d["name"] {
			t.Errorf("scenario %d: %q", i, sc.Name)
		}
		for field, got := range map[string]string{"description": sc.Description, "armies": sc.Armies, "layout": sc.Layout, "startingPositions": sc.StartingPositions, "objectives": sc.Objectives, "outline": sc.Outline} {
			if want, _ := d[field].(string); got != want {
				t.Errorf("%s.%s differs", sc.Name, field)
			}
		}
		for field, got := range map[string]any{"usesRules": sc.UsesRules, "specialRules": sc.SpecialRules, "roles": sc.Roles, "rewards": sc.Rewards} {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(d[field])
			if string(a) != string(b) {
				t.Errorf("%s.%s differs: %s vs %s", sc.Name, field, a, b)
			}
		}
		objs, _ := d["scoringObjectives"].([]any)
		if len(sc.Scoring) != len(objs) {
			t.Errorf("%s: %d lean objectives, %d old objectives", sc.Name, len(sc.Scoring), len(objs))
			continue
		}
		var all []string
		for j, sco := range sc.Scoring {
			ob := objs[j].(obj)
			var gotNom any
			if sco.Nomination != nil {
				n := obj{"from": sco.Nomination.From}
				if sco.Nomination.AllowLeader != "" {
					n["allowLeader"] = sco.Nomination.AllowLeader
				}
				if sco.Nomination.Where != "" {
					n["where"] = sco.Nomination.Where
				}
				gotNom = n
			}
			if !reflect.DeepEqual(gotNom, ob["nomination"]) {
				t.Errorf("%s/%s: nomination %v, old %v", sc.Name, sco.Key, gotNom, ob["nomination"])
			}
			var sentences []string
			for _, o := range sco.Options {
				sentences = append(sentences, o.Text)
			}
			line := ComposeObjectiveText(sco.Instruction, sentences)
			if line != ob["text"] {
				t.Errorf("%s/%s: composed %q, objective text %q", sc.Name, sco.Key, line, ob["text"])
			}
			item, _ := ob["item"].(string)
			if sco.Key != ob["key"] || sco.Kind != ob["kind"] || sco.Item != item || sco.Secret != (ob["secret"] == true) {
				t.Errorf("%s/%s: key/kind/item/secret differ: %+v", sc.Name, sco.Key, sco)
			}
			if sco.Side != ob["side"] { // the default is applied by Normalise
				t.Errorf("%s/%s: side %q, old %v", sc.Name, sco.Key, sco.Side, ob["side"])
			}
			if got := leanMaxPoints(sco.Kind, sco.Count, sco.Options); got != int(ob["maxPoints"].(float64)) {
				t.Errorf("%s/%s: derived maxPoints %d, old %v", sc.Name, sco.Key, got, ob["maxPoints"])
			}
			if ob["secret"] == true && (ob["reveal"] != "endOfGame" || ob["completion"] != "tiers") {
				t.Errorf("%s/%s: a secret objective that does not reveal at the end by tiers is no longer derivable", sc.Name, sco.Key)
			}
			if _, ok := ob["pointsEach"]; ok { // a "for each" objective: count = maxCount, the option is worth pointsEach
				if len(sco.Options) != 1 || float64(sco.Options[0].Points) != ob["pointsEach"] || float64(sco.Count) != ob["maxCount"] {
					t.Errorf("%s/%s: for-each %+v vs %v x %v", sc.Name, sco.Key, sco, ob["pointsEach"], ob["maxCount"])
				}
			}
			all = append(all, sentences...)
		}
		// each tier sentence is stored once in the scenario
		raw, _ := json.Marshal(sc)
		for _, s := range all {
			q, _ := json.Marshal(s)
			if n := strings.Count(string(raw), string(q)); n != 1 {
				t.Errorf("%s: sentence %q is stored %d times", sc.Name, s, n)
			}
		}
		// the app's view of the old file equals what the lean document gives: tiers ascending by points, one line per objective
		info, ok := rs.BC.Scenario(sc.Name)
		if !ok || len(info.ScoringObjectives) != len(sc.Scoring) || !slices.Equal(info.Scoring, composedLines(sc)) {
			t.Errorf("%s: the loader's scoring lines differ from the lean ones", sc.Name)
			continue
		}
		for j, so := range info.ScoringObjectives {
			var tiers []ScoringTier
			{
				for _, o := range sc.Scoring[j].Options {
					tiers = append(tiers, ScoringTier{Text: o.Text, Points: o.Points})
				}
				slices.SortStableFunc(tiers, func(a, b ScoringTier) int { return a.Points - b.Points })
			}
			if !slices.Equal(so.Options, tiers) {
				t.Errorf("%s/%s: tiers %v, lean %v", sc.Name, so.Key, so.Options, tiers)
			}
		}
	}
}

func composedLines(sc lean.Scenario) []string {
	var out []string
	for _, o := range sc.Scoring {
		var sentences []string
		for _, x := range o.Options {
			sentences = append(sentences, x.Text)
		}
		out = append(out, ComposeObjectiveText(o.Instruction, sentences))
	}
	return out
}

// TestLeanScenariosGaps pins what the lean model cannot hold yet, so a silent loss shows up here.
func TestLeanScenariosGaps(t *testing.T) {
	raw, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Fatal(err)
	}
	old := decodeOld(t, raw)
	found := map[string]bool{}
	for _, d := range get(old, "data", "scenarioDetails").([]any) {
		for k := range d.(obj) {
			switch k {
			case "name", "description", "outline", "usesRules", "armies", "layout", "startingPositions", "objectives", "specialRules", "roles", "rewards", "scoringObjectives":
			default:
				found["scenarioDetails[]."+k] = true
			}
		}
		objs, _ := d.(obj)["scoringObjectives"].([]any)
		for _, o := range objs {
			for k := range o.(obj) {
				switch k {
				case "key", "side", "kind", "item", "instruction", "secret", "options", "count", "text", "maxPoints", "pointsEach", "maxCount", "reveal", "completion", "nomination":
				default:
					found["scoringObjectives[]."+k] = true
				}
			}
		}
	}
	var got []string
	for k := range found {
		got = append(got, k)
	}
	slices.Sort(got)
	want := slices.Clone(leanGaps)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("old scenario keys without a lean field: %v, expected %v", got, want)
	}
}

// TestParseScenariosComposesTheOldText: the scoring text the match page shows is composed from the typed objectives and equals
// the objective text of the file byte for byte, for every scenario.
func TestParseScenariosComposesTheOldText(t *testing.T) {
	raw, err := os.ReadFile("../../definitions/battle_companies/battle_companies.json")
	if err != nil {
		t.Fatal(err)
	}
	old := decodeOld(t, raw)
	list, err := ParseScenarios(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 18 {
		t.Fatalf("%d scenarios", len(list))
	}
	want := map[string][]string{} // the objective text of the file, per scenario in the order of its objectives
	for _, d := range get(old, "data", "scenarioDetails").([]any) {
		for _, o := range d.(obj)["scoringObjectives"].([]any) {
			want[d.(obj)["name"].(string)] = append(want[d.(obj)["name"].(string)], o.(obj)["text"].(string))
		}
	}
	for _, info := range list {
		if !slices.Equal(info.Scoring, want[info.Name]) {
			t.Errorf("%s: composed %q, old %q", info.Name, info.Scoring, want[info.Name])
		}
		for i, o := range info.ScoringObjectives {
			if o.Text != want[info.Name][i] {
				t.Errorf("%s/%s: objective text %q, old %q", info.Name, o.Key, o.Text, want[info.Name][i])
			}
		}
	}
}

func TestComposeObjectiveText(t *testing.T) {
	cases := []struct {
		name, instruction string
		sentences         []string
		want              string
	}{
		{"tiers joined by a space", "", []string{"A.", "B."}, "A. B."},
		{"instruction first", "Note one down.", []string{"A."}, "Note one down. A."},
		{"the order given is kept", "", []string{"B.", "A."}, "B. A."},
		{"nothing", "", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ComposeObjectiveText(c.instruction, c.sentences); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
