package web

import (
	"io/fs"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The sprite holds every icon the pages draw and nothing else, and every item class and item key has a look of its own.
func TestIconSpriteMatchesTheIconsInUse(t *testing.T) {
	sprite, ok := staticFiles()[iconSprite]
	if !ok {
		t.Fatalf("no %s", iconSprite)
	}
	symbols := map[string]bool{}
	for _, m := range regexp.MustCompile(`<symbol id="([^"]+)"`).FindAllSubmatch(sprite.data, -1) {
		symbols[string(m[1])] = true
	}

	used := map[string]string{} // icon -> where it is used
	for k, icon := range kindIcons {
		used[icon] = "kind " + string(k)
	}
	for r, l := range roleLooks {
		used[l.Icon] = "role " + string(r)
	}
	for k, l := range gearLooks {
		used[l.Icon] = "gear " + k
	}
	used[gearLook("no such key").Icon] = "unknown gear"
	for name := range pathLooks {
		used[pathLook(name).Icon] = name
	}
	used[pathLook("Path of the Smith").Icon] = "unknown path"
	lu := core.LiveUnit{Fate: core.LiveStat{Start: 1}}
	for _, m := range (liveView{}).Milestones(5, lu) {
		used[m.Icon] = "milestone " + m.Label
	}
	// The templates name a few icons themselves: {{template "gi_icon" (dict "N" "death-skull" ...)}}.
	literal := regexp.MustCompile(`"gi_icon" \(dict "N" "([a-z0-9-]+)"`)
	err := fs.WalkDir(assets, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(assets, p)
		for _, m := range literal.FindAllSubmatch(b, -1) {
			used[string(m[1])] = p
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	for icon, where := range used {
		if !symbols[icon] {
			t.Errorf("%s draws %q, which is not in the sprite", where, icon)
		}
	}
	for icon := range symbols {
		if _, ok := used[icon]; !ok {
			t.Errorf("the sprite's %q is drawn nowhere: drop it (and its credit in static/img/CREDITS.md)", icon)
		}
	}

	for _, c := range core.EquipmentClasses() {
		if _, ok := gearLooks[string(c)]; !ok {
			t.Errorf("item class %q has no look", c)
		}
	}
	for _, k := range core.IconKeys() {
		if _, ok := gearLooks[k]; !ok {
			t.Errorf("item key %q (core.IconFor) has no look", k)
		}
	}
	css, err := os.ReadFile("../../tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	var tones []string
	for _, m := range regexp.MustCompile(`\.eq-tone-([a-z]+) \{`).FindAllSubmatch(css, -1) {
		tones = append(tones, string(m[1]))
	}
	for k, l := range gearLooks {
		if !slices.Contains(tones, l.Tone) {
			t.Errorf("gear %q has the tone %q, which has no eq-tone-* rule", k, l.Tone)
		}
	}
}
