package ttshelper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/ttshelper/ttsfake"
)

// armyWorld spawns the Rohan fixture and returns the world, the figures in spawn order and the
// payload text.
func armyWorld(t *testing.T) (*ttsfake.World, []*ttsfake.Obj, string) {
	t.Helper()
	w := ttsfake.New(t, Script())
	figs := w.Spawn(fixture(t, "rohan_list.txt"), 10, 20)
	body := fixture(t, "rohan_names.json")
	w.SetNames(body, "White")
	return w, figs, body
}

func mustContain(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, s := range want {
		if !strings.Contains(got, s) {
			t.Errorf("messages %q do not contain %q", got, s)
		}
	}
}

func TestFixtureSpawnerShape(t *testing.T) {
	w := ttsfake.New(t, Script())
	figs := w.Spawn(fixture(t, "rohan_list.txt"), 10, 20)
	if len(figs) != 9 {
		t.Fatalf("spawned %d figures, want 9", len(figs))
	}
	if figs[1].Z != 21.5 || figs[7].Z != 22.6 || figs[7].X != 10 || figs[8].X < 11.09 || figs[8].X > 11.11 {
		t.Errorf("grid: %+v %+v", figs[1], figs[7])
	}
	var p struct{ Figures []any }
	if err := json.Unmarshal([]byte(fixture(t, "rohan_names.json")), &p); err != nil || len(p.Figures) != 9 {
		t.Errorf("fixture payload: %v, %d figures", err, len(p.Figures))
	}
}

func TestApplyNamesInGridOrderWithIdenticalNicknames(t *testing.T) {
	w, figs, _ := armyWorld(t)
	// the player (or the table) lists the figures in no particular order
	for i, j := 0, len(w.Objs)-1; i < j; i, j = i+1, j-1 {
		w.Objs[i], w.Objs[j] = w.Objs[j], w.Objs[i]
	}
	w.Press("applyNames", "White")

	want := []string{"Theoden", "Eadric", "Wulf", "Hild", "Banner A", "Banner B", "Eadric", "Grub", "Ulaire"}
	descs := []string{"", "Rider 1", "Rider 2", "Rider 3", "Banner 1", "Banner 2", "Eadric on foot", "Warg of Grub", "Accented key"}
	for i, f := range figs {
		if !strings.Contains(f.Name, want[i]) {
			t.Errorf("figure %d name %q, want it to contain %q", i, f.Name, want[i])
		}
		if i > 0 && f.Desc != descs[i] {
			t.Errorf("figure %d description %q, want %q", i, f.Desc, descs[i])
		}
		if !f.Tags["BCNamed"] || !strings.HasPrefix(f.Notes, "bc-unit:") {
			t.Errorf("figure %d not tagged or noted: %v %q", i, f.Tags, f.Notes)
		}
	}
	if figs[6].Notes != "bc-unit:2:foot" || figs[1].Notes != "bc-unit:2" || figs[7].Notes != "bc-unit:7:mount" || figs[8].Notes != "bc-unit:8" {
		t.Errorf("notes: %q %q %q", figs[6].Notes, figs[7].Notes, figs[8].Notes)
	}
	if !strings.Contains(figs[0].Desc, "[00CCFF]") {
		t.Errorf("the rich text is passed through: %q", figs[0].Desc)
	}
	mustContain(t, w.Texts(), "Named 9 of 9 figures.")
	if w.Msgs[0].RGB != [3]float64{0.2, 1, 0.2} {
		t.Errorf("summary colour %v", w.Msgs[0].RGB)
	}
}

func TestSecondRowComesAfterFirstRow(t *testing.T) {
	// two identical figures, one on the second row with a smaller x: the first row wins
	w := ttsfake.New(t, Script())
	b := w.Add("Rider of Rohan: Shield", 10, 22.6) // second row
	a := w.Add("Rider of Rohan: Shield", 15.5, 21.5)
	w.SetNames(`{"v":1,"figures":[{"k":"rider of rohan: shield","u":1,"n":"first","d":"1"},{"k":"rider of rohan: shield","u":2,"n":"second","d":"2"}]}`, "White")
	w.Press("applyNames", "White")
	if a.Name != "first" || b.Name != "second" {
		t.Errorf("a=%q b=%q", a.Name, b.Name)
	}
}

