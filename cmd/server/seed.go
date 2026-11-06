package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

const seedUsage = `usage: server seed [--world] [--password <password>]

Fills a local development database with demo data: four accounts, a campaign with a company each, an ended match (its
aftermath waived by the organiser) and an open one. Safe to run twice: it does nothing when the demo campaign exists.

--world loads the rich demo world instead (docs/demo-world.md); make demo does that on an empty database.`

const demoCampaign = "The Siege of Minas Tirith"

// demoUsers are the accounts the seed command creates; the first is the superuser, the second organises the campaign.
var demoUsers = []struct {
	email, name string
	role        core.Role
	company     string
	faction     string
}{
	{"admin@example.com", "Gandalf", core.RoleSuperuser, "Wanderers of the White Council", "Wanderers in the Wild"},
	{"you@example.com", "You", core.RoleUser, "Rangers of Ithilien", "Osgiliath & Ithilien"},
	{"eowyn@example.com", "Éowyn", core.RoleUser, "Eorlings of the Mark", "Rohan"},
	{"sauron@example.com", "Sauron", core.RoleUser, "Black Gate Vanguard", "Mordor"},
}

// runSeed handles "server seed".
func runSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	password := fs.String("password", "demo-password", "password of every demo account (at least 10 characters)")
	world := fs.Bool("world", false, "load the rich demo world (many campaigns in every state) instead of the minimal demo")
	if err := fs.Parse(args); err != nil {
		//lint:ignore ST1005 the usage text is printed to the terminal as is, not wrapped into a longer message
		return errors.New(seedUsage)
	}
	ctx := context.Background()
	store, databaseURL, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer store.Close()
	if *world {
		dice := &demoDice{}
		b, err := newBackendWithDice(ctx, store, dice)
		if err != nil {
			return err
		}
		conn, err := pgx.Connect(ctx, databaseURL)
		if err != nil {
			return err
		}
		defer conn.Close(ctx)
		exec := func(ctx context.Context, query string, args ...any) error {
			_, err := conn.Exec(ctx, query, args...)
			return err
		}
		if err := seedWorld(ctx, b, dice, *password, os.Stdout, exec); err != nil {
			return fmt.Errorf("%w\n(a half-loaded database: run make db-reset and seed again)", err)
		}
		return nil
	}
	b, err := newBackend(ctx, store)
	if err != nil {
		return err
	}
	if err := seed(ctx, b, *password); err != nil {
		return fmt.Errorf("%w\n(a half-seeded database: run make db-reset and seed again)", err)
	}
	return nil
}

