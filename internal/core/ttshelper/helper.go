// Package ttshelper builds the "BC names helper" for Tabletop Simulator: a Saved Object (a
// locked block with a Lua script) that gives the figures spawned by the MESBG FTC mod the name
// and description of their card, read from the Notebook tab "ArmyNames". It holds pure
// functions only: the Lua script (embedded), the Saved Object JSON, a generated 256x256 icon
// and the zip the app offers as a download. See docs/tabletop-simulator.md, "Level B".
package ttshelper

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"sync"
	"time"
)

//go:embed helper.lua
var script string

// File names inside the zip. TTS lists a Saved Object by its file name.
const (
	ObjectFileName = "BC names helper.json"
	IconFileName   = "BC names helper.png"
	// ObjectName is the Nickname of the block.
	ObjectName = "BC names helper"
)

// howTo is the Description of the block, shown on hover.
const howTo = "Names and describes the figures of a Battle Company.\n" +
	"1. In the app, open the company's Tabletop Simulator window and press Copy names.\n" +
	"2. In the Notebook, add a tab named ArmyNames and paste.\n" +
	"3. Spawn the army with Spawn Army, drag-select it (or select nothing), press Apply names.\n" +
	"Re-apply sends the names again, also to figures that are already named.\n" +
	"Names v2 also draw aura rings on the figures: Rings shows or hides them, Redraw draws them again, B C S E U switch one kind."

// Script returns the Lua source of the helper.
func Script() string { return script }

// ShowLua returns the text of the "Show the Lua" fallback: the script to paste into the Lua
// scripting window of any block (right-click, Scripting), then Save & Play, before spawning an
// army.
func ShowLua() string { return script }

type vec struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type transform struct {
	PosX   float64 `json:"posX"`
	PosY   float64 `json:"posY"`
	PosZ   float64 `json:"posZ"`
	RotX   float64 `json:"rotX"`
	RotY   float64 `json:"rotY"`
	RotZ   float64 `json:"rotZ"`
	ScaleX float64 `json:"scaleX"`
	ScaleY float64 `json:"scaleY"`
	ScaleZ float64 `json:"scaleZ"`
}

type colour struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
}

// object is a built-in BlockRectangle; the keys and their order are those of a block in the
// owner's real saved games.
type object struct {
	GUID                 string    `json:"GUID"`
	Name                 string    `json:"Name"`
	Transform            transform `json:"Transform"`
	Nickname             string    `json:"Nickname"`
	Description          string    `json:"Description"`
	GMNotes              string    `json:"GMNotes"`
	AltLookAngle         vec       `json:"AltLookAngle"`
	ColorDiffuse         colour    `json:"ColorDiffuse"`
	LayoutGroupSortIndex int       `json:"LayoutGroupSortIndex"`
	Value                int       `json:"Value"`
	Locked               bool      `json:"Locked"`
	Grid                 bool      `json:"Grid"`
	Snap                 bool      `json:"Snap"`
	IgnoreFoW            bool      `json:"IgnoreFoW"`
	MeasureMovement      bool      `json:"MeasureMovement"`
	DragSelectable       bool      `json:"DragSelectable"`
	Autoraise            bool      `json:"Autoraise"`
	Sticky               bool      `json:"Sticky"`
	Tooltip              bool      `json:"Tooltip"`
	GridProjection       bool      `json:"GridProjection"`
	HideWhenFaceDown     bool      `json:"HideWhenFaceDown"`
	Hands                bool      `json:"Hands"`
	Tags                 []string  `json:"Tags"`
	LuaScript            string    `json:"LuaScript"`
	LuaScriptState       string    `json:"LuaScriptState"`
	XmlUI                string    `json:"XmlUI"`
}

// envelope is the file TTS writes for a Saved Object.
type envelope struct {
	SaveName       string         `json:"SaveName"`
	Date           string         `json:"Date"`
	VersionNumber  string         `json:"VersionNumber"`
	GameMode       string         `json:"GameMode"`
	GameType       string         `json:"GameType"`
	GameComplexity string         `json:"GameComplexity"`
	Tags           []string       `json:"Tags"`
	Gravity        float64        `json:"Gravity"`
	PlayArea       float64        `json:"PlayArea"`
	Table          string         `json:"Table"`
	Sky            string         `json:"Sky"`
	Note           string         `json:"Note"`
	TabStates      map[string]any `json:"TabStates"`
	LuaScript      string         `json:"LuaScript"`
	LuaScriptState string         `json:"LuaScriptState"`
	XmlUI          string         `json:"XmlUI"`
	ObjectStates   []object       `json:"ObjectStates"`
}

