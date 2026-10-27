package main

// targetsCampaign is a small campaign for the live match's rule markers (docs/unit-keywords.md "Rule targets"): the Dwarves of Erebor have
// Ancient Enemies (Orc) and the Mirkwood company is full of Orcs, Goblins and Spiders. Nobody plays a match here: the e2e flow 95-keyword-targets
// opens one through the match form, so the markers can be seen when the match starts.
func (w *world) targetsCampaign() {
	c := w.campaign("you", "Keyword targets: who the rules work against",
		"Erebor's Dwarves hate Orcs (Ancient Enemies) and Mirkwood's company has them: open a match between the two to see the Hatred markers on the enemy's units.", defaultSettings)
	dw := w.company(c, "gimli", "Dwarves who hate Orcs", "Erebor & Dale", true)
	w.grant(dw, 20, 60)
	w.recruitAll(dw)
	w.show(dw)
	mir := w.company(c, "radagast", "Mirkwood's Orcs and spiders", "Mirkwood", true)
	w.grant(mir, 20, 60)
	w.recruitAll(mir)
	w.show(mir)
}