func seed(ctx context.Context, b *backend, password string) error {
	users := make([]core.User, len(demoUsers))
	for i, d := range demoUsers {
		_, err := b.auth.CreateUser(ctx, d.email, d.name, password)
		if err != nil && !errors.Is(err, core.ErrEmailTaken) {
			return fmt.Errorf("account %s: %w", d.email, err)
		}
		if err := b.auth.SetPermissions(ctx, d.email, d.role, i < 2); err != nil {
			return err
		}
		if _, _, users[i], err = b.auth.Login(ctx, d.email, password); err != nil {
			return fmt.Errorf("sign in %s (is the password the one given to the first seed?): %w", d.email, err)
		}
	}
	c := b.campaigns
	organiser := users[1]
	existing, err := c.List(ctx, organiser.ID, true)
	if err != nil {
		return err
	}
	for _, s := range existing {
		if s.Name == demoCampaign {
			fmt.Println("The demo campaign exists already; nothing to do.")
			return nil
		}
	}

	camp, err := c.Create(ctx, organiser, core.CampaignInput{Name: demoCampaign,
		Description: "A twelve-week campaign for the great houses of Gondor and their allies. Winners hold the White City.",
		Settings:    core.CampaignSettings{ShakeTheTable: true, ManualRolls: true}})
	if err != nil {
		return err
	}
	var companies []core.CampaignCompany
	for i, d := range demoUsers {
		if i != 1 { // the others follow an invite link, like real players
			inv, err := c.Invite(ctx, organiser, camp.ID)
			if err != nil {
				return err
			}
			if _, err := c.Join(ctx, users[i], inv.Token); err != nil {
				return err
			}
		}
		co, err := c.CreateCompany(ctx, users[i], camp.ID, core.CompanyInput{Name: d.company, Faction: d.faction})
		if err != nil {
			return fmt.Errorf("company of %s: %w", d.name, err)
		}
		if err := choosePaths(ctx, c, users[i], camp.ID, co.ID); err != nil {
			return fmt.Errorf("paths of %s: %w", d.company, err)
		}
		// influence and gold to spend: the organiser rewards everybody, like after the first games of a campaign
		if _, err := c.GrantReward(ctx, organiser, camp.ID, co.ID, core.GrantInput{Gold: 10, Influence: 6, Note: "Welcome to the campaign"}); err != nil {
			return err
		}
		companies = append(companies, co)
	}

	// an ended match (its aftermath waived), and an open one with three sides
	ended, err := c.CreateMatch(ctx, organiser, camp.ID, core.NewMatch{Sides: []core.NewSide{{Companies: []int64{companies[1].ID}}, {Companies: []int64{companies[3].ID}}}})
	if err != nil {
		return err
	}
	scenarios, err := c.Scenarios(ctx, camp.ID)
	if err != nil || len(scenarios) == 0 {
		return fmt.Errorf("the rules file offers no scenario: %v", err)
	}
	if ended, err = c.SetScenario(ctx, organiser, camp.ID, ended.ID, ended.Revision, core.ScenarioInput{Kind: core.ScenarioRulebook, Name: scenarios[0].Name}); err != nil {
		return err
	}
	if ended, err = c.SaveResult(ctx, organiser, camp.ID, ended.ID, ended.Revision, core.MatchResult{Decided: true, Winner: 1}); err != nil {
		return err
	}
	if _, err = endNow(ctx, c, organiser, camp.ID, ended.ID, ended.Revision); err != nil {
		return err
	}
	// a company with an aftermath still to play cannot start another match: the organiser waives Match 1's, through the service
	for _, co := range []core.CampaignCompany{companies[1], companies[3]} {
		if _, err = c.WaiveAftermath(ctx, organiser, camp.ID, ended.ID, co.ID, "Not played in the demo: waived to free the company"); err != nil {
			return fmt.Errorf("waive the aftermath of %s: %w", co.Name, err)
		}
	}
	if _, err = c.CreateMatch(ctx, organiser, camp.ID, core.NewMatch{Sides: []core.NewSide{{Companies: []int64{companies[1].ID}}, {Companies: []int64{companies[2].ID}}, {Companies: []int64{companies[3].ID}}}}); err != nil {
		return err
	}
	fmt.Printf("Seeded %q. Sign in with any of %s, %s, %s, %s and the password given (default demo-password).\n",
		demoCampaign, demoUsers[0].email, demoUsers[1].email, demoUsers[2].email, demoUsers[3].email)
	return nil
}

