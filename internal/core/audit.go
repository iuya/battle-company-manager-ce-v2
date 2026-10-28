package core

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// The audit builds, for every Battle Company of a rule set, what the app really shows and does with it (the same services and
// builders the pages use: BuildDetail, the armoury's whyNot, GearMatchesOption, the recruit charts and the limits), and flags what
// looks wrong. It exists so reviewers can check the 37 companies against the rules quickly and objectively (docs/definitions.md,
// "Audit report").

// Anomaly kinds the audit flags.
const (
	FlagOfferNotDerivable = "offer-not-derivable" // the armoury offers wargear that no option of the profile (or company grant) explains
	FlagOptionUnbuyable   = "option-unbuyable"    // a profile option no item of the company's armoury matches
	FlagPriceMissing      = "price-missing"       // an armoury item without influence cost
	FlagUnitUnresolved    = "unit-unresolved"     // a unit named by the company that is no vanilla profile (nor a label of the company)
	FlagUnitNoWargear     = "unit-no-wargear"     // a chart unit with no wargear at all
	FlagDuplicateName     = "duplicate-name"      // a vanilla name defined twice (first one wins) that the company uses
	FlagTooltipName       = "tooltip-name"        // a tooltip that never mentions the item it explains
	FlagTooltipMissing    = "tooltip-missing"     // an item or rule chip without tooltip text
	FlagOptionUnknown     = "option-unknown-item" // an option names an item the item table does not know
	FlagNameFormat        = "name-format"         // a name that differs from the canonical one only by case or spacing
	FlagRuleUnresolved    = "rule-unresolved"     // a special rule no file defines
	FlagPartialBundle     = "partial-bundle"      // a row names one piece of a multi-item option: the unit gets the whole bundle
	FlagPriceMismatch     = "price-mismatch"      // a unit's points differ from the vanilla base cost plus the cost of its option (price rule)
	auditInfluence        = 1000                  // what the audited company holds, so that cost never hides a rule
	auditUnreviewedStatus = "unreviewed"          // a duplicate that differs and that nobody reviewed
)

// AuditKinds lists the flag kinds in report order.
var AuditKinds = []string{FlagOfferNotDerivable, FlagOptionUnbuyable, FlagPriceMissing, FlagUnitUnresolved, FlagUnitNoWargear,
	FlagDuplicateName, FlagTooltipName, FlagTooltipMissing, FlagOptionUnknown, FlagNameFormat, FlagRuleUnresolved, FlagPriceMismatch, FlagPartialBundle}

// AuditFlag is one anomaly.
type AuditFlag struct {
	Kind, Company, Subject, Detail string
}

// AuditInput is what the loader says about the vanilla file besides the rule set itself.
type AuditInput struct {
	Ambiguities []VanillaAmbiguity // data.vanillaAmbiguities of the Battle Companies file
	Unreviewed  []VanillaAmbiguity // duplicates that differ and nobody reviewed (logged by the loader; see CaptureDuplicates)
}

// AuditReport is the audit of every company of a rule set.
type AuditReport struct {
	Companies []CompanyAudit
}

// Flags lists the anomalies of every company.
func (r AuditReport) Flags() []AuditFlag {
	var out []AuditFlag
	for _, c := range r.Companies {
		out = append(out, c.Flags...)
	}
	return out
}

// CompanyAudit is everything the app derives for one company.
type CompanyAudit struct {
	Name, Alignment string
	Profile         CompanyProfile
	Detail          CompanyDetail // the starting company as the page builds it
	Units           []UnitAudit
	Armoury         []OfferAudit
	Tips            []TipAudit
	Duplicates      []string // the duplicated vanilla names the company uses, with how the loader settled them
	Flags           []AuditFlag
}

