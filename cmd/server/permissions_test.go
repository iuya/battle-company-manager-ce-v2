package main

// The permission matrix: every state-changing action of the core services, tried by every kind of account in every
// campaign state, against the real services over PostgreSQL (the same wiring as the server). One table, permActions, says for each
// action who may do it while the campaign is active, paused and finished; every other account must be refused with the expected
// reason. Anonymous visitors never reach the core: the web layer's own test (internal/web, TestAnonymousWritesRedirectToLogin) covers
// them for every route.
//
// To add an action: add a permAction to permActions (name, the fixture it needs, run, who is allowed per state, the reasons).
//   - fixture: permRanger (the Rangers are Hobbits of the Shire whose Ranger of the North fell and has been chosen at a recruit:
//     the officers wait to be arranged; no match), permIdle (no match), permOpen (Rangers vs Raiders, scenario and result saved), permProposed (the same with the end
//     proposed by the Raiders' owner), permEnded (ended, both aftermaths still to play).
//   - run does one call as the given account and returns its error; build the arguments from the fixture (fx.rev() is the match's
//     current revision, fx.unitA() a unit of the Rangers).
//   - allow is [active, paused, finished]; use the role sets (managers, players, members...) and + to combine them.
//   - roleMsg is what the refusal says to an account that could never do it, stateMsg what it says to an account that could in
//     another state ("This campaign is paused.", ...). Accounts that are not members get ErrNotFound unless the action is
//     listed with them in allow (the campaign status changes only look at the role).
//   - strangersRefused: true for the actions that never look at the membership (campaign status, handover).
//   - roleMsg and stateMsg may list alternatives separated by "|" (the first check that fails answers).
//   - notFound: members that get ErrNotFound for lack of a company of their own; alsoErr: a second error a refusal may be.
//   - prep: a func(f *permFx) that sets up game state (fx.grantBoth, fx.xpLeader, fx.injureSergeants, fx.strengthenOther).
//   - repeatable: true when the allowed accounts can all do it one after the other on the same campaign; otherwise every allowed
//     account gets a fresh campaign.
//
// A failing cell prints "action / state / account" with what the service answered.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

type permRole int

const (
	roleOutsider  permRole = 1 << iota // signed in, not in the campaign
	roleAdminOut                       // an administrator who is not in the campaign
	roleNoCompany                      // member without a company
	roleOwner                          // member, owns the Rangers
	roleOther                          // member, owns the Raiders: the other player of the match
	roleOrganiser                      // organises the campaign, no company
	roleAdmin                          // administrator, member
	roleSuperuser                      // superuser, member

	managers   = roleOrganiser | roleAdmin | roleSuperuser
	admins     = roleAdmin | roleSuperuser
	players    = roleOwner | roleOther
	members    = roleNoCompany | roleOwner | roleOther | roleOrganiser | roleAdmin | roleSuperuser
	nonMembers = roleOutsider | roleAdminOut
	everyone   = members | nonMembers
)

var permRoleNames = map[permRole]string{
	roleOutsider: "outsider", roleAdminOut: "admin not in campaign", roleNoCompany: "member without company", roleOwner: "owner (Rangers)",
	roleOther: "other player (Raiders)", roleOrganiser: "organiser", roleAdmin: "admin member", roleSuperuser: "superuser member",
}

func (r permRole) String() string { return permRoleNames[r] }

type permState int

const (
	stActive permState = iota
	stPaused
	stFinished
)

var permStateNames = [...]string{"active", "paused", "finished"}

type permFixture int

const (
	permIdle     permFixture = iota // two companies, no match
	permOpen                        // an open match with scenario and result
	permProposed                    // the same, its end proposed by the other player
	permEnded                       // ended: both aftermaths are to play
	permRanger                      // no match; the Rangers are Shire Hobbits with a new Ranger of the North waiting for his officers
)

