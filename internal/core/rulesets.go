package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/iuya/battle-company-manager-ce-v2/internal/core/lean"
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
	for _, kind := range []RulesetKind{KindVanilla, KindGlossary} { // the Battle Companies file is checked by the lean decoder, not by a schema
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

// Find answers "do you already have this file?": the stored file of that kind with that hash. Files are identified by their hash alone:
// name and version are labels, so several files may share them. name and version are kept in the
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
	return r.storeAgainst(ctx, kind, hash, content, uploader, nil)
}

// storeAgainst is store checking a Battle Companies file against the vanilla file and glossary given (the shipped ones, at the start)
// instead of the defaults of the repository.
func (r *Rulesets) storeAgainst(ctx context.Context, kind RulesetKind, hash string, content []byte, uploader int64, against *lowers) (RulesetInfo, error) {
	p, err := r.prepare(ctx, kind, hash, content, against)
	if err != nil {
		return RulesetInfo{}, err
	}
	return r.insert(ctx, p, uploader)
}

// prepared is an upload that passed every check and is ready to insert (known: the file was already stored, nothing to insert).
type prepared struct {
	info    RulesetInfo
	known   bool
	compact []byte
}

// prepare runs every check of an upload (size, hash, kind, shape, schema or lean decode) without writing anything.
func (r *Rulesets) prepare(ctx context.Context, kind RulesetKind, hash string, content []byte, against *lowers) (prepared, error) {
	got, err := r.preflightHash(kind, content)
	if err != nil {
		return prepared{}, err
	}
	if got != hash {
		return prepared{}, errors.Join(ErrInvalidRuleset, errors.New("the file does not match the hash it was announced with"))
	}
	name, version, err := preflightShape(kind, content)
	if err != nil {
		return prepared{}, err
	}
	known, err := r.repo.Info(ctx, hash)
	switch {
	case err == nil:
		if known.Kind != kind {
			return prepared{}, errors.Join(ErrInvalidRuleset, errors.New("this file is already stored as a "+known.Kind.Label()+" file"))
		}
		return prepared{info: known, known: true}, nil
	case !errors.Is(err, ErrNotFound):
		return prepared{}, err
	}
	if rep := r.validateFile(ctx, kind, content, against); rep.Errors() > 0 {
		return prepared{}, rep
	}
	compact, err := compactJSON(content)
	if err != nil {
		return prepared{}, err
	}
	return prepared{info: RulesetInfo{Kind: kind, Hash: hash, Name: name, Version: version}, compact: compact}, nil
}

func (r *Rulesets) insert(ctx context.Context, p prepared, uploader int64) (RulesetInfo, error) {
	if p.known {
		return p.info, nil
	}
	if err := r.repo.Insert(ctx, StoredRuleset{RulesetInfo: p.info, Size: len(p.compact), Content: p.compact, UploadedBy: uploader}); err != nil {
		return RulesetInfo{}, err
	}
	return p.info, nil
}

// RulesetUpload is one file of an upload: its kind, the hash the browser announced, and the bytes.
type RulesetUpload struct {
	Kind    RulesetKind
	Hash    string
	Content []byte
}

// StoreAllAs stores several uploaded files all or nothing: every file is checked first, the problems of all of them are
// returned together (one report, each issue naming its file), and nothing is inserted unless every file is good. The infos come back in the order of the uploads.
func (r *Rulesets) StoreAllAs(ctx context.Context, uploader User, uploads []RulesetUpload) ([]RulesetInfo, error) {
	// a Battle Companies file is checked against the vanilla file and glossary uploaded with it
	var vanilla, glossary []byte
	for _, u := range uploads {
		switch u.Kind {
		case KindVanilla:
			vanilla = u.Content
		case KindGlossary:
			glossary = u.Content
		}
	}
	preps := make([]prepared, len(uploads))
	var errs []error
	for i, u := range uploads {
		var against *lowers
		if u.Kind == KindBattleCompanies {
			var err error
			if against, _, err = r.lowersOf(ctx, vanilla, glossary); err != nil {
				return nil, err
			}
		}
		p, err := r.prepare(ctx, u.Kind, u.Hash, u.Content, against)
		if err != nil {
			if !errors.Is(err, ErrInvalidRuleset) {
				return nil, err // the repository failed: not a problem of the file
			}
			_, rerr := failure(u.Kind, err)
			errs = append(errs, rerr)
			continue
		}
		preps[i] = p
	}
	if len(errs) > 0 {
		return nil, JoinRulesetReports(errs...)
	}
	infos := make([]RulesetInfo, len(uploads))
	for i, p := range preps {
		info, err := r.insert(ctx, p, uploader.ID)
		if err != nil {
			return nil, err
		}
		infos[i] = info
	}
	return infos, nil
}