func TestSelectionOnly(t *testing.T) {
	w, figs, _ := armyWorld(t)
	other := w.Spawn(fixture(t, "rohan_list.txt"), 10, 60) // the opponent's identical army
	w.Selected["White"] = figs
	w.Press("applyNames", "White")
	for _, f := range other {
		if f.Calls != 0 {
			t.Fatalf("an unselected figure was touched: %+v", f)
		}
	}
	if !strings.Contains(figs[0].Name, "Theoden") {
		t.Errorf("selected hero not named: %q", figs[0].Name)
	}
	mustContain(t, w.Texts(), "Named 9 of 9 figures.", "9 figures named like the list were left alone (not selected).")
}

func TestWholeTableWithoutSelectionNamesTheFirstArmyInGridOrder(t *testing.T) {
	w, figs, _ := armyWorld(t)
	other := w.Spawn(fixture(t, "rohan_list.txt"), 10, 60)
	w.Press("applyNames", "White")
	if !strings.Contains(figs[0].Name, "Theoden") {
		t.Errorf("hero of the army with the smaller z not named: %q", figs[0].Name)
	}
	if other[0].Calls != 0 {
		t.Errorf("the second army must keep its names when there are no entries left")
	}
	mustContain(t, w.Texts(), "Named 9 of 9 figures.", "more figures than names")
}

func TestDoubleApplyIsIdempotent(t *testing.T) {
	w, figs, _ := armyWorld(t)
	w.Press("applyNames", "White")
	before := make([]int, len(figs))
	for i, f := range figs {
		before[i] = f.Calls
	}
	w.Press("applyNames", "White")
	for i, f := range figs {
		if f.Calls != before[i] {
			t.Errorf("figure %d changed on the second Apply", i)
		}
	}
	mustContain(t, w.Texts(), "0 new, 9 already named.")
	if strings.Contains(w.Texts(), "Not found") {
		t.Errorf("already named figures are not missing: %s", w.Texts())
	}
}

func TestReapplySendsNewDataAndFindsRenamedFigures(t *testing.T) {
	w, figs, body := armyWorld(t)
	w.Press("applyNames", "White")
	figs[2].Name = "my own name" // the player renamed a figure by hand: the GM note still says who it is
	newBody := strings.ReplaceAll(body, `"d":"Rider 2"`, `"d":"Rider 2 injured"`)
	w.SetNames(newBody, "White")
	w.Press("reapplyNames", "White")
	if figs[2].Desc != "Rider 2 injured" || !strings.Contains(figs[2].Name, "Wulf") {
		t.Errorf("not re-applied: %q %q", figs[2].Name, figs[2].Desc)
	}
	mustContain(t, w.Texts(), "Named 9 of 9 figures.")
	// and a plain Apply afterwards does not touch them
	calls := figs[2].Calls
	w.Press("applyNames", "White")
	if figs[2].Calls != calls {
		t.Errorf("plain Apply touched a named figure")
	}
}

func TestReapplyAlsoNamesFiguresThatAreNotTaggedYet(t *testing.T) {
	w, figs, _ := armyWorld(t)
	w.Press("reapplyNames", "White")
	if !figs[8].Tags["BCNamed"] {
		t.Errorf("Re-apply on fresh figures must name them by nickname")
	}
}

func TestMissingFigureIsReported(t *testing.T) {
	w, figs, _ := armyWorld(t)
	// the Fell Warg and one banner bearer are gone
	keep := w.Objs[:0]
	for _, o := range w.Objs {
		if o != figs[7] && o != figs[5] {
			keep = append(keep, o)
		}
	}
	w.Objs = keep
	w.Press("applyNames", "White")
	mustContain(t, w.Texts(), "Named 7 of 9 figures.", "Not found:", "fell warg: (1)", "warrior of rohan: banner (1)")
	if w.Msgs[0].RGB != [3]float64{1, 0.5, 0} {
		t.Errorf("a partial result is orange: %v", w.Msgs[0].RGB)
	}
}

