package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// A company profile is everything a player may want to know about a Battle Company before choosing it: its history, its starting
// company, its charts, its promotions, its exclusive items, rules and hero upgrades. It is read from the Battle Companies file.

// StartUnit is a unit of a starting company.
type StartUnit struct {
	Count   int
	Unit    string
	Wargear []string
	Role    UnitRole // the role the company suggests for it (leader, sergeant); empty for a plain warrior
}

// StartRestriction limits which starting units may be made heroes (Lake-town: only the Lake-town Guard; Cirith Ungol: no Spider).
type StartRestriction struct{ Allowed, Excluded []string }

// RaceBalance says a keyword may not outnumber another (Defenders of the North: Dwarves not more than Dale models).
type RaceBalance struct{ Keyword, NotMoreThan string }

// HeroUpgrade is the upgrade the heroes of a company can take instead of a level-up result.
type HeroUpgrade struct {
	Name        string
	Flavour     string
	Rule        string   // the special rule it gives
	Text        string   // what the rule does, when the file has it
	Restriction string   // only models of this kind can take it (Fiefdoms: Dol Amroth...); empty for any
	Units       []string // exact vanilla unit names that can take it (Feral Charge: Isengard Warg); when present they replace the word match
}

// Allows says whether a model can take the upgrade: it is open to everyone unless it names a race, a faction or a kind of
// unit (a Hobbit in the Wanderers in the Wild, the Spiders of Cirith Ungol, Dol Amroth in the Fiefdoms); plurals count.
func (h HeroUpgrade) Allows(u VanillaUnit) bool {
	if len(h.Units) > 0 { // by unit identity, not by the word of the restriction
		return slices.ContainsFunc(h.Units, func(n string) bool { return strings.EqualFold(n, u.Name) })
	}
	want := strings.ToLower(strings.TrimSpace(h.Restriction))
	if want == "" {
		return true
	}
	one := func(s string) string { return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), "s") }
	for _, k := range append(append(slices.Clone(u.Race), u.Faction...), u.UnitTypes...) {
		if one(k) == one(want) {
			return true
		}
	}
	return strings.Contains(" "+strings.ToLower(u.Name)+" ", " "+want+" ") || strings.Contains(" "+strings.ToLower(u.Name)+" ", " "+one(want)+" ")
}

// GrantedRule is a special rule a company gives some of its models for free.
type GrantedRule struct {
	Rule      string
	AppliesTo string
	Cost      int
	Text      string
}

// RuleExchange is a rule models of a company swap for another.
type RuleExchange struct{ Remove, Add, Text string }

// UnitAlias is a unit with a name of its own in a company: a vanilla profile with rules added.
type UnitAlias struct {
	Label    string
	Unit     string
	AddRules []string
	Selected []string // options of the vanilla unit it has selected (Tookish Hunter): their points are part of the unit
	Keywords []string // keywords the company adds to the vanilla unit (Goblin Hulk: Goblin); a race word goes to the race group
	Text     string
}

// TypeOverride changes the unit types of a unit in a company (the Warg Marauder is Cavalry, vanilla has it as Infantry).
type TypeOverride struct{ Add, Remove []string }

// ModelPart is a part of a unit made of several models on one card (the riders and the mount of a Warg Marauder).
type ModelPart struct {
	Role    string // shieldRider, bowRider, mount...
	Name    string
	Count   int
	Wargear []string
}

// CompositeModel is one model on the table that counts as several for the company (the Warg Marauder: three riders on a Warg).
type CompositeModel struct {
	Parts     []ModelPart
	Slots     int      // models it counts as against the company's maximum and its roster
	InjuredAt string   // when the whole model is injured: "allRidersRemoved"
	Table     string   // the injury table it rolls on
	Rules     []string // the special rules that explain how it plays
}

// RecruitFallback lets a recruit that would break a limit take another result (Vault Warden Team: over the model maximum, any
// result of both charts).
type RecruitFallback struct {
	Unit   string
	When   string
	Charts []string
}

// HeroPromotion makes a unit that becomes a hero change its profile at the same time (Ranger of Arnor to Ranger of the North).
type HeroPromotion struct {
	From, OnResult, Becomes string
	Keep, Drop              []string
	StatGain                bool
	ForgoAllowed            bool
}

// Epigraph is the quoted title and flavour sentence that head a company's rule text in the rulebook (flavour, not a rule).
type Epigraph struct{ Quote, Text string }

