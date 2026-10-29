package core

import (
	"context"
	"fmt"
	"slices"
)

// TreatmentService pays influence to treat injuries (Battle Companies, Step 3; costs, targets and the injuries each way can
// remove come from campaign.injuries.treatment in the rules file). A warrior's Injured condition goes for the warrior cost. A
// hero either attempts it (the dice of the rules against a target, raised with extra influence; a failed attempt keeps its cost) or goes to the
// healers (no roll, it misses the next game); either removes one injury. A roll that missed by little waits for the player's
// decision on extra influence: UnitState.Pending.Treat, stored with the unit.
type TreatmentService struct{ companyDeps }

// NewTreatmentService builds the service. rules is the loader (*Rules); dice rolls for the players who let the app roll.
func NewTreatmentService(campaigns campaignRepository, companies companyRepository, rules ruleSource, dice Dice) *TreatmentService {
	return &TreatmentService{companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: dice}}
}

// TreatmentState is the treatment roll of a unit that waits for the player's decision on extra influence, nil when none.
func (s *TreatmentService) TreatmentState(ctx context.Context, user User, campaignID, companyID, unitID int64) (*TreatResult, error) {
	id, err := s.ownCompany(ctx, user, campaignID, companyID)
	if err != nil {
		return nil, err
	}
	st, err := s.companies.Read(ctx, campaignID, id)
	if err != nil {
		return nil, err
	}
	us, ok := stateUnit(&st, unitID)
	if !ok {
		return nil, ErrNotFound
	}
	if us.Pending.Treat == nil {
		return nil, nil
	}
	r := us.Pending.Treat.Result
	return &r, nil
}

// TreatInjury treats a unit of the user's company. The result says what happened; a failed attempt is not an error: its
// influence is lost. What the rules do not allow is an error wrapping ErrRefused.
func (s *TreatmentService) TreatInjury(ctx context.Context, user User, campaignID, companyID, unitID int64, t Treatment) (TreatResult, error) {
	id, err := s.ownCompanyToChange(ctx, user, campaignID, companyID)
	if err != nil {
		return TreatResult{}, err
	}
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return TreatResult{}, err
	}
	if t.Method == "attempt" && !t.Resolve {
		if msg := settings.RollRefusal(t.Roll > 0, t.Thrown); msg != "" {
			return TreatResult{}, Refuse("%s", msg)
		}
	}
	var res TreatResult
	err = s.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		if err := st.Locked(); err != nil {
			return err
		}
		env, err := newLevelEnv(rs, settings, *st, unitID)
		if err != nil {
			return err
		}
		us, _ := stateUnit(st, unitID)
		res, err = s.treat(env, st, us, t)
		return err
	})
	return res, err
}

func (s *TreatmentService) treat(env levelEnv, st *CompanyState, us *UnitState, t Treatment) (TreatResult, error) {
	var res TreatResult
	u, rules := env.unit, env.d.Injuries

	if u.Role == RoleWarrior {
		switch {
		case t.Method != "warrior":
			return res, Refuse("%s is a warrior: treating its injury needs no roll.", u.Title())
		case !u.Injured:
			return res, Refuse("%s is not injured.", u.Title())
		case st.Influence < rules.WarriorCost:
			return res, Refuse("Treating an injury costs %d influence, the company has %d.", rules.WarriorCost, st.Influence)
		}
		st.Influence -= rules.WarriorCost
		us.Injured, us.SentToHealers = false, false
		res.Healed, res.Summary = true, u.Title()+" is treated and can play again."
		return res, nil
	}

	m, ok := rules.Method(t.Method)
	if !ok {
		return res, Refuse("Choose how to treat %s.", u.Title())
	}
	options := rules.Healable(u, m)
	heal := func(injury string) TreatResult { // the treatment worked
		name := injury
		if k := slices.IndexFunc(options, func(o UnitInjury) bool { return o.Key == injury }); k >= 0 {
			name = options[k].Name
		}
		if injury == "justAScratch" { // the missed game is cancelled: the hero plays the next one
			us.Injured, us.SentToHealers = false, false
		} else if n := us.Injuries[injury]; n > 1 {
			us.Injuries[injury] = n - 1
		} else {
			delete(us.Injuries, injury)
		}
		if m.MissNextGame {
			us.SentToHealers = true
		}
		res.Healed = true
		res.Summary = fmt.Sprintf("%s: %s removed.", u.Title(), name)
		if m.MissNextGame {
			res.Summary += " It misses the next game."
		}
		return res
	}
	total := func(roll, extra int) int {
		v := roll + extra*m.PerPoint
		if m.Ceiling > 0 {
			v = min(m.Ceiling, v)
		}
		return v
	}

	if t.Resolve { // the roll waits for the player to decide whether to spend extra influence
		p := us.Pending.Treat
		if p == nil || !m.Rolls() {
			return res, Refuse("There is no treatment roll waiting.")
		}
		extra := max(0, t.Extra)
		if extra > st.Influence {
			return res, Refuse("That costs %d influence, the company has %d.", extra, st.Influence)
		}
		us.Pending.Treat = nil
		st.Influence -= extra
		res.Rolled, res.Roll, res.Needed, res.Total = true, p.Result.Roll, m.Target, total(p.Result.Roll, extra)
		if res.Total < m.Target {
			res.Summary = fmt.Sprintf("Rolled %d (%d needed): the treatment failed and the influence is lost.", p.Result.Roll, m.Target)
			return res, nil
		}
		return heal(p.Injury), nil
	}
	if us.Pending.Treat != nil {
		return res, Refuse("Finish the treatment roll in progress first.")
	}
	if k := slices.IndexFunc(options, func(o UnitInjury) bool { return o.Key == t.Injury }); k < 0 {
		return res, Refuse("%s has no injury that this treatment can remove.", u.Title())
	}
	if st.Influence < m.Cost {
		return res, Refuse("That costs %d influence, the company has %d.", m.Cost, st.Influence)
	}
	st.Influence -= m.Cost
	if !m.Rolls() {
		return heal(t.Injury), nil
	}
	res.Rolled, res.Roll = true, t.Roll
	if spec := m.TreatDice; res.Roll < spec.Min() || res.Roll > spec.Max() {
		res.Dice = RollDice(s.dice, spec)
		res.Roll = 0
		for _, d := range res.Dice {
			res.Roll += d
		}
	}
	res.Needed, res.Total = m.Target, total(res.Roll, 0)
	if res.Total >= m.Target {
		return heal(t.Injury), nil
	}
	// it failed: extra influence, spent after seeing the roll, may still make it
	for extra := 1; extra <= st.Influence; extra++ {
		if total(res.Roll, extra) >= m.Target {
			res.Pending, res.Boost = true, extra
			res.Summary = fmt.Sprintf("Rolled %d (%d needed). Spend %d more influence to make it %d?", res.Roll, m.Target, extra, total(res.Roll, extra))
			us.Pending.Treat = &PendingTreat{Injury: t.Injury, Result: res}
			return res, nil
		}
	}
	res.Summary = fmt.Sprintf("Rolled %d (%d needed): the treatment failed and the influence is lost.", res.Total, m.Target)
	return res, nil
}
