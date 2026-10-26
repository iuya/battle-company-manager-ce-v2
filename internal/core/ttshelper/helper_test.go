package ttshelper

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestSavedObjectJSON(t *testing.T) {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(SavedObjectJSON(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	// key names of the envelope and of the block, taken from the owner's real Saved Objects and saves
	wantEnv := "ObjectStates,Date,GameComplexity,GameMode,GameType,Gravity,LuaScript,LuaScriptState,Note,PlayArea,SaveName,Sky,TabStates,Table,Tags,VersionNumber,XmlUI"
	if got := sortedKeys(env); got != sortedList(wantEnv) {
		t.Errorf("envelope keys\n got %s\nwant %s", got, sortedList(wantEnv))
	}
	var objs []map[string]json.RawMessage
	if err := json.Unmarshal(env["ObjectStates"], &objs); err != nil || len(objs) != 1 {
		t.Fatalf("ObjectStates: %v, %d", err, len(objs))
	}
	wantObj := "GUID,Name,Transform,Nickname,Description,GMNotes,AltLookAngle,ColorDiffuse,LayoutGroupSortIndex,Value,Locked,Grid,Snap,IgnoreFoW,MeasureMovement,DragSelectable,Autoraise,Sticky,Tooltip,GridProjection,HideWhenFaceDown,Hands,Tags,LuaScript,LuaScriptState,XmlUI"
	if got := sortedKeys(objs[0]); got != sortedList(wantObj) {
		t.Errorf("object keys\n got %s\nwant %s", got, sortedList(wantObj))
	}
	var o struct {
		Name, Nickname, Description, LuaScript string
		Locked                                 bool
		Tags                                   []string
	}
	b, _ := json.Marshal(objs[0])
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatal(err)
	}
	if o.Name != "BlockRectangle" || o.Nickname != ObjectName || !o.Locked {
		t.Errorf("object: %+v", o)
	}
	if o.LuaScript != Script() || !strings.Contains(o.LuaScript, "function applyNames") {
		t.Errorf("the script is not embedded as is")
	}
	for _, s := range []string{"ArmyNames", "Apply names", "Re-apply"} {
		if !strings.Contains(o.Description, s) {
			t.Errorf("how-to lacks %q", s)
		}
	}
	if len(o.Tags) != 1 {
		t.Errorf("tags: %v", o.Tags)
	}
	if !bytes.Equal(SavedObjectJSON(), SavedObjectJSON()) {
		t.Errorf("not constant")
	}
}

func TestSavedObjectKeysMatchTheRealOnesWhenAvailable(t *testing.T) {
	dir := filepath.Join(os.Getenv("HOME"), "Library", "Tabletop Simulator", "Saves", "Saved Objects")
	b, err := os.ReadFile(filepath.Join(dir, "El dado.json"))
	if err != nil {
		t.Skip("no Tabletop Simulator saved objects on this machine")
	}
	var real, ours map[string]json.RawMessage
	_ = json.Unmarshal(b, &real)
	_ = json.Unmarshal(SavedObjectJSON(), &ours)
	if sortedKeys(real) != sortedKeys(ours) {
		t.Errorf("envelope keys differ from a real Saved Object:\n real %s\n ours %s", sortedKeys(real), sortedKeys(ours))
	}
}

func TestPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(PNG()))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Errorf("size %v", b)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Errorf("corner should be transparent")
	}
	if _, _, _, a := img.At(128, 128).RGBA(); a == 0 {
		t.Errorf("centre should be drawn")
	}
}

func TestZip(t *testing.T) {
	zr, err := zip.NewReader(bytes.NewReader(Zip()), int64(len(Zip())))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]byte{}
	for _, f := range zr.File {
		r, _ := f.Open()
		got[f.Name], _ = io.ReadAll(r)
		r.Close()
	}
	if len(got) != 2 || !bytes.Equal(got[ObjectFileName], SavedObjectJSON()) || !bytes.Equal(got[IconFileName], PNG()) {
		t.Errorf("zip files: %v", len(got))
	}
	if ObjectFileName != "BC names helper.json" || IconFileName != "BC names helper.png" {
		t.Errorf("file names")
	}
	if !bytes.Equal(Zip(), Zip()) || !strings.HasPrefix(ETag(), `"`) || len(ETag()) < 10 {
		t.Errorf("constant content and strong etag expected: %s", ETag())
	}
}

func TestShowLuaIsTheScript(t *testing.T) {
	if ShowLua() != Script() || !strings.Contains(ShowLua(), "onLoad") {
		t.Errorf("fallback text")
	}
}

func sortedKeys(m map[string]json.RawMessage) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

func sortedList(s string) string {
	ks := strings.Split(s, ",")
	sort.Strings(ks)
	return strings.Join(ks, ",")
}
