package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The campaign section of a lean document read as the typed values the engine uses (docs/definitions.md, "Engine parameters"). The
// parameters the lean model types (the company shape, the recruiting rules, the break point, the scenario selection) are read from the
// typed fields; the tables whose parameter slice has not come yet (injuries and their treatments, Against the Odds, the hero rating
// per point, the break point exceptions) are read from the keys the file keeps in the model's Rest and Free values.

// LeanCompanyRules is the shape of a company, the hero rating threshold and the limits of a lean document.
func LeanCompanyRules(d *lean.Document) (CompanyRules, WargearThreshold, []LimitRule) {
	c := d.Campaign.Company
	rules := CompanyRules{MaxModels: c.MaxModels, Leaders: c.Leaders, Sergeants: c.Sergeants,
		HeroStart:      HeroStart{Might: c.HeroStart.Might, Will: c.HeroStart.Will, Fate: c.HeroStart.Fate},
		EquipmentSlots: SlotCounts{Large: c.EquipmentSlots.Large, Small: c.EquipmentSlots.Small}}
	t := d.Campaign.Rating.Hero.Wargear.Threshold
	return rules, WargearThreshold{Stats: t.Stats, SumAtLeast: t.SumAtLeast}, LeanLimitRules(d.Limits)
}

// LeanRecruitRules are the recruiting parameters of a lean document: how influence moves the die, the die, Rare, the perks that move a
// die and the starting influence.
func LeanRecruitRules(d *lean.Document) RecruitRules {
	c := d.Campaign
	r := RecruitRules{
		Adjust:            AdjustRules{Max: c.Recruitment.Adjust.MaxPerAttempt, CostPerPoint: c.Recruitment.Adjust.CostPerPoint, Direction: c.Recruitment.Adjust.Direction, Die: true},
		Sides:             c.Recruitment.Die.Sides,
		Rare:              RareRules{Injured: c.Recruitment.Rare.InjuredCount, Dead: c.Recruitment.Rare.DeadCount, Promoted: c.Recruitment.Rare.PromotedCount},
		StartingInfluence: c.Company.StartingInfluence,
	}
	for _, e := range c.AftermathEffects {
		if len(e.Range) != 2 {
			continue
		}
		switch e.Key {
		case PerkRendezvous:
			r.Rendezvous = PerkRange{e.Range[0], e.Range[1]}
		case PerkVantagePoint:
			r.VantagePoint = PerkRange{e.Range[0], e.Range[1]}
		}
	}
	return r
}

// LeanScenarioSelection is how the dice of a lean document pick a scenario.
func LeanScenarioSelection(d *lean.Document) ScenarioSelection {
	s := d.Campaign.ScenarioSelection
	sel := ScenarioSelection{Die: s.Die}
	for _, t := range s.Tables {
		sel.Tables = append(sel.Tables, ScenarioTableRange{t.Roll.Lo, t.Roll.Hi})
	}
	return sel
}

// LeanBreakRules are the break point rules of a lean document.
func LeanBreakRules(d *lean.Document) (BreakRules, error) {
	bp := d.Campaign.BreakPoint
	r := BreakRules{DefaultPercent: bp.Default.Percent, AtLeast: bp.Threshold.Comparison == "gte",
		CourageDice: strings.ToLower(bp.WhenBroken.CourageTest.Dice.String()), CourageSpec: bp.WhenBroken.CourageTest.Dice}
	if s, ok := bp.WhenBroken.CourageTest.Rest["passIf"].(string); ok {
		r.PassIf = s
	}
	if raw, ok := bp.Rest["scenarioExceptions"]; ok {
		var list []struct {
			Scenario string `json:"scenario"`
			Setting  string `json:"scenarioSetting"`
			Rule     string `json:"rule"`
			Effect   string `json:"effect"`
			Who      string `json:"who"`
			Text     string `json:"text"`
		}
		if err := leanRoundTrip(raw, &list); err != nil {
			return BreakRules{}, errors.Join(ErrInvalidRuleset, err)
		}
		for _, e := range list {
			r.Exceptions = append(r.Exceptions, BreakException{Scenario: e.Scenario, Setting: e.Setting, Rule: e.Rule, Effect: e.Effect, Who: e.Who, Prose: e.Text})
		}
	}
	return r, nil
}