// permAction is one row of the matrix.
type permAction struct {
	name       string
	fixture    permFixture
	run        func(f *permFx, actor core.User) error
	allow      [3]permRole // who may, while active / paused / finished
	roleMsg    string      // the refusal for an account that is never allowed
	stateMsg   [3]string   // the refusal for an account that is allowed in another state (index by the current state)
	repeatable bool
	// only limits the accounts tried (0: all of them)
	only permRole
	// strangersRefused: the action looks at the role only, so an account outside the campaign is refused (ErrRefused with roleMsg)
	// instead of getting ErrNotFound (the campaign status changes and the handover)
	strangersRefused bool
	// notFound: members that get ErrNotFound because the action works on their own company and they have none (or the unit is not
	// theirs)
	notFound permRole
	// prep prepares the game state after the companies exist and before the match is made (money, experience, injuries, a stronger
	// opponent); nil: nothing
	prep func(f *permFx)
	// alsoErr is another error, besides ErrRefused, a refusal may be (ErrCompanyLimit)
	alsoErr error
}

const (
	msgPaused   = "This campaign is paused."
	msgFinished = "The campaign is finished."
)

var permActions = []permAction{
	// -- invitations
	{name: "Invite", run: func(f *permFx, a core.User) error { _, err := f.c().Invite(f.ctx, a, f.camp.id); return err },
		allow: [3]permRole{managers, 0, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", msgPaused, msgFinished}, repeatable: true},
	{name: "InvalidateInvites", run: func(f *permFx, a core.User) error { _, err := f.c().InvalidateInvites(f.ctx, a, f.camp.id); return err },
		allow: [3]permRole{managers, managers, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "RegenerateInvite", run: func(f *permFx, a core.User) error { _, err := f.c().RegenerateInvite(f.ctx, a, f.camp.id); return err },
		allow: [3]permRole{managers, 0, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", msgPaused, msgFinished}, repeatable: true},
	{name: "Join", run: func(f *permFx, a core.User) error { _, err := f.c().Join(f.ctx, a, f.token); return err },
		only: nonMembers, allow: [3]permRole{nonMembers, 0, 0}, stateMsg: [3]string{"", msgPaused, "not found"}, repeatable: true},

	// -- the campaign itself
	{strangersRefused: true, name: "SetCampaignStatus pause", run: status(core.CampaignPaused),
		allow: [3]permRole{managers | roleAdminOut, 0, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", "already paused", "Reactivate the campaign first|Only an administrator can reactivate"}},
	{strangersRefused: true, name: "SetCampaignStatus resume", run: status(core.CampaignActive),
		allow: [3]permRole{0, managers | roleAdminOut, admins | roleAdminOut}, roleMsg: "Only the organiser",
		stateMsg: [3]string{"already active", "", "Only an administrator can reactivate"}},
	{strangersRefused: true, name: "SetCampaignStatus end", run: status(core.CampaignFinished),
		allow: [3]permRole{managers | roleAdminOut, managers | roleAdminOut, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", "", "already finished"}},
	{strangersRefused: true, name: "HandOver to a member without a company", run: func(f *permFx, a core.User) error {
		return f.c().HandOver(f.ctx, a, f.camp.id, f.u[roleNoCompany].ID)
	}, allow: [3]permRole{managers | roleAdminOut, managers | roleAdminOut, managers | roleAdminOut}, roleMsg: "Only the organiser"},

	// -- companies
	{name: "CreateCompany", run: func(f *permFx, a core.User) error {
		_, err := f.c().CreateCompany(f.ctx, a, f.camp.id, core.CompanyInput{Name: "New " + a.DisplayName, Faction: "Arnor"})
		return err
	}, allow: [3]permRole{roleNoCompany | roleOrganiser | admins, roleNoCompany | roleOrganiser | admins, 0},
		roleMsg: "already have all the companies", alsoErr: core.ErrCompanyLimit, stateMsg: [3]string{"", "", msgFinished}, fixture: permIdle},
	{name: "RenameCompany", run: func(f *permFx, a core.User) error {
		return f.c().RenameCompany(f.ctx, a, f.camp.id, f.companyOf(a), "Renamed "+a.DisplayName)
	}, allow: [3]permRole{players, players, 0}, notFound: roleNoCompany | managers, stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "SaveFormation", run: func(f *permFx, a core.User) error {
		d, err := f.c().CompanyDetail(f.ctx, f.camp.id, f.companyOf(a))
		if err != nil {
			return err
		}
		var order []int64
		for _, u := range d.Roster {
			order = append(order, u.ID)
		}
		return f.c().SaveFormation(f.ctx, a, f.camp.id, f.companyOf(a), order)
	}, allow: [3]permRole{players, players, 0}, notFound: roleNoCompany | managers, stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "SaveCemeteryOrder", run: func(f *permFx, a core.User) error {
		return f.c().SaveCemeteryOrder(f.ctx, a, f.camp.id, f.companyOf(a), nil)
	}, allow: [3]permRole{players, players, 0}, notFound: roleNoCompany | managers, stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "RenameUnit (a unit of the Rangers)", run: func(f *permFx, a core.User) error {
		return f.c().RenameUnit(f.ctx, a, f.camp.id, f.companyOf(a), f.unitA().ID, "Renamed")
	}, allow: [3]permRole{roleOwner, roleOwner, 0}, notFound: members &^ roleOwner, stateMsg: [3]string{"", "", msgFinished}, repeatable: true},

	// -- the Ranger of the North takes command (the Shire fixture: a fallen Ranger and three other officers wait to be arranged)
	{name: "ArrangeOfficers", fixture: permRanger, run: func(f *permFx, a core.User) error {
		panel, err := f.c().PendingOfficers(f.ctx, f.u[roleOwner], f.camp.id, f.a.id)
		if err != nil || panel == nil {
			return fmt.Errorf("the fixture has no officers waiting: %v", err)
		}
		arr := core.OfficerArrangement{Leader: core.RangerID}
		for _, o := range panel.Officers[:2] {
			arr.Sergeants = append(arr.Sergeants, o.ID)
		}
		_, err = f.c().ArrangeOfficers(f.ctx, a, f.camp.id, f.companyOf(a), arr)
		return err
	}, allow: [3]permRole{roleOwner, roleOwner, 0}, notFound: roleNoCompany | managers, roleMsg: "No Ranger of the North is waiting", stateMsg: [3]string{"", "", msgFinished}},

	// -- matches
	{name: "CreateMatch", fixture: permIdle, run: func(f *permFx, a core.User) error {
		_, err := f.c().CreateMatch(f.ctx, a, f.camp.id, core.NewMatch{Sides: []core.NewSide{{Name: "A", Companies: []int64{f.a.id}}, {Name: "B", Companies: []int64{f.b.id}}}})
		return err
	}, allow: [3]permRole{members, 0, 0}, stateMsg: [3]string{"", msgPaused, msgFinished}},
	{name: "SetScenario", fixture: permOpen, run: func(f *permFx, a core.User) error {
		_, err := f.c().SetScenario(f.ctx, a, f.camp.id, f.match.ID, f.rev(), core.ScenarioInput{Kind: core.ScenarioCustom, Name: "Duel by " + a.DisplayName})
		return err
	}, allow: [3]permRole{players | managers, players | managers, 0}, roleMsg: "Only the players in this match", stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "ClearScenario", fixture: permOpen, run: func(f *permFx, a core.User) error {
		_, err := f.c().ClearScenario(f.ctx, a, f.camp.id, f.match.ID, f.rev())
		return err
	}, allow: [3]permRole{players | managers, players | managers, 0}, roleMsg: "Only the players in this match", stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "SaveResult", fixture: permOpen, run: func(f *permFx, a core.User) error {
		_, err := f.c().SaveResult(f.ctx, a, f.camp.id, f.match.ID, f.rev(), core.MatchResult{Decided: true, Winner: 2, Sides: make([]core.SideResult, 2)})
		return err
	}, allow: [3]permRole{players | managers, players | managers, 0}, roleMsg: "Only the players in this match", stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "SetSideRoles", fixture: permOpen, run: func(f *permFx, a core.User) error {
		_, err := f.c().SetSideRoles(f.ctx, a, f.camp.id, f.match.ID, f.rev(), []string{core.RoleAttacker, core.RoleDefender})
		return err
	}, allow: [3]permRole{players | managers, players | managers, 0}, roleMsg: "Only the players in this match", stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "ProposeEnd", fixture: permOpen, run: func(f *permFx, a core.User) error {
		_, err := f.c().ProposeEnd(f.ctx, a, f.camp.id, f.match.ID, f.rev())
		return err
	}, allow: [3]permRole{players | managers, players | managers, 0}, roleMsg: "Only the players in this match", stateMsg: [3]string{"", "", msgFinished}},
	{name: "ConfirmEnd", fixture: permProposed, run: func(f *permFx, a core.User) error {
		_, err := f.c().ConfirmEnd(f.ctx, a, f.camp.id, f.match.ID, f.rev())
		return err
	}, allow: [3]permRole{roleOwner, roleOwner, 0}, roleMsg: "no company waiting|Only the players in this match", stateMsg: [3]string{"", "", msgFinished}},
	{name: "WithdrawEnd", fixture: permProposed, run: func(f *permFx, a core.User) error {
		_, err := f.c().WithdrawEnd(f.ctx, a, f.camp.id, f.match.ID, f.rev())
		return err
	}, allow: [3]permRole{roleOther, roleOther, 0}, roleMsg: "not confirmed|Only the players in this match", stateMsg: [3]string{"", "", msgFinished}},
	{name: "CancelEnd", fixture: permProposed, run: func(f *permFx, a core.User) error {
		_, err := f.c().CancelEnd(f.ctx, a, f.camp.id, f.match.ID, f.rev())
		return err
	}, allow: [3]permRole{players | managers, players | managers, 0}, roleMsg: "Only the players in this match", stateMsg: [3]string{"", "", msgFinished}},
	{name: "OverrideEnd", fixture: permProposed, run: func(f *permFx, a core.User) error {
		_, err := f.c().OverrideEnd(f.ctx, a, f.camp.id, f.match.ID, f.rev(), "the other player left")
		return err
	}, allow: [3]permRole{managers, managers, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", "", msgFinished}},
	{name: "ReopenMatch", fixture: permEnded, run: func(f *permFx, a core.User) error {
		_, err := f.c().ReopenMatch(f.ctx, a, f.camp.id, f.match.ID, f.rev())
		return err
	}, allow: [3]permRole{managers, managers, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", "", msgFinished}},
	{name: "GrantReward", fixture: permIdle, run: func(f *permFx, a core.User) error {
		_, err := f.c().GrantReward(f.ctx, a, f.camp.id, f.a.id, core.GrantInput{Gold: 1, Note: "for the test"})
		return err
	}, allow: [3]permRole{managers, managers, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", "", msgFinished}, repeatable: true},

	// -- what a company does with its money and its units (the owner only; a locked or finished company refuses)
	{name: "RollRecruit", prep: (*permFx).grantBoth, run: func(f *permFx, a core.User) error {
		return f.c().RollRecruit(f.ctx, a, f.camp.id, f.companyOf(a), 0, false)
	}, allow: [3]permRole{players, players, 0}, notFound: roleNoCompany | managers, stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "BuyEquipment", prep: (*permFx).grantBoth, run: func(f *permFx, a core.User) error {
		co := f.companyOf(a)
		d, err := f.c().CompanyDetail(f.ctx, f.camp.id, co)
		if err != nil {
			return err
		}
		for _, u := range d.Roster {
			items, err := f.c().Armoury(f.ctx, f.camp.id, co, u.ID)
			if err != nil {
				return err
			}
			for _, it := range items {
				if it.Buyable() && it.PayInfluence {
					return f.c().BuyEquipment(f.ctx, a, f.camp.id, f.companyOf(a), u.ID, it.Name, "")
				}
			}
		}
		return errors.New("the test found nothing to buy")
	}, allow: [3]permRole{players, players, 0}, notFound: roleNoCompany | managers, stateMsg: [3]string{"", "", msgFinished}},
	{name: "RollLevelUp (the leader)", prep: (*permFx).xpLeaders, run: func(f *permFx, a core.User) error {
		_, err := f.c().RollLevelUp(f.ctx, a, f.camp.id, f.companyOf(a), f.leaderOf(f.companyOf(a)), nil, false)
		return err
	}, allow: [3]permRole{players, players, 0}, notFound: roleNoCompany | managers, stateMsg: [3]string{"", "", msgFinished}},
	{name: "TreatInjury (healers)", prep: (*permFx).injureSergeants, run: func(f *permFx, a core.User) error {
		_, err := f.c().TreatInjury(f.ctx, a, f.camp.id, f.companyOf(a), f.sergeantOf(f.companyOf(a)), core.Treatment{Method: "healers", Injury: "brokenHonour"})
		return err
	}, allow: [3]permRole{players, players, 0}, notFound: roleNoCompany | managers, stateMsg: [3]string{"", "", msgFinished}},

	// -- the live match (the Rangers' tracker) and Against the Odds (the Rangers are the lower rated company)
	{name: "LiveAction (Rangers)", fixture: permOpen, run: func(f *permFx, a core.User) error {
		_, _, err := f.c().Apply(f.ctx, a, f.camp.id, f.match.ID, f.a.id, core.LiveAction{Kind: core.LiveStatChange, UnitID: f.leaderOf(f.a.id), Stat: "wounds", Delta: -1})
		return err
	}, allow: [3]permRole{roleOwner | managers, roleOwner | managers, 0}, roleMsg: "Only the owner of", stateMsg: [3]string{"", "", msgFinished}},
	{name: "SaveAgainstTheOdds (clear)", fixture: permOpen, prep: (*permFx).strengthenOther, run: func(f *permFx, a core.User) error {
		_, err := f.c().SaveAgainstTheOdds(f.ctx, a, f.camp.id, f.match.ID, f.rev(), core.ATOSelection{})
		return err
	}, allow: [3]permRole{roleOwner | managers, roleOwner | managers, 0}, roleMsg: "Only the owner of", stateMsg: [3]string{"", "", msgFinished}, repeatable: true},

	// -- aftermath of an ended match (the Rangers' company)
	{name: "SaveAftermath (the Rangers' sheet)", fixture: permEnded, run: func(f *permFx, a core.User) error {
		cur, err := f.c().Aftermath(f.ctx, f.u[roleOrganiser], f.camp.id, f.match.ID, f.a.id)
		if err != nil {
			return err
		}
		d, err := f.c().CompanyDetail(f.ctx, f.camp.id, f.a.id)
		if err != nil {
			return err
		}
		_, err = f.c().SaveAftermath(f.ctx, a, f.camp.id, f.match.ID, f.a.id, cur.Revision, core.DefaultSheet(d.Roster))
		return err
	}, allow: [3]permRole{roleOwner | managers, roleOwner | managers, 0}, roleMsg: "Only the owner of", stateMsg: [3]string{"", "", msgFinished}, repeatable: true},
	{name: "WaiveAftermath (the Rangers)", fixture: permEnded, run: func(f *permFx, a core.User) error {
		_, err := f.c().WaiveAftermath(f.ctx, a, f.camp.id, f.match.ID, f.a.id, "never played")
		return err
	}, allow: [3]permRole{managers, managers, 0}, roleMsg: "Only the organiser", stateMsg: [3]string{"", "", msgFinished}},
}

// status changes the campaign status the way the Manage campaign card does.
func status(to core.CampaignStatus) func(f *permFx, a core.User) error {
	return func(f *permFx, a core.User) error {
		return f.c().SetCampaignStatus(f.ctx, a, f.camp.id, to, f.camp.name)
	}
}

// permFx is one campaign of the matrix with its accounts, companies and match.
type permFx struct {
	ctx   context.Context
	w     *world
	camp  *wcamp
	a, b  *wco // the Rangers (owner) and the Raiders (other)
	match core.Match
	token string // an invitation link made while the campaign was active
	u     map[permRole]core.User
}

func (f *permFx) c() *campaignsFacade { return f.w.c() }

func (f *permFx) rev() int {
	m, err := f.c().Match(f.ctx, f.camp.id, f.match.ID)
	if err != nil {
		panic(worldAbort{err})
	}
	return m.Revision
}

// The helpers of prep: they run as the organiser, who may grant, and through the services like any player.

// grantBoth gives both companies gold and influence.
func (f *permFx) grantBoth() {
	f.w.grant(f.a, 30, 30)
	f.w.grant(f.b, 30, 30)
}

// xpLeaders gives both leaders experience.
func (f *permFx) xpLeaders() {
	f.w.xp(f.a, f.leaderOf(f.a.id), 5, "matrix")
	f.w.xp(f.b, f.leaderOf(f.b.id), 5, "matrix")
}

// injureSergeants gives the first Sergeant of each company a broken honour, and the companies influence to treat it.
func (f *permFx) injureSergeants() {
	f.grantBoth()
	for _, co := range []*wco{f.a, f.b} {
		_, err := f.c().Injure(f.ctx, f.camp.id, co.id, f.sergeantOf(co.id), "brokenHonour", "the matrix")
		f.w.must("injure", err)
	}
}

// strengthenOther makes the Raiders worth more than the Rangers, so that the Rangers are the lower rated company.
func (f *permFx) strengthenOther() {
	f.w.grant(f.b, 60, 60)
	f.w.must("a wanderer for the Raiders", func() error {
		list, err := f.c().ArmouryWanderers(f.ctx, f.camp.id, f.b.id)
		if err != nil {
			return err
		}
		for _, x := range list {
			if x.Hirable() && (x.PayInfluence || x.PayGold) {
				how := ""
				if !x.PayInfluence {
					how = "gold"
				}
				return f.c().HireWanderer(f.ctx, f.u[roleOther], f.camp.id, f.companyOf(f.u[roleOther]), x.Name, how)
			}
		}
		return errors.New("no wanderer to hire")
	}())
}

// shireRangerWaits buries the Shire company's Ranger of the North, gives it a fourth officer and has the owner choose a new Ranger
// at a recruit, so that the officers wait to be arranged.
func (f *permFx) shireRangerWaits() {
	f.w.grant(f.a, 10, 40)
	f.w.recruitAll(f.a)
	d := f.w.detail(f.a)
	for _, u := range d.Roster {
		if u.Type == "Ranger of the North" {
			f.w.must("bury the Ranger", f.c().Bury(f.ctx, f.camp.id, f.a.id, u.ID, "Dead", "the matrix"))
			break
		}
	}
	f.w.grant(f.a, 10, 10)
	f.w.rollWith(4)
	f.w.must("roll the recruit", f.c().RollRecruit(f.ctx, f.u[roleOwner], f.camp.id, f.companyOf(f.u[roleOwner]), 0, false))
	_, err := f.c().ConfirmRecruit(f.ctx, f.u[roleOwner], f.camp.id, f.companyOf(f.u[roleOwner]), 0, "ranger")
	f.w.must("choose the Ranger of the North", err)
}

func (f *permFx) leaderOf(company int64) int64 {
	d, err := f.c().CompanyDetail(f.ctx, f.camp.id, company)
	f.w.must("detail", err)
	for _, u := range d.Roster {
		if u.Role == core.RoleLeader {
			return u.ID
		}
	}
	panic(worldAbort{errors.New("no leader")})
}

func (f *permFx) sergeantOf(company int64) int64 {
	d, err := f.c().CompanyDetail(f.ctx, f.camp.id, company)
	f.w.must("detail", err)
	for _, u := range d.Roster {
		if u.Role == core.RoleSergeant {
			return u.ID
		}
	}
	panic(worldAbort{errors.New("no sergeant")})
}

func (f *permFx) unitA() core.CompanyUnit { return f.w.detail(f.a).Roster[0] }

func (f *permFx) companyOf(a core.User) int64 {
	if a.ID == f.u[roleOther].ID {
		return f.b.id
	}
	return f.a.id
}

var permSeq int // numbers the campaigns of the matrix

// permBuild makes a campaign in the fixture's shape and moves it to the state.
func permBuild(w *world, users map[permRole]core.User, fixture permFixture, prep func(*permFx), state permState) *permFx {
	org := users[roleOrganiser]
	permSeq++
	s, err := w.c().Create(w.ctx, org, core.CampaignInput{Name: fmt.Sprintf("Matrix %d", permSeq)})
	w.must("create campaign", err)
	camp := &wcamp{id: s.ID, name: s.Name, org: org}
	inv, err := w.c().Invite(w.ctx, org, camp.id)
	w.must("invite", err)
	for _, r := range []permRole{roleNoCompany, roleOwner, roleOther, roleAdmin, roleSuperuser} {
		_, err := w.c().Join(w.ctx, users[r], inv.Token)
		w.must("join", err)
	}
	for k, u := range users {
		w.users[k.String()] = u
	}
	f := &permFx{ctx: w.ctx, w: w, camp: camp, u: users, token: inv.Token}
	if fixture == permRanger {
		f.a = w.company(camp, roleOwner.String(), "Rangers", "The Shire", true)
	} else {
		f.a = w.company(camp, roleOwner.String(), "Rangers", "Arnor", true)
	}
	f.b = w.company(camp, roleOther.String(), "Raiders", "Mordor", true)
	if prep != nil {
		prep(f)
	}
	if fixture == permRanger {
		f.shireRangerWaits()
	}
	if fixture != permIdle && fixture != permRanger {
		f.match = w.newMatch(camp, []*wco{f.a}, []*wco{f.b})
		f.match = w.oneOff(camp, f.match, "Duel", "Two companies meet", "Hold the field")
		f.match = w.report(camp, f.match, outcome{winner: 1})
	}
	switch fixture {
	case permProposed:
		m, err := w.c().ProposeEnd(w.ctx, users[roleOther], camp.id, f.match.ID, f.match.Revision)
		w.must("propose the end", err)
		f.match = m
	case permEnded:
		m, err := endNow(w.ctx, w.c(), org, camp.id, f.match.ID, f.match.Revision)
		w.must("end the match", err)
		f.match = m
	}
	switch state {
	case stPaused:
		w.must("pause", w.c().SetCampaignStatus(w.ctx, org, camp.id, core.CampaignPaused, ""))
	case stFinished:
		w.must("end the campaign", w.c().SetCampaignStatus(w.ctx, org, camp.id, core.CampaignFinished, camp.name))
	}
	return f
}

func TestPermissionMatrix(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	t.Parallel() // its own database (a copy of the seeded template), no state shared with the other tests
	ctx := context.Background()
	store, err := postgres.OpenWith(ctx, freshDatabase(ctx, t, url), postgres.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dice := &demoDice{}
	b, err := newBackendWithDice(ctx, store, dice)
	if err != nil {
		t.Fatal(err)
	}
	w := &world{ctx: ctx, b: b, dice: dice, pw: "matrix-password", out: io.Discard, users: map[string]core.User{}}
	defer func() {
		if r := recover(); r != nil {
			if a, ok := r.(worldAbort); ok {
				t.Fatalf("building the matrix: %v", a.err)
			}
			panic(r)
		}
	}()
	users := map[permRole]core.User{
		roleOutsider:  w.account("matrix outsider", "outsider@matrix.test", "Outsider", core.RoleUser, false),
		roleAdminOut:  w.account("matrix admin out", "adminout@matrix.test", "AdminOut", core.RoleAdmin, false),
		roleNoCompany: w.account("matrix nocompany", "nocompany@matrix.test", "NoCompany", core.RoleUser, false),
		roleOwner:     w.account("matrix owner", "owner@matrix.test", "Owner", core.RoleUser, false),
		roleOther:     w.account("matrix other", "other@matrix.test", "Other", core.RoleUser, false),
		roleOrganiser: w.account("matrix organiser", "organiser@matrix.test", "Organiser", core.RoleUser, true),
		roleAdmin:     w.account("matrix admin", "admin@matrix.test", "Admin", core.RoleAdmin, true),
		roleSuperuser: w.account("matrix superuser", "superuser@matrix.test", "Superuser", core.RoleSuperuser, true),
	}
	roles := []permRole{roleOutsider, roleAdminOut, roleNoCompany, roleOwner, roleOther, roleOrganiser, roleAdmin, roleSuperuser}

	for _, act := range permActions {
		t.Run(act.name, func(t *testing.T) {
			var eligible permRole // allowed in some state: its refusal in the others is about the state
			for _, a := range act.allow {
				eligible |= a
			}
			for state := stActive; state <= stFinished; state++ {
				var shared *permFx
				build := func() *permFx {
					var f *permFx
					func() {
						defer func() {
							if r := recover(); r != nil {
								if a, ok := r.(worldAbort); ok {
									t.Fatalf("%s: building the campaign: %v", permStateNames[state], a.err)
								}
								panic(r)
							}
						}()
						f = permBuild(w, users, act.fixture, act.prep, state)
					}()
					return f
				}
				// refused accounts first (they change nothing), then the allowed ones
				for _, allowedPass := range []bool{false, true} {
					for _, role := range roles {
						if act.only != 0 && act.only&role == 0 {
							continue
						}
						allowed := act.allow[state]&role != 0
						if allowed != allowedPass {
							continue
						}
						f := shared
						if f == nil || (allowed && !act.repeatable) {
							f = build()
							if !allowed || act.repeatable {
								shared = f
							}
						}
						err := act.run(f, users[role])
						cell := fmt.Sprintf("%s / %s / %s", act.name, permStateNames[state], role)
						switch {
						case allowed:
							if err != nil {
								t.Errorf("%s: should be allowed, got %v", cell, err)
							}
						case nonMembers&role != 0 && !act.strangersRefused && !strings.HasPrefix(act.name, "Join"):
							if !errors.Is(err, core.ErrNotFound) {
								t.Errorf("%s: a stranger gets ErrNotFound, got %v", cell, err)
							}
						case act.notFound&role != 0:
							// (a finished campaign answers first to an account that has a company)
							if !errors.Is(err, core.ErrNotFound) && !(state == stFinished && err != nil && strings.Contains(err.Error(), msgFinished)) {
								t.Errorf("%s: an account without that company gets ErrNotFound, got %v", cell, err)
							}
						default:
							// the answers that are right: the role's reason, and the state's when the account could do it elsewhere
							// or the campaign is not active (a finished campaign answers before the role is looked at)
							want := act.roleMsg
							if state != stActive || eligible&role != 0 {
								want += "|" + act.stateMsg[state]
							}
							switch {
							case err == nil:
								t.Errorf("%s: should be refused (%q), it worked", cell, want)
							case strings.HasPrefix(act.name, "Join") && state == stFinished:
								if !errors.Is(err, core.ErrNotFound) {
									t.Errorf("%s: the links of a finished campaign are gone, got %v", cell, err)
								}
							case !errors.Is(err, core.ErrRefused) && !(act.alsoErr != nil && errors.Is(err, act.alsoErr)):
								t.Errorf("%s: should be ErrRefused (%q), got %v", cell, want, err)
							case !containsAny(err.Error(), want):
								t.Errorf("%s: the reason should contain %q, got %q", cell, want, err.Error())
							}
						}
					}
				}
			}
		})
	}
}

// containsAny is true when s contains one of the alternatives (separated by "|"; an empty one is skipped).
func containsAny(s, alternatives string) bool {
	for _, a := range strings.Split(alternatives, "|") {
		if a != "" && strings.Contains(s, a) {
			return true
		}
	}
	return false
}
