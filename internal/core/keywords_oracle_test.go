package core

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The keyword oracle: for every company, every unit name it uses is built through the real loader (RuleSet.UnitKeywords) and compared
// with a keyword set computed here from the raw JSON files with encoding/json into generic values: nothing of core's parsing is used.

type orObj = map[string]any

func orList(v any) []any { l, _ := v.([]any); return l }

func orStrings(v any) []string {
	var out []string
	for _, x := range orList(v) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func orStr(o orObj, k string) string { s, _ := o[k].(string); return s }

// oracleNormalize is the test's own copy of the spelling table of docs/unit-keywords.md.
func oracleNormalize(words []string) []string {
	canon := map[string]string{
		"khazad-dum": "Khazad-dum", "lothlorien": "Lothlórien", "lothlórien": "Lothlórien", "the iron hills": "Iron Hills",
		"iron hills": "Iron Hills", "the misty mountains": "Misty Mountains", "misty mountains": "Misty Mountains", "hero": "Hero",
	}
	var out []string
	for _, raw := range words {
		for _, w := range strings.Split(raw, ",") {
			w = strings.TrimSpace(w)
			if c, ok := canon[strings.ToLower(w)]; ok {
				w = c
			}
			if w == "" || w == "*" || strings.EqualFold(w, "none") {
				continue
			}
			if !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, w) }) {
				out = append(out, w)
			}
		}
	}
	return out
}

type oracleKeywords struct{ race, faction, types []string }

func TestKeywordOracleAllCompanies(t *testing.T) {
	rs := shippedRules(t)
	var vanilla, bc struct {
		Data orObj `json:"data"`
	}
	if err := json.Unmarshal(readDef(t, "vanilla/data2024.json"), &vanilla); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(readDef(t, "battle_companies/battle_companies.json"), &bc); err != nil {
		t.Fatal(err)
	}
	profiles := map[string]orObj{} // first occurrence wins, heroes before warriors
	for _, group := range []string{"heroes", "warriors"} {
		for _, x := range orList(vanilla.Data[group]) {
			u := x.(orObj)
			if _, ok := profiles[orStr(u, "name")]; !ok {
				profiles[orStr(u, "name")] = u
			}
		}
	}
	races := map[string]bool{} // every race word of the vanilla file, lower-cased
	for _, u := range profiles {
		for _, r := range oracleNormalize(orStrings(u["race"])) {
			races[strings.ToLower(r)] = true
		}
	}
	fillIns := map[string]orObj{}
	for _, x := range orList(bc.Data["unitFillIns"].(orObj)["items"]) {
		f := x.(orObj)
		fillIns[orStr(f, "unit")] = f
	}

	allRace, allFaction, allType := map[string]bool{}, map[string]bool{}, map[string]bool{}
	units := 0
	for _, x := range orList(bc.Data["companies"]) {
		co := x.(orObj)
		name := orStr(co, "name")
		t.Run(name, func(t *testing.T) {
			// the company's own names: aliases, type changes
			aliases := map[string]orObj{}
			typeFx := map[string]orObj{}
			for _, a := range orList(co["unitAliases"]) {
				aliases[strings.ToLower(orStr(a.(orObj), "label"))] = a.(orObj)
			}
			for _, e := range orList(co["effects"]) {
				e := e.(orObj)
				switch orStr(e, "kind") {
				case "unitAlias":
					aliases[strings.ToLower(orStr(e, "label"))] = e
				case "unitTypes":
					typeFx[orStr(e, "unit")] = e
				}
			}
			used := map[string]bool{} // every "unit"/"label"/"units" string of the company that names a unit
			var walk func(v any)
			walk = func(v any) {
				switch v := v.(type) {
				case orObj:
					for k, x := range v {
						if s, ok := x.(string); ok && (k == "unit" || k == "label") {
							used[s] = true
						}
						if k == "units" || k == "includes" {
							for _, s := range orStrings(x) {
								used[s] = true
							}
						}
						walk(x)
					}
				case []any:
					for _, x := range v {
						walk(x)
					}
				}
			}
			walk(co)
			p, ok := rs.BC.Profile(name)
			if !ok {
				t.Fatalf("no profile for %s", name)
			}
			checked := 0
			for unit := range used {
				vname, alias := unit, orObj(nil)
				if _, ok := profiles[unit]; !ok {
					a, isAlias := aliases[strings.ToLower(unit)]
					if !isAlias {
						continue // not a unit (a hero upgrade's name, a label of a rule...)
					}
					vname, alias = orStr(a, "unit"), a
				}
				v, ok := profiles[vname]
				if !ok {
					continue
				}
				if alias != nil && strings.EqualFold(orStr(alias, "label"), unit) {
					// the company's own name for the profile
				} else {
					alias = nil
				}
				want := oracleKeywordsOf(v, profiles, fillIns, typeFx[unit], alias, races)
				got := rs.UnitKeywords(p, UnitKeywordsInput{Type: unit})
				for _, g := range []struct {
					kind KeywordKind
					want []string
				}{{KeywordRace, want.race}, {KeywordFaction, want.faction}, {KeywordType, want.types}} {
					have := names(got.Group(g.kind))
					if !slices.Equal(have, g.want) {
						t.Errorf("%s / %s: %s keywords = %v, want %v", name, unit, g.kind, have, g.want)
					}
				}
				for _, w := range want.race {
					allRace[w] = true
				}
				for _, w := range want.faction {
					allFaction[w] = true
				}
				for _, w := range want.types {
					allType[w] = true
				}
				checked++
				units++
			}
			if checked == 0 {
				t.Errorf("%s: no unit checked", name)
			}
		})
	}

	// The sets of keywords over all companies: a data change that adds a race, a faction or a type is noticed here.
	keys := func(m map[string]bool) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	t.Logf("%d unit names over the companies", units)
	for _, g := range []struct {
		what string
		have []string
		want []string
	}{
		{"races", keys(allRace), wantRaces},
		{"factions", keys(allFaction), wantFactions},
		{"types", keys(allType), wantTypes},
	} {
		if !slices.Equal(g.have, g.want) {
			t.Errorf("the %s of the companies changed (%d, was %d):\n got  %q\n want %q\nreview the keyword tooltips and the spelling table, then update the guard", g.what, len(g.have), len(g.want), g.have, g.want)
		}
	}
}