// Oath is what the player swears when they take command of a company: a text with the company's name typed in at {name}, and the
// label of the button that seals it.
type Oath struct {
	Text string // contains {name} once
	Vow  string
}

// Before and After are the text on either side of the place of the name.
func (o Oath) Before() string { b, _ := o.Split(); return b }
func (o Oath) After() string  { _, a := o.Split(); return a }

// Split cuts the text around the place of the name.
func (o Oath) Split() (before, after string) {
	before, after, _ = strings.Cut(o.Text, "{name}")
	return before, after
}

// KeywordLimit caps the share of a company that may have a keyword (Helm's Deep: Elves, 33%).
type KeywordLimit struct {
	Keyword string
	Percent float64
	Text    string
}

// CompanyNote is a note of a company page, with the section it belongs to.
type CompanyNote struct{ Section, Text string }

// CompanyProfile is a Battle Company with everything it has.
type CompanyProfile struct {
	BCCompany
	Description    string
	BreakPoint     int // the share of casualties that breaks the company, when it differs from the usual (0: the usual)
	Start          []StartUnit
	StartGold      int
	Advancements   []Advancement
	HeroUpgrades   []HeroUpgrade
	Rules          []string // the company's special rules, as sentences
	Granted        []GrantedRule
	Exchanges      []RuleExchange
	Substitutions  []string
	Aliases        []UnitAlias
	KeywordLimits  []KeywordLimit
	Oath           Oath
	Epigraph       *Epigraph         // nil for a company without one
	Paths          []PathProgression // the paths a starting hero can follow
	Powers         []LevelChoice     // the magical powers a starting Sorcerer can begin with (Key "power:Name")
	OwnPathUnits   []string          // units On Their Own Path: they earn no experience and follow the path below
	OwnPath        string            // the path they are given when recruited (empty: none)
	Types          map[string]TypeOverride
	Composites     map[string]CompositeModel
	IncludedPoints map[string][]ModelPoints // the points of the units a unit includes, by the unit
	ModelCounts    map[string]int           // how many models a recruit counts as against the maximum, by unit
	Fallbacks      []RecruitFallback
	Promotions     []HeroPromotion
	StartHeroes    StartRestriction
	// RangerReplacement is the Shire's rule (nil without it): a successful reinforcement result may be swapped for the unit once the
	// company's one is slain.
	RangerReplacement *RangerReplacement
	NoStartGain       []string // starting units that keep their own Might, Will and Fate when made heroes
	RecruitAsHero     bool     // every recruit joins as a Hero in the Making with a point of Might, Will and Fate (Wanderers in the Wild)
	Balance           []RaceBalance
	OnTheirOwnPath    []string
	Notes             []CompanyNote
	Previews          map[string]UnitPreview // every unit the page mentions, by PreviewKey
}

// ModelPoints is the points a member of a unit made of several units is worth.
type ModelPoints struct {
	Name   string
	Points int
}

// UnitPreview is a unit as a small card: what a tooltip shows of a unit named in a company page.
type UnitPreview struct {
	ID            string // an id for the page, "pv1", "pv2"...
	Key           string
	Name          string
	Points        int
	Kind          UnitKind
	Types         []string
	Keywords      UnitKeywords // race, faction and unit types of the profile (filled where the rules set is known: see RuleSet.fillPreviewKeywords)
	Stats         []Attribute
	Wargear       []string
	Rules         []string
	HeroicActions []string
}

// PreviewKey identifies a unit with the wargear it is given in a chart or a promotion.
func PreviewKey(unit string, wargear []string) string {
	w := make([]string, len(wargear))
	for i, x := range wargear {
		w[i] = strings.ToLower(strings.TrimSpace(x))
	}
	slices.Sort(w)
	return unit + "|" + strings.Join(w, ",")
}

// profileSet is the companies' profiles as the pages use them (paths and powers attached), built once per book: the book is immutable
// after loading and every unit of every page asks for its company's profile.
type profileSet struct {
	list   []CompanyProfile
	exact  map[string]int // by name, the first of the file
	folded map[string]int // by lower-cased name, the first of the file
}

