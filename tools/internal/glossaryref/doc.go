// Package glossaryref holds the data and checks behind the glossary file
// (definitions/glossary/mesbg_glossary.json): the term list taken from the MESBG
// rules manual, the authored race and faction keyword entries, and the structured
// reference tables (the To Wound chart). Only programs under tools/ may import it.
//
// Exported API:
//
//	type Term struct{ Name, Category string; Headings, Aliases []string; Paragraphs []ParagraphRule; See []string; First int; Synth bool }
//	type ParagraphRule struct{ Regex string; Following int }
//	func Terms() []Term                      // the manual terms (a fresh copy)
//	var KeywordSource Source                 // {Section: "Keywords", RulebookPage: 8}
//	type Source struct{ Section string; RulebookPage int }
//	var RaceKeywords, FactionKeywords []string
//	type KeywordTerm struct{ Name, Kind, Definition string }
//	func KeywordTerms() []KeywordTerm        // authored race and faction entries
//	var ToWoundRows []string                 // the hand-transcribed chart rows
//	type ToWoundTable struct{ Title, Term string; RulebookPage int; Strengths, Defences []int; Cells [][]string; Notation Notation; Notes []string }
//	func ToWound() ToWoundTable              // the table as the glossary file carries it
//	func CellRank(cell string) int           // difficulty order of a cell
//	func ToWoundErrors(tw ToWoundTable) []string   // empty when the table is valid
//	func ToWoundErrorsJSON(v map[string]any) []string // the same check on a decoded JSON "toWound" object
//	type BuildOptions struct{ Src, Out, Summaries, Uncapped string }
//	func BuildGlossary(opts BuildOptions) (BuildResult, error) // what tools/glossary runs
package glossaryref
