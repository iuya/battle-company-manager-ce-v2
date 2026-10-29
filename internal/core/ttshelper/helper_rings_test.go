package ttshelper

import (
	"math"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/ttshelper/ttsfake"
)

// The aura rings (docs/aura-ranges.md, section 4.3) against the fake Tabletop Simulator.

var (
	colBanner  = [3]float64{0xF0 / 255.0, 0xE4 / 255.0, 0x42 / 255.0}
	colCommand = [3]float64{0xE6 / 255.0, 0x9F / 255.0, 0x00 / 255.0}
	colEnemy   = [3]float64{0xD5 / 255.0, 0x5E / 255.0, 0x00 / 255.0}
	colUtility = [3]float64{0xCC / 255.0, 0x79 / 255.0, 0xA7 / 255.0}
	colSupport = [3]float64{0x56 / 255.0, 0xB4 / 255.0, 0xE9 / 255.0}
	modCircle  = ttsfake.Line{Color: [3]float64{0, 0.8, 0.4}, Thickness: 0.05, Points: [][3]float64{{1, 0.1, 0}, {0, 0.1, 1}, {-1, 0.1, 0}, {1, 0.1, 0}}}
)

// ringWorld is the Rohan army with the v2 payload, the buttons made and the rings not yet drawn.
func ringWorld(t *testing.T) (*ttsfake.World, []*ttsfake.Obj) {
	t.Helper()
	w := ttsfake.New(t, Script())
	figs := w.Spawn(fixture(t, "rohan_list.txt"), 10, 20)
	w.SetNames(fixture(t, "rohan_names_v2.json"), "White")
	w.Load("")
	return w, figs
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func sameColour(a, b [3]float64) bool {
	return near(a[0], b[0]) && near(a[1], b[1]) && near(a[2], b[2])
}

// ours are the lines of an object that are not the mod's circle.
func ours(o *ttsfake.Obj) []ttsfake.Line {
	var out []ttsfake.Line
	for _, l := range o.Lines {
		if l.Thickness != 0.05 {
			out = append(out, l)
		}
	}
	return out
}

func radiusOf(l ttsfake.Line) float64 {
	p := l.Points[0]
	return math.Hypot(p[0], p[2])
}

func totalLines(figs []*ttsfake.Obj) int {
	n := 0
	for _, f := range figs {
		n += len(ours(f))
	}
	return n
}

func TestRingGeometryAndColours(t *testing.T) {
	w, figs := ringWorld(t)
	w.Press("applyNames", "White")

	// a banner on a 0.98 wide figure: radius 3 + 0.49 - 0.0225, 48 segments closed, per-kind height
	ls := ours(figs[4])
	if len(ls) != 1 {
		t.Fatalf("banner figure has %d lines", len(ls))
	}
	l := ls[0]
	if len(l.Points) != 49 {
		t.Errorf("%d points, want 49", len(l.Points))
	}
	if l.Points[0] != l.Points[48] && (math.Abs(l.Points[0][0]-l.Points[48][0]) > 1e-9 || math.Abs(l.Points[0][2]-l.Points[48][2]) > 1e-9) {
		t.Errorf("the ring is not closed: %v %v", l.Points[0], l.Points[48])
	}
	want := 3 + 0.49 - 0.0225
	for i, p := range l.Points {
		if !near(math.Hypot(p[0], p[2]), want) || !near(p[1], 0.115) {
			t.Fatalf("point %d %v: radius %.4f want %.4f, height 0.115", i, p, math.Hypot(p[0], p[2]), want)
		}
	}
	if !sameColour(l.Color, colBanner) || l.Thickness != 0.045 {
		t.Errorf("colour %v thickness %v", l.Color, l.Thickness)
	}
	mustContain(t, w.Texts(), "Rings drawn for 8 figures (15 rings).")
	if w.Msgs[len(w.Msgs)-1].RGB != [3]float64{0.2, 1, 0.2} {
		t.Errorf("the summary of the rings is green")
	}
}

func TestRingBaseFromBoundsPayloadBaseIgnored(t *testing.T) {
	w, figs := ringWorld(t)
	figs[4].Size = [3]float64{1.5, 1, 1.2} // the larger half side is 0.75
	figs[8].Size = [3]float64{3, 1, 3}     // the payload still says b = 0.6 (an old payload): it is ignored, the bounds count
	w.Press("applyNames", "White")
	if r := radiusOf(ours(figs[4])[0]); !near(r, 3+0.75-0.0225) {
		t.Errorf("bounds based radius %.4f", r)
	}
	var enemy ttsfake.Line
	for _, l := range ours(figs[8]) {
		if sameColour(l.Color, colEnemy) {
			enemy = l
		}
	}
	if r := radiusOf(enemy); !near(r, 12+1.5-0.0225) {
		t.Errorf("bounds radius, payload b ignored %.4f", r)
	}
}

func TestRingScaleDividesPointsAndThickness(t *testing.T) {
	w, figs := ringWorld(t)
	figs[4].Scale = 1.5
	figs[4].Size = [3]float64{1.47, 2, 1.47} // world size of a 0.98 base at scale 1.5
	w.Press("applyNames", "White")
	l := ours(figs[4])[0]
	if r := radiusOf(l); !near(r, (3+0.735-0.0225)/1.5) {
		t.Errorf("radius %.4f", r)
	}
	if !near(l.Thickness, 0.045/1.5) || !near(l.Points[3][1], 0.115/1.5) {
		t.Errorf("thickness %v height %v", l.Thickness, l.Points[3][1])
	}
}

func TestKindsHaveDistinctColoursHeightsAndRadiiGaps(t *testing.T) {
	w, figs := ringWorld(t)
	w.Press("applyNames", "White")
	ls := ours(figs[0]) // banner 3, command 6, enemy 6, support 3
	if len(ls) != 4 {
		t.Fatalf("hero has %d rings", len(ls))
	}
	cols := map[[3]float64]bool{}
	heights := map[float64]bool{}
	for _, l := range ls {
		cols[l.Color] = true
		heights[l.Points[0][1]] = true
	}
	if len(cols) != 4 || len(heights) != 4 {
		t.Errorf("colours %d heights %d, want 4 each", len(cols), len(heights))
	}
	var command, enemy ttsfake.Line
	for _, l := range ls {
		switch {
		case sameColour(l.Color, colCommand):
			command = l
		case sameColour(l.Color, colEnemy):
			enemy = l
		}
	}
	if enemy.Thickness != 0.06 || command.Thickness != 0.045 {
		t.Errorf("thickness enemy %v command %v", enemy.Thickness, command.Thickness)
	}
	if !near(radiusOf(enemy)-radiusOf(command), 0.03) {
		t.Errorf("two kinds at 6 inches are 0.03 apart: %.4f %.4f", radiusOf(command), radiusOf(enemy))
	}
}

func TestRingsKeepTheModsOwnLines(t *testing.T) {
	w, figs := ringWorld(t)
	figs[1].SetLines([]ttsfake.Line{modCircle})
	w.Press("applyNames", "White")
	if len(figs[1].Lines) != 2 || figs[1].Lines[0].Thickness != 0.05 || figs[1].Lines[0].Color != modCircle.Color {
		t.Fatalf("the mod's circle must stay first, ours added: %+v", figs[1].Lines)
	}
	// Re-apply: exactly one set of ours
	w.Press("reapplyNames", "White")
	w.Press("reapplyNames", "White")
	if len(figs[1].Lines) != 2 {
		t.Errorf("%d lines after Re-apply, want mod + ours", len(figs[1].Lines))
	}
	// a thin line of the same colour as ours, but not our thickness, is somebody else's
	figs[4].SetLines([]ttsfake.Line{{Color: colBanner, Thickness: 0.2, Points: modCircle.Points}})
	w.Press("reapplyNames", "White")
	if len(figs[4].Lines) != 2 {
		t.Errorf("a line with our colour and another thickness is kept: %d lines", len(figs[4].Lines))
	}
}

func TestRingsOverwriteWhenLinesCannotBeRead(t *testing.T) {
	w, figs := ringWorld(t)
	figs[1].RemoveGetVectorLines()
	figs[4].RemoveGetVectorLines()
	w.Press("applyNames", "White")
	if len(figs[1].Lines) != 1 || len(figs[4].Lines) != 1 {
		t.Errorf("only ours: %d %d", len(figs[1].Lines), len(figs[4].Lines))
	}
	if strings.Count(w.Texts(), "cannot be read") != 1 {
		t.Errorf("one message about it:\n%s", w.Texts())
	}
	w.Press("reapplyNames", "White")
	if strings.Contains(w.Texts(), "cannot be read") {
		t.Errorf("said once only:\n%s", w.Texts())
	}
	if len(figs[1].Lines) != 1 {
		t.Errorf("Re-apply in overwrite mode leaves one set: %d", len(figs[1].Lines))
	}
}

func TestSwitchesHideAndShowRings(t *testing.T) {
	w, figs := ringWorld(t)
	figs[1].SetLines([]ttsfake.Line{modCircle})
	w.Press("applyNames", "White")
	all := totalLines(figs)

	btn := map[string]int{}
	for i, b := range w.Buttons {
		btn[b.ClickFunction] = i
	}
	if len(w.Buttons) != 9 {
		t.Fatalf("%d buttons, want Apply, Re-apply, Rings, Redraw and five kinds", len(w.Buttons))
	}
	if w.Buttons[2].Label != "Rings: on" || w.Buttons[btn["toggleKindB"]].Label != "B" || !w.Buttons[btn["toggleKindB"]].HasColor {
		t.Errorf("buttons: %+v", w.Buttons)
	}

	w.Press("toggleKindB", "White")
	if n := totalLines(figs); n != all-4 { // banner rings: the hero, two banners, the last figure
		t.Errorf("hiding banner leaves %d of %d lines", n, all)
	}
	if len(ours(figs[4])) != 0 || len(ours(figs[1])) != 1 {
		t.Errorf("banner figure %d lines, rider %d", len(ours(figs[4])), len(ours(figs[1])))
	}
	if w.Buttons[btn["toggleKindB"]].Label != "(B)" {
		t.Errorf("label %q", w.Buttons[btn["toggleKindB"]].Label)
	}
	if len(figs[1].Lines) != 2 {
		t.Errorf("the mod's circle stays")
	}
	w.Press("toggleKindB", "White")
	if totalLines(figs) != all {
		t.Errorf("showing banner again: %d of %d", totalLines(figs), all)
	}

	w.Press("toggleRings", "White")
	if totalLines(figs) != 0 || w.Buttons[2].Label != "Rings: off" || len(figs[1].Lines) != 1 || figs[1].Lines[0].Thickness != 0.05 {
		t.Errorf("hide all: %d lines, label %q, rider %+v", totalLines(figs), w.Buttons[2].Label, figs[1].Lines)
	}
	w.Press("toggleKindC", "White") // a kind switch while all is off draws nothing
	if totalLines(figs) != 0 {
		t.Errorf("rings are off")
	}
	w.Press("toggleKindC", "White")
	w.Press("toggleRings", "White")
	if totalLines(figs) != all {
		t.Errorf("show all: %d of %d", totalLines(figs), all)
	}
}

func TestRedrawRestoresRingsWipedByTheMod(t *testing.T) {
	w, figs := ringWorld(t)
	w.Press("applyNames", "White")
	n := totalLines(figs)
	figs[4].SetLines(nil)
	figs[0].SetLines([]ttsfake.Line{modCircle})
	w.Press("redrawRings", "White")
	if totalLines(figs) != n {
		t.Errorf("lines %d, had %d", totalLines(figs), n)
	}
	if len(ours(figs[4])) != 1 {
		t.Errorf("the banner ring is back")
	}
	if len(figs[0].Lines) != 1+4 {
		t.Errorf("the hero has the mod's circle and four rings: %d", len(figs[0].Lines))
	}
	mustContain(t, w.Texts(), "Rings drawn for 8 figures")
}

func TestRingsAreRedrawnThreeSecondsAfterLoad(t *testing.T) {
	w, figs := ringWorld(t)
	w.Press("applyNames", "White")
	saved := w.Save()

	// a new game: the same figures (same GUIDs), the mod's script clears their lines at 2 seconds
	w2 := ttsfake.New(t, Script())
	w2.Spawn(fixture(t, "rohan_list.txt"), 10, 20)
	f2 := w2.Objs
	for _, o := range f2 {
		o := o
		o.SetLines([]ttsfake.Line{modCircle})
		w2.After(2, func() { o.SetLines(nil) })
	}
	w2.Load(saved)
	if len(w2.Buttons) != 9 {
		t.Fatalf("buttons after load: %d", len(w2.Buttons))
	}
	w2.Advance(2.5)
	if totalLines(f2) != 0 {
		t.Fatalf("the mod's wipe at 2 seconds must come first")
	}
	w2.Advance(0.6)
	if totalLines(f2) != totalLines(figs) {
		t.Errorf("after 3 seconds %d lines, want %d", totalLines(f2), totalLines(figs))
	}
	if w2.Pending() != 0 {
		t.Errorf("%d timers left", w2.Pending())
	}
}

func TestSavedStateKeepsSwitches(t *testing.T) {
	w, figs := ringWorld(t)
	w.Press("applyNames", "White")
	w.Press("toggleKindE", "White")
	saved := w.Save()
	mustContain(t, saved, `"enemy":true`, `"on":true`, figs[4].GUID)

	w2 := ttsfake.New(t, Script())
	w2.Spawn(fixture(t, "rohan_list.txt"), 10, 20)
	w2.Load(saved)
	if w2.Buttons[2].Label != "Rings: on" || w2.Buttons[7].Label != "(E)" {
		t.Errorf("labels after load: %+v", w2.Buttons)
	}
	w2.Advance(3.1)
	for _, l := range ours(w2.Objs[0]) {
		if sameColour(l.Color, colEnemy) {
			t.Errorf("a hidden kind comes back after load")
		}
	}
	if len(ours(w2.Objs[0])) != 3 {
		t.Errorf("hero has %d rings, want 3 (enemy hidden)", len(ours(w2.Objs[0])))
	}
}

func TestStateChangeMovesTheRingsToTheNewObject(t *testing.T) {
	for _, destroyFirst := range []bool{false, true} {
		w, figs := ringWorld(t)
		w.Press("applyNames", "White")
		old := figs[4]
		fresh := w.ChangeState(old, destroyFirst)
		if len(fresh.Lines) != 0 {
			t.Fatal("a new state has no lines")
		}
		w.Advance(2.9)
		if len(fresh.Lines) != 0 {
			t.Errorf("destroyFirst=%v: drawn too early", destroyFirst)
		}
		w.Advance(0.2)
		if len(ours(fresh)) != 1 || !sameColour(ours(fresh)[0].Color, colBanner) {
			t.Errorf("destroyFirst=%v: the new state has %d lines", destroyFirst, len(fresh.Lines))
		}
		if strings.Contains(w.Save(), old.GUID) || !strings.Contains(w.Save(), fresh.GUID) {
			t.Errorf("the entry moves to the new GUID: %s", w.Save())
		}
	}
	// an unknown figure changing state is ignored
	w, _ := ringWorld(t)
	other := w.Add("Somebody", 0, 0)
	n := w.ChangeState(other, false)
	w.Advance(4)
	if len(n.Lines) != 0 || len(n.LineSets) != 0 {
		t.Errorf("an unknown figure got lines")
	}
}

func TestDestroyedFigureLeavesTheTableAndIsNotTouched(t *testing.T) {
	w, figs := ringWorld(t)
	w.Press("applyNames", "White")
	gone := figs[5]
	sets := len(gone.LineSets)
	w.Destroy(gone)
	w.Press("redrawRings", "White") // the fake reports any call on a destroyed object
	w.Press("toggleRings", "White")
	w.Press("reapplyNames", "White")
	w.Advance(10)
	if len(gone.LineSets) != sets {
		t.Errorf("the destroyed figure was touched")
	}
	if strings.Contains(w.Save(), gone.GUID) {
		t.Errorf("the destroyed figure is still remembered")
	}
}

func TestReapplyCleansUpFirst(t *testing.T) {
	w, figs := ringWorld(t)
	figs[1].SetLines([]ttsfake.Line{modCircle})
	w.Press("applyNames", "White")

	// the company changed: the banner of unit 5 is gone, the hero has one ring, one rider is not in the payload any more
	body := fixture(t, "rohan_names_v2.json")
	body = strings.Replace(body, `"d":"Banner 1","a":[{"r":3,"k":"banner","w":"a","n":"Banner"}]`, `"d":"Banner 1"`, 1)
	body = strings.Replace(body, `"a":[{"r":3,"k":"banner","w":"x","n":"Claim the Throne"},{"r":6,"k":"command","w":"a","n":"Stand Fast, With Me!"},{"r":6,"k":"enemy","w":"a","n":"Furious Tirade"},{"r":3,"k":"support","w":"a","n":"Remember Today"}]`, `"a":[{"r":6,"k":"command","w":"a","n":"Stand Fast"}]`, 1)
	body = strings.Replace(body, `"d":"Rider 1","a":[{"r":6,"k":"command","w":"a","n":"Stand Fast, With Me!"}]`, `"d":"Rider 1"`, 1)
	if strings.Count(body, `"a":[`) != 6 {
		t.Fatalf("test payload not edited as expected (%d)", strings.Count(body, `"a":[`))
	}
	w.SetNames(body, "White")
	w.Press("reapplyNames", "White")
	if len(ours(figs[0])) != 1 || len(ours(figs[4])) != 0 || len(ours(figs[1])) != 0 {
		t.Errorf("hero %d, old banner %d, rider %d", len(ours(figs[0])), len(ours(figs[4])), len(ours(figs[1])))
	}
	if len(figs[1].Lines) != 1 || figs[1].Lines[0].Thickness != 0.05 {
		t.Errorf("the mod's circle of the rider stays alone: %+v", figs[1].Lines)
	}
	if len(ours(figs[5])) != 1 {
		t.Errorf("the other banner keeps its ring")
	}

	// a v1 payload with Re-apply only cleans up
	w.SetNames(fixture(t, "rohan_names.json"), "White")
	w.Press("reapplyNames", "White")
	if totalLines(figs) != 0 || len(figs[1].Lines) != 1 {
		t.Errorf("v1 Re-apply leaves %d rings", totalLines(figs))
	}
	if strings.Contains(w.Texts(), "ings") {
		t.Errorf("no word about rings for a v1 payload: %s", w.Texts())
	}
}

func TestApplyTwiceDrawsTheSameRings(t *testing.T) {
	w, figs := ringWorld(t)
	w.Press("applyNames", "White")
	before := map[string]int{}
	var first [][]ttsfake.Line
	for _, f := range figs {
		before[f.GUID] = f.Calls
		first = append(first, append([]ttsfake.Line(nil), f.Lines...))
	}
	w.Press("applyNames", "White")
	w.Press("reapplyNames", "White")
	for i, f := range figs {
		if len(f.Lines) != len(first[i]) {
			t.Fatalf("figure %d: %d lines, then %d", i, len(first[i]), len(f.Lines))
		}
		for j := range f.Lines {
			if f.Lines[j].Color != first[i][j].Color || f.Lines[j].Thickness != first[i][j].Thickness || len(f.Lines[j].Points) != len(first[i][j].Points) || f.Lines[j].Points[5] != first[i][j].Points[5] {
				t.Errorf("figure %d line %d changed", i, j)
			}
		}
	}
	for i, f := range figs {
		if i == 7 && len(f.LineSets) != 0 {
			t.Errorf("a mount gets no rings")
		}
	}
}

func TestRingCaps(t *testing.T) {
	// eight rings on one figure: the last two is left out
	w := ttsfake.New(t, Script())
	f := w.Add("Hero: ", 0, 0)
	w.SetNames(`{"v":2,"figures":[{"k":"hero:","l":"Hero: ","u":1,"n":"H","d":"","a":[`+
		`{"r":3,"k":"banner","w":"a","n":"a"},{"r":6,"k":"command","w":"a","n":"b"},{"r":3,"k":"support","w":"a","n":"c"},{"r":6,"k":"enemy","w":"a","n":"d"},{"r":6,"k":"utility","w":"a","n":"e"},{"r":9,"k":"banner","w":"a","n":"f"},{"r":9,"k":"support","w":"a","n":"g"},{"r":12,"k":"support","w":"a","n":"h"}]}]}`, "White")
	w.Press("applyNames", "White")
	if len(ours(f)) != 6 {
		t.Errorf("%d rings, want 6", len(ours(f)))
	}
	mustContain(t, w.Texts(), "2 rings left out (limit 6 per figure, 200 in all)")

	// 51 figures with 4 rings each: the 201st ring on is left out
	w = ttsfake.New(t, Script())
	var figs []*ttsfake.Obj
	var sb strings.Builder
	sb.WriteString(`{"v":2,"figures":[`)
	for i := 0; i < 51; i++ {
		name := "Unit " + string(rune('A'+i/26)) + string(rune('a'+i%26)) + ":"
		figs = append(figs, w.Add(name, float64(i%6)*1.1, float64(i/6)*1.1))
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"k":"` + strings.ToLower(name) + `","u":` + itoa(i+1) + `,"n":"n","d":"","a":[`)
		sb.WriteString(`{"r":3,"k":"banner","w":"a","n":"a"},{"r":6,"k":"command","w":"a","n":"b"},{"r":3,"k":"support","w":"a","n":"c"},{"r":6,"k":"enemy","w":"a","n":"d"}]}`)
	}
	sb.WriteString(`]}`)
	w.SetNames(sb.String(), "White")
	w.Press("applyNames", "White")
	w.Advance(1)
	if n := totalLines(figs); n != 200 {
		t.Errorf("%d rings drawn, want 200", n)
	}
	mustContain(t, w.Texts(), "4 rings left out")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for ; n > 0; n /= 10 {
		d = append([]byte{byte('0' + n%10)}, d...)
	}
	return string(d)
}

func TestRingsAreDrawnInChunks(t *testing.T) {
	w := ttsfake.New(t, Script())
	var figs []*ttsfake.Obj
	var sb strings.Builder
	sb.WriteString(`{"v":2,"figures":[`)
	for i := 0; i < 25; i++ {
		figs = append(figs, w.Add("Soldier: ", float64(i%6)*1.1, float64(i/6)*1.1))
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"k":"soldier:","l":"Soldier: ","u":` + itoa(i+1) + `,"n":"n","d":"","a":[{"r":3,"k":"banner","w":"a","n":"a"}]}`)
	}
	sb.WriteString(`]}`)
	w.SetNames(sb.String(), "White")
	w.Press("applyNames", "White")
	if n := totalLines(figs); n != 10 {
		t.Fatalf("%d figures drawn in the first frame, want 10", n)
	}
	if strings.Contains(w.Texts(), "Rings drawn") {
		t.Errorf("the summary comes at the end")
	}
	w.Advance(0.0167)
	if n := totalLines(figs); n != 20 {
		t.Errorf("%d after two frames, want 20", n)
	}
	w.Advance(0.1)
	if n := totalLines(figs); n != 25 {
		t.Errorf("%d at the end, want 25", n)
	}
	mustContain(t, w.Texts(), "Rings drawn for 25 figures (25 rings).")
}