// UnitAudit is one distinct unit profile of a company: a unit with the wargear a chart gives it.
type UnitAudit struct {
	Name     string   // as the company names it
	Wargear  []string // what the chart or start list adds
	Sources  []string // where the company gets it: start, chart rows, special units, promotions
	Alias    *UnitAlias
	Base     string // the vanilla unit it resolves to; empty when unresolved
	Vanilla  VanillaUnit
	Stats    []Attribute
	Warrior  CompanyUnit // as a warrior of the company
	Hero     CompanyUnit
	Offers   []UnitOffer
	Upgrades []HeroUpgrade
	Promotes []Advancement
	CanLead  bool
	OwnPath  bool
}

// UnitOffer is how one armoury item looks to a unit, as a warrior and as a hero.
type UnitOffer struct {
	Item             GearOffer
	WarriorReason    string // empty: the warrior can buy it
	HeroReason       string
	WarriorGold      int
	HeroGold         int
	DerivedByWarrior string // the option of the profile that explains the offer ("Shield and Spear"), or "equipment"/"company grant"
	DerivedByHero    string
}

// OfferAudit is one item of a company's armoury.
type OfferAudit struct {
	GearOffer
	Tip TipAudit
}

// TipAudit is the tooltip a chip shows and the glossary term its name leads to.
type TipAudit struct {
	Name   string
	Kind   string // gear or rule
	Text   string
	Source string // where the text comes from: bc definition, bc armoury, vanilla gear, bc rule, vanilla rule, none
	Term   string // the glossary term the name resolves to; empty when none
}

// CaptureDuplicates is a slog handler that collects the loader's warnings about unreviewed vanilla duplicates.
type CaptureDuplicates struct{ Found []VanillaAmbiguity }

func (c *CaptureDuplicates) Enabled(context.Context, slog.Level) bool { return true }
func (c *CaptureDuplicates) WithAttrs([]slog.Attr) slog.Handler       { return c }
func (c *CaptureDuplicates) WithGroup(string) slog.Handler            { return c }
func (c *CaptureDuplicates) Handle(_ context.Context, r slog.Record) error {
	var a VanillaAmbiguity
	r.Attrs(func(at slog.Attr) bool {
		switch at.Key {
		case "kind":
			a.Kind = at.Value.String()
		case "name":
			a.Name = at.Value.String()
		}
		return true
	})
	if a.Name != "" {
		a.Status = auditUnreviewedStatus
		c.Found = append(c.Found, a)
	}
	return nil
}

// LoadRuleSet builds a rule set straight from the three files, as Rules does from the stored ones, logging to log.
func LoadRuleSet(vanilla, bc, glossary []byte, log *slog.Logger) (*RuleSet, error) {
	if isLeanFile(bc) { // a Battle Companies file of the lean format
		return loadV2(vanilla, bc, glossary, log)
	}
	reviewed, err := ParseVanillaAmbiguities(bc)
	if err != nil {
		return nil, err
	}
	vb, err := ParseVanillaWith(vanilla, reviewed, log)
	if err != nil {
		return nil, err
	}
	bb, err := ParseBattleCompanies(bc)
	if err != nil {
		return nil, err
	}
	bb.SpellWargearLikeVanilla(vb)
	bb.ApplyUnitFillIns(vb)
	if err := bb.CheckRaces(vb); err != nil {
		return nil, err
	}
	hash, err := RulesetHash(glossary)
	if err != nil {
		return nil, err
	}
	gl, err := BuildGlossaryWith(RulesetInfo{Kind: KindGlossary, Hash: hash}, glossary, GlossaryExtras(vanilla, bc))
	if err != nil {
		return nil, err
	}
	return &RuleSet{Vanilla: vb, BC: bb, Glossary: gl}, nil
}

// itemTable knows every name of a piece of gear the files define, to check the names companies use.
type itemTable struct {
	byKey     map[string]string // ItemKey -> canonical name
	canonical map[string]string // lower-cased name -> canonical spelling
}

