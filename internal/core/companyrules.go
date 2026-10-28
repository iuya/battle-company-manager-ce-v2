package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
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

// intIn validates a required whole number against lo..hi and returns it.
func (p *problems) intIn(path string, v *int, lo, hi int) int {
	switch {
	case v == nil:
		p.add("%s is missing", path)
		return 0
	case *v < lo || *v > hi:
		p.add("%s: %d is outside %d..%d", path, *v, lo, hi)
	}
	return *v
}

// knownStats are the characteristics of a profile (the vocabulary of stat lists).
var knownStats = []string{"movement", "fight", "shoot", "strength", "defence", "attack", "wounds", "courage", "intelligence", "might", "will", "fate"}

// ParseCompanyRules reads and validates the company shape, the hero rating threshold and the limits of the Battle Companies file.
// A file without a campaign section (a fixture) has none of them: the zero value, and no limits.
func ParseCompanyRules(content []byte) (CompanyRules, WargearThreshold, []LimitRule, error) {
	var doc struct {
		Campaign struct {
			Company struct {
				MaxModels      *int `json:"maxModels"`
				EliteMaxModels *int `json:"eliteMaxModels"`
				Leaders        *int `json:"leaders"`
				Sergeants      *int `json:"sergeants"`
				HeroStats      *struct {
					Might *int `json:"might"`
					Will  *int `json:"will"`
					Fate  *int `json:"fate"`
				} `json:"startingHeroStats"`
				Slots *struct {
					Large *int `json:"large"`
					Small *int `json:"small"`
				} `json:"equipmentSlots"`
			} `json:"company"`
			Rating struct {
				Hero struct {
					Wargear struct {
						Threshold *struct {
							Stats      []string `json:"stats"`
							SumAtLeast *int     `json:"sumAtLeast"`
						} `json:"threshold"`
					} `json:"wargear"`
				} `json:"hero"`
			} `json:"rating"`
		} `json:"campaign"`
		Data struct {
			Companies []struct {
				Name       string `json:"name"`
				MaxModels  *int   `json:"maxModels"`
				BreakPoint *int   `json:"breakPointPercentage"`
			} `json:"companies"`
		} `json:"data"`
		Limits []struct {
			ID      string   `json:"id"`
			Type    string   `json:"type"`
			Value   string   `json:"value"`
			Label   string   `json:"label"`
			Percent *float64 `json:"maximumPercentage"`
			RoundUp *bool    `json:"roundUp"`
			Classes []string `json:"classes"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return CompanyRules{}, WargearThreshold{}, nil, fmt.Errorf("campaign.company: %w", err)
	}
	if !bytes.Contains(content, []byte(`"campaign"`)) {
		return CompanyRules{}, WargearThreshold{}, nil, nil
	}
	var p problems
	c := doc.Campaign.Company
	var rules CompanyRules
	rules.MaxModels = p.intIn("campaign.company.maxModels", c.MaxModels, 1, 50)
	rules.EliteMaxModels = p.intIn("campaign.company.eliteMaxModels", c.EliteMaxModels, 1, 50)
	rules.Leaders = p.intIn("campaign.company.leaders", c.Leaders, 1, 3)
	rules.Sergeants = p.intIn("campaign.company.sergeants", c.Sergeants, 0, 6)
	if c.HeroStats == nil {
		p.add("campaign.company.startingHeroStats is missing")
	} else {
		rules.HeroStart = HeroStart{
			Might: p.intIn("campaign.company.startingHeroStats.might", c.HeroStats.Might, 0, 5),
			Will:  p.intIn("campaign.company.startingHeroStats.will", c.HeroStats.Will, 0, 5),
			Fate:  p.intIn("campaign.company.startingHeroStats.fate", c.HeroStats.Fate, 0, 5)}
	}
	if c.Slots == nil {
		p.add("campaign.company.equipmentSlots is missing")
	} else {
		rules.EquipmentSlots = SlotCounts{
			Large: p.intIn("campaign.company.equipmentSlots.large", c.Slots.Large, 0, 10),
			Small: p.intIn("campaign.company.equipmentSlots.small", c.Slots.Small, 0, 10)}
	}
	if c.MaxModels != nil && rules.Officers() > rules.MaxModels {
		p.add("campaign.company.leaders: %d leaders and %d sergeants are more than maxModels %d", rules.Leaders, rules.Sergeants, rules.MaxModels)
	}
	for _, co := range doc.Data.Companies {
		if co.BreakPoint != nil {
			p.intIn("data.companies["+co.Name+"].breakPointPercentage", co.BreakPoint, 1, 100)
		}
		if co.MaxModels == nil {
			continue
		}
		path := "data.companies[" + co.Name + "].maxModels"
		if n := p.intIn(path, co.MaxModels, 1, 50); n < rules.Officers() && n >= 1 {
			p.add("%s: %d is fewer than the %d officers of a starting company", path, n, rules.Officers())
		}
	}

	var wt WargearThreshold
	if t := doc.Campaign.Rating.Hero.Wargear.Threshold; t == nil {
		p.add("campaign.rating.hero.wargear.threshold is missing")
	} else {
		wt.SumAtLeast = p.intIn("campaign.rating.hero.wargear.threshold.sumAtLeast", t.SumAtLeast, 0, 20)
		if len(t.Stats) == 0 {
			p.add("campaign.rating.hero.wargear.threshold.stats is missing")
		}
		for i, s := range t.Stats {
			if !slices.Contains(knownStats, s) {
				p.add("campaign.rating.hero.wargear.threshold.stats[%d]: %q is not a characteristic; known: %s", i, s, strings.Join(knownStats, ", "))
			}
		}
		wt.Stats = t.Stats
	}

	var limits []LimitRule
	seen := map[string]bool{}
	for i, l := range doc.Limits {
		path := fmt.Sprintf("limits[%d]", i)
		if l.ID == "" || seen[l.ID] {
			p.add("%s.id: %q is empty or used twice", path, l.ID)
		}
		seen[l.ID] = true
		r := LimitRule{ID: l.ID, Label: limitLabel(l.Label)}
		switch pc := l.Percent; {
		case pc == nil:
			p.add("%s.maximumPercentage is missing", path)
		case *pc <= 0 || *pc > 100:
			p.add("%s.maximumPercentage: %v is outside (0, 100]", path, *pc)
		default:
			r.Percent = *pc
		}
		if l.RoundUp == nil {
			p.add("%s.roundUp is missing", path)
		} else {
			r.RoundUp = *l.RoundUp
		}
		if l.Type == "keyword" {
			if l.Value != KeywordCavalry {
				p.add("%s.value: %q is not a keyword the engine counts; known: %s", path, l.Value, KeywordCavalry)
			}
			r.Keyword = l.Value
		} else {
			if len(l.Classes) == 0 {
				p.add("%s.classes is missing", path)
			}
			for j, cl := range l.Classes {
				if !slices.Contains(knownEquipmentClasses, EquipmentClass(cl)) {
					p.add("%s.classes[%d]: %q is not an item class; known: %s", path, j, cl, joinClasses(knownEquipmentClasses))
				}
				r.Classes = append(r.Classes, EquipmentClass(cl))
			}
		}
		limits = append(limits, r)
	}
	if err := p.err(); err != nil {
		return CompanyRules{}, WargearThreshold{}, nil, err
	}
	return rules, wt, limits, nil
}

func joinClasses(cs []EquipmentClass) string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = string(c)
	}
	return strings.Join(out, ", ")
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
