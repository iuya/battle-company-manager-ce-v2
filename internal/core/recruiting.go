package core

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// RecruitService runs the recruit of reinforcements. The roll pays the reinforcement price; it can then be raised with influence
// (up only, at most MaxAdjust across both charts) and confirmed; a "special chart" result keeps the recruit going; a result that
// cannot join the company (a rare unit it has, a limit) leaves the player to choose a lower one; a "choice of options" unit
// waits for its options. What is in between two requests is CompanyState.Recruit, stored with the company, and every step is a
// companyRepository.Update, so a recruit cannot be started twice or paid twice.
type RecruitService struct{ companyDeps }

// NewRecruitService builds the service. rules is the loader (*Rules); dice rolls for the players who let the app roll.
func NewRecruitService(campaigns campaignRepository, companies companyRepository, rules ruleSource, dice Dice) *RecruitService {
	return &RecruitService{companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: dice}}
}

// recruitEnv is the company as a recruit sees it.
type recruitEnv struct {
	rs       *RuleSet
	d        CompanyDetail
	bcC      BCCompany
	settings CampaignSettings
}

func newRecruitEnv(rs *RuleSet, settings CampaignSettings, st CompanyState) recruitEnv {
	bcCo, _ := rs.BC.Company(st.Company.Faction)
	return recruitEnv{rs: rs, d: BuildDetail(rs, settings, st), bcC: bcCo, settings: settings}
}

// mutate runs a step of the recruit on the locked state of the user's company.
func (s *RecruitService) mutate(ctx context.Context, user User, campaignID int64, fn func(env recruitEnv, st *CompanyState) error) error {
	id, err := s.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	return s.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		return fn(newRecruitEnv(rs, settings, *st), st)
	})
}

