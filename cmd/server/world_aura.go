package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// The Aura parade: companies that carry every aura reachable from the data (docs/aura-ranges.md, data.auras), so that the rings of the
// Tabletop Simulator export can be smoke tested with one click on each company. docs/demo-world.md says which company covers which
// aura and which auras no company can reach.

const auraParadeName = "Aura parade: every aura ring a company can carry"

// pathChart is the part of the Battle Companies file the script reads to aim its level-up rolls: the 2D6 chart of each path.
type pathChart struct {
	Data struct {
		Paths []struct {
			Name        string `json:"name"`
			Progression []struct {
				Roll    int `json:"roll"`
				Options []struct {
					Type        string `json:"type"`
					SpecialRule string `json:"specialRule"`
				} `json:"options"`
			} `json:"progression"`
		} `json:"paths"`
	} `json:"data"`
}

func (w *world) pathChart() pathChart {
	raw, err := os.ReadFile("definitions/battle_companies/battle_companies.json")
	w.must("read the Battle Companies file", err)
	var c pathChart
	w.must("parse the paths", json.Unmarshal(raw, &c))
	return c
}

// rollFor is the 2D6 sum on which the path offers the choice (a special rule by name, the Sorcerer's powers on their row).
func (w *world) rollFor(path, key string) int {
	kind, name, _ := strings.Cut(key, ":")
	for _, p := range w.pathChart().Data.Paths {
		if p.Name != path {
			continue
		}
		for _, row := range p.Progression {
			for _, o := range row.Options {
				switch {
				case kind == "rule" && o.Type == "specialRule" && o.SpecialRule == name:
					return row.Roll
				case kind == "power" && o.Type == "oneOf" && row.Roll == 3:
					return row.Roll
				}
			}
		}
	}
	if kind == "upgrade" {
		return 4 // any result: the company's hero upgrade is offered on every roll
	}
	panic(worldAbort{fmt.Errorf("the %s has no roll that offers %s", path, key)})
}

func (w *world) unitByID(co *wco, id int64) core.CompanyUnit {
	return w.mustUnit(co, fmt.Sprintf("unit %d", id), func(u core.CompanyUnit) bool { return u.ID == id })
}

// gain gives a hero one result of its path chart (or its company's upgrade): 5 experience, a typed roll that offers it, the choice.
func (w *world) gain(co *wco, id int64, key string) {
	u := w.unitByID(co, id)
	path := ""
	if u.Path != nil {
		path = u.Path.Name
	}
	sum := w.rollFor(path, key)
	w.xp(co, id, 5, "A lesson of the Aura parade")
	r := w.rollLevel(co, u, sum, sum)
	if r.Done {
		panic(worldAbort{fmt.Errorf("%s: the roll of %d for %s offered nothing", u.Title(), sum, key)})
	}
	w.chooseLevel(co, u, key)
}

// becomeHero turns a warrior into a hero on the path with the roll of a 6 (a Sorcerer then names its first power).
func (w *world) becomeHero(co *wco, id int64, path, power string) {
	u := w.unitByID(co, id)
	w.xp(co, id, 5, "Hero in the making")
	w.rollLevel(co, u, 6)
	r := w.chooseLevel(co, u, "path:"+path)
	if power != "" {
		r = w.chooseLevel(co, u, "power:"+power)
	}
	_ = r
}

// pathOf gives an officer its path (a Sorcerer its first power).
func (w *world) pathOf(co *wco, id int64, path, power string) {
	w.must("choose "+path, w.c().ChoosePath(w.ctx, co.user, co.camp.id, id, path, power))
}

// buy buys the item for the unit with gold, or influence when it has no gold price; false (and a line in the log) when it cannot.
func (w *world) buy(co *wco, id int64, item string) bool {
	return w.try("buy "+item, w.c().BuyEquipment(w.ctx, co.user, co.camp.id, id, item, "gold"))
}

