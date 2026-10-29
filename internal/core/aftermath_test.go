package core

import (
	"errors"
	"slices"
	"testing"
)

func realAftermathRules(t *testing.T) AftermathRules {
	t.Helper()
	return shippedRules(t).BC.Aftermath()
}

func TestParseAftermath(t *testing.T) {
	r := realAftermathRules(t)
	if r.Participating != 2 || r.Draw != 1 || r.Win != 2 {
		t.Errorf("influence/threshold: %+v", r)
	}
	if g, ok := r.Gain("slainHeroOrMonster"); !ok || !g.Repeatable || g.Amount != 1 || len(r.Gains) != 7 {
		t.Errorf("gains: %+v", r.Gains)
	}
	if b, _ := r.Bonus("influence"); b.InfluenceWin != 2 || b.InfluenceOther != 1 || !b.Effective() {
		t.Errorf("influence bonus: %+v", b)
	}
	if b, _ := r.Bonus("reroll"); b.Rerolls != 2 {
		t.Errorf("reroll bonus: %+v", b)
	}
	if b, _ := r.Bonus("ambush"); b.Effective() {
		t.Errorf("Ambush has no effect in the aftermath: %+v", b)
	}
	if len(r.Warrior) != 4 || len(r.Hero) != 8 {
		t.Errorf("charts: %d warrior rows, %d hero rows", len(r.Warrior), len(r.Hero))
	}
	if rules := r.Scenarios["Baggage Train"]; len(rules) != 1 || rules[0].Condition.Key != "allCargo" || r.Effects["cargo"].Amount != 1 {
		t.Errorf("Baggage Train: %+v", rules)
	}
	if r.Effects["toTheVictorTheSpoils"].MaxValue != 2 {
		t.Errorf("spoils: %+v", r.Effects["toTheVictorTheSpoils"])
	}
	if _, err := customRulesetErr(t, func(doc map[string]any) {
		inj := doc["campaign"].(map[string]any)["injuries"].(map[string]any)
		inj["warrior"] = inj["warrior"].([]any)[:2] // the chart no longer covers the injury dice
	}); !errors.Is(err, ErrInvalidRuleset) {
		t.Errorf("a chart with a gap must be rejected: %v", err)
	}
}

// Every row of both charts, at its lowest and highest roll.
func TestResolveCasualty_EveryRow(t *testing.T) {
	r := realAftermathRules(t)
	none := CasualtyUnit{}
	hurt := CasualtyUnit{Hero: true, Injuries: map[string]int{"armWound": 1}}
	cases := []struct {
		name  string
		chart []InjuryRow
		total int
		unit  CasualtyUnit
		kind  CasualtyKind
		key   string
		dec   string // the decision kind, "" for none
		bonus string
	}{
		{"warrior 2 dead", r.Warrior, 2, none, CasualtyDead, "dead", "", ""},
		{"warrior 3 dead", r.Warrior, 3, none, CasualtyDead, "dead", "", ""},
		{"warrior 4 injured", r.Warrior, 4, none, CasualtyInjured, "injured", "", ""},
		{"warrior 5 injured", r.Warrior, 5, none, CasualtyInjured, "injured", "", ""},
		{"warrior 6 recovery", r.Warrior, 6, none, CasualtyRecovered, "fullRecovery", "", ""},
		{"warrior 11 recovery", r.Warrior, 11, none, CasualtyRecovered, "fullRecovery", "", ""},
		{"warrior 12 lesson learned", r.Warrior, 12, none, CasualtyRecovered, "lessonLearned", "", "d3"},
		{"hero 2 dead", r.Hero, 2, none, CasualtyDead, "dead", "", ""},
		{"hero 3 arm wound", r.Hero, 3, none, CasualtyInjury, "armWound", "", ""},
		{"hero 4 leg wound", r.Hero, 4, none, CasualtyInjury, "legWound", "", ""},
		{"hero 5 broken honour", r.Hero, 5, none, CasualtyInjury, "brokenHonour", "", ""},
		{"hero 6 scratch", r.Hero, 6, none, CasualtyScratch, "justAScratch", "scratch", ""},
		{"hero 7 recovery, nothing to heal", r.Hero, 7, none, CasualtyRecovered, "fullRecovery", "", ""},
		{"hero 10 recovery, heals", r.Hero, 10, hurt, CasualtyHeal, "fullRecovery", "heal", ""},
		{"hero 11 valar", r.Hero, 11, none, CasualtyRecovered, "protectionByTheValar", "", ""},
		{"hero 12 wounds of a hero", r.Hero, 12, none, CasualtyRecovered, "woundsOfAHero", "", "d6"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, ok := ResolveCasualty(c.chart, c.total, c.unit)
			if !ok || e.Kind != c.kind || e.Row.Key != c.key || e.Row.BonusDice != c.bonus {
				t.Fatalf("got %+v ok=%v", e, ok)
			}
			if (e.Decision == nil) != (c.dec == "") || (e.Decision != nil && e.Decision.Kind != c.dec) {
				t.Errorf("decision: %+v, want %q", e.Decision, c.dec)
			}
		})
	}
	if _, ok := ResolveCasualty(r.Hero, 13, none); ok {
		t.Error("13 is not on the chart")
	}
	if e, _ := ResolveCasualty(r.Hero, 11, none); e.Row.Fate != 1 {
		t.Errorf("Protection by the Valar gives Fate: %+v", e.Row)
	}
	if e, _ := ResolveCasualty(r.Hero, 10, hurt); len(e.Decision.Options) != 1 || e.Decision.Options[0].Key != "armWound" || e.Decision.Options[0].Label != "Arm Wound" {
		t.Errorf("only the injuries the hero has can be healed: %+v", e.Decision)
	}
}