// RecruitState is the recruit in progress, nil when there is none.
func (s *RecruitService) RecruitState(ctx context.Context, user User, campaignID int64) (*RecruitRoll, error) {
	id, err := s.myCompany(ctx, user, campaignID)
	if err != nil {
		return nil, err
	}
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	st, err := s.companies.Read(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	env := newRecruitEnv(rs, settings, st)
	r := st.Recruit
	switch {
	case r.Lower != nil:
		lower := &RecruitLower{Roll: r.Lower.Roll, Reason: r.Lower.Reason, Picks: r.Lower.Picks}
		for i, e := range r.Lower.Entries {
			blocked, _ := env.block(e.Res)
			lower.Rows = append(lower.Rows, LowerRow{Index: i, Chart: e.Chart, Roll: e.Roll, Text: describe(e.Res), Blocked: blocked})
		}
		return &RecruitRoll{Stage: "lower", Rolled: true, Roll: r.Lower.Roll, Lower: lower}, nil
	case r.Roll != nil:
		out := &RecruitRoll{Stage: r.Roll.Stage, Rolled: r.Roll.Rolled, Roll: r.Roll.Roll, PerkAdjust: r.Roll.PerkAdjust, PerkRaw: r.Roll.PerkRaw, Budget: max(0, min(MaxAdjust-r.Roll.Adjusted, 6-r.Roll.Roll))}
		if r.Roll.Stage == "special" && r.Roll.Rolled {
			out.Replace = env.missingMember()
		}
		if r.Roll.Stage == "chart" && r.Roll.Rolled {
			if res, ok := Roll(env.bcC.Reinforcements, r.Roll.Roll); ok && res.Type != "nothing" && res.Type != "specialChart" {
				out.Ranger = env.rangerOffer()
			}
		}
		return out, nil
	}
	return nil, nil
}

// RollRecruit makes the roll of a recruit (roll 0 is rolled by the app). The first roll pays the company's reinforcement price,
// whatever it gives; it can then be adjusted with influence and confirmed (ConfirmRecruit). A "special chart" result leads to a
// second roll, which is free.
func (s *RecruitService) RollRecruit(ctx context.Context, user User, campaignID int64, roll int, thrown bool) error {
	return s.RollRecruitWithPerk(ctx, user, campaignID, roll, thrown, 0)
}

// ErrInvalidRoll is a roll or a perk adjustment the rules do not allow (fit to show).
var ErrInvalidRoll = errors.New("invalid roll")

// Perk effects that move a die (campaign.aftermathEffects keys).
const (
	PerkRendezvous   = "rendezvous"
	PerkVantagePoint = "vantagePoint"
	whenFirstRecruit = "firstReinforcementRollThisAftermath"
	whenNextScenario = "nextScenarioTableRoll"
)

// UnusedPerk finds an unused perk of the effect among the perks a company holds.
func UnusedPerk(perks []Perk, effect string) (Perk, bool) {
	for _, p := range perks {
		if p.Effect == effect && p.UsedOn == nil {
			return p, true
		}
	}
	return Perk{}, false
}

// RollRecruitWithPerk is RollRecruit with the Rendezvous bonus: perkAdjust is -1, 0 or +1 (others are ErrInvalidRoll) and moves the die
// before the chart lookup. It needs an unused Rendezvous perk, which is used up in the same transaction as the roll (so it cannot be
// used twice); the roll line says "5, Rendezvous +1 = 6". The perk is for the first reinforcement roll of the aftermath, so only the
// roll that pays for a new recruit takes it (not the free roll on the special chart).
func (s *RecruitService) RollRecruitWithPerk(ctx context.Context, user User, campaignID int64, roll int, thrown bool, perkAdjust int) error {
	if perkAdjust < -1 || perkAdjust > 1 {
		return errors.Join(ErrInvalidRoll, errors.New("the Rendezvous bonus is +1 or -1"))
	}
	_, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	if msg := settings.RollRefusal(roll > 0, thrown); msg != "" {
		return Refuse("%s", msg)
	}
	return s.mutate(ctx, user, campaignID, func(env recruitEnv, st *CompanyState) error {
		if st.Recruit.Roll == nil { // a recruit in progress may be finished; a new one is locked
			if err := st.Locked(); err != nil {
				return err
			}
		}
		r := &st.Recruit
		if r.Choice != nil {
			return Refuse("Choose the options of the %s that just joined first.", r.Choice.Unit)
		}
		if r.Lower != nil {
			return Refuse("Choose a lower result for the recruit in progress first.")
		}
		if roll < 0 || roll > 6 {
			roll = 0
		}
		if roll == 0 {
			roll = s.dice.D6()
		}
		switch {
		case r.Roll == nil:
			cost := env.bcC.ReinforcementCost
			if ok, why := env.d.CanRecruit(cost); !ok {
				return Refuse("%s", why)
			}
			step := &RecruitStep{Stage: "chart", Rolled: true, Roll: roll, Paid: cost}
			if perkAdjust != 0 {
				perk, ok := UnusedPerk(st.Perks, PerkRendezvous)
				if !ok {
					return Refuse("The company has no Rendezvous to use.")
				}
				if n := roll + perkAdjust; n < 1 || n > 6 {
					return errors.Join(ErrInvalidRoll, fmt.Errorf("a %d cannot be moved to a %d", roll, n))
				}
				step.PerkRaw, step.PerkAdjust, step.Roll = roll, perkAdjust, roll+perkAdjust
				st.PerksUsed = append(st.PerksUsed, perk.ID)
				st.Perks = slices.DeleteFunc(slices.Clone(st.Perks), func(p Perk) bool { return p.ID == perk.ID })
			}
			st.Influence -= cost
			r.Roll = step
		case perkAdjust != 0:
			return Refuse("Rendezvous is for the first reinforcement roll.")
		case r.Roll.Stage == "special" && !r.Roll.Rolled:
			r.Roll.Rolled, r.Roll.Roll = true, roll
		default:
			return Refuse("This recruit has been rolled: confirm it first.")
		}
		return nil
	})
}

// ConfirmRecruit settles the recruit in progress. adjust raises the roll by that many points for 1 influence each (at most
// MaxAdjust across both charts; a roll is never lowered by paying). pick chooses between the options a result offers: "alt" for
// the company's second option, "opt:N" for one of the units of a choice, "member" for the missing member of a team instead of a
// special chart result. The result is a unit joining, a "choice of options" waiting for the player (Choice), or nothing; a
// "special chart" result keeps the recruit going (Next), one that cannot join leaves a choice of a lower one (Lower).
func (s *RecruitService) ConfirmRecruit(ctx context.Context, user User, campaignID int64, adjust int, pick string) (RecruitResult, error) {
	var out RecruitResult
	err := s.mutate(ctx, user, campaignID, func(env recruitEnv, st *CompanyState) error {
		rr := st.Recruit.Roll
		if rr == nil || !rr.Rolled {
			return Refuse("There is no roll to confirm.")
		}
		adjust = max(0, min(MaxAdjust-rr.Adjusted, 6-rr.Roll, adjust)) // the die is only ever raised, never past 6 (no influence is taken for points it cannot move); a lower result is chosen, never paid for
		if adjust > st.Influence {
			return Refuse("Adjusting costs %d influence and the company has %d.", adjust, st.Influence)
		}
		final := max(1, min(6, rr.Roll+adjust))
		chart := env.bcC.Reinforcements
		if rr.Stage == "special" {
			chart = env.bcC.SpecialChart
		}
		res, ok := Roll(chart, final)
		if !ok {
			return Refuse("%d is not a result on the chart", final)
		}
		st.Influence -= adjust
		rr.Paid += adjust
		prefix := fmt.Sprintf("You rolled a %d", rr.Roll)
		if rr.PerkAdjust != 0 {
			prefix = fmt.Sprintf("You rolled a %d, Rendezvous %+d = %d", rr.PerkRaw, rr.PerkAdjust, rr.Roll)
		}
		if final != rr.Roll {
			prefix += fmt.Sprintf(", raised to a %d", final)
		}
		if rr.Stage == "special" {
			prefix += " on the special chart"
		}
		prefix += ": "
		if pick == "ranger" {
			// the Shire swaps the whole rolled result for its Ranger of the North, before the company checks whether the result can join
			if rr.Stage != "chart" || res.Type == "nothing" || res.Type == "specialChart" {
				return Refuse("The Ranger of the North can only replace a successful result of the reinforcements chart.")
			}
			parked, err := env.parkRanger(st, prefix, rr.Paid, final)
			if err != nil {
				return err
			}
			st.Recruit.Roll = nil
			out = parked
			return nil
		}
		switch {
		case pick == "member" && rr.Stage == "special" && env.missingMember() != "":
			res = Reinforcement{Type: "unit", Unit: env.missingMember(), Count: 1}
			prefix += "you take the missing member of the team instead: "
		case pick == "alt" && res.Alt != "": // the company gives a second option on this roll
			res = Reinforcement{Type: "unit", Unit: res.Alt}
		}
		if res.Type == "unitChoice" { // the player picks one of the units the result offers; the first one the company can take when they do not
			res = env.pickOption(res, pick)
		}
		step := *rr
		st.Recruit.Roll = nil
		if why, kind := env.block(res); why != "" {
			st.Recruit.Lower = &LowerChoice{Prefix: prefix, Paid: step.Paid, Adjusted: step.Adjusted + adjust, Roll: final,
				Reason: fmt.Sprintf("%s, but %s.", describe(res), why), Cannot: fmt.Sprintf("the %s cannot join (%s)", describe(res), why), Stage: step.Stage,
				Entries: env.lowerEntries(step.Stage, final, res, kind)}
			out = RecruitResult{Roll: final, Summary: prefix + "a result that cannot join the company. Choose a lower one.", Lower: true}
			return nil
		}
		if res.Type == "choice" { // any result of the chart, chosen by the player
			entries := env.choiceEntries(step.Stage, final, res)
			if !slices.ContainsFunc(entries, func(e LowerEntry) bool { why, _ := env.block(e.Res); return why == "" }) {
				// nothing on the chart can join (every rare unit is there already): the recruit ends instead of waiting for a pick that cannot be made
				out = RecruitResult{Roll: final, Summary: fmt.Sprintf("%s%s, but no result on the chart can join the company (−%d influence).", prefix, strings.ToLower(res.Text), step.Paid)}
				return nil
			}
			st.Recruit.Lower = &LowerChoice{Prefix: prefix, Paid: step.Paid, Adjusted: step.Adjusted + adjust, Roll: final,
				Reason: res.Text + ": choose from the chart.", Cannot: strings.ToLower(res.Text), Stage: step.Stage, Entries: entries, Picks: picksOf(res.Text)}
			out = RecruitResult{Roll: final, Summary: prefix + strings.ToLower(res.Text) + ": choose your result.", Lower: true}
			return nil
		}
		out = env.settle(st, res, prefix, step.Paid, step.Adjusted+adjust, final, step.Stage)
		return nil
	})
	return out, err
}

var picksPattern = regexp.MustCompile(`(?i)\bany (\d)\b`)

// picksOf is how many results a "choice" result lets the player take ("Choose any 2 Hobbits from above").
func picksOf(text string) int {
	if m := picksPattern.FindStringSubmatch(text); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 1 {
			return n
		}
	}
	return 1
}