// preflightHash checks the kind and the size, and returns the file's hash (an error when it is not JSON).
func (r *Rulesets) preflightHash(kind RulesetKind, content []byte) (string, error) {
	if !kind.Valid() {
		return "", errors.Join(ErrInvalidRuleset, errors.New("unknown kind of rules file"))
	}
	if len(content) > MaxRulesetSize {
		return "", errors.Join(ErrInvalidRuleset, fmt.Errorf("the file is larger than %d MB", MaxRulesetSize>>20))
	}
	return RulesetHash(content)
}

// preflightShape checks the shape limits and that the file is the kind of document it claims to be; it returns name and version.
func preflightShape(kind RulesetKind, content []byte) (name, version string, err error) {
	if err := checkRulesetShape(content); err != nil {
		return "", "", err
	}
	return RulesetMeta(kind, content)
}

// Check validates one rules file without storing it: CheckSet with only that file, so a Battle Companies file is checked against
// the default vanilla file and glossary.
func (r *Rulesets) Check(ctx context.Context, kind RulesetKind, content []byte) (RulesetReport, error) {
	set := RulesetSet{}
	if content == nil {
		content = []byte{}
	}
	switch kind {
	case KindVanilla:
		set.Vanilla = content
	case KindGlossary:
		set.Glossary = content
	case KindBattleCompanies:
		set.BattleCompanies = content
	default:
		return failure(kind, errors.Join(ErrInvalidRuleset, errors.New("unknown kind of rules file")))
	}
	return r.CheckSet(ctx, set)
}

// RulesetSet is the files of one check or one campaign form. A nil file is not given: it is not checked, and a Battle
// Companies file is then checked against the default (shipped) vanilla file or glossary.
type RulesetSet struct {
	Vanilla, Glossary, BattleCompanies []byte
}

// CheckSet validates the files of a set without storing or creating anything (the dry run of the admin "Check this file" button and
// of the online validator). Vanilla and glossary are checked against their schemas; the Battle Companies file is checked against
// the vanilla file and glossary of the SAME set, the default ones for what is not given. One report carries every problem and
// warning, issues grouped by file in the order vanilla, glossary, Battle Companies; the error is non-nil, wraps ErrInvalidRuleset
// and is that report when a file has errors. A valid schemaVersion 2 file is not an error here: CheckSet says whether the files
// are right, StoreAs whether this build can play them.
func (r *Rulesets) CheckSet(ctx context.Context, set RulesetSet) (RulesetReport, error) {
	all := &RulesetReport{}
	reports := 0
	add := func(rep *RulesetReport) {
		if reports++; reports == 1 { // one file's own one-line text (schema failures keep their wording) stands for the report
			all.summary = rep.summary
		} else {
			all.summary = ""
		}
		all.issues = append(all.issues, rep.issues...)
		all.truncated += rep.truncated
	}
	files := []struct {
		kind    RulesetKind
		content []byte
	}{{KindVanilla, set.Vanilla}, {KindGlossary, set.Glossary}, {KindBattleCompanies, set.BattleCompanies}}
	var against *lowers
	var warns []RulesetIssue
	for _, f := range files {
		if f.content == nil {
			continue
		}
		if _, err := r.preflightHash(f.kind, f.content); err != nil {
			rep, _ := failure(f.kind, err)
			add(&rep)
			continue
		}
		if _, _, err := preflightShape(f.kind, f.content); err != nil {
			rep, _ := failure(f.kind, err)
			add(&rep)
			continue
		}
		if f.kind == KindBattleCompanies {
			var err error
			if against, warns, err = r.lowersOf(ctx, set.Vanilla, set.Glossary); err != nil {
				return RulesetReport{}, err
			}
		}
		rep := r.validateFile(ctx, f.kind, f.content, against)
		if f.kind == KindBattleCompanies && len(warns) > 0 {
			rep.issues = append(append([]RulesetIssue(nil), warns...), rep.issues...)
		}
		add(rep)
	}
	if all.Errors() > 0 {
		return *all, all
	}
	return *all, nil
}