func TestResolveCasualty_Retirement(t *testing.T) {
	r := realAftermathRules(t)
	cases := []struct {
		name  string
		total int
		have  map[string]int
		kind  CasualtyKind
		count int
	}{
		{"first arm wound", 3, nil, CasualtyInjury, 1},
		{"second arm wound retires", 3, map[string]int{"armWound": 1}, CasualtyRetired, 2},
		{"second leg wound retires", 4, map[string]int{"legWound": 1}, CasualtyRetired, 2},
		{"second broken honour stays", 5, map[string]int{"brokenHonour": 1}, CasualtyInjury, 2},
		{"third broken honour retires", 5, map[string]int{"brokenHonour": 2}, CasualtyRetired, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, _ := ResolveCasualty(r.Hero, c.total, CasualtyUnit{Hero: true, Injuries: c.have})
			if e.Kind != c.kind || e.Count != c.count {
				t.Errorf("got %+v", e)
			}
		})
	}
}

func TestScratchReroll(t *testing.T) {
	r := realAftermathRules(t)
	u := CasualtyUnit{Hero: true}
	// the second result stands even if worse
	if e, _ := ResolveScratchReroll(r.Hero, 2, u); e.Kind != CasualtyDead {
		t.Errorf("a worse second result stands: %+v", e)
	}
	// another Scratch means the hero misses the next game, with no further choice
	if e, _ := ResolveScratchReroll(r.Hero, 6, u); e.Kind != CasualtyInjured || e.Decision != nil {
		t.Errorf("second Scratch: %+v", e)
	}
}

func TestCasualtyBonusDice(t *testing.T) {
	r := realAftermathRules(t)
	lesson, _ := ResolveCasualty(r.Warrior, 12, CasualtyUnit{})
	wounds, _ := ResolveCasualty(r.Hero, 12, CasualtyUnit{Hero: true})
	for roll, want := range map[int]int{1: 1, 2: 1, 3: 2, 4: 2, 5: 3, 6: 3} {
		if got := lesson.Bonus(&FixedDice{Rolls: []int{roll}}); got != want {
			t.Errorf("D3 from a %d: got %d, want %d", roll, got, want)
		}
	}
	if got := wounds.Bonus(&FixedDice{Rolls: []int{5}}); got != 5 {
		t.Errorf("Wounds of a Hero is a D6: %d", got)
	}
	if plain, _ := ResolveCasualty(r.Warrior, 6, CasualtyUnit{}); plain.Bonus(&FixedDice{Rolls: []int{6}}) != 0 {
		t.Error("a plain recovery has no bonus")
	}
}

