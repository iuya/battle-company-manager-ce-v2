package core

import (
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
)

// The companies of a lean document read as the typed values the engine uses today (docs/definitions.md).
// They are pure functions of the document, the vanilla book and the glossary.
//
// The file stores each fact once: a rule's text lives in rules[] (a hero upgrade names the rule), a sentence lives in
// the typed object that explains it (`specialRules` keeps the rest), the rare counts live on the chart rows, the units a company
// alias builds live in units[] with `extends`. The wargear of the charts, the starting forces and the composite models is written
// in the armoury's spelling in the file, so nothing is spelled again here.

// LeanCompanySet is every company of a lean document: the page of each (limits, charts, special units), its profile, and the
// promotions each offers.
type LeanCompanySet struct {
	Companies    []BCCompany // file order
	Profiles     []CompanyProfile
	Advancements map[string][]Advancement // by company
}

// Company finds a company by its exact name.
func (s LeanCompanySet) Company(name string) (BCCompany, bool) {
	for _, c := range s.Companies {
		if c.Name == name {
			return c, true
		}
	}
	return BCCompany{}, false
}

// LeanCompanies reads the companies of a lean document. gear supplies each company's unique wargear (LeanGear).
func LeanCompanies(d *lean.Document, vanilla *VanillaBook, gear LeanGearTable) LeanCompanySet {
	set := LeanCompanySet{Advancements: map[string][]Advancement{}}
	text := LeanRuleText(d.Rules, vanilla)
	shape := d.Campaign.Company
	for _, c := range d.Companies {
		bc, p := leanCompany(d, c, text, gear)
		p.Leaders, p.Sergeants = shape.Leaders, shape.Sergeants
		p.HeroStart = HeroStart{Might: shape.HeroStart.Might, Will: shape.HeroStart.Will, Fate: shape.HeroStart.Fate}
		set.Companies = append(set.Companies, bc)
		if adv := p.Advancements; len(adv) > 0 {
			set.Advancements[c.Name] = adv
		}
		p.BCCompany = bc
		p.tidyAdvancements()
		set.Profiles = append(set.Profiles, p)
	}
	return set
}

// leanOwnUnits are a company's own units that build on another unit: the ones the engine treats as aliases.
func leanOwnUnits(c lean.Company) map[string]lean.Unit {
	own := map[string]lean.Unit{}
	for _, u := range c.Units {
		if u.Extends != "" && lean.NameKey(u.Extends) != lean.NameKey(u.Name) {
			own[lean.NameKey(u.Name)] = u
		}
	}
	return own
}

func leanChart(rows []lean.ChartRow, own map[string]lean.Unit) []ChartRow {
	var out []ChartRow
	for _, r := range rows {
		res := Reinforcement{Count: max(1, r.Count)}
		switch {
		case r.Nothing:
			res.Type, res.Blocks = "nothing", r.Blocks
		case r.Special:
			res.Type = "specialChart"
		case r.Choose != "":
			res.Type, res.Text, res.Scope, res.Picks = "choice", r.Choose, r.Scope, max(1, r.Picks)
		case len(r.Options) > 0:
			res.Type, res.Text = "unitChoice", r.Text
			for _, o := range r.Options {
				cu := ChoiceUnit{Unit: o.Unit, Rare: o.Rare}
				if u, ok := own[lean.NameKey(o.Unit)]; ok { // a unit of the company's own: the vanilla unit with the option selected
					cu.Unit, cu.Label, cu.Selected = u.Extends, u.Name, u.With
				}
				res.Options = append(res.Options, cu)
			}
		default:
			res.Type, res.Unit, res.Wargear, res.Choice, res.Rare = "unit", r.Unit, r.With, r.Pick, r.Rare
		}
		out = append(out, ChartRow{Min: r.Roll.Lo, Max: r.Roll.Hi, Result: res})
	}
	return out
}