// topUp gives the company money for the shopping of the script.
func (w *world) topUp(co *wco) {
	for i := 0; i < 3; i++ {
		w.grant(co, 100, 100)
	}
}

// auraParade loads the campaign.
func (w *world) auraParade() {
	c := w.campaign("you", auraParadeName,
		"For the Tabletop Simulator smoke test of the aura rings: open a company, press the Tabletop Simulator button, apply the names. docs/demo-world.md lists which company shows which ring. Own paths level up (Lead by Example).",
		core.CampaignSettings{ShakeTheTable: true, ManualRolls: true, CustomOnTheirOwnPath: true})
	w.auraFiefdoms(c)
	w.auraRohan(c)
	w.auraLothlorien(c)
	w.auraAngmar(c)
	for _, sat := range []struct{ company, upgrade string }{
		{"Lake-town", "Moneybags"}, {"Moria", "Callous Leadership"}, {"City of Umbar", "Seize Command"},
		{"Osgiliath & Ithilien", "Remember Today"}, {"Goblin-Town", "Swap with Me!"}, {"Wanderers in the Wild", "You Have My Sword"},
	} {
		w.auraSatellite(c, sat.company, sat.upgrade)
	}
	w.auraCoverage()
}

// auraUnreachable are the entries of data.auras no company of the app can carry, with the reason (docs/demo-world.md prints them).
var auraUnreachable = map[string]string{
	"banner-3": "a plain Banner is not sold: the Battle Companies equipment table replaces it with the Company Standard (data.optionSales notSold)",
}

// auraCoverage says (as a gap) which entry of data.auras no company of the parade carries and the list above does not explain.
func (w *world) auraCoverage() {
	for _, e := range w.auraRules().BC.Auras().Entries {
		cos := w.auraCovered[e.ID]
		slices.Sort(cos)
		cos = slices.Compact(cos)
		fmt.Fprintf(w.out, "  aura entry %-18s %-34s %s\n", e.ID, e.Label, strings.Join(cos, "; "))
		if len(cos) == 0 && auraUnreachable[e.ID] == "" {
			w.gap("the Aura parade does not carry the aura %s (%s) and nothing says why", e.ID, e.Label)
		}
	}
}

// auraRohan: the paths the Fiefdoms do not have, a rider with a Company Standard.
func (w *world) auraRohan(c *wcamp) {
	co := w.company(c, "eowyn", "Aura parade: Riders of the Mark", "Rohan", false)
	w.topUp(co)
	l, s, ws := w.roster(co)
	w.pathOf(co, l.ID, "Path of the Claimant", "")
	w.pathOf(co, s[0].ID, "Path of the Ranger", "")
	w.pathOf(co, s[1].ID, "Path of the Berserker", "")
	w.gain(co, l.ID, "rule:Claim the Throne")
	w.gain(co, s[0].ID, "rule:Chief Advisor")
	w.gain(co, s[1].ID, "rule:To Arms!")
	w.becomeHero(co, ws[0].ID, "Path of the Duelist", "")
	w.gain(co, ws[0].ID, "rule:For Glory")
	w.becomeHero(co, ws[1].ID, "Path of the Shieldbearer", "")
	w.gain(co, ws[1].ID, "rule:The King's Protector")
	w.recruitAll(co)
	for _, u := range w.detail(co).Roster {
		if strings.Contains(u.Type, "Rider") && u.Role == core.RoleWarrior {
			w.buy(co, u.ID, "Company Standard")
			break
		}
	}
	w.show(co)
	w.auraInventory(co)
}