func roster() []CompanyUnit {
	return []CompanyUnit{
		{ID: 1, Name: "Aldor", Role: RoleLeader, XP: 6, Points: 60},
		{ID: 2, Name: "Brand", Role: RoleSergeant, XP: 3, Points: 50},
		{ID: 3, Name: "Cene", Role: RoleSergeant, XP: 2, Points: 50},
		{ID: 4, Name: "Dal", Role: RoleHero, XP: 1, Points: 50},
		{ID: 5, Name: "Edo", Role: RoleWarrior, XP: 4, Points: 10},
		{ID: 6, Name: "Fen", Role: RoleWarrior, XP: 4, Points: 10},
		{ID: 7, Name: "Gul", Role: RoleWarrior, XP: 0, Points: 10, Injured: true},
		{ID: 8, Name: "Hob", Role: RoleWarrior, XP: 2, Points: 12, OwnPath: true},
	}
}

func TestSheetValidate(t *testing.T) {
	r := realAftermathRules(t)
	def := DefaultSheet(roster())
	if len(def.Models) != 7 || def.Models[0].Gains[participatedKey] != 1 {
		t.Fatalf("default sheet: %+v", def)
	}
	sheet := func(f func(*AftermathInput)) AftermathInput {
		in := DefaultSheet(roster())
		f(&in)
		return in
	}
	ok, err := sheet(func(in *AftermathInput) {
		in.AgainstTheOdds = []string{"experience", "experience", "ambush"}
		in.Models[4].Gains["slainHeroOrMonster"] = 3
		in.Models[4].Gains["mvp"] = 1
		in.Models[5].Gains["woundedEnemy"] = 0
	}).Validate(roster(), r, CompanyResult{})
	if err != nil || len(ok.AgainstTheOdds) != 2 || ok.Models[4].Gains["slainHeroOrMonster"] != 3 {
		t.Fatalf("valid sheet: %+v %v", ok, err)
	}
	if empty, err := (AftermathInput{}).Validate(roster(), r, CompanyResult{}); err != nil || len(empty.Models) != 7 {
		t.Errorf("missing units get the default: %+v %v", empty, err)
	}
	bad := map[string]AftermathInput{
		"unknown unit":             sheet(func(in *AftermathInput) { in.Models[0].UnitID = 99 }),
		"absent unit on sheet":     sheet(func(in *AftermathInput) { in.Models[0].UnitID = 7 }),
		"unknown gain":             sheet(func(in *AftermathInput) { in.Models[0].Gains["bogus"] = 1 }),
		"flag twice":               sheet(func(in *AftermathInput) { in.Models[0].Gains["woundedEnemy"] = 2 }),
		"repeatable too many":      sheet(func(in *AftermathInput) { in.Models[0].Gains["slainHeroOrMonster"] = 10 }),
		"two MVPs":                 sheet(func(in *AftermathInput) { in.Models[0].Gains["mvp"], in.Models[1].Gains["mvp"] = 1, 1 }),
		"gain without taking part": sheet(func(in *AftermathInput) { in.Models[0].Gains = map[string]int{"woundedEnemy": 1} }),
		"unknown outcome":          sheet(func(in *AftermathInput) { in.Models[0].Outcome = "lost" }),
		"unknown bonus":            sheet(func(in *AftermathInput) { in.AgainstTheOdds = []string{"nope"} }),
		"relic not ticked":         sheet(func(in *AftermathInput) { in.Models[0].Relic = true }),
	}
	for name, in := range bad {
		if _, err := in.Validate(roster(), r, CompanyResult{}); !errors.Is(err, ErrInvalidAftermath) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	// the relic: exactly one bearer when the result ticked it, never a casualty
	ticked := CompanyResult{Objectives: map[string]int{"possessesRelic": 1}}
	if _, err := DefaultSheet(roster()).Validate(roster(), r, ticked); !errors.Is(err, ErrInvalidAftermath) {
		t.Errorf("a ticked relic needs a bearer: %v", err)
	}
	one := sheet(func(in *AftermathInput) { in.Models[2].Relic = true })
	if _, err := one.Validate(roster(), r, ticked); err != nil {
		t.Errorf("one bearer: %v", err)
	}
	two := sheet(func(in *AftermathInput) { in.Models[2].Relic, in.Models[3].Relic = true, true })
	if _, err := two.Validate(roster(), r, ticked); !errors.Is(err, ErrInvalidAftermath) {
		t.Errorf("two bearers: %v", err)
	}
	dead := sheet(func(in *AftermathInput) { in.Models[2].Relic, in.Models[2].Outcome = true, OutcomeCasualty })
	if _, err := dead.Validate(roster(), r, ticked); !errors.Is(err, ErrInvalidAftermath) {
		t.Errorf("a casualty cannot hold the relic: %v", err)
	}
}

func ctxFor(t *testing.T, outcome, scenario string, fled bool, result CompanyResult) AftermathContext {
	return AftermathContext{Rules: realAftermathRules(t), Outcome: outcome, Fled: fled, Scenario: scenario, Result: result, Roster: roster()}
}

func gainOf(gains []UnitGain, id int64) UnitGain {
	for _, g := range gains {
		if g.UnitID == id {
			return g
		}
	}
	return UnitGain{}
}

func TestExperience(t *testing.T) {
	ctx := ctxFor(t, "loss", "", false, CompanyResult{})
	in := DefaultSheet(roster())
	in.Models[0].Gains["woundedEnemy"] = 1
	in.Models[0].Gains["slainHeroOrMonster"] = 2
	in.Models[0].Gains["mvp"] = 1
	in.Models[3].Gains = nil // Dal did not take part
	in.Models[6].Gains = map[string]int{participatedKey: 1}
	got := Experience(ctx, in)
	if g := gainOf(got, 1); g.Total != 5 { // participated 1 + wounded 1 + 2 slain + mvp 1
		t.Errorf("Aldor: %+v", g)
	}
	if g := gainOf(got, 4); g.Total != 0 || len(g.Lines) != 0 {
		t.Errorf("a unit that did not take part earns nothing: %+v", g)
	}
	if g := gainOf(got, 8); g.Total != 0 { // On Its Own Path
		t.Errorf("On Its Own Path earns nothing: %+v", g)
	}
	if _, ok := in.Model(7); ok {
		t.Error("the absent unit is not on the sheet")
	}

	// Against the Odds: +2 per participating unit for a win, +1 otherwise
	in.AgainstTheOdds = []string{"experience"}
	if g := gainOf(Experience(ctx, in), 5); g.Total != 2 { // 1 + 1
		t.Errorf("loss bonus: %+v", g)
	}
	win := ctxFor(t, "win", "", false, CompanyResult{})
	if g := gainOf(Experience(win, in), 5); g.Total != 3 {
		t.Errorf("win bonus: %+v", g)
	}
	if g := gainOf(Experience(win, in), 4); g.Total != 0 {
		t.Errorf("the bonus is for participating units only: %+v", g)
	}
}

func TestInfluence(t *testing.T) {
	sum := func(lines []AmountLine) int {
		n := 0
		for _, l := range lines {
			n += l.Amount
		}
		return n
	}
	in := DefaultSheet(roster())
	cases := []struct {
		name    string
		outcome string
		fled    bool
		odds    []string
		want    int
	}{
		{"loss", "loss", false, nil, 2},
		{"draw", "draw", false, nil, 3},
		{"win", "win", false, nil, 4},
		{"win with the bonus", "win", false, []string{"influence"}, 6},
		{"loss with the bonus", "loss", false, []string{"influence"}, 3},
		{"draw with the bonus", "draw", false, []string{"influence"}, 4},
		{"All is Lost: participating only", "loss", true, nil, 2},
		{"All is Lost keeps the influence bonus", "loss", true, []string{"influence"}, 3},
		{"no-effect bonus", "win", false, []string{"ambush", "toolKit"}, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in.AgainstTheOdds = c.odds
			ctx := ctxFor(t, c.outcome, "", c.fled, CompanyResult{})
			if got := sum(Influence(ctx, in)); got != c.want {
				t.Errorf("got %d, want %d: %+v", got, c.want, Influence(ctx, in))
			}
		})
	}
	in.AgainstTheOdds = []string{"reroll", "influence"}
	if n := RerollsFor(realAftermathRules(t), in); n != 2 {
		t.Errorf("re-rolls: %d", n)
	}
}