func newItemTable(rs *RuleSet) itemTable {
	t := itemTable{byKey: map[string]string{}, canonical: map[string]string{}}
	add := func(n string) {
		if n = strings.TrimSpace(n); n != "" {
			t.byKey[ItemKey(n)] = n
			// The canonical spelling is the capitalised one; among equals the smallest, so the table never depends on map order.
			cur, ok := t.canonical[strings.ToLower(n)]
			if !ok || (unicode.IsUpper([]rune(n)[0]) && !unicode.IsUpper([]rune(cur)[0])) || (unicode.IsUpper([]rune(n)[0]) == unicode.IsUpper([]rune(cur)[0]) && n < cur) {
				t.canonical[strings.ToLower(n)] = n
			}
		}
	}
	for _, o := range rs.BC.armoury {
		add(o.Name)
		for _, a := range o.AlsoCalled {
			add(a)
		}
	}
	for _, a := range rs.BC.annotations {
		add(a.Name)
		for _, x := range a.Aliases {
			add(x)
		}
	}
	for n := range rs.Vanilla.gear {
		add(n)
	}
	for _, u := range rs.Vanilla.units {
		for _, w := range u.Wargear {
			add(w)
		}
	}
	return t
}

func (t itemTable) knows(part string) bool { _, ok := t.byKey[ItemKey(part)]; return ok }

// AuditRules audits every company of the rule set.
func AuditRules(rs *RuleSet, in AuditInput) AuditReport {
	table := newItemTable(rs)
	var out AuditReport
	for _, p := range rs.BC.Profiles() {
		out.Companies = append(out.Companies, auditCompany(rs, in, table, p.WithPreviews(rs.Vanilla)))
	}
	return out
}

type unitRef struct {
	name    string
	wargear []string
	sources []string
}

