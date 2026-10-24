package core

import (
	"slices"
	"strings"
)

// BecomeHero makes a unit a hero the way a warrior that rolled a 6 on the level-up table does, except that the path is chosen
// later (the path window): the company's promotion for the unit (Ranger of Arnor to Ranger of the North, with the wargear it
// keeps and loses) and the point of Might, Will and Fate the company gives a new hero. The caller sets the role. Choosing a unit
// as an officer of the starting company, a warrior taking the place of a lost officer and a recruit that joins as a hero all go
// through here, so that none of them diverges from a Hero in the Making.
func (p CompanyProfile) BecomeHero(us *UnitState) {
	effective := us.Base
	if us.Progress != nil && us.Progress.Base != "" {
		effective = us.Progress.Base
	}
	promotion, promotes := p.PromotionOnHero(effective)
	if p.StartGains(effective) && !(promotes && !promotion.StatGain) { // a Ranger of the North already has its Might, Will and Fate
		us.Bonus = mergeBonus(us.Bonus, HeroBonus(nil))
	}
	if promotes {
		p.promote(us, promotion)
	}
}

// promote makes the profile change of a promotion: the new unit, minus the wargear the company says is lost.
func (p CompanyProfile) promote(us *UnitState, promotion HeroPromotion) {
	if us.Progress != nil && us.Progress.Base != "" { // promoted by a level-up before: the promotion is kept there, like a level-up does
		us.Progress.Base = promotion.Becomes
		us.Progress.Drop = append(slices.Clone(us.Progress.Drop), promotion.Drop...)
		return
	}
	us.Base = promotion.Becomes
	for _, lost := range promotion.Drop {
		if i := slices.IndexFunc(us.Gear, func(g string) bool { return strings.EqualFold(g, lost) }); i >= 0 {
			us.Gear = slices.Delete(slices.Clone(us.Gear), i, i+1)
		}
	}
}

// healOfficer promotes an officer made before officers were Heroes in the Making (a Ranger of Arnor that is still waiting for its
// path): the promotion is made and the point of Might, Will and Fate that a Ranger of the North already has is taken back.
func (p CompanyProfile) healOfficer(us *UnitState) {
	effective := us.Base
	if us.Progress != nil && us.Progress.Base != "" {
		effective = us.Progress.Base
	}
	promotion, promotes := p.PromotionOnHero(effective)
	if !promotes {
		return
	}
	if !promotion.StatGain && p.StartGains(effective) {
		for _, k := range []string{"might", "will", "fate"} {
			us.Bonus = mergeBonus(us.Bonus, map[string]int{k: -1})
		}
	}
	p.promote(us, promotion)
}
