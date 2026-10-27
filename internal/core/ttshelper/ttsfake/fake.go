// Package ttsfake is a fake Tabletop Simulator for testing the helper script: just enough of the
// scripting API, run by gopher-lua (a test-support package; the server does not import it).
// Objects are Lua tables whose methods are Go closures; they have no setPosition, clone or
// destruct, so a helper that moved, cloned or destroyed anything fails its test.
package ttsfake

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// This file is a fake Tabletop Simulator: just enough of the scripting API for the helper, with
// the objects as Lua tables whose methods are Go closures. An object has no setPosition, clone
// or destruct, so a helper that moved, cloned or destroyed anything fails its test.

// Line is one vector line of an object (object-local points).
type Line struct {
	Points    [][3]float64
	Color     [3]float64
	Thickness float64
}

type Obj struct {
	GUID    string
	Name    string
	Desc    string
	Notes   string
	Tags    map[string]bool
	X, Y, Z float64
	States  int
	table   *lua.LTable
	Calls   int // setters called (not the vector lines)

	Scale     float64    // uniform scale (default 1)
	Size      [3]float64 // getBoundsNormalized().size (default a 25 mm base, 0.98 wide)
	Lines     []Line     // the current vector lines
	LineSets  [][]Line   // every setVectorLines call, in order, copied
	NoLines   bool       // the object has no getVectorLines (overwrite mode)
	Destroyed bool
}

// Button is a button created on the helper block or an edit of one.
type Button struct {
	Index                         int
	Label, ClickFunction, Tooltip string
	Color                         [3]float64
	HasColor                      bool
}

type timer struct {
	due float64
	seq int
	run func()
}

type Message struct {
	To   string // "all" or the colour
	Text string
	RGB  [3]float64
}

type Tab struct {
	Title, Body, Color string
}

type World struct {
	t        *testing.T
	L        *lua.LState
	Objs     []*Obj
	Selected map[string][]*Obj
	Tabs     []Tab
	Msgs     []Message
	nextGUID int
	self     *lua.LTable

	Buttons []Button // createButton calls in order; editButton changes the label in place
	Now     float64  // virtual seconds; a frame is 1/60
	timers  []timer
	seq     int
}

