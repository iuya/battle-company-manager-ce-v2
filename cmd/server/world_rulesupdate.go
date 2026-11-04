package main

// The campaigns of the rules update (docs/admin.md, "Rules files of a campaign"): two campaigns pinned to an OLDER edition of the app's
// Battle Companies file (same name, another version), so the Rules card of the overview has something to offer. The old edition prices
// Healing Herbs one point lower and still sells a "Demo Relic (retired)" that the current file has dropped.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

const (
	rulesUpdateOK      = "Older edition: move to the current files"
	rulesUpdateBlocked = "Older edition: the update is blocked"
	retiredRelic       = "Demo Relic (retired)"
)

// olderEdition stores the previous edition of the shipped Battle Companies file and returns its hash.
func (w *world) olderEdition() string {
	raw, err := os.ReadFile("definitions/battle_companies/battle_companies.json")
	w.must("read the Battle Companies file", err)
	var doc map[string]json.RawMessage
	w.must("parse the Battle Companies file", json.Unmarshal(raw, &doc))
	doc["version"] = json.RawMessage(`"1.9-demo"`)
	var gear []map[string]any
	w.must("parse the gear", json.Unmarshal(doc["gear"], &gear))
	var relic map[string]any
	for _, g := range gear {
		if g["name"] == "Healing Herbs" {
			g["rating"] = 4 // the current file says 5
			relic = map[string]any{}
			for k, v := range g {
				relic[k] = v
			}
		}
	}
	if relic == nil {
		w.must("the Healing Herbs of the shipped file", fmt.Errorf("not found"))
	}
	relic["name"] = retiredRelic
	delete(relic, "aftermathEffect")
	gear = append(gear, relic)
	var err2 error
	doc["gear"], err2 = json.Marshal(gear)
	w.must("write the gear", err2)
	older, err := json.Marshal(doc)
	w.must("older edition", err)
	hash, err := core.RulesetHash(older)
	w.must("hash", err)
	report, err := w.b.rulesets.Check(w.ctx, core.KindBattleCompanies, older)
	w.must("check the older edition", err)
	fmt.Fprintf(w.out, "  older edition checked: %d issues\n", len(report.Issues()))
	info, err := w.b.rulesets.StoreAs(w.ctx, w.user("admin"), core.KindBattleCompanies, hash, older)
	w.must("store the older edition", err)
	return info.Hash
}

// rulesUpdateStates makes the two campaigns: one that moves cleanly (a hero carries the Healing Herbs, a point dearer now) and one that is
// blocked (a hero carries the relic the current file no longer sells).
func (w *world) rulesUpdateStates() {
	hash := w.olderEdition()
	for _, s := range []struct{ name, desc, company, item string }{
		{rulesUpdateOK, "Pinned to the previous edition of the app's Battle Companies file. As the organiser open the Rules card and press Check the update: every company works, and the Healing Herbs cost a point more.", "Heralds of the older edition", "Healing Herbs"},
		{rulesUpdateBlocked, "Pinned to the previous edition of the app's Battle Companies file, which still sold the Demo Relic. Check the update: the company that carries it blocks the move.", "Relic bearers of the older edition", retiredRelic},
	} {
		c := w.campaignWith("you", s.name, s.desc, defaultSettings, core.CampaignInput{BattleCompanies: hash})
		co := w.company(c, "gimli", s.company, "Iron Hills", true)
		w.company(c, "bilbo", "Plain company of the older edition", "The Shire", true)
		w.topUp(co)
		leader, _, _ := w.roster(co)
		w.must("buy "+s.item, w.c().BuyEquipment(w.ctx, co.user, co.camp.id, co.id, leader.ID, s.item, "gold"))
	}
}
