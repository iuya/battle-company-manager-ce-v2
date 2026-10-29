package main

// The demo world (`server seed --world`, `make demo`): many accounts and campaigns in every state the app can be in, built
// with the real core services so that all data is valid. docs/demo-world.md lists what each campaign shows. Dice that decide
// an outcome are fixed (core.FixedDice, through demoDice) or typed in, so the world is the same every time.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"slices"
	"strings"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// demoDice is the dice of the services in the demo world: random, except while a script fixes the next results.
type demoDice struct{ fixed *core.FixedDice }

func (d *demoDice) D6() int { return d.Die(6) }

func (d *demoDice) Die(sides int) int {
	if d.fixed == nil {
		return rand.IntN(sides) + 1
	}
	return d.fixed.Die(sides)
}

func (d *demoDice) Roll(spec core.DiceSpec) int {
	n := 0
	for range spec.Count {
		n += d.Die(spec.Sides)
	}
	return n
}

// Set fixes the next results (the last one repeats); Random goes back to random dice.
func (d *demoDice) Set(rolls ...int) { d.fixed = &core.FixedDice{Rolls: rolls} }
func (d *demoDice) Random()          { d.fixed = nil }

// worldAbort carries a failed step out of the script.
type worldAbort struct{ err error }

// world is the script's state: the backend, the accounts and what could not be reached through the services.
type world struct {
	ctx   context.Context
	b     *backend
	dice  *demoDice
	pw    string
	out   io.Writer
	users map[string]core.User
	gaps  []string
	links []string
	// exec runs raw SQL for the few states no service reaches (nil: those states are skipped)
	exec func(ctx context.Context, query string, args ...any) error
	// scenarios of the default Battle Companies file, by name
	scenarios map[string]core.ScenarioInfo
	shipped   *core.RuleSet // the rules the app ships with, loaded on first use (rules)
	spareNext int           // next faction of the rules file for w.spare
	// the Aura parade: the default rules and the data.auras entries (by id) the companies of the script cover, with the companies
	rules       *core.RuleSet
	auraCovered map[string][]string
}

// wcamp is a campaign of the world with the actor who organises it.
type wcamp struct {
	id       int64
	name     string
	org      core.User
	settings core.CampaignSettings
}

// wco is a company of the world with its owner.
type wco struct {
	id    int64
	name  string
	user  core.User
	camp  *wcamp
	owner string
}

// must aborts the script when a step that has to work failed.
func (w *world) must(what string, err error) {
	if err != nil {
		panic(worldAbort{fmt.Errorf("%s: %w", what, err)})
	}
}

// try runs a step that may legitimately be refused by the rules (a limit reached, no influence left); it says so and goes on.
func (w *world) try(what string, err error) bool {
	if err != nil {
		fmt.Fprintf(w.out, "  (skipped) %s: %v\n", what, firstLine(err))
		return false
	}
	return true
}

func firstLine(err error) string {
	return strings.ReplaceAll(err.Error(), "\n", "; ")
}

// gap records a state of the wish list the services cannot produce (docs/demo-world.md prints them).
func (w *world) gap(format string, args ...any) {
	g := fmt.Sprintf(format, args...)
	w.gaps = append(w.gaps, g)
	fmt.Fprintln(w.out, "  GAP:", g)
}

func (w *world) c() *campaignsFacade { return w.b.campaigns }

// user returns the account registered under the key.
func (w *world) user(key string) core.User {
	u, ok := w.users[key]
	if !ok {
		panic(worldAbort{fmt.Errorf("no demo account %q", key)})
	}
	return u
}

// account makes (or finds) an account and signs it in to get its current record.
func (w *world) account(key, email, name string, role core.Role, canCreate bool) core.User {
	_, err := w.b.auth.CreateUser(w.ctx, email, name, w.pw)
	if err != nil && !errors.Is(err, core.ErrEmailTaken) {
		w.must("account "+email, err)
	}
	w.must("permissions of "+email, w.b.auth.SetPermissions(w.ctx, email, role, canCreate))
	_, _, u, err := w.b.auth.Login(w.ctx, email, w.pw)
	w.must("sign in "+email+" (is the password the one given to the first seed?)", err)
	w.users[key] = u
	return u
}

// campaign creates a campaign organised by the account.
func (w *world) campaign(org, name, description string, set core.CampaignSettings) *wcamp {
	return w.campaignWith(org, name, description, set, core.CampaignInput{})
}

// campaignWith is campaign with the rules files of in (empty: the app's defaults).
func (w *world) campaignWith(org, name, description string, set core.CampaignSettings, in core.CampaignInput) *wcamp {
	o := w.user(org)
	in.Name, in.Description, in.Settings = name, description, set
	s, err := w.c().Create(w.ctx, o, in)
	w.must("create campaign "+name, err)
	c := &wcamp{id: s.ID, name: name, org: o, settings: set}
	w.join(c, "admin", "steward") // the administrators follow every campaign they have to look after
	return c
}

