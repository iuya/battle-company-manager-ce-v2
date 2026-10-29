package core

import "context"

// The repository interfaces below are the persistence contract of the backend (see docs/architecture.md, "Backend design").
// internal/postgres implements them; core services (Rulesets, Campaigns, ...) consume them. Each returns ErrNotFound for an
// unknown row and never leaks driver errors. Company state is companyRepository, below.

//go:generate go tool mockgen -typed -source=repositories.go -destination=mock_repositories_test.go -package=core

// StoredRuleset is one rules document as persisted. Content is the compacted JSON the hash was computed from.
type StoredRuleset struct {
	RulesetInfo
	Size       int
	Content    []byte
	UploadedBy int64 // user id; 0 for the files the app ships with
}

// rulesetRepository keeps rules files, content addressed.
type rulesetRepository interface {
	// Insert stores the file unless one with the same hash exists (then it is a no-op, never an error).
	Insert(ctx context.Context, r StoredRuleset) error
	// Get returns the file with that hash (content included), or ErrNotFound.
	Get(ctx context.Context, hash string) (StoredRuleset, error)
	// Info returns the same without the content, or ErrNotFound.
	Info(ctx context.Context, hash string) (RulesetInfo, error)
	// SetDefault makes the file the default of its kind and unmarks the previous one, in one transaction.
	SetDefault(ctx context.Context, kind RulesetKind, hash string) error
	// Default returns the default file of a kind, or ErrNotFound.
	Default(ctx context.Context, kind RulesetKind) (RulesetInfo, error)
	// ListUsage returns every stored file without its content, with its uploader and the campaigns using it, ordered by
	// kind, then newest first.
	ListUsage(ctx context.Context) ([]RulesetUsage, error)
	// Delete removes the file unless a campaign uses it or it is a default (ErrRefused with the reason); ErrNotFound for an
	// unknown hash. One guarded statement: the foreign keys of campaigns have the last word.
	Delete(ctx context.Context, hash string) error
	// DeleteUnused removes every file no campaign uses and that is not a default, and returns how many.
	DeleteUnused(ctx context.Context) (int, error)
}

// NewCampaign is a validated campaign ready to be stored: the hashes are resolved (never empty) and checked to exist with the
// right kind.
type NewCampaign struct {
	Name, Description                  string
	Vanilla, BattleCompanies, Glossary string
	Settings                           CampaignSettings
	Rounds                             int
}

// campaignRepository keeps campaigns and their membership. Companies are created through the company repository (a company
// row belongs to a campaign and to a user; a player may own up to CampaignSettings.CompanyCap() of them).
type campaignRepository interface {
	// List returns the user's campaigns: unfinished first, then (withFinished) finished, each newest first, one row per campaign;
	// Companies are the user's own companies in it, oldest first.
	List(ctx context.Context, userID int64, withFinished bool) ([]CampaignSummary, error)
	// Create stores the campaign and makes the owner its organiser and first member, atomically.
	Create(ctx context.Context, owner User, c NewCampaign) (CampaignSummary, error)
	// Get returns the campaign as the member sees it (ErrNotFound for non-members too, so ids do not leak), with the hashes of
	// its rules files.
	Get(ctx context.Context, userID, id int64) (CampaignDetail, error)
	// Companies lists the campaign's companies with their rating figures; Company returns one.
	Companies(ctx context.Context, id int64) ([]CampaignCompany, error)
	Company(ctx context.Context, id, companyID int64) (CampaignCompany, error)
	// Members lists the campaign's members once each, the organiser first, then by name, each with their companies (oldest first).
	// ErrNotFound for an unknown campaign.
	Members(ctx context.Context, id int64) ([]CampaignMember, error)
	// Hashes returns the rules file hashes of a campaign (vanilla, Battle Companies, glossary) and its settings; the loader keys on them.
	Hashes(ctx context.Context, id int64) (RulesetHashes, CampaignSettings, error)
	// SetStatus changes the status of a campaign under a lock: fn gets the organiser's id, the campaign's name and the current status and returns the new
	// one (or an error, which is returned as is and changes nothing). Becoming finished also revokes every invitation link, in the
	// same transaction. ErrNotFound for an unknown campaign. Members or not: the caller decides who may.
	SetStatus(ctx context.Context, campaignID int64, fn func(organiserID int64, name string, current CampaignStatus) (CampaignStatus, error)) error
	// HandOver makes newOrganiser the organiser and the previous organiser a player, in one transaction. allowed is called
	// inside it with the current organiser's id and may refuse (its error is returned as is). ErrNotFound for an unknown
	// campaign; ErrRefused when newOrganiser is not a member of the campaign.
	HandOver(ctx context.Context, campaignID, newOrganiser int64, allowed func(currentOrganiser int64) error) error
	// UpdateSettings changes the settings under a lock: fn gets the organiser's id, the status and the current settings and returns the
	// new ones (or an error, returned as is, changing nothing). ErrNotFound for an unknown campaign.
	UpdateSettings(ctx context.Context, campaignID int64, fn func(organiserID int64, status CampaignStatus, current CampaignSettings) (CampaignSettings, error)) error
}

// RulesetHashes names the three files a campaign is played with. It is the key of the loader's cache.
type RulesetHashes struct{ Vanilla, BattleCompanies, Glossary string }

// companyRepository keeps the company aggregate (see docs/architecture.md, "Company aggregate and transactions"). A company
// belongs to a campaign: ids that do not match are ErrNotFound.
type companyRepository interface {
	// Read returns the company's state without locking it.
	Read(ctx context.Context, campaignID, companyID int64) (CompanyState, error)
	// ReadRoster returns the state of every company of the campaign in a constant number of queries, enough to rate them (BuildDetail's
	// Points and Units): resources, order and units with equipment. Aftermaths, OpenMatch, Perks and Cemetery are not filled.
	ReadRoster(ctx context.Context, campaignID int64) ([]CompanyState, error)
	// Update runs fn on the state inside one transaction holding a row lock on the company, and stores what fn leaves behind
	// when it returns nil. When fn returns an error nothing is stored and the error is returned as is.
	Update(ctx context.Context, campaignID, companyID int64, fn func(*CompanyState) error) error
	// Create stores the company for the owner in the campaign, with its units. In one transaction holding the owner's member row it
	// checks the cap (c.Cap: ErrCompanyLimit when the owner owns that many companies there) and that no other company of the owner
	// there has the name, ignoring case (ErrInvalidCompany joined with "You already have a company with that name."); ErrNotFound when the owner is not a member.
	Create(ctx context.Context, campaignID int64, owner User, c NewCompany) (CampaignCompany, error)
}