func (b *BCBook) profileSet() *profileSet {
	b.profileOnce.Do(func() {
		ps := &profileSet{list: slices.Clone(b.profiles), exact: map[string]int{}, folded: map[string]int{}}
		powers := b.progression.PowerChoices(HeroSheet{Powers: map[string]int{}})
		for i := range ps.list {
			ps.list[i].Paths = b.progression.Paths
			ps.list[i].Powers = powers
			if _, ok := ps.exact[ps.list[i].Name]; !ok {
				ps.exact[ps.list[i].Name] = i
			}
			k := strings.ToLower(ps.list[i].Name)
			if _, ok := ps.folded[k]; !ok {
				ps.folded[k] = i
			}
		}
		b.profileSets = ps
	})
	if b.profileSets == nil { // the build panicked (sync.Once does not run it again): answer an empty set, not a nil pointer
		return &profileSet{exact: map[string]int{}, folded: map[string]int{}}
	}
	return b.profileSets
}

// Profile finds a company's profile by name.
func (b *BCBook) Profile(name string) (CompanyProfile, bool) {
	ps := b.profileSet()
	if i, ok := ps.exact[name]; ok {
		return ps.list[i], true
	}
	// an older spelling of the name ("Lake-Town" before "Lake-town") still resolves
	if i, ok := ps.folded[strings.ToLower(name)]; ok {
		return ps.list[i], true
	}
	return CompanyProfile{}, false
}

// Profiles lists every company's profile in the order of the file.
func (b *BCBook) Profiles() []CompanyProfile {
	return slices.Clone(b.profileSet().list)
}

