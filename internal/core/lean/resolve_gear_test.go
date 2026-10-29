package lean

import (
	"slices"
	"strings"
	"testing"
)

// mountLower is the fake lower layer with two vanilla mounts.
type mountLower struct{ *fakeLower }

func (m mountLower) Has(kind Kind, name string) bool {
	if kind == KindMount {
		return slices.ContainsFunc([]string{"War goat", "Horse"}, func(n string) bool { return NameKey(n) == NameKey(name) })
	}
	return m.fakeLower.Has(kind, name)
}

func (m mountLower) Names(kind Kind) []string {
	if kind == KindMount {
		return []string{"Horse", "War goat"}
	}
	return m.fakeLower.Names(kind)
}

// TestResolveGearReferences: the names an item holds are checked like every other name: the mount whose stat block it uses, the items it
// is related to or cancelled by, the rules that lift a shield's cancellation, the other items that draw its ring, and a rule written with
// its argument ("Hatred (Orc)") is a use of the rule of that name.
func TestResolveGearReferences(t *testing.T) {
	ring := `"auras":[{"radius":3,"kind":"banner","affects":"friendly","when":"always","items":[%s]}]`
	cases := []struct {
		name string
		gear string
		want string // a fragment of the one issue, "" for none
	}{
		{"a rule with its argument", `{"name":"Torch","kind":"equipment","size":"large","grants":["Hatred (Orc)"]}`, ""},
		{"a rule with an argument whose rule is unknown", `{"name":"Torch","kind":"equipment","size":"large","grants":["Fearful (Orc)"]}`, `no rule "Fearful (Orc)"`},
		{"a mount of vanilla", `{"name":"Goat","kind":"wargear","type":"mount","mount":"War goat"}`, ""},
		{"a mount nobody knows", `{"name":"Goat","kind":"wargear","type":"mount","mount":"War sheep"}`, `no mount "War sheep"`},
		{"related to a known item", `{"name":"Bow","kind":"wargear","related":["Spear"]}`, ""},
		{"related to an unknown item", `{"name":"Bow","kind":"wargear","related":["Sling"]}`, `no gear "Sling"`},
		{"a shield that a known item cancels", `{"name":"Shield","kind":"wargear","abilities":{"defenceBonus":{"value":1,"cancelledByItems":["Spear"],"liftedByRules":[{"rule":"Hatred","cancels":["Spear"]}]}}}`, ""},
		{"a shield cancelled by an unknown item", `{"name":"Shield","kind":"wargear","abilities":{"defenceBonus":{"value":1,"cancelledByItems":["Pike"]}}}`, `no gear "Pike"`},
		{"a shield lifted by an unknown rule", `{"name":"Shield","kind":"wargear","abilities":{"defenceBonus":{"value":1,"liftedByRules":[{"rule":"Phalanx"}]}}}`, `no rule "Phalanx"`},
		{"a ring shared with a known item", `{"name":"War Horn","kind":"wargear",` + strings.Replace(ring, "%s", `"Banner"`, 1) + `}`, ""},
		{"a ring shared with an unknown item", `{"name":"War Horn","kind":"wargear",` + strings.Replace(ring, "%s", `"Company Horn"`, 1) + `}`, `no gear "Company Horn"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, rep := resolved(t, `"gear":[`+c.gear+`]`, mountLower{newFakeLower()})
			lines := issueLines(rep)
			switch {
			case c.want == "" && len(lines) != 0:
				t.Errorf("unexpected issues: %v", lines)
			case c.want != "" && !slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, c.want) }):
				t.Errorf("issues %v, want one with %q", lines, c.want)
			}
		})
	}
}

func TestBaseRuleName(t *testing.T) {
	for in, want := range map[string]string{
		"Terror (Beast)": "Terror", "Poisoned Attacks (X)": "Poisoned Attacks", "Hatred": "Hatred", "Keen Sight (Falcon) ": "Keen Sight",
		"(odd)": "(odd)", "": "", "Dominant (2)": "Dominant",
	} {
		if got := BaseRuleName(in); got != want {
			t.Errorf("BaseRuleName(%q) = %q, want %q", in, got, want)
		}
	}
}