// ChooseLower settles a recruit whose result could not join (or was a "choice") with the result the player picked.
func (s *RecruitService) ChooseLower(ctx context.Context, user User, campaignID int64, index int) (RecruitResult, error) {
	var out RecruitResult
	err := s.mutate(ctx, user, campaignID, func(env recruitEnv, st *CompanyState) error {
		low := st.Recruit.Lower
		if low == nil {
			return Refuse("There is no recruit waiting for a lower result.")
		}
		if index < 0 || index >= len(low.Entries) {
			return ErrNotFound
		}
		e := low.Entries[index]
		if why, _ := env.block(e.Res); why != "" {
			return Refuse("That result cannot join the company either: %s.", why)
		}
		prefix := fmt.Sprintf("%s%s; you choose the lower result (%s): ", low.Prefix, low.Cannot, e.Roll)
		if low.Picks > 0 { // a result the player picks from the chart is not a lower one: "You rolled a 6: choice of any: a Hobbit Archer joins the company. Then a Hobbit Militia joins the company."
			prefix = low.Prefix
			if low.Cannot != "" {
				prefix += low.Cannot + ": "
			}
		}
		if low.Picks > 1 { // more results to take: this one joins now (the state is built again for the next pick)
			low.Picks--
			low.Cannot = ""
			paid := low.Paid
			low.Paid = 0
			out = env.settle(st, e.Res, prefix, paid, low.Adjusted, low.Roll, low.Stage)
			low.Prefix = out.Summary + " Then "
			out.Lower = true
			return nil
		}
		st.Recruit.Lower = nil
		out = env.settle(st, e.Res, prefix, low.Paid, low.Adjusted, low.Roll, low.Stage)
		return nil
	})
	return out, err
}

