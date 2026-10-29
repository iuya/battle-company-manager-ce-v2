package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
)

// FreeEquipment is an armoury item a scenario reward lets a unit take for free (To the Victor the Spoils), with the units that can
// take and carry it. The cost is ignored; what it would cost is still the item's value.
type FreeEquipment struct {
	GearOffer
	Icon  string
	Units []UnitOption // only the units that can take it
}

// freeOffers lists the armoury items worth at most maxValue Influence that some unit of the company can take and carry: the
// armoury's legality (wargear options, slots, limits, restrictions) without the price.
func freeOffers(sh shop, maxValue int) []FreeEquipment {
	var out []FreeEquipment
	for _, offer := range sh.rs.BC.Armoury(sh.bcC) {
		if offer.Cost > maxValue {
			continue
		}
		item := FreeEquipment{GearOffer: offer, Icon: IconFor(offer.Name, offer.Classes, offer.Size)}
		for _, u := range Distinguish(sh.d.Roster) {
			reason, blocker := sh.whyNot(u, offer)
			if reason != "" && blocker != BlockedByInfluence {
				continue
			}
			item.Units = append(item.Units, UnitOption{ID: u.ID, Title: u.Title(), Type: u.Type, Role: u.Role, Kind: u.Kind})
		}
		if len(item.Units) > 0 {
			out = append(out, item)
		}
	}
	return out
}

// checkFreeEquipment is nil when the unit can take the item as a free one worth at most maxValue, else an error fit to show.
func checkFreeEquipment(rs *RuleSet, settings CampaignSettings, st CompanyState, maxValue int, item string, unitID int64) error {
	for _, o := range freeOffers(newShop(rs, settings, st), maxValue) {
		if !sameName(o.Name, item) {
			continue
		}
		for _, u := range o.Units {
			if u.ID == unitID {
				return nil
			}
		}
		return invalidAftermath("that unit cannot take " + o.Name)
	}
	return invalidAftermath("choose an item worth at most " + strconv.Itoa(maxValue) + " Influence that a unit can take")
}

// FreeEquipmentOffers lists the armoury items worth at most maxInfluenceValue that units of the company can take for free, with
// the units that can take and carry each.
func (s *AftermathService) FreeEquipmentOffers(ctx context.Context, id, companyID int64, maxInfluenceValue int) ([]FreeEquipment, error) {
	rs, settings, err := s.ruleset(ctx, id)
	if err != nil {
		return nil, err
	}
	st, err := s.companies.Read(ctx, id, companyID)
	if err != nil {
		return nil, err
	}
	return freeOffers(newShop(rs, settings, st), maxInfluenceValue), nil
}

// Perks lists the company's scenario perks, unused first.
func (s *AftermathService) Perks(ctx context.Context, id, companyID int64) ([]Perk, error) {
	return s.aftermaths.Perks(ctx, id, companyID)
}

// UsePerk marks a perk used. Only the owner of the company does it (applying it is manual; Rendezvous is not applied by the recruit window).
func (s *AftermathService) UsePerk(ctx context.Context, user User, id, companyID, perkID int64) error {
	if _, err := s.ownCompanyToChange(ctx, user, id, companyID); err != nil {
		return err
	}
	return s.aftermaths.UsePerk(ctx, id, companyID, perkID)
}

// UseVantagePoint applies the company's Vantage Point to the second die of the scenario selection of an open match the company plays
// in: adjust is within the range of the rules (+1 or -1 by default; others are ErrInvalidRoll), the second die must stay on its faces (campaign.scenarioSelection.die). The perk is used up (a second use is
// refused). It returns the die as moved and the line for the roll ("4, Vantage Point +1 = 5"). Only the owner of the company uses it.
func (s *AftermathService) UseVantagePoint(ctx context.Context, user User, id, matchID, companyID int64, second, adjust int) (int, string, error) {
	rs, _, err := s.ruleset(ctx, id)
	if err != nil {
		return 0, "", err
	}
	vantage := rs.BC.Recruiting().VantagePoint
	if adjust == 0 || !vantage.Allows(adjust) {
		return 0, "", errors.Join(ErrInvalidRoll, fmt.Errorf("the Vantage Point bonus is %s", vantage.Describe()))
	}
	if sides := rs.BC.Selection().Die.Sides; second < 1 || second > sides || second+adjust < 1 || second+adjust > sides {
		return 0, "", errors.Join(ErrInvalidRoll, fmt.Errorf("a %d cannot be moved by %+d", second, adjust))
	}
	if _, err := s.ownCompanyToChange(ctx, user, id, companyID); err != nil {
		return 0, "", err
	}
	m, err := s.matches.Get(ctx, id, matchID)
	if err != nil {
		return 0, "", err
	}
	if m.Ended {
		return 0, "", Refuse("The match has ended.")
	}
	if !slices.ContainsFunc(m.Sides, func(sd MatchSide) bool {
		return slices.ContainsFunc(sd.Companies, func(c CampaignCompany) bool { return c.ID == companyID })
	}) {
		return 0, "", Refuse("The company is not in this match.")
	}
	perks, err := s.aftermaths.Perks(ctx, id, companyID)
	if err != nil {
		return 0, "", err
	}
	perk, ok := UnusedPerk(perks, PerkVantagePoint)
	if !ok {
		return 0, "", Refuse("The company has no Vantage Point to use.")
	}
	if err := s.aftermaths.UsePerk(ctx, id, companyID, perk.ID); err != nil {
		return 0, "", err
	}
	moved := second + adjust
	return moved, fmt.Sprintf("%d, Vantage Point %+d = %d", second, adjust, moved), nil
}
