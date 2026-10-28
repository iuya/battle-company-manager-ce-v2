package core

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The behaviour-freezing golden snapshot (docs/testing.md, "Golden snapshot"; docs/bcjson-audit-backend.md section 3).
//
// It renders everything the app derives from the Battle Companies file as one deterministic text file per company plus a few
// global files (testdata/snapshot/), and fails on any difference. It reads only what the core exposes (BCBook, CompanyProfile,
// AuditRules, BuildDetail, the progression and campaign tables), never the json, so that the file format can change and the
// goldens must stay as they are. A deliberate change is made with `go test ./internal/core -run Snapshot -update-snapshot`
// and reviewed as a diff of testdata/snapshot/.
//
// The few seams that are not exported API are collected in the seam block below, so a rewrite of the loader touches one place.
var updateSnapshot = flag.Bool("update-snapshot", false, "rewrite testdata/snapshot/")

const snapshotDir = "testdata/snapshot"

// ---- seams: unexported helpers of core used to reach behaviour that has no exported entry point yet ----

func seamResolveUnit(rs *RuleSet, p CompanyProfile, name string) (VanillaUnit, *UnitAlias, bool) {
	return resolveUnit(rs, p, name)
}
func seamRuleTip(rs *RuleSet, name string) TipAudit { return ruleTip(rs, name) }
func seamBuildUnit(rs *RuleSet, c BCCompany, st UnitState) CompanyUnit {
	return buildUnit(rs, c, st, CampaignSettings{})
}

// seamPromote applies the promotion rule for gear: what a promoted warrior still carries.
func seamPromote(rs *RuleSet, c BCCompany, st UnitState, newBase string, given []string) []string {
	us := st.Clone()
	dropUnlisted(&levelEnv{rs: rs, bcC: c}, &us, newBase, given)
	return us.Gear
}
func seamPromotionChoices(rs *RuleSet, c BCCompany, u CompanyUnit) []LevelChoice {
	return promotionChoices(rs, c, u)
}
func seamPromotedName(rs *RuleSet, c BCCompany, to AdvancementTarget) string {
	return promotedName(rs, c, to)
}
func seamSheet(u CompanyUnit) HeroSheet     { return sheetOf(u, &Progress{}, nil, nil) }
func seamLevelLists(rs *RuleSet) LevelLists { return levelLists(rs) }
func seamImprovedStat(r ProgressionRules, p PathProgression, s HeroSheet, stat string, amount int) (int, bool) {
	return r.improvedStat(p, s, stat, amount, 0)
}

// ---- the dumper: a deterministic text form of any exported value ----

type dumper struct {
	skip map[string]bool // field names left out
	sb   strings.Builder
}

func (d *dumper) compact(v reflect.Value, depth int) (string, bool) {
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "nil", true
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return strconv.Quote(v.String()), true
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), true
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64), true
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return "", true
	}
	if depth > 3 {
		return "", false
	}
	var parts []string
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			s, ok := d.compact(v.Index(i), depth+1)
			if !ok {
				return "", false
			}
			parts = append(parts, s)
		}
		out := "[" + strings.Join(parts, ", ") + "]"
		return out, len(out) <= 140
	case reflect.Map:
		for _, k := range sortedKeys(v) {
			ks, _ := d.compact(k, depth+1)
			s, ok := d.compact(v.MapIndex(k), depth+1)
			if !ok {
				return "", false
			}
			parts = append(parts, ks+": "+s)
		}
		out := "{" + strings.Join(parts, ", ") + "}"
		return out, len(out) <= 140
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || d.skip[f.Name] || v.Field(i).IsZero() {
				continue
			}
			s, ok := d.compact(v.Field(i), depth+1)
			if !ok {
				return "", false
			}
			if s == "" {
				continue
			}
			parts = append(parts, f.Name+": "+s)
		}
		out := "{" + strings.Join(parts, ", ") + "}"
		return out, len(out) <= 140
	}
	return "", false
}

func sortedKeys(m reflect.Value) []reflect.Value {
	keys := m.MapKeys()
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface()) })
	return keys
}

// put writes value v under a label, nested by two spaces per level.
func (d *dumper) put(indent int, label string, v reflect.Value) {
	pad := strings.Repeat("  ", indent)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			d.sb.WriteString(pad + label + " nil\n")
			return
		}
		v = v.Elem()
	}
	if s, ok := d.compact(v, 0); ok {
		if s != "" {
			d.sb.WriteString(pad + label + " " + s + "\n")
		}
		return
	}
	d.sb.WriteString(pad + label + "\n")
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			d.put(indent+1, "-", v.Index(i))
		}
	case reflect.Map:
		for _, k := range sortedKeys(v) {
			ks, _ := d.compact(k, 0)
			d.put(indent+1, ks+":", v.MapIndex(k))
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || d.skip[f.Name] || v.Field(i).IsZero() {
				continue
			}
			d.put(indent+1, f.Name+":", v.Field(i))
		}
	}
}

