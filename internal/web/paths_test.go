package web

import "testing"

func TestPathLook(t *testing.T) {
	for _, c := range []struct{ name, icon, kind, lead, main string }{
		{"Path of the Duelist", "fencer", "melee", "Path of the", "Duelist"},
		{"Path of the Marksman", "arrow-cluster", "ranged", "Path of the", "Marksman"},
		{"Path of the Ranger", "boot-prints", "ranged", "Path of the", "Ranger"},
		{"Path of the Warlord", "crown", "command", "Path of the", "Warlord"},
		{"Path of the Sorcerer", "wizard-staff", "magic", "Path of the", "Sorcerer"},
		{"Path of Their Own Making", "hiking", "other", "Path of", "Their Own Making"},
		{"On Their Own Path", "padlock", "other", "", "On Their Own Path"},
		{"Path of the Smith", "medal", "other", "Path of the", "Smith"}, // another rules file
	} {
		if got := pathLook(c.name); got.Icon != c.icon || got.Kind != c.kind || got.Lead != c.lead || got.Main != c.main {
			t.Errorf("pathLook(%q) = %+v, want %s %s %q %q", c.name, got, c.icon, c.kind, c.lead, c.main)
		}
	}
}
