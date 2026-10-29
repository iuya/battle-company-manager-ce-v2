package core

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

// The company rows of TestCustomRuleset (docs/testing.md, "Custom rulesets"): the shape of a company (officers, hero start,
// model limits, XP per level), the hero rating threshold, the limits, the creature, wanderer and path limits and the caps on Courage and
// Intelligence. Each changes one value of the shipped file and asserts the new behaviour; the "unchanged file" row asserts the default.

func campaignAt(t *testing.T, doc map[string]any, path ...any) map[string]any {
	t.Helper()
	return jsonAt(t, doc, path...).(map[string]any)
}

func limitNamed(t *testing.T, doc map[string]any, id string) map[string]any {
	t.Helper()
	for _, l := range doc["limits"].([]any) {
		if m := l.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("no limit %q", id)
	return nil
}

// rosterOf is a roster with the given roles, one unit each.
func rosterOf(roles ...UnitRole) []CompanyUnit {
	var out []CompanyUnit
	for i, r := range roles {
		out = append(out, CompanyUnit{ID: int64(i + 1), Role: r})
	}
	return out
}

func p2p3Rows(t *testing.T) []struct {
	name   string
	mutate func(t *testing.T, doc map[string]any)
	check  func(t *testing.T, rs *RuleSet)
} {
	type row = struct {
		name   string
		mutate func(t *testing.T, doc map[string]any)
		check  func(t *testing.T, rs *RuleSet)
	}
	noChange := func(t *testing.T, doc map[string]any) {}
	profile := func(t *testing.T, rs *RuleSet, name string) CompanyProfile {
		t.Helper()
		p, ok := rs.BC.Profile(name)
		if !ok {
			t.Fatalf("no company %s", name)
		}
		return p
	}
	// a hero outside the options of its profile pays for a Horse at the hero rating when its characteristics reach the threshold
	horsePays := func(t *testing.T, rs *RuleSet, bonus map[string]int) (heroRate bool) {
		t.Helper()
		co, _ := rs.BC.Company("Rohan")
		offer, _ := rs.BC.Offer(co, "Horse")
		points := func(bought ...string) int {
			return buildUnit(rs, co, UnitState{ID: 1, Base: "Warrior of Rohan", Role: RoleHero, Bonus: bonus, Bought: bought}, CampaignSettings{}).Points
		}
		got := points("Horse") - points()
		if offer.Rating.Hero == offer.Rating.Warrior {
			t.Fatal("the Horse must cost differently for a hero")
		}
		return got == offer.Rating.Hero
	}
	infantry := func(t *testing.T, n int) []CompanyUnit {
		units := make([]CompanyUnit, n)
		for i := range units {
			units[i].Kind = KindInfantry
		}
		return units
	}
	bow := Equipment{Name: "Bow", Class: EquipBow}
	crossbow := Equipment{Name: "Crossbow", Class: EquipCrossbow}
	limit := func(rs *RuleSet, id string, units []CompanyUnit) CompanyLimit {
		for _, l := range rs.BC.Limits(BCCompany{}, units) {
			if l.ID == id {
				return l
			}
		}
		return CompanyLimit{}
	}

	return []row{
		{"P2 unchanged file: the shape of the shipped company", noChange, func(t *testing.T, rs *RuleSet) {
			if got, want := rs.BC.Shape(), (CompanyRules{Leaders: 1, Sergeants: 2, HeroStart: HeroStart{1, 1, 1}, MaxModels: 15, EquipmentSlots: SlotCounts{Large: 1, Small: 1}}); got != want {
				t.Errorf("shape = %+v, want %+v", got, want)
			}
			if got := rs.BC.Progression().Threshold; got != 5 {
				t.Errorf("threshold = %d", got)
			}
			p := profile(t, rs, "Minas Tirith")
			if p.Leaders != 1 || p.Sergeants != 2 || len(p.DefaultHeroes()) != 3 {
				t.Errorf("profile officers = %d %d, default heroes %v", p.Leaders, p.Sergeants, p.DefaultHeroes())
			}
		}},
		{"P2 C1: progression.threshold 3 is the XP of a level-up", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "progression")["threshold"] = 3.0
		}, func(t *testing.T, rs *RuleSet) {
			if got := rs.BC.Progression().Threshold; got != 3 {
				t.Fatalf("threshold = %d", got)
			}
			d := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Minas Tirith"}, Units: []UnitState{{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleWarrior, XP: 7}}})
			u := d.Roster[0]
			if u.LevelEvery != 3 || u.PendingLevelUps() != 2 || u.XPInLevel() != 7 {
				t.Errorf("7 XP at 3 per level: every %d, pending %d, in level %d", u.LevelEvery, u.PendingLevelUps(), u.XPInLevel())
			}
		}},
		{"P2 B1: one leader and three sergeants", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "company")["sergeants"] = 3.0
		}, func(t *testing.T, rs *RuleSet) {
			p := profile(t, rs, "Minas Tirith")
			if p.Sergeants != 3 || len(p.DefaultHeroes()) != 4 {
				t.Errorf("sergeants %d, default heroes %v", p.Sergeants, p.DefaultHeroes())
			}
			if _, err := p.ValidateStart(p.DefaultHeroes()); err != nil {
				t.Errorf("the suggestion is valid: %v", err)
			}
			_, err := p.ValidateStart(p.DefaultHeroes()[:3])
			if !errors.Is(err, ErrInvalidCompany) || !strings.Contains(err.Error(), "choose one leader and three sergeants") {
				t.Errorf("two sergeants are no longer enough: %v", err)
			}
			roster := rosterOf(RoleLeader, RoleSergeant, RoleSergeant, RoleHero)
			if s := NextSergeant(roster, false, rs.BC.Shape().Sergeants); !s.NeedSergeant || s.Sergeants != 3 {
				t.Errorf("a third Sergeant place is open: %+v", s)
			}
		}},
		{"P2 B1: two leaders and one sergeant", func(t *testing.T, doc map[string]any) {
			c := campaignAt(t, doc, "campaign", "company")
			c["leaders"], c["sergeants"] = 2.0, 1.0
		}, func(t *testing.T, rs *RuleSet) {
			p := profile(t, rs, "Minas Tirith")
			var leaders, sergeants int
			for _, h := range p.DefaultHeroes() {
				switch h.Role {
				case RoleLeader:
					leaders++
				case RoleSergeant:
					sergeants++
				}
			}
			if leaders != 2 || sergeants != 1 {
				t.Errorf("default heroes: %d leaders, %d sergeants", leaders, sergeants)
			}
			if s := NextSuccession(rosterOf(RoleLeader, RoleSergeant), true, rs.BC.Shape().Leaders); !s.NeedLeader {
				t.Errorf("one leader left of two: another is needed: %+v", s)
			}
			if s := NextSuccession(rosterOf(RoleLeader, RoleLeader, RoleSergeant), true, rs.BC.Shape().Leaders); s.NeedLeader {
				t.Errorf("two leaders again: %+v", s)
			}
		}},
		{"P2 B1: the Ranger of the North arrangement keeps as many Sergeants as the rules give", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "company")["sergeants"] = 3.0
		}, func(t *testing.T, rs *RuleSet) {
			panel := newRecruitEnv(rs, CampaignSettings{}, gondor()).officerPanel("Ranger of the North")
			if panel.Sergeants != 3 || panel.Drop != 0 || len(panel.Officers) != 3 {
				t.Errorf("3 officers and 3 Sergeant places: nobody stops being an officer: %+v", panel)
			}
		}},
		{"P2 B3: a new hero starts with 2 Might, 1 Will and no Fate", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "company", "heroStart")["might"] = 2.0
			campaignAt(t, doc, "campaign", "company", "heroStart")["fate"] = 0.0
		}, func(t *testing.T, rs *RuleSet) {
			want := map[string]int{"might": 2, "will": 1, "fate": 0}
			if got := rs.BC.HeroBonus(nil); got["might"] != want["might"] || got["will"] != want["will"] || got["fate"] != want["fate"] {
				t.Errorf("hero bonus = %v", got)
			}
			if got := rs.BC.HeroBonus(map[string]int{"fight": 1, "might": 1}); got["might"] != 3 || got["fight"] != 1 {
				t.Errorf("extra is added: %v", got)
			}
			p := profile(t, rs, "Minas Tirith")
			officers := 0
			for _, s := range StartingUnits(p, p.DefaultHeroes()) {
				if s.Role == RoleLeader || s.Role == RoleSergeant {
					officers++
					if s.Bonus["might"] != 2 || s.Bonus["fate"] != 0 {
						t.Errorf("a starting officer: %v", s.Bonus)
					}
				}
			}
			if officers != 3 {
				t.Errorf("%d officers", officers)
			}
			hero := buildUnit(rs, rs.mustCompany(t, "Minas Tirith"), UnitState{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleHero, Bonus: rs.BC.HeroBonus(nil)}, CampaignSettings{})
			for _, a := range hero.Stats {
				if a.Name == "Might" && a.Value != 2 {
					t.Errorf("Might of the built hero = %d", a.Value)
				}
			}
			wild := newRecruitEnv(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Wanderers in the Wild"}})
			if spec := wild.newRecruit("Wanderer", nil); spec.Role != RoleHero || spec.Bonus["might"] != 2 {
				t.Errorf("a recruit of Wanderers in the Wild is a Hero in the Making with the start: %+v", spec)
			}
		}},
		{"P2 B4: maxModels 20 is the limit of a company without its own, a company's own stays", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "company")["maxModels"] = 20.0
		}, func(t *testing.T, rs *RuleSet) {
			if got := rs.mustCompany(t, "Minas Tirith").MaxModels; got != 20 {
				t.Errorf("Minas Tirith = %d", got)
			}
			if got := rs.mustCompany(t, "The Last Alliance").MaxModels; got != 12 {
				t.Errorf("The Last Alliance has its own limit: %d", got)
			}
			d := BuildDetail(rs, CampaignSettings{}, CompanyState{Company: CampaignCompany{Faction: "Minas Tirith"}})
			if d.MaxUnits != 20 {
				t.Errorf("the page shows %d", d.MaxUnits)
			}
		}},
		{"P3 F1: the wargear threshold is 4", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "rating", "hero", "wargear", "threshold")["sumAtLeast"] = 4.0
		}, func(t *testing.T, rs *RuleSet) {
			if horsePays(t, rs, HeroBonus(map[string]int{"attack": 1})) {
				t.Error("Attacks 2 + Wounds 1 = 3 no longer pays the hero rating")
			}
			if !horsePays(t, rs, HeroBonus(map[string]int{"attack": 2})) {
				t.Error("Attacks 3 + Wounds 1 = 4 pays the hero rating")
			}
		}},
		{"P3 F1: unchanged threshold 3 over attack and wounds", noChange, func(t *testing.T, rs *RuleSet) {
			if !horsePays(t, rs, HeroBonus(map[string]int{"attack": 1})) || horsePays(t, rs, HeroBonus(nil)) {
				t.Error("Attacks + Wounds of 3 pays the hero rating, 2 does not")
			}
		}},
		{"P3 F1: the threshold counts the stats the file lists", func(t *testing.T, doc map[string]any) {
			t3 := campaignAt(t, doc, "campaign", "rating", "hero", "wargear", "threshold")
			t3["stats"], t3["sumAtLeast"] = []any{"attack"}, 2.0
		}, func(t *testing.T, rs *RuleSet) {
			if !horsePays(t, rs, HeroBonus(map[string]int{"attack": 1})) || horsePays(t, rs, HeroBonus(nil)) {
				t.Error("Attacks 2 pays the hero rating, Attacks 1 does not; Wounds no longer count")
			}
			if horsePays(t, rs, HeroBonus(map[string]int{"wounds": 2})) {
				t.Error("Wounds are not in the list")
			}
		}},
		{"P3 B8: unchanged limits, a third rounded up", noChange, func(t *testing.T, rs *RuleSet) {
			units := infantry(t, 7)
			units[0].Equipment, units[1].Equipment = []Equipment{bow}, []Equipment{crossbow}
			if l := limit(rs, "bow", units); l.Used != 2 || l.Max != 3 || l.Label != "Bows" {
				t.Errorf("bow limit of 7 models with a bow and a crossbow: %+v", l)
			}
		}},
		{"P3 B8: maximumPercentage 50", func(t *testing.T, doc map[string]any) {
			limitNamed(t, doc, "bow")["maximumPercentage"] = 50.0
		}, func(t *testing.T, rs *RuleSet) {
			if l := limit(rs, "bow", infantry(t, 6)); l.Max != 3 || l.Next != 4 {
				t.Errorf("half of 6, and of 7 rounded up: %+v", l)
			}
			if l := limit(rs, "throwing", infantry(t, 6)); l.Max != 2 {
				t.Errorf("the other limits are unchanged: %+v", l)
			}
		}},
		{"P3 B8: roundUp false rounds the share down", func(t *testing.T, doc map[string]any) {
			limitNamed(t, doc, "throwing")["roundUp"] = false
		}, func(t *testing.T, rs *RuleSet) {
			if l := limit(rs, "throwing", infantry(t, 7)); l.Max != 2 || l.Next != 2 {
				t.Errorf("a third of 7 rounded down is 2, of 8 it is 2: %+v", l)
			}
			if l := limit(rs, "bow", infantry(t, 7)); l.Max != 3 {
				t.Errorf("bow still rounds up: %+v", l)
			}
		}},
		{"P3 B9: the classes of a limit decide what counts", func(t *testing.T, doc map[string]any) {
			limitNamed(t, doc, "bow")["classes"] = []any{"bow"}
		}, func(t *testing.T, rs *RuleSet) {
			units := infantry(t, 6)
			units[0].Equipment, units[1].Equipment = []Equipment{bow}, []Equipment{crossbow}
			if l := limit(rs, "bow", units); l.Used != 1 {
				t.Errorf("a crossbow no longer counts against the bow limit: %+v", l)
			}
		}},
		{"P3 B9: the cavalry limit counts the keyword", func(t *testing.T, doc map[string]any) {
			limitNamed(t, doc, "cavalry")["maximumPercentage"] = 50.0
		}, func(t *testing.T, rs *RuleSet) {
			units := infantry(t, 4)
			units[0].Kind = KindCavalry
			if l := limit(rs, "cavalry", units); l.Used != 1 || l.Max != 2 || l.LimitKeyword != KeywordCavalry {
				t.Errorf("cavalry limit: %+v", l)
			}
		}},
		{"P3 B10: maxPerHero 0 gives no officer a creature", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "creatures", "rules")["maxPerHero"] = 0.0
		}, func(t *testing.T, rs *RuleSet) {
			cr, offers := rs.BC.Creatures()
			if cr.MaxPerHero != 0 || len(offers) == 0 {
				t.Fatalf("creature rules %+v", cr)
			}
			if why, _ := creatureReason(cr, CompanyDetail{}, offers[0], CompanyUnit{Role: RoleLeader}); why == "" {
				t.Error("an officer cannot take a creature when the rules allow none")
			}
		}},
		{"P3 B10: unchanged creature rules, one per officer and no company-wide count", noChange, func(t *testing.T, rs *RuleSet) {
			cr, offers := rs.BC.Creatures()
			many := CompanyDetail{Creatures: make([]CompanyCreature, 5)}
			if why, _ := creatureReason(cr, many, offers[0], CompanyUnit{Role: RoleLeader, Creature: &UnitCreatureRef{}}); !strings.HasPrefix(why, "Already has") {
				t.Errorf("an officer with a creature: %q", why)
			}
			if why, _ := creatureReason(cr, many, offers[0], CompanyUnit{Role: RoleSergeant}); strings.Contains(why, "company already has") {
				t.Errorf("no company-wide limit, the count follows from the officers: %q", why)
			}
		}},
		{"P3 B10: wanderers.rules.maxAtOnce 2 lets a second wanderer in", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "wanderers", "rules")["maxAtOnce"] = 2.0
		}, func(t *testing.T, rs *RuleSet) {
			wr, _ := rs.BC.Wanderers()
			if wr.MaxAtOnce != 2 {
				t.Fatalf("max at once = %d", wr.MaxAtOnce)
			}
			f := newCompanyFixture(t)
			f.state.Influence = 40
			f.update()
			s := NewArmouryService(f.campaigns, f.companies, staticRules{rs})
			for i := 1; i <= 2; i++ {
				if err := s.HireWanderer(f.ctx, f.user, 5, 1, "Wandering Swordsman", ""); err != nil {
					t.Fatalf("wanderer %d: %v", i, err)
				}
			}
			if err := s.HireWanderer(f.ctx, f.user, 5, 1, "Wandering Swordsman", ""); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "2 wanderers") {
				t.Errorf("a third is refused with the number: %v", err)
			}
		}},
		{"P3 B10: wanderers.rules.maxAtOnce 0 hires none", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "wanderers", "rules")["maxAtOnce"] = 0.0
		}, func(t *testing.T, rs *RuleSet) {
			f := newCompanyFixture(t)
			f.state.Influence = 40
			f.update()
			s := NewArmouryService(f.campaigns, f.companies, staticRules{rs})
			if err := s.HireWanderer(f.ctx, f.user, 5, 1, "Wandering Swordsman", ""); !errors.Is(err, ErrRefused) {
				t.Errorf("no wanderer may be hired: %v", err)
			}
		}},
		{"P3 B10: a path limit of 2 lets two heroes take the Sorcerer", func(t *testing.T, doc map[string]any) {
			for _, p := range doc["paths"].([]any) {
				if m := p.(map[string]any); m["name"] == "Path of the Sorcerer" {
					m["limit"] = 2.0
				}
			}
		}, func(t *testing.T, rs *RuleSet) {
			path, ok := rs.BC.Progression().Path("Path of the Sorcerer")
			if !ok || path.LimitPerCompany != 2 {
				t.Errorf("sorcerer path = %+v", path)
			}
		}},
		{"P3 C7: the Courage and Intelligence total cap is 3 and counts gear", noChange, func(t *testing.T, rs *RuleSet) {
			pr := rs.BC.Progression()
			if pr.CITotalCap != 3 || !pr.CITotalGear {
				t.Fatalf("cap %d gear %v", pr.CITotalCap, pr.CITotalGear)
			}
			// the profile has Courage 7; gear took 1 and levelling 2, so 3 are used: one more is refused, and 4 is allowed only without gear counting
			sheet := HeroSheet{Stats: map[string]SheetStat{"courage": {Value: 4, Boost: -2}}, Gear: map[string]int{"courage": -1}}
			if v, ok := pr.improvedStat(PathProgression{}, sheet, "courage", 1, 0); ok {
				t.Errorf("a fourth point of improvement from 7 is refused: %d", v)
			}
			sheet.Stats["courage"] = SheetStat{Value: 5, Boost: -1}
			if v, ok := pr.improvedStat(PathProgression{}, sheet, "courage", 1, 0); !ok || v != 4 {
				t.Errorf("the third point is allowed: %d %v", v, ok)
			}
		}},
		{"shem: Will maximum 6 on the Tactician and Sorcerer, 3 elsewhere", noChange, func(t *testing.T, rs *RuleSet) {
			willFrom(t, rs.BC.Progression(), "Path of the Tactician", 3, 4, true)
			willFrom(t, rs.BC.Progression(), "Path of the Tactician", 6, 0, false)
			willFrom(t, rs.BC.Progression(), "Path of the Sorcerer", 5, 6, true)
			willFrom(t, rs.BC.Progression(), "Path of the Duelist", 2, 3, true)
			willFrom(t, rs.BC.Progression(), "Path of the Duelist", 3, 0, false)
			willFrom(t, rs.BC.Progression(), "", 3, 0, false)
		}},
		{"shem: a changed default peak Will of 4 holds the other paths only", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "characteristicLimits", "peak", "will")["max"] = 4.0
			for _, p := range doc["paths"].([]any) {
				if m := p.(map[string]any); m["name"] == "Path of the Duelist" {
					m["maximums"].(map[string]any)["will"] = map[string]any{"max": 5.0}
				}
			}
		}, func(t *testing.T, rs *RuleSet) {
			willFrom(t, rs.BC.Progression(), "Path of the Duelist", 3, 4, true)
			willFrom(t, rs.BC.Progression(), "Path of the Duelist", 4, 0, false)
			willFrom(t, rs.BC.Progression(), "Path of the Tactician", 5, 6, true)
		}},
		{"shem: a changed override (Tactician 4)", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "characteristicLimits", "peak", "willMaxOverrides")["Path of the Tactician"] = 4.0
		}, func(t *testing.T, rs *RuleSet) {
			willFrom(t, rs.BC.Progression(), "Path of the Tactician", 3, 4, true)
			willFrom(t, rs.BC.Progression(), "Path of the Tactician", 4, 0, false)
			willFrom(t, rs.BC.Progression(), "Path of the Sorcerer", 5, 6, true)
		}},
		{"P3 C7: courageIntelligenceTotalCap 4", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "characteristicLimits", "increases", "courageIntelligenceTotalCap")["value"] = 4.0
		}, func(t *testing.T, rs *RuleSet) {
			sheet := HeroSheet{Stats: map[string]SheetStat{"intelligence": {Value: 4, Boost: -2}}, Gear: map[string]int{"intelligence": -1}}
			if v, ok := rs.BC.Progression().improvedStat(PathProgression{}, sheet, "intelligence", 1, 0); !ok || v != 3 {
				t.Errorf("the fourth point is allowed when the cap is 4: %d %v", v, ok)
			}
		}},
		{"P3 C7: includesGear false leaves the gear's point out of the cap", func(t *testing.T, doc map[string]any) {
			campaignAt(t, doc, "campaign", "characteristicLimits", "increases", "courageIntelligenceTotalCap")["includesGear"] = false
		}, func(t *testing.T, rs *RuleSet) {
			sheet := HeroSheet{Stats: map[string]SheetStat{"courage": {Value: 4, Boost: -2}}, Gear: map[string]int{"courage": -1}}
			if v, ok := rs.BC.Progression().improvedStat(PathProgression{}, sheet, "courage", 1, 0); !ok || v != 3 {
				t.Errorf("only the 2 levelled points and the next count: %d %v", v, ok)
			}
		}},
	}
}

