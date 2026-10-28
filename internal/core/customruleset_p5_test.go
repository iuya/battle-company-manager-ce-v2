package core

import (
	"strings"
	"testing"
)

// Slice P5 rows of TestCustomRuleset (docs/definitions-lean-design.md 12.8): the break point comparison and percent, the
// no-promotion amounts and the stat an injury row gains.

func valar(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	for _, r := range jsonAt(t, doc, "campaign", "injuries", "hero").([]any) {
		if m := r.(map[string]any); m["key"] == "protectionByTheValar" {
			return m
		}
	}
	t.Fatal("no protectionByTheValar row")
	return nil
}

func TestCustomRulesetBreakComparison(t *testing.T) {
	tests := []struct {
		name       string
		comparison any // nil: leave the shipped value
		wantBroken bool
	}{
		{"shipped: exactly half is not Broken", nil, false},
		{"gt: exactly half is not Broken", "gt", false},
		{"gte: exactly half is Broken", "gte", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) {
				if tt.comparison != nil {
					campaignAt(t, doc, "campaign", "breakPoint", "threshold")["comparison"] = tt.comparison
				}
			})
			rules := rs.BC.BreakRules()
			b := newBP(t, 12, 50)
			b.cfg.Break[1] = BreakConfig{AtLeast: rules.AtLeast, Percent: 50}
			b.fall(6)
			if got := b.state().Break(1); got.WillBreak != tt.wantBroken {
				t.Errorf("WillBreak = %v, want %v", got.WillBreak, tt.wantBroken)
			}
			b.turn()
			if got := b.state().Break(1).Broken; got != tt.wantBroken {
				t.Errorf("Broken = %v, want %v", got, tt.wantBroken)
			}
			// 5 of 12 is below the Break Point under either comparison
			b = newBP(t, 12, 50)
			b.cfg.Break[1] = BreakConfig{AtLeast: rules.AtLeast, Percent: 50}
			b.fall(5)
			b.turn()
			if b.state().Break(1).Broken {
				t.Error("5 of 12 casualties must not break the company")
			}
		})
	}
}

func TestCustomRulesetRefusesBadBreakPoint(t *testing.T) {
	bp := func(doc map[string]any) map[string]any { return campaignAt(t, doc, "campaign", "breakPoint") }
	tests := []struct {
		name   string
		mutate func(doc map[string]any)
		want   string
	}{
		{"unknown comparison", func(doc map[string]any) { bp(doc)["threshold"].(map[string]any)["comparison"] = "ge" }, `campaign.breakPoint.threshold.comparison: "ge" is not a comparison; known: gt, gte`},
		{"percent 0", func(doc map[string]any) { bp(doc)["default"].(map[string]any)["percent"] = 0.0 }, "campaign.breakPoint.default.percent: 0 is outside 1..100"},
		{"percent 101", func(doc map[string]any) { bp(doc)["default"].(map[string]any)["percent"] = 101.0 }, "campaign.breakPoint.default.percent: 101 is outside 1..100"},
		{"percent missing", func(doc map[string]any) { delete(bp(doc)["default"].(map[string]any), "percent") }, "campaign.breakPoint.default.percent is missing"},
		{"company percent 120", func(doc map[string]any) {
			for _, c := range doc["data"].(map[string]any)["companies"].([]any) {
				if m := c.(map[string]any); m["name"] == "Muster of Isengard" {
					m["breakPointPercentage"] = 120.0
				}
			}
		}, "data.companies[Muster of Isengard].breakPointPercentage: 120 is outside 1..100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := customRulesetErr(t, tt.mutate)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestCustomRulesetNoPromotionAmounts(t *testing.T) {
	warrior := func(courage, intel int) CompanyUnit {
		return CompanyUnit{Name: "Hob", Stats: []Attribute{{Name: "Courage", Value: courage}, {Name: "Intelligence", Value: intel}}}
	}
	tests := []struct {
		name         string
		courage, int any // nil: shipped
		maximum      any
		unit         CompanyUnit
		want         map[string]int // Progress.Bonus
		wantText     string
	}{
		{"shipped: one each", nil, nil, nil, warrior(5, 4), map[string]int{"courage": -1, "intelligence": -1}, "Courage 4+ and Intelligence 3+"},
		{"courage amount 2", 2.0, nil, nil, warrior(5, 4), map[string]int{"courage": -2, "intelligence": -1}, "Courage 3+"},
		{"amount stops at the maximum", 2.0, nil, nil, warrior(4, 4), map[string]int{"courage": -1, "intelligence": -1}, "Courage 3+"},
		{"intelligence amount 0 leaves it", nil, 0.0, nil, warrior(5, 4), map[string]int{"courage": -1}, "Courage 4+"},
		{"maximum 2", nil, nil, 2.0, warrior(3, 3), map[string]int{"courage": -1, "intelligence": -1}, "Courage 2+"},
		{"already at the maximum", nil, nil, nil, warrior(3, 3), nil, "as good as they go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) {
				np := campaignAt(t, doc, "campaign", "progression", "warrior", "noPromotion")
				if tt.courage != nil {
					np["courage"] = tt.courage
				}
				if tt.int != nil {
					np["intelligence"] = tt.int
				}
				if tt.maximum != nil {
					np["maximum"] = tt.maximum
				}
			})
			var p Progress
			text := noPromotion(tt.unit, &p, rs.BC.Progression())
			if len(p.Bonus) != len(tt.want) {
				t.Errorf("bonus = %v, want %v", p.Bonus, tt.want)
			}
			for k, v := range tt.want {
				if p.Bonus[k] != v {
					t.Errorf("bonus[%s] = %d, want %d (%v)", k, p.Bonus[k], v, p.Bonus)
				}
			}
			if !strings.Contains(text, tt.wantText) {
				t.Errorf("text = %q, want it to contain %q", text, tt.wantText)
			}
		})
	}
}