func TestRewards(t *testing.T) {
	in := DefaultSheet(roster())
	// Baggage Train: +1 Influence when the company ticked all cargo
	cargo := ctxFor(t, "loss", "Baggage Train", false, CompanyResult{Objectives: map[string]int{"allCargo": 1}})
	if r := Rewards(cargo, in); len(r) != 1 || r[0].Kind != "influence" || r[0].Amount != 1 {
		t.Errorf("cargo: %+v", r)
	}
	if lines := Influence(cargo, in); lines[len(lines)-1].Amount != 1 {
		t.Errorf("cargo in the breakdown: %+v", lines)
	}
	if r := Rewards(ctxFor(t, "loss", "Baggage Train", false, CompanyResult{}), in); len(r) != 0 {
		t.Errorf("no tick, no reward: %+v", r)
	}
	// All is Lost: no scenario rewards
	if r := Rewards(ctxFor(t, "loss", "Baggage Train", true, CompanyResult{Objectives: map[string]int{"allCargo": 1}}), in); len(r) != 0 {
		t.Errorf("a fled company gets no rewards: %+v", r)
	}
	// Rescue: the Leader's survival earns an Experience Point
	rescue := ctxFor(t, "loss", "Rescue", false, CompanyResult{})
	if g := gainOf(Experience(rescue, in), 1); g.Total != 2 {
		t.Errorf("survival of the fittest: %+v", g)
	}
	fallen := DefaultSheet(roster())
	fallen.Models[0].Outcome = OutcomeCasualty
	if g := gainOf(Experience(rescue, fallen), 1); g.Total != 1 {
		t.Errorf("a leader removed from the board earns nothing extra: %+v", g)
	}
	// victory-only perks: the winner only (a draw has no victor)
	for outcome, want := range map[string]int{"win": 1, "draw": 0, "loss": 0} {
		got := Rewards(ctxFor(t, outcome, "Chance Encounter", false, CompanyResult{}), in)
		if len(got) != want || (want == 1 && (got[0].Kind != "perk" || got[0].Key != "rendezvous")) {
			t.Errorf("Rendezvous for a %s: %+v", outcome, got)
		}
	}
	// To the Victor the Spoils: a decision at Finish
	spoils := Rewards(ctxFor(t, "win", "Scavenge the Battlefield", false, CompanyResult{Objectives: map[string]int{"allObjectiveMarkers": 1}}), in)
	if len(spoils) != 1 || !spoils[0].Decision || spoils[0].MaxValue != 2 {
		t.Errorf("spoils: %+v", spoils)
	}
	// the relic: only when ticked, the bearer gets the perk
	relicCtx := ctxFor(t, "win", "Relic of Ages Past", false, CompanyResult{Objectives: map[string]int{"possessesRelic": 1}})
	if r := Rewards(relicCtx, in); len(r) != 0 {
		t.Errorf("no bearer on the sheet: %+v", r)
	}
	in.Models[3].Relic = true
	if r := Rewards(relicCtx, in); len(r) != 1 || r[0].UnitID != 4 || r[0].Kind != "perk" {
		t.Errorf("relic: %+v", r)
	}
	if r := Rewards(ctxFor(t, "win", "Relic of Ages Past", false, CompanyResult{}), in); len(r) != 0 {
		t.Errorf("not ticked: %+v", r)
	}
}

