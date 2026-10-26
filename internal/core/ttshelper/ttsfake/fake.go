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

type Obj struct {
	GUID    string
	Name    string
	Desc    string
	Notes   string
	Tags    map[string]bool
	X, Y, Z float64
	States  int
	table   *lua.LTable
	Calls   int // setters called
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

	self := L.NewTable()
	self.RawSetString("createButton", L.NewFunction(func(L *lua.LState) int { return 0 }))
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
	o := &Obj{GUID: fmt.Sprintf("f%05d", w.nextGUID), Name: name, X: x, Y: 1, Z: z, Tags: map[string]bool{}}
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
	o.table = t
	w.Objs = append(w.Objs, o)
	return o
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