// join makes the accounts members of the campaign through one invite link, like real players.
func (w *world) join(c *wcamp, keys ...string) {
	var inv core.Invite
	for i, k := range keys {
		if i == 0 {
			var err error
			inv, err = w.c().Invite(w.ctx, c.org, c.id)
			w.must("invite to "+c.name, err)
		}
		_, err := w.c().Join(w.ctx, w.user(k), inv.Token)
		w.must(k+" joins "+c.name, err)
	}
}

// company makes the account's company in the campaign (joining first when it is not a member); paths says whether its
// officers choose their paths (a company with a pending path cannot play).
func (w *world) company(c *wcamp, key, name, faction string, paths bool) *wco {
	u := w.user(key)
	if _, err := w.c().Get(w.ctx, u.ID, c.id); err != nil {
		w.join(c, key)
	}
	co, err := w.c().CreateCompany(w.ctx, u, c.id, core.CompanyInput{Name: name, Faction: faction})
	w.must("company "+name, err)
	x := &wco{id: co.ID, name: name, user: u, camp: c, owner: key}
	if paths {
		w.choosePaths(x, 0)
	}
	return x
}

// choosePaths gives every officer without one a path: the shift-th unblocked path, then the following ones for the next
// officers, so that companies differ.
func (w *world) choosePaths(co *wco, shift int) {
	d := w.detail(co)
	n := 0
	for _, u := range d.Roster {
		if !u.PathPending {
			continue
		}
		offer, err := w.c().PathOptions(w.ctx, co.user, co.camp.id, co.id, u.ID)
		w.must("path options", err)
		var open []core.PathOption
		for _, p := range offer.Paths {
			if p.Blocked == "" && !p.StartPower {
				open = append(open, p)
			}
		}
		p := open[(shift+n)%len(open)]
		n++
		w.must("choose path for "+u.Title(), w.c().ChoosePath(w.ctx, co.user, co.camp.id, co.id, u.ID, p.Name, ""))
	}
}

func (w *world) detail(co *wco) core.CompanyDetail {
	d, err := w.c().CompanyDetail(w.ctx, co.camp.id, co.id)
	w.must("detail of "+co.name, err)
	return d
}

// unit finds the first unit of the roster the test accepts.
func (w *world) unit(co *wco, ok func(core.CompanyUnit) bool) (core.CompanyUnit, bool) {
	for _, u := range w.detail(co).Roster {
		if ok(u) {
			return u, true
		}
	}
	return core.CompanyUnit{}, false
}

// mustUnit is unit for a unit the script needs.
func (w *world) mustUnit(co *wco, what string, ok func(core.CompanyUnit) bool) core.CompanyUnit {
	u, found := w.unit(co, ok)
	if !found {
		panic(worldAbort{fmt.Errorf("%s has no %s", co.name, what)})
	}
	return u
}

func isRole(r core.UnitRole) func(core.CompanyUnit) bool {
	return func(u core.CompanyUnit) bool { return u.Role == r }
}

// grant awards the company as its organiser does.
func (w *world) grant(co *wco, gold, influence int) {
	_, err := w.c().GrantReward(w.ctx, co.camp.org, co.camp.id, co.id, core.GrantInput{Gold: gold, Influence: influence, Note: "Demo purse"})
	w.must("grant "+co.name, err)
}

// xp awards experience to a unit (the organiser's grant).
func (w *world) xp(co *wco, unitID int64, n int, note string) {
	_, err := w.c().GrantReward(w.ctx, co.camp.org, co.camp.id, co.id, core.GrantInput{Experience: n, UnitID: unitID, Note: note})
	w.must("experience for "+co.name, err)
}

// rollWith makes the next server rolls the given results.
func (w *world) rollWith(rolls ...int) { w.dice.Set(rolls...) }

// ---------------------------------------------------------------------------------------------------------------------
// Recruiting

// recruit makes one recruit of the company: the roll on the reinforcements chart, then (when it leads there) on the special
// chart, answering whatever the result asks (a lower result, the options of a unit). It returns what the rules say joined, or
// the reason nothing could.
func (w *world) recruit(co *wco, adjust int, rolls ...int) (string, error) {
	next := func() int {
		if len(rolls) == 0 {
			return 3
		}
		r := rolls[0]
		rolls = rolls[1:]
		return r
	}
	roll := func() error {
		r := next()
		if co.camp.settings.ManualRolls {
			return w.c().RollRecruit(w.ctx, co.user, co.camp.id, co.id, r, false)
		}
		w.rollWith(r)
		return w.c().RollRecruit(w.ctx, co.user, co.camp.id, co.id, 0, false)
	}
	if err := roll(); err != nil {
		return "", err
	}
	summary := ""
	first := true
	for step := 0; step < 12; step++ {
		if ch, err := w.c().PendingChoice(w.ctx, co.user, co.camp.id, co.id); err != nil {
			return summary, err
		} else if ch != nil {
			opt := ""
			if len(ch.Options) > 0 {
				opt = ch.Options[step%len(ch.Options)].Name
			}
			res, err := w.c().ChooseOptions(w.ctx, co.user, co.camp.id, co.id, opt)
			if err != nil {
				return summary, err
			}
			summary += " " + res.Summary
			continue
		}
		st, err := w.c().RecruitState(w.ctx, co.user, co.camp.id, co.id)
		if err != nil {
			return summary, err
		}
		if st == nil {
			return strings.TrimSpace(summary), nil
		}
		switch {
		case st.Stage == "lower" && st.Lower != nil:
			done := false
			for _, row := range st.Lower.Rows {
				if row.Blocked != "" {
					continue
				}
				res, err := w.c().ChooseLower(w.ctx, co.user, co.camp.id, co.id, row.Index)
				if err != nil {
					continue
				}
				summary += " " + res.Summary
				done = true
				break
			}
			if !done {
				return strings.TrimSpace(summary), errors.New("no lower result can join")
			}
		case !st.Rolled:
			if err := roll(); err != nil {
				return strings.TrimSpace(summary), err
			}
		default:
			a := 0
			if first {
				a = adjust
			}
			first = false
			res, err := w.c().ConfirmRecruit(w.ctx, co.user, co.camp.id, co.id, a, "")
			if err != nil {
				return strings.TrimSpace(summary), err
			}
			summary += " " + res.Summary
		}
	}
	return strings.TrimSpace(summary), errors.New("the recruit did not settle")
}

