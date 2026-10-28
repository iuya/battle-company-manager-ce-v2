package core

import "testing"

// HeroBonus is what a hero of the shipped rules starts with (campaign.company.startingHeroStats: 1 Might, 1 Will, 1 Fate), plus extra.
func HeroBonus(extra map[string]int) map[string]int { return HeroStart{1, 1, 1}.Bonus(extra) }

// shippedLimits counts the limits of the shipped rules file for a company without exemptions.
func shippedLimits(t *testing.T, units []CompanyUnit) []CompanyLimit {
	t.Helper()
	return shippedRules(t).BC.Limits(BCCompany{}, units)
}
