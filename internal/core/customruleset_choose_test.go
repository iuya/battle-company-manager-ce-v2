package core

import (
	"strings"
	"testing"
)

// choiceRow is the "choose" row of a company's reinforcement chart in the document.
func choiceRow(t *testing.T, doc map[string]any, company string) map[string]any {
	t.Helper()
	for _, r := range companyDoc(t, doc, company)["reinforcements"].(map[string]any)["rows"].([]any) {
		if m := r.(map[string]any); m["choose"] != nil {
			return m
		}
	}
	t.Fatalf("%s has no choose row", company)
	return nil
}

// TestCustomRulesetChoose: a choose row says what the player chooses from (scope) and how many results (picks); the words are display text.
func TestCustomRulesetChoose(t *testing.T) {
	for _, tt := range []struct {
		name        string
		mutate      func(row map[string]any)
		scope       string
		picks       int
		errContains string
	}{
		{"shipped: two results from the rows above", func(row map[string]any) {}, "above", 2, ""},
		{"scope any, picks 3", func(row map[string]any) { row["scope"], row["picks"] = "any", 3.0 }, "any", 3, ""},
		{"picks default to one", func(row map[string]any) { delete(row, "picks") }, "above", 1, ""},
		{"the words do not decide: text says above, scope says any", func(row map[string]any) { row["scope"] = "any" }, "any", 2, ""},
		{"scope is needed", func(row map[string]any) { delete(row, "scope") }, "", 0, "scope"},
		{"scope is above or any", func(row map[string]any) { row["scope"] = "abov" }, "", 0, "scope"},
		{"picks are at least one", func(row map[string]any) { row["picks"] = 0.0 }, "", 0, "out of range"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rs, err := customRulesetErr(t, func(doc map[string]any) { tt.mutate(choiceRow(t, doc, "The Shire")) })
			if tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error = %v, want one about %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			bcCo, _ := rs.BC.Company("The Shire")
			for _, row := range bcCo.Reinforcements {
				if row.Result.Type == "choice" {
					if row.Result.Scope != tt.scope || row.Result.Picks != tt.picks {
						t.Errorf("scope %q picks %d, want %q %d", row.Result.Scope, row.Result.Picks, tt.scope, tt.picks)
					}
					return
				}
			}
			t.Error("no choice row")
		})
	}
}