func TestExtraFigureIsLeftAlone(t *testing.T) {
	w, figs, _ := armyWorld(t)
	extra := w.Add("Rider of Rohan: Shield", 30, 40)
	w.Press("applyNames", "White")
	if extra.Calls != 0 {
		t.Errorf("the extra figure must keep its name")
	}
	if !strings.Contains(figs[1].Name, "Eadric") {
		t.Errorf("the army is still named")
	}
	mustContain(t, w.Texts(), "Named 9 of 9 figures.", "1 more figures than names")
}

func TestTaggedObjectsAreSkippedAndKeepTheirData(t *testing.T) {
	w, figs, _ := armyWorld(t)
	figs[1].Tags["BCNamed"] = true
	figs[1].Notes = "bc-unit:999" // named for another company
	w.Press("applyNames", "White")
	if figs[1].Calls != 0 || figs[1].Name != "Rider of Rohan: Shield" {
		t.Errorf("a figure of another company was touched: %+v", figs[1])
	}
	// the three riders of this payload: the entry of the first is taken by the next free figure
	if !strings.Contains(figs[2].Name, "Eadric") {
		t.Errorf("the free figures take the entries in order: %q", figs[2].Name)
	}
}

func TestMessagesForBadInput(t *testing.T) {
	cases := []struct {
		name  string
		setup func(w *ttsfake.World)
		want  string
	}{
		{"no tab", func(w *ttsfake.World) { w.Tabs = nil }, "Notebook tab 'ArmyNames' not found"},
		{"other tab", func(w *ttsfake.World) { w.Tabs = []ttsfake.Tab{{Title: "ArmyList", Body: "x", Color: "White"}} }, "not found"},
		{"empty", func(w *ttsfake.World) { w.SetNames("   ", "White") }, "is empty"},
		{"bad json", func(w *ttsfake.World) { w.SetNames(`{"v":1,"figures":[{"k":"a"`, "White") }, "not valid JSON"},
		{"not an object", func(w *ttsfake.World) { w.SetNames(`"hello"`, "White") }, "not valid JSON"},
		{"unknown version", func(w *ttsfake.World) { w.SetNames(`{"v":3,"figures":[]}`, "White") }, "older than the app"},
		{"no version", func(w *ttsfake.World) { w.SetNames(`{"figures":[]}`, "White") }, "older than the app"},
		{"no figures", func(w *ttsfake.World) { w.SetNames(`{"v":1}`, "White") }, "have no figures"},
		{"oversized", func(w *ttsfake.World) {
			w.SetNames(`{"v":1,"figures":[],"pad":"`+strings.Repeat("x", 270000)+`"}`, "White")
		}, "too large"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, figs, _ := armyWorld(t)
			c.setup(w)
			w.Press("applyNames", "White")
			if len(w.Msgs) != 1 || w.Msgs[0].To != "White" || !strings.Contains(w.Msgs[0].Text, c.want) {
				t.Fatalf("messages: %q, want one to White containing %q", w.Texts(), c.want)
			}
			if w.Msgs[0].RGB != [3]float64{1, 0, 0} {
				t.Errorf("errors are red: %v", w.Msgs[0].RGB)
			}
			for _, f := range figs {
				if f.Calls != 0 {
					t.Fatalf("a figure was touched on an error")
				}
			}
		})
	}
}

func TestTabOfThePressingPlayerIsPreferred(t *testing.T) {
	w, figs, body := armyWorld(t)
	other := strings.ReplaceAll(body, "[b]Theoden[/b]", "[b]Somebody else[/b]")
	w.Tabs = []ttsfake.Tab{{Title: "ArmyNames", Body: other, Color: "Red"}, {Title: "ArmyNames", Body: body, Color: "White"}}
	w.Press("applyNames", "White")
	if !strings.Contains(figs[0].Name, "Theoden") {
		t.Errorf("the White tab must win: %q", figs[0].Name)
	}
	w2, figs2, body2 := armyWorld(t)
	w2.Tabs = []ttsfake.Tab{{Title: "ArmyNames", Body: strings.ReplaceAll(body2, "[b]Theoden[/b]", "[b]First tab[/b]"), Color: "Red"}}
	w2.Press("applyNames", "Blue")
	if !strings.Contains(figs2[0].Name, "First tab") {
		t.Errorf("without a tab of the player the first one is read: %q", figs2[0].Name)
	}
}

