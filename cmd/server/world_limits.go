package main

import (
	"fmt"
	"slices"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// buyCreatures buys the named creature for the first officer who can take it (and the second one for the next, with n 2).
func (w *world) buyCreature(co *wco, name string) bool {
	list, err := w.c().ArmouryCreatures(w.ctx, co.camp.id, co.id)
	if err != nil {
		return w.try("creatures of "+co.name, err)
	}
	for _, cr := range list {
		if cr.Name != name {
			continue
		}
		for _, o := range cr.Owners {
			if !o.Able() {
				continue
			}
			how := ""
			if !cr.PayInfluence && cr.PayGold {
				how = "gold"
			}
			return w.try("buy "+name, w.c().BuyCreature(w.ctx, co.user, co.camp.id, co.id, o.ID, name, how))
		}
		w.try("buy "+name, fmt.Errorf("no officer of %s can take it", co.name))
	}
	return false
}

// hireWanderer hires the named wanderer.
func (w *world) hireWanderer(co *wco, name string) bool {
	list, err := w.c().ArmouryWanderers(w.ctx, co.camp.id, co.id)
	if err != nil {
		return w.try("wanderers of "+co.name, err)
	}
	for _, x := range list {
		if x.Name != name {
			continue
		}
		how := ""
		if !x.PayInfluence && x.PayGold {
			how = "gold"
		}
		return w.try("hire "+name, w.c().HireWanderer(w.ctx, co.user, co.camp.id, co.id, name, how))
	}
	return false
}

// fillLimit buys items of the class for the company's units until the limit (bow, throwing, cavalry) is reached, and says
// whether it was.
func (w *world) fillLimit(co *wco, limit string, class core.EquipmentClass) bool {
	full := func() bool {
		for _, l := range w.detail(co).Limits {
			if l.ID == limit {
				return l.Full()
			}
		}
		return false
	}
	for _, u := range w.detail(co).Roster {
		if full() {
			return true
		}
		if u.OwnPath {
			continue
		}
		items, err := w.c().Armoury(w.ctx, co.camp.id, co.id, u.ID)
		if err != nil {
			continue
		}
		for _, it := range items {
			if !it.Buyable() || it.WeaponModifier || !slices.Contains(it.Classes, class) {
				continue
			}
			how := ""
			if it.PayGold {
				how = "gold"
			}
			if w.c().BuyEquipment(w.ctx, co.user, co.camp.id, co.id, u.ID, it.Name, how) == nil {
				break
			}
		}
	}
	return full()
}

// limitsCampaign shows what the armoury and the recruit window do at their limits, and the creatures and wanderers.
func (w *world) limitsCampaign() {
	c := w.campaign("you", "Limits, creatures and wanderers: the armoury at its edge",
		"Bow, throwing and cavalry limits reached; races limited by the company; companies at their maximum of models; a Warg Marauder; creatures; a Baggage Pony with a pack; a wanderer.", defaultSettings)

	// a Warg Marauder is the 6 of the special chart of Mirkwood: three models on one card; a Hunting Dog for an officer
	mir := w.company(c, "radagast", "Marauders of the Greenwood", "Mirkwood", true)
	w.grant(mir, 40, 60)
	if s, err := w.recruit(mir, 0, 6, 6); err != nil {
		w.try("Warg Marauder", err)
	} else {
		fmt.Fprintln(w.out, "  ", s)
	}
	w.buyCreature(mir, "Hunting Dog")
	w.show(mir)

	// the Shire: a Baggage Pony with a full pack, and a wanderer
	shire := w.company(c, "pippin", "Pippin's Pony Express", "The Shire", true)
	w.grant(shire, 60, 60)
	if w.buyCreature(shire, "Baggage Pony") {
		l, s, _ := w.roster(shire)
		for _, u := range []core.CompanyUnit{l, s[0]} {
			w.buyKind(shire, u, 2, true)
		}
		for _, u := range []core.CompanyUnit{l, s[0]} {
			u, _ = w.unit(shire, func(x core.CompanyUnit) bool { return x.ID == u.ID })
			for _, e := range u.Equipment {
				if e.Size != "" && e.Drop == "" && !e.Upgrade {
					w.try("store "+e.Name, w.c().Carry(w.ctx, shire.user, c.id, shire.id, u.ID, core.CarryAction{Kind: "store", Item: e.Name}))
					break
				}
			}
		}
	}
	w.buyCreature(shire, "Falcon")
	w.hireWanderer(shire, "Wandering Sage")
	w.show(shire)

	// Dwarves: bows and throwing weapons at a third of the company
	dw := w.company(c, "gimli", "Dwarves with all the bows", "Erebor & Dale", true)
	w.grant(dw, 200, 100)
	for _, l := range []struct {
		id    string
		class core.EquipmentClass
	}{{"bow", core.EquipBow}, {"throwing", core.EquipThrowing}} {
		if !w.fillLimit(dw, l.id, l.class) {
			w.gap("the %s limit of %s could not be reached", l.id, dw.name)
		}
	}
	w.show(dw)

	// Rohan: horses up to the cavalry limit
	roh := w.company(c, "eowyn", "Riders on every horse", "Rohan", true)
	w.grant(roh, 200, 100)
	w.recruitAll(roh) // the cavalry limit counts cavalry units, which join from the chart
	w.show(roh)

	// Helm's Deep: the company limits of races and keywords
	hd := w.company(c, "sauron", "Garrison of Helm's Deep", "Helm's Deep", true)
	w.grant(hd, 20, 60)
	w.recruitAll(hd)
	w.show(hd)

	// Rivendell is full at 12 models: nothing more can be recruited
	riv := w.company(c, "galadriel", "Rivendell at its maximum", "Rivendell", true)
	w.grant(riv, 20, 60)
	w.recruitAll(riv)
	w.show(riv)
}