// auraLothlorien: a Sorcerer who knows every power that gives a ring, a Falcon and the Wood Elf Sentinels' song.
func (w *world) auraLothlorien(c *wcamp) {
	co := w.company(c, "galadriel", "Aura parade: Golden Wood", "Lothlorien", false)
	w.topUp(co)
	l, s, _ := w.roster(co)
	w.pathOf(co, l.ID, "Path of the Sorcerer", "Aura of Command")
	for _, p := range []string{"Aura of Dismay", "Blinding Light", "Fog of Disarray", "Instill Fear", "Writhing Vines", "Fury (Beast)"} {
		w.gain(co, l.ID, "power:"+p)
	}
	w.choosePaths(co, 3)
	_ = s
	w.recruitAll(co) // the Wood Elf Sentinel is a result of the special chart
	var sentinels []core.CompanyUnit
	for _, u := range w.detail(co).Roster {
		if u.Type == "Wood Elf Sentinel" {
			sentinels = append(sentinels, u)
		}
	}
	if len(sentinels) == 0 {
		w.gap("no Wood Elf Sentinel in %s for the Enchanting Song", co.name)
	} else { // a Sentinel on the Path of Their Own Making can take the homebrew rule Lead by Example
		w.xp(co, sentinels[0].ID, 5, "Walked its own way")
		w.rollLevel(co, sentinels[0], 6)
		w.gain(co, sentinels[0].ID, "rule:Lead by Example")
	}
	w.buyCreature(co, "Falcon")
	w.show(co)
	w.auraInventory(co)
}

// auraAngmar: Warg Riders (the rider, its on-foot version and the mount are three figures in Tabletop Simulator) with a Company Standard, and a Spectre.
func (w *world) auraAngmar(c *wcamp) {
	co := w.company(c, "sauron", "Aura parade: Angmar", "Angmar & Carn Dum", false)
	w.topUp(co)
	l, s, _ := w.roster(co)
	w.pathOf(co, l.ID, "Path of the Warlord", "") // Heroic Resolve alone: no Voice of Command to replace its ring
	w.pathOf(co, s[0].ID, "Path of the Marksman", "")
	w.pathOf(co, s[1].ID, "Path of the Duelist", "")
	w.recruitAll(co) // Warg Riders, a Spectre and a Werewolf come from the charts
	for _, u := range w.detail(co).Roster {
		if strings.Contains(u.Type, "Warg Rider") && u.Role == core.RoleWarrior {
			w.buy(co, u.ID, "Company Standard")
			break
		}
	}
	w.show(co)
	w.auraInventory(co)
}

// auraSatellite: a company of the gallery accounts with one officer who took the hereditary upgrade of its company.
func (w *world) auraSatellite(c *wcamp, company, upgrade string) {
	key := "g-" + slug(company)
	w.account(key, slug(company)+"@example.com", company+" player", core.RoleUser, false)
	co := w.company(c, key, "Aura parade: "+company, company, false)
	w.topUp(co)
	w.choosePaths(co, 1)
	l, _, _ := w.roster(co)
	w.gain(co, l.ID, "upgrade:"+upgrade)
	w.show(co)
	w.auraInventory(co)
}

// auraFiefdoms: the company of the owner. Banners, drums and horns on officers and warriors, a mounted Knight with the on-foot version, six heroes
// on five paths and the four hereditary upgrades of the Fiefdoms.
func (w *world) auraFiefdoms(c *wcamp) {
	co := w.company(c, "you", "Aura parade: Fiefdoms of Gondor", "Fiefdoms", false)
	w.topUp(co)
	l, s, ws := w.roster(co)
	w.pathOf(co, l.ID, "Path of the Warlord", "")
	w.pathOf(co, s[0].ID, "Path of the Tactician", "")
	w.pathOf(co, s[1].ID, "Path of the Marksman", "")
	for _, k := range []string{"rule:Furious Tirade", "rule:Commanding Bellow", "rule:Master of Battle (3+)", "rule:Voice of Command", "upgrade:Lord of Dol Amroth"} {
		w.gain(co, l.ID, k)
	}
	for _, k := range []string{"rule:Heroic Move Mastery", "rule:Reading the Portents", "rule:Spy", "rule:Gatekeeper", "upgrade:Chief of Lamedon"} {
		w.gain(co, s[0].ID, k)
	}
	w.gain(co, s[1].ID, "rule:\"Hold positions, loose the arrows!\"")
	axe, vale := ws[1], ws[2]
	w.becomeHero(co, axe.ID, "Path of the Soldier", "")
	for _, k := range []string{"rule:A Good Sort", "rule:Relentless Commander", "upgrade:Lord of Lossarnach"} {
		w.gain(co, axe.ID, k)
	}
	w.becomeHero(co, vale.ID, "Path of the Sharpshooter", "")
	for _, k := range []string{"rule:Knife Mastery", "upgrade:Lord of the Vale"} {
		w.gain(co, vale.ID, k)
	}
	w.buy(co, l.ID, "Armoured horse")
	w.buy(co, l.ID, "Company Horn")
	w.buy(co, ws[0].ID, "Company Standard")
	w.buy(co, ws[3].ID, "Company Drum")
	w.buy(co, s[1].ID, "Company Instrument")
	w.buy(co, axe.ID, "Torching Brand")
	w.show(co)
	w.auraInventory(co)
}