func TestCustomRulesetRefusesBadNoPromotion(t *testing.T) {
	np := func(doc map[string]any) map[string]any {
		return campaignAt(t, doc, "campaign", "progression", "warrior", "noPromotion")
	}
	tests := []struct {
		name   string
		mutate func(doc map[string]any)
		want   string
	}{
		{"courage 6", func(doc map[string]any) { np(doc)["courage"] = 6.0 }, "campaign.progression.warrior.noPromotion.courage: 6 is outside 0..5"},
		{"intelligence -1", func(doc map[string]any) { np(doc)["intelligence"] = -1.0 }, "noPromotion.intelligence: -1 is outside 0..5"},
		{"maximum 0", func(doc map[string]any) { np(doc)["maximum"] = 0.0 }, "noPromotion.maximum: 0 is outside 1..10"},
		{"maximum missing", func(doc map[string]any) { delete(np(doc), "maximum") }, "noPromotion.maximum is missing"},
		{"courage missing", func(doc map[string]any) { delete(np(doc), "courage") }, "noPromotion.courage is missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := customRulesetErr(t, tt.mutate)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestCustomRulesetInjuryStatGain(t *testing.T) {
	rowOf := func(t *testing.T, mutate func(doc map[string]any)) (InjuryRow, error) {
		t.Helper()
		rules, err := ParseAftermath(customBC(t, mutate))
		if err != nil {
			return InjuryRow{}, err
		}
		for _, r := range rules.Hero {
			if r.Key == "protectionByTheValar" {
				return r, nil
			}
		}
		t.Fatal("no protectionByTheValar row")
		return InjuryRow{}, nil
	}
	t.Run("shipped: Fate", func(t *testing.T) {
		r, err := rowOf(t, func(map[string]any) {})
		if err != nil || r.StatGains["fate"] != 1 || r.Fate != 1 || len(r.StatGains) != 1 {
			t.Errorf("row = %+v, %v", r, err)
		}
	})
	t.Run("Might instead of Fate, by 2", func(t *testing.T) {
		r, err := rowOf(t, func(doc map[string]any) {
			sc := valar(t, doc)["statChanges"].([]any)[0].(map[string]any)
			sc["stat"], sc["delta"] = "might", 2.0
		})
		if err != nil || r.StatGains["might"] != 2 || r.Fate != 0 || len(r.StatGains) != 1 {
			t.Errorf("row = %+v, %v", r, err)
		}
	})
	for name, tc := range map[string]struct {
		stat  any
		delta any
		want  string
	}{
		"unknown stat":   {"luck", 1.0, `statChanges[0].stat: "luck" is not a characteristic`},
		"a loss is kept": {"fate", -1.0, "statChanges[0].delta: -1 must be a gain"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := rowOf(t, func(doc map[string]any) {
				sc := valar(t, doc)["statChanges"].([]any)[0].(map[string]any)
				sc["stat"], sc["delta"] = tc.stat, tc.delta
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// The experience of a gain and the Reroll bonus are printed from the rules (H4, H7).
func TestCustomRulesetExperienceAndRerollFigures(t *testing.T) {
	rs := customRuleset(t, func(doc map[string]any) {
		for _, g := range jsonAt(t, doc, "campaign", "experience", "gains").([]any) {
			switch m := g.(map[string]any); m["key"] {
			case "objective":
				m["amount"] = 2.0
			case "mvp":
				m["amount"] = 3.0
			}
		}
		for _, b := range jsonAt(t, doc, "campaign", "againstTheOdds", "bonuses").([]any) {
			if m := b.(map[string]any); m["key"] == "reroll" {
				m["rating"], m["count"] = 40.0, 3.0
			}
		}
	})
	if o, m := rs.BC.ExperienceGain(GainObjective), rs.BC.ExperienceGain(GainMVP); o != 2 || m != 3 {
		t.Errorf("objective %d, mvp %d experience, want 2 and 3", o, m)
	}
	if d := ruleDice(rs); d.RerollRating != 40 || d.RerollCount != 3 {
		t.Errorf("reroll = %d rating, %d re-rolls, want 40 and 3", d.RerollRating, d.RerollCount)
	}
	shipped := customRuleset(t, nil)
	if d := ruleDice(shipped); d.RerollRating != 15 || d.RerollCount != 2 || shipped.BC.ExperienceGain(GainMVP) != 1 {
		t.Errorf("shipped: reroll %+v, mvp %d", d, shipped.BC.ExperienceGain(GainMVP))
	}
}

func gearAnnotation(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	for _, it := range jsonAt(t, doc, "data", "gearAnnotations", "items").([]any) {
		if m := it.(map[string]any); m["name"] == name {
			return m
		}
	}
	t.Fatalf("no gear annotation %q", name)
	return nil
}

// B6, B7: the equipment slots of a model and the slots the Backpack adds come from the file.
func TestCustomRulesetEquipmentSlots(t *testing.T) {
	tests := []struct {
		name         string
		large, small any // nil: shipped
		backpack     any // grantsSlots.small of the Backpack; nil: shipped
		want         Slots
	}{
		{"shipped: 1 large, 1 small, a Backpack holds three more", nil, nil, nil, Slots{LargeUsed: 1, LargeMax: 1, SmallUsed: 1, SmallMax: 4}},
		{"slots 2 large and 1 small", 2.0, 1.0, nil, Slots{LargeUsed: 1, LargeMax: 2, SmallUsed: 1, SmallMax: 4}},
		{"slots 1 large and 2 small", 1.0, 2.0, nil, Slots{LargeUsed: 1, LargeMax: 1, SmallUsed: 1, SmallMax: 5}},
		{"the Backpack grants 2", nil, nil, 2.0, Slots{LargeUsed: 1, LargeMax: 1, SmallUsed: 1, SmallMax: 3}},
		{"the Backpack grants 0", nil, nil, 0.0, Slots{LargeUsed: 1, LargeMax: 1, SmallUsed: 1, SmallMax: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, func(doc map[string]any) {
				if tt.large != nil {
					campaignAt(t, doc, "campaign", "company", "equipmentSlots")["large"] = tt.large
					campaignAt(t, doc, "campaign", "company", "equipmentSlots")["small"] = tt.small
				}
				if tt.backpack != nil {
					gearAnnotation(t, doc, "Backpack")["grantsSlots"] = map[string]any{"small": tt.backpack}
				}
			})
			pack, _ := equipmentFor(rs, BCCompany{}, "Backpack", true, false, false)
			u := CompanyUnit{BaseSlots: rs.BC.Shape().EquipmentSlots, Equipment: []Equipment{pack}}
			if got := u.Slots(); got.LargeMax != tt.want.LargeMax || got.SmallMax != tt.want.SmallMax || got.LargeUsed != 1 {
				t.Errorf("slots = %+v, want maxima %d large and %d small", got, tt.want.LargeMax, tt.want.SmallMax)
			}
			built := buildUnit(rs, BCCompany{}, UnitState{ID: 1, Base: "Warrior of Minas Tirith", Role: RoleWarrior}, CampaignSettings{})
			if built.BaseSlots != rs.BC.Shape().EquipmentSlots {
				t.Errorf("a built unit carries the base slots %+v, want %+v", built.BaseSlots, rs.BC.Shape().EquipmentSlots)
			}
		})
	}
}

func TestCustomRulesetRefusesBadSlots(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(doc map[string]any)
		want   string
	}{
		{"large 11", func(doc map[string]any) { campaignAt(t, doc, "campaign", "company", "equipmentSlots")["large"] = 11.0 }, "campaign.company.equipmentSlots.large: 11 is outside 0..10"},
		{"small -1", func(doc map[string]any) { campaignAt(t, doc, "campaign", "company", "equipmentSlots")["small"] = -1.0 }, "campaign.company.equipmentSlots.small: -1 is outside 0..10"},
		{"small missing", func(doc map[string]any) { delete(campaignAt(t, doc, "campaign", "company", "equipmentSlots"), "small") }, "campaign.company.equipmentSlots.small is missing"},
		{"equipmentSlots missing", func(doc map[string]any) { delete(campaignAt(t, doc, "campaign", "company"), "equipmentSlots") }, "campaign.company.equipmentSlots is missing"},
		{"Backpack grants 11", func(doc map[string]any) {
			gearAnnotation(t, doc, "Backpack")["grantsSlots"] = map[string]any{"small": 11.0}
		}, "data.gearAnnotations.items[Backpack].grantsSlots.small: 11 is outside 0..10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := customRulesetErr(t, tt.mutate)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

// G3: who may take the Elven-made items is the race the file names.
func TestCustomRulesetItemRace(t *testing.T) {
	setRace := func(race string) func(doc map[string]any) {
		return func(doc map[string]any) {
			gearAnnotation(t, doc, "Elven spear")["who"] = map[string]any{"race": race}
		}
	}
	tests := []struct {
		name         string
		mutate       func(doc map[string]any)
		manBlocked   bool
		elfBlocked   bool
		wantWhyOfMan string
	}{
		{"shipped: Elf only", nil, true, false, "Elven-made weaponry is for Elves"},
		{"race Man: Men may, Elves may not", setRace("Man"), false, true, ""},
		{"no race: nobody is restricted", func(doc map[string]any) { delete(gearAnnotation(t, doc, "Elven spear"), "who") }, false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := customRuleset(t, tt.mutate)
			bcCo, _ := rs.BC.Company("Fornost")
			sh := shop{rs: rs, bcC: bcCo}
			offer, ok := rs.BC.Offer(bcCo, "Elven spear")
			if !ok {
				t.Fatal("no Elven spear in the armoury")
			}
			why := func(unit string) string {
				w, _ := sh.whyNot(CompanyUnit{Type: unit, Role: RoleHero}, offer)
				return w
			}
			const msg = "Elven-made weaponry is for"
			if got := strings.HasPrefix(why("Warrior of Minas Tirith"), msg); got != tt.manBlocked {
				t.Errorf("Man blocked = %v (%q), want %v", got, why("Warrior of Minas Tirith"), tt.manBlocked)
			}
			if got := strings.HasPrefix(why("Rivendell Warrior"), msg); got != tt.elfBlocked {
				t.Errorf("Elf blocked = %v (%q), want %v", got, why("Rivendell Warrior"), tt.elfBlocked)
			}
			if tt.wantWhyOfMan != "" && why("Warrior of Minas Tirith") != tt.wantWhyOfMan {
				t.Errorf("message = %q, want %q", why("Warrior of Minas Tirith"), tt.wantWhyOfMan)
			}
		})
	}
}

func TestCustomRulesetRefusesUnknownItemRace(t *testing.T) {
	_, err := customRulesetErr(t, func(doc map[string]any) {
		gearAnnotation(t, doc, "Elven spear")["who"] = map[string]any{"race": "Quux"}
	})
	if want := `data.gearAnnotations.items[Elven spear].who.race: "Quux" is not a race of the vanilla data`; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("error = %v, want one containing %q", err, want)
	}
}
