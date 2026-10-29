// Package web is the HTTP adapter: handlers, templates and static assets. It depends on the core only through the
// interfaces declared here.
package web

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core"
)

// liveMatches is what the live match pages need (docs/live-match.md "Go contracts"); core.LiveService implements it.
//
//go:generate go tool mockgen -typed -source=server.go -destination=mock_server_test.go -package=web
type liveMatches interface {
	// Live is the live state of a company in an open match; core.ErrNotFound when it is in none (the page is the usual one).
	Live(ctx context.Context, user core.User, id, companyID int64) (core.LiveCompany, error)
	// Apply records an action; core.ErrRefused (shown), core.ErrInvalidLive and core.ErrNotFound.
	Apply(ctx context.Context, user core.User, id, matchID, companyID int64, a core.LiveAction) (core.LiveCompany, core.LiveEntry, error)
	// Undo removes an entry; core.ErrLiveDepends (a refusal) when a later entry needs it.
	Undo(ctx context.Context, user core.User, id, matchID, entryID, companyID int64) (core.LiveCompany, error)
	// Log lists the not-undone entries newest first; before is a seq to page from (0: the newest). It also works after the match.
	Log(ctx context.Context, user core.User, id, matchID, before int64, limit int) ([]core.LiveEntry, error)
	// Subscribe feeds the SSE handler; core.ErrRefused at the stream caps.
	Subscribe(ctx context.Context, user core.User, id, matchID int64) (core.LiveStream, error)
	// EndChecks is the card of the end of the game (objective flags, MVP ballots, tally, what is still open); core.ErrNotFound when the
	// match is not open.
	EndChecks(ctx context.Context, user core.User, id, matchID int64) (core.EndChecks, error)
	// SecretObjectives is what the viewer may see of the secret objectives (their own while the match is open, everybody's after it);
	// SecretScore the Victory Points the reported tiers of a company add up to, for the result form (ok false before the end or when none).
	SecretObjectives(ctx context.Context, user core.User, id, matchID int64) (core.SecretView, error)
	SecretScore(ctx context.Context, user core.User, id, matchID, companyID int64) (points int, lines []core.SecretLine, ok bool, err error)
	// RollCourage rolls (or reads the typed dice of) a Courage test of a unit in the Field; it writes nothing, the player records the
	// result with Apply. core.ErrRefused (not an editor, roll refused, unit not in the Field or without Courage), core.ErrNotFound.
	RollCourage(ctx context.Context, user core.User, id, matchID, unitID int64, entered []int, thrown bool) (core.CourageRoll, error)
	// MatchDefaults is what the live log says about the result form: who fled (All is Lost) and for whom every opposing company is
	// Broken. ok is false without live data; there is no user: the handler gates.
	MatchDefaults(ctx context.Context, id, matchID int64) (core.LiveMatchDefaults, bool, error)
}

// ttsExporter is what the copy button needs (docs/tabletop-simulator.md); core.TTSService implements it. The handler has checked that
// the viewer may see the company.
type ttsExporter interface {
	// TTSExport is the company as an army list; live (the company's state in an open match) puts the units that are wounded or out of
	// the fight in a second warband; nil is the plain list. core.ErrNotFound for an unknown company.
	TTSExport(ctx context.Context, campaignID, companyID int64, live *core.TTSLive, opts core.TTSOptions) (core.TTSExport, error)
	// TTSNames is the ArmyNames JSON v1 (level B) of the company for that export, so the list and the names agree.
	TTSNames(ctx context.Context, campaignID, companyID int64, exp core.TTSExport, opts core.TTSNamesOptions) (core.TTSNamesPayload, error)
	// TTSAuras is the Auras section of the dialog for a selection: the kinds the company has, their rings and radii, what each switch adds and
	// the notes for rings left out (docs/aura-ranges.md). live and opts are those of TTSExport, so the counts agree with the figures.
	TTSAuras(ctx context.Context, campaignID, companyID int64, live *core.TTSLive, opts core.TTSOptions, sel core.AuraSelection) (core.AuraSummary, error)
}

