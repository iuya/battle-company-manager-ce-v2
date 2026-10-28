package lean

//go:generate go tool mockgen -typed -source=lower.go -destination=mock_lower_test.go -package=lean

// Kind is a kind of named thing; within a kind a name is unique (design 1).
type Kind string

const (
	KindCompany  Kind = "company"
	KindUnit     Kind = "unit"
	KindGear     Kind = "gear"
	KindRule     Kind = "rule"
	KindPath     Kind = "path"
	KindScenario Kind = "scenario"
	KindPower    Kind = "power"
	KindRace     Kind = "race"
	KindKeyword  Kind = "keyword"
	KindMount    Kind = "mount"
)

// Lower is what the layers below the Battle Companies file know: the vanilla file and the glossary (design plan 3.3). It is
// declared here, where it is used, and implemented in core over VanillaBook and Glossary; lean never sees their types.
type Lower interface {
	// Has reports whether a thing of that kind has that name or alias, compared by NameKey: exact, never fuzzy.
	Has(kind Kind, name string) bool
	// Names lists the names of a kind, sorted, for the closest-match hint. An empty list means the lower layers do not
	// know the kind (the checks of that kind are skipped).
	Names(kind Kind) []string
	// Duplicates counts the occurrences of a name in the vanilla file (for `vanilla.pins`).
	Duplicates(kind Kind, name string) int
	// Options lists the option names of a vanilla unit, nil when the unit is unknown.
	Options(unit string) []string
}
