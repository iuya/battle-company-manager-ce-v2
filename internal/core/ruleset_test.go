package core

import (
	"errors"
	"testing"
)

func TestRulesetHashIgnoresWhitespaceOutsideStrings(t *testing.T) {
	a, err := RulesetHash([]byte("{\n  \"game\": \"A  B\",\n  \"n\": [1, 2.50]\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := RulesetHash([]byte(`{"game":"A  B","n":[1,2.50]}`))
	c, _ := RulesetHash([]byte(`{"game":"A B","n":[1,2.50]}`))
	if a != b || a == c {
		t.Errorf("whitespace outside strings must not matter, inside it must: %s %s %s", a, b, c)
	}
	// A fixed vector: the browser implementation (static/js/ruleset_picker.js) must produce the same value.
	if want := "7aa3e75e556020070b397b701e1de0bfdae7ed243a79f3e8ece31eeee958993b"; a != want {
		t.Errorf("hash = %s, want %s", a, want)
	}
	if _, err := RulesetHash([]byte("{not json")); !errors.Is(err, ErrInvalidRuleset) {
		t.Errorf("invalid JSON: %v", err)
	}
}

func TestRulesetMeta(t *testing.T) {
	name, version, err := RulesetMeta(KindVanilla, []byte(`{"game":"MESBG 2024","appVersion":2}`))
	if err != nil || name != "MESBG 2024" || version != "2" {
		t.Errorf("vanilla: %q %q %v", name, version, err)
	}
	name, version, err = RulesetMeta(KindBattleCompanies, []byte(`{"name":"BC v2","version":"1.1","extends":{}}`))
	if err != nil || name != "BC v2" || version != "1.1" {
		t.Errorf("bc: %q %q %v", name, version, err)
	}
	for _, c := range []struct {
		kind RulesetKind
		doc  string
	}{
		{KindVanilla, `{"game":"x"}`}, {KindBattleCompanies, `{"name":"x"}`}, {KindVanilla, `{"appVersion":2}`}, {KindVanilla, `[]`}, {"other", `{}`},
	} {
		if _, _, err := RulesetMeta(c.kind, []byte(c.doc)); !errors.Is(err, ErrInvalidRuleset) {
			t.Errorf("%s %s: got %v", c.kind, c.doc, err)
		}
	}
}
