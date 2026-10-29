package lean

import (
	"reflect"
	"testing"
)

func yes() *bool { y := true; return &y }
func no() *bool  { n := false; return &n }

// TestWhoAdmits: one row per field of the selector, and the combination.
func TestWhoAdmits(t *testing.T) {
	knight := Subject{Names: []string{"Knight of Dol Amroth", "Knight"}, Hero: true, Known: true, Race: []string{"Man"}, Keywords: []string{"Man", "Dol Amroth", "Gondor", "Cavalry"}, Alignment: "good"}
	archer := Subject{Names: []string{"Hobbit Archer"}, Known: true, Race: []string{"Hobbit"}, Keywords: []string{"Hobbit", "Shire"}, Alignment: "good"}
	unknown := Subject{Names: []string{"Mystery"}, Alignment: "evil"}
	for _, c := range []struct {
		name string
		who  *Who
		s    Subject
		want string
	}{
		{"nil admits everyone", nil, archer, ""},
		{"empty admits everyone", &Who{}, archer, ""},
		{"hero admits a hero", &Who{Hero: yes()}, knight, ""},
		{"hero refuses a warrior", &Who{Hero: yes()}, archer, "Heroes only"},
		{"hero false refuses a hero", &Who{Hero: no()}, knight, "Warriors only"},
		{"hero false admits a warrior", &Who{Hero: no()}, archer, ""},
		{"race admits", &Who{Race: []string{"Hobbit"}}, archer, ""},
		{"race refuses", &Who{Race: []string{"Hobbit"}}, knight, "Hobbit only"},
		{"race, any of several", &Who{Race: []string{"Elf", "Man"}}, knight, ""},
		{"race of a model with no profile is not tested", &Who{Race: []string{"Elf"}}, unknown, ""},
		{"keywords admit a faction", &Who{Keywords: []string{"Dol Amroth"}}, knight, ""},
		{"keywords refuse", &Who{Keywords: []string{"Dol Amroth", "Lamedon"}}, archer, "Dol Amroth or Lamedon only"},
		{"keywords compare as names do", &Who{Keywords: []string{"dol-amroth"}}, knight, ""},
		{"units admit by name", &Who{Units: []string{"Hobbit Archer"}}, archer, ""},
		{"units admit by the name of the profile", &Who{Units: []string{"Knight"}}, knight, ""},
		{"units refuse", &Who{Units: []string{"Hobbit Archer", "Hobbit Militia"}}, knight, "Hobbit Archer, Hobbit Militia only"},
		{"units never match part of a name", &Who{Units: []string{"Hobbit"}}, archer, "Hobbit only"},
		{"except refuses the named", &Who{Except: []string{"Hobbit Archer"}}, archer, "Not for Hobbit Archer"},
		{"except admits the others", &Who{Except: []string{"Hobbit Archer"}}, knight, ""},
		{"alignment admits", &Who{Alignment: "good"}, archer, ""},
		{"alignment refuses", &Who{Alignment: "evil"}, archer, "Evil companies only"},
		{"the refusal is the file's when it says one", &Who{Hero: yes(), Refusal: "Only for the brave"}, archer, "Only for the brave"},
		{"all fields must hold", &Who{Hero: yes(), Race: []string{"Man"}, Keywords: []string{"Cavalry"}}, knight, ""},
		{"all fields must hold: one fails", &Who{Hero: yes(), Race: []string{"Elf"}, Keywords: []string{"Cavalry"}}, knight, "Elf only"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := c.who.Admits(c.s); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestWhoLabelAndBadges(t *testing.T) {
	for _, c := range []struct {
		name   string
		who    *Who
		label  string
		badges []string
	}{
		{"nil", nil, "", nil},
		{"text wins", &Who{Text: "All Hero models", Hero: yes()}, "All Hero models", []string{"hero"}},
		{"race and heroes", &Who{Race: []string{"Hobbit"}, Hero: yes()}, "Hobbit Heroes", []string{"hero"}},
		{"keywords", &Who{Keywords: []string{"Cavalry"}}, "Cavalry", []string{"cavalry"}},
		{"units", &Who{Units: []string{"A", "B"}}, "A B", nil},
		{"warriors", &Who{Hero: no()}, "Warriors", nil},
		{"alignment", &Who{Alignment: "evil"}, "", []string{"evil"}},
	} {
		if got := c.who.Label(); got != c.label {
			t.Errorf("%s: label %q, want %q", c.name, got, c.label)
		}
		if got := c.who.Badges(); !reflect.DeepEqual(got, c.badges) {
			t.Errorf("%s: badges %v, want %v", c.name, got, c.badges)
		}
	}
}

func TestWhoWithoutRace(t *testing.T) {
	w := &Who{Hero: yes(), Race: []string{"Elf"}}
	if got := w.WithoutRace(); got.Race != nil || got.Hero == nil || len(w.Race) != 1 {
		t.Errorf("race dropped, the rest and the original kept: %+v %+v", got, w)
	}
	if (*Who)(nil).WithoutRace() != nil {
		t.Error("nil stays nil")
	}
}

func TestWhoModels(t *testing.T) {
	for _, c := range []struct {
		who  *Who
		want string
	}{
		{nil, "models"},
		{&Who{}, "models"},
		{&Who{Units: []string{"Corsair Reaver", "Corsair of Umbar"}}, "Corsair Reaver and Corsair of Umbar models"},
		{&Who{Units: []string{"A", "B", "C"}}, "A, B and C models"},
		{&Who{Keywords: []string{"Dol Amroth"}, Hero: yes()}, "Dol Amroth Hero models"},
		{&Who{Race: []string{"Man"}, Keywords: []string{"Rohan"}}, "Man or Rohan models"},
		{&Who{Hero: no()}, "Warrior models"},
		{&Who{Except: []string{"Hobbit Archer"}}, "models except Hobbit Archer"},
		{&Who{Alignment: "evil"}, "models of evil companies"},
	} {
		if got := c.who.Models(); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.who, got, c.want)
		}
	}
}