// snapshotUnpinned are the fields slice P4 added (the dice of the rules, docs/definitions-lean-design.md 12.1): the goldens pin the
// behaviour of the shipped data and stay byte-identical, the dice are tested in dice_test.go and TestCustomRuleset.
var snapshotUnpinned = []string{"InjuryDice", "WarriorDice", "HeroDice", "HeroRolls", "HeroChoose", "PathDice", "CourageSpec", "ScenarioDie", "BonusSpec", "RuleDice", "FirstDie", "TableDice",
	// slices P2 and P3: the officers, the hero start, the level threshold of a unit and the classes of a limit
	"Leaders", "Sergeants", "HeroStart", "LevelEvery", "TreatDice", "LimitClasses", "LimitKeyword", "CITotalCap", "CITotalGear",
	// slice P5: the amounts of the no-promotion improvement, the break point comparison, the stats an injury row gains
	"NoPromotionBy", "Comparison", "StatGains", "GivesRerolls", "RerollRating", "RerollCount",
	// the equipment slots, the slots an item adds and the race it is restricted to
	"EquipmentSlots", "GrantsSlots", "BaseSlots", "OnlyRace"}

// dump renders a value (a struct, slice or map) under a heading.
func dump(heading string, v any, skip ...string) string {
	d := &dumper{skip: map[string]bool{}}
	for _, s := range snapshotUnpinned {
		d.skip[s] = true
	}
	for _, s := range skip {
		d.skip[s] = true
	}
	d.put(0, heading, reflect.ValueOf(v))
	return d.sb.String()
}

// ---- sections ----

type snap struct {
	strings.Builder
}

func (s *snap) h(format string, args ...any) { fmt.Fprintf(s, "\n=== "+format+" ===\n", args...) }
func (s *snap) l(format string, args ...any) { fmt.Fprintf(s, format+"\n", args...) }

func joinS(xs []string) string { return strings.Join(xs, ", ") }

