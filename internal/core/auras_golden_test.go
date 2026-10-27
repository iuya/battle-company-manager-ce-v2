package core

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

var updateAurasGolden = flag.Bool("update-auras", false, "rewrite testdata/auras_golden.txt")

func auraText(as []Aura) string {
	var parts []string
	for _, a := range as {
		parts = append(parts, fmt.Sprintf("%g\" %s/%s %s", a.Radius, a.Kind, a.When, a.Label))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " | ")
}

// pathHero is a hero of the company on a path with every rule of the path unlocked and the path's starting heroic action and powers.
func pathHero(rs *RuleSet, bcCo BCCompany, base string, p PathProgression, gear []string) CompanyUnit {
	st := UnitState{ID: 1, Base: base, Role: RoleHero, Gear: slices.Clone(gear), Path: p.Name, PathRules: 99, Bonus: HeroBonus(nil)}
	if p.StartAction != "" {
		st.Actions = []string{p.StartAction}
	}
	if p.StartPower {
		for _, o := range rs.BC.Progression().Powers {
			names := []string{o.Name}
			if len(o.Variants) > 0 {
				names = nil
				for _, v := range o.Variants {
					names = append(names, o.Name+" ("+v+")")
				}
			}
			for _, n := range names {
				st.Spells = append(st.Spells, PathSpell{Name: n, Casting: o.Casting, Range: o.Range})
			}
		}
	}
	return buildUnit(rs, bcCo, st, CampaignSettings{})
}

// TestUnitAurasAllCompanies writes the auras of every start, reinforcement, special-chart, special and advancement unit of the 37 companies
// (as a warrior and as a hero without a path), then of the company's first start unit as a hero on each path with all its rules unlocked,
// and compares the lot with testdata/auras_golden.txt (go test -run UnitAurasAll -update-auras rewrites it).
func TestUnitAurasAllCompanies(t *testing.T) {
	rs := shippedRules(t)
	var golden strings.Builder
	companies := ttsCompanies(rs)
	if len(companies) != 37 {
		t.Fatalf("the shipped file has %d companies, want 37", len(companies))
	}
	kinds := map[string]bool{}
	for _, k := range rs.BC.Auras().Kinds {
		kinds[k.ID] = true
	}
	check := func(label string, as []Aura) {
		perKind := map[AuraKind]map[float64]int{}
		for _, a := range as {
			if !kinds[string(a.Kind)] || a.Radius <= 0 || a.Radius > 48 || a.Label == "" {
				t.Errorf("%s: bad aura %+v", label, a)
			}
			if perKind[a.Kind] == nil {
				perKind[a.Kind] = map[float64]int{}
			}
			if perKind[a.Kind][a.Radius]++; perKind[a.Kind][a.Radius] > 1 && a.Group != AuraGroupCast {
				t.Errorf("%s: two %s rings of %g\" were not merged", label, a.Kind, a.Radius)
			}
		}
		if !slices.IsSortedFunc(as, func(x, y Aura) int {
			if kx, ky := rs.kindRank(x.Kind), rs.kindRank(y.Kind); kx != ky {
				return kx - ky
			}
			switch {
			case x.Radius < y.Radius:
				return -1
			case x.Radius > y.Radius:
				return 1
			}
			return 0
		}) {
			t.Errorf("%s: not sorted by kind then radius: %s", label, auraText(as))
		}
	}
	def := rs.DefaultAuraSelection()
	most := func(label string, u CompanyUnit) {
		if n := len(rs.selectedAuras(rs.auraMatches(u), def)); n > AurasPerFigure {
			t.Errorf("%s: %d rings in the default selection, the limit is %d", label, n, AurasPerFigure)
		}
	}
	for _, bcCo := range companies {
		fmt.Fprintf(&golden, "## %s\n", bcCo.Name)
		for _, e := range ttsEntries(rs, bcCo) {
			label := fmt.Sprintf("%s: %s %v", e.source, e.unit, e.wargear)
			w := buildUnit(rs, bcCo, UnitState{ID: 1, Base: e.unit, Role: RoleWarrior, Gear: slices.Clone(e.wargear)}, CampaignSettings{})
			h := buildUnit(rs, bcCo, UnitState{ID: 1, Base: e.unit, Role: RoleHero, Gear: slices.Clone(e.wargear)}, CampaignSettings{})
			wa, ha := rs.UnitAuras(w), rs.UnitAuras(h)
			check(bcCo.Name+" "+label, wa)
			check(bcCo.Name+" "+label+" (hero)", ha)
			fmt.Fprintf(&golden, "%s\n    warrior: %s\n", label, auraText(wa))
			if auraText(ha) != auraText(wa) {
				fmt.Fprintf(&golden, "    hero: %s\n", auraText(ha))
			}
		}
		profile, _ := rs.BC.Profile(bcCo.Name)
		if len(profile.Start) == 0 {
			continue
		}
		for _, up := range profile.HeroUpgrades { // the company's hero upgrades, on the first unit that may take them
			for _, e := range ttsEntries(rs, bcCo) {
				if v, _, ok := resolveUnit(rs, profile, e.unit); ok && up.Allows(v) {
					u := buildUnit(rs, bcCo, UnitState{ID: 1, Base: e.unit, Role: RoleHero, Gear: slices.Clone(e.wargear), Path: "Path of the Soldier", Progress: &Progress{Upgrades: []string{up.Name}}}, CampaignSettings{})
					as := rs.UnitAuras(u)
					check(bcCo.Name+" "+up.Name, as)
					fmt.Fprintf(&golden, "upgrade %s on %s: %s\n", up.Name, e.unit, auraText(as))
					break
				}
			}
		}
		for _, p := range rs.BC.Progression().Paths {
			if p.Name == "Path of the Sorcerer" && len(rs.BC.Progression().Powers) == 0 {
				continue
			}
			u := pathHero(rs, bcCo, profile.Start[0].Unit, p, profile.Start[0].Wargear)
			if u.Path == nil || u.Path.Name != p.Name {
				continue
			}
			as := rs.UnitAuras(u)
			check(bcCo.Name+" "+p.Name, as)
			most(bcCo.Name+" "+p.Name, u)
			fmt.Fprintf(&golden, "path %s on %s: %s\n", p.Name, profile.Start[0].Unit, auraText(as))
		}
	}
	const path = "testdata/auras_golden.txt"
	if *updateAurasGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(golden.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (go test -run UnitAurasAll -update-auras writes it)", err)
	}
	if string(want) != golden.String() {
		t.Errorf("the auras differ from %s; run go test -run UnitAurasAll -update-auras and review the diff:\n%s", path, firstDiff(string(want), golden.String()))
	}
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < max(len(al), len(bl)); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			return fmt.Sprintf("line %d:\n  want %q\n  got  %q", i+1, x, y)
		}
	}
	return ""
}