func auditCompany(rs *RuleSet, in AuditInput, table itemTable, p CompanyProfile) CompanyAudit {
	ca := CompanyAudit{Name: p.Name, Alignment: p.Alignment, Profile: p}
	flag := func(kind, subject, format string, args ...any) {
		f := AuditFlag{kind, p.Name, subject, fmt.Sprintf(format, args...)}
		if !slices.Contains(ca.Flags, f) { // the same profile reached from several chart rows is flagged once
			ca.Flags = append(ca.Flags, f)
		}
	}

	// The starting company as the creation form builds it.
	heroes := p.DefaultHeroes()
	st := CompanyState{Company: CampaignCompany{ID: 1, Name: p.Name, Faction: p.Name}, Influence: rs.BC.Recruiting().StartingInfluence, Gold: p.StartGold, Units: StartingUnits(p, heroes)}
	ca.Detail = BuildDetail(rs, CampaignSettings{}, st)
	ca.Armoury = nil

	// Every distinct unit the company can get, with where from.
	var refs []unitRef
	addRef := func(name string, wargear []string, source string) {
		if strings.TrimSpace(name) == "" {
			return
		}
		key := exactKey(name, wargear)
		for i := range refs {
			if exactKey(refs[i].name, refs[i].wargear) == key {
				if !slices.Contains(refs[i].sources, source) {
					refs[i].sources = append(refs[i].sources, source)
				}
				return
			}
		}
		refs = append(refs, unitRef{name, slices.Clone(wargear), []string{source}})
	}
	for _, s := range p.Start {
		addRef(s.Unit, s.Wargear, "start")
	}
	chartUnit := func(r Reinforcement, source string) {
		switch r.Type {
		case "unit":
			addRef(r.Unit, r.Wargear, source)
			if r.Alt != "" {
				addRef(r.Alt, nil, source+" (alternative)")
			}
		case "unitChoice":
			for _, o := range r.Options {
				res := optionResult(o)
				addRef(res.Unit, res.Wargear, source+" (choice)")
			}
		}
	}
	for _, c := range []struct {
		name string
		rows []ChartRow
	}{{"reinforcements", p.Reinforcements}, {"special chart", p.SpecialChart}} {
		for _, row := range c.rows {
			chartUnit(row.Result, fmt.Sprintf("%s %s", c.name, auditRoll(row.Min, row.Max)))
		}
	}
	for _, s := range p.SpecialUnits {
		addRef(s.Unit, nil, "special unit")
	}
	for _, a := range p.Advancements {
		for _, t := range a.To {
			addRef(t.Unit, t.Wargear, "promotion from "+a.FromUnit)
		}
	}

	armoury := rs.BC.Armoury(p.BCCompany)
	for _, o := range armoury {
		ca.Armoury = append(ca.Armoury, OfferAudit{GearOffer: o, Tip: gearTip(rs, p.BCCompany, o.Name)})
		if o.Cost <= 0 {
			flag(FlagPriceMissing, o.Name, "influence cost %d (gold %d/%d)", o.Cost, o.Gold.Warrior, o.Gold.Hero)
		}
	}

	// Resolve every unit and take the real armoury answer for it.
	gearNames := map[string]bool{}
	for _, o := range armoury {
		gearNames[o.Name] = true
	}
	ruleNames := map[string]bool{}
	var resolved []VanillaUnit // the profiles of the company, in chart order, to explain a hero's wargear
	for _, r := range refs {
		if v, _, ok := resolveUnit(rs, p, r.name); ok {
			resolved = append(resolved, v)
		}
	}
	for _, r := range refs {
		ua := UnitAudit{Name: r.name, Wargear: r.wargear, Sources: r.sources}
		v, alias, ok := resolveUnit(rs, p, r.name)
		if !ok {
			flag(FlagUnitUnresolved, r.name, "named by %s; no vanilla profile, no company label", strings.Join(r.sources, ", "))
			ca.Units = append(ca.Units, ua)
			continue
		}
		ua.Alias, ua.Base, ua.Vanilla = alias, v.Name, v
		if v.Name != r.name && alias == nil {
			flag(FlagNameFormat, r.name, "resolves to %q", v.Name)
		}
		hero := v.Stats["might"] > 0 || v.Stats["will"] > 0 || v.Stats["fate"] > 0
		ua.Stats = rs.Vanilla.Profile(v, hero, nil)
		ua.CanLead, ua.OwnPath = p.CanLead(r.name), p.OnOwnPath(r.name)

		us := CompanyState{Company: CampaignCompany{ID: 1, Name: p.Name, Faction: p.Name}, Influence: auditInfluence, Gold: auditInfluence,
			Units: []UnitState{{ID: 1, Base: r.name, Role: RoleWarrior, Gear: slices.Clone(r.wargear)}, {ID: 2, Base: r.name, Role: RoleHero, Bonus: rs.BC.HeroBonus(nil), Gear: slices.Clone(r.wargear)}}}
		sh := newShop(rs, CampaignSettings{}, us)
		ua.Warrior, _ = sh.unit(1)
		ua.Hero, _ = sh.unit(2)
		if _, pb := completeBundle(rs, p.BCCompany, v, r.wargear, nil); pb != nil {
			flag(FlagPartialBundle, r.name, "%s names %s, a piece of the option %q (%d points as a bundle); the unit gets %s too", strings.Join(r.sources, ", "), strings.Join(pb.Named, ", "), pb.Option, pb.Points, strings.Join(pb.Missing, ", "))
		}
		if why := priceMismatch(v, alias, ua.Warrior); why != "" {
			flag(FlagPriceMismatch, r.name, "%s", why)
		}
		ua.Upgrades = upgradesOf(rs, p.BCCompany, ua.Hero)
		for _, a := range p.Advancements {
			if a.Matches(ua.Warrior) { // the unit, carrying the wargear the promotion asks for
				ua.Promotes = append(ua.Promotes, a)
			}
		}
		if len(v.Wargear) == 0 && len(v.Includes) == 0 && len(r.wargear) == 0 && (alias == nil || len(alias.Selected) == 0) {
			flag(FlagUnitNoWargear, r.name, "the vanilla profile lists no wargear and the company adds none")
		}
		for _, w := range append(slices.Clone(v.Wargear), r.wargear...) {
			gearNames[w] = true
		}
		for _, o := range v.Options {
			for _, part := range OptionItems(o.Name) {
				gearNames[part] = true
			}
		}
		for _, rule := range append(slices.Clone(v.SpecialRules), aliasRules(alias)...) {
			ruleNames[rule] = true
		}
		heroOptions := companyOptions(rs, p, resolved, v)
		for _, offer := range armoury {
			var uo UnitOffer
			uo.Item = offer
			uo.WarriorReason, _ = sh.whyNot(ua.Warrior, offer)
			uo.HeroReason, _ = sh.whyNot(ua.Hero, offer)
			uo.WarriorGold, uo.HeroGold = sh.goldPrice(ua.Warrior, offer), offer.Gold.Hero
			uo.DerivedByWarrior = derivation(offer, p, v.Options, v)
			uo.DerivedByHero = derivation(offer, p, heroOptions, v)
			if offer.Size == "" && offer.Only == "" && uo.WarriorReason == "" && uo.DerivedByWarrior == "" {
				flag(FlagOfferNotDerivable, r.name+" / "+offer.Name, "a warrior can buy it, but no option of %s matches", v.Name)
			}
			if offer.Size == "" && offer.Only == "" && uo.HeroReason == "" && uo.DerivedByHero == "" {
				flag(FlagOfferNotDerivable, r.name+" (hero) / "+offer.Name, "a hero can buy it, but no option of the company's units matches")
			}
			ua.Offers = append(ua.Offers, uo)
		}
		for _, o := range v.Options {
			parts := OptionItems(o.Name)
			if rs.BC.Reserved(v.Name, o.Name) {
				continue
			}
			optionName := o.Name
			if item, ok := rs.BC.ReadAs(v.Name, o.Name); ok { // the hammer is read as the Hand-and-a-half Weapon of the armoury; the Troll Spear is its own item
				optionName = item
			}
			if _, unsold := rs.BC.Unsold(v.Name, o.Name); !unsold && !slices.ContainsFunc(armoury, func(g GearOffer) bool { return GearMatchesOption(g, optionName) }) {
				flag(FlagOptionUnbuyable, v.Name+" / "+o.Name, "no armoury item of the company matches this option")
			}
			for _, part := range parts {
				if !table.knows(part) {
					flag(FlagOptionUnknown, v.Name+" / "+o.Name, "%q is not in the item table", part)
				}
			}
		}
		ca.Units = append(ca.Units, ua)
	}

	// Names, tooltips.
	for _, r := range refs {
		for _, w := range r.wargear {
			if c, ok := table.canonical[strings.ToLower(strings.TrimSpace(w))]; ok && c != w {
				flag(FlagNameFormat, r.name+" / "+w, "wargear spelled %q, the item table has %q", w, c)
			} else if !ok {
				flag(FlagOptionUnknown, r.name+" / "+w, "wargear %q is not in the item table", w)
			}
		}
		if n := strings.Join(strings.Fields(r.name), " "); n != r.name {
			flag(FlagNameFormat, r.name, "stray whitespace")
		}
	}
	names := make([]string, 0, len(gearNames))
	for n := range gearNames {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if li, lj := strings.ToLower(names[i]), strings.ToLower(names[j]); li != lj {
			return li < lj
		}
		return names[i] < names[j]
	})
	for _, n := range names {
		tip := gearTip(rs, p.BCCompany, n)
		ca.Tips = append(ca.Tips, tip)
		switch {
		case tip.Source == "vanilla mount profile":
		case tip.Text == "":
			flag(FlagTooltipMissing, n, "no tooltip text (gear)")
		case !tipMentions(rs, tip.Text, n):
			flag(FlagTooltipName, n, "tooltip (%s) never says %q: %s", tip.Source, n, clip(tip.Text, 80))
		}
	}
	rnames := make([]string, 0, len(ruleNames))
	for n := range ruleNames {
		rnames = append(rnames, n)
	}
	sort.Strings(rnames)
	for _, n := range rnames {
		tip := ruleTip(rs, n)
		ca.Tips = append(ca.Tips, tip)
		if tip.Text == "" {
			flag(FlagRuleUnresolved, n, "no file defines this special rule")
		}
	}

	// Duplicated vanilla names the company uses.
	used := map[string]string{} // lower-cased kind|name -> how it is used
	for _, r := range refs {
		used["unit|"+strings.ToLower(r.name)] = "unit " + r.name
		if v, _, ok := resolveUnit(rs, p, r.name); ok {
			used["unit|"+strings.ToLower(v.Name)] = "unit " + r.name
			for _, w := range append(slices.Clone(v.Wargear), r.wargear...) {
				used["gear|"+strings.ToLower(w)] = "wargear of " + r.name
			}
			for _, o := range v.Options {
				for _, part := range OptionItems(o.Name) {
					used["gear|"+strings.ToLower(part)] = "option of " + r.name
				}
			}
			for _, rule := range v.SpecialRules {
				used["keyword|"+strings.ToLower(rule)] = "rule of " + r.name
			}
		}
	}
	for _, a := range slices.Concat(in.Ambiguities, in.Unreviewed) {
		how, hit := used[strings.ToLower(a.Kind)+"|"+strings.ToLower(a.Name)]
		if !hit {
			continue
		}
		ca.Duplicates = append(ca.Duplicates, fmt.Sprintf("%s %q (%s): %s", a.Kind, a.Name, a.Status, how))
		if a.Status == "firstWins" || a.Status == auditUnreviewedStatus {
			flag(FlagDuplicateName, a.Name, "%s defined more than once in vanilla, status %s: %s", a.Kind, a.Status, how)
		}
	}
	slices.SortStableFunc(ca.Flags, func(a, b AuditFlag) int {
		return strings.Compare(fmt.Sprintf("%02d%s%s", slices.Index(AuditKinds, a.Kind), a.Subject, a.Detail), fmt.Sprintf("%02d%s%s", slices.Index(AuditKinds, b.Kind), b.Subject, b.Detail))
	})
	return ca
}

