package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var messagePrinter = message.NewPrinter(language.English)

// MaxRulesetSize caps an uploaded rules file (the vanilla file is a few MB).
const MaxRulesetSize = 16 << 20

// Rulesets stores the rules files campaigns are played with. It implements web.rulesets over rulesetRepository: it
// recomputes hashes, checks the kind, validates against the JSON schema of the kind and applies the vanilla edition rule.
type Rulesets struct {
	repo    rulesetRepository
	schemas map[RulesetKind]*jsonschema.Schema
}

// NewRulesets builds the service. schemas holds the JSON schema document of each kind (definitions/*/*.schema.json); the
// caller reads them (from disk today) so core stays free of file access.
func NewRulesets(repo rulesetRepository, schemas map[RulesetKind][]byte) (*Rulesets, error) {
	rs := &Rulesets{repo: repo, schemas: map[RulesetKind]*jsonschema.Schema{}}
	for _, kind := range []RulesetKind{KindVanilla, KindBattleCompanies, KindGlossary} {
		raw, ok := schemas[kind]
		if !ok {
			return nil, fmt.Errorf("no JSON schema for the %s file", kind)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("the %s schema is not valid JSON: %w", kind, err)
		}
		url := "schema/" + string(kind) + ".json"
		if m, ok := doc.(map[string]any); ok {
			if id, ok := m["$id"].(string); ok && id != "" {
				url = id
			}
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(url, doc); err != nil {
			return nil, fmt.Errorf("the %s schema: %w", kind, err)
		}
		s, err := c.Compile(url)
		if err != nil {
			return nil, fmt.Errorf("the %s schema: %w", kind, err)
		}
		rs.schemas[kind] = s
	}
	return rs, nil
}

func (r *Rulesets) Default(ctx context.Context, kind RulesetKind) (RulesetInfo, error) {
	return r.repo.Default(ctx, kind)
}

// Lookup returns ErrNotFound when no file with that hash is stored.
func (r *Rulesets) Lookup(ctx context.Context, hash string) (RulesetInfo, error) {
	return r.repo.Info(ctx, hash)
}

// Find answers "do you already have this file?": the stored file of that kind with that hash. Files are identified by their hash alone
// (user decision 2026-10-21): name and version are labels, so several files may share them. name and version are kept in the
// signature for the callers and ignored. ErrNotFound when there is none.
func (r *Rulesets) Find(ctx context.Context, kind RulesetKind, hash, name, version string) (RulesetInfo, error) {
	info, err := r.repo.Info(ctx, hash)
	switch {
	case err == nil && info.Kind == kind:
		return info, nil
	case err != nil && !errors.Is(err, ErrNotFound):
		return RulesetInfo{}, err
	}
	return RulesetInfo{}, ErrNotFound
}

// Store verifies an upload (size, hash, kind, schema) and keeps it unless it is already stored. A file stored once is
// returned as is, whoever uploads it again.
func (r *Rulesets) Store(ctx context.Context, kind RulesetKind, hash string, content []byte) (RulesetInfo, error) {
	return r.store(ctx, kind, hash, content, 0)
}

// StoreAs is Store recording who uploaded the file (the Rules files page shows it).
func (r *Rulesets) StoreAs(ctx context.Context, uploader User, kind RulesetKind, hash string, content []byte) (RulesetInfo, error) {
	return r.store(ctx, kind, hash, content, uploader.ID)
}

func (r *Rulesets) store(ctx context.Context, kind RulesetKind, hash string, content []byte, uploader int64) (RulesetInfo, error) {
	if !kind.Valid() {
		return RulesetInfo{}, errors.Join(ErrInvalidRuleset, errors.New("unknown kind of rules file"))
	}
	if len(content) > MaxRulesetSize {
		return RulesetInfo{}, errors.Join(ErrInvalidRuleset, fmt.Errorf("the file is larger than %d MB", MaxRulesetSize>>20))
	}
	got, err := RulesetHash(content)
	if err != nil {
		return RulesetInfo{}, err
	}
	if got != hash {
		return RulesetInfo{}, errors.Join(ErrInvalidRuleset, errors.New("the file does not match the hash it was announced with"))
	}
	name, version, err := RulesetMeta(kind, content)
	if err != nil {
		return RulesetInfo{}, err
	}
	known, err := r.repo.Info(ctx, hash)
	switch {
	case err == nil:
		if known.Kind != kind {
			return RulesetInfo{}, errors.Join(ErrInvalidRuleset, errors.New("this file is already stored as a "+known.Kind.Label()+" file"))
		}
		return known, nil
	case !errors.Is(err, ErrNotFound):
		return RulesetInfo{}, err
	}
	if err := r.validate(kind, content); err != nil {
		return RulesetInfo{}, err
	}
	compact, err := compactJSON(content)
	if err != nil {
		return RulesetInfo{}, err
	}
	info := RulesetInfo{Kind: kind, Hash: hash, Name: name, Version: version}
	if err := r.repo.Insert(ctx, StoredRuleset{RulesetInfo: info, Size: len(compact), Content: compact, UploadedBy: uploader}); err != nil {
		return RulesetInfo{}, err
	}
	return info, nil
}

func compactJSON(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, content); err != nil {
		return nil, errors.Join(ErrInvalidRuleset, errors.New("the file is not valid JSON"))
	}
	return buf.Bytes(), nil
}