func yn(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func tipDigest(text string) string {
	if text == "" {
		return "-"
	}
	return fmt.Sprintf("%x", sha1.Sum([]byte(text)))[:8]
}

// snapshotFiles renders every golden file of a rule set, by file name.
func snapshotFiles(t *testing.T, rs *RuleSet) map[string]string {
	t.Helper()
	files := map[string]string{}
	report := AuditRules(rs, AuditInput{})
	profiles := rs.BC.Profiles()
	if len(profiles) < 37 {
		t.Fatalf("%d companies, want at least 37", len(profiles))
	}

	// the common armoury must be the same for every company: it is rendered once
	var common []GearOffer
	tips := map[string]string{} // digest -> text
	sets := map[string]string{} // digest -> equipment offered
	universe := map[string]string{}
	rules := map[string]bool{}
	for i, c := range report.Companies {
		bc, _ := rs.BC.Company(c.Name)
		all := rs.BC.Armoury(bc)
		cm := all[:len(all)-len(bc.UniqueWargear)]
		if i == 0 {
			common = cm
		} else if !reflect.DeepEqual(common, cm) {
			t.Errorf("%s: the common armoury differs from the first company's", c.Name)
		}
		for _, u := range c.Units {
			if u.Base == "" {
				continue
			}
			kw := rs.UnitKeywords(c.Profile, UnitKeywordsInput{Type: u.Name})
			var names []string
			for _, k := range kw.Model {
				names = append(names, k.Name)
			}
			universe[u.Name+" | "+joinS(names)] = joinS(names)
			for _, r := range u.Vanilla.SpecialRules {
				rules[r] = true
			}
			for _, r := range u.Warrior.Rules {
				rules[r.Name] = true
			}
		}
	}

	for _, c := range report.Companies {
		files[companyFileName(c.Name)] = snapshotCompany(t, rs, c, tips, sets)
	}

	files["_index.txt"] = snapshotIndex(t, rs, report)
	files["_armoury.txt"] = snapshotArmoury(rs, common)
	files["_progression.txt"] = snapshotProgression(t, rs, report)
	files["_campaign.txt"] = snapshotCampaign(t, rs)
	files["_ruletargets.txt"] = snapshotRuleTargets(rs, universe, rules)
	var th strings.Builder
	var keys []string
	for k := range tips {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	th.WriteString("Tooltip texts by digest (the companies' files cite the digest).\n")
	for _, k := range keys {
		fmt.Fprintf(&th, "%s %s\n", k, strconv.Quote(tips[k]))
	}
	files["_tooltips.txt"] = th.String()
	var sb strings.Builder
	sb.WriteString("Equipment a unit can buy, and what is refused to it for a rule (the units' files cite the digest).\n")
	keys = keys[:0]
	for k := range sets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s %s\n", k, sets[k])
	}
	files["_offersets.txt"] = sb.String()
	return files
}

func companyFileName(company string) string {
	return strings.TrimSuffix(AuditFileName(company), ".md") + ".txt"
}

func snapshotIndex(t *testing.T, rs *RuleSet, r AuditReport) string {
	var s snap
	s.l("Companies of the rule set, with their starting gold and points.")
	for _, c := range r.Companies {
		p := c.Profile
		pts, models := startingPoints(rs, p, p.DefaultHeroes())
		s.l("%s | %s | gold %d | points %d (default officers) | total %d | models %d | break point %d%% | max models %d | reinforcement %d | flags %d",
			c.Name, c.Alignment, p.StartGold, pts, pts+p.StartGold, models, c.Detail.BreakPointPercent, p.MaxModels, p.ReinforcementCost, len(c.Flags))
	}
	return s.String()
}

func startingPoints(rs *RuleSet, p CompanyProfile, heroes []StartHero) (points, models int) {
	st := CompanyState{Company: CampaignCompany{ID: 1, Name: p.Name, Faction: p.Name}, Influence: rs.BC.Recruiting().StartingInfluence, Gold: p.StartGold, Units: StartingUnits(p, heroes)}
	d := BuildDetail(rs, CampaignSettings{}, st)
	return d.Points, d.ModelCount()
}

func snapshotArmoury(rs *RuleSet, common []GearOffer) string {
	var s snap
	s.l("The common armoury every company sells (the company's own unique wargear is in its file), with the gear annotation of each item.")
	for _, o := range common {
		s.l("")
		s.WriteString(dump("item "+strconv.Quote(o.Name), o))
		if a, ok := rs.BC.Annotation(o.Name); ok {
			s.WriteString(dump("annotation", a))
		} else {
			s.l("annotation none")
		}
	}
	return s.String()
}

// snapshotCompany renders everything derived for one company.
func snapshotCompany(t *testing.T, rs *RuleSet, c CompanyAudit, tips, sets map[string]string) string {
	var s snap
	p := c.Profile
	bc, _ := rs.BC.Company(c.Name)
	s.l("# %s (%s)", c.Name, c.Alignment)

	s.h("anomalies (%d)", len(c.Flags))
	for _, f := range c.Flags {
		s.l("%s | %s | %s", f.Kind, f.Subject, f.Detail)
	}

	s.h("profile (everything the company page holds)")
	s.WriteString(dump("profile", p, "Previews", "Advancements", "Paths", "Powers"))
	pr := rs.BC.Progression()
	for _, pa := range p.Paths {
		g, _ := pr.Path(pa.Name)
		note := "chart as in _progression.txt"
		same := pa
		same.LimitPerCompany, same.StartAction = g.LimitPerCompany, g.StartAction
		if !reflect.DeepEqual(same, g) {
			note = "differs from _progression.txt:\n" + dump("path", pa)
		}
		s.l("path %q: limit per company %d, start action %q, start power %s; %s", pa.Name, pa.LimitPerCompany, pa.StartAction, yn(pa.StartPower), note)
	}
	for _, pw := range p.Powers {
		s.l("starting power %s = %s", pw.Key, pw.Label)
	}

	s.h("start: gold, points and officer choices")
	pts, models := startingPoints(rs, p, p.DefaultHeroes())
	s.l("starting influence %d, gold %d", rs.BC.Recruiting().StartingInfluence, p.StartGold)
	s.l("default officers %v: %d models, %d points, total %d", p.DefaultHeroes(), models, pts, pts+p.StartGold)
	totals := map[int]int{}
	var cand []int
	for i, su := range p.StartList() {
		if p.CanLeadStart(su) {
			cand = append(cand, i)
		}
	}
	allowed, refused := 0, 0
	for _, l := range cand {
		for a := 0; a < len(cand); a++ {
			for b := a + 1; b < len(cand); b++ {
				if cand[a] == l || cand[b] == l {
					continue
				}
				h := []StartHero{{l, RoleLeader}, {cand[a], RoleSergeant}, {cand[b], RoleSergeant}}
				if _, err := p.ValidateStart(h); err != nil {
					refused++
					continue
				}
				allowed++
				pt, _ := startingPoints(rs, p, h)
				totals[pt+p.StartGold]++
			}
		}
	}
	var tk []int
	for k := range totals {
		tk = append(tk, k)
	}
	sort.Ints(tk)
	s.l("officer choices: %d allowed, %d refused; totals (points + gold) of the allowed ones:", allowed, refused)
	for _, k := range tk {
		s.l("  total %d: %d choices", k, totals[k])
	}
	d := c.Detail
	s.l("company at creation: %d points, max units %d, break point %d%%, limits:", d.Points, d.MaxUnits, d.BreakPointPercent)
	for _, l := range d.Limits {
		s.l("  limit %s %q used %d max %d next %d", l.ID, l.Label, l.Used, l.Max, l.Next)
	}

	s.h("armoury of the company: unique wargear")
	for _, o := range bc.UniqueWargear {
		s.WriteString(dump("unique "+strconv.Quote(o.Name), o))
	}
	s.l("armoury size %d (common %d + unique %d)", len(c.Armoury), len(c.Armoury)-len(bc.UniqueWargear), len(bc.UniqueWargear))

	s.h("units")
	for _, u := range c.Units {
		snapshotUnit(&s, rs, c, u, sets)
	}

	s.h("hero upgrades and who may take them")
	for _, h := range p.HeroUpgrades {
		var who []string
		for _, u := range c.Units {
			if u.Base != "" && h.Allows(u.Vanilla) {
				who = append(who, u.Name)
			}
		}
		slices.Sort(who)
		who = slices.Compact(who)
		s.l("%s (rule %q, restriction %q, units %v): allowed to %d of %d units: %s", h.Name, h.Rule, h.Restriction, h.Units, len(who), len(c.Units), joinS(who))
	}

	s.h("promotion: matching, choices and gear kept")
	snapshotPromotion(&s, rs, bc, p)

	s.h("tooltips and glossary terms (digest of the text in _tooltips.txt)")
	for _, tp := range c.Tips {
		dg := tipDigest(tp.Text)
		if dg != "-" {
			tips[dg] = tp.Text
		}
		s.l("%s | %s | %s | %s | %s", tp.Name, tp.Kind, tp.Source, tp.Term, dg)
	}

	s.h("duplicated vanilla names the company uses")
	for _, dpl := range c.Duplicates {
		s.l("%s", dpl)
	}

	s.h("name resolution: what every name the company mentions resolves to")
	snapshotNames(&s, rs, bc, c)
	return s.String()
}

func snapshotUnit(s *snap, rs *RuleSet, c CompanyAudit, u UnitAudit, sets map[string]string) {
	title := u.Name
	if len(u.Wargear) > 0 {
		title += " (" + joinS(u.Wargear) + ")"
	}
	s.l("")
	s.l("-- %s", title)
	s.l("from: %s", strings.Join(u.Sources, "; "))
	if u.Base == "" {
		s.l("DOES NOT RESOLVE to a vanilla profile")
		return
	}
	if u.Alias != nil {
		s.WriteString(dump("alias", *u.Alias))
	}
	v := u.Vanilla
	s.l("vanilla %s: %d points, types %s, race %s, faction %s", v.Name, v.Points, joinS(v.UnitTypes), joinS(v.Race), joinS(v.Faction))
	s.l("profile: %s", auditStats(u.Stats))
	s.l("rules: %s | actions: %s | wargear: %s", joinS(v.SpecialRules), joinS(v.HeroicActions), joinS(v.Wargear))
	kw := rs.UnitKeywords(c.Profile, UnitKeywordsInput{Type: u.Name})
	var ks []string
	for _, k := range kw.Model {
		ks = append(ks, fmt.Sprintf("%s/%s/%s", k.Name, k.Kind, k.Source))
	}
	s.l("keywords: %s", joinS(ks))
	for _, o := range v.Options {
		var tags []string
		if rs.BC.Reserved(v.Name, o.Name) {
			tags = append(tags, "reserved")
		}
		if why, ok := rs.BC.Unsold(v.Name, o.Name); ok {
			tags = append(tags, "unsold: "+why)
		}
		if item, ok := rs.BC.ReadAs(v.Name, o.Name); ok {
			tags = append(tags, "read as "+item)
		}
		buy := false
		for _, g := range c.Armoury {
			if GearMatchesOption(g.GearOffer, o.Name) {
				buy = true
			}
		}
		s.l("option %q %d pts, buyable %s%s", o.Name, o.Points, yn(buy), prefixed(" | ", tags))
	}
	var choosable []string
	for _, o := range rs.BC.ChoosableOptions(v.Name, v.Options) {
		choosable = append(choosable, o.Name)
	}
	s.l("choosable at recruit: %s", joinS(choosable))
	for _, side := range []struct {
		label string
		unit  CompanyUnit
	}{{"warrior", u.Warrior}, {"hero", u.Hero}} {
		var eq, rl []string
		for _, e := range side.unit.Equipment {
			tag := ""
			switch {
			case e.Upgrade:
				tag = " (upgrade)"
			case e.Profile:
				tag = " (profile)"
			}
			eq = append(eq, e.Name+tag)
		}
		for _, r := range side.unit.Rules {
			rl = append(rl, fmt.Sprintf("%s (%s)", r.Name, r.Source))
		}
		s.l("built %s: %d points | equipment: %s | rules: %s", side.label, side.unit.Points, joinS(eq), joinS(rl))
	}
	s.l("may lead %s | own path %s | hero upgrades: %s", yn(u.CanLead), yn(u.OwnPath), upgradeNames(u.Upgrades))
	for _, a := range u.Promotes {
		var to []string
		for _, tg := range a.To {
			to = append(to, tg.Unit)
		}
		s.l("promotes to: %s", strings.Join(to, " or "))
	}
	// offers: what the unit can buy as warrior and as hero (equipment, which every unit shares, is kept apart in _offersets.txt)
	for _, role := range []string{"warrior", "hero"} {
		var wargear, equipment, reasons []string
		refusedByProfile := 0
		for _, o := range u.Offers {
			it := o.Item
			reason, gold, own, via := o.WarriorReason, o.WarriorGold, it.Gold.Warrior, o.DerivedByWarrior
			if role == "hero" {
				reason, gold, own, via = o.HeroReason, o.HeroGold, it.Gold.Hero, o.DerivedByHero
			}
			switch {
			case reason == "" && it.Size == "":
				txt := it.Name
				if gold != own {
					txt += fmt.Sprintf(" (gold %d, item %d)", gold, own)
				}
				wargear = append(wargear, txt+prefixed(" via ", []string{via}))
			case reason == "":
				txt := it.Name
				if gold != own {
					txt += fmt.Sprintf(" (gold %d, item %d)", gold, own)
				}
				equipment = append(equipment, txt+prefixed(" via ", []string{via}))
			case reason == "Not an option of its profile":
				refusedByProfile++
			default:
				reasons = append(reasons, it.Name+": "+reason)
			}
		}
		s.l("%s can buy wargear (%d): %s", role, len(wargear), strings.Join(wargear, "; "))
		set := strings.Join(equipment, "; ")
		sets[tipDigest(set)] = set
		s.l("%s can buy equipment (%d): set %s", role, len(equipment), tipDigest(set))
		rj := strings.Join(reasons, "; ")
		sets[tipDigest(rj)] = rj
		s.l("%s refused: %d not an option of the profile; %d for other reasons: set %s", role, refusedByProfile, len(reasons), tipDigest(rj))
	}
}

func prefixed(p string, xs []string) string {
	var keep []string
	for _, x := range xs {
		if x != "" {
			keep = append(keep, x)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	return p + strings.Join(keep, "; ")
}

func snapshotPromotion(s *snap, rs *RuleSet, c BCCompany, p CompanyProfile) {
	for i, adv := range rs.BC.Advancements(c) {
		s.l("")
		s.l("-- advancement %d", i)
		s.WriteString(dump("advancement", adv))
		items := sortedGear(gearOfUnit(rs, c, adv.FromUnit))
		s.l("gear a %s may carry (%d): %s", adv.FromUnit, len(items), joinS(items))
		combos := [][]string{nil}
		for _, it := range items {
			combos = append(combos, []string{it})
		}
		if len(items) > 1 {
			combos = append(combos, items)
		}
		for _, combo := range combos {
			label := "carrying " + joinS(combo)
			if combo == nil {
				label = "carrying nothing"
			}
			if len(combo) == len(items) && len(items) > 1 {
				label = "carrying everything"
			}
			var w, h CompanyUnit
			for _, role := range []UnitRole{RoleWarrior, RoleHero} {
				st := UnitState{ID: 1, Base: adv.FromUnit, Role: role, Gear: slices.Clone(combo), Bought: slices.Clone(combo)}
				if role == RoleWarrior {
					w = seamBuildUnit(rs, c, st)
				} else {
					h = seamBuildUnit(rs, c, st)
				}
			}
			var choices []string
			for _, ch := range seamPromotionChoices(rs, c, w) {
				choices = append(choices, ch.Key+"="+ch.Label)
			}
			var hchoices []string
			for _, ch := range seamPromotionChoices(rs, c, h) {
				hchoices = append(hchoices, ch.Key+"="+ch.Label)
			}
			s.l("%s: matches warrior %s hero %s | choices warrior: %s | hero: %s", label, yn(adv.Matches(w)), yn(adv.Matches(h)), joinS(choices), joinS(hchoices))
			if !adv.Matches(w) {
				continue
			}
			for _, to := range adv.To {
				st := UnitState{ID: 1, Base: adv.FromUnit, Role: RoleWarrior, Gear: slices.Clone(combo), Bought: slices.Clone(combo)}
				base := seamPromotedName(rs, c, to)
				kept := seamPromote(rs, c, st, base, to.Wargear)
				var dropped []string
				for _, g := range combo {
					if !slices.Contains(kept, g) {
						dropped = append(dropped, g)
					}
				}
				s.l("    to %s (given %s): warrior keeps [%s] drops [%s]; a hero keeps everything", base, joinS(to.Wargear), joinS(kept), joinS(dropped))
			}
		}
	}
	for _, hp := range p.Promotions {
		s.l("hero promotion %+v", hp)
	}
}

func sortedGear(m map[string]string) []string {
	var out []string
	for g := range m {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// snapshotNames lists every name the company mentions and what it resolves to.
func snapshotNames(s *snap, rs *RuleSet, bc BCCompany, c CompanyAudit) {
	p := c.Profile
	unitNames, gearNames, ruleNames := map[string]bool{}, map[string]bool{}, map[string]bool{}
	addU := func(xs ...string) {
		for _, x := range xs {
			if x = strings.TrimSpace(x); x != "" {
				unitNames[x] = true
			}
		}
	}
	addG := func(xs ...string) {
		for _, x := range xs {
			for _, part := range append([]string{x}, OptionItems(x)...) {
				if part = strings.TrimSpace(part); part != "" {
					gearNames[part] = true
				}
			}
		}
	}
	addR := func(xs ...string) {
		for _, x := range xs {
			if x = strings.TrimSpace(x); x != "" {
				ruleNames[x] = true
			}
		}
	}
	for _, u := range c.Units {
		addU(u.Name)
		addG(u.Wargear...)
		addG(u.Vanilla.Wargear...)
		addR(u.Vanilla.SpecialRules...)
		addR(u.Vanilla.HeroicActions...)
		for _, o := range u.Vanilla.Options {
			addG(o.Name)
		}
		for _, r := range u.Warrior.Rules {
			addR(r.Name)
		}
		for _, e := range u.Warrior.Equipment {
			addG(e.Name)
		}
	}
	for _, su := range p.Start {
		addU(su.Unit)
		addG(su.Wargear...)
	}
	for _, rows := range [][]ChartRow{p.Reinforcements, p.SpecialChart} {
		for _, r := range rows {
			addU(r.Result.Unit, r.Result.Alt)
			addG(r.Result.Wargear...)
			for _, o := range r.Result.Options {
				addU(o.Unit, o.Label)
				addG(o.Selected...)
			}
		}
	}
	for _, su := range p.SpecialUnits {
		addU(su.Unit)
	}
	for _, a := range p.Aliases {
		addU(a.Label, a.Unit)
		addG(a.Selected...)
		addR(a.AddRules...)
	}
	for _, a := range rs.BC.Advancements(bc) {
		addU(a.FromUnit)
		addG(a.FromWargear...)
		for _, t := range a.To {
			addU(t.Unit, t.Label)
			addG(t.Wargear...)
		}
	}
	for _, hp := range p.Promotions {
		addU(hp.From, hp.Becomes)
		addG(hp.Keep...)
		addG(hp.Drop...)
	}
	for _, f := range p.Fallbacks {
		addU(f.Unit)
	}
	addU(p.OwnPathUnits...)
	addU(p.NoStartGain...)
	addU(p.OnTheirOwnPath...)
	addU(p.StartHeroes.Allowed...)
	addU(p.StartHeroes.Excluded...)
	for k := range p.Types {
		addU(k)
	}
	for k, cm := range p.Composites {
		addU(k)
		for _, part := range cm.Parts {
			addU(part.Name)
			addG(part.Wargear...)
		}
		addR(cm.Rules...)
	}
	for k, ms := range p.IncludedPoints {
		addU(k)
		for _, m := range ms {
			addU(m.Name)
		}
	}
	for k := range p.ModelCounts {
		addU(k)
	}
	for _, h := range p.HeroUpgrades {
		addR(h.Rule)
		addU(h.Units...)
	}
	for _, g := range p.Granted {
		addR(g.Rule)
	}
	for _, e := range p.Exchanges {
		addR(e.Remove, e.Add)
	}
	for _, o := range bc.UniqueWargear {
		addG(o.Name)
		addR(o.Grants...)
		addU(o.OnlyUnits...)
	}
	for _, o := range c.Armoury {
		addR(o.Grants...)
	}
	for _, pa := range p.Paths {
		addR(pa.SpecialRules...)
		addR(pa.StartAction)
	}

	for _, n := range sortedSet(unitNames) {
		v, alias, ok := seamResolveUnit(rs, p, n)
		switch {
		case !ok:
			s.l("unit %q -> unresolved", n)
		case alias != nil:
			s.l("unit %q -> label of %q (rules +[%s] selected [%s])", n, v.Name, joinS(alias.AddRules), joinS(alias.Selected))
		default:
			s.l("unit %q -> vanilla %q", n, v.Name)
		}
	}
	for _, n := range sortedSet(gearNames) {
		var parts []string
		if o, ok := rs.BC.Offer(bc, n); ok {
			parts = append(parts, fmt.Sprintf("offer %q size %q also %v", o.Name, o.Size, o.AlsoCalled))
		}
		if a, ok := rs.BC.Annotation(n); ok {
			parts = append(parts, fmt.Sprintf("annotation %q", a.Name))
		}
		if g, ok := rs.Vanilla.Gear(n); ok {
			parts = append(parts, fmt.Sprintf("vanilla gear %q", g.Name))
		}
		if len(parts) == 0 {
			parts = append(parts, "unknown")
		}
		s.l("gear %q -> key %q | %s", n, ItemKey(n), strings.Join(parts, " | "))
	}
	for _, n := range sortedSet(ruleNames) {
		tp := seamRuleTip(rs, n)
		s.l("rule %q -> label %q | text from %s | glossary %q", n, rs.BC.RuleLabel(n), tp.Source, tp.Term)
	}
}

func sortedSet(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// snapshotProgression renders the level-up tables, hero upgrades' limits and what each path offers a fresh hero.
func snapshotProgression(t *testing.T, rs *RuleSet, report AuditReport) string {
	var s snap
	pr := rs.BC.Progression()
	s.h("progression tables")
	s.WriteString(dump("progression", pr))
	s.h("warrior table by D6")
	for d := 1; d <= 6; d++ {
		w, ok := pr.WarriorFor(d)
		s.l("D6 %d -> %s (%s) ok=%s", d, w.Key, w.Name, yn(ok))
	}
	// a canonical hero: the first hero-capable start unit of the first company, on each path with every rule unlocked
	first := report.Companies[0]
	bc, _ := rs.BC.Company(first.Name)
	var base string
	for _, su := range first.Profile.StartList() {
		if first.Profile.CanLeadStart(su) {
			base = su.Unit
			break
		}
	}
	s.l("")
	s.l("canonical hero: %s of %s", base, first.Name)
	lists := seamLevelLists(rs)
	for _, path := range slices.Concat(pr.Paths, pr.Hidden) {
		s.h("path %s: what each 2D6 result offers a fresh hero", path.Name)
		hero := pathHero(rs, bc, base, path, nil)
		sheet := seamSheet(hero)
		for sum := 2; sum <= 12; sum++ {
			offer, ok := pr.OfferFor(sheet, lists, sum)
			var ch []string
			for _, c := range offer.Choices {
				ch = append(ch, fmt.Sprintf("%s=%s", c.Key, c.Label))
				if c.RollAgain {
					ch[len(ch)-1] += "(+again)"
				}
				if c.Group != "" {
					ch[len(ch)-1] += "{" + c.Group + "}"
				}
			}
			sort.Strings(ch) // the order of the cast choices follows a map in the rules code (bead filed); the set is what is frozen
			s.l("%2d ok=%s %q: %s", sum, yn(ok), offer.Text, strings.Join(ch, "; "))
		}
		s.l("starting stats %v rules %v actions %v powers %v", sheet.Stats, sortedSet(sheet.Rules), sortedSet(sheet.Actions), sheet.Powers)
		var stats []string
		for st := range sheet.Stats {
			stats = append(stats, st)
		}
		sort.Strings(stats)
		s.l("raising a stat by 1 or 2 from value v (boost 0 / boost 2), '-' when it cannot be raised:")
		for _, st := range stats {
			var row []string
			for v := 1; v <= 10; v++ {
				var cell []string
				for _, boost := range []int{0, 2} {
					sh := sheet
					sh.Stats = map[string]SheetStat{st: {Value: v, Boost: boost}}
					a, ok1 := seamImprovedStat(pr, path, sh, st, 1)
					b, ok2 := seamImprovedStat(pr, path, sh, st, 2)
					f := func(n int, ok bool) string {
						if !ok {
							return "-"
						}
						return strconv.Itoa(n)
					}
					cell = append(cell, f(a, ok1)+"/"+f(b, ok2))
				}
				row = append(row, fmt.Sprintf("%d:%s", v, strings.Join(cell, ",")))
			}
			s.l("  %s %s", st, strings.Join(row, " "))
		}
	}
	return s.String()
}

// snapshotCampaign renders the campaign tables as the app uses them.
func snapshotCampaign(t *testing.T, rs *RuleSet) string {
	var s snap
	b := rs.BC
	s.h("injuries")
	s.WriteString(dump("injuries", b.Injuries()))
	s.h("aftermath: experience, influence, Against the Odds bonuses, injury charts, effects, scenario rewards")
	bcJSON := readDef(t, "battle_companies/battle_companies.json")
	after, err := ParseAftermath(bcJSON)
	if err != nil {
		t.Fatal(err)
	}
	s.WriteString(dump("aftermath", after))
	s.h("scenarios")
	scenarios, err := ParseScenarios(bcJSON)
	if err != nil {
		t.Fatal(err)
	}
	s.WriteString(dump("scenarios", scenarios))
	for _, sc := range scenarios {
		if got, ok := b.Scenario(sc.Name); !ok || !reflect.DeepEqual(got, sc) {
			t.Errorf("scenario %q is not found by name", sc.Name)
		}
	}
	s.h("campaign options")
	opts, err := ParseCampaignOptions(bcJSON)
	if err != nil {
		t.Fatal(err)
	}
	s.WriteString(dump("options", opts))
	s.h("Against the Odds")
	s.WriteString(dump("ato", b.ATO()))
	s.h("break point")
	s.WriteString(dump("break", b.BreakRules()))
	s.h("rating of a hero: what the rules add per point gained")
	for _, stat := range []string{"movement", "fight", "shoot", "strength", "defence", "attack", "wounds", "courage", "intelligence", "might", "will", "fate"} {
		var row []string
		for v := 0; v <= 12; v++ {
			row = append(row, strconv.Itoa(b.HeroRating(VanillaUnit{Points: 0}, map[string]int{stat: v}, 0, 0)))
		}
		s.l("%s 0..12: %s", stat, strings.Join(row, " "))
	}
	var row []string
	for n := 0; n <= 6; n++ {
		row = append(row, fmt.Sprintf("%d:%d/%d", n, b.HeroRating(VanillaUnit{}, nil, n, 0), b.HeroRating(VanillaUnit{}, nil, 0, n)))
	}
	s.l("special / minor rules 0..6: %s", strings.Join(row, " "))
	s.h("creatures")
	cr, co := b.Creatures()
	// the company-wide creature count is the consequence of one creature per officer (the rules file has no such limit)
	s.WriteString(dump("rules", struct {
		AttachTo                  []string
		MaxPerHero, MaxPerCompany int
	}{cr.AttachTo, cr.MaxPerHero, cr.MaxPerHero * b.Shape().Officers()}))
	s.WriteString(dump("offers", co))
	s.h("wanderers")
	wr, wo := b.Wanderers()
	s.WriteString(dump("rules", wr))
	s.WriteString(dump("offers", wo))
	s.h("armour upgrade")
	s.WriteString(dump("armourUpgrade", b.ArmourUpgrade()))
	return s.String()
}

// snapshotRuleTargets renders the rule-target table and, for every rule written on a unit, who it works against, expanded over
// the keywords of every unit of every company.
func snapshotRuleTargets(rs *RuleSet, universe map[string]string, ruleSet map[string]bool) string {
	var s snap
	rt := rs.BC.RuleTargets()
	s.h("rule targets table")
	s.WriteString(dump("rows", rt.Rows()))
	s.h("rules in use, who they work against, and the units (name | keywords) they name")
	var units []string
	for u := range universe {
		units = append(units, u)
	}
	sort.Strings(units)
	for _, r := range sortedSet(ruleSet) {
		w := rt.Describe(r)
		if w.Works == "" && len(w.Against) == 0 && len(w.Needs) == 0 && !w.Friends {
			continue
		}
		var matched []string
		for _, u := range units {
			words := map[string]bool{}
			for _, k := range strings.Split(universe[u], ", ") {
				words[strings.ToLower(k)] = true
			}
			for _, alt := range w.Against {
				all := true
				for _, k := range alt {
					all = all && words[strings.ToLower(k)]
				}
				if all {
					matched = append(matched, strings.SplitN(u, " |", 2)[0])
					break
				}
			}
		}
		slices.Sort(matched)
		matched = slices.Compact(matched)
		s.l("%q: works %q against %v needs %v friends %s | %d unit names: %s", r, w.Works, w.Against, w.Needs, yn(w.Friends), len(matched), joinS(matched))
	}
	return s.String()
}

// ---- the test ----

func TestSnapshot(t *testing.T) {
	rs := shippedRules(t)
	got := snapshotFiles(t, rs)
	if *updateSnapshot {
		if err := os.RemoveAll(snapshotDir); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range got {
			if err := os.WriteFile(filepath.Join(snapshotDir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatalf("no golden snapshot (run with -update-snapshot): %v", err)
	}
	have := map[string]bool{}
	for _, e := range entries {
		have[e.Name()] = true
	}
	for name := range have {
		if _, ok := got[name]; !ok {
			t.Errorf("%s is in the golden but no longer rendered", name)
		}
	}
	names := make([]string, 0, len(got))
	for n := range got {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(snapshotDir, name))
			if err != nil {
				t.Fatalf("no golden file for %s (run with -update-snapshot)", name)
			}
			if string(want) != got[name] {
				t.Errorf("%s differs from the golden:\n%s\n(review the change; go test ./internal/core -run Snapshot -update-snapshot rewrites the goldens)", name, firstDiffs(string(want), got[name], 6))
			}
		})
	}
}

// firstDiffs shows the first lines that differ, as - golden / + now.
func firstDiffs(want, got string, max int) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var out []string
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			out = append(out, fmt.Sprintf("line %d:\n- %s\n+ %s", i+1, clip(a, 300), clip(b, 300)))
			if len(out) >= max {
				break
			}
		}
	}
	return strings.Join(out, "\n")
}
