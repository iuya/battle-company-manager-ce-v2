package core

import (
	"strconv"
	"strings"
)

// Plural picks the singular form for exactly one and the plural form otherwise (zero included).
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Count writes a count and its noun in words, the noun pluralised with an s: "one leader", "two sergeants", "no sergeants".
// Beyond six the number is written in digits.
func Count(n int, noun string) string {
	words := []string{"no", "one", "two", "three", "four", "five", "six"}
	word := strconv.Itoa(n)
	if n >= 0 && n < len(words) {
		word = words[n]
	}
	return word + " " + Plural(n, noun, noun+"s")
}

// Inches writes a distance without a needless fraction: 6, 4.5 (the caller adds the inch mark).
func Inches(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Capitalise upper-cases the first letter and leaves the rest as it is; an empty string stays empty.
func Capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// LowerFirst lower-cases the first letter and leaves the rest as it is, so a reason fits into a sentence.
func LowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
