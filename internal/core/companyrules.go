package core

import (
	"fmt"
	"strings"
)

// HeroStart is what a new hero starts with (campaign.company.startingHeroStats): points of Might, Will and Fate.
type HeroStart struct{ Might, Will, Fate int }

// Bonus is the hero's profile bonus: its start plus extra (the same keys a Progress carries).
func (h HeroStart) Bonus(extra map[string]int) map[string]int {
	b := map[string]int{"might": h.Might, "will": h.Will, "fate": h.Fate}
	for k, v := range extra {
		b[k] += v
	}
	return b
}

// CompanyRules are the parameters of a company's shape that the engine reads from campaign.company.
type CompanyRules struct {
	Leaders, Sergeants int        // officers of a company: a starting company has this many heroes (leaders + sergeants)
	HeroStart          HeroStart  // what a new hero starts with
	MaxModels          int        // the models of a company; a company's own maxModels overrides it
	EliteMaxModels     int        // the models of an Elite company (carried by the companies' own maxModels in the shipped file)
	EquipmentSlots     SlotCounts // the equipment slots every model has (campaign.company.equipmentSlots)
}

// SlotCounts is a number of large and of small equipment slots.
type SlotCounts struct{ Large, Small int }

// Officers is how many heroes a starting company has.
func (c CompanyRules) Officers() int { return c.Leaders + c.Sergeants }

// WargearThreshold says when a hero pays the hero rating for its wargear (campaign.rating.hero.wargear.threshold): the sum of Stats
// reaches SumAtLeast.
type WargearThreshold struct {
	Stats      []string
	SumAtLeast int
}

// Reached reports whether the profile (by stat key) reaches the threshold.
func (w WargearThreshold) Reached(stats map[string]int) bool {
	sum := 0
	for _, s := range w.Stats {
		sum += stats[s]
	}
	return sum >= w.SumAtLeast
}

// problems collects the errors of a parse so one report names every path.
type problems []string

func (p *problems) add(format string, args ...any) { *p = append(*p, fmt.Sprintf(format, args...)) }
func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(p, "; "))
}

// limitLabel is the short name a limit is shown under: the file's label without its note in brackets, first letter capitalised
// ("bows (heroes included)" is "Bows").
func limitLabel(fromFile string) string {
	label, _, _ := strings.Cut(fromFile, "(")
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	return strings.ToUpper(label[:1]) + label[1:]
}
