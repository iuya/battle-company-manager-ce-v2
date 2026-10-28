package lean

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func normalised(t *testing.T, src string) (*Document, Report) {
	t.Helper()
	var doc Document
	var rep Report
	if !Bind("f.json", []byte(src), &doc, &rep) {
		t.Fatalf("not usable: %s", rep)
	}
	if !rep.OK() {
		t.Fatalf("bind errors:\n%s", rep)
	}
	var nrep Report
	Normalise(&doc, NewReporter("f.json", &nrep))
	return &doc, nrep
}

// minimalWith splices extra top-level sections into the minimal fixture.
func minimalWith(t *testing.T, extra string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/valid/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	var have map[string]json.RawMessage
	var add map[string]json.RawMessage
	if err := json.Unmarshal(raw, &have); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte("{"+extra+"}"), &add); err != nil {
		t.Fatalf("bad test json: %v", err)
	}
	for k, v := range add {
		have[k] = v
	}
	out, _ := json.Marshal(have)
	return string(out)
}

func TestDefaults(t *testing.T) {
	doc, rep := normalised(t, minimalWith(t, `
"companies":[{"name":"Lake-Town Guard","alignment":"good","description":"d",
  "start":{"gold":1,"units":[{"unit":"A"},{"unit":"B","count":3}]},
  "reinforcements":{"influence":1,"rows":[{"roll":"1-3","unit":"A"},{"roll":"4-6","unit":"B","count":2,"rare":1}]},
  "special":{"rows":[{"roll":"1-6","unit":"A"}]},
  "ownPath":{"units":["A"]},
  "units":[{"name":"Local","extends":"Base","ownPath":{"units":["x"]}}],
  "gear":[{"name":"Sling","rating":3}]}],
"units":[{"extends":"Ranger of Gondor","tts":{"foot":"x"}}],
"gear":[{"name":"Bow","rating":{"warrior":1,"hero":5}},{"name":"Fine Bow","rating":2,"gold":7}],
"rules":[{"name":"Hatred","target":{"keywords":["Orc"]}},{"name":"Plain"}],
"paths":[{"name":"Path of Test","rows":[{"roll":2,"options":[{"rule":{"name":"Inline","target":{}}}]}]}],
"scenarios":[{"name":"Rescue","scoring":[{"key":"k"}]}]`))
	if len(rep.Issues) != 0 {
		t.Fatal(rep)
	}
	c := doc.Companies[0]
	checks := []struct {
		name      string
		got, want any
	}{
		{"company id", c.ID, "lake-town-guard"},
		{"faction", c.Faction, "Lake-Town Guard"},
		{"maxModels from campaign", c.MaxModels, 15},
		{"start count default", c.Start.Units[0].Count, 1},
		{"start count kept", c.Start.Units[1].Count, 3},
		{"chart count default", c.Reinforcements.Rows[0].Count, 1},
		{"chart count kept", c.Reinforcements.Rows[1].Count, 2},
		{"rare kept", c.Reinforcements.Rows[1].Rare, 1},
		{"rare default", c.Reinforcements.Rows[0].Rare, 0},
		{"special count", c.Special.Rows[0].Count, 1},
		{"company ownPath", c.OwnPath.Path, "On Their Own Path"},
		{"local unit ownPath", c.Units[0].OwnPath.Path, "On Their Own Path"},
		{"local unit id", c.Units[0].ID, "local"},
		{"local gear gold", *c.Gear[0].Gold, Rating{3, 3}},
		{"annotation name", doc.Units[0].Name, "Ranger of Gondor"},
		{"annotation id", doc.Units[0].ID, "ranger-of-gondor"},
		{"gold = rating", *doc.Gear[0].Gold, Rating{1, 5}},
		{"gold kept", *doc.Gear[1].Gold, Rating{7, 7}},
		{"rule target appliesTo", doc.Rules[0].Target.AppliesTo, "own"},
		{"rule target match", doc.Rules[0].Target.Match, "any"},
		{"rule without target stays nil", doc.Rules[1].Target == nil, true},
		{"path kind", doc.Paths[0].Kind, "heroic"},
		{"path dice from hero roll", doc.Paths[0].Dice, DiceSpec{2, 6}},
		{"inline rule defaults", doc.Paths[0].Rows[0].Options[0].Rule.Def.Target.AppliesTo, "own"},
		{"scenario side", doc.Scenarios[0].Side, "both"},
		{"scoring side", doc.Scenarios[0].Scoring[0].Side, "both"},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestExtends(t *testing.T) {
	doc, rep := normalised(t, minimalWith(t, `
"companies":[
 {"name":"Elite Arnor","extends":"Arnor","alignment":"good","description":"elite","maxModels":12},
 {"name":"Arnor","alignment":"good","description":"base","faction":"Arnor Realm","oath":{"text":"base oath"},
  "start":{"gold":4,"units":[{"unit":"Ranger","count":3}]},
  "reinforcements":{"influence":3,"rows":[{"roll":"1-6","unit":"Ranger"}]},"notes":["base note"],
  "heroUpgrades":[{"name":"Foresight"}]},
 {"name":"Third","extends":"Elite Arnor","alignment":"evil","description":"t","start":{"gold":9,"units":[{"unit":"Other"}]}}],
"units":[{"name":"Ranger of Ithilien","extends":"Ranger of Gondor","addRules":["Loyal"]},
         {"name":"Scout of Ithilien","extends":"Ranger of Ithilien","with":["Bow"],"note":"mine"}],
"gear":[{"name":"Dunharrow Spear","extends":"Spear","influence":1,"definition":"mine"},
        {"name":"Spear","kind":"wargear","definition":"base text","influence":2,"rating":4,"classes":["melee","spear"],"tts":{"name":"Spear model"}},
        {"name":"Fine Dunharrow","extends":"Dunharrow Spear","rating":9}]`))
	if len(rep.Issues) != 0 {
		t.Fatal(rep)
	}
	elite := doc.Companies[0]
	if elite.MaxModels != 12 || elite.Start == nil || elite.Start.Gold != 4 || elite.Reinforcements.Influence != 3 || elite.Oath.Text != "base oath" ||
		len(elite.HeroUpgrades) != 1 || elite.Description != "elite" {
		t.Errorf("child inherits the base and overrides what it writes: %+v", elite)
	}
	if elite.Faction != "Elite Arnor" || len(elite.Notes) != 0 {
		t.Errorf("faction and notes are not inherited: faction %q notes %v", elite.Faction, elite.Notes)
	}
	third := doc.Companies[2]
	if third.Start.Gold != 9 || len(third.Start.Units) != 1 || third.Start.Units[0].Unit != "Other" || third.MaxModels != 12 || third.Reinforcements == nil {
		t.Errorf("lists replace, the rest chains through two bases: %+v", third)
	}
	elite.Start.Units[0].Unit = "Changed"
	if doc.Companies[1].Start.Units[0].Unit != "Ranger" {
		t.Error("the child shares memory with its base")
	}
	if got := doc.Units[1]; got.AddRules[0] != "Loyal" || got.With[0] != "Bow" || got.Note != "mine" || got.Extends != "Ranger of Ithilien" {
		t.Errorf("unit chain: %+v", got)
	}
	d := doc.Gear[0]
	if d.Kind != "wargear" || d.Rating != (Rating{4, 4}) || d.Influence != 1 || d.Definition != "mine" || len(d.Classes) != 2 || d.TTS != nil {
		t.Errorf("gear inherits all but name, definition and tts: %+v", d)
	}
	if doc.Gear[2].Rating != (Rating{9, 9}) || doc.Gear[2].Definition != "" || doc.Gear[2].Influence != 1 || doc.Gear[2].Kind != "wargear" {
		t.Errorf("gear chain: %+v", doc.Gear[2])
	}
	if *doc.Gear[0].Gold != (Rating{4, 4}) || *doc.Gear[2].Gold != (Rating{9, 9}) {
		t.Errorf("gold defaults after extends: %v %v", *doc.Gear[0].Gold, *doc.Gear[2].Gold)
	}
}

func TestExtendsScopes(t *testing.T) {
	doc, rep := normalised(t, minimalWith(t, `
"units":[{"name":"Shared Scout","extends":"Ranger of Gondor","addRules":["Shared rule"],"with":["Bow"]}],
"gear":[{"name":"House Spear","extends":"Spear","influence":3}, {"name":"Spear","influence":1,"rating":2}],
"companies":[{"name":"C","alignment":"good","description":"d",
  "units":[{"name":"Local Scout","extends":"Shared Scout","note":"n"},{"name":"Annotated","extends":"Local Scout","with":["x"]}],
  "gear":[{"name":"Banner of C","extends":"House Spear","limit":1}]}]`))
	if len(rep.Issues) != 0 {
		t.Fatal(rep)
	}
	c := doc.Companies[0]
	if c.Units[0].AddRules[0] != "Shared rule" || c.Units[1].AddRules[0] != "Shared rule" || c.Units[1].With[0] != "x" {
		t.Errorf("company units extend shared units and each other: %+v", c.Units)
	}
	if c.Gear[0].Influence != 3 || c.Gear[0].Rating != (Rating{2, 2}) {
		t.Errorf("company gear extends shared gear whose own base is the file's Spear: %+v", c.Gear[0])
	}
}

func TestExtendsAnnotationNeverExtendsItself(t *testing.T) {
	doc, rep := normalised(t, minimalWith(t, `"units":[{"name":"Ranger of Gondor","extends":"Ranger of Gondor","tts":{"foot":"x"}},{"extends":"Ranger of Gondor","addRules":["R"]}]`))
	if len(rep.Issues) != 0 {
		t.Fatalf("a name equal to its base is an annotation, not a loop:\n%s", rep)
	}
	if doc.Units[0].Name != "Ranger of Gondor" || doc.Units[1].Name != "Ranger of Gondor" {
		t.Error("names")
	}
}

func TestExtendsErrors(t *testing.T) {
	for _, c := range []struct {
		name, extra string
		want        []string
	}{
		{"loop of two", `"units":[{"name":"A","extends":"B"},{"name":"B","extends":"A"}]`,
			[]string{`units[1].extends: "A" extends itself through A -> B -> A; extends must not form a loop`}},
		{"loop of three", `"gear":[{"name":"A","extends":"B"},{"name":"B","extends":"C"},{"name":"C","extends":"A"}]`,
			[]string{`gear[2].extends: "A" extends itself through A -> B -> C -> A; extends must not form a loop`}},
		{"loop of companies via aka", `"companies":[{"name":"A","aka":["Aa"],"extends":"B","alignment":"good"},{"name":"B","extends":"Aa","alignment":"good"}]`,
			[]string{`companies[1].extends: "Aa" extends itself through A -> B -> A; extends must not form a loop`}},
		{"unit with no name", `"units":[{"note":"x"}]`, []string{`units[0]: a unit needs a "name" or an "extends"`}},
		{"missing base is left to resolution", `"units":[{"name":"A","extends":"Nobody"}]`, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, rep := normalised(t, minimalWith(t, c.extra))
			var got []string
			for _, i := range rep.Issues {
				got = append(got, i.Path+": "+i.Message)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
		})
	}
}

// TestNormaliseIdempotent: Normalise(Normalise(x)) equals Normalise(x) on every valid fixture and on the extends documents above.
func TestNormaliseIdempotent(t *testing.T) {
	files, _ := filepath.Glob("testdata/valid/*.json")
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, _ := os.ReadFile(f)
			once, _ := normalised(t, string(raw))
			twice := deepCopy(reflect.ValueOf(once).Elem()).Interface().(Document)
			var rep Report
			Normalise(&twice, NewReporter("f", &rep))
			if !reflect.DeepEqual(*once, twice) || len(rep.Issues) != 0 {
				t.Errorf("not idempotent:\n%s", rep)
			}
		})
	}
}
