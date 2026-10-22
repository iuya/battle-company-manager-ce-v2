-- +goose Up
-- Rules files are identified by their hash alone (rulesets_hash_kind_key, rulesets pkey): two files with the same name and version
-- may coexist. The (kind, name, version) index only served the lookup of the vanilla "edition" that no longer exists.
DROP INDEX IF EXISTS rulesets_edition_idx;

-- +goose Down
CREATE INDEX rulesets_edition_idx ON rulesets (kind, name, version);