// failure turns a plain rejection (too large, not JSON, wrong kind of document) into a one-issue report.
func failure(kind RulesetKind, err error) (RulesetReport, error) {
	var rep *RulesetReport
	if errors.As(err, &rep) {
		return *rep, rep
	}
	msg := reasonOf(err)
	r := &RulesetReport{issues: []RulesetIssue{{File: kind.fileName(), Message: msg}}, summary: msg}
	return *r, r
}

// reasonOf is the human part of an error built with errors.Join(ErrInvalidRuleset, reason).
func reasonOf(err error) string {
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		if errs := u.Unwrap(); len(errs) > 1 {
			return errs[len(errs)-1].Error()
		}
	}
	return err.Error()
}

func compactJSON(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, content); err != nil {
		return nil, errors.Join(ErrInvalidRuleset, errors.New("the file is not valid JSON"))
	}
	return buf.Bytes(), nil
}

// validate checks the file against what its kind demands and returns every problem. A Battle Companies file goes
// through the lean decoder, against the default vanilla file and glossary; every other file against the JSON schema of its kind
// (the failures converted to issues, none dropped). The report never is nil.
func (r *Rulesets) validateFile(ctx context.Context, kind RulesetKind, content []byte, against *lowers) *RulesetReport {
	if kind == KindBattleCompanies {
		if rep, old := oldFormatReport(kind.fileName(), content); old {
			return rep
		}
		return r.validateV2(ctx, content, against)
	}
	return r.validateSchema(kind, content)
}

func (r *Rulesets) validateV2(ctx context.Context, content []byte, against *lowers) *RulesetReport {
	vanilla, glossary, err := r.lowersFor(ctx, against)
	if err != nil {
		return &RulesetReport{issues: []RulesetIssue{{File: KindBattleCompanies.fileName(), Message: "the default vanilla and glossary files needed to check this file are not available: " + err.Error()}}}
	}
	_, rep := DecodeBattleCompanies(KindBattleCompanies.fileName(), content, vanilla, glossary)
	return newRulesetReport(rep)
}

// lowers are the vanilla book and the glossary a Battle Companies file is checked against.
type lowers struct {
	book *VanillaBook
	gl   Glossary
}

func (r *Rulesets) lowersFor(ctx context.Context, against *lowers) (*VanillaBook, Glossary, error) {
	if against != nil {
		return against.book, against.gl, nil
	}
	return r.defaultLowers(ctx)
}

// defaultLowers builds the vanilla book and the glossary of the default files.
func (r *Rulesets) defaultLowers(ctx context.Context) (*VanillaBook, Glossary, error) {
	book, err := r.defaultBook(ctx)
	if err != nil {
		return nil, Glossary{}, err
	}
	gl, err := r.defaultGlossary(ctx)
	if err != nil {
		return nil, Glossary{}, err
	}
	return book, gl, nil
}

func (r *Rulesets) loadDefault(ctx context.Context, kind RulesetKind) (StoredRuleset, error) {
	info, err := r.repo.Default(ctx, kind)
	if err != nil {
		return StoredRuleset{}, fmt.Errorf("no default %s file: %w", kind, err)
	}
	return r.repo.Get(ctx, info.Hash)
}

func (r *Rulesets) defaultBook(ctx context.Context) (*VanillaBook, error) {
	v, err := r.loadDefault(ctx, KindVanilla)
	if err != nil {
		return nil, err
	}
	return ParseVanilla(v.Content)
}

func (r *Rulesets) defaultGlossary(ctx context.Context) (Glossary, error) {
	g, err := r.loadDefault(ctx, KindGlossary)
	if err != nil {
		return Glossary{}, err
	}
	return BuildGlossary(g.RulesetInfo, g.Content)
}

// lowersOf resolves what a Battle Companies file is checked against from the vanilla and glossary files that come with it
// (nil: the default one). A supplied file that cannot be read is replaced by the default and said so in the warnings, so the
// report names the file that is wrong instead of blaming the Battle Companies file for it.
func (r *Rulesets) lowersOf(ctx context.Context, vanilla, glossary []byte) (*lowers, []RulesetIssue, error) {
	if vanilla == nil && glossary == nil {
		return nil, nil, nil
	}
	var warns []RulesetIssue
	against := &lowers{}
	var err error
	if vanilla != nil {
		if against.book, err = ParseVanilla(vanilla); err != nil {
			against.book = nil
			warns = append(warns, RulesetIssue{File: KindBattleCompanies.fileName(), Warn: true, Message: "your vanilla file could not be read, so this file was checked against the shipped vanilla file"})
		}
	}
	if against.book == nil {
		if against.book, err = r.defaultBook(ctx); err != nil {
			return nil, nil, err
		}
	}
	if glossary != nil {
		if against.gl, err = BuildGlossary(RulesetInfo{Kind: KindGlossary}, glossary); err != nil {
			against.gl = Glossary{}
			glossary = nil
			warns = append(warns, RulesetIssue{File: KindBattleCompanies.fileName(), Warn: true, Message: "your glossary file could not be read, so this file was checked against the shipped glossary"})
		}
	}
	if glossary == nil {
		if against.gl, err = r.defaultGlossary(ctx); err != nil {
			return nil, nil, err
		}
	}
	return against, warns, nil
}

