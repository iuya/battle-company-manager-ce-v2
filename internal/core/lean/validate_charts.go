package lean

import (
	"fmt"
	"strconv"
	"strings"
)

// Chart coverage and parameter ranges: every table keyed by a roll covers Min..Max of its dice spec exactly
// once, and the parameters that depend on each other agree.

type rolled struct {
	roll Roll
	path string // JSON path of the roll
}

func (v *validator) charts() {
	c := &v.doc.Campaign
	v.parameters(c)
	for i := range v.doc.Companies {
		co := &v.doc.Companies[i]
		cp := Idx("companies", i)
		if co.Reinforcements != nil {
			v.chart(Join(Join(cp, "reinforcements"), "rows"), chartRolls(co.Reinforcements.Rows, Join(Join(cp, "reinforcements"), "rows")), c.Recruitment.Die, "chart")
		}
		if co.Special != nil {
			v.chart(Join(Join(cp, "special"), "rows"), chartRolls(co.Special.Rows, Join(Join(cp, "special"), "rows")), c.Recruitment.Die, "chart")
		}
	}
	inj := &c.Injuries
	v.chart("campaign.injuries.warrior", tableRolls(inj.Warrior, "campaign.injuries.warrior"), inj.Roll, "table")
	v.chart("campaign.injuries.hero", tableRolls(inj.Hero, "campaign.injuries.hero"), inj.Roll, "table")
	w := &c.Progression.Warrior
	v.chart("campaign.progression.warrior.table", progressionRolls(w.Table), w.Roll, "table")
	for i, row := range w.Table {
		if row.Key != "" && !inWords(row.Key, "noChange", "promoted", "heroInTheMaking") {
			v.rep.Add(Issue{Path: Join(Idx("campaign.progression.warrior.table", i), "key"), Message: fmt.Sprintf("%q is not a key; known: noChange, promoted, heroInTheMaking", row.Key),
				Closest: Closest([]string{"noChange", "promoted", "heroInTheMaking"}, row.Key)})
		}
	}
	for i := range v.doc.Paths {
		p := &v.doc.Paths[i]
		if len(p.Rows) == 0 && p.Kind == "endOfPath" {
			continue
		}
		var rs []rolled
		for j, row := range p.Rows {
			rs = append(rs, rolled{row.Roll, Join(Idx(Join(Idx("paths", i), "rows"), j), "roll")})
		}
		v.chart(Join(Idx("paths", i), "rows"), rs, p.Dice, "path")
	}
	v.scenarioTables(&c.ScenarioSelection)
}

func inWords(s string, words ...string) bool {
	for _, w := range words {
		if s == w {
			return true
		}
	}
	return false
}

func chartRolls(rows []ChartRow, path string) []rolled {
	out := make([]rolled, len(rows))
	for i, r := range rows {
		out[i] = rolled{r.Roll, Join(Idx(path, i), "roll")}
	}
	return out
}

func tableRolls(rows []InjuryRow, path string) []rolled {
	out := make([]rolled, len(rows))
	for i, r := range rows {
		out[i] = rolled{r.Roll, Join(Idx(path, i), "roll")}
	}
	return out
}

func progressionRolls(rows []TableRow) []rolled {
	out := make([]rolled, len(rows))
	for i, r := range rows {
		out[i] = rolled{r.Roll, Join(Idx("campaign.progression.warrior.table", i), "roll")}
	}
	return out
}