func TestPayloadVersionsAndRings(t *testing.T) {
	// v1 draws nothing and says nothing about rings, even when a figure carries an "a"
	w, figs := ringWorld(t)
	v1 := strings.Replace(fixture(t, "rohan_names_v2.json"), `"v":2`, `"v":1`, 1)
	w.SetNames(v1, "White")
	w.Press("applyNames", "White")
	for i, f := range figs {
		if len(f.LineSets) != 0 {
			t.Fatalf("figure %d got lines from a v1 payload", i)
		}
	}
	if strings.Contains(w.Texts(), "ings") {
		t.Errorf("v1 prints nothing about rings:\n%s", w.Texts())
	}
	w.Advance(5)

	// v2 without rings: nothing; v3: the helper is older than the app
	w, figs = ringWorld(t)
	w.SetNames(fixture(t, "rohan_names.json"), "White")
	w.SetNames(strings.Replace(fixture(t, "rohan_names.json"), `"v":1`, `"v":2`, 1), "White")
	w.Press("applyNames", "White")
	if totalLines(figs) != 0 || strings.Contains(w.Texts(), "ings") {
		t.Errorf("a v2 payload without rings draws nothing:\n%s", w.Texts())
	}
	mustContain(t, w.Texts(), "Named 9 of 9 figures.")

	w.SetNames(strings.Replace(fixture(t, "rohan_names_v2.json"), `"v":2`, `"v":3`, 1), "White")
	w.Press("applyNames", "White")
	if len(w.Msgs) != 1 || !strings.Contains(w.Msgs[0].Text, "older than the app (names format 3)") || totalLines(figs) != 0 {
		t.Errorf("v3: %q", w.Texts())
	}
}