// validate checks the file against the schema of its kind and joins the reasons (at most a handful) into the error.
func (r *Rulesets) validate(kind RulesetKind, content []byte) error {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		return errors.Join(ErrInvalidRuleset, errors.New("the file is not valid JSON"))
	}
	err = r.schemas[kind].Validate(doc)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return errors.Join(ErrInvalidRuleset, err)
	}
	var reasons []string
	collectCauses(ve, &reasons)
	if len(reasons) > 5 {
		reasons = append(reasons[:5], fmt.Sprintf("and %d more", len(reasons)-5))
	}
	return errors.Join(ErrInvalidRuleset, errors.New("the file does not follow the "+strings.ToLower(kind.Label())+" format: "+strings.Join(reasons, "; ")))
}

func collectCauses(ve *jsonschema.ValidationError, out *[]string) {
	if len(ve.Causes) == 0 {
		loc := "/" + strings.Join(ve.InstanceLocation, "/")
		*out = append(*out, fmt.Sprintf("%s: %s", loc, ve.ErrorKind.LocalizedString(messagePrinter)))
		return
	}
	for _, c := range ve.Causes {
		collectCauses(c, out)
	}
}

// RegisterDefaults stores the files the app ships with and makes them the defaults of their kinds. Called at every start:
// files already stored and already default are left alone.
func (r *Rulesets) RegisterDefaults(ctx context.Context, vanilla, bc, glossary []byte) error {
	for _, d := range []struct {
		kind    RulesetKind
		content []byte
	}{{KindVanilla, vanilla}, {KindBattleCompanies, bc}, {KindGlossary, glossary}} {
		hash, err := RulesetHash(d.content)
		if err != nil {
			return fmt.Errorf("the shipped %s file: %w", d.kind, err)
		}
		info, err := r.store(ctx, d.kind, hash, d.content, 0)
		if err != nil {
			return fmt.Errorf("the shipped %s file: %w", d.kind, err)
		}
		if cur, err := r.repo.Default(ctx, d.kind); err == nil && cur.Hash == info.Hash {
			continue
		} else if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := r.repo.SetDefault(ctx, d.kind, info.Hash); err != nil {
			return err
		}
	}
	return nil
}

// CampaignOptions are the settings the default Battle Companies file offers an organiser.
func (r *Rulesets) CampaignOptions(ctx context.Context) ([]CampaignOption, error) {
	info, err := r.repo.Default(ctx, KindBattleCompanies)
	if err != nil {
		return nil, err
	}
	stored, err := r.repo.Get(ctx, info.Hash)
	if err != nil {
		return nil, err
	}
	return ParseCampaignOptions(stored.Content)
}

// CampaignRef names a campaign.
type CampaignRef struct {
	ID   int64
	Name string
}

// RulesetUsage is a stored file with who made it and what uses it.
type RulesetUsage struct {
	RulesetInfo
	Size       int
	UploadedBy *User // nil: shipped with the app
	CreatedAt  time.Time
	Campaigns  []CampaignRef // campaigns whose vanilla, Battle Companies or glossary file this is
}

func requireSuperuser(actor User) error {
	if !actor.IsSuperuser() {
		return Refuse("Only a superuser can manage the rules files")
	}
	return nil
}

// ListWithUsage returns every stored file (content not loaded) with its usage, ordered by kind and newest first.
func (r *Rulesets) ListWithUsage(ctx context.Context, actor User) ([]RulesetUsage, error) {
	if err := requireSuperuser(actor); err != nil {
		return nil, err
	}
	return r.repo.ListUsage(ctx)
}

// SetDefault makes the stored file the default of its kind: ErrNotFound for an unknown hash, ErrRefused unless actor is a
// superuser. Existing campaigns keep their files.
func (r *Rulesets) SetDefault(ctx context.Context, actor User, hash string) error {
	if err := requireSuperuser(actor); err != nil {
		return err
	}
	info, err := r.repo.Info(ctx, hash)
	if err != nil {
		return err
	}
	return r.repo.SetDefault(ctx, info.Kind, hash)
}

// Delete removes a stored file no campaign uses and that is not a default; ErrRefused with the reason otherwise,
// ErrNotFound for an unknown hash.
func (r *Rulesets) Delete(ctx context.Context, actor User, hash string) error {
	if err := requireSuperuser(actor); err != nil {
		return err
	}
	return r.repo.Delete(ctx, hash)
}

// PruneUnused deletes every file no campaign uses and that is not a default, and returns how many.
func (r *Rulesets) PruneUnused(ctx context.Context, actor User) (int, error) {
	if err := requireSuperuser(actor); err != nil {
		return 0, err
	}
	return r.repo.DeleteUnused(ctx)
}

// Download returns a stored file's info and content (compact JSON) for the administration Rules files page; ErrNotFound
// for an unknown hash, ErrRefused unless actor is a superuser.
func (r *Rulesets) Download(ctx context.Context, actor User, hash string) (RulesetInfo, []byte, error) {
	if err := requireSuperuser(actor); err != nil {
		return RulesetInfo{}, nil, err
	}
	stored, err := r.repo.Get(ctx, hash)
	if err != nil {
		return RulesetInfo{}, nil, err
	}
	return stored.RulesetInfo, stored.Content, nil
}
