package lean

import "sort"

// fakeLower is a small hand-written lower layer for the fixtures: the names the worked examples of the design use.
type fakeLower struct {
	names   map[Kind][]string
	options map[string][]string
	dups    map[string]int
}

func newFakeLower() *fakeLower {
	return &fakeLower{
		names: map[Kind][]string{
			KindUnit: {"Warrior of Arnor", "Ranger of Arnor", "Knight of Arnor", "Ranger of Gondor", "Hobbit Archer", "Angmar Orc Warrior",
				"Angmar Warg Rider", "Rider of Rohan", "Ranger of the North", "Cave Troll", "Peregrin Took"},
			KindGear:    {"Spear", "Glaive", "Armour", "Horse", "Banner"},
			KindRule:    {"Fearless", "Loyal to the Captain", "Stand Fast", "Lead by Example", "Hatred", "Foresight", "Gatekeeper", "At 'em, Lads!"},
			KindKeyword: {"Orc", "Troll", "Hobbit", "Elf", "Man"},
			KindRace:    {"Elf", "Orc", "Man", "Warg"},
			KindPower:   {"Fortify Spirit", "Wrath of Bruinen"},
		},
		options: map[string][]string{
			"Ranger of Arnor": {"Spear", "Armour", "Horse"},
			"Hobbit Archer":   {"Bow", "Hand weapon"},
			"Cave Troll":      {"Hand-and-a-half hammer", "Troll Spear"},
			"Rider of Rohan":  {"Spear", "Throwing spear"},
		},
		dups: map[string]int{"Peregrin Took": 2},
	}
}

func (f *fakeLower) Has(kind Kind, name string) bool {
	for _, n := range f.names[kind] {
		if NameKey(n) == NameKey(name) {
			return true
		}
	}
	return false
}

func (f *fakeLower) Names(kind Kind) []string {
	out := append([]string(nil), f.names[kind]...)
	sort.Strings(out)
	return out
}

func (f *fakeLower) Duplicates(kind Kind, name string) int {
	if n, ok := f.dups[name]; ok {
		return n
	}
	if f.Has(kind, name) {
		return 1
	}
	return 0
}

func (f *fakeLower) Options(unit string) []string { return f.options[unit] }