func TestNoMatchShowsTheFirstNicknames(t *testing.T) {
	w := ttsfake.New(t, Script())
	w.Add("Some Elf Warrior", 0, 0)
	w.Add("Another Thing", 1, 0)
	w.SetNames(fixture(t, "rohan_names.json"), "White")
	w.Press("applyNames", "White")
	mustContain(t, w.Texts(), "No figure matches", "Some Elf Warrior | Another Thing")
}

func TestStatesAreMentioned(t *testing.T) {
	w, figs, _ := armyWorld(t)
	figs[1].States = 1
	w.Press("applyNames", "White")
	mustContain(t, w.Texts(), "1 figures have states")
}

func TestSelectionWithoutPlayerObjectFallsBackToTable(t *testing.T) {
	w, figs, _ := armyWorld(t)
	w.Press("applyNames", "Blue") // nothing selected by Blue
	if !figs[0].Tags["BCNamed"] {
		t.Errorf("no selection means the whole table")
	}
}

func TestHelperDoesNotNameItself(t *testing.T) {
	w, _, _ := armyWorld(t)
	me := w.Add("rider of rohan: shield", 0, 0)
	me.GUID = "bc4e01" // the helper block, with a matching nickname by accident
	w.Press("applyNames", "White")
	if me.Calls != 0 {
		t.Errorf("the helper object was renamed")
	}
}

// The script must stay in the common subset of Lua 5.1 and 5.2 that MoonSharp (the interpreter
// of Tabletop Simulator) runs.
func TestScriptUsesOnlyTheCommonSubset(t *testing.T) {
	src := stripStringsAndComments(Script())
	banned := map[string]*regexp.Regexp{
		"goto":                       regexp.MustCompile(`\bgoto\b`),
		"label ::":                   regexp.MustCompile(`::`),
		"integer division //":        regexp.MustCompile(`//`),
		"shift << >>":                regexp.MustCompile(`<<|>>`),
		"bit and &":                  regexp.MustCompile(`&`),
		"bit or |":                   regexp.MustCompile(`\|`),
		"bit xor/not ~":              regexp.MustCompile(`~[^=]`),
		"attribs <const>":            regexp.MustCompile(`<(const|close)>`),
		"string.pack":                regexp.MustCompile(`string\.(pack|unpack|packsize)`),
		"utf8 library":               regexp.MustCompile(`\butf8\.`),
		"math.tointeger":             regexp.MustCompile(`math\.(tointeger|ult)`),
		"bare unpack":                regexp.MustCompile(`(^|[^.\w])unpack\(`),
		"bit32":                      regexp.MustCompile(`\bbit32\b`),
		"table.move":                 regexp.MustCompile(`table\.move`),
		"destruct/clone/setPosition": regexp.MustCompile(`\b(destruct|clone|setPosition|setPositionSmooth|takeObject|putObject|destroyObject|spawnObject|setRotation|setRotationSmooth|setScale|setState|setLock)\b`),
	}
	for name, re := range banned {
		if loc := re.FindStringIndex(src); loc != nil {
			t.Errorf("%s found near %q", name, src[max(0, loc[0]-20):min(len(src), loc[1]+20)])
		}
	}
}

// stripStringsAndComments blanks string literals and comments (single line forms are enough
// for the helper).
func stripStringsAndComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		inStr := byte(0)
		for i := 0; i < len(line); i++ {
			c := line[i]
			if inStr != 0 {
				if c == '\\' {
					i++
				} else if c == inStr {
					inStr = 0
				}
				continue
			}
			if c == '"' || c == '\'' {
				inStr = c
				continue
			}
			if c == '-' && i+1 < len(line) && line[i+1] == '-' {
				break
			}
			b.WriteByte(c)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "tts", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