// exactKey identifies a unit with its wargear by the exact spelling, so that a spelling mistake is a profile of its own.
func exactKey(unit string, wargear []string) string {
	w := slices.Clone(wargear)
	slices.Sort(w)
	return unit + "|" + strings.Join(w, ",")
}

func auditRoll(lo, hi int) string {
	if lo == hi {
		return fmt.Sprint(lo)
	}
	return fmt.Sprintf("%d-%d", lo, hi)
}

func aliasRules(a *UnitAlias) []string {
	if a == nil {
		return nil
	}
	return a.AddRules
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// resolveUnit finds the vanilla profile a unit name stands for in a company, as buildUnit does: a vanilla name, else a label.
func resolveUnit(rs *RuleSet, p CompanyProfile, name string) (VanillaUnit, *UnitAlias, bool) {
	if v, ok := rs.Vanilla.Unit(name); ok {
		return v, nil, true
	}
	for i := range p.Aliases {
		if strings.EqualFold(p.Aliases[i].Label, name) {
			if v, ok := rs.Vanilla.Unit(p.Aliases[i].Unit); ok {
				return v, &p.Aliases[i], true
			}
		}
	}
	return VanillaUnit{}, nil, false
}

// companyOptions are the options a hero of the company could take from any profile of its charts, as legalWargear counts them.
func companyOptions(rs *RuleSet, p CompanyProfile, resolved []VanillaUnit, own VanillaUnit) []ChoiceOption {
	var out []ChoiceOption
	seen := map[string]bool{}
	add := func(v VanillaUnit) {
		if seen[v.Name] {
			return
		}
		seen[v.Name] = true
		out = append(out, v.Options...)
		for _, w := range v.Wargear {
			out = append(out, ChoiceOption{Name: w})
		}
	}
	for _, v := range resolved {
		add(v)
	}
	add(own)
	return out
}

// derivation names the reason the profile explains an offer: an option it matches, the company's own wargear, or equipment
// (which needs no option); empty when nothing does.
func derivation(o GearOffer, p CompanyProfile, options []ChoiceOption, v VanillaUnit) string {
	if o.Size != "" {
		return "equipment"
	}
	for _, opt := range options {
		if GearMatchesOption(o, opt.Name) {
			return opt.Name
		}
	}
	return ""
}

// gearTip is the tooltip a gear chip shows for the item, and the glossary term its name leads to.
func gearTip(rs *RuleSet, c BCCompany, name string) TipAudit {
	e, _ := equipmentFor(rs, c, name, false, false, false)
	tip := TipAudit{Name: name, Kind: "gear", Text: e.Description, Source: "none"}
	if tip.Text == "" && len(e.MountStats) > 0 {
		tip.Text, tip.Source = "mount profile", "vanilla mount profile" // the chip shows the mount's stat block
	}
	a, annotated := rs.BC.Annotation(name)
	_, offered := rs.BC.Offer(c, name)
	_, vanilla := rs.Vanilla.Gear(name)
	switch {
	case tip.Source != "none":
	case tip.Text == "":
	case annotated && a.Definition != "":
		tip.Source = "bc definition"
	case offered:
		tip.Source = "bc armoury"
	case vanilla:
		tip.Source = "vanilla gear"
	}
	if t, ok := rs.Glossary.Find(name); ok {
		tip.Term = t.Name
	}
	return tip
}

// ruleTip is the text a special rule chip shows.
func ruleTip(rs *RuleSet, name string) TipAudit {
	tip := TipAudit{Name: name, Kind: "rule", Source: "none"}
	if d, ok := rs.BC.DefinedKeyword(name); ok {
		tip.Text, tip.Source = d, "bc rule"
	} else if d, ok := rs.Vanilla.Keyword(name); ok {
		tip.Text, tip.Source = d, "vanilla rule"
	} else if d, ok := rs.BC.ParameterisedKeyword(name); ok {
		tip.Text, tip.Source = d, "bc parameterised rule"
	}
	if t, ok := rs.Glossary.Find(name); ok {
		tip.Term = t.Name
	}
	return tip
}

// tipMentions says whether a tooltip text names its item (or one of the item's other spellings, or the same item by ItemKey).
func tipMentions(rs *RuleSet, text, name string) bool {
	low := strings.ToLower(text)
	names := []string{name}
	if a, ok := rs.BC.Annotation(name); ok {
		names = append(names, a.Name)
		names = append(names, a.Aliases...)
	}
	for _, n := range names {
		if strings.Contains(low, strings.ToLower(n)) || strings.Contains(strings.ReplaceAll(ItemKey(low), " ", ""), ItemKey(n)) {
			return true
		}
	}
	return false
}

// priceMismatch checks the price rule on a unit as a warrior: its points are the vanilla base profile cost with the options the
// company selected for it, plus the cost of the one option its wargear is (nothing else is a price source). It describes the
// difference, or returns "" when the price is the rule's.
func priceMismatch(v VanillaUnit, alias *UnitAlias, w CompanyUnit) string {
	want := v.Points
	if alias != nil {
		for _, name := range alias.Selected {
			if i := slices.IndexFunc(v.Options, func(o ChoiceOption) bool { return o.Name == name }); i >= 0 {
				want += v.Options[i].Points
			}
		}
	}
	var wargear []Equipment
	for _, e := range w.Equipment {
		if !e.Profile && e.Size == "" {
			wargear = append(wargear, e)
		}
	}
	covered, rest := OptionsCover(wargear, v.Options)
	if len(rest) > 0 {
		return fmt.Sprintf("its wargear is no part of any option of %s, so it is priced by rating instead of base %d + option (%d points)", v.Name, v.Points, w.Points)
	}
	if want += covered; w.Points != want {
		return fmt.Sprintf("%d points, the price rule gives %d (base %d + options %d)", w.Points, want, want-covered, covered)
	}
	return ""
}