func TestCustomRulesetCompanyAndLimits(t *testing.T) {
	for _, tt := range p2p3Rows(t) {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) { tt.mutate(t, doc) })
			tt.check(t, rs)
		})
	}
}

// A value outside its range, or a required value that is missing, is an error of the file that names the path.
func TestCustomRulesetRefusesBadCompanyAndLimitParameters(t *testing.T) {
	company := func(doc map[string]any) map[string]any { return campaignAt(t, doc, "campaign", "company") }
	tests := []struct {
		name   string
		mutate func(doc map[string]any)
		want   string
	}{
		{"no leader", func(doc map[string]any) { company(doc)["leaders"] = 0.0 }, "campaign.company.leaders: 0 is outside 1..3"},
		{"four leaders", func(doc map[string]any) { company(doc)["leaders"] = 4.0 }, "campaign.company.leaders: 4 is outside 1..3"},
		{"seven sergeants", func(doc map[string]any) { company(doc)["sergeants"] = 7.0 }, "campaign.company.sergeants: 7 is outside 0..6"},
		{"leaders missing", func(doc map[string]any) { delete(company(doc), "leaders") }, "campaign.company.leaders is missing"},
		{"sergeants missing", func(doc map[string]any) { delete(company(doc), "sergeants") }, "campaign.company.sergeants is missing"},
		{"more officers than models", func(doc map[string]any) {
			company(doc)["maxModels"] = 2.0
		}, "more than maxModels 2"},
		{"a company with fewer models than officers", func(doc map[string]any) {
			for _, c := range doc["companies"].([]any) {
				if m := c.(map[string]any); m["name"] == "The Last Alliance" {
					m["maxModels"] = 2.0
				}
			}
		}, "the starting force has 6 models, more than maxModels 2"},
		{"maxModels too high", func(doc map[string]any) { company(doc)["maxModels"] = 51.0 }, "campaign.company.maxModels: 51 is outside 1..50"},
		{"hero start missing", func(doc map[string]any) { delete(company(doc), "heroStart") }, "campaign.company.heroStart is missing"},
		{"hero start too high", func(doc map[string]any) { company(doc)["heroStart"].(map[string]any)["will"] = 6.0 }, "campaign.company.heroStart.will: 6 is outside 0..5"},
		{"hero start fate missing", func(doc map[string]any) { delete(company(doc)["heroStart"].(map[string]any), "fate") }, "campaign.company.heroStart.fate is missing"},
		{"threshold 0", func(doc map[string]any) { campaignAt(t, doc, "campaign", "progression")["threshold"] = 0.0 }, "campaign.progression.threshold: 0 is outside 1..50"},
		{"threshold 51", func(doc map[string]any) { campaignAt(t, doc, "campaign", "progression")["threshold"] = 51.0 }, "campaign.progression.threshold: 51 is outside 1..50"},
		{"threshold missing", func(doc map[string]any) { delete(campaignAt(t, doc, "campaign", "progression"), "threshold") }, "campaign.progression.threshold is missing"},
		{"wargear threshold too high", func(doc map[string]any) {
			campaignAt(t, doc, "campaign", "rating", "hero", "wargear", "threshold")["sumAtLeast"] = 21.0
		}, "campaign.rating.hero.wargear.threshold.sumAtLeast: 21 is outside 0..20"},
		{"wargear threshold with a stat nobody has", func(doc map[string]any) {
			campaignAt(t, doc, "campaign", "rating", "hero", "wargear", "threshold")["stats"] = []any{"attack", "luck"}
		}, `campaign.rating.hero.wargear.threshold.stats[1]: "luck" is not a characteristic`},
		{"wargear threshold missing", func(doc map[string]any) {
			delete(campaignAt(t, doc, "campaign", "rating", "hero", "wargear"), "threshold")
		}, "campaign.rating.hero.wargear.threshold is missing"},
		{"limit share 0", func(doc map[string]any) { limitNamed(t, doc, "bow")["maximumPercentage"] = 0.0 }, "limits[0].maximumPercentage: 0 is outside (0, 100]"},
		{"limit share 101", func(doc map[string]any) { limitNamed(t, doc, "throwing")["maximumPercentage"] = 101.0 }, "limits[1].maximumPercentage: 101 is outside (0, 100]"},
		{"limit share missing", func(doc map[string]any) { delete(limitNamed(t, doc, "bow"), "maximumPercentage") }, "limits[0].maximumPercentage is missing"},
		{"limit roundUp missing", func(doc map[string]any) { delete(limitNamed(t, doc, "bow"), "roundUp") }, "limits[0].roundUp is missing"},
		{"limit class unknown", func(doc map[string]any) { limitNamed(t, doc, "bow")["classes"] = []any{"bow", "sling"} }, `limits[0].classes[1]: "sling" is not an item class`},
		{"creatures per hero 100", func(doc map[string]any) { campaignAt(t, doc, "creatures", "rules")["maxPerHero"] = 100.0 }, "creatures.rules.maxPerHero: 100 is outside 0..99"},
		{"wanderers at once -1", func(doc map[string]any) { campaignAt(t, doc, "wanderers", "rules")["maxAtOnce"] = -1.0 }, "wanderers.rules.maxAtOnce: -1 is outside 0..99"},
		{"cap value 0", func(doc map[string]any) {
			campaignAt(t, doc, "campaign", "characteristicLimits", "increases", "courageIntelligenceTotalCap")["value"] = 0.0
		}, "campaign.characteristicLimits.increases.courageIntelligenceTotalCap.value: 0 is outside 1..10"},
		{"path limit -1", func(doc map[string]any) {
			jsonAt(t, doc, "paths", 5).(map[string]any)["limit"] = -1.0
		}, "paths[5].limit: -1 is outside 0..99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := customRulesetErr(t, tt.mutate)
			if err == nil || !strings.Contains(err.Error(), leanWording(tt.want)) {
				t.Errorf("error = %v, want one containing %q", err, leanWording(tt.want))
			}
		})
	}
}