type campaigns interface {
	liveMatches
	ttsExporter

	// List returns the user's campaigns: the unfinished ones first, then (when withFinished) the finished ones, each
	// group newest first.
	List(ctx context.Context, userID int64, withFinished bool) ([]core.CampaignSummary, error)
	// NavCampaigns is List for the sidebar: only id, name and status, nobody's company rated (no rules code, one statement).
	NavCampaigns(ctx context.Context, userID int64, withFinished bool) ([]core.CampaignNav, error)
	// Create returns an error wrapping core.ErrInvalidCampaign when the input is rejected.
	Create(ctx context.Context, owner core.User, in core.CampaignInput) (core.CampaignSummary, error)

	// The methods below return core.ErrNotFound for an unknown campaign, company or match. Get also returns the
	// viewer's own companies in the campaign (CampaignDetail.Companies, oldest first; empty when they have none).
	//
	// Every owner action below takes the company id (companyID) right after the campaign id and returns core.ErrNotFound when it is not
	// one of the user's own companies there.
	Get(ctx context.Context, userID, id int64) (core.CampaignDetail, error)
	Companies(ctx context.Context, id int64) ([]core.CampaignCompany, error)
	// Members lists everyone in the campaign (also members without a company), the organiser first; the handover form offers the others.
	Members(ctx context.Context, id int64) ([]core.CampaignMember, error)
	Company(ctx context.Context, id, companyID int64) (core.CampaignCompany, error)
	// CompanyDetail is a company with its resources, limits and units, in the company's order.
	CompanyDetail(ctx context.Context, id, companyID int64) (core.CompanyDetail, error)
	// Armoury lists what a unit of the user's company could buy, each item with the reason it cannot be bought now (empty
	// when it can).
	Armoury(ctx context.Context, id, companyID, unitID int64) ([]core.ArmouryItem, error)
	// LevelUpState, RollLevelUp and ChooseLevelUp run a unit's level-up: the roll (a warrior's D6, or a hero's two 2D6 sums; empty or
	// zero for the app to roll) and then the choice among the results it offers. A roll in progress waits for its choice.
	LevelUpState(ctx context.Context, user core.User, id, companyID, unitID int64) (*core.LevelRoll, error)
	RollLevelUp(ctx context.Context, user core.User, id, companyID, unitID int64, entered []int, thrown bool) (core.LevelRoll, error)
	ChooseLevelUp(ctx context.Context, user core.User, id, companyID, unitID int64, keys []string) (core.LevelRoll, error)
	// RerollLevelUp spends an Against the Odds re-roll on one result of the roll in progress (which: the hero's result, 0 or 1).
	RerollLevelUp(ctx context.Context, user core.User, id, companyID, unitID int64, which int) (core.LevelRoll, error)
	// ArmouryCreatures and ArmouryWanderers list what the armoury sells besides gear. A creature goes to a leader or a sergeant and
	// takes no roster slot; a wanderer joins as a unit. BuyCreature, HireWanderer and DismissWanderer act on them; Carry stores,
	// retrieves or swaps a unit's gear with the Baggage Pony.
	ArmouryCreatures(ctx context.Context, id, companyID int64) ([]core.ArmouryCreature, error)
	ArmouryWanderers(ctx context.Context, id, companyID int64) ([]core.ArmouryWanderer, error)
	BuyCreature(ctx context.Context, user core.User, id, companyID, ownerID int64, name, pay string) error
	HireWanderer(ctx context.Context, user core.User, id, companyID int64, name, pay string) error
	DismissWanderer(ctx context.Context, user core.User, id, companyID, unitID int64) error
	Carry(ctx context.Context, user core.User, id, companyID, unitID int64, a core.CarryAction) error
	// ArmouryEntries lists every item the company's armoury sells, each with, per unit, whether it can buy it now and why not.
	ArmouryEntries(ctx context.Context, id, companyID int64) ([]core.ArmouryEntry, error)
	// SaveFormation stores the order the player put their units in (unit ids, first to last). Once saved it takes priority
	// over the default order for good; units it does not name (recruited since) follow in the default order. Unknown ids come
	// back as an error wrapping core.ErrRefused.
	SaveFormation(ctx context.Context, user core.User, id, companyID int64, order []int64) error
	// SaveCemeteryOrder stores the order of the fallen units, the same way; the cemetery and the roster stay apart.
	SaveCemeteryOrder(ctx context.Context, user core.User, id, companyID int64, order []int64) error
	// BuyEquipment, the steps of a recruit and RecruitSpecial spend the company's influence; what the rules do not allow
	// comes back as an error wrapping core.ErrRefused with the reason. A recruit is rolled (RollRecruit takes the player's
	// own roll, 0 means a random D6; the first roll pays the reinforcement price) and then confirmed (ConfirmRecruit takes
	// an adjustment of the roll, -3 to +3, 1 influence each across both charts). A "special chart" result keeps the recruit
	// going (RecruitResult.Next): roll again. RecruitState tells whether a recruit is in progress. A "choice of options"
	// result is left pending in the RecruitResult (Choice) until ChooseOptions; PendingChoice tells whether one is waiting.
	BuyEquipment(ctx context.Context, user core.User, id, companyID, unitID int64, item, pay string) error
	// BuyEquipmentFor buys an item that is a modifier of one weapon (Envenom Weapon) with the weapon the player chose among the unit's
	// PoisonableWeapons; ChooseModifierWeapon chooses it, once, for a legacy one the unit carries without a weapon (Equipment.Choices lists the weapons).
	BuyEquipmentFor(ctx context.Context, user core.User, id, companyID, unitID int64, item, pay, weapon string) error
	ChooseModifierWeapon(ctx context.Context, user core.User, id, companyID, unitID int64, modifier, weapon string) error
	// TreatInjury pays to treat a unit's injuries (costs and rolls come from the rules file). The result says what happened;
	// a failed attempt is not an error: its influence is lost.
	// RenameUnit gives a unit the name the player chose (empty: it goes back to its type). Rules that aim at a unit keep
	// following it by id.
	RenameUnit(ctx context.Context, user core.User, id, companyID, unitID int64, name string) error
	// RenameCompany gives the player's company a new name; the officers and the company itself are final, the name is not.
	RenameCompany(ctx context.Context, user core.User, id, companyID int64, name string) error
	// PathOptions and ChoosePath: a leader or sergeant starts without a path and chooses one from the company page (a Sorcerer also
	// its first magical power). The company cannot play a match before every hero has one.
	PathOptions(ctx context.Context, user core.User, id, companyID, unitID int64) (core.PathOffer, error)
	ChoosePath(ctx context.Context, user core.User, id, companyID, unitID int64, path, power string) error
	// TreatmentState is the treatment roll of a unit that waits for the decision on extra influence (nil when none).
	TreatmentState(ctx context.Context, user core.User, id, companyID, unitID int64) (*core.TreatResult, error)
	TreatInjury(ctx context.Context, user core.User, id, companyID, unitID int64, t core.Treatment) (core.TreatResult, error)
	// DropEquipment makes a unit drop any item it carries, also one of its profile (Battle Companies allows it); nothing is
	// refunded, and slots and company limits are freed.
	DropEquipment(ctx context.Context, user core.User, id, companyID, unitID int64, item string) error
	RecruitState(ctx context.Context, user core.User, id, companyID int64) (*core.RecruitRoll, error)
	// thrown says the browser decided the roll (Shake the table); a backend refuses a roll the campaign settings do not allow
	// (core.CampaignSettings.RollRefusal), also for a typed roll in TreatInjury and RollLevelUp.
	RollRecruit(ctx context.Context, user core.User, id, companyID int64, roll int, thrown bool) error
	// RollRecruitWithPerk is RollRecruit with the Rendezvous bonus (perkAdjust -1, 0 or +1; needs an unused Rendezvous perk).
	RollRecruitWithPerk(ctx context.Context, user core.User, campaignID, companyID int64, roll int, thrown bool, perkAdjust int) error
	// UseVantagePoint moves the second die of the scenario roll of an open match by +1 or -1 with the company's perk; it returns the moved die and the line.
	UseVantagePoint(ctx context.Context, user core.User, id, matchID, companyID int64, second, adjust int) (int, string, error)
	ConfirmRecruit(ctx context.Context, user core.User, id, companyID int64, adjust int, pick string) (core.RecruitResult, error)
	ChooseLower(ctx context.Context, user core.User, id, companyID int64, index int) (core.RecruitResult, error)
	RecruitSpecial(ctx context.Context, user core.User, id, companyID int64, unit string) (core.RecruitResult, error)
	ChooseOptions(ctx context.Context, user core.User, id, companyID int64, option string) (core.RecruitResult, error)
	PendingChoice(ctx context.Context, user core.User, id, companyID int64) (*core.RecruitChoice, error)
	// PendingOfficers is the officer panel of a Ranger of the North waiting to take command (nil when nothing waits);
	// ArrangeOfficers settles it with the player's arrangement (the Ranger is core.RangerID, 0).
	PendingOfficers(ctx context.Context, user core.User, id, companyID int64) (*core.OfficerPanel, error)
	ArrangeOfficers(ctx context.Context, user core.User, id, companyID int64, a core.OfficerArrangement) (core.RecruitResult, error)
	// CompanyProfiles are the companies a player can choose, each with everything about it (see core.CompanyProfile).
	CompanyProfiles(ctx context.Context, campaignID int64) ([]core.CompanyProfile, error)
	// CreateCompany returns core.ErrCompanyLimit when the user owns the companies the campaign allows (CampaignSettings.CompanyCap), or an error
	// wrapping core.ErrInvalidCompany (also core.ErrCompanyNameTaken: another company of theirs has that name).
	CreateCompany(ctx context.Context, user core.User, id int64, in core.CompanyInput) (core.CampaignCompany, error)
	Matches(ctx context.Context, id int64) ([]core.Match, error) // newest first
	Match(ctx context.Context, id, matchID int64) (core.Match, error)
	// CreateMatch returns an error wrapping core.ErrInvalidMatch when the sides are rejected (see core.ValidateSides).
	CreateMatch(ctx context.Context, user core.User, id int64, sides []core.NewSide) (core.Match, error)

	// The match layer (docs/matches.md). Every write takes the acting user and the Match.Revision the form was rendered with;
	// all return core.ErrNotFound for an unknown campaign or match, an error wrapping core.ErrRefused when Match.Access says
	// no, core.ErrMatchChanged for a stale revision and an error wrapping core.ErrInvalidMatch for bad input.
	// SetScenario chooses or replaces the scenario (a rulebook name must be in the campaign's rules file) and clears the
	// objectives reported; ClearScenario removes it.
	SetScenario(ctx context.Context, user core.User, id, matchID int64, revision int, in core.ScenarioInput) (core.Match, error)
	ClearScenario(ctx context.Context, user core.User, id, matchID int64, revision int) (core.Match, error)
	// SaveResult replaces the whole reported result (see core.Match.ValidateResult) and sets Match.Winner.
	SaveResult(ctx context.Context, user core.User, id, matchID int64, revision int, r core.MatchResult) (core.Match, error)
	// ProposeEnd starts the end of the match (docs/matches.md "Ending a match: confirmation by all participants"): refused with the
	// CanEnd reasons, it ends the match at once when nobody else has to confirm. ConfirmEnd, WithdrawEnd and CancelEnd answer a pending
	// end; OverrideEnd (organiser or administrator, note of 1-200 characters) ends it without the companies still waiting.
	// ReopenMatch is for administrators.
	ProposeEnd(ctx context.Context, user core.User, id, matchID int64, revision int) (core.Match, error)
	ConfirmEnd(ctx context.Context, user core.User, id, matchID int64, revision int) (core.Match, error)
	WithdrawEnd(ctx context.Context, user core.User, id, matchID int64, revision int) (core.Match, error)
	CancelEnd(ctx context.Context, user core.User, id, matchID int64, revision int) (core.Match, error)
	OverrideEnd(ctx context.Context, user core.User, id, matchID int64, revision int, note string) (core.Match, error)
	ReopenMatch(ctx context.Context, user core.User, id, matchID int64, revision int) (core.Match, error)
	// SetSideRoles tags the sides of an open match (one of "", "attacker", "defender" per side, in order); SwapSideRoles swaps the two teams' tags.
	SetSideRoles(ctx context.Context, user core.User, id, matchID int64, revision int, roles []string) (core.Match, error)
	SwapSideRoles(ctx context.Context, user core.User, id, matchID int64, revision int) (core.Match, error)
	// AgainstTheOdds loads the Against the Odds card for the viewer (never an error for "not eligible": ATO.Eligible false with Why);
	// SaveAgainstTheOdds replaces the lower company's whole selection (an empty one clears it): core.ErrRefused (not allowed or the game
	// began, reason fit to show), core.ErrMatchChanged (stale match revision), core.ErrInvalidMatch (refusals fit to show). Ambush! also
	// sets the match's scenario.
	AgainstTheOdds(ctx context.Context, user core.User, id, matchID int64) (core.ATO, error)
	SaveAgainstTheOdds(ctx context.Context, user core.User, id, matchID int64, revision int, sel core.ATOSelection) (core.ATO, error)
	// Scenarios lists what the scenario picker offers: the campaign's rules file, by table and roll.
	Scenarios(ctx context.Context, id int64) ([]core.ScenarioInfo, error)
	// ScenarioSelection is how the campaign's rules pick a scenario by dice: the size of the two dice and the range of the first that picks each table.
	ScenarioSelection(ctx context.Context, id int64) (core.ScenarioSelection, error)
	// GrantReward lets the organiser (or an administrator) award a company Gold, Influence and Experience with a note; the
	// company's resources change at once. An error wrapping core.ErrRefused for anyone else (core.CampaignDetail.CanGrant) and
	// for a negative result, core.ErrInvalidCompany for bad input (core.GrantInput.Validate). Grants is the company's history
	// of awards, newest first.
	GrantReward(ctx context.Context, user core.User, id, companyID int64, g core.GrantInput) (core.Grant, error)
	Grants(ctx context.Context, id, companyID int64) ([]core.Grant, error)

	// The War's Aftermath of an ended match (docs/aftermath.md). All take the acting user, return core.ErrNotFound for an
	// unknown campaign, match or company (or one not in the match), an error wrapping core.ErrRefused (match open, not
	// allowed, wrong step, reason fit to show), core.ErrAftermathChanged for a stale revision and an error wrapping
	// core.ErrInvalidAftermath for bad input. Aftermath loads the state (a default sheet when none is saved).
	Aftermath(ctx context.Context, user core.User, id, matchID, companyID int64) (core.Aftermath, error)
	// AftermathRules is what the aftermath pages read from the campaign's rules file: the ways to earn experience, the Against the
	// Odds bonuses, the injuries charts and the scenario rewards.
	AftermathRules(ctx context.Context, id int64) (core.AftermathRules, error)
	// FreeEquipmentOffers lists the armoury items worth at most maxInfluenceValue that units of the company can take for free (the
	// To the Victor the Spoils reward), each with the units that can take and carry it.
	FreeEquipmentOffers(ctx context.Context, id, companyID int64, maxInfluenceValue int) ([]core.FreeEquipment, error)
	// AftermathStatuses lists the status of every company of the match; PendingAftermaths lists the aftermaths of a company
	// that still lock it.
	AftermathStatuses(ctx context.Context, id, matchID int64) ([]core.AftermathRef, error)
	PendingAftermaths(ctx context.Context, id, companyID int64) ([]core.AftermathRef, error)
	SaveAftermath(ctx context.Context, user core.User, id, matchID, companyID int64, revision int, in core.AftermathInput) (core.Aftermath, error)
	// StartAftermath saves the sheet and starts the injuries; ReopenSheet unlocks it again before the first roll.
	StartAftermath(ctx context.Context, user core.User, id, matchID, companyID int64, revision int, in core.AftermathInput) (core.Aftermath, error)
	ReopenSheet(ctx context.Context, user core.User, id, matchID, companyID int64, revision int) (core.Aftermath, error)
	// RollCasualty rolls one casualty, or every unrolled one when unitID is 0 (then entered must be nil); entered holds the two
	// dice of a typed roll and thrown says the browser threw them (core.CampaignSettings.RollRefusal applies).
	RollCasualty(ctx context.Context, user core.User, id, matchID, companyID, unitID int64, entered []int, thrown bool) (core.Aftermath, error)
	// RerollCasualty spends one Against the Odds re-roll on a rolled casualty that is not final; entered holds the two typed dice
	// (nil: the server rolls), core.CampaignSettings.RollRefusal applies.
	RerollCasualty(ctx context.Context, user core.User, id, matchID, companyID, unitID int64, entered []int, thrown bool) (core.Aftermath, error)
	// Decide answers an open decision (see core.AftermathAnswer).
	Decide(ctx context.Context, user core.User, id, matchID, companyID int64, answer core.AftermathAnswer) (core.Aftermath, error)
	// FinishAftermath applies the Influence and rewards and unlocks the company; WaiveAftermath (organiser and administrators)
	// unlocks it without effects, with a note of 1 to 200 characters.
	FinishAftermath(ctx context.Context, user core.User, id, matchID, companyID int64, revision int) (core.Aftermath, error)
	WaiveAftermath(ctx context.Context, user core.User, id, matchID, companyID int64, note string) (core.Aftermath, error)
	// Invite makes an invite link for the campaign (core.Invite carries the secret token, shown once): only its organiser and
	// administrators (core.ErrRefused), not once it is finished. Join makes the user a member of the campaign the token invites to
	// and returns its id; core.ErrNotFound for an unknown, expired or used-up token, a no-op when already a member.
	Invite(ctx context.Context, user core.User, campaignID int64) (core.Invite, error)
	// InvalidateInvites makes every link of the campaign stop working (returns how many there were); RegenerateInvite does that and
	// makes one new link (shown once). Managers only; RegenerateInvite is refused while the campaign is paused.
	InvalidateInvites(ctx context.Context, actor core.User, campaignID int64) (int, error)
	RegenerateInvite(ctx context.Context, actor core.User, campaignID int64) (core.Invite, error)
	// SetCampaignStatus pauses, resumes, ends (confirm = the exact campaign name) or reactivates (administrators only) a campaign;
	// core.ErrRefused with the reason otherwise.
	SetCampaignStatus(ctx context.Context, actor core.User, campaignID int64, to core.CampaignStatus, confirm string) error
	Join(ctx context.Context, user core.User, token string) (int64, error)
	Preview(ctx context.Context, user core.User, token string) (core.InvitePreview, error)
	// HandOver makes another member the organiser; the organiser or an administrator may (core.ErrRefused otherwise).
	HandOver(ctx context.Context, actor core.User, campaignID, newOrganiserID int64) error
	// SetCompaniesPerPlayer changes how many companies each player may have (1 to 4), at any time; lowering only blocks new ones. The
	// organiser and administrators (core.ErrRefused otherwise), core.ErrInvalidCampaign out of range.
	SetCompaniesPerPlayer(ctx context.Context, actor core.User, campaignID int64, n int) error
	// UsePerk marks one of the company's scenario perks used; only its owner may (core.ErrNotFound otherwise).
	UsePerk(ctx context.Context, user core.User, id, companyID, perkID int64) error
}

