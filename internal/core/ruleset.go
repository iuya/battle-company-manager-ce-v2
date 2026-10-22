package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// RulesetKind says which of the rules documents a file is.
type RulesetKind string

const (
	KindVanilla         RulesetKind = "vanilla"
	KindBattleCompanies RulesetKind = "battle_companies"
	KindGlossary        RulesetKind = "glossary"
)

func (k RulesetKind) Valid() bool {
	return k == KindVanilla || k == KindBattleCompanies || k == KindGlossary
}

func (k RulesetKind) Label() string {
	switch k {
	case KindVanilla:
		return "Vanilla rules data"
	case KindGlossary:
		return "Glossary"
	}
	return "Battle Companies rules data"
}

// ErrInvalidRuleset carries (joined) the reason an uploaded rules file was rejected.
var ErrInvalidRuleset = errors.New("invalid rules file")

// RulesetInfo identifies one stored rules document. Hash is the SHA-256 of the file with its insignificant whitespace
// removed (see RulesetHash). Files are stored once whoever uploads them: Every kind is identified by its hash alone: two files
// with the same Name and Version but different content coexist (the pickers and the admin list show name, version and ShortHash).
type RulesetInfo struct {
	Kind    RulesetKind
	Hash    string
	Name    string
	Version string
	Default bool // the file the app ships with
}

// ShortHash is the first characters of the hash, enough to tell files apart on screen.
func (r RulesetInfo) ShortHash() string {
	if len(r.Hash) > 8 {
		return r.Hash[:8]
	}
	return r.Hash
}

// RulesetHash is the identity of a rules file: the hex SHA-256 of the JSON with all whitespace outside strings removed
// (json.Compact), so reformatting a file does not change it. The browser computes the same value before uploading
// (static/js/ruleset_picker.js) and the server recomputes it from the upload; they must agree byte for byte.
func RulesetHash(content []byte) (string, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, content); err != nil {
		return "", errors.Join(ErrInvalidRuleset, errors.New("the file is not valid JSON"))
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

// RulesetMeta reads the name and version of a rules file and checks that it is the kind of document it claims to be.
// Battle Companies and glossary files carry top-level "name" and "version". The vanilla file is a third-party store
// that is never edited: its name is "game" and its version "appVersion".
func RulesetMeta(kind RulesetKind, content []byte) (name, version string, err error) {
	var doc struct {
		Game       string `json:"game"`
		AppVersion any    `json:"appVersion"`
		Name       string `json:"name"`
		Version    any    `json:"version"`
		Extends    any    `json:"extends"`
		Terms      any    `json:"terms"`
	}
	if err := json.Unmarshal(content, &doc); err != nil {
		return "", "", errors.Join(ErrInvalidRuleset, errors.New("the file is not a JSON object"))
	}
	switch kind {
	case KindVanilla:
		if doc.AppVersion == nil {
			return "", "", errors.Join(ErrInvalidRuleset, errors.New("this does not look like the vanilla rules file"))
		}
		name, version = doc.Game, fmt.Sprint(doc.AppVersion)
	case KindBattleCompanies, KindGlossary:
		marker := doc.Extends
		if kind == KindGlossary {
			marker = doc.Terms
		}
		if marker == nil {
			return "", "", errors.Join(ErrInvalidRuleset, errors.New("this does not look like a "+kind.Label()+" file"))
		}
		name = doc.Name
		if doc.Version != nil {
			version = fmt.Sprint(doc.Version)
		}
		if version == "" {
			return "", "", errors.Join(ErrInvalidRuleset, errors.New("the file has no version"))
		}
	default:
		return "", "", errors.Join(ErrInvalidRuleset, errors.New("unknown kind of rules file"))
	}
	if name == "" {
		return "", "", errors.Join(ErrInvalidRuleset, errors.New("the file has no name"))
	}
	return name, version, nil
}
