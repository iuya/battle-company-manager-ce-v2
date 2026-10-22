package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
	"github.com/iuya/battle-company-manager-ce-v2/internal/postgres"
)

// The services are embedded under aliases so the field names do not hide the methods of the same name (Matches.Matches).
type (
	campaignsSvc = core.Campaigns
	companySvc   = core.CompanyService
	armourySvc   = core.ArmouryService
	recruitSvc   = core.RecruitService
	levelSvc     = core.LevelService
	treatSvc     = core.TreatmentService
	cemeterySvc  = core.CemeteryService
	matchesSvc   = core.Matches
	aftermathSvc = core.AftermathService
	invitesSvc   = core.Invites
	liveSvc      = core.LiveService
)

// campaignsFacade implements web.campaigns by embedding the core services: each owns its use cases, their method names are
// disjoint, so they are promoted as they are.
type campaignsFacade struct {
	*campaignsSvc
	*companySvc
	*armourySvc
	*recruitSvc
	*levelSvc
	*treatSvc
	*cemeterySvc
	*matchesSvc
	*aftermathSvc
	*invitesSvc
	*liveSvc
}

// backend is everything the server and the seed command use, built over one store.
type backend struct {
	campaigns  *campaignsFacade
	store      *postgres.Store
	rulesets   *core.Rulesets
	glossaries *core.Glossaries
	auth       *core.Auth
	liveHub    *core.LiveHub // main starts it (Start) and closes it on shutdown
}

// rulesCacheMax is MAX_CACHED_RULESETS as run read it; 0 (the commands that do not load the server config) keeps core's default.
var rulesCacheMax int

// newBackend builds the services over the Postgres store, registers the rules files the app ships with (read from
// definitions/, so run from the repository root) and returns them.
func newBackend(ctx context.Context, store *postgres.Store) (*backend, error) {
	return newBackendWithDice(ctx, store, core.RandomDice{})
}

// newBackendWithDice is newBackend with the dice the services roll when a player lets the app roll (the demo world fixes them).
func newBackendWithDice(ctx context.Context, store *postgres.Store, dice core.Dice) (*backend, error) {
	files := map[string][]byte{}
	for _, path := range []string{
		"definitions/vanilla/data2024.json", "definitions/battle_companies/battle_companies.json", "definitions/glossary/mesbg_glossary.json",
		"definitions/vanilla/data2024.schema.json", "definitions/battle_companies/battle_companies.schema.json", "definitions/glossary/mesbg_glossary.schema.json",
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files[path] = b
	}
	vanilla, bc, glossary := files["definitions/vanilla/data2024.json"], files["definitions/battle_companies/battle_companies.json"], files["definitions/glossary/mesbg_glossary.json"]
	schemas := map[core.RulesetKind][]byte{
		core.KindVanilla:         files["definitions/vanilla/data2024.schema.json"],
		core.KindBattleCompanies: files["definitions/battle_companies/battle_companies.schema.json"],
		core.KindGlossary:        files["definitions/glossary/mesbg_glossary.schema.json"],
	}
	rulesets, err := core.NewRulesets(store.Rulesets(), schemas)
	if err != nil {
		return nil, err
	}
	if err := rulesets.RegisterDefaults(ctx, vanilla, bc, glossary); err != nil {
		return nil, err
	}

	log := slog.Default()
	rules := core.NewRules(store.Rulesets(), log).WithMaxCached(rulesCacheMax)
	camps, comps, matches, aftermaths := store.Campaigns(), store.Companies(), store.Matches(), store.Aftermaths()

	aftermath := core.NewAftermathService(camps, comps, matches, store.Rulesets(), aftermaths, rules, dice).WithATO(store.ATO())
	matchSvc := core.NewMatches(camps, comps, matches, store.Rulesets(), rules).WithATO(store.ATO())
	matchSvc.AftermathStarted = aftermaths.Started           // a match cannot be reopened once a company started its aftermath
	matchSvc.CompanyAftermaths = aftermath.PendingAftermaths // a company with an unfinished aftermath cannot start a match
	hub := core.NewLiveHub(store.LiveFeed())
	live := core.NewLiveService(store.Live(), camps, comps, matches, rules).WithHub(hub).WithATO(store.ATO())
	matchSvc.LiveInit = live.Init                                                                 // the live data of a match is made when it is created (and on the first read otherwise)
	matchSvc.EndNotify = func(ctx context.Context, ch core.EndChange) { live.RecordEnd(ctx, ch) } // the log lines of a pending end
	live.WithEnds(matchSvc)                                                                       // a game action cancels a pending end
	aftermath.WithLive(live)                                                                      // the default aftermath sheet comes from the live match

	return &backend{
		campaigns: &campaignsFacade{
			campaignsSvc: core.NewCampaigns(camps, comps, store.Rulesets(), rules, log),
			companySvc:   core.NewCompanyService(camps, comps, rules).WithAftermaths(aftermaths),
			armourySvc:   core.NewArmouryService(camps, comps, rules),
			recruitSvc:   core.NewRecruitService(camps, comps, rules, dice),
			levelSvc:     core.NewLevelService(camps, comps, rules, dice),
			treatSvc:     core.NewTreatmentService(camps, comps, rules, dice),
			cemeterySvc:  core.NewCemeteryService(camps, comps, rules),
			matchesSvc:   matchSvc,
			aftermathSvc: aftermath,
			invitesSvc:   core.NewInvites(camps, store.Invites()),
			liveSvc:      live,
		},
		liveHub:    hub,
		store:      store,
		rulesets:   rulesets,
		glossaries: core.NewGlossaries(camps, rules, store.Rulesets()),
		auth:       core.NewAuth(store),
	}, nil
}