// glossaries gives the glossary to display, already linked (core.BuildGlossary).
type glossaries interface {
	// Default is the glossary the app ships with.
	Default(ctx context.Context) (core.Glossary, error)
	// ForCampaign is the glossary the campaign is played with; core.ErrNotFound for an unknown campaign.
	ForCampaign(ctx context.Context, campaignID int64) (core.Glossary, error)
	// ByHash is the glossary file with this hash alone ("use another glossary"); core.ErrNotFound for an unknown hash or
	// a file that is not a glossary.
	ByHash(ctx context.Context, hash string) (core.Glossary, error)
}

// rulesets stores the rules files campaigns are played with, addressed by content hash (core.RulesetHash).
type rulesets interface {
	Default(ctx context.Context, kind core.RulesetKind) (core.RulesetInfo, error)
	// Lookup returns core.ErrNotFound when no file with that hash is stored.
	Lookup(ctx context.Context, hash string) (core.RulesetInfo, error)
	// Find answers "do you already have this file?" for a file the browser has hashed: the stored file of that kind with that hash
	// (files are identified by their hash alone; name and version are ignored). core.ErrNotFound when there is none.
	Find(ctx context.Context, kind core.RulesetKind, hash, name, version string) (core.RulesetInfo, error)
	// StoreAs verifies an upload (hash and kind) and keeps it, recording who uploaded it unless the file is already stored (same hash; the stored file is returned then); invalid files come back as an error wrapping core.ErrInvalidRuleset.
	StoreAs(ctx context.Context, uploader core.User, kind core.RulesetKind, hash string, content []byte) (core.RulesetInfo, error)
	// StoreAllAs stores several uploaded files all or nothing: every file is checked first, the problems of all of them come back
	// together as one error wrapping core.ErrInvalidRuleset (errors.As to *core.RulesetReport lists them), and nothing is stored
	// unless every file is good. The infos come back in the order of the uploads.
	StoreAllAs(ctx context.Context, uploader core.User, uploads []core.RulesetUpload) ([]core.RulesetInfo, error)
	// CheckSet validates the files of a set without storing or creating anything (the dry run behind "Check this file" and the
	// online validator). A nil file is not given: it is not checked, and the Battle Companies file is checked against the
	// default vanilla file and glossary instead; the files that are given are what it is checked against. One report lists every
	// problem and warning, grouped by file; the error is non-nil, wraps core.ErrInvalidRuleset and is that report when a file has errors.
	CheckSet(ctx context.Context, set core.RulesetSet) (core.RulesetReport, error)
	// ListWithUsage, SetDefault, Delete and PruneUnused are the administration Rules files page; superusers only
	// (core.ErrRefused otherwise). Delete and PruneUnused spare defaults and files campaigns use.
	ListWithUsage(ctx context.Context, actor core.User) ([]core.RulesetUsage, error)
	SetDefault(ctx context.Context, actor core.User, hash string) error
	Delete(ctx context.Context, actor core.User, hash string) error
	PruneUnused(ctx context.Context, actor core.User) (int, error)
	// Download is a stored file's info and content (core.ErrNotFound for an unknown hash).
	Download(ctx context.Context, actor core.User, hash string) (core.RulesetInfo, []byte, error)
	// CampaignOptions are the settings the default Battle Companies file offers an organiser, the ones the app knows.
	CampaignOptions(ctx context.Context) ([]core.CampaignOption, error)
}