// recruitAll recruits on every row of the reinforcements chart and, for the 6 that leads to the special chart, on every row of
// that one, as far as the company can pay and fit them.
func (w *world) recruitAll(co *wco) {
	var list [][]int
	for sr := 1; sr <= 6; sr++ {
		list = append(list, []int{6, sr})
	}
	list = append(list, []int{2}, []int{3}, []int{4}, []int{5}, []int{1})
	for _, rolls := range list {
		if _, err := w.recruit(co, 0, rolls...); err != nil {
			w.try(fmt.Sprintf("%s recruits on %v", co.name, rolls), err)
			w.abandonRecruit(co)
			if strings.Contains(err.Error(), "influence") || strings.Contains(err.Error(), "models") || strings.Contains(err.Error(), "full") {
				return
			}
		}
	}
}

// abandonRecruit settles a recruit left in progress by taking its first possible lower result, so the next one can start.
func (w *world) abandonRecruit(co *wco) {
	for i := 0; i < 8; i++ {
		st, err := w.c().RecruitState(w.ctx, co.user, co.camp.id, co.id)
		if err != nil || st == nil {
			return
		}
		if st.Lower != nil {
			for _, row := range st.Lower.Rows {
				if _, err := w.c().ChooseLower(w.ctx, co.user, co.camp.id, co.id, row.Index); err == nil {
					break
				}
			}
			continue
		}
		if st.Rolled {
			if _, err := w.c().ConfirmRecruit(w.ctx, co.user, co.camp.id, co.id, 0, ""); err != nil {
				return
			}
			continue
		}
		return
	}
}

// ---------------------------------------------------------------------------------------------------------------------
// Entry point

// seedWorld loads the whole world. It does nothing when the world is there already.
func seedWorld(ctx context.Context, b *backend, dice *demoDice, password string, out io.Writer, exec func(ctx context.Context, query string, args ...any) error) (err error) {
	if out == nil {
		out = os.Stdout
	}
	w := &world{ctx: ctx, b: b, dice: dice, pw: password, out: out, users: map[string]core.User{}, exec: exec, auraCovered: map[string][]string{}}
	defer func() {
		if r := recover(); r != nil {
			if a, ok := r.(worldAbort); ok {
				err = a.err
				return
			}
			panic(r)
		}
	}()
	w.accounts()
	if w.exists() {
		fmt.Fprintln(out, "The demo world exists already; nothing to do. (make demo starts from an empty database.)")
		return nil
	}
	w.loadScenarios()
	for _, step := range []struct {
		name string
		run  func()
	}{
		{"campaigns of the first steps", w.stepsCampaigns},
		{"gallery of companies", w.galleryCampaign},
		{"injuries and treatment", w.injuryCampaign},
		{"level-ups", w.levelCampaign},
		{"units, creatures, wanderers and limits", w.limitsCampaign},
		{"keyword targets", w.targetsCampaign},
		{"matches and scenarios", w.matchCampaign},
		{"aftermaths", w.aftermathCampaigns},
		{"settings, editions and the end", w.settingsCampaigns},
		{"aura parade", w.auraParade},
		{"cemetery, invites and handover", w.finishingTouches},
		{"several companies per player", w.bandsCampaign},
	} {
		fmt.Fprintln(out, "==", step.name)
		step.run()
		dice.Random()
	}
	w.printAccounts()
	w.printLinks()
	w.printGaps()
	return nil
}

func (w *world) exists() bool {
	list, err := w.c().List(w.ctx, w.user("you").ID, true)
	w.must("list campaigns", err)
	return slices.ContainsFunc(list, func(s core.CampaignSummary) bool { return s.Name == worldMarker })
}

func (w *world) loadScenarios() {
	// the campaign that exists first is a scratch one to read the scenarios of the default file
	w.scenarios = map[string]core.ScenarioInfo{}
}

var errNoWarrior = errors.New("the company has no warrior for the script")

func itoa(n int64) string { return fmt.Sprint(n) }