func New(t *testing.T, script string) *World {
	t.Helper()
	w := &World{t: t, Selected: map[string][]*Obj{}, nextGUID: 1}
	L := lua.NewState()
	t.Cleanup(L.Close)
	w.L = L

	str := func(L *lua.LState, i int) string { return L.OptString(i, "") }
	rgbOf := func(L *lua.LState, i int) [3]float64 {
		var c [3]float64
		if tb, ok := L.Get(i).(*lua.LTable); ok {
			for k := 0; k < 3; k++ {
				if n, ok := tb.RawGetInt(k + 1).(lua.LNumber); ok {
					c[k] = float64(n)
				}
			}
		}
		return c
	}
	L.SetGlobal("broadcastToAll", L.NewFunction(func(L *lua.LState) int {
		w.Msgs = append(w.Msgs, Message{"all", str(L, 1), rgbOf(L, 2)})
		return 0
	}))
	L.SetGlobal("broadcastToColor", L.NewFunction(func(L *lua.LState) int {
		w.Msgs = append(w.Msgs, Message{str(L, 2), str(L, 1), rgbOf(L, 3)})
		return 0
	}))
	L.SetGlobal("getObjects", L.NewFunction(func(L *lua.LState) int {
		L.Push(w.list(w.Objs))
		return 1
	}))
	L.SetGlobal("getNotebookTabs", L.NewFunction(func(L *lua.LState) int {
		out := L.NewTable()
		for i, tb := range w.Tabs {
			e := L.NewTable()
			e.RawSetString("index", lua.LNumber(i))
			e.RawSetString("title", lua.LString(tb.Title))
			e.RawSetString("body", lua.LString(tb.Body))
			e.RawSetString("color", lua.LString(tb.Color))
			out.Append(e)
		}
		L.Push(out)
		return 1
	}))
	players := L.NewTable()
	for _, c := range []string{"White", "Red", "Blue"} {
		color := c
		p := L.NewTable()
		p.RawSetString("getSelectedObjects", L.NewFunction(func(L *lua.LState) int {
			L.Push(w.list(w.Selected[color]))
			return 1
		}))
		players.RawSetString(color, p)
	}
	L.SetGlobal("Player", players)

	js := L.NewTable()
	js.RawSetString("decode", L.NewFunction(func(L *lua.LState) int {
		var v any
		if err := json.Unmarshal([]byte(L.CheckString(1)), &v); err != nil {
			L.RaiseError("JSON.decode: %v", err)
		}
		L.Push(toLua(L, v))
		return 1
	}))
	L.SetGlobal("JSON", js)

	js.RawSetString("encode", L.NewFunction(func(L *lua.LState) int {
		b, err := json.Marshal(fromLua(L.CheckAny(1)))
		if err != nil {
			L.RaiseError("JSON.encode: %v", err)
		}
		L.Push(lua.LString(b))
		return 1
	}))
	L.SetGlobal("getObjectFromGUID", L.NewFunction(func(L *lua.LState) int {
		g := L.CheckString(1)
		for _, o := range w.Objs {
			if o.GUID == g && !o.Destroyed {
				L.Push(o.table)
				return 1
			}
		}
		L.Push(lua.LNil)
		return 1
	}))
	wait := L.NewTable()
	wait.RawSetString("time", L.NewFunction(func(L *lua.LState) int {
		w.schedule(L.CheckFunction(1), float64(L.CheckNumber(2)))
		return 0
	}))
	wait.RawSetString("frames", L.NewFunction(func(L *lua.LState) int {
		w.schedule(L.CheckFunction(1), float64(L.OptNumber(2, 1))/60)
		return 0
	}))
	L.SetGlobal("Wait", wait)

	self := L.NewTable()
	self.RawSetString("createButton", L.NewFunction(func(L *lua.LState) int {
		p := L.CheckTable(1)
		b := Button{Index: len(w.Buttons)}
		b.Label = lua.LVAsString(p.RawGetString("label"))
		b.ClickFunction = lua.LVAsString(p.RawGetString("click_function"))
		b.Tooltip = lua.LVAsString(p.RawGetString("tooltip"))
		if c, ok := p.RawGetString("color").(*lua.LTable); ok {
			b.HasColor = true
			for k := 0; k < 3; k++ {
				if n, ok := c.RawGetInt(k + 1).(lua.LNumber); ok {
					b.Color[k] = float64(n)
				}
			}
		}
		w.Buttons = append(w.Buttons, b)
		return 0
	}))
	self.RawSetString("editButton", L.NewFunction(func(L *lua.LState) int {
		p := L.CheckTable(1)
		if i, ok := p.RawGetString("index").(lua.LNumber); ok && int(i) >= 0 && int(i) < len(w.Buttons) {
			if l, ok := p.RawGetString("label").(lua.LString); ok {
				w.Buttons[int(i)].Label = string(l)
			}
		}
		return 0
	}))
	self.RawSetString("getGUID", L.NewFunction(func(L *lua.LState) int { L.Push(lua.LString("bc4e01")); return 1 }))
	L.SetGlobal("self", self)
	w.self = self

	if err := L.DoString(script); err != nil {
		t.Fatalf("helper does not load: %v", err)
	}
	return w
}

func toLua(L *lua.LState, v any) lua.LValue {
	switch x := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(x)
	case float64:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	case []any:
		t := L.NewTable()
		for _, e := range x {
			t.Append(toLua(L, e))
		}
		return t
	case map[string]any:
		t := L.NewTable()
		for k, e := range x {
			t.RawSetString(k, toLua(L, e))
		}
		return t
	}
	return lua.LNil
}

func (w *World) list(objs []*Obj) *lua.LTable {
	t := w.L.NewTable()
	for _, o := range objs {
		t.Append(o.table)
	}
	return t
}