// SavedObjectJSON returns the content of "BC names helper.json". The result is constant for a
// given build.
func SavedObjectJSON() []byte {
	e := envelope{
		Tags:      []string{},
		Gravity:   0.5,
		PlayArea:  0.5,
		TabStates: map[string]any{},
		ObjectStates: []object{{
			GUID: "bc4e01",
			Name: "BlockRectangle",
			Transform: transform{
				PosY: 1, ScaleX: 1, ScaleY: 1, ScaleZ: 1,
			},
			Nickname:       ObjectName,
			Description:    howTo,
			ColorDiffuse:   colour{R: 0.86, G: 0.62, B: 0.1},
			Locked:         true,
			DragSelectable: true,
			Sticky:         true,
			Tooltip:        true,
			Tags:           []string{"BCHelper"},
			LuaScript:      script,
		}},
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(e); err != nil {
		panic("ttshelper: " + err.Error()) // only fixed, encodable values
	}
	return b.Bytes()
}

// PNG returns the 256x256 icon of the Saved Object, drawn here (no external asset): a dark
// rounded plate with a gold banner and three figure marks.
func PNG() []byte {
	const n = 256
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	bg := color.NRGBA{R: 38, G: 40, B: 52, A: 255}
	gold := color.NRGBA{R: 219, G: 158, B: 26, A: 255}
	light := color.NRGBA{R: 240, G: 236, B: 224, A: 255}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			// rounded corners, radius 36
			if inCorner(x, y, n, 36) {
				continue
			}
			c := bg
			switch {
			case x >= 28 && x < 228 && y >= 28 && y < 88: // banner
				c = gold
			case y >= 120 && y < 220 && figure(x, 64, y):
				c = light
			case y >= 120 && y < 220 && figure(x, 128, y):
				c = light
			case y >= 120 && y < 220 && figure(x, 192, y):
				c = light
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic("ttshelper: " + err.Error())
	}
	return b.Bytes()
}

func inCorner(x, y, n, r int) bool {
	cx, cy := x, y
	if x < r {
		cx = r
	} else if x >= n-r {
		cx = n - r - 1
	}
	if y < r {
		cy = r
	} else if y >= n-r {
		cy = n - r - 1
	}
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy > r*r
}

// figure reports whether (x, y) is on a simple pawn centred on cx: a head and a base.
func figure(x, cx, y int) bool {
	dx := x - cx
	if y < 150 { // head: circle centre y=138
		dy := y - 138
		return dx*dx+dy*dy <= 14*14
	}
	if y < 200 { // body narrowing to the top
		w := 10 + (y-150)*12/50
		return dx >= -w && dx <= w
	}
	return dx >= -22 && dx <= 22 // base
}

var (
	zipOnce  sync.Once
	zipBytes []byte
	zipTag   string
)

// Zip returns the download: "BC names helper.json" and "BC names helper.png". The content is
// constant (fixed timestamps), so it can carry an ETag.
func Zip() []byte {
	zipOnce.Do(buildZip)
	return zipBytes
}

// ETag is the strong entity tag of Zip, quotes included.
func ETag() string {
	zipOnce.Do(buildZip)
	return zipTag
}

func buildZip() {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	stamp := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	for _, f := range []struct {
		name string
		data []byte
	}{{ObjectFileName, SavedObjectJSON()}, {IconFileName, PNG()}} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: stamp})
		if err != nil {
			panic("ttshelper: " + err.Error())
		}
		if _, err := w.Write(f.data); err != nil {
			panic("ttshelper: " + err.Error())
		}
	}
	if err := zw.Close(); err != nil {
		panic("ttshelper: " + err.Error())
	}
	zipBytes = b.Bytes()
	sum := sha256.Sum256(zipBytes)
	zipTag = `"` + hex.EncodeToString(sum[:8]) + `"`
}