type auth interface {
	Login(ctx context.Context, email, password string) (token string, expires time.Time, user core.User, err error)
	Authenticate(ctx context.Context, token string) (user core.User, renewed time.Time, err error)
	Logout(ctx context.Context, token string) error
	UpdateProfile(ctx context.Context, userID int64, displayName string) (core.User, error)
	ChangePassword(ctx context.Context, userID int64, current, next, keepToken string) error
	SignOutOthers(ctx context.Context, userID int64, keepToken string) (int, error)
	CloseAccount(ctx context.Context, userID int64, password string) error
	SetAppearance(ctx context.Context, userID int64, palette, mode string) error
	// SetTimezone saves the zone times are shown in ("" for automatic); core.ErrInvalidUser for an unknown name.
	SetTimezone(ctx context.Context, userID int64, zone string) error
	// ListUsers and SetUserPermissions are the administration Users page; superusers only (core.ErrRefused otherwise).
	ListUsers(ctx context.Context, actor core.User, q string, limit, offset int) ([]core.AdminUser, int, error)
	SetUserPermissions(ctx context.Context, actor core.User, userID int64, role core.Role, canCreateCampaigns bool) error
}

type Config struct {
	StaticMaxAge time.Duration `env:"STATIC_MAX_AGE" default:"0s"`
	// AppEnv is "production" (the default: secure by default) or "development" (make run, dev and demo: plain http on localhost).
	// Anything but "production" is treated as development, so a zero Config (the tests) is development.
	AppEnv string `env:"APP_ENV" default:"production"`
	// SecureCookiesEnv is SECURE_COOKIES as written ("", "true" or "false"); Resolve turns it into SecureCookies: unset means on in
	// production and off in development.
	SecureCookiesEnv string `env:"SECURE_COOKIES"`
	// AllowInsecureCookies is the explicit opt-out that lets production start with SECURE_COOKIES=false (plain http on a private
	// network, a local try-out). Without it Resolve refuses that combination.
	AllowInsecureCookies bool `env:"ALLOW_INSECURE_COOKIES" default:"false"`
	// SecureCookies marks the session cookie Secure and turns on HSTS. Set by Resolve; tests set it directly.
	SecureCookies bool
	// CSPMode is the Content-Security-Policy mode: "off", "report" (the default: Content-Security-Policy-Report-Only plus the
	// /csp-report logger) or "enforce". See docs/operations.md, "Content-Security-Policy".
	CSPMode string `env:"CSP_MODE" default:"report"`
	// TrustProxy takes the client address for the login throttle from the X-Forwarded-For header (its last entry, the one the
	// reverse proxy in front of the app adds). Turn it on only behind such a proxy: without one the header can be forged.
	TrustProxy bool `env:"TRUST_PROXY" default:"false"`
	// StyleGuide serves /styleguide (the theme lab) and marks the pages as development. Off unless asked: make run, dev and
	// demo turn it on.
	StyleGuide bool `env:"STYLEGUIDE" default:"false"`
	// AppTimezone is the zone times are shown in for a visitor whose account has none and whose browser sent none (IANA name; UTC when empty).
	AppTimezone string `env:"APP_TIMEZONE"`
	// RequestTimeout is how long one request may work before its context is cancelled (queries stop, the answer is 503). net/http's
	// WriteTimeout only cuts the connection and never cancels r.Context(). The live stream (SSE) is exempt. 0 turns it off, which is
	// also what a zero Config (the tests) means.
	RequestTimeout time.Duration `env:"REQUEST_TIMEOUT" default:"30s"`
}