// add puts a figure on the table.
func (w *World) Add(name string, x, z float64) *Obj {
	L := w.L
	o := &Obj{GUID: fmt.Sprintf("f%05d", w.nextGUID), Name: name, X: x, Y: 1, Z: z, Tags: map[string]bool{},
		Scale: 1, Size: [3]float64{0.98, 1.5, 0.98}}
	w.nextGUID++
	t := L.NewTable()
	fn := func(f func(L *lua.LState) int) lua.LValue { return L.NewFunction(f) }
	t.RawSetString("getGUID", fn(func(L *lua.LState) int { L.Push(lua.LString(o.GUID)); return 1 }))
	t.RawSetString("getName", fn(func(L *lua.LState) int { L.Push(lua.LString(o.Name)); return 1 }))
	t.RawSetString("setName", fn(func(L *lua.LState) int { o.Name = L.CheckString(1); o.Calls++; return 0 }))
	t.RawSetString("getDescription", fn(func(L *lua.LState) int { L.Push(lua.LString(o.Desc)); return 1 }))
	t.RawSetString("setDescription", fn(func(L *lua.LState) int { o.Desc = L.CheckString(1); o.Calls++; return 0 }))
	t.RawSetString("getGMNotes", fn(func(L *lua.LState) int { L.Push(lua.LString(o.Notes)); return 1 }))
	t.RawSetString("setGMNotes", fn(func(L *lua.LState) int { o.Notes = L.CheckString(1); o.Calls++; return 0 }))
	t.RawSetString("addTag", fn(func(L *lua.LState) int { o.Tags[L.CheckString(1)] = true; o.Calls++; return 0 }))
	t.RawSetString("hasTag", fn(func(L *lua.LState) int { L.Push(lua.LBool(o.Tags[L.CheckString(1)])); return 1 }))
	t.RawSetString("getPosition", fn(func(L *lua.LState) int {
		p := L.NewTable()
		p.RawSetString("x", lua.LNumber(o.X))
		p.RawSetString("y", lua.LNumber(o.Y))
		p.RawSetString("z", lua.LNumber(o.Z))
		L.Push(p)
		return 1
	}))
	t.RawSetString("getStates", fn(func(L *lua.LState) int {
		s := L.NewTable()
		for i := 0; i < o.States; i++ {
			s.Append(lua.LString("state"))
		}
		L.Push(s)
		return 1
	}))
	t.RawSetString("getScale", fn(func(L *lua.LState) int {
		p := L.NewTable()
		for _, k := range []string{"x", "y", "z"} {
			p.RawSetString(k, lua.LNumber(o.Scale))
		}
		L.Push(p)
		return 1
	}))
	t.RawSetString("getBoundsNormalized", fn(func(L *lua.LState) int {
		b, c, size := L.NewTable(), L.NewTable(), L.NewTable()
		for i, k := range []string{"x", "y", "z"} {
			size.RawSetString(k, lua.LNumber(o.Size[i]))
			c.RawSetString(k, lua.LNumber([3]float64{o.X, o.Y, o.Z}[i]))
		}
		b.RawSetString("center", c)
		b.RawSetString("size", size)
		L.Push(b)
		return 1
	}))
	t.RawSetString("setVectorLines", fn(func(L *lua.LState) int {
		if o.Destroyed {
			w.t.Errorf("setVectorLines on the destroyed object %s", o.GUID)
		}
		o.Lines = linesFromLua(L.CheckTable(1))
		o.LineSets = append(o.LineSets, append([]Line(nil), o.Lines...))
		return 0
	}))
	if !o.NoLines {
		t.RawSetString("getVectorLines", fn(func(L *lua.LState) int {
			if len(o.Lines) == 0 {
				L.Push(lua.LNil)
				return 1
			}
			L.Push(linesToLua(L, o.Lines))
			return 1
		}))
	}
	o.table = t
	w.Objs = append(w.Objs, o)
	return o
}

// RemoveGetVectorLines makes the object lack getVectorLines (overwrite mode).
func (o *Obj) RemoveGetVectorLines() {
	o.NoLines = true
	o.table.RawSetString("getVectorLines", lua.LNil)
}

// SetLines is what the mod's own script does: replaces the lines (nil clears them).
func (o *Obj) SetLines(lines []Line) { o.Lines = lines }