// LeanRating are the rules that turn a hero's profile into a rating: the per-point table, the special rules and the average profile.
func LeanRating(d *lean.Document, wargear WargearThreshold) (RatingRules, error) {
	r := RatingRules{PerPoint: map[string]int{}, Wargear: wargear, Average: map[string]int{}}
	var hero struct {
		PerPoint []struct {
			Stat   string `json:"stat"`
			Rating int    `json:"rating"`
		} `json:"perPoint"`
		PerSpecialRule            int `json:"perSpecialRule"`
		PerMinorSpecialRule       int `json:"perMinorSpecialRule"`
		MaxMinorSpecialRuleRating int `json:"maxMinorSpecialRuleRating"`
	}
	if err := leanRoundTrip(d.Campaign.Rating.Hero.Rest, &hero); err != nil {
		return RatingRules{}, errors.Join(ErrInvalidRuleset, err)
	}
	r.PerSpecialRule, r.PerMinorSpecialRule, r.MaxMinorSpecialRule = hero.PerSpecialRule, hero.PerMinorSpecialRule, hero.MaxMinorSpecialRuleRating
	for _, p := range hero.PerPoint {
		r.PerPoint[p.Stat] = p.Rating
	}
	var limits struct {
		Average map[string]any `json:"average"`
	}
	if err := leanRoundTrip(d.Campaign.CharacteristicLimits, &limits); err != nil {
		return RatingRules{}, errors.Join(ErrInvalidRuleset, err)
	}
	for k, v := range limits.Average {
		if n, ok := v.(float64); ok {
			r.Average[k] = int(n)
		}
	}
	return r, nil
}

// LeanInjuries are the injury chart and the treatments of a lean document.
func LeanInjuries(d *lean.Document) (InjuryRules, error) {
	raw, err := lean.Encode(d.Campaign)
	if err != nil {
		return InjuryRules{}, err
	}
	return readInjuries(raw)
}

// LeanCampaignOptions are the settings the file offers the organiser of a campaign. An option the app does not know or has not
// implemented is listed, marked Unsupported, with a warning in the log.
func LeanCampaignOptions(d *lean.Document) ([]CampaignOption, error) {
	var list []struct {
		Key         string `json:"key"`
		Scope       string `json:"scope"`
		Label       string `json:"label"`
		Description string `json:"description"`
		Default     bool   `json:"default"`
		Implemented *bool  `json:"implemented"`
	}
	if err := leanRoundTrip(d.Campaign.Options, &list); err != nil {
		return nil, errors.Join(ErrInvalidRuleset, err)
	}
	var out []CampaignOption
	for _, o := range list {
		unsupported := !KnownOption(o.Key) || (o.Implemented != nil && !*o.Implemented)
		if unsupported {
			slog.Warn("campaign option the app does not support: it cannot be switched on", "option", o.Key)
		}
		out = append(out, CampaignOption{Key: o.Key, Scope: o.Scope, Label: o.Label, Description: o.Description, Default: o.Default, Unsupported: unsupported})
	}
	return out, nil
}

// jsonRoll reads the roll of a chart row as lean.Encode writes it: a number, "lo-hi" or [lo, hi].
type jsonRoll [2]int

func (r *jsonRoll) UnmarshalJSON(b []byte) error {
	var n int
	if json.Unmarshal(b, &n) == nil {
		*r = jsonRoll{n, n}
		return nil
	}
	var s string
	if json.Unmarshal(b, &s) == nil {
		lo, hi, ok := strings.Cut(s, "-")
		a, ea := strconv.Atoi(strings.TrimSpace(lo))
		z, ez := a, error(nil)
		if ok {
			z, ez = strconv.Atoi(strings.TrimSpace(hi))
		}
		if ea != nil || ez != nil {
			return fmt.Errorf("%q is not a roll", s)
		}
		*r = jsonRoll{a, z}
		return nil
	}
	var pair [2]int
	if err := json.Unmarshal(b, &pair); err != nil {
		return fmt.Errorf("%s is not a roll", b)
	}
	*r = pair
	return nil
}
