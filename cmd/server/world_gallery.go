package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// bcDefs is the little of the Battle Companies file the script reads to aim its rolls: the special units of each company.
type bcDefs struct {
	Companies []struct {
		Name         string
		SpecialUnits []struct {
			Unit string
			Cost int
		}
	}
}

// shippedRules is the rule set of the files the app ships with, read once through the loader like the app does.
func (w *world) shippedRules() *core.RuleSet {
	if w.shipped != nil {
		return w.shipped
	}
	var files [3][]byte
	for i, name := range []string{"vanilla/data2024.json", "battle_companies/battle_companies.json", "glossary/mesbg_glossary.json"} {
		b, err := os.ReadFile("definitions/" + name)
		w.must("read "+name, err)
		files[i] = b
	}
	rs, err := core.LoadRuleSet(files[0], files[1], files[2], nil)
	w.must("load the shipped rules", err)
	w.shipped = rs
	return rs
}

func (w *world) defs() bcDefs {
	var d bcDefs
	for _, c := range w.shippedRules().Doc.Companies {
		dc := struct {
			Name         string
			SpecialUnits []struct {
				Unit string
				Cost int
			}
		}{Name: c.Name}
		if c.Special != nil {
			for _, b := range c.Special.Buy {
				dc.SpecialUnits = append(dc.SpecialUnits, struct {
					Unit string
					Cost int
				}{b.Unit, b.Influence})
			}
		}
		d.Companies = append(d.Companies, dc)
	}
	return d
}

// show prints the roster of a company in one line per unit, for the log of the script.
func (w *world) show(co *wco) {
	d := w.detail(co)
	fmt.Fprintf(w.out, "  %s (%s): %d models, %d influence, %d gold\n", co.name, d.Faction, d.ModelCount(), d.Influence, d.Gold)
	for _, l := range d.Limits {
		fmt.Fprintf(w.out, "    limit %s %d/%d\n", l.ID, l.Used, l.Max)
	}
	for _, cr := range d.Creatures {
		fmt.Fprintf(w.out, "    creature %s of %s\n", cr.Name, cr.OwnerName)
	}
	if d.Pack != nil {
		fmt.Fprintf(w.out, "    pack %d/%d\n", len(d.Pack.Items), d.Pack.Capacity)
	}
	for _, u := range d.Roster {
		var gear []string
		for _, e := range u.Equipment {
			gear = append(gear, e.Name)
		}
		path := ""
		if u.Path != nil {
			path = " " + u.Path.Name
		}
		var inj []string
		for _, i := range u.Injuries {
			inj = append(inj, fmt.Sprintf("%s×%d", i.Key, i.Count))
		}
		flags := ""
		if u.Injured {
			flags += " INJURED"
		}
		if u.PathPending {
			flags += " PATH?"
		}
		if len(inj) > 0 {
			flags += " " + strings.Join(inj, ",")
		}
		fmt.Fprintf(w.out, "    - %s [%s]%s L%d XP%d%s %v\n", u.Title(), u.Role, path, u.Level, u.XP, flags, gear)
	}
}

// buyAny buys the first n things the unit may buy with Gold, or influence when it has no Gold price; it says what it got.
func (w *world) buyAny(co *wco, u core.CompanyUnit, n int) { w.buyKind(co, u, n, false) }

// buyKind is buyAny limited to equipment that takes a slot (sized) or to wargear.
func (w *world) buyKind(co *wco, u core.CompanyUnit, n int, sized bool) {
	items, err := w.c().Armoury(w.ctx, co.camp.id, co.id, u.ID)
	if err != nil {
		return
	}
	for _, it := range items {
		if n == 0 {
			return
		}
		if !it.Buyable() || it.WeaponModifier || (it.Size != "") != sized {
			continue
		}
		how := ""
		if it.PayGold {
			how = "gold"
		}
		if err := w.c().BuyEquipment(w.ctx, co.user, co.camp.id, co.id, u.ID, it.Name, how); err == nil {
			n--
		}
	}
}

const galleryName = "Gallery: all 37 Battle Companies, a few games in"

// galleryCampaign makes one company of every Battle Company in a campaign: officers on different paths, a recruit on every
// row of its reinforcement chart (so every special case of the charts shows: choices, pairs, rare units, lower results), its
// special units and a piece of gear for its first officer.
func (w *world) galleryCampaign() {
	defs := w.defs()
	c := w.campaign("you", galleryName, "A company of each of the 37 factions, each with a recruit on every roll of its chart. Pick any company to see its special cases; the gallery accounts are <company>@example.com.", defaultSettings)
	profiles, err := w.c().CompanyProfiles(w.ctx, c.id)
	w.must("profiles", err)
	special := map[string]int{}
	cost := map[string]map[string]int{}
	for _, dc := range defs.Companies {
		cost[dc.Name] = map[string]int{}
		for _, su := range dc.SpecialUnits {
			cost[dc.Name][su.Unit] = su.Cost
			special[dc.Name]++
		}
	}
	for i, p := range profiles {
		email := slug(p.Name) + "@example.com"
		key := "g-" + slug(p.Name)
		w.account(key, email, p.Name+" player", core.RoleUser, false)
		co := w.company(c, key, p.Name+" Company", p.Name, false)
		w.choosePaths(co, i)
		w.grant(co, 30, 45)
		w.grant(co, 0, 30)
		for unit := range cost[p.Name] {
			_, err := w.c().RecruitSpecial(w.ctx, co.user, c.id, co.id, unit)
			w.try(p.Name+" buys a "+unit, err)
			w.finishChoice(co)
		}
		w.recruitAll(co)
		if h, ok := w.unit(co, isRole(core.RoleLeader)); ok {
			w.buyAny(co, h, 1)
		}
		w.show(co)
	}
}

// finishChoice answers a pending choice of options with the base profile.
func (w *world) finishChoice(co *wco) {
	for i := 0; i < 4; i++ {
		ch, err := w.c().PendingChoice(w.ctx, co.user, co.camp.id, co.id)
		if err != nil || ch == nil {
			return
		}
		opt := ""
		if len(ch.Options) > 0 {
			opt = ch.Options[0].Name
		}
		if _, err := w.c().ChooseOptions(w.ctx, co.user, co.camp.id, co.id, opt); err != nil {
			return
		}
	}
}