func TestRingsOnlyForMatchedFiguresAndBadRingsAreIgnored(t *testing.T) {
	w := ttsfake.New(t, Script())
	f := w.Add("Hero: ", 0, 0)
	other := w.Add("Stranger", 5, 5)
	w.SetNames(`{"v":2,"figures":[{"k":"hero:","l":"Hero: ","u":1,"n":"H","d":"","a":[`+
		`{"r":0,"k":"banner"},{"r":-3,"k":"banner"},{"r":99,"k":"banner"},{"r":"x","k":"banner"},{"r":3,"k":"mystery"},{"r":3},"junk",{"r":6,"k":"command"}]}]}`, "White")
	w.Press("applyNames", "White")
	if len(ours(f)) != 1 || len(other.LineSets) != 0 {
		t.Errorf("hero %d, stranger %d", len(ours(f)), len(other.LineSets))
	}
}

func TestPayloadPaletteAndCap(t *testing.T) {
	w := ttsfake.New(t, Script())
	f := w.Add("Hero: ", 0, 0)
	w.SetNames(`{"v":2,"auras":{"cap":2,"kinds":[{"id":"banner","rgb":"112233"},{"id":"bogus","rgb":"zzzzzz"}]},"figures":[{"k":"hero:","l":"Hero: ","u":1,"n":"H","d":"","a":[`+
		`{"r":3,"k":"banner"},{"r":6,"k":"command"},{"r":3,"k":"support"}]}]}`, "White")
	w.Press("applyNames", "White")
	ls := ours(f)
	if len(ls) != 2 || !sameColour(ls[0].Color, [3]float64{0x11 / 255.0, 0x22 / 255.0, 0x33 / 255.0}) {
		t.Fatalf("payload palette and cap: %+v", ls)
	}
	// our lines are recognised under the payload palette and the built-in one
	w.Press("reapplyNames", "White")
	if len(f.Lines) != 2 {
		t.Errorf("%d lines after Re-apply", len(f.Lines))
	}
	// the palette survives a save
	w2 := ttsfake.New(t, Script())
	g := w2.Add("Hero: ", 0, 0)
	w2.Load(w.Save())
	w2.Advance(3.1)
	if len(ours(g)) != 2 || !sameColour(ours(g)[0].Color, [3]float64{0x11 / 255.0, 0x22 / 255.0, 0x33 / 255.0}) {
		t.Errorf("after load: %+v", g.Lines)
	}
}

func TestRingsNeverMoveOrDestroy(t *testing.T) {
	// the fake objects have no setPosition, clone or destruct: any call would have failed above.
	// Here: the object tables really lack them, and the script only touches lines through the one method.
	w, figs := ringWorld(t)
	w.Press("applyNames", "White")
	for _, name := range []string{"setPosition", "clone", "destruct"} {
		if w.HasMethod(figs[0], name) {
			t.Errorf("the fake object has %s", name)
		}
	}
}