// validate checks the file against the JSON schema of its kind and returns the report as the error (nil when it passes).
func (r *Rulesets) validate(kind RulesetKind, content []byte) error {
	if rep := r.validateSchema(kind, content); rep.Errors() > 0 {
		return rep
	}
	return nil
}

// validateSchema checks the file against the JSON schema of its kind.
func (r *Rulesets) validateSchema(kind RulesetKind, content []byte) *RulesetReport {
	file := kind.fileName()
	notJSON := &RulesetReport{issues: []RulesetIssue{{File: file, Message: "the file is not valid JSON"}}, summary: "the file is not valid JSON"}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		return notJSON
	}
	err = r.schemas[kind].Validate(doc)
	if err == nil {
		return &RulesetReport{}
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return &RulesetReport{issues: []RulesetIssue{{File: file, Message: err.Error()}}, summary: err.Error()}
	}
	rep := &RulesetReport{}
	var reasons []string
	collectCauses(ve, file, &rep.issues, &reasons)
	if len(rep.issues) > lean.MaxIssues {
		rep.truncated = len(rep.issues) - lean.MaxIssues
		rep.issues = rep.issues[:lean.MaxIssues]
	}
	if len(reasons) > schemaSummaryReasons {
		reasons = append(reasons[:schemaSummaryReasons], fmt.Sprintf("and %d more", len(reasons)-schemaSummaryReasons))
	}
	rep.summary = "the file does not follow the " + strings.ToLower(kind.Label()) + " format: " + strings.Join(reasons, "; ")
	return rep
}

// schemaSummaryReasons is how many schema failures the one-line error text names; the report lists them all.
const schemaSummaryReasons = 5

// collectCauses lists the leaf failures of a schema validation as issues (json path, message) and as the "/pointer: message"
// strings of the one-line summary.
func collectCauses(ve *jsonschema.ValidationError, file string, issues *[]RulesetIssue, reasons *[]string) {
	if len(ve.Causes) == 0 {
		msg := ve.ErrorKind.LocalizedString(messagePrinter)
		*reasons = append(*reasons, fmt.Sprintf("%s: %s", "/"+strings.Join(ve.InstanceLocation, "/"), msg))
		*issues = append(*issues, RulesetIssue{File: file, Path: schemaPath(ve.InstanceLocation), Message: msg})
		return
	}
	for _, c := range ve.Causes {
		collectCauses(c, file, issues, reasons)
	}
}

// schemaPath writes a JSON pointer's parts in the path form of the other reports: a.b[3].c
func schemaPath(parts []string) string {
	p := ""
	for _, part := range parts {
		if n, err := strconv.Atoi(part); err == nil && n >= 0 {
			p = lean.Idx(p, n)
		} else {
			p = lean.Join(p, part)
		}
	}
	return p
}

// RegisterDefaults stores the files the app ships with and makes them the defaults of their kinds. Called at every start:
// files already stored and already default are left alone.
func (r *Rulesets) RegisterDefaults(ctx context.Context, vanilla, bc, glossary []byte) error {
	// the Battle Companies file is checked against the shipped vanilla file and glossary, which are registered first
	var against *lowers
	if book, err := ParseVanilla(vanilla); err == nil {
		if gl, err := BuildGlossary(RulesetInfo{Kind: KindGlossary}, glossary); err == nil {
			against = &lowers{book, gl}
		}
	}
	for _, d := range []struct {
		kind    RulesetKind
		content []byte
	}{{KindVanilla, vanilla}, {KindGlossary, glossary}, {KindBattleCompanies, bc}} {
		hash, err := RulesetHash(d.content)
		if err != nil {
			return fmt.Errorf("the shipped %s file: %w", d.kind, err)
		}
		info, err := r.storeAgainst(ctx, d.kind, hash, d.content, 0, against)
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