type Server struct {
	cfg       Config
	campaigns campaigns
	rulesets  rulesets
	glossary  glossaries
	concepts  concepts // the helper popups' graph; nil serves no popup (SetConcepts)
	auth      auth
	render    *renderer
	throttle  *loginThrottle
	// accountThrottle limits wrong current passwords on the Account page, per signed-in user.
	accountThrottle *loginThrottle
	die             func(sides int) int         // the server's die (1..sides); tests replace it
	ready           func(context.Context) error // the database check of /readyz (SetReadiness)
	cspReports      *reportLimiter
	validator       *validatorGate // limits of the online validator (validator.go)
	recent          recentMatches
	onceMu          sync.Mutex // guards onceSeen only
	onceStripes     [onceStripeCount]sync.Mutex
	onceSeen        map[int64]onceAnswer // by account: the latest guarded answer
}

func NewServer(cfg Config, campaigns campaigns, rulesets rulesets, glossary glossaries, auth auth) (*Server, error) {
	r, err := newRenderer()
	if err != nil {
		return nil, err
	}
	_ = staticFiles() // builds the embedded asset table now: a broken build fails at start, not at the first request
	return &Server{cfg: cfg, campaigns: campaigns, rulesets: rulesets, glossary: glossary, auth: auth, render: r, throttle: newLoginThrottle(), accountThrottle: newLoginThrottle(), cspReports: newReportLimiter(), validator: newValidatorGate(), die: func(sides int) int { return 1 + rand.IntN(sides) }}, nil
}