func TestPreview(t *testing.T) {
	ctx := ctxFor(t, "win", "", false, CompanyResult{})
	in := DefaultSheet(roster())
	in.Models[4].Outcome, in.Models[5].Outcome = OutcomeCasualty, OutcomeCasualty
	in.Models[1].Outcome = OutcomeFled
	p := Preview(ctx, in)
	if p.Rolls != 2 || p.InfluenceTotal() != 4 || len(p.Experience) != 7 {
		t.Errorf("preview: %+v", p)
	}
}

func TestCompanyOutcome(t *testing.T) {
	a, b := CampaignCompany{ID: 1}, CampaignCompany{ID: 2}
	m := Match{Ended: true, Winner: 1, Sides: []MatchSide{{Companies: []CampaignCompany{a}}, {Companies: []CampaignCompany{b}}}}
	for _, c := range []struct {
		id      int64
		winner  int
		fled    bool
		outcome string
	}{{1, 1, false, "win"}, {2, 1, false, "loss"}, {1, 0, false, "draw"}, {2, 0, false, "draw"}, {2, 1, true, "loss"}} {
		m.Winner = c.winner
		m.Result = MatchResult{Companies: []CompanyResult{{CompanyID: c.id, Fled: c.fled}}}
		if got, fled, ok := CompanyOutcome(m, c.id); !ok || got != c.outcome || fled != c.fled {
			t.Errorf("%+v: %s %v %v", c, got, fled, ok)
		}
	}
	if _, _, ok := CompanyOutcome(m, 9); ok {
		t.Error("a company outside the match has no outcome")
	}
}

