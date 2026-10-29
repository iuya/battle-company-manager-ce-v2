package web

import (
	"os"
	"sync"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

var (
	shippedOnce sync.Once
	shippedSet  *core.RuleSet
	shippedErr  error
)

// shippedRules is the rule set of the files the app ships with, built once for the tests of this package.
func shippedRules(t *testing.T) *core.RuleSet {
	t.Helper()
	shippedOnce.Do(func() {
		var files [3][]byte
		for i, name := range []string{"vanilla/data2024.json", "battle_companies/battle_companies.json", "glossary/mesbg_glossary.json"} {
			b, err := os.ReadFile("../../definitions/" + name)
			if err != nil {
				shippedErr = err
				return
			}
			files[i] = b
		}
		shippedSet, shippedErr = core.LoadRuleSet(files[0], files[1], files[2], nil)
	})
	if shippedErr != nil {
		t.Skip("no rules file: ", shippedErr)
	}
	return shippedSet
}