// Handler returns the fully wired http.Handler (routes + middleware).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.StripPrefix("/static/", noListing(s.staticHandler())))

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("POST /csp-report", s.handleCSPReport)

	mux.HandleFunc("GET /login", s.session(s.handleLoginPage))
	mux.HandleFunc("POST /login", s.session(s.handleLogin))
	mux.HandleFunc("POST /logout", s.session(s.handleLogout))

	mux.HandleFunc("GET /{$}", s.protected(s.handleHome))
	mux.HandleFunc("GET /campaigns", s.protected(s.handleCampaignList))
	mux.HandleFunc("GET /campaigns/new", s.protected(s.handleNewCampaignPage))
	mux.HandleFunc("POST /campaigns", s.protected(s.handleCreateCampaign))
	mux.HandleFunc("GET /campaigns/{id}", s.protected(s.handleCampaign("campaign_overview", "overview")))
	mux.HandleFunc("POST /campaigns/{id}/settings/companies", s.protected(s.once(s.handleSetCompanyCap)))
	mux.HandleFunc("POST /campaigns/{id}/invite", s.protected(s.once(s.handleInvite)))
	mux.HandleFunc("POST /campaigns/{id}/status", s.protected(s.once(s.handleCampaignStatus)))
	mux.HandleFunc("POST /campaigns/{id}/invites/invalidate", s.protected(s.once(s.handleInvalidateInvites)))
	mux.HandleFunc("POST /campaigns/{id}/invites/regenerate", s.protected(s.once(s.handleRegenerateInvite)))
	mux.HandleFunc("POST /campaigns/{id}/handover", s.protected(s.handleHandOver))
	mux.HandleFunc("GET /join/{token}", s.session(s.handleJoinPage))
	mux.HandleFunc("POST /join/{token}", s.protected(s.handleJoin))
	mux.HandleFunc("GET /campaigns/{id}/my-company", s.protected(s.handleMyCompanyAlias))
	mux.HandleFunc("GET /campaigns/{id}/my-company/{company}", s.protected(s.handleMyCompany))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/perks/{perk}/use", s.protected(s.once(s.handleUsePerk)))
	mux.HandleFunc("GET /campaigns/{id}/my-company/{company}/recruit", s.protected(s.handleRecruitPanel))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/recruit", s.protected(s.handleRecruit))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/formation", s.protected(s.handleSaveFormation))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/cemetery", s.protected(s.handleSaveCemetery))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/recruit/roll", s.protected(s.once(s.handleRollRecruit)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/recruit/confirm", s.protected(s.once(s.handleConfirmRecruit)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/recruit/lower", s.protected(s.once(s.handleChooseLower)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/recruit/choose", s.protected(s.once(s.handleChooseOptions)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/recruit/officers", s.protected(s.once(s.handleArrangeOfficers)))
	mux.HandleFunc("GET /campaigns/{id}/my-company/{company}/units/{unit}/levelup", s.protected(s.handleLevelUpPanel))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/levelup/roll", s.protected(s.once(s.handleLevelUpRoll)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/levelup/reroll", s.protected(s.once(s.handleLevelUpReroll)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/levelup/choose", s.protected(s.once(s.handleLevelUpChoose)))
	mux.HandleFunc("POST /campaigns/{id}/armory/creature", s.protected(s.once(s.handleBuyCreature)))
	mux.HandleFunc("POST /campaigns/{id}/armory/wanderer", s.protected(s.once(s.handleHireWanderer)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/dismiss", s.protected(s.once(s.handleDismissWanderer)))
	mux.HandleFunc("GET /campaigns/{id}/my-company/{company}/pack", s.protected(s.handlePackPanel))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/pack", s.protected(s.once(s.handleCarry)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/rename", s.protected(s.handleRenameUnit))
	mux.HandleFunc("GET /campaigns/{id}/my-company/{company}/units/{unit}/treat", s.protected(s.handleTreatPanel))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/treat", s.protected(s.once(s.handleTreatInjury)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/equipment/weapon", s.protected(s.once(s.handleChooseModifierWeapon)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/equipment/drop", s.protected(s.once(s.handleDropEquipment)))
	mux.HandleFunc("GET /campaigns/{id}/my-company/{company}/units/{unit}/armoury", s.protected(s.handleArmouryPanel))
	mux.HandleFunc("GET /campaigns/{id}/my-company/{company}/units/{unit}/path", s.protected(s.handlePathPanel))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/path", s.protected(s.once(s.handleChoosePath)))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/rename", s.protected(s.handleRenameCompany))
	mux.HandleFunc("POST /campaigns/{id}/my-company/{company}/units/{unit}/equipment", s.protected(s.once(s.handleBuyEquipment)))
	mux.HandleFunc("GET /campaigns/{id}/armory", s.protected(s.handleArmory))
	mux.HandleFunc("POST /campaigns/{id}/armory/buy", s.protected(s.once(s.handleArmoryBuy)))
	mux.HandleFunc("GET /campaigns/{id}/companies", s.protected(s.handleCampaign("campaign_companies", "companies")))
	mux.HandleFunc("GET /campaigns/{id}/rankings", s.protected(s.handleCampaign("campaign_rankings", "rankings")))
	mux.HandleFunc("GET /campaigns/{id}/matches", s.protected(s.handleCampaign("campaign_matches", "matches")))
	mux.HandleFunc("GET /campaigns/{id}/company/new", s.protected(s.handleNewCompanyPage))
	mux.HandleFunc("GET /campaigns/{id}/company/info", s.protected(s.handleCompanyInfo))
	mux.HandleFunc("POST /campaigns/{id}/company", s.protected(s.handleCreateCompany))
	mux.HandleFunc("GET /campaigns/{id}/companies/{company}", s.protected(s.handleCompany))
	mux.HandleFunc("GET /campaigns/{id}/companies/{company}/live", s.protected(s.handleLiveFragment))
	mux.HandleFunc("GET /campaigns/{id}/companies/{company}/tts", s.protected(s.handleTTS))
	mux.HandleFunc("GET /tts/helper.zip", s.protected(s.handleTTSHelper))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/live/{company}/units/{unit}/sheet/{kind}", s.protected(s.handleLiveSheet))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/live/{company}/units/{unit}/courage/roll", s.protected(s.handleCourageRoll))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/live/{company}/actions", s.protected(s.handleLiveAction))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/live/log/{entry}/undo", s.protected(s.handleLiveUndo))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/live/log", s.protected(s.handleLiveLog))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/secret", s.protected(s.handleSecretCard))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/secret/{company}", s.protected(s.handleSecretWrite))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/live/end", s.protected(s.handleEndChecks))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/live/end/reminder", s.protected(s.handleEndReminder))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/live/{company}/objective", s.protected(s.handleEndObjective))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/live/{company}/mvp", s.protected(s.handleEndMVP))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/events", s.protected(s.handleLiveEvents))
	mux.HandleFunc("GET /campaigns/{id}/matches/new", s.protected(s.handleNewMatchPage))
	mux.HandleFunc("POST /campaigns/{id}/matches", s.protected(s.handleCreateMatch))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}", s.protected(s.handleMatch))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/scenario", s.protected(s.handleScenarioCard))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/scenario", s.protected(s.handleSetScenario))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/scenario/roll", s.protected(s.handleScenarioRoll))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/scenario/remove", s.protected(s.handleClearScenario))
	mux.HandleFunc("GET /campaigns/{id}/to-wound", s.protected(s.handleToWound))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/ato", s.protected(s.handleATOCard))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/ato", s.protected(s.handleATOSave))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/result", s.protected(s.handleSaveResult))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/live/end/banner", s.protected(s.handleEndBanner))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/end/card", s.protected(s.handleEndCard))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/end/propose", s.protected(s.handleEnd(endPropose)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/end/confirm", s.protected(s.handleEnd(endConfirm)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/end/withdraw", s.protected(s.handleEnd(endWithdraw)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/end/cancel", s.protected(s.handleEnd(endCancel)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/end/override", s.protected(s.handleEnd(endOverride)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/roles", s.protected(s.handleSetRoles))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/roles/swap", s.protected(s.handleSwapRoles))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/reopen", s.protected(s.handleReopenMatch))
	mux.HandleFunc("GET /campaigns/{id}/matches/{match}/aftermath/{company}", s.protected(s.handleAftermath))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/aftermath/{company}/sheet", s.protected(s.handleSheet(false)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/aftermath/{company}/start", s.protected(s.handleSheet(true)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/aftermath/{company}/reopen-sheet", s.protected(s.handleReopenSheet))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/aftermath/{company}/roll", s.protected(s.once(s.handleCasualtyRoll)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/aftermath/{company}/reroll", s.protected(s.once(s.handleCasualtyReroll)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/aftermath/{company}/decide", s.protected(s.once(s.handleDecide)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/aftermath/{company}/finish", s.protected(s.once(s.handleFinishAftermath)))
	mux.HandleFunc("POST /campaigns/{id}/matches/{match}/aftermath/{company}/waive", s.protected(s.handleWaiveAftermath))
	mux.HandleFunc("POST /campaigns/{id}/companies/{company}/grants", s.protected(s.once(s.handleGrantReward)))
	mux.HandleFunc("GET /rulesets/{hash}", s.protected(s.handleRulesetLookup))
	mux.HandleFunc("GET /nav/campaigns", s.protected(s.handleNavCampaigns))
	mux.HandleFunc("GET /glossary", s.protected(s.handleGlossary))
	mux.HandleFunc("POST /glossary", s.protected(s.handleChooseGlossary))
	mux.HandleFunc("GET /campaigns/{id}/glossary", s.protected(s.handleCampaignGlossary))
	if s.concepts != nil {
		mux.HandleFunc("GET /campaigns/{id}/concepts/resolve", s.protected(s.handleConceptByName))
		mux.HandleFunc("GET /campaigns/{id}/concepts/{concept}", s.protected(s.handleConcept))
	}
	mux.HandleFunc("GET /account", s.protected(s.handleAccount))
	mux.HandleFunc("PUT /account/appearance", s.session(s.handleAccountAppearance))
	mux.HandleFunc("POST /account/timezone", s.protected(s.handleAccountTimezone))
	mux.HandleFunc("POST /account/profile", s.protected(s.handleAccountProfile))
	mux.HandleFunc("POST /account/password", s.protected(s.handleAccountPassword))
	mux.HandleFunc("POST /account/sessions/sign-out-others", s.protected(s.handleAccountSignOutOthers))
	mux.HandleFunc("POST /account/close", s.protected(s.handleAccountClose))
	mux.HandleFunc("GET /tools/validate", s.protected(s.handleValidatorPage))
	mux.HandleFunc("POST /tools/validate", s.protected(s.handleValidatorCheck))
	mux.HandleFunc("GET /admin", s.requireSuperuser(s.handleAdmin))
	mux.HandleFunc("GET /admin/users", s.requireSuperuser(s.handleAdminUsers))
	mux.HandleFunc("POST /admin/users/{id}/handover", s.requireSuperuser(s.handleAdminHandover))
	mux.HandleFunc("POST /admin/users/{id}/permissions", s.requireSuperuser(s.handleAdminPermissions))
	mux.HandleFunc("GET /admin/rulesets", s.requireSuperuser(s.handleAdminRulesets))
	mux.HandleFunc("POST /admin/rulesets", s.requireSuperuser(s.handleAdminRulesetUpload))
	mux.HandleFunc("POST /admin/rulesets/check", s.requireSuperuser(s.handleAdminRulesetCheck))
	mux.HandleFunc("POST /admin/rulesets/prune", s.requireSuperuser(s.handleAdminRulesetPrune))
	mux.HandleFunc("GET /admin/rulesets/{hash}/download", s.requireSuperuser(s.handleAdminRulesetDownload))
	mux.HandleFunc("POST /admin/rulesets/{hash}/default", s.requireSuperuser(s.handleAdminRulesetDefault))
	mux.HandleFunc("POST /admin/rulesets/{hash}/delete", s.requireSuperuser(s.handleAdminRulesetDelete))
	mux.HandleFunc("/", s.session(s.handleNotFound)) // anything no route matches
	if s.cfg.StyleGuide {
		mux.HandleFunc("GET /styleguide", s.session(s.handleStyleGuide))
	}

	// Rejects cross-site POSTs (CSRF) using the browser's Sec-Fetch-Site / Origin headers.
	// Browsers send CSP reports without the headers the check relies on; a report only becomes a log line, so it is let through.
	cop := http.NewCrossOriginProtection()
	cop.AddInsecureBypassPattern("POST /csp-report")
	return s.recoverer(requestTimeout(s.cfg.RequestTimeout, s.securityHeaders(logger(s.guardInput(gzipResponses(cop.Handler(mux)))))))
}

// pageBase is embedded in every page's data: what the layout needs to draw the header.
type pageBase struct {
	User *core.User
	Path string // current URL path, so the sidebar can mark the page you are on
	Dev  bool   // show the links to the developer tools (style guide)
	// NavInline says the page's data carries the sidebar's campaign list (Campaigns, as the main page's does): the sidebar draws it
	// in place and fetches /nav/campaigns only when the list changes.
	NavInline bool
	// NavList is the sidebar's campaign list when NavInline is set by base (a full page for a signed-in user); the main page
	// uses its own Campaigns instead.
	NavList []core.CampaignNav
	NavOwn  bool // the page's own Campaigns is the list (main page)
}

// bare is the page base without the sidebar's campaign list.
func (s *Server) bare(r *http.Request) pageBase {
	return pageBase{User: userFrom(r.Context()), Path: r.URL.Path, Dev: s.cfg.StyleGuide}
}

// base is bare plus, on a full page (not an htmx content swap, which has no sidebar), the sidebar's campaign list, so the
// browser needs no /nav/campaigns round trip. If the list cannot be read the sidebar falls back to fetching it lazily.
func (s *Server) base(r *http.Request) pageBase {
	b := s.bare(r)
	if b.User == nil || (r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-History-Restore-Request") != "true") {
		return b
	}
	if list, err := s.campaigns.NavCampaigns(r.Context(), b.User.ID, showFinished(r)); err == nil {
		b.NavList, b.NavInline = list, true
	} else {
		slog.Warn("sidebar campaign list failed, left to the lazy fetch", "err", err)
	}
	return b
}

// noListing answers 404 to a directory (the file server would list its files).
func noListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