// RecruitSpecial buys one of the company's special units directly for its own influence cost.
func (s *RecruitService) RecruitSpecial(ctx context.Context, user User, campaignID int64, unit string) (RecruitResult, error) {
	var out RecruitResult
	err := s.mutate(ctx, user, campaignID, func(env recruitEnv, st *CompanyState) error {
		if err := st.Locked(); err != nil {
			return err
		}
		i := slices.IndexFunc(env.bcC.SpecialUnits, func(su SpecialUnit) bool { return strings.EqualFold(su.Unit, unit) })
		if i < 0 {
			return ErrNotFound
		}
		special := env.bcC.SpecialUnits[i]
		if why, _ := env.block(Reinforcement{Type: "unit", Unit: special.Unit, Rare: special.Rare}); why != "" {
			return Refuse("The %s cannot join: %s.", unit, why)
		}
		r := &st.Recruit
		if r.Choice != nil {
			return Refuse("Choose the options of the %s that just joined first.", r.Choice.Unit)
		}
		if r.Roll != nil || r.Lower != nil {
			return Refuse("Finish the recruit in progress first.")
		}
		if ok, why := env.d.CanRecruit(special.Cost); !ok {
			return Refuse("%s", why)
		}
		st.Influence -= special.Cost
		out = env.join(st, Reinforcement{Type: "unit", Unit: special.Unit, Choice: special.Choice}, "", special.Cost, 0)
		return nil
	})
	return out, err
}

// PendingChoice is the recruit waiting for its options, nil when there is none.
func (s *RecruitService) PendingChoice(ctx context.Context, user User, campaignID int64) (*RecruitChoice, error) {
	id, err := s.myCompany(ctx, user, campaignID)
	if err != nil {
		return nil, err
	}
	rs, _, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	st, err := s.companies.Read(ctx, campaignID, id)
	if err != nil || st.Recruit.Choice == nil {
		return nil, err
	}
	return choiceFor(rs, st.Recruit.Choice.Unit), nil
}