// parseProfiles reads the descriptive parts of the companies and attaches the unit previews (made with the vanilla book).
func (b *BCBook) parseProfiles(content []byte) {
	var doc struct {
		Campaign struct {
			Company struct {
				Oath map[string]struct { // by alignment: what a company of a custom file with no oath of its own swears
					Text string `json:"text"`
					Vow  string `json:"vow"`
				} `json:"oath"`
			} `json:"company"`
		} `json:"campaign"`
		Data struct {
			Companies []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				BreakPoint  int    `json:"breakPointPercentage"`
				Start       struct {
					Units []struct {
						Count   int      `json:"count"`
						Unit    string   `json:"unit"`
						Label   string   `json:"label"`
						Wargear []string `json:"wargear"`
						Role    string   `json:"role"`
					} `json:"units"`
					Gold int `json:"gold"`
				} `json:"startingCompany"`
				HeroUpgrades []struct {
					Name    string   `json:"name"`
					Flavour string   `json:"flavour"`
					Rule    string   `json:"specialRule"`
					Text    string   `json:"text"`
					Limit   string   `json:"restriction"`
					Units   []string `json:"restrictedToUnits"`
				} `json:"heroUpgrades"`
				Epigraph *struct {
					Quote string `json:"quote"`
					Text  string `json:"text"`
				} `json:"epigraph"`
				SpecialRules []string `json:"specialRules"`
				GrantedRules []struct {
					Rule      string `json:"specialRule"`
					AppliesTo string `json:"appliesTo"`
					Cost      int    `json:"cost"`
					Text      string `json:"text"`
				} `json:"grantedSpecialRules"`
				RuleExchanges []struct {
					Remove string `json:"remove"`
					Add    string `json:"add"`
					Text   string `json:"text"`
				} `json:"ruleExchanges"`
				RuleSubstitutions []struct {
					Text string `json:"text"`
				} `json:"ruleSubstitutions"`
				UnitAliases []struct {
					Label    string   `json:"label"`
					Unit     string   `json:"unit"`
					AddRules []string `json:"addSpecialRules"`
					Selected []string `json:"selectedOptions"`
					Keywords []string `json:"addKeywords"`
					Text     string   `json:"text"`
				} `json:"unitAliases"`
				CompanyLimits []struct {
					Keyword string  `json:"keyword"`
					Percent float64 `json:"maximumPercentage"`
					Text    string  `json:"text"`
				} `json:"companyLimits"`
				OnTheirOwnPath []string `json:"onTheirOwnPath"`
				Oath           struct {
					Text string `json:"text"`
					Vow  string `json:"vow"`
				} `json:"oath"`
				Effects []struct {
					Kind        string          `json:"kind"`
					Allowed     []string        `json:"allowed"`
					Excluded    []string        `json:"excluded"`
					Unit        string          `json:"unit"`
					Keyword     string          `json:"keyword"`
					NotMoreThan string          `json:"notMoreThanKeyword"`
					Chart       string          `json:"chart"`
					Add         json.RawMessage `json:"add"` // a list for unitTypes, a string for ruleExchange
					Remove      json.RawMessage `json:"remove"`
					Models      int             `json:"models"`
					When        string          `json:"when"`
					Charts      []string        `json:"charts"`
					Units       []string        `json:"units"`
					Path        string          `json:"path"`
					From        string          `json:"from"`
					OnResult    string          `json:"onResult"`
					Becomes     string          `json:"becomes"`
					Keep        []string        `json:"keep"`
					Drop        []string        `json:"drop"`
					StatGain    *bool           `json:"heroStatGain"`
					Forgo       *bool           `json:"forgoAllowed"`
					Members     []struct {
						Name   string `json:"name"`
						Points int    `json:"points"`
					} `json:"members"`
					Parts []struct {
						Role    string   `json:"role"`
						Unit    string   `json:"unit"`
						Mount   string   `json:"mount"`
						Count   int      `json:"count"`
						Wargear []string `json:"wargear"`
					} `json:"parts"`
					Counts struct {
						Slots int `json:"rosterSlots"`
					} `json:"counts"`
					Casualty struct {
						InjuredWhen string `json:"injuredWhen"`
						Table       string `json:"table"`
					} `json:"casualty"`
					Rules       []string `json:"rules"`
					Results     []int    `json:"results"`
					Max         int      `json:"max"`
					Roles       []string `json:"role"` // rangerReplacement: the places the replacement may take
					Alternative struct {
						Unit string `json:"unit"`
					} `json:"alternative"`
				} `json:"effects"`
				Notes []struct {
					Section string `json:"section"`
					Text    string `json:"text"`
				} `json:"notes"`
			} `json:"companies"`
		} `json:"data"`
	}
	if json.Unmarshal(content, &doc) != nil {
		return
	}
	for _, c := range doc.Data.Companies {
		base, ok := b.companies[c.Name]
		if !ok {
			continue
		}
		p := CompanyProfile{BCCompany: base, Description: strings.TrimSpace(c.Description), BreakPoint: c.BreakPoint, StartGold: c.Start.Gold,
			Rules: c.SpecialRules, OnTheirOwnPath: c.OnTheirOwnPath, Advancements: b.advancements[c.Name]}
		ownNames := map[string]bool{} // the company's own names that change a unit (an option selected, a rule added), not just call it differently
		for _, a := range c.UnitAliases {
			if len(a.Selected) > 0 || len(a.AddRules) > 0 {
				ownNames[strings.ToLower(a.Label)] = true
			}
		}
		for _, u := range c.Start.Units {
			unit := u.Unit
			if ownNames[strings.ToLower(u.Label)] { // Wanderers in the Wild start with a Tookish Hunter, not a plain Hobbit Archer
				unit = u.Label
			}
			p.Start = append(p.Start, StartUnit{Count: max(1, u.Count), Unit: unit, Wargear: u.Wargear, Role: UnitRole(u.Role)})
		}
		for _, e := range c.Effects {
			switch e.Kind {
			case "startingHeroRestriction":
				p.StartHeroes = StartRestriction{Allowed: e.Allowed, Excluded: e.Excluded}
			case "recruitAsHero":
				p.RecruitAsHero = true
			case "rangerReplacement":
				rr := &RangerReplacement{Unit: e.Unit, Max: max(1, e.Max)}
				for _, r := range e.Roles {
					rr.Roles = append(rr.Roles, UnitRole(r))
				}
				p.RangerReplacement = rr
			case "startingHeroNoStatGain":
				p.NoStartGain = append(p.NoStartGain, e.Unit)
			case "onTheirOwnPath":
				p.OwnPathUnits = append(p.OwnPathUnits, e.Units...)
				p.OwnPath = e.Path
			case "unitTypes":
				if p.Types == nil {
					p.Types = map[string]TypeOverride{}
				}
				var o TypeOverride
				_ = json.Unmarshal(e.Add, &o.Add)
				_ = json.Unmarshal(e.Remove, &o.Remove)
				p.Types[e.Unit] = o
			case "includedUnitPoints":
				if p.IncludedPoints == nil {
					p.IncludedPoints = map[string][]ModelPoints{}
				}
				for _, m := range e.Members {
					p.IncludedPoints[e.Unit] = append(p.IncludedPoints[e.Unit], ModelPoints{Name: m.Name, Points: m.Points})
				}
			case "modelCount":
				if p.ModelCounts == nil {
					p.ModelCounts = map[string]int{}
				}
				p.ModelCounts[e.Unit] = e.Models
			case "recruitFallback":
				p.Fallbacks = append(p.Fallbacks, RecruitFallback{Unit: e.Unit, When: e.When, Charts: e.Charts})
			case "compositeModel":
				if p.Composites == nil {
					p.Composites = map[string]CompositeModel{}
				}
				m := CompositeModel{Slots: e.Counts.Slots, InjuredAt: e.Casualty.InjuredWhen, Table: e.Casualty.Table, Rules: e.Rules}
				for _, part := range e.Parts {
					name := part.Unit
					if part.Mount != "" {
						name = part.Mount
					}
					m.Parts = append(m.Parts, ModelPart{Role: part.Role, Name: name, Count: max(1, part.Count), Wargear: part.Wargear})
				}
				p.Composites[e.Unit] = m
			case "heroPromotion":
				p.Promotions = append(p.Promotions, HeroPromotion{From: e.From, OnResult: e.OnResult, Becomes: e.Becomes, Keep: e.Keep, Drop: e.Drop,
					StatGain: e.StatGain != nil && *e.StatGain, ForgoAllowed: e.Forgo != nil && *e.Forgo})
			case "reinforcementReplacement": // two options on every roll of the range: the chart's unit or the alternative
				if e.Chart == "reinforcements" && len(e.Results) == 2 {
					for i := range base.Reinforcements {
						if row := &base.Reinforcements[i]; row.Min >= e.Results[0] && row.Max <= e.Results[1] && row.Result.Type != "nothing" {
							row.Result.Alt = e.Alternative.Unit
						}
					}
				}
			case "raceBalance":
				p.Balance = append(p.Balance, RaceBalance{Keyword: e.Keyword, NotMoreThan: e.NotMoreThan})
			}
		}
		for _, h := range c.HeroUpgrades {
			text := h.Text
			if text == "" { // the rule's text is the keyword of the same name
				text = strings.Join(strings.Fields(b.keywords[h.Rule]), " ")
			}
			p.HeroUpgrades = append(p.HeroUpgrades, HeroUpgrade{Name: h.Name, Flavour: h.Flavour, Rule: h.Rule, Text: text, Restriction: h.Limit, Units: h.Units})
		}
		for _, g := range c.GrantedRules {
			p.Granted = append(p.Granted, GrantedRule{Rule: g.Rule, AppliesTo: g.AppliesTo, Cost: g.Cost, Text: g.Text})
		}
		for _, e := range c.RuleExchanges {
			p.Exchanges = append(p.Exchanges, RuleExchange{Remove: e.Remove, Add: e.Add, Text: e.Text})
		}
		for _, s := range c.RuleSubstitutions {
			p.Substitutions = append(p.Substitutions, s.Text)
		}
		for _, a := range c.UnitAliases {
			p.Aliases = append(p.Aliases, UnitAlias{Label: a.Label, Unit: a.Unit, AddRules: a.AddRules, Selected: a.Selected, Keywords: a.Keywords, Text: a.Text})
		}
		p.Advancements = slices.Clone(p.Advancements)
		for i, adv := range p.Advancements { // a promotion is named by the company's label only where the label changes the unit
			adv.To = slices.Clone(adv.To)
			for j, t := range adv.To {
				if t.Label != "" && !p.ChangesUnit(t.Label) {
					adv.To[j].Label = ""
				}
			}
			p.Advancements[i] = adv
		}
		for _, l := range c.CompanyLimits {
			p.KeywordLimits = append(p.KeywordLimits, KeywordLimit{Keyword: l.Keyword, Percent: l.Percent, Text: l.Text})
		}
		for _, n := range c.Notes {
			p.Notes = append(p.Notes, CompanyNote{Section: n.Section, Text: n.Text})
		}
		p.Oath = Oath{Text: c.Oath.Text, Vow: c.Oath.Vow}
		if c.Epigraph != nil {
			p.Epigraph = &Epigraph{Quote: c.Epigraph.Quote, Text: c.Epigraph.Text}
		}
		if f, ok := doc.Campaign.Company.Oath[base.Alignment]; ok && !strings.Contains(p.Oath.Text, "{name}") {
			p.Oath = Oath{Text: f.Text, Vow: f.Vow}
		}
		if !strings.Contains(p.Oath.Text, "{name}") { // the file has nothing: a plain oath, so the form still works
			p.Oath = Oath{Text: "Today I take command of this Battle Company. Henceforth let it be called {name}.", Vow: "I solemnly vow"}
		}
		b.profiles = append(b.profiles, p)
	}
}

