package core

import (
	"os"
	"slices"
	"testing"
)

const vanillaSample = `{"stats":[{"name":"Movement","abbreviation":"Mv","trail":"\""},{"name":"Fight","abbreviation":"FV"},{"name":"","abbreviation":"|"},{"name":"Might","abbreviation":"M"}],
"data":{"heroes":[{"name":"Hero X","points":50,"movement":6,"fight":5,"might":3,"unitType":["Hero","Infantry"],"specialRules":["Terror (2)"],"wargear":["Sword"]}],
"warriors":[{"name":"Grunt","points":8,"movement":6,"fight":4,"unitType":["Warrior","Infantry"]},{"name":"Grunt","points":99,"movement":9}],
"gear":[{"name":"Bow","type":"bow","definition":"24\" Range, 2 Strength"}],
"keywords":[{"name":"Terror","definition":"Scary."}],"magicalPowers":[{"name":"Wither","duration":"Instant","definition":"Bad."}]}}`

func TestVanillaBook(t *testing.T) {
	b, err := ParseVanilla([]byte(vanillaSample))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(b.Stats()); got != 3 { // the "|" separator is not an attribute
		t.Errorf("stats: %d", got)
	}
	grunt, _ := b.Unit("Grunt")
	if grunt.Points != 8 {
		t.Errorf("the first occurrence of a name wins: %+v", grunt)
	}
	hero, _ := b.Unit("Hero X")
	profile := b.Profile(hero, true, map[string]int{"fight": 1})
	if len(profile) != 3 || profile[0].Abbr != "Mv" || profile[0].Value != 6 || profile[1].Value != 6 || profile[1].Boost != 1 || profile[2].Abbr != "M" || !profile[2].HeroOnly {
		t.Errorf("hero profile: %+v", profile)
	}
	if got := b.Profile(grunt, false, nil); len(got) != 2 {
		t.Errorf("a warrior has no Might: %+v", got)
	}
	if d, ok := b.Keyword("Terror (2)"); !ok || d != "Scary." {
		t.Errorf("keyword with a parameter: %q %v", d, ok)
	}
	if g, ok := b.Gear("Bow"); !ok || g.Type != "bow" {
		t.Errorf("gear: %+v", g)
	}
	if p, ok := b.Power("Wither"); !ok || p.Duration != "Instant" {
		t.Errorf("power: %+v", p)
	}
}

func TestRealVanillaBook(t *testing.T) {
	content, err := os.ReadFile("../../definitions/vanilla/data2024.json")
	if err != nil {
		t.Skip("definitions not available: ", err)
	}
	b, err := ParseVanilla(content)
	if err != nil {
		t.Fatal(err)
	}
	var abbrs []string
	for _, s := range b.Stats() {
		abbrs = append(abbrs, s.Abbr)
	}
	t.Logf("attributes: %v", abbrs)
	u, ok := b.Unit("Captain of Minas Tirith")
	if !ok || u.Points != 60 || u.Stats["movement"] != 6 || u.Stats["might"] == 0 {
		t.Errorf("Captain of Minas Tirith: %+v", u)
	}
	if _, ok := b.Keyword("Terror"); !ok {
		t.Error("Terror")
	}
}

func TestStatVocabularyIsTheVanillaList(t *testing.T) {
	raw, err := os.ReadFile("../../definitions/vanilla/data2024.json")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseVanilla(raw)
	if err != nil {
		t.Fatal(err)
	}
	var phrased []string
	for _, s := range statPhrases {
		phrased = append(phrased, s.key)
	}
	if got := b.StatKeys(); !slices.Equal(got, phrased) {
		t.Errorf("the phrasing table %v is not the vanilla stat list %v", phrased, got)
	}
	heroes := b.HeroStats()
	if !heroes["might"] || !heroes["will"] || !heroes["fate"] || len(heroes) != 3 || b.HeroStat("fight") || !b.HeroStat("fate") {
		t.Errorf("hero stats: %v", heroes)
	}
	if !b.HeroProfile(map[string]int{"fate": 1}) || b.HeroProfile(map[string]int{"fight": 3, "wounds": 2}) {
		t.Error("a profile is a hero's when it has a hero characteristic")
	}
}