// ChooseOptions finishes a "choice of options" recruit: the unit joins with the chosen option (empty for its base profile). A
// result of several models asks again for the next one.
func (s *RecruitService) ChooseOptions(ctx context.Context, user User, campaignID int64, option string) (RecruitResult, error) {
	var out RecruitResult
	err := s.mutate(ctx, user, campaignID, func(env recruitEnv, st *CompanyState) error {
		pending := st.Recruit.Choice
		if pending == nil {
			return Refuse("There is no recruit waiting for its options.")
		}
		spec := env.newRecruit(pending.Unit, slices.Clone(pending.Wargear))
		text := "base profile"
		if option != "" {
			options := choiceFor(env.rs, pending.Unit).Options
			i := slices.IndexFunc(options, func(o ChoiceOption) bool { return o.Name == option })
			if i < 0 {
				return Refuse("%q is not an option of the %s.", option, pending.Unit)
			}
			spec.Gear = append(spec.Gear, strings.Split(options[i].Name, " and ")...) // the points follow from the option
			text = option
		}
		unit := pending.Unit
		if pending.Count > 1 {
			pending.Count--
		} else {
			st.Recruit.Choice = nil
		}
		st.Units = append(st.Units, spec)
		out = RecruitResult{Summary: fmt.Sprintf("A %s joins the company with %s.", unit, text)}
		if st.Recruit.Choice != nil {
			out.Choice = choiceFor(env.rs, unit)
		}
		return nil
	})
	return out, err
}

// settle applies a result that can be recruited: the special chart (the recruit goes on), nothing, or units joining.
func (env recruitEnv) settle(st *CompanyState, res Reinforcement, prefix string, paid, adjusted, roll int, stage string) RecruitResult {
	if res.Type == "specialChart" && stage == "chart" {
		st.Recruit.Roll = &RecruitStep{Stage: "special", Adjusted: adjusted, Paid: paid}
		return RecruitResult{Roll: roll, Summary: prefix + "the special chart. Roll again on it.", Next: true}
	}
	if res.Type != "unit" {
		return RecruitResult{Roll: roll, Summary: fmt.Sprintf("%snothing (−%d influence).", prefix, paid)}
	}
	return env.join(st, res, prefix, paid, roll)
}

// pickOption turns a "unitChoice" result into the unit the player picked ("opt:N"), or the first one that can join. When none
// can, the result stays a choice and block says so.
func (env recruitEnv) pickOption(res Reinforcement, pick string) Reinforcement {
	if n, ok := strings.CutPrefix(pick, "opt:"); ok {
		if i, err := strconv.Atoi(n); err == nil && i >= 0 && i < len(res.Options) {
			return optionResult(res.Options[i])
		}
	}
	for _, o := range res.Options {
		if why, _ := env.block(optionResult(o)); why == "" {
			return optionResult(o)
		}
	}
	return res
}

// block says why a result cannot join the company (empty when it can), with the kind of reason. It is resultBlock, and for a
// result of several models it also checks they all fit.
func (env recruitEnv) block(res Reinforcement) (why, kind string) {
	if why, kind := resultBlock(env.rs, env.bcC, env.d.Roster, res); why != "" {
		return why, kind
	}
	if res.Type == "unit" && res.Count > 1 {
		if profile, ok := env.rs.BC.Profile(env.bcC.Name); ok {
			if n := profile.Models(res.Unit) * res.Count; env.d.ModelCount()+n > env.bcC.MaxModels {
				return fmt.Sprintf("it counts as %d model(s) and the company has %d of its %d", n, env.d.ModelCount(), env.bcC.MaxModels), blockModels
			}
		}
	}
	return env.limitBlock(res)
}

// limitBlock says why a recruit would break a company limit (bows, throwing weapons, cavalry): the models that join are counted
// in the limit's numerator and in its denominator, and the limit rounds up. A limit the recruit does not add to cannot stop it.
func (env recruitEnv) limitBlock(res Reinforcement) (why, kind string) {
	if res.Type != "unit" || res.Choice || len(env.rs.BC.limitRules) == 0 {
		return "", ""
	}
	if profile, ok := env.rs.BC.Profile(env.bcC.Name); ok && len(profile.IncludedPoints[res.Unit]) > 0 {
		return "", ""
	}
	roster := slices.Clone(env.d.Roster)
	for range max(1, res.Count) {
		trial := buildUnit(env.rs, env.bcC, env.newRecruit(res.Unit, slices.Clone(res.Wargear)), env.settings)
		roster = append(roster, trial)
	}
	before := env.rs.BC.Limits(env.bcC, env.d.Roster)
	after := env.rs.BC.Limits(env.bcC, roster)
	for i, l := range after {
		if l.Used > before[i].Used && l.Used > l.Max {
			return fmt.Sprintf("%s limit: the company would have %d of %d with the new model", l.Label, l.Used, l.Max), blockLimit
		}
	}
	return "", ""
}

