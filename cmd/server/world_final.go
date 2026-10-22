package main

import (
	"fmt"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// finishingTouches adds the cemetery, the invite links in every state, a handover and the closed account.
func (w *world) finishingTouches() {
	// the cemetery: heroes and warriors who fell, in an order the player chose
	c := w.campaign("you", "Cemetery: the fallen of the Misty Mountains",
		"A company that lost heroes (dead, retired) and warriors; the cemetery card lists them in the order the player gave. Boromir played here and closed his account.", defaultSettings)
	co := w.company(c, "boromir", "Company of the broken horn", "Minas Tirith", true)
	w.grant(co, 10, 40)
	w.recruitAll(co)
	l, s, ws := w.roster(co)
	w.must("bury a sergeant", w.c().Bury(w.ctx, c.id, co.id, s[0].ID, "Dead", "Match 1"))
	if r, err := w.c().Injure(w.ctx, c.id, co.id, s[1].ID, "armWound", "Match 1"); err == nil && !r {
		_, err = w.c().Injure(w.ctx, c.id, co.id, s[1].ID, "armWound", "Match 2")
		w.must("retire a sergeant", err)
	}
	for i := 0; i < 4 && i < len(ws); i++ {
		w.must("bury a warrior", w.c().Bury(w.ctx, c.id, co.id, ws[i].ID, "Dead", fmt.Sprintf("Match %d", i+1)))
	}
	_ = l
	d := w.detail(co)
	var order []int64
	for i := len(d.Cemetery) - 1; i >= 0; i-- {
		order = append(order, d.Cemetery[i].Unit.ID)
	}
	w.try("order the cemetery", w.c().SaveCemeteryOrder(w.ctx, co.user, c.id, order))
	w.show(co)

	// the Shire whose Ranger of the North was slain: the next reinforcement may swap its result for a new one (Ranger replacement)
	shire := w.company(c, "bilbo", "Hobbits who lost their Ranger", "The Shire", true)
	w.grant(shire, 10, 40)
	w.recruitAll(shire)
	// A Hobbit rolls Hero in the Making (a 6) and chooses its path. Then the Ranger falls in a lost match (Rescue against the
	// Minas Tirith company), through the real aftermath: a Sergeant takes command and the Hero fills the Sergeant place the
	// promotion left. The company ends with three officers (a company recruits only with three) and the Ranger in the cemetery.
	_, _, hobbits := w.roster(shire)
	w.xp(shire, hobbits[0].ID, 5, "Saved the Ranger's pipe")
	r := w.rollLevel(shire, hobbits[0], 6)
	if len(r.Offers) > 0 {
		for _, ch := range r.Offers[0].Choices {
			if strings.HasPrefix(ch.Key, "path:") && !strings.Contains(ch.Key, "Sorcerer") {
				w.chooseLevel(shire, hobbits[0], ch.Key)
				break
			}
		}
	}
	w.choosePaths(shire, 0)
	if h, _ := w.unit(shire, func(u core.CompanyUnit) bool { return u.Role == core.RoleHero }); h.ID == 0 {
		w.gap("%s: the Hobbit who rolled a 6 is not a Hero in the Making", shire.name)
	}
	m := w.played(c, "Rescue", outcome{winner: 2, vp: []int{1, 5}, note: "The Hobbits lost their Ranger and the field."}, []*wco{shire}, []*wco{co})
	rl, _, _ := w.roster(shire)
	in := w.sheet(shire)
	set(&in, rl.ID, core.OutcomeCasualty, nil)
	a := w.settle(shire, m, in, map[int64][3]int{rl.ID: {1, 1, 1}}, false) // 2: dead
	for range 4 {                                                          // the Leader, then the Sergeant the company is short of
		a = w.am(shire, m) // the questions still open
		switch {
		case a.Succession != nil && a.Succession.NeedLeader && len(a.Succession.LeaderOptions) > 0:
			a = w.decide(shire, m, core.AftermathAnswer{Kind: "succession", Answer: itoa(a.Succession.LeaderOptions[0])})
		case a.Succession != nil && a.Succession.NeedLeader && len(a.Succession.Fallback) > 0:
			a = w.decide(shire, m, core.AftermathAnswer{Kind: "succession", Answer: itoa(a.Succession.Fallback[0])})
		case a.Succession != nil && a.Succession.NeedSergeant && len(a.Succession.SergeantOptions) > 0 && len(a.Succession.Fallback) == 0:
			a = w.decide(shire, m, core.AftermathAnswer{Kind: "sergeant", Answer: itoa(a.Succession.SergeantOptions[0])})
		case a.Succession != nil && a.Succession.NeedSergeant && len(a.Succession.Fallback) > 0:
			a = w.decide(shire, m, core.AftermathAnswer{Kind: "sergeant", Answer: itoa(a.Succession.Fallback[0])})
		}
	}
	a = w.am(shire, m)
	if a.Status == core.AftermathInjuries && a.Succession == nil {
		a = w.decide(shire, m, core.AftermathAnswer{Kind: "continue"})
	}
	if a.Status != core.AftermathLevelUps {
		w.gap("the succession of %s was not settled (status %s, succession %+v, rerolls %d)", shire.name, a.Status, a.Succession, a.Rerolls)
	} else {
		w.finishAftermath(shire, m)
	}
	w.choosePaths(shire, 0) // the Warrior who became an officer chooses its path in the company window
	w.show(shire)

	// invite links of the first campaign: valid, expired, used up (the secrets are printed once, like the page shows them)
	c1 := &wcamp{org: w.user("you"), name: worldMarker}
	list, err := w.c().List(w.ctx, c1.org.ID, true)
	w.must("list", err)
	for _, s := range list {
		if s.Name == worldMarker {
			c1.id = s.ID
		}
	}
	valid, err := w.c().Invite(w.ctx, c1.org, c1.id)
	w.must("valid invite", err)
	expired, err := w.c().Invite(w.ctx, c1.org, c1.id)
	w.must("invite", err)
	used, err := w.c().Invite(w.ctx, c1.org, c1.id)
	w.must("invite", err)
	w.sql("an expired invite", `UPDATE campaign_invites SET expires_at = now() - interval '2 days' WHERE token_hash = $1`, core.HashInviteToken(expired.Token))
	_, err = w.c().Join(w.ctx, w.user("pippin"), used.Token)
	w.must("pippin uses the link", err)
	w.sql("a used-up invite", `UPDATE campaign_invites SET max_uses = 1, uses = 1 WHERE token_hash = $1`, core.HashInviteToken(used.Token))
	w.links = []string{
		"valid    /join/" + valid.Token + "   (Brand new campaign; sign in as frodo@example.com and open it)",
		"expired  /join/" + expired.Token + "   (\"this link has expired\")",
		"used up  /join/" + used.Token + "   (a one-use link already used by Pippin)",
	}

	// a handover: the campaign of the Rohan players is now organised by Faramir
	h := w.campaign("you", "Handed over: Faramir organises now",
		"You created this campaign and handed it over; Faramir is its organiser, You is a player without a company.", defaultSettings)
	w.company(h, "eowyn", "Éowyn's own company", "Rohan", true)
	w.join(h, "faramir")
	w.must("hand over", w.c().HandOver(w.ctx, h.org, h.id, w.user("faramir").ID))

	// a campaign Faramir created himself
	f := w.campaign("faramir", "Faramir's campaign: a small start",
		"Organised by Faramir from the start; two players have companies.", defaultSettings)
	w.company(f, "aragorn", "Rangers of the Ithilien road", "Osgiliath & Ithilien", true)
	w.company(f, "pippin", "Pippin's Guard of the Citadel", "Minas Tirith", true)

	// the closed account
	boromir := w.user("boromir")
	w.must("close Boromir's account", w.b.auth.CloseAccount(w.ctx, boromir.ID, w.pw))
}
