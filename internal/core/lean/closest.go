package lean

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// NameKey is the comparison form of a name: lower case, accents dropped, apostrophes dropped, every
// other run of punctuation or spaces one space. "Lake-Town" and "lake town" are equal, so are "Lothlorien" and "Lothlórien"; plurals and
// "made" are NOT folded (that fuzz lives only in the vanilla bridge). An empty result means the name has no letter or digit.
func NameKey(s string) string { return nameKey(s, true) }

// nameKey is NameKey with the accent folding switched on or off (the test of the shipped names compares both).
func nameKey(s string, fold bool) string {
	if isASCII(s) {
		return asciiNameKey(s)
	}
	if fold {
		s = foldAccents(s)
	}
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case r == '\'' || r == '’' || r == '`':
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(unicode.ToLower(r))
		default:
			space = true
		}
	}
	return b.String()
}

// asciiNameKey is nameKey for a name of ASCII bytes only: a stack buffer, no rune decoding, and the string itself when it is a key already.
func asciiNameKey(s string) string {
	var stack [96]byte
	buf := stack[:0]
	space := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' || c == '`':
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			if space && len(buf) > 0 {
				buf = append(buf, ' ')
			}
			space = false
			buf = append(buf, c)
		case c >= 'A' && c <= 'Z':
			if space && len(buf) > 0 {
				buf = append(buf, ' ')
			}
			space = false
			buf = append(buf, c+'a'-'A')
		default:
			space = true
		}
	}
	if string(buf) == s { // no allocation: the compiler does not copy for a comparison
		return s
	}
	return string(buf)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// foldAccents takes the accents off the letters: a letter decomposed (NFD) loses its combining marks (û is u, ó is o).
func foldAccents(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, norm.NFD.String(s))
}

// Slug is the default id of a name: its NameKey with dashes.
func Slug(name string) string { return strings.ReplaceAll(NameKey(name), " ", "-") }

// Closest returns the candidate nearest to s, or "" when none is close. Distance is the optimal-string-alignment (Damerau-
// Levenshtein) distance of the NameKeys; a candidate that contains s or is contained in it (4 letters or more) also counts, after
// the near misses. Ties go to the alphabetically first candidate, so the answer is deterministic.
func Closest(candidates []string, s string) string {
	key := NameKey(s)
	if key == "" {
		return ""
	}
	maxd := 3
	switch n := len([]rune(key)); {
	case n <= 4:
		maxd = 1
	case n <= 8:
		maxd = 2
	}
	best, bestCost := "", 1<<30
	for _, c := range candidates {
		ck := NameKey(c)
		if ck == "" {
			continue
		}
		cost := 1 << 30
		if d := distance(key, ck); d <= maxd {
			cost = d
		} else if contains4(key, ck) {
			cost = maxd + 1
		}
		if cost < bestCost || (cost == bestCost && cost < 1<<30 && c < best) {
			best, bestCost = c, cost
		}
	}
	if bestCost == 1<<30 {
		return ""
	}
	return best
}

func contains4(a, b string) bool {
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	return len([]rune(short)) >= 4 && strings.Contains(long, short)
}

// distance is the optimal string alignment distance (insert, delete, substitute, swap neighbours).
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev2 := make([]int, len(rb)+1)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(rb)]
}

// SortedKeys returns the keys of m in alphabetical order (the binder walks maps in this order).
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