// WithPreviews fills in the unit previews of a profile: every unit its starting company, charts, special units and promotions
// name, drawn from the vanilla profiles (a company's own label for a unit is resolved to the unit it is).
func (p CompanyProfile) WithPreviews(vanilla *VanillaBook) CompanyProfile {
	p.Previews = map[string]UnitPreview{}
	add := func(unit string, wargear []string) {
		if unit == "" {
			return
		}
		key := PreviewKey(unit, wargear)
		if _, done := p.Previews[key]; !done {
			pv := p.preview(vanilla, unit, wargear)
			pv.ID = fmt.Sprintf("pv%d", len(p.Previews)+1)
			p.Previews[key] = pv
		}
	}
	for _, s := range p.Start {
		add(s.Unit, s.Wargear)
	}
	for _, chart := range [][]ChartRow{p.Reinforcements, p.SpecialChart} {
		for _, r := range chart {
			add(r.Result.Unit, r.Result.Wargear)
		}
	}
	for _, s := range p.SpecialUnits {
		add(s.Unit, nil)
	}
	for _, a := range p.Advancements {
		add(a.FromUnit, a.FromWargear)
		for _, t := range a.To {
			add(t.Name(), t.Wargear)
		}
	}
	return p
}

func (p CompanyProfile) preview(vanilla *VanillaBook, unit string, wargear []string) UnitPreview {
	pv := UnitPreview{Key: PreviewKey(unit, wargear), Name: unit}
	name, extraRules := unit, []string(nil)
	for _, a := range p.Aliases { // a company's own name for a profile
		if strings.EqualFold(a.Label, unit) {
			name, extraRules = a.Unit, a.AddRules
		}
	}
	vu, ok := vanilla.Unit(name)
	if !ok {
		return pv
	}
	hero := vu.Stats["might"] > 0 || vu.Stats["will"] > 0 || vu.Stats["fate"] > 0
	pv.Points, pv.Types, pv.Kind = vu.Points, vu.UnitTypes, KindOf(vu.UnitTypes)
	pv.Stats = vanilla.Profile(vu, hero, nil)
	pv.Wargear = slices.Clone(vu.Wargear)
	for _, w := range wargear {
		if w = strings.TrimSpace(w); w != "" {
			pv.Wargear = append(pv.Wargear, strings.ToUpper(w[:1])+w[1:])
		}
	}
	pv.Rules = append(slices.Clone(vu.SpecialRules), extraRules...)
	pv.HeroicActions = vu.HeroicActions
	return pv
}