// missingMember is the member of a team (Vault Warden Team) the company lacks, when its file lets a special chart roll be
// swapped for it. Only the active roster counts, never the cemetery: a team exists while at least one of its members is on the
// roster, and the missing member is the one the roster has fewer of than the other. A team whose members are all gone (or that was
// never recruited) is not "missing a member": a normal roll recruits a whole new team. Empty when none is missing.
func (env recruitEnv) missingMember() string {
	profile, ok := env.rs.BC.Profile(env.bcC.Name)
	if !ok {
		return ""
	}
	for team, members := range profile.IncludedPoints {
		if _, ok := profile.Fallback(team, "memberMissing"); !ok {
			continue
		}
		alive := make([]int, len(members))
		most := 0
		for i, m := range members {
			for _, u := range env.d.Roster {
				if u.Type == m.Name {
					alive[i]++
				}
			}
			most = max(most, alive[i])
		}
		best, gap := "", 0
		for i, m := range members {
			if d := most - alive[i]; d > gap {
				best, gap = m.Name, d
			}
		}
		if best != "" {
			return best
		}
	}
	return ""
}

// lowerEntries are the results a player may take when the one they got cannot join: every lower result of the chart they are on
// (a result with two options gives two entries), or, when the company says so for a unit over the model maximum (Vault Warden
// Team), every result of the charts it names.
func (env recruitEnv) lowerEntries(stage string, final int, blocked Reinforcement, kind string) []LowerEntry {
	type source struct {
		chart []ChartRow
		name  string
		all   bool // every row, not only the lower ones
	}
	sources := []source{{env.bcC.Reinforcements, "Reinforcements", false}}
	if stage == "special" {
		sources = []source{{env.bcC.SpecialChart, "Special chart", false}}
	}
	if profile, ok := env.rs.BC.Profile(env.bcC.Name); ok && kind == blockModels {
		if f, ok := profile.Fallback(blocked.Unit, "wouldExceedMaxModels"); ok {
			sources = nil
			for _, c := range f.Charts {
				if c == "specialChart" {
					sources = append(sources, source{env.bcC.SpecialChart, "Special chart", true})
				} else {
					sources = append(sources, source{env.bcC.Reinforcements, "Reinforcements", true})
				}
			}
		}
	}
	var out []LowerEntry
	for _, src := range sources {
		for _, row := range src.chart {
			if !src.all && row.Max >= final {
				continue
			}
			for _, o := range rowResults(row) {
				if o.Type == "unit" && o.Unit == blocked.Unit && !(kind == blockModels && o.Count < blocked.Count) { // the same unit is no lower result, unless one of it fits where a pair did not
					continue
				}
				out = append(out, LowerEntry{Res: o, Chart: src.name, Roll: rollText(row)})
			}
		}
	}
	return out
}

// choiceEntries are the results a "choice" result lets the player take. "Choice of above" (and "any 2 Hobbits from above") offers the
// units of the rows above the roll on the chart being rolled. "Choice of any" offers the units of any row of any chart of the
// company, the normal and the special one: the player may take a lower roll as far as they want.
func (env recruitEnv) choiceEntries(stage string, final int, choice Reinforcement) []LowerEntry {
	type source struct {
		chart []ChartRow
		name  string
	}
	above := strings.Contains(strings.ToLower(choice.Text), "above")
	sources := []source{{env.bcC.Reinforcements, "Reinforcements"}, {env.bcC.SpecialChart, "Special chart"}}
	if above && stage == "special" {
		sources = sources[1:]
	} else if above {
		sources = sources[:1]
	}
	var out []LowerEntry
	for _, src := range sources {
		for _, row := range src.chart {
			if above && row.Max >= final {
				continue
			}
			for _, o := range rowResults(row) {
				if o.Type == "unit" {
					out = append(out, LowerEntry{Res: o, Chart: src.name, Roll: rollText(row)})
				}
			}
		}
	}
	return out
}

// rowResults are the results a row of a chart gives, one per unit the player can take.
func rowResults(row ChartRow) []Reinforcement {
	options := []Reinforcement{row.Result}
	if row.Result.Type == "unitChoice" { // each unit it offers is a result of its own
		options = nil
		for _, o := range row.Result.Options {
			options = append(options, optionResult(o))
		}
	}
	if row.Result.Alt != "" {
		options = append(options, Reinforcement{Type: "unit", Unit: row.Result.Alt})
	}
	return options
}