// chart checks that the rows cover spec.Min..spec.Max exactly once. form is "chart" or "table" (a die-named message) or "path".
func (v *validator) chart(path string, rows []rolled, spec DiceSpec, form string) {
	if !spec.Valid() || len(rows) == 0 && form == "table" {
		return // the spec's own error was reported by the binder; a table that is absent is optional
	}
	lo, hi := spec.Min(), spec.Max()
	owner := make(map[int]int, hi-lo+1) // total -> index of the row that covers it
	for i, r := range rows {
		if r.roll.Empty() {
			v.rep.Add(Issue{Path: r.path, Message: fmt.Sprintf("%q is empty", r.roll.String()), Hint: "write the lower number first: \"2-3\""})
			continue
		}
		if r.roll.Lo < lo || r.roll.Hi > hi {
			v.rep.Add(Issue{Path: r.path, Message: fmt.Sprintf("roll %s is outside %d-%d, the totals of %s", r.roll.String(), lo, hi, strings.ToUpper(spec.String()))})
		}
		for n := max(r.roll.Lo, lo); n <= min(r.roll.Hi, hi); n++ {
			if prev, dup := owner[n]; dup {
				v.rep.Add(Issue{Path: r.path, Message: fmt.Sprintf("roll %d is covered twice (by rows[%d] and rows[%d]); each total belongs to exactly one row", n, prev, i)})
				continue
			}
			owner[n] = i
		}
	}
	var covered, missing []int
	for n := lo; n <= hi; n++ {
		if _, ok := owner[n]; ok {
			covered = append(covered, n)
		} else {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return
	}
	switch form {
	case "path":
		v.rep.Add(Issue{Path: path, Message: fmt.Sprintf("rolls %d..%d need an option each; %s", lo, hi, missingText(missing))})
	case "table":
		v.rep.Add(Issue{Path: path, Message: fmt.Sprintf("rows cover %s%s (a table keyed by %s must cover %d-%d exactly once)", rangesText(covered), missingJoin(missing), strings.ToUpper(spec.String()), lo, hi)})
	default:
		cov := rangesText(covered)
		if len(covered) == 0 {
			cov = "nothing"
		}
		v.rep.Add(Issue{Path: path, Message: fmt.Sprintf("rolls cover %s%s (a %s chart must cover %d-%d exactly once)", cov, missingJoin(missing), strings.ToUpper(spec.String()), lo, hi)})
	}
}

// missingJoin continues "rolls cover 1-5" with ", 6 is missing" (one gap) or "; 4, 6 are missing" (several).
func missingJoin(missing []int) string {
	if len(missing) == 1 {
		return ", " + missingText(missing)
	}
	return "; " + missingText(missing)
}

func missingText(missing []int) string {
	if len(missing) == 1 {
		return strconv.Itoa(missing[0]) + " is missing"
	}
	return rangesText(missing) + " are missing"
}

// rangesText writes sorted ints as "1-3, 5".
func rangesText(ns []int) string {
	var parts []string
	for i := 0; i < len(ns); {
		j := i
		for j+1 < len(ns) && ns[j+1] == ns[j]+1 {
			j++
		}
		if j == i {
			parts = append(parts, strconv.Itoa(ns[i]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", ns[i], ns[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}

// scenarioTables checks the first die's ranges partition 1..Sides and that each table lists Sides scenarios.
func (v *validator) scenarioTables(s *ScenarioSelection) {
	if !s.Die.Valid() {
		return
	}
	sides := s.Die.Sides
	if s.Die.Count != 1 {
		v.rep.Add(Issue{Path: "campaign.scenarioSelection.die", Message: fmt.Sprintf("%q has %d dice; the scenario die is a single die", s.Die.String(), s.Die.Count)})
		return
	}
	owner := map[int]int{}
	for i, t := range s.Tables {
		p := Join(Idx("campaign.scenarioSelection.tables", i), "roll")
		if t.Roll.Empty() {
			v.rep.Add(Issue{Path: p, Message: fmt.Sprintf("%q is empty", t.Roll.String())})
			continue
		}
		if t.Roll.Lo < 1 || t.Roll.Hi > sides {
			v.rep.Add(Issue{Path: p, Message: fmt.Sprintf("[%d,%d] is outside 1..%d; ranges must partition 1..%d", t.Roll.Lo, t.Roll.Hi, sides, sides)})
		}
		for n := max(t.Roll.Lo, 1); n <= min(t.Roll.Hi, sides); n++ {
			if prev, dup := owner[n]; dup {
				o := s.Tables[prev].Roll
				v.rep.Add(Issue{Path: p, Message: fmt.Sprintf("[%d,%d] overlaps [%d,%d]; ranges must partition 1..%d", t.Roll.Lo, t.Roll.Hi, o.Lo, o.Hi, sides)})
				break
			}
			owner[n] = i
		}
		if len(t.Scenarios) != sides {
			v.rep.Add(Issue{Path: Join(Idx("campaign.scenarioSelection.tables", i), "scenarios"), Message: fmt.Sprintf("lists %s; each table lists exactly %d (the second die picks one)", plural(len(t.Scenarios), "scenario"), sides)})
		}
	}
	var covered, missing []int
	for n := 1; n <= sides; n++ {
		if _, ok := owner[n]; ok {
			covered = append(covered, n)
		} else {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		cov := rangesText(covered)
		if cov == "" {
			cov = "nothing"
		}
		v.rep.Add(Issue{Path: "campaign.scenarioSelection.tables", Message: fmt.Sprintf("ranges cover %s%s; ranges must partition 1..%d", cov, missingJoin(missing), sides)})
	}
}

// parameters checks the ranges that depend on other fields.
func (v *validator) parameters(c *Campaign) {
	if d := c.Recruitment.Die; d.Valid() && d.Count != 1 {
		v.rep.Add(Issue{Path: "campaign.recruitment.die", Message: fmt.Sprintf("%q has %d dice; a recruiting die is a single die", d.String(), d.Count), Hint: "write \"d6\""})
	}
	co := c.Company
	if co.MaxModels > 0 && co.Leaders+co.Sergeants < 1 {
		v.rep.Errorf("campaign.company.leaders", "%d with sergeants %d; a company needs at least one officer", co.Leaders, co.Sergeants)
	}
	if co.MaxModels > 0 && co.Leaders+co.Sergeants > co.MaxModels {
		v.rep.Errorf("campaign.company.sergeants", "leaders %d + sergeants %d is more than maxModels %d", co.Leaders, co.Sergeants, co.MaxModels)
	}
	if h := c.Progression.Hero; h.Choose > h.RollsPerProgression {
		v.rep.Errorf("campaign.progression.hero.choose", "%d is more than rollsPerProgression %d", h.Choose, h.RollsPerProgression)
	}
	for i, e := range c.AftermathEffects {
		if e.Key != "rendezvous" && e.Key != "vantagePoint" {
			continue
		}
		p := Join(Idx("campaign.aftermathEffects", i), "range")
		switch {
		case len(e.Range) != 2:
			v.rep.Errorf(p, "missing; %s moves a die and needs a range [low, high]", e.Key)
		case e.Range[0] > e.Range[1]:
			v.rep.Errorf(p, "[%d,%d]: low is above high", e.Range[0], e.Range[1])
		}
	}
	v.injuryParameters(&c.Injuries)
	seen := map[string]int{}
	for i, g := range c.Experience.Gains {
		if first, dup := seen[g.Key]; dup {
			v.rep.Errorf(Join(Idx("campaign.experience.gains", i), "key"), "%q is already used at campaign.experience.gains[%d]", g.Key, first)
		} else {
			seen[g.Key] = i
		}
	}
}

// injuryParameters checks what the types of the injuries tables cannot: the stat keys of a row, the counts of grantsAtCount, what a disable
// needs for its kind, the injury keys a treatment or a healing result names and the treatment rolls against their dice.
func (v *validator) injuryParameters(inj *Injuries) {
	heroKeys := map[string]bool{}
	for _, r := range inj.Hero {
		heroKeys[r.Key] = true
	}
	known := strings.Split(Characteristics, "|")
	for _, chart := range []struct {
		name string
		rows []InjuryRow
	}{{"warrior", inj.Warrior}, {"hero", inj.Hero}} {
		for i, r := range chart.rows {
			rp := Idx("campaign.injuries."+chart.name, i)
			for _, stat := range SortedKeys(r.Stat) {
				p := Join(Join(rp, "stat"), stat)
				if !inWords(stat, known...) {
					v.rep.Add(Issue{Path: p, Message: fmt.Sprintf("%q is not a characteristic; known: %s", stat, strings.Join(known, ", ")), Closest: Closest(known, stat)})
				} else if n := r.Stat[stat]; n < -20 || n > 20 {
					v.rep.Errorf(p, "%d is out of range -20..20", n)
				}
			}
			for _, count := range SortedKeys(r.GrantsAtCount) {
				if n, err := strconv.Atoi(count); err != nil || n < 1 || n > 20 {
					v.rep.Errorf(Join(Join(rp, "grantsAtCount"), count), "%q is not a count; write a whole number 1..20", count)
				}
			}
			for j, d := range r.Disables {
				dp := Idx(Join(rp, "disables"), j)
				switch {
				case d.Kind == "gearClass" && len(d.Classes) == 0:
					v.rep.Errorf(Join(dp, "classes"), "missing; a gearClass entry lists the classes it disables")
				case d.Kind == "gearItem" && len(d.Names) == 0:
					v.rep.Errorf(Join(dp, "names"), "missing; a gearItem entry lists the items it disables")
				case d.Kind == "ability" && d.Key == "":
					v.rep.Errorf(Join(dp, "key"), "missing; an ability entry names the ability it takes away (%s)", strings.ReplaceAll(InjuryAbilities, "|", ", "))
				}
			}
			if r.Heals != nil {
				for j, k := range r.Heals.Keys {
					if !heroKeys[k] {
						v.rep.Errorf(Idx(Join(Join(rp, "heals"), "keys"), j), "%q is not the key of a hero injury", k)
					}
				}
			}
		}
	}
	t := &inj.Treatment
	seen := map[string]bool{}
	for i, m := range t.Hero {
		mp := Idx("campaign.injuries.treatment.hero", i)
		if seen[m.Key] {
			v.rep.Errorf(Join(mp, "key"), "%q is already used by another treatment", m.Key)
		}
		seen[m.Key] = true
		for j, k := range m.Healable {
			if !heroKeys[k] {
				v.rep.Errorf(Idx(Join(mp, "healable"), j), "%q is not the key of a hero injury", k)
			}
		}
		v.treatRoll(m.Roll, Join(mp, "roll"))
	}
	v.treatRoll(t.Warrior.Roll, "campaign.injuries.treatment.warrior.roll")
}

// treatRoll checks a treatment roll against its own dice: the target is a total the dice can reach and the ceiling is not below it.
func (v *validator) treatRoll(r *TreatRoll, path string) {
	if r == nil || !r.Dice.Valid() {
		return
	}
	if r.Target < 1 || r.Target > r.Dice.Max() {
		v.rep.Errorf(Join(path, "target"), "%d is outside 1..%d, the highest roll of %s", r.Target, r.Dice.Max(), r.Dice)
	} else if c := r.Modifier.Ceiling; c > 0 && c < r.Target {
		v.rep.Errorf(Join(Join(path, "modifier"), "ceiling"), "%d is below the target %d", c, r.Target)
	}
}