// ChangesUnit says whether the company's own name for a unit is more than another name for it: it selects an option or adds a
// rule (Ranger of Ithilien, Helminga), where a legacy name (Grimhammer for the Grim Hammer Warrior) is the same unit.
func (p CompanyProfile) ChangesUnit(label string) bool {
	return slices.ContainsFunc(p.Aliases, func(a UnitAlias) bool {
		return strings.EqualFold(a.Label, label) && (len(a.Selected) > 0 || len(a.AddRules) > 0)
	})
}

// Preview is the preview of a unit as the company page names it (the zero value when the unit is unknown).
func (p CompanyProfile) Preview(unit string, wargear []string) UnitPreview {
	return p.Previews[PreviewKey(unit, wargear)]
}

// HasHeroStats says whether the preview shows Might, Will and Fate.
func (u UnitPreview) HasHeroStats() bool {
	for _, a := range u.Stats {
		if a.HeroOnly {
			return true
		}
	}
	return false
}

// NotesIn are the notes of a section of the company page ("startingCompany", "specialChart", "advancements", "eliteCompany").
func (p CompanyProfile) NotesIn(section string) []string {
	var out []string
	for _, n := range p.Notes {
		if n.Section == section && !slices.Contains(out, n.Text) {
			out = append(out, n.Text)
		}
	}
	return out
}