func TestSuccession(t *testing.T) {
	without := func(ids ...int64) []CompanyUnit {
		var out []CompanyUnit
		for _, u := range roster() {
			if !slices.Contains(ids, u.ID) {
				out = append(out, u)
			}
		}
		return out
	}
	if s := NextSuccession(without(), false, 1); s.NeedLeader {
		t.Error("nothing to decide when the leader lives")
	}
	// the leader fell: a sergeant takes over
	s := NextSuccession(without(1), true, 1)
	if !s.NeedLeader || !slices.Equal(s.LeaderOptions, []int64{2, 3}) {
		t.Errorf("leader: %+v", s)
	}
	// after Brand becomes leader there is one sergeant: another hero fills the place
	next := without(1)
	next[0].Role = RoleLeader // Brand
	if s := NextSuccession(next, true, 1); s.NeedLeader {
		t.Errorf("somebody already took over: %+v", s)
	}
	sg := NextSergeant(next, false, 2)
	if !sg.NeedSergeant || !slices.Equal(sg.SergeantOptions, []int64{4}) {
		t.Errorf("sergeant: %+v", sg)
	}
	if NextSergeant(roster(), false, 2).NeedSergeant {
		t.Error("two sergeants are enough")
	}
	// no sergeants: the other heroes
	if s := NextSuccession(without(1, 2, 3), true, 1); !slices.Equal(s.LeaderOptions, []int64{4}) {
		t.Errorf("heroes: %+v", s)
	}
	// no hero left: the most experienced warrior (a tie of XP goes to the points, a full tie to the player)
	only := without(1, 2, 3, 4)
	if s := NextSuccession(only, true, 1); !slices.Equal(s.Fallback, []int64{5, 6}) {
		t.Errorf("tie: %+v", s)
	}
	only[0].Points = 11
	if s := NextSuccession(only, true, 1); !slices.Equal(s.Fallback, []int64{5}) {
		t.Errorf("points break the tie: %+v", s)
	}
}

