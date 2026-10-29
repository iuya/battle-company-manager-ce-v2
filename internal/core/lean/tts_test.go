package lean

import (
	"strings"
	"testing"
)

// The tts of a unit and of an item are typed: a vanilla unit is annotated by an entry with a name and only tts, the option
// spellings are a map, and a value of the wrong shape is an error with its path.
func TestTTSIsTyped(t *testing.T) {
	doc, _ := normalised(t, minimalWith(t, `
"units":[{"name":"Isengard Orc Warrior","tts":{"foot":"Isengard Orc Warrior","states":false,"options":{"Orc bow":"Bow"},"defaultOptions":["Spear"],"alternatives":["Orc"],"marker":"Lorien Guard"}}],
"gear":[{"name":"Warg","tts":{"model":"Fell Warg","alternatives":["Wild Warg"]}}]`))
	u := doc.Units[0].TTS
	if u == nil || u.Foot != "Isengard Orc Warrior" || u.States == nil || *u.States || u.Options["Orc bow"] != "Bow" || len(u.DefaultOptions) != 1 || u.Alternatives[0] != "Orc" || u.Marker != "Lorien Guard" {
		t.Errorf("unit tts: %+v", u)
	}
	if g := doc.Gear[0].TTS; g == nil || g.Model != "Fell Warg" || len(g.Alternatives) != 1 {
		t.Errorf("gear tts: %+v", g)
	}
	for name, src := range map[string]string{
		"options of the wrong shape": `"units":[{"name":"X","tts":{"options":["a"]}}]`,
		"options with a number":      `"units":[{"name":"X","tts":{"options":{"a":1}}}]`,
		"alternatives that is text":  `"gear":[{"name":"Warg","tts":{"model":"m","alternatives":"Wild Warg"}}]`,
		"an unknown key":             `"gear":[{"name":"Warg","tts":{"mdoel":"m"}}]`,
	} {
		var d Document
		var rep Report
		if Bind("f.json", []byte(minimalWith(t, src)), &d, &rep); rep.OK() {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The rider on foot of a unit is a unit the game knows: a misspelt one is reported with its closest match.
func TestTTSFootMustBeAUnit(t *testing.T) {
	_, _, rep := resolved(t, `"units":[{"name":"Angmar Warg Rider","tts":{"foot":"Ranger of Gondr"}}]`, newFakeLower())
	got := issueLines(rep)
	if len(got) != 1 || !strings.Contains(got[0], "units[0].tts.foot") || !strings.Contains(got[0], "Ranger of Gondr") {
		t.Errorf("issues: %q", got)
	}
}