// OwnRules are the company's special rules the page prints as "Company rule" cards: a rule whose sentence a typed card already
// shows (a substitution, an exchange, a free special rule, a limit line) is left out, so no sentence appears twice.
func (p CompanyProfile) OwnRules() []string {
	key := func(t string) string { return strings.ToLower(strings.Join(strings.Fields(t), " ")) }
	var typed []string
	for _, g := range p.Granted {
		typed = append(typed, key(g.Text))
	}
	for _, e := range p.Exchanges {
		typed = append(typed, key(e.Text))
	}
	for _, s := range p.Substitutions {
		typed = append(typed, key(s))
	}
	for _, l := range p.LimitLines() {
		typed = append(typed, key(l))
	}
	var out []string
	for _, r := range p.Rules {
		k := key(r)
		if k == "" || slices.Contains(typed, k) || slices.ContainsFunc(out, func(o string) bool { return key(o) == k }) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// LimitLines are the sentences about what limits the company: keyword limits and exemptions, each once.
func (p CompanyProfile) LimitLines() []string {
	var out []string
	add := func(t string) {
		if t = strings.TrimSpace(t); t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	for _, l := range p.KeywordLimits {
		add(l.Text)
	}
	for _, e := range p.LimitExemptions {
		add(e.Text)
	}
	return out
}

// OnOwnPath says whether a unit is On Its Own Path in this company: it earns no experience and never levels up. The file names
// the units exactly as the vanilla file does (or by the label the company gives them).
func (p CompanyProfile) OnOwnPath(unit string) bool {
	named := func(unit string) bool {
		return slices.ContainsFunc(p.OwnPathUnits, func(n string) bool { return strings.EqualFold(n, unit) })
	}
	return named(unit) || slices.ContainsFunc(p.Aliases, func(a UnitAlias) bool { return strings.EqualFold(a.Unit, unit) && named(a.Label) }) // the Goblin Hulk is a Gundabad Ogre
}

// Types gives a unit's types in this company: the vanilla ones with the company's changes.
func (p CompanyProfile) UnitTypes(unit string, vanilla []string) []string {
	o, ok := p.Types[unit]
	if !ok {
		return vanilla
	}
	out := slices.DeleteFunc(slices.Clone(vanilla), func(t string) bool { return slices.Contains(o.Remove, t) })
	for _, t := range o.Add {
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// Models is how many models a unit counts as against the company's maximum.
func (p CompanyProfile) Models(unit string) int {
	if m, ok := p.Composites[unit]; ok && m.Slots > 0 {
		return m.Slots
	}
	if n := p.ModelCounts[unit]; n > 0 {
		return n
	}
	return 1
}

// Fallback finds the rule that lets a unit that would break a limit be replaced by another result.
func (p CompanyProfile) Fallback(unit, when string) (RecruitFallback, bool) {
	for _, f := range p.Fallbacks {
		if f.Unit == unit && f.When == when {
			return f, true
		}
	}
	return RecruitFallback{}, false
}

// PromotionOnHero finds the promotion a unit makes when it becomes a hero.
func (p CompanyProfile) PromotionOnHero(unit string) (HeroPromotion, bool) {
	for _, h := range p.Promotions {
		if h.From == unit && h.OnResult == "heroInTheMaking" {
			return h, true
		}
	}
	return HeroPromotion{}, false
}

// PageNotes are the notes of a company's page grouped by where the page shows them, each sentence once: a note that repeats a
// limit line or another note is dropped, and so are the ones about On Their Own Path (the units are marked instead).
type PageNotes struct{ Start, Chart, Promotions, Limits []string }

func (p CompanyProfile) PageNotes() PageNotes {
	seen := map[string]bool{}
	for _, l := range p.LimitLines() { // the limits are printed in their own section
		seen[strings.ToLower(strings.Join(strings.Fields(l), " "))] = true
	}
	take := func(texts ...[]string) []string {
		var out []string
		for _, list := range texts {
			for _, t := range list {
				k := strings.ToLower(strings.Join(strings.Fields(t), " "))
				if k == "" || seen[k] || strings.Contains(k, "on their own path") || strings.Contains(k, "on its own path") {
					continue
				}
				seen[k] = true
				out = append(out, strings.TrimSpace(t))
			}
		}
		return out
	}
	return PageNotes{Start: take(p.NotesIn("startingCompany")), Chart: take(p.RecruitNotes, p.NotesIn("specialChart")),
		Promotions: take(p.NotesIn("advancements")), Limits: take(p.NotesIn("eliteCompany"))}
}
