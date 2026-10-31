package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"time"

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
	ttsSvc       = core.TTSService
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
	*ttsSvc
}

// backend is everything the server and the seed command use, built over one store.
type backend struct {
	campaigns   *campaignsFacade
	store       *postgres.Store
	rulesets    *core.Rulesets
	glossaries  *core.Glossaries
	concepts    *core.Concepts // the helper popups' graph (docs/concept-graph.md)
	auth        *core.Auth
	invitations *core.AccountInvitations // account invitations (docs/auth.md)
	recovery    *core.Recovery           // password recovery links
	liveHub     *core.LiveHub            // main starts it (Start) and closes it on shutdown
}

// rulesCacheMax is MAX_CACHED_RULESETS as run read it; 0 (the commands that do not load the server config) keeps core's default.
var rulesCacheMax int

// rulesBuildTimeout is RULES_BUILD_TIMEOUT as run read it; 0 keeps core's default.
var rulesBuildTimeout time.Duration

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
		"definitions/vanilla/data2024.schema.json", "definitions/glossary/mesbg_glossary.schema.json",
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files[path] = b
	}
	vanilla, bc, glossary := files["definitions/vanilla/data2024.json"], files["definitions/battle_companies/battle_companies.json"], files["definitions/glossary/mesbg_glossary.json"]
	schemas := map[core.RulesetKind][]byte{ // the Battle Companies file has no JSON schema check: the lean decoder is its authority
		core.KindVanilla:  files["definitions/vanilla/data2024.schema.json"],
		core.KindGlossary: files["definitions/glossary/mesbg_glossary.schema.json"],
	}
	rulesets, err := core.NewRulesets(store.Rulesets(), schemas)
	if err != nil {
		return nil, err
	}
	if err := rulesets.RegisterDefaults(ctx, vanilla, bc, glossary); err != nil {
		return nil, err
	}

	log := slog.Default()
	rules := core.NewRules(store.Rulesets(), log).WithMaxCached(rulesCacheMax).WithBuildTimeout(rulesBuildTimeout)
	// the reads of a request that carries a read scope (the aftermath page) are made once; everything else passes through
	camps, comps, matches, aftermaths := core.ScopeCampaigns(store.Campaigns()), core.ScopeCompanies(store.Companies()), core.ScopeMatches(store.Matches()), store.Aftermaths()
	atoRepo, liveRepo := core.ScopeATO(store.ATO()), core.ScopeLive(store.Live())

	aftermath := core.NewAftermathService(camps, comps, matches, aftermaths, rules, dice).WithATO(atoRepo)
	matchSvc := core.NewMatches(camps, comps, matches, rules).WithATO(atoRepo)
	matchSvc.AftermathStarted = aftermaths.Started           // a match cannot be reopened once a company started its aftermath
	matchSvc.CompanyAftermaths = aftermath.PendingAftermaths // a company with an unfinished aftermath cannot start a match
	hub := core.NewLiveHub(store.LiveFeed())
	live := core.NewLiveService(liveRepo, camps, comps, matches, rules).WithHub(hub).WithATO(atoRepo)
	matchSvc.LiveInit = live.Init                                                                 // the live data of a match is made when it is created (and on the first read otherwise)
	matchSvc.EndNotify = func(ctx context.Context, ch core.EndChange) { live.RecordEnd(ctx, ch) } // the log lines of a pending end
	live.WithEnds(matchSvc)                                                                       // a game action cancels a pending end
	aftermath.WithLive(live)                                                                      // the default aftermath sheet comes from the live match

	curated, err := loadConceptFile(log)
	if err != nil {
		return nil, err
	}

	auth := core.NewAuth(store)
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
			ttsSvc:       core.NewTTSService(camps, comps, rules).WithCatalogue(ttsCatalogue(log)),
		},
		liveHub:     hub,
		store:       store,
		rulesets:    rulesets,
		glossaries:  core.NewGlossaries(camps, rules, store.Rulesets()),
		concepts:    core.NewConcepts(rules, curated, log),
		auth:        auth,
		invitations: core.NewAccountInvitations(store.AccountInvitations()),
		recovery:    core.NewRecovery(store.Recovery(), auth),
	}, nil
}

// conceptGraphFile is the generated graph of curated edges (grants, cancels, lifts, requires) shipped next to the rules files.
const conceptGraphFile = "definitions/concepts/concept_graph.json"

// loadConceptFile reads the curated concept graph. A missing file is not fatal (the popups then show mentions and see-also only,
// which the graph computes from each ruleset's own text); a file that is there but cannot be read is, like the rules files.
func loadConceptFile(log *slog.Logger) (*core.ConceptFile, error) {
	b, err := os.ReadFile(conceptGraphFile)
	if errors.Is(err, fs.ErrNotExist) {
		log.Warn("no curated concept graph file; the helper popups show mentions and see-also only", "file", conceptGraphFile)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return core.ParseConceptFile(b)
}

// ttsCatalogue loads the optional Tabletop Simulator model catalogue named by TTS_CATALOGUE (the mod's content: never in the image);
// without it the lists never fall back to another figure.
func ttsCatalogue(log *slog.Logger) core.TTSCatalogue {
	path := os.Getenv("TTS_CATALOGUE")
	if path == "" {
		return nil
	}
	c, err := core.LoadTTSCatalogue(path)
	if err != nil {
		log.Warn("tts catalogue not loaded", "path", path, "err", err)
		return nil
	}
	return c
}