// choosePaths gives every hero of the company the first path it may take (a company cannot play before that), and a Sorcerer
// its first power.
func choosePaths(ctx context.Context, c *campaignsFacade, user core.User, campaignID, companyID int64) error {
	detail, err := c.CompanyDetail(ctx, campaignID, companyID)
	if err != nil {
		return err
	}
	for _, u := range detail.Roster {
		if !u.PathPending {
			continue
		}
		offer, err := c.PathOptions(ctx, user, campaignID, companyID, u.ID)
		if err != nil {
			return err
		}
		for _, p := range offer.Paths {
			if p.Blocked != "" {
				continue
			}
			power := ""
			if p.StartPower && len(offer.Powers) > 0 {
				power = offer.Powers[0].Key
			}
			if err := c.ChoosePath(ctx, user, campaignID, companyID, u.ID, p.Name, power); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

// endNow ends a match in the demo data and the seed: the organiser proposes, and overrides the companies of other players, with a note.
func endNow(ctx context.Context, c *campaignsFacade, organiser core.User, campaignID, matchID int64, revision int) (core.Match, error) {
	decided, err := decideATO(ctx, c, organiser, campaignID, matchID)
	if err != nil {
		return core.Match{}, err
	}
	revision = max(revision, decided.Revision)
	if err := voteMissing(ctx, c, campaignID, matchID); err != nil {
		return core.Match{}, err
	}
	m, err := c.ProposeEnd(ctx, organiser, campaignID, matchID, revision)
	if err != nil || m.Ended {
		return m, err
	}
	return c.OverrideEnd(ctx, organiser, campaignID, matchID, m.Revision, "Demo data: ended without the players' confirmation")
}

// decideATO confirms, as the organiser, the Against the Odds decision a match waits for (docs/matches.md, "Against the Odds before the game"),
// with whatever completes it (atoFill), and returns the match as it is now.
func decideATO(ctx context.Context, c *campaignsFacade, organiser core.User, campaignID, matchID int64) (core.Match, error) {
	m, err := c.Match(ctx, campaignID, matchID)
	if err != nil {
		return m, err
	}
	st, err := c.ATOStatus(ctx, campaignID, m)
	if err != nil || !st.Pending {
		return m, err
	}
	card, err := c.AgainstTheOdds(ctx, organiser, campaignID, matchID)
	if err != nil {
		return m, err
	}
	if _, err := c.SaveAgainstTheOdds(ctx, organiser, campaignID, matchID, m.Revision, atoFill(card, m, core.ATOSelection{})); err != nil {
		return m, err
	}
	return c.Match(ctx, campaignID, matchID)
}

// atoFill completes a selection until nothing else fits the gap, in the card's order: the bonuses that repeat take what they can, so Influence
// usually ends it; a practice match has no reward bonus and takes kits and Wanderers. Ambush!, when it comes to that, keeps the match's scenario.
func atoFill(card core.ATO, m core.Match, sel core.ATOSelection) core.ATOSelection {
	left := card.Gap - sel.SpentOn(card)
	fits := func(rating int) bool { return rating < left || (rating == left && card.Inclusive) }
	for _, o := range card.Options {
		if o.Disabled && !o.OverBudget || o.NoMore {
			continue
		}
		switch {
		case o.Key == "toolKit":
			for _, k := range card.Kits {
				if len(sel.Kits) < card.MaxKits && fits(o.Rating) && !slices.Contains(sel.Kits, k.Key) {
					sel.Kits, left = append(sel.Kits, k.Key), left-o.Rating
				}
			}
			if len(sel.Kits) > 0 && !sel.Has(o.Key) {
				sel.Bonuses = append(sel.Bonuses, o.Key)
			}
		case o.Key == "wanderer":
			for _, w := range card.Wanderers {
				if len(sel.Wanderers) < card.MaxWanderers && fits(o.Rating) && !slices.Contains(sel.Wanderers, w.Name) {
					sel.Wanderers, left = append(sel.Wanderers, w.Name), left-o.Rating
				}
			}
			if len(sel.Wanderers) > 0 && !sel.Has(o.Key) {
				sel.Bonuses = append(sel.Bonuses, o.Key)
			}
		case o.Key == "ambush":
			if !sel.Has(o.Key) && fits(o.Rating) && m.Scenario != nil {
				sel.Bonuses, sel.Ambush, left = append(sel.Bonuses, o.Key), m.Scenario.Name, left-o.Rating
			}
		default:
			for fits(o.Rating) && o.Rating > 0 && (o.Repeatable || !sel.Has(o.Key)) {
				sel.Bonuses, left = append(sel.Bonuses, o.Key), left-o.Rating
			}
		}
	}
	return sel
}

// voteMissing casts the MVP votes a competitive match still needs before it can end (docs/matches.md, "Ending a match"): each owner
// votes the first unit of every opposing company that took part, as the demo data does not say who stood out.
func voteMissing(ctx context.Context, c *campaignsFacade, campaignID, matchID int64) error {
	missing, err := c.liveSvc.MissingVotes(ctx, campaignID, matchID, 0)
	if err != nil {
		return err
	}
	for _, v := range missing {
		voter := core.User{ID: v.OwnerID, DisplayName: v.Owner}
		lc, err := c.Live(ctx, voter, campaignID, v.CompanyID)
		if err != nil {
			return fmt.Errorf("the live state of %s: %w", v.Company, err)
		}
		for _, u := range lc.Units {
			if u.Temporary {
				continue
			}
			if _, _, err := c.Apply(ctx, voter, campaignID, matchID, v.CompanyID, core.LiveAction{Kind: core.LiveMVPVote, UnitID: u.UnitID}); err != nil {
				return fmt.Errorf("%s votes the MVP of %s: %w", v.Owner, v.Company, err)
			}
			break
		}
	}
	return nil
}
