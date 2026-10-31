package web

import "strings"

// pathLookView is how a hero's path is drawn: its game-icons.net icon (see static/img/CREDITS.md), the kind of hero it makes
// (which colours the icon and the box) and its name cut in two, the "Path of the" lead in small and the rest in large.
type pathLookView struct {
	Icon string
	Kind string // melee, ranged, command, magic or other
	Lead string // "Path of the", "Path of" or empty
	Main string
}

// pathLooks are the paths of the Battle Companies file. Fighters are melee; archers and rangers ranged; those who lead or
// guard the company are command; the Sorcerer is magic; the paths that are not chosen are other.
var pathLooks = map[string]pathLookView{
	"Path of the Soldier":      {Icon: "sword-clash", Kind: "melee"},
	"Path of the Claimant":     {Icon: "crowned-skull", Kind: "melee"},
	"Path of the Berserker":    {Icon: "enrage", Kind: "melee"},
	"Path of the Duelist":      {Icon: "fencer", Kind: "melee"},
	"Path of the Sharpshooter": {Icon: "bullseye", Kind: "ranged"},
	"Path of the Marksman":     {Icon: "arrow-cluster", Kind: "ranged"},
	"Path of the Ranger":       {Icon: "boot-prints", Kind: "ranged"},
	"Path of the Tactician":    {Icon: "compass", Kind: "command"},
	"Path of the Warlord":      {Icon: "crown", Kind: "command"},
	"Path of the Shieldbearer": {Icon: "checked-shield", Kind: "command"},
	"Path of the Sorcerer":     {Icon: "wizard-staff", Kind: "magic"},
	"Path of Their Own Making": {Icon: "hiking", Kind: "other"},
	"On Their Own Path":        {Icon: "padlock", Kind: "other"},
}

// pathLook is the look of a path by its name; a path the app does not know (another rules file) gets the medal.
func pathLook(name string) pathLookView {
	l, ok := pathLooks[name]
	if !ok {
		l = pathLookView{Icon: "medal", Kind: "other"}
	}
	l.Main = name
	for _, lead := range []string{"Path of the ", "Path of "} {
		if rest, ok := strings.CutPrefix(name, lead); ok && rest != "" {
			l.Lead, l.Main = strings.TrimSpace(lead), rest
			break
		}
	}
	return l
}
