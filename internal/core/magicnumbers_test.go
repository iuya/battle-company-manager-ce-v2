package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Magic-number lint (docs/testing.md "Magic-number lint"). REPORT ONLY: it
// logs the count and the worst files and never fails. Promote it with magicNumbersEnforce = true.
const magicNumbersEnforce = false

// magicAllowedValues are integer literals that need no justification (indexes, increments, halving).
var magicAllowedValues = map[int64]bool{0: true, 1: true, 2: true}

// magicAllowList names accepted literals that are not rules: "file.go:value" (any place in that file) or "file.go:func:value".
// Add a line with a short reason; the list must shrink, not grow.
var magicAllowList = map[string]string{
	"dice.go:6":           "a die has six faces: the d6 shortcut, the six-sided face a d2/d3 is folded from when typed or fixed (TypedMax, FromTyped), 6 is not a rule parameter",
	"rulesets.go:5":       "reasons named in the one-line text of a schema failure (display, the report lists them all; docs/definitions.md upload validation)",
	"ruleset_report.go:3": "issues quoted in the one-line summary of a report (display only)",
}

// magicForbiddenNames are engine constants that must come from the rules file; a declaration of one is reported.
var magicForbiddenNames = []string{"XPPerLevel", "MaxAdjust", "maxSergeants", "defaultLimitRules", "HeroBonus"}

func TestMagicNumbersReport(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	perFile := map[string]int{}
	perValue := map[string]int{}
	total := 0
	var forbidden []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		fn := ""
		var stack []ast.Node
		ast.Inspect(af, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			switch x := n.(type) {
			case *ast.FuncDecl:
				fn = x.Name.Name
			case *ast.Ident:
				for _, name := range magicForbiddenNames {
					if x.Name == name && x.Obj != nil && x.Obj.Pos() == x.Pos() {
						forbidden = append(forbidden, f+":"+name)
					}
				}
			case *ast.BasicLit:
				if x.Kind != token.INT {
					return true
				}
				v, err := strconv.ParseInt(strings.ReplaceAll(x.Value, "_", ""), 0, 64)
				if err != nil || magicAllowedValues[v] {
					return true
				}
				val := strconv.FormatInt(v, 10)
				if _, ok := magicAllowList[f+":"+val]; ok {
					return true
				}
				if _, ok := magicAllowList[f+":"+fn+":"+val]; ok {
					return true
				}
				total++
				perFile[f]++
				perValue[val]++
			}
			return true
		})
	}
	t.Logf("magic numbers: %d integer literals outside {0,1,2} and the allow-list; %d engine constants to delete %v", total, len(forbidden), forbidden)
	t.Logf("top files: %s", top(perFile, 10))
	t.Logf("top values: %s", top(perValue, 10))
	if magicNumbersEnforce && (total > 0 || len(forbidden) > 0) {
		t.Errorf("%d magic numbers, forbidden %v", total, forbidden)
	}
}

func top(m map[string]int, n int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > n {
		keys = keys[:n]
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + strconv.Itoa(m[k])
	}
	return strings.Join(parts, " ")
}

// templateRuleLiteral finds rule numbers written into prose or markup; the engine parameters should supply them.
var templateRuleLiteral = regexp.MustCompile(`(?i)\bup to [0-9]+\b|\b[0-9]*D[0-9]+\b`)

func TestTemplateRuleLiteralsReport(t *testing.T) {
	files, _ := filepath.Glob("../web/templates/*/*.gohtml")
	if len(files) == 0 {
		t.Skip("templates not found")
	}
	perFile := map[string]int{}
	total := 0
	for _, f := range files {
		b := readFileOrSkip(t, f)
		n := len(templateRuleLiteral.FindAll(b, -1))
		if n > 0 {
			perFile[filepath.Base(f)] = n
			total += n
		}
	}
	t.Logf("template rule literals ('up to N', 'ND6'): %d; top files: %s", total, top(perFile, 10))
	if magicNumbersEnforce && total > 0 {
		t.Errorf("%d rule literals in templates", total)
	}
}

func readFileOrSkip(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Skip(err)
	}
	return b
}