// auraFigures logs the rings the Tabletop Simulator export of the company puts on its figures with the default selection (what the dialog
// will send), and says so as a gap when a company that carries auras sends none.
func (w *world) auraFigures(co *wco) {
	rs := w.auraRules()
	opts := core.TTSOptions{FootVersions: true, MountFigures: true}
	exp, err := w.c().TTSExport(w.ctx, co.camp.id, co.id, nil, opts)
	w.must("export of "+co.name, err)
	p, err := w.c().TTSNames(w.ctx, co.camp.id, co.id, exp, core.TTSNamesOptions{Rules: core.TTSRulesNone, Auras: rs.DefaultAuraSelection()})
	w.must("names of "+co.name, err)
	rings, figures := 0, 0
	for _, f := range p.Figures {
		if len(f.Auras) > 0 {
			figures++
			rings += len(f.Auras)
			if f.Extra != "" {
				fmt.Fprintf(w.out, "    figure %s (%s) has %d rings\n", f.Line, f.Extra, len(f.Auras))
			}
		}
	}
	fmt.Fprintf(w.out, "    Tabletop Simulator: %d rings on %d of %d figures (payload v%d)\n", rings, figures, len(p.Figures), p.Version)
	if rings == 0 {
		w.gap("the export of %s sends no aura ring", co.name)
	}
}

// auraRules are the default rules of the app, to read the auras of the companies.
func (w *world) auraRules() *core.RuleSet {
	if w.rules == nil {
		var h core.RulesetHashes
		for kind, set := range map[core.RulesetKind]*string{core.KindVanilla: &h.Vanilla, core.KindBattleCompanies: &h.BattleCompanies, core.KindGlossary: &h.Glossary} {
			info, err := w.b.rulesets.Default(w.ctx, kind)
			w.must("default "+string(kind), err)
			*set = info.Hash
		}
		rs, err := core.NewRules(w.b.store.Rulesets(), nil).For(w.ctx, h)
		w.must("rules of the aura parade", err)
		w.rules = rs
	}
	return w.rules
}

// auraInventory prints the auras of every unit and creature of the company and remembers the entries of data.auras they cover.
func (w *world) auraInventory(co *wco) {
	rs := w.auraRules()
	d := w.detail(co)
	note := func(who string, as []core.Aura) {
		if len(as) == 0 {
			return
		}
		var parts []string
		for _, a := range as {
			parts = append(parts, fmt.Sprintf("%g\" %s %s", a.Radius, a.Kind, a.Label))
			for _, id := range a.Entries {
				w.auraCovered[id] = append(w.auraCovered[id], co.name)
			}
		}
		fmt.Fprintf(w.out, "    aura %s: %s\n", who, strings.Join(parts, " | "))
	}
	defer w.auraFigures(co)
	for _, u := range d.Roster {
		note(u.Title(), rs.UnitAuras(u))
	}
	for _, c := range d.Creatures {
		note(c.Name+" of "+c.OwnerName, rs.CreatureAuras(c))
	}
}
