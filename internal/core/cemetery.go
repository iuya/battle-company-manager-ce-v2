package core

import (
	"context"
	"slices"
)

// CemeteryService moves units that died or retired out of the roster into the cemetery, and keeps the order the player gave the
// fallen. A fallen unit never comes back: its id stays with it, so ids never cross between the two lists. The fallen count
// towards nothing: rare units, model maxima and the wargear limits look at the roster only.
type CemeteryService struct{ companyDeps }

// NewCemeteryService builds the service. rules is the loader (*Rules).
func NewCemeteryService(campaigns campaignRepository, companies companyRepository, rules ruleSource) *CemeteryService {
	return &CemeteryService{companyDeps{campaigns: campaigns, companies: companies, rules: rules, dice: RandomDice{}}}
}

// Bury moves a unit of the company from the roster to the cemetery as it is now (its profile, wargear, path, level and
// experience are frozen in the snapshot). how is what ended it ("Dead", "Retired"), where is the match it fell in. Its gear is
// lost with it, a creature it owned is gone from the company, and what it left in the Baggage Pony's pack stays there. This is
// a use case of the casualty flow, not of a player: the caller has decided who may bury (the match layer, or Injure).
func (s *CemeteryService) Bury(ctx context.Context, campaignID, companyID, unitID int64, how, where string) error {
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return err
	}
	return s.companies.Update(ctx, campaignID, companyID, func(st *CompanyState) error {
		return bury(rs, settings, st, unitID, how, where)
	})
}

func bury(rs *RuleSet, settings CampaignSettings, st *CompanyState, unitID int64, how, where string) error {
	i := slices.IndexFunc(st.Units, func(u UnitState) bool { return u.ID == unitID })
	if i < 0 {
		return ErrNotFound
	}
	d := BuildDetail(rs, settings, *st)
	j := slices.IndexFunc(d.Roster, func(u CompanyUnit) bool { return u.ID == unitID })
	if j < 0 {
		return ErrNotFound
	}
	snapshot := d.Roster[j]
	snapshot.Creature = nil // the creature is freed, it does not go to the grave with the unit
	st.Cemetery = append(st.Cemetery, FallenUnit{Unit: snapshot, Fate: how, Match: where})
	st.Units = slices.Delete(st.Units, i, i+1)
	st.Order = slices.DeleteFunc(st.Order, func(id int64) bool { return id == unitID })
	return nil
}

// Injure gives a hero of the company one more of a persistent injury (arm wound, leg wound, broken honour). A hero that reaches
// the count at which the injury forces it to retire (the second arm or leg wound) is buried at once with the fate "Retired";
// retired says whether it was. Rolling which injury a hero suffers is the casualty flow's job.
func (s *CemeteryService) Injure(ctx context.Context, campaignID, companyID, unitID int64, injury, where string) (retired bool, err error) {
	rs, settings, err := s.ruleset(ctx, campaignID)
	if err != nil {
		return false, err
	}
	def, ok := rs.BC.Injuries().Def(injury)
	if !ok || !def.Persistent {
		return false, Refuse("%q is not an injury a hero keeps.", injury)
	}
	err = s.companies.Update(ctx, campaignID, companyID, func(st *CompanyState) error {
		u, ok := stateUnit(st, unitID)
		if !ok {
			return ErrNotFound
		}
		if u.Role == RoleWarrior {
			return Refuse("Only heroes keep injuries.")
		}
		if u.Injuries == nil {
			u.Injuries = map[string]int{}
		}
		u.Injuries[injury]++
		if def.RetireAt > 0 && u.Injuries[injury] >= def.RetireAt {
			retired = true
			return bury(rs, settings, st, unitID, "Retired", where)
		}
		return nil
	})
	return retired && err == nil, err
}

// SaveCemeteryOrder stores the order the player put their fallen units in. A unit never moves between the cemetery and the
// roster: an id that is not in the cemetery is refused.
func (s *CemeteryService) SaveCemeteryOrder(ctx context.Context, user User, campaignID int64, order []int64) error {
	id, err := s.myCompanyToChange(ctx, user, campaignID)
	if err != nil {
		return err
	}
	return s.companies.Update(ctx, campaignID, id, func(st *CompanyState) error {
		known := map[int64]bool{}
		for _, f := range st.Cemetery {
			known[f.Unit.ID] = true
		}
		clean, err := cleanOrder(order, known, "in the cemetery")
		if err != nil {
			return err
		}
		st.Graveyard = clean
		return nil
	})
}