// willFrom checks a hero on path with Will `from` (+1): raised to want, or refused when !can.
func willFrom(t *testing.T, pr ProgressionRules, path string, from, want int, can bool) {
	t.Helper()
	p, _ := pr.Path(path)
	sheet := HeroSheet{Path: path, Stats: map[string]SheetStat{"will": {Value: from}}}
	v, ok := pr.improvedStat(p, sheet, "will", 1, 0)
	if ok != can || (can && v != want) {
		t.Errorf("%s: Will %d -> %d %v, want %d %v", path, from, v, ok, want, can)
	}
}

func TestWillMaxOverridesValidated(t *testing.T) {
	for name, val := range map[string]any{"Path of the Nobody": 6.0, "Path of the Tactician": 0.0, "Path of the Sorcerer": 11.0} {
		_, err := customRulesetErr(t, func(d map[string]any) {
			campaignAt(t, d, "campaign", "characteristicLimits", "peak", "willMaxOverrides")[name] = val
		})
		if !errors.Is(err, ErrInvalidRuleset) {
			t.Errorf("%s=%v: %v", name, val, err)
		}
	}
}

var outsideRange = regexp.MustCompile(` is outside (-?[0-9.]+\.\.-?[0-9.]+)`)

// leanWording writes the expected message of a bad parameter in the words of the lean decoder ("x: 0 is out of range 1..3", "x: missing").
func leanWording(want string) string {
	want = strings.NewReplacer("(0, 100]", "0.01..100", "is not a characteristic", "is not a valid characteristic", "is not an item class", "is not a valid gear class").Replace(want)
	want = outsideRange.ReplaceAllString(want, " is out of range $1")
	if p, ok := strings.CutSuffix(want, " is missing"); ok {
		return p + ": missing"
	}
	return want
}
