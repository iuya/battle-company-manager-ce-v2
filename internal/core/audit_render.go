package core

import (
	"fmt"
	"sort"
	"strings"
)

// AuditFileName is the file a company's audit is written to.
func AuditFileName(company string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(company) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-") + ".md"
}

// RenderAudit renders the index (key "index.md") and one markdown file per company, keyed by file name. The output is stable: the
// order of the rules files, no timestamps.
func RenderAudit(r AuditReport) map[string]string {
	files := map[string]string{}
	for _, c := range r.Companies {
		files[AuditFileName(c.Name)] = renderCompany(c)
	}
	files["index.md"] = renderIndex(r)
	return files
}

type md struct{ strings.Builder }

func (m *md) line(format string, args ...any) { fmt.Fprintf(m, format+"\n", args...) }
func (m *md) row(cells ...string) {
	m.WriteString("|")
	for _, c := range cells {
		m.WriteString(" " + cell(c) + " |")
	}
	m.WriteString("\n")
}
func (m *md) head(cells ...string) {
	m.row(cells...)
	m.WriteString("|" + strings.Repeat(" --- |", len(cells)) + "\n")
}

func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	if s == "" {
		return "-"
	}
	return s
}

func renderIndex(r AuditReport) string {
	var m md
	m.line("# Battle Companies audit")
	m.line("")
	m.line("Generated from the shipped rules through core.BuildDetail, the armoury and the recruit charts (`make audit`).")
	m.line("")
	counts := map[string]int{}
	for _, f := range r.Flags() {
		counts[f.Kind]++
	}
	distinct := map[string]map[[2]string]bool{}
	for _, f := range r.Flags() {
		if distinct[f.Kind] == nil {
			distinct[f.Kind] = map[[2]string]bool{}
		}
		distinct[f.Kind][[2]string{f.Subject, f.Detail}] = true
	}
	m.line("## Anomalies by kind")
	m.line("")
	m.head("Kind", "Count", "Distinct across companies")
	for _, k := range AuditKinds {
		m.row(k, fmt.Sprint(counts[k]), fmt.Sprint(len(distinct[k])))
	}
	m.line("")
	m.line("## Companies")
	m.line("")
	head := append([]string{"Company", "Alignment"}, AuditKinds...)
	m.head(append(head, "Total")...)
	for _, c := range r.Companies {
		per := map[string]int{}
		for _, f := range c.Flags {
			per[f.Kind]++
		}
		cells := []string{fmt.Sprintf("[%s](%s)", c.Name, AuditFileName(c.Name)), c.Alignment}
		for _, k := range AuditKinds {
			cells = append(cells, fmt.Sprint(per[k]))
		}
		m.row(append(cells, fmt.Sprint(len(c.Flags)))...)
	}
	return m.String()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func renderCompany(c CompanyAudit) string {
	var m md
	p := c.Profile
	m.line("# %s", c.Name)
	m.line("")
	m.line("Alignment %s. Max models %d. Reinforcement cost %d influence. Break point %s.", c.Alignment, p.MaxModels, p.ReinforcementCost, breakPoint(p.BreakPoint))
	m.line("")

	m.line("## Anomalies (%d)", len(c.Flags))
	m.line("")
	if len(c.Flags) == 0 {
		m.line("None.")
	} else {
		m.head("Kind", "Subject", "Detail")
		for _, f := range c.Flags {
			m.row(f.Kind, f.Subject, f.Detail)
		}
	}
	m.line("")

	m.line("## Start")
	m.line("")
	m.line("Starting influence %d, gold %d.", c.Detail.Influence, p.StartGold)
	m.line("")
	m.line("Oath: %s", strings.ReplaceAll(p.Oath.Text, "{name}", "<name>"))
	m.line("")
	m.line("Vow button: %s", p.Oath.Vow)
	m.line("")
	m.head("Count", "Unit", "Wargear", "Role", "May lead", "Points")
	for _, s := range p.Start {
		pts := 0
		if v, _, ok := resolveUnit2(c, s.Unit); ok {
			pts = v.Points
		}
		m.row(fmt.Sprint(s.Count), s.Unit, strings.Join(s.Wargear, ", "), string(s.Role), yesNo(p.CanLead(s.Unit)), fmt.Sprint(pts))
	}
	m.line("")
	m.line("Company at creation (default heroes): %d models, %d points.", c.Detail.ModelCount(), c.Detail.Points)
	m.line("")

	m.line("## Recruiting")
	m.line("")
	m.line("Cost %d influence per roll.", p.ReinforcementCost)
	m.line("")
	chart := func(title string, rows []ChartRow) {
		if len(rows) == 0 {
			return
		}
		m.line("### %s", title)
		m.line("")
		m.head("Roll", "Type", "Result", "Count", "Rare", "Choice", "Wargear", "Options (unitChoice)", "Alternative")
		for _, row := range rows {
			r := row.Result
			var opts []string
			for _, o := range r.Options {
				s := choiceName(o)
				if o.Label != "" {
					s += " (" + o.Unit + ")"
				}
				if len(o.Selected) > 0 {
					s += " [" + strings.Join(o.Selected, ", ") + "]"
				}
				if o.Rare > 0 {
					s += fmt.Sprintf(" rare %d", o.Rare)
				}
				opts = append(opts, s)
			}
			m.row(auditRoll(row.Min, row.Max), r.Type, describe(r), fmt.Sprint(r.Count), fmt.Sprint(r.Rare), yesNo(r.Choice), strings.Join(r.Wargear, ", "), strings.Join(opts, "; "), r.Alt)
		}
		m.line("")
	}
	chart("Reinforcement chart", p.Reinforcements)
	chart("Special chart", p.SpecialChart)
	if len(p.SpecialUnits) > 0 {
		m.line("### Special units")
		m.line("")
		m.head("Unit", "Cost", "Rare", "Choice")
		for _, s := range p.SpecialUnits {
			m.row(s.Unit, fmt.Sprint(s.Cost), fmt.Sprint(s.Rare), yesNo(s.Choice))
		}
		m.line("")
	}
	for _, n := range p.RecruitNotes {
		m.line("- Note: %s", n)
	}
	m.line("")

	m.line("## Company rules")
	m.line("")
	for _, r := range p.Rules {
		m.line("- %s", r)
	}
	for _, g := range p.Granted {
		m.line("- Granted rule %s to %s (cost %d): %s", g.Rule, g.AppliesTo(), g.Cost, g.Text)
	}
	for _, e := range p.Exchanges {
		m.line("- Exchange %s for %s: %s", e.Remove, e.Add, e.Text)
	}
	for _, s := range p.Substitutions {
		m.line("- Substitution: %s", s)
	}
	for _, a := range p.Aliases {
		m.line("- Unit %s is %s with %s, options selected [%s]: %s", a.Label, a.Unit, strings.Join(a.AddRules, ", "), strings.Join(a.Selected, ", "), a.Text)
	}
	for _, k := range p.KeywordLimits {
		m.line("- Keyword limit %s %s: %s", k.Keyword, k.Share.Text(), k.Text)
	}
	for _, b := range p.Balance {
		m.line("- Balance: %s not more than %s", b.Keyword, b.NotMoreThan)
	}
	for _, h := range p.HeroUpgrades {
		m.line("- Hero upgrade %s (%s), restriction %q: %s", h.Name, h.Rule, h.Restriction(), h.Text)
	}
	m.line("")
	m.line("### Limits")
	m.line("")
	m.head("Limit", "Used at creation", "Max at creation")
	for _, l := range c.Detail.Limits {
		m.row(l.Label, fmt.Sprint(l.Used), fmt.Sprint(l.Max))
	}
	var ids []string
	for k := range p.LimitOverrides {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	for _, k := range ids {
		m.line("")
		m.line("Own share for limit %s: %s", k, p.LimitOverrides[k].Text())
	}
	for _, e := range p.LimitExemptions {
		m.line("")
		m.line("Exemption %v: %s", e.Limits, e.Text)
	}
	m.line("")
	m.line("### Paths")
	m.line("")
	m.head("Path", "Hero limit per company", "Start action")
	for _, pa := range p.Paths {
		m.row(pa.Name, fmt.Sprint(pa.LimitPerCompany), pa.StartAction)
	}
	m.line("")
	m.line("Units on their own path: %s. Own path given: %s.", strings.Join(p.OwnPathUnits, ", "), p.OwnPath)
	m.line("")

	m.line("## Units")
	for _, u := range c.Units {
		renderUnit(&m, c, u)
	}

	m.line("")
	m.line("## Armoury of the company")
	m.line("")
	m.head("Item", "Size", "Influence", "Gold (warrior/hero)", "Restrictions", "Classes", "Exclusive", "Tooltip source")
	for _, o := range c.Armoury {
		var classes []string
		for _, cl := range o.Classes {
			classes = append(classes, string(cl))
		}
		m.row(o.Name, o.Size, fmt.Sprint(o.Cost), fmt.Sprintf("%d/%d", o.Gold.Warrior, o.Gold.Hero), strings.Join(o.Restrictions, ", "), strings.Join(classes, ", "), yesNo(o.Exclusive), o.Tip.Source)
	}
	m.line("")
	m.line("## Tooltips and glossary terms")
	m.line("")
	m.head("Name", "Kind", "Source", "Glossary term", "Text")
	for _, t := range c.Tips {
		m.row(t.Name, t.Kind, t.Source, t.Term, clip(t.Text, 120))
	}
	m.line("")
	m.line("## Duplicated vanilla names the company uses")
	m.line("")
	if len(c.Duplicates) == 0 {
		m.line("None.")
	}
	for _, d := range c.Duplicates {
		m.line("- %s", d)
	}
	return m.String()
}

func breakPoint(n int) string {
	if n == 0 {
		return "usual"
	}
	return fmt.Sprintf("%d%%", n)
}

// resolveUnit2 resolves a unit of the audit through the profile already audited.
func resolveUnit2(c CompanyAudit, name string) (VanillaUnit, *UnitAlias, bool) {
	for _, u := range c.Units {
		if u.Name == name && u.Base != "" {
			return u.Vanilla, u.Alias, true
		}
	}
	return VanillaUnit{}, nil, false
}

func auditStats(stats []Attribute) string {
	var parts []string
	for _, a := range stats {
		parts = append(parts, fmt.Sprintf("%s %d%s", a.Abbr, a.Value, a.Trail))
	}
	return strings.Join(parts, " · ")
}

func renderUnit(m *md, c CompanyAudit, u UnitAudit) {
	title := u.Name
	if len(u.Wargear) > 0 {
		title += " (" + strings.Join(u.Wargear, ", ") + ")"
	}
	m.line("")
	m.line("### %s", title)
	m.line("")
	m.line("From: %s.", strings.Join(u.Sources, "; "))
	if u.Base == "" {
		m.line("")
		m.line("**Does not resolve to a vanilla profile.**")
		return
	}
	m.line("")
	if u.Alias != nil {
		m.line("Company label for %s, rules added [%s], options selected [%s].", u.Alias.Unit, strings.Join(u.Alias.AddRules, ", "), strings.Join(u.Alias.Selected, ", "))
		m.line("")
	}
	v := u.Vanilla
	m.line("Vanilla %s, %d points, types %s, race %s, faction %s.", v.Name, v.Points, strings.Join(v.UnitTypes, ", "), strings.Join(v.Race, ", "), strings.Join(v.Faction, ", "))
	m.line("")
	m.line("Profile: %s", auditStats(u.Stats))
	m.line("")
	m.line("Special rules: %s. Heroic actions: %s.", strings.Join(v.SpecialRules, ", "), strings.Join(v.HeroicActions, ", "))
	m.line("")
	m.line("Vanilla wargear: %s.", strings.Join(v.Wargear, ", "))
	m.line("")
	if len(v.Options) > 0 {
		m.head("Vanilla option", "Points", "Buyable in this armoury", "Reserved")
		for _, o := range v.Options {
			buy := false
			for _, g := range c.Armoury {
				if GearMatchesOption(g.GearOffer, o.Name) {
					buy = true
				}
			}
			m.row(o.Name, fmt.Sprint(o.Points), yesNo(buy), "")
		}
		m.line("")
	}
	var eq []string
	for _, e := range u.Warrior.Equipment {
		tag := ""
		switch {
		case e.Upgrade:
			tag = " (upgrade)"
		case e.Profile:
			tag = " (profile)"
		}
		eq = append(eq, e.Name+tag)
	}
	m.line("Built warrior: %d points. Equipment: %s.", u.Warrior.Points, strings.Join(eq, ", "))
	m.line("")
	var rules []string
	for _, r := range u.Warrior.Rules {
		rules = append(rules, fmt.Sprintf("%s (%s)", r.Name, r.Source))
	}
	m.line("Built rules: %s.", strings.Join(rules, ", "))
	m.line("")
	m.line("May lead: %s. On its own path: %s. Hero upgrades: %s.", yesNo(u.CanLead), yesNo(u.OwnPath), upgradeNames(u.Upgrades))
	for _, a := range u.Promotes {
		var to []string
		for _, t := range a.To {
			to = append(to, t.Unit)
		}
		m.line("")
		m.line("Promotes to: %s.", strings.Join(to, " or "))
	}
	m.line("")
	m.line("Armoury offers (influence price; derived by = the option that explains the offer):")
	m.line("")
	m.head("Item", "Size", "Influence", "Gold W/H", "Warrior", "Hero", "Warrior derived by", "Hero derived by")
	var refused []string
	for _, o := range u.Offers {
		if isRule(o.WarriorReason) && isRule(o.HeroReason) {
			refused = append(refused, fmt.Sprintf("%s (%s / %s)", o.Item.Name, o.WarriorReason, o.HeroReason))
			continue
		}
		m.row(o.Item.Name, o.Item.Size, fmt.Sprint(o.Item.Cost), fmt.Sprintf("%d/%d", o.WarriorGold, o.HeroGold), reasonOrBuy(o.WarriorReason), reasonOrBuy(o.HeroReason), o.DerivedByWarrior, o.DerivedByHero)
	}
	sort.Strings(refused)
	m.line("")
	m.line("Refused to both by rule (%d): %s.", len(refused), strings.Join(refused, "; "))
}

func isRule(reason string) bool {
	if reason == "" {
		return false
	}
	for _, k := range []string{"Not an option", "only", "Already carried"} {
		if strings.Contains(reason, k) {
			return true
		}
	}
	return false
}

func reasonOrBuy(r string) string {
	if r == "" {
		return "can buy"
	}
	return r
}

func upgradeNames(us []HeroUpgrade) string {
	var out []string
	for _, u := range us {
		out = append(out, u.Name)
	}
	return strings.Join(out, ", ")
}
