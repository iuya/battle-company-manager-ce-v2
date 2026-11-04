// Package ttsmodels extracts the model keys ('Name: options') of the MESBG FTC mod's model bag from a LOCAL Tabletop Simulator saved game.
//
// Only the Nickname of every object in the bag with a given GUID is read out of the save: nothing of the mod's scripts or descriptions
// is kept. The result is the mod's content, so it is only ever written under the git-ignored sources/tts/ folder.
package ttsmodels

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/cases"
)

// BagGUID is the GUID of the model bag in the mod.
const BagGUID = "24f6d0"

type object = map[string]any

// NewestSave is the most recently modified TS_Save_*.json of a folder.
func NewestSave(saves string) (string, error) {
	found, err := filepath.Glob(filepath.Join(saves, "TS_Save_*.json"))
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", fmt.Errorf("no TS_Save_*.json in %s: pass the path of a saved game", saves)
	}
	mtime := func(p string) int64 {
		if st, err := os.Stat(p); err == nil {
			return st.ModTime().UnixNano()
		}
		return 0
	}
	sort.SliceStable(found, func(i, j int) bool { return mtime(found[i]) < mtime(found[j]) })
	return found[len(found)-1], nil
}

// FindBag is the object with the bag's GUID, also when it sits inside another container; nil when there is none.
func FindBag(objects []any, guid string) object {
	for _, x := range objects {
		o, ok := x.(object)
		if !ok {
			continue
		}
		if o["GUID"] == guid {
			return o
		}
		if inner, _ := o["ContainedObjects"].([]any); inner != nil {
			if bag := FindBag(inner, guid); bag != nil {
				return bag
			}
		}
	}
	return nil
}

// text is v as plain text: the empty string for null and other falsy values, the plain text of a string or a number.
func text(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == 0 {
			return ""
		}
		return fmt.Sprint(x)
	case bool:
		if x {
			return "True"
		}
	}
	return ""
}

var fold = cases.Fold()

// ModelKeys are the case- and space-folded unique nicknames of the bag's objects, in the save's order.
func ModelKeys(save object, guid string) ([]string, error) {
	states, _ := save["ObjectStates"].([]any)
	bag := FindBag(states, guid)
	if bag == nil {
		return nil, fmt.Errorf("no object with GUID %s in the save: is it a MESBG FTC game?", guid)
	}
	seen := map[string]bool{}
	var keys []string
	contained, _ := bag["ContainedObjects"].([]any)
	for _, x := range contained {
		o, ok := x.(object)
		if !ok {
			continue
		}
		key := strings.Join(strings.Fields(text(o["Nickname"])), " ")
		folded := fold.String(key)
		if key != "" && !seen[folded] {
			seen[folded] = true
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("the bag %s holds no named objects", guid)
	}
	return keys, nil
}

// ReadSave reads a saved game.
func ReadSave(path string) (object, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var save object
	if err := json.Unmarshal(raw, &save); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return save, nil
}

// CheckOutput refuses any path that is not sources/tts/ of the repository, or that git does not ignore.
func CheckOutput(root, out string) error {
	tts := filepath.Join(root, "sources", "tts")
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(resolved, filepath.Base(abs))
	}
	if r, err := filepath.EvalSymlinks(tts); err == nil {
		tts = r
	}
	if !strings.HasPrefix(abs, tts+string(filepath.Separator)) {
		return fmt.Errorf("refusing to write %s: only sources/tts/ is allowed", out)
	}
	if err := exec.Command("git", "-C", root, "check-ignore", "-q", out).Run(); err != nil {
		return fmt.Errorf("refusing to write %s: git does not ignore it (the mod's content must never be committed)", out)
	}
	return nil
}

// Write writes the keys as a JSON array of strings, one per line (what json.dump(keys, indent=0) wrote).
func Write(out string, keys []string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("[\n")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",\n")
		}
		enc, err := marshalString(k)
		if err != nil {
			return err
		}
		b.WriteString(enc)
	}
	b.WriteString("\n]")
	return os.WriteFile(out, []byte(b.String()), 0o644)
}

// marshalString is a JSON string with only the characters JSON requires escaped (non-ASCII is kept).
func marshalString(s string) (string, error) {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}