func TestAftermathAccess(t *testing.T) {
	co := CampaignCompany{ID: 1, OwnerID: 7}
	ended, open := Match{Ended: true}, Match{}
	cases := []struct {
		name string
		u    User
		m    Match
		want AftermathAccess
	}{
		{"owner", User{ID: 7}, ended, AftermathEdit},
		{"other player", User{ID: 8}, ended, AftermathNone},
		{"organiser", User{ID: 9}, ended, AftermathEdit},
		{"admin", User{ID: 10, Role: RoleAdmin}, ended, AftermathEdit},
		{"owner of an open match", User{ID: 7}, open, AftermathNone},
	}
	for _, c := range cases {
		if got := AftermathAccessFor(c.u, c.m, co, 9); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	if !CanWaive(User{ID: 9}, 9) || CanWaive(User{ID: 7}, 9) || !CanWaive(User{Role: RoleSuperuser}, 9) {
		t.Error("only the organiser and administrators waive")
	}
}

func TestAftermathStatusLocks(t *testing.T) {
	for s, want := range map[AftermathStatus]bool{AftermathPending: true, AftermathSheet: true, AftermathInjuries: true, AftermathLevelUps: true, AftermathDone: false, AftermathWaived: false} {
		if s.Locks() != want {
			t.Errorf("%s locks=%v", s, s.Locks())
		}
	}
}

func TestDistinguish(t *testing.T) {
	roster := []CompanyUnit{
		{ID: 1, Type: "Orc Warrior", Role: RoleLeader},
		{ID: 2, Type: "Orc Warrior", Role: RoleSergeant},
		{ID: 3, Type: "Orc Warrior", Role: RoleSergeant},
		{ID: 4, Type: "Orc Warrior", Role: RoleWarrior},
		{ID: 5, Type: "Orc Warrior", Role: RoleWarrior},
		{ID: 6, Type: "Orc Archer", Role: RoleWarrior},
		{ID: 7, Type: "Orc Archer", Role: RoleWarrior, Name: "Snaga"},
	}
	want := []string{"Orc Warrior (Leader)", "Orc Warrior (Sergeant) 1", "Orc Warrior (Sergeant) 2", "Orc Warrior 1", "Orc Warrior 2", "Orc Archer", "Snaga"}
	got := Distinguish(roster)
	for i, u := range got {
		if u.Title() != want[i] {
			t.Errorf("unit %d: %q, want %q", u.ID, u.Title(), want[i])
		}
	}
	if roster[0].Name != "" {
		t.Error("the caller's roster must not change")
	}
}

func TestAllIsLostKeepsTheExperienceBonus(t *testing.T) {
	in := DefaultSheet(roster())
	in.AgainstTheOdds = []string{"experience"}
	rules := realAftermathRules(t)
	if _, ok := rules.Bonus("experience"); !ok {
		t.Skip("no experience bonus in the file")
	}
	fled := Experience(ctxFor(t, "loss", "", true, CompanyResult{}), in)
	stayed := Experience(ctxFor(t, "loss", "", false, CompanyResult{}), in)
	if len(fled) == 0 || fled[0].Total != stayed[0].Total || fled[0].Total == 0 {
		t.Errorf("a fled company gets the same experience bonus: %+v vs %+v", fled, stayed)
	}
}

// A Sergeant who died is replaced: a Hero first (optional), else a Warrior with the most Experience (not optional).
func TestSergeantSuccession(t *testing.T) {
	without := func(ids ...int64) []CompanyUnit {
		var out []CompanyUnit
		for _, u := range roster() {
			if !slices.Contains(ids, u.ID) {
				out = append(out, u)
			}
		}
		return out
	}
	s := NextSergeant(without(3), true, 2) // Cene died; Dal is a Hero
	if !s.NeedSergeant || !slices.Equal(s.SergeantOptions, []int64{4}) || len(s.Fallback) != 0 {
		t.Errorf("a Hero first: %+v", s)
	}
	s = NextSergeant(without(3, 4), true, 2) // no Hero left: Edo and Fen tie on Experience and points
	if !s.NeedSergeant || len(s.SergeantOptions) != 0 || !slices.Equal(s.Fallback, []int64{5, 6}) {
		t.Errorf("the Warriors with the most Experience: %+v", s)
	}
	if s := NextSergeant(without(3, 4), false, 2); s.NeedSergeant {
		t.Errorf("without a lost Sergeant a Warrior is not made an officer: %+v", s)
	}
	if s := NextSergeant(without(2, 3, 4, 5, 6, 7, 8), true, 2); s.NeedSergeant {
		t.Errorf("nobody to choose: %+v", s)
	}
}

func TestWoundsOfAHeroWithAnInjuryKeepsItsBonus(t *testing.T) {
	r := realAftermathRules(t)
	e, _ := ResolveCasualty(r.Hero, 12, CasualtyUnit{Hero: true, Injuries: map[string]int{"armWound": 1}})
	if e.Row.BonusDice != "d6" {
		t.Fatalf("Wounds of a Hero keeps its D6 influence: %+v", e.Row)
	}
	if e.Kind != CasualtyRecovered || e.Decision != nil { // no heal question: the bonus die is never held back
		t.Errorf("Wounds of a Hero is a plain Full Recovery in the data: kind %s, decision %+v", e.Kind, e.Decision)
	}
}

// BC: a Wanderer cannot gain Experience Points or advancements: the aftermath credits it none (also with Against the Odds and a reward) and
// the card never shows "Level up ready" for it, whatever XP it carries.
func TestExperienceSkipsWanderers(t *testing.T) {
	ctx := ctxFor(t, "win", "", false, CompanyResult{})
	ro := append(roster(), CompanyUnit{ID: 9, Name: "Wandering Ruffian", Role: RoleWarrior, Points: 10, Wanderer: true})
	ctx.Roster = ro
	in := DefaultSheet(ro)
	in.AgainstTheOdds = []string{"experience"}
	in.Models[len(in.Models)-1].Gains["mvp"] = 1
	if g := gainOf(Experience(ctx, in), 9); g.Total != 0 || len(g.Lines) != 0 {
		t.Errorf("a Wanderer earns nothing: %+v", g)
	}
	if g := gainOf(Experience(ctx, in), 5); g.Total == 0 {
		t.Errorf("a warrior still earns: %+v", g)
	}
	if n := (CompanyUnit{Wanderer: true, XP: 10, LevelEvery: 5}).PendingLevelUps(); n != 0 {
		t.Errorf("a Wanderer has %d level-ups waiting", n)
	}
	if n := (CompanyUnit{OwnPath: true, XP: 10, LevelEvery: 5}).PendingLevelUps(); n != 0 {
		t.Errorf("a unit On Its Own Path has %d level-ups waiting", n)
	}
	if n := (CompanyUnit{XP: 10, LevelEvery: 5}).PendingLevelUps(); n != 2 {
		t.Errorf("an ordinary unit has %d level-ups waiting", n)
	}
}
