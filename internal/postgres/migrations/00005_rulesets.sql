-- +goose Up
-- Rules files, content addressed: hash is the SHA-256 (hex) of the compacted JSON kept in content, so bytea keeps the
-- hashed bytes exact. uploaded_by is NULL for the files the app ships with.
CREATE TABLE rulesets (
    hash        TEXT        PRIMARY KEY CHECK (hash ~ '^[0-9a-f]{64}$'),
    kind        TEXT        NOT NULL CHECK (kind IN ('vanilla', 'battle_companies', 'glossary')),
    name        TEXT        NOT NULL,
    version     TEXT        NOT NULL,
    size        INTEGER     NOT NULL CHECK (size = octet_length(content)),
    content     BYTEA       NOT NULL,
    uploaded_by BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    is_default  BOOLEAN     NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX rulesets_default_key ON rulesets (kind) WHERE is_default;
CREATE INDEX rulesets_edition_idx ON rulesets (kind, name, version);

-- +goose Down
DROP TABLE rulesets;