func linesFromLua(t *lua.LTable) []Line {
	var out []Line
	t.ForEach(func(_, v lua.LValue) {
		lt, ok := v.(*lua.LTable)
		if !ok {
			return
		}
		var l Line
		if pts, ok := lt.RawGetString("points").(*lua.LTable); ok {
			pts.ForEach(func(_, pv lua.LValue) {
				if pt, ok := pv.(*lua.LTable); ok {
					var p [3]float64
					for i, k := range []string{"x", "y", "z"} {
						if n, ok := pt.RawGetInt(i + 1).(lua.LNumber); ok {
							p[i] = float64(n)
						} else if n, ok := pt.RawGetString(k).(lua.LNumber); ok {
							p[i] = float64(n)
						}
					}
					l.Points = append(l.Points, p)
				}
			})
		}
		if c, ok := lt.RawGetString("color").(*lua.LTable); ok {
			for i, k := range []string{"r", "g", "b"} {
				if n, ok := c.RawGetInt(i + 1).(lua.LNumber); ok {
					l.Color[i] = float64(n)
				} else if n, ok := c.RawGetString(k).(lua.LNumber); ok {
					l.Color[i] = float64(n)
				}
			}
		}
		if n, ok := lt.RawGetString("thickness").(lua.LNumber); ok {
			l.Thickness = float64(n)
		}
		out = append(out, l)
	})
	return out
}

func linesToLua(L *lua.LState, lines []Line) *lua.LTable {
	out := L.NewTable()
	for _, l := range lines {
		lt := L.NewTable()
		pts := L.NewTable()
		for _, p := range l.Points {
			pt := L.NewTable()
			pt.RawSetString("x", lua.LNumber(p[0]))
			pt.RawSetString("y", lua.LNumber(p[1]))
			pt.RawSetString("z", lua.LNumber(p[2]))
			pts.Append(pt)
		}
		lt.RawSetString("points", pts)
		c := L.NewTable()
		c.RawSetString("r", lua.LNumber(l.Color[0]))
		c.RawSetString("g", lua.LNumber(l.Color[1]))
		c.RawSetString("b", lua.LNumber(l.Color[2]))
		lt.RawSetString("color", c)
		lt.RawSetString("thickness", lua.LNumber(l.Thickness))
		out.Append(lt)
	}
	return out
}

func fromLua(v lua.LValue) any {
	switch x := v.(type) {
	case lua.LBool:
		return bool(x)
	case lua.LNumber:
		return float64(x)
	case lua.LString:
		return string(x)
	case *lua.LTable:
		if n := x.Len(); n > 0 {
			arr := make([]any, 0, n)
			for i := 1; i <= n; i++ {
				arr = append(arr, fromLua(x.RawGetInt(i)))
			}
			return arr
		}
		m := map[string]any{}
		x.ForEach(func(k, e lua.LValue) { m[lua.LVAsString(k)] = fromLua(e) })
		return m
	}
	return nil
}

func (w *World) schedule(fn *lua.LFunction, after float64) {
	w.seq++
	w.timers = append(w.timers, timer{due: w.Now + after, seq: w.seq, run: func() {
		if err := w.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}); err != nil {
			w.t.Fatalf("Wait callback: %v", err)
		}
	}})
}

// After runs a Go callback after virtual seconds (the mod's own script, for example).
func (w *World) After(seconds float64, fn func()) {
	w.seq++
	w.timers = append(w.timers, timer{due: w.Now + seconds, seq: w.seq, run: fn})
}

// Advance moves the virtual clock forward and runs the timers that fall due, in order, including
// those they schedule within the window.
func (w *World) Advance(seconds float64) {
	w.t.Helper()
	end := w.Now + seconds + 1e-9
	for {
		best := -1
		for i, tm := range w.timers {
			if tm.due <= end && (best < 0 || tm.due < w.timers[best].due || (tm.due == w.timers[best].due && tm.seq < w.timers[best].seq)) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		tm := w.timers[best]
		w.timers = append(w.timers[:best], w.timers[best+1:]...)
		if tm.due > w.Now {
			w.Now = tm.due
		}
		tm.run()
	}
	w.Now = end - 1e-9
}

// Pending is the number of timers that have not run.
func (w *World) Pending() int { return len(w.timers) }

// call runs a global function of the script when it exists.
func (w *World) call(name string, args ...lua.LValue) {
	w.t.Helper()
	fn := w.L.GetGlobal(name)
	if fn.Type() != lua.LTFunction {
		return
	}
	if err := w.L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true}, args...); err != nil {
		w.t.Fatalf("%s: %v", name, err)
	}
}