func oracleKeywordsOf(v orObj, profiles map[string]orObj, fillIns map[string]orObj, fx orObj, alias orObj, races map[string]bool) oracleKeywords {
	race, faction, types := orStrings(v["race"]), orStrings(v["faction"]), orStrings(v["unitType"])
	if len(race)+len(faction)+len(types) == 0 {
		if f, ok := fillIns[orStr(v, "name")]; ok {
			race, faction, types = orStrings(f["race"]), orStrings(f["faction"]), orStrings(f["unitType"])
		}
	}
	if len(race)+len(faction)+len(types) == 0 { // a bundle: its first member
		if inc := orList(v["includes"]); len(inc) > 0 {
			m := profiles[orStr(inc[0].(orObj), "name")]
			race, faction, types = orStrings(m["race"]), orStrings(m["faction"]), orStrings(m["unitType"])
		}
	}
	out := oracleKeywords{race: oracleNormalize(race), faction: oracleNormalize(faction), types: oracleNormalize(types)}
	if fx != nil {
		var kept []string
		for _, ty := range out.types {
			if !slices.Contains(orStrings(fx["remove"]), ty) {
				kept = append(kept, ty)
			}
		}
		out.types = oracleNormalize(append(kept, orStrings(fx["add"])...))
	}
	if alias != nil {
		for _, w := range oracleNormalize(orStrings(alias["addKeywords"])) {
			if races[strings.ToLower(w)] {
				out.race = oracleNormalize(append(out.race, w))
			} else {
				out.faction = oracleNormalize(append(out.faction, w))
			}
		}
	}
	return out
}

// The measured sets over the 37 companies (docs/unit-keywords.md): 15 races, 36 factions, 8 unit types.
var (
	wantRaces    = []string{"Bat", "Bird", "Drake", "Dwarf", "Elf", "Goblin", "Hobbit", "Man", "Ogre", "Orc", "Spider", "Spirit", "Troll", "Uruk-hai", "Warg"}
	wantFactions = []string{"Angmar", "Arnor", "Black Dragon", "Blackroot Vale", "Carn Dum", "Corsair", "Dale", "Dol Amroth", "Dol Guldur", "Druadan", "Dunedain", "Dunland", "Easterling", "Erebor", "Goblin-town", "Gondor", "Gundabad", "Harad", "Iron Hills", "Isengard", "Khand", "Khazad-dum", "Lake-town", "Lamedon", "Lossarnach", "Lothlórien", "Mahûd", "Mirkwood", "Mordor", "Moria", "Numenor", "Rivendell", "Rohan", "Ruffian", "Shire", "Umbar"}
	wantTypes    = []string{"Beast", "Cavalry", "Elite", "Hero", "Infantry", "Monster", "Swarm", "Warrior"}
)
