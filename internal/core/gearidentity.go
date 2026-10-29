package core

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

// Gear identity. Two pieces of gear are the same item only when they have the same name once case, punctuation, the plural s,
// the word "made" (Elven-made) and spacing (Warspear, War spear) are ignored, or when the rules file lists one as an alias of
// the other. Having the same stats, the same class or the same price never makes two items one: a Pike is not a Spear, an Elf
// bow is not a Longbow, a Light Shield is not a Shield (data.gearAnnotations, `relatedTo`).

// ItemKey is the identity of an item name; the same normalisation as `item_key` in tools/bc/gear.py (with spaces removed).
func ItemKey(name string) string { return itemKeys.get(name, itemKey) }

var itemKeys memo

func itemKey(name string) string {
	var words []string
	for _, w := range strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, strings.ReplaceAll(name, "&", " and "))) {
		if w == "made" {
			continue
		}
		if len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us") {
			w = w[:len(w)-1]
		}
		words = append(words, w)
	}
	return strings.Join(words, "")
}

var (
	optionVerb   = regexp.MustCompile(`(?i)^\s*(exchange|swap|replace)\s+`)
	optionSplit  = regexp.MustCompile(`(?i)\s*,\s*and\s+|\s+and\s+|\s*&\s*|\s*,\s*|\s+for\s+|\s+with\s+`)
	optionParens = regexp.MustCompile(`\([^)]*\)`)
)

// Option is a profile option name read once: "Shield and Spear" is a shield and a spear, "Exchange lance for banner" names the
// lance and the banner and gives the banner. Exchange is true for the verbs Exchange, Swap and Replace; Items are all the items it
// names, Gives the items a unit has to carry to have taken it (the item given for an exchange, else all of them).
type Option struct {
	Exchange bool
	Items    []string
	Gives    []string
}

var (
	optionMemo  sync.Map
	optionCount atomic.Int64
)

// ParseOption reads an option name, memoised like ItemKey. The slices are shared: callers do not change them.
func ParseOption(name string) Option {
	if v, ok := optionMemo.Load(name); ok {
		return v.(Option)
	}
	o := parseOption(name)
	if optionCount.Load() < memoLimit {
		if _, loaded := optionMemo.LoadOrStore(name, o); !loaded {
			optionCount.Add(1)
		}
	}
	return o
}

func parseOption(name string) Option {
	o := Option{Exchange: optionVerb.MatchString(name), Items: optionItems(name)}
	o.Gives = o.Items
	if o.Exchange {
		stripped := optionVerb.ReplaceAllString(name, "")
		for _, sep := range []string{" for ", " with "} {
			if _, after, ok := strings.Cut(strings.ToLower(stripped), sep); ok {
				o.Gives = optionItems(stripped[len(stripped)-len(after):])
				break
			}
		}
	}
	return o
}

// OptionItems are the items a profile option names.
func OptionItems(option string) []string { return ParseOption(option).Items }

func optionItems(option string) []string {
	option = optionParens.ReplaceAllString(option, " ")
	if optionVerb.MatchString(option) {
		option = optionVerb.ReplaceAllString(option, "")
	} else if strings.HasPrefix(strings.ToLower(strings.TrimSpace(option)), "bow with") {
		return []string{strings.TrimSpace(option)}
	}
	var out []string
	for _, p := range optionSplit.Split(option, -1) {
		if p = strings.TrimSpace(strings.Trim(p, " .;")); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// namesItem says whether the item (its name and the other spellings it is known by) is the item called `part`.
func namesItem(name string, also []string, part string) bool {
	key := ItemKey(part)
	if key == "" {
		return false
	}
	if ItemKey(name) == key {
		return true
	}
	for _, n := range also {
		if ItemKey(n) == key {
			return true
		}
	}
	return false
}

// SameItem says whether two names are the same item: equal names, or one is among the other's aliases.
func SameItem(a string, aliasesOfA []string, b string, aliasesOfB []string) bool {
	if namesItem(a, aliasesOfA, b) {
		return true
	}
	for _, n := range aliasesOfB {
		if namesItem(a, aliasesOfA, n) {
			return true
		}
	}
	return false
}