// Load calls onLoad with the saved state of an earlier Save ("" for a new game).
func (w *World) Load(saved string) {
	w.t.Helper()
	w.Buttons = nil
	w.call("onLoad", lua.LString(saved))
}

// Save calls onSave and returns the saved state.
func (w *World) Save() string {
	w.t.Helper()
	if err := w.L.CallByParam(lua.P{Fn: w.L.GetGlobal("onSave"), NRet: 1, Protect: true}); err != nil {
		w.t.Fatalf("onSave: %v", err)
	}
	s := w.L.Get(-1)
	w.L.Pop(1)
	return lua.LVAsString(s)
}

// Destroy removes the object from the table and fires onObjectDestroy.
func (w *World) Destroy(o *Obj) {
	w.t.Helper()
	o.Destroyed = true
	keep := w.Objs[:0]
	for _, x := range w.Objs {
		if x != o {
			keep = append(keep, x)
		}
	}
	w.Objs = keep
	w.call("onObjectDestroy", o.table)
}

// ChangeState replaces o by a new object with a new GUID at the same place (another state of the
// figure, without lines) and fires onObjectStateChange(new, oldGUID); the old one is destroyed
// first when destroyFirst is set (the order of the two events is not known).
func (w *World) ChangeState(o *Obj, destroyFirst bool) *Obj {
	w.t.Helper()
	n := w.Add(o.Name, o.X, o.Z)
	old := o.GUID
	if destroyFirst {
		w.Destroy(o)
		w.call("onObjectStateChange", n.table, lua.LString(old))
		return n
	}
	w.call("onObjectStateChange", n.table, lua.LString(old))
	w.Destroy(o)
	return n
}

// Spawn simulates the MESBG FTC spawner for an ArmyList text, from (x0, z0), the position the
// spawner uses as its base. Warband k (a line that is not "Nx ..." starts one) has its hero at
// x0 + (k div 6) * 10, z0 + (k mod 6) * 8; follower i of a warband is at hero x + (i mod 6) * 1.1,
// hero z + floor(i / 6) * 1.1 + 1.5. The Nickname of a figure is the text of its line. The
// figures come in list order (the real spawner walks the warbands in an unspecified order).
// An "Nx" line before any hero line is dropped, as the real one does.
func (w *World) Spawn(list string, x0, z0 float64) []*Obj {
	var out []*Obj
	nx := regexp.MustCompile(`^(\d+)x\s+(.+)`)
	band, follower := -1, 0
	var hx, hz float64
	for _, line := range strings.FieldsFunc(list, func(r rune) bool { return r == '\n' || r == '\r' }) {
		open, cl := strings.LastIndex(line, "("), strings.LastIndex(line, ")")
		if open < 0 || cl < open {
			continue
		}
		text := strings.TrimSpace(line[open+1 : cl])
		if text == "" {
			continue
		}
		if m := nx.FindStringSubmatch(text); m != nil {
			if band < 0 {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			for k := 0; k < n; k++ {
				out = append(out, w.Add(m[2], hx+float64(follower%6)*1.1, hz+math.Floor(float64(follower)/6)*1.1+1.5))
				follower++
			}
			continue
		}
		band++
		follower = 0
		hx, hz = x0+float64(band/6)*10, z0+float64(band%6)*8
		out = append(out, w.Add(text, hx, hz))
	}
	return out
}

func (w *World) SetNames(body string, color string) {
	w.Tabs = []Tab{{"ArmyNames", body, color}}
}

// Press calls a button function of the helper as the given player.
func (w *World) Press(fnName, color string) {
	w.t.Helper()
	w.Msgs = nil
	err := w.L.CallByParam(lua.P{Fn: w.L.GetGlobal(fnName), NRet: 0, Protect: true}, w.self, lua.LString(color))
	if err != nil {
		w.t.Fatalf("%s: %v", fnName, err)
	}
}

func (w *World) Texts() string {
	var b strings.Builder
	for _, m := range w.Msgs {
		b.WriteString(m.To + ": " + m.Text + "\n")
	}
	return b.String()
}

// HasMethod reports whether the object table has a method of that name.
func (w *World) HasMethod(o *Obj, name string) bool {
	return o.table.RawGetString(name) != lua.LNil
}