func rollText(row ChartRow) string {
	if row.Min != row.Max {
		return fmt.Sprintf("%d–%d", row.Min, row.Max)
	}
	return strconv.Itoa(row.Min)
}

func gearText(gear []string) string {
	if len(gear) == 0 {
		return ""
	}
	return " (" + strings.Join(gear, ", ") + ")"
}

func influenceText(cost int) string {
	if cost <= 0 {
		return ""
	}
	return fmt.Sprintf(" (−%d influence)", cost)
}

// newRecruit is a unit joining the company: a warrior, or a Hero in the Making when the company's rules make every recruit one.
func (env recruitEnv) newRecruit(base string, gear []string) UnitState {
	spec := UnitState{Base: base, Role: RoleWarrior, Gear: gear}
	if profile, ok := env.rs.BC.Profile(env.bcC.Name); ok && profile.RecruitAsHero {
		spec.Role = RoleHero
		profile.BecomeHero(&spec)
	}
	return spec
}

// join adds the recruit, or parks it until the player has chosen its options.
func (env recruitEnv) join(st *CompanyState, res Reinforcement, prefix string, cost, roll int) RecruitResult {
	count := max(1, res.Count)
	if res.Choice {
		st.Recruit.Choice = &OptionChoice{Unit: res.Unit, Wargear: slices.Clone(res.Wargear), Count: count}
		return RecruitResult{Roll: roll, Summary: fmt.Sprintf("%sa %s joins%s: choose its options.", prefix, res.Unit, influenceText(cost)), Choice: choiceFor(env.rs, res.Unit)}
	}
	if profile, ok := env.rs.BC.Profile(env.bcC.Name); ok && len(profile.IncludedPoints[res.Unit]) > 0 { // a unit that is several: each joins as a unit of its own
		var names []string
		for range count {
			for _, m := range profile.IncludedPoints[res.Unit] {
				st.Units = append(st.Units, UnitState{Base: m.Name, Role: RoleWarrior, Points: m.Points})
				names = append(names, m.Name)
			}
		}
		return RecruitResult{Roll: roll, Summary: fmt.Sprintf("%sa %s joins the company as %s%s.", prefix, res.Unit, strings.Join(names, " and "), influenceText(cost))}
	}
	for range count {
		st.Units = append(st.Units, env.newRecruit(res.Unit, slices.Clone(res.Wargear)))
	}
	what := "a " + res.Unit
	if count > 1 {
		what = fmt.Sprintf("%d × %s", count, res.Unit)
	}
	return RecruitResult{Roll: roll, Summary: fmt.Sprintf("%s%s%s joins the company%s.", prefix, what, gearText(res.Wargear), influenceText(cost))}
}

func choiceFor(rs *RuleSet, unit string) *RecruitChoice {
	choice := &RecruitChoice{Unit: unit}
	if v, ok := rs.Vanilla.Unit(unit); ok {
		choice.Options = rs.BC.ChoosableOptions(v.Name, v.Options)
	}
	return choice
}

// PendingOfficers is the officer panel of a Ranger of the North waiting to take command, nil when nothing waits.
func (s *RecruitService) PendingOfficers(ctx context.Context, user User, campaignID int64) (*OfficerPanel, error) {
	id, err := s.myCompany(ctx, user, campaignID)
	if err != nil {
		return nil, err
	}
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	st, err := s.companies.Read(ctx, campaignID, id)
	if err != nil || st.Recruit.Officers == nil {
		return nil, err
	}
	return newRecruitEnv(rs, settings, st).officerPanel(st.Recruit.Officers.Unit), nil
}

// ArrangeOfficers settles the Ranger of the North's arrival: the Ranger joins in the place he was put in, the named units take
// theirs, and an old officer left out becomes an ordinary Hero keeping everything else. ErrRefused (nothing is stored) when
// nothing waits or the arrangement is not valid.
func (s *RecruitService) ArrangeOfficers(ctx context.Context, user User, campaignID int64, a OfficerArrangement) (RecruitResult, error) {
	var out RecruitResult
	err := s.mutate(ctx, user, campaignID, func(env recruitEnv, st *CompanyState) error {
		if st.Recruit.Officers == nil {
			return Refuse("No Ranger of the North is waiting to take command.")
		}
		var err error
		out, err = env.arrange(st, *st.Recruit.Officers, a)
		return err
	})
	return out, err
}
