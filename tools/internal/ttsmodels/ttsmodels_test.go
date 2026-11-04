package ttsmodels

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/tools/internal/repo"
)

// fake is a hand-written save with no mod content.
const fake = `{"ObjectStates": [
  {"GUID": "aaaaaa", "Nickname": "Table"},
  {"GUID": "bbbbbb", "ContainedObjects": [{"GUID": "24f6d0", "ContainedObjects": [
    {"Nickname": "Test Warrior: Shield"}, {"Nickname": "test  warrior: shield"}, {"Nickname": "Test Hero:"}, {"Nickname": ""}, {"Nickname": "Test Hero: Horse, Bow"}]}]}
]}`

func save(t *testing.T, raw string) object {
	t.Helper()
	var s object
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestKeysAreStrippedDeduplicatedAndOrdered(t *testing.T) {
	got, err := ModelKeys(save(t, fake), BagGUID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Test Warrior: Shield", "Test Hero:", "Test Hero: Horse, Bow"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBagIsFoundWhenNested(t *testing.T) {
	bag := FindBag(save(t, fake)["ObjectStates"].([]any), BagGUID)
	if bag == nil || bag["GUID"] != BagGUID {
		t.Fatalf("bag not found: %v", bag)
	}
}

func TestMissingOrEmptyBagIsAnError(t *testing.T) {
	for name, tc := range map[string]struct{ raw, guid string }{
		"no bag":       {`{"ObjectStates": [{"GUID": "x"}]}`, BagGUID},
		"other guid":   {fake, "ffffff"},
		"no states":    {`{}`, BagGUID},
		"empty bag":    {`{"ObjectStates": [{"GUID": "24f6d0", "ContainedObjects": [{"Nickname": " "}]}]}`, BagGUID},
		"bag no items": {`{"ObjectStates": [{"GUID": "24f6d0"}]}`, BagGUID},
	} {
		if _, err := ModelKeys(save(t, tc.raw), tc.guid); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestOddEntriesAreSkippedNotFatal(t *testing.T) {
	s := save(t, `{"ObjectStates": [5, "x", {"GUID": "24f6d0", "ContainedObjects": [7, {"Nickname": 12}, {"Nickname": null}, {"Nickname": "Real: One"}]}]}`)
	got, err := ModelKeys(s, BagGUID)
	if err != nil || !reflect.DeepEqual(got, []string{"12", "Real: One"}) {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRefusesToWriteOutsideSourcesTTS(t *testing.T) {
	root := repo.Root()
	for _, p := range []string{"definitions/models.json", "sources/models.json", "sources/tts/../models.json", "sources/ttsx/models.json"} {
		if err := CheckOutput(root, filepath.Join(root, p)); err == nil {
			t.Errorf("%s was accepted", p)
		}
	}
}

func TestOutputIsIgnoredByGit(t *testing.T) {
	root := repo.Root()
	if err := CheckOutput(root, filepath.Join(root, "sources/tts/models.json")); err != nil {
		t.Fatalf("the real output path must be accepted: %v", err)
	}
}

func TestRefusesWhenGitDoesNotIgnore(t *testing.T) {
	// A throw-away repository with no ignore rules: sources/tts/ is the right place but git would track the file.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sources", "tts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CheckOutput(dir, filepath.Join(dir, "sources", "tts", "models.json")); err == nil {
		t.Fatal("accepted a path git does not ignore")
	}
}

func TestWriteFormat(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sub", "models.json")
	if err := Write(out, []string{"A: b", `Q"uote`, "Élan"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(out)
	if want := "[\n\"A: b\",\n\"Q\\\"uote\",\n\"Élan\"\n]"; string(raw) != want {
		t.Fatalf("got %q want %q", raw, want)
	}
	var back []string
	if err := json.Unmarshal(raw, &back); err != nil || len(back) != 3 {
		t.Fatalf("output is not a JSON array of strings: %v", err)
	}
}

func TestNewestSave(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewestSave(dir); err == nil {
		t.Fatal("an empty folder must be an error")
	}
	old, newer := filepath.Join(dir, "TS_Save_1.json"), filepath.Join(dir, "TS_Save_2.json")
	for _, p := range []string{old, newer, filepath.Join(dir, "other.json")} {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(newer, past.Add(-time.Hour), past.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := NewestSave(dir); err != nil || got != old {
		t.Fatalf("got %q, %v", got, err)
	}
}