func leanCompany(d *lean.Document, c lean.Company, text func(string) (string, bool), gear LeanGearTable) (BCCompany, CompanyProfile) {
	own := leanOwnUnits(c)
	bc := BCCompany{Name: c.Name, Label: c.Label, Alignment: c.Alignment, MaxModels: c.MaxModels, LimitOverrides: map[string]float64{}}
	if c.Reinforcements != nil {
		bc.ReinforcementCost = c.Reinforcements.Influence
		bc.Reinforcements = leanChart(c.Reinforcements.Rows, own)
	}
	if c.Special != nil {
		bc.SpecialChart = leanChart(c.Special.Rows, own)
		for _, b := range c.Special.Buy {
			bc.SpecialUnits = append(bc.SpecialUnits, SpecialUnit{Unit: b.Unit, Cost: b.Influence, Rare: b.Rare, Choice: b.Pick})
		}
	}
	for _, n := range c.Notes {
		if n.Section == "reinforcements" {
			bc.RecruitNotes = append(bc.RecruitNotes, n.Text)
		}
	}
	if l := c.Limits; l != nil {
		for _, x := range l.Exempt {
			bc.LimitExemptions = append(bc.LimitExemptions, LimitExemption{Unit: x.Unit, Item: x.Item, Limits: x.Limits, Text: x.Text})
		}
		for k, v := range l.Override {
			bc.LimitOverrides[k] = v
		}
	}
	bc.UniqueWargear = gear.Local[c.Name]

	p := CompanyProfile{BCCompany: bc, Description: strings.TrimSpace(c.Description), BreakPoint: c.BreakPoint, Rules: slicesClone(c.SpecialRules)}
	if c.Start != nil {
		p.StartGold = c.Start.Gold
		for _, u := range c.Start.Units {
			p.Start = append(p.Start, StartUnit{Count: max(1, u.Count), Unit: u.Unit, Wargear: u.With, Role: UnitRole(u.Role)})
		}
	}
	// the promotions: the unit a company alias builds is the vanilla unit under the company's label
	for _, a := range c.Advancements {
		adv := Advancement{FromUnit: a.From, FromWargear: a.With}
		for _, t := range a.To {
			target := AdvancementTarget{Unit: t.Unit, Wargear: t.With}
			if u, ok := own[lean.NameKey(t.Unit)]; ok {
				target.Unit, target.Label = u.Extends, u.Name
			}
			adv.To = append(adv.To, target)
		}
		p.Advancements = append(p.Advancements, adv)
	}
	for _, h := range c.HeroUpgrades {
		t, _ := text(h.Name)
		if h.Definition != "" {
			t = h.Definition
		}
		p.HeroUpgrades = append(p.HeroUpgrades, HeroUpgrade{Name: h.Name, Flavour: h.Flavour, Rule: h.Name, Text: strings.Join(strings.Fields(t), " "), Who: h.Who})
	}
	for _, g := range c.Granted {
		p.Granted = append(p.Granted, GrantedRule{Rule: g.Rule, Who: g.Who, Cost: g.Cost, Text: g.Text})
	}
	for _, e := range c.Exchanges {
		p.Exchanges = append(p.Exchanges, RuleExchange{Remove: e.Remove, Add: e.Add, Text: e.Text})
	}
	for _, s := range c.Substitutions {
		p.Substitutions = append(p.Substitutions, s.Text)
	}
	for _, u := range c.Units { // a company unit that builds on another is an alias
		if _, ok := own[lean.NameKey(u.Name)]; ok {
			p.Aliases = append(p.Aliases, UnitAlias{Label: u.Name, Unit: u.Extends, AddRules: u.AddRules, Selected: u.With, Keywords: u.AddKeywords, Text: u.Note})
		}
	}
	if l := c.Limits; l != nil {
		for _, k := range l.Company {
			p.KeywordLimits = append(p.KeywordLimits, KeywordLimit{Keyword: k.Keyword, Percent: k.Percent, Text: k.Text})
		}
	}
	for _, n := range c.Notes {
		p.Notes = append(p.Notes, CompanyNote{Section: n.Section, Text: n.Text})
	}
	if c.Oath != nil {
		p.Oath = Oath{Text: c.Oath.Text, Vow: c.Oath.Vow}
	}
	if c.Epigraph != nil {
		p.Epigraph = &Epigraph{Quote: c.Epigraph.Quote, Text: c.Epigraph.Text}
	}
	p.Oath = leanOath(d, c, p.Oath)
	if c.OwnPath != nil {
		p.OwnPathUnits, p.OwnPath = c.OwnPath.Units, c.OwnPath.Path
		p.OnTheirOwnPath = c.OwnPath.Units
	}
	for _, u := range c.Units { // the properties of a unit that only annotates a vanilla one, or builds on it
		if t := u.Types; t != nil {
			if p.Types == nil {
				p.Types = map[string]TypeOverride{}
			}
			p.Types[u.Name] = TypeOverride{Add: t.Add, Remove: t.Remove}
		}
		if len(u.Includes) > 0 {
			if p.IncludedPoints == nil {
				p.IncludedPoints = map[string][]ModelPoints{}
			}
			for _, m := range u.Includes {
				p.IncludedPoints[u.Name] = append(p.IncludedPoints[u.Name], ModelPoints{Name: m.Name, Points: m.Points})
			}
		}
		if u.Models > 0 {
			if p.ModelCounts == nil {
				p.ModelCounts = map[string]int{}
			}
			p.ModelCounts[u.Name] = u.Models
		}
		if cm := u.Composite; cm != nil {
			if p.Composites == nil {
				p.Composites = map[string]CompositeModel{}
			}
			m := CompositeModel{Rules: cm.Rules}
			if cm.Counts != nil {
				m.Slots = cm.Counts.Slots
			}
			if cm.Casualty != nil {
				m.InjuredAt, m.Table = cm.Casualty.InjuredWhen, cm.Casualty.Table
			}
			for _, part := range cm.Parts {
				name := part.Unit
				if part.Mount != "" {
					name = part.Mount
				}
				m.Parts = append(m.Parts, ModelPart{Role: part.Role, Name: name, Count: max(1, part.Count), Wargear: part.With})
			}
			p.Composites[u.Name] = m
		}
	}
	for _, pr := range c.Promotions {
		p.Promotions = append(p.Promotions, HeroPromotion{From: pr.From, OnResult: pr.On, Becomes: pr.Becomes, StatGain: pr.StatGain, ForgoAllowed: pr.Forgo})
	}
	p.ElvenWeaponry = c.ElvenWeaponry
	if h := c.Heroes; h != nil {
		p.StartHeroes = h.Who
		p.NoStartGain = h.NoStartGain
		p.RecruitAsHero = h.RecruitAsHero != nil
	}
	if r := c.Recruit; r != nil {
		for _, f := range r.Fallback {
			p.Fallbacks = append(p.Fallbacks, RecruitFallback{Unit: f.Unit, When: f.When, Charts: f.Charts})
		}
		for _, x := range r.Replace {
			switch {
			case x.Alternative != "": // two options on every roll of the range: the chart's unit or the alternative
				chart := p.BCCompany.Reinforcements
				if x.Chart != "reinforcements" || x.Roll == nil {
					continue
				}
				for i := range chart {
					if row := &chart[i]; row.Min >= x.Roll.Lo && row.Max <= x.Roll.Hi && row.Result.Type != "nothing" {
						row.Result.Alt = x.Alternative
					}
				}
			default:
				rr := &RangerReplacement{Unit: x.Unit, Max: max(1, x.Max)}
				for _, role := range x.Roles {
					rr.Roles = append(rr.Roles, UnitRole(role))
				}
				p.RangerReplacement = rr
			}
		}
		for _, b := range r.Balance {
			p.Balance = append(p.Balance, RaceBalance{Keyword: b.Keyword, NotMoreThan: b.NotMoreThan})
		}
	}
	return p.BCCompany, p
}

// leanOath is the company's oath, else the campaign's for its alignment, else a plain one, so the form still works.
func leanOath(d *lean.Document, c lean.Company, own Oath) Oath {
	if strings.Contains(own.Text, "{name}") {
		return own
	}
	var byAlignment map[string]struct {
		Text string `json:"text"`
		Vow  string `json:"vow"`
	}
	if d.Campaign.Company.Oaths != nil && leanRoundTrip(d.Campaign.Company.Oaths, &byAlignment) == nil {
		if f, ok := byAlignment[c.Alignment]; ok {
			own = Oath{Text: f.Text, Vow: f.Vow}
		}
	}
	if strings.Contains(own.Text, "{name}") {
		return own
	}
	return Oath{Text: "Today I take command of this Battle Company. Henceforth let it be called {name}.", Vow: "I solemnly vow"}
}

func slicesClone(s []string) []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s...)
}
