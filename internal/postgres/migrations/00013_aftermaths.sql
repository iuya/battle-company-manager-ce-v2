-- +goose Up
-- The War's Aftermath (docs/aftermath.md): one row per company per match once something is saved (no row means pending), and the
-- perks scenarios leave a company. input, state and lines are opaque to SQL and versioned with the code (jsonb with a top-level "v").
CREATE TABLE match_aftermaths (
    match_id    BIGINT      NOT NULL,
    company_id  BIGINT      NOT NULL,
    status      TEXT        NOT NULL CHECK (status IN ('pending', 'sheet', 'injuries', 'levelups', 'done', 'waived')),
    revision    INTEGER     NOT NULL DEFAULT 0 CHECK (revision >= 0),
    input       JSONB       NOT NULL DEFAULT '{"v": 1}' CHECK (jsonb_typeof(input) = 'object'),
    state       JSONB       NOT NULL DEFAULT '{"v": 1}' CHECK (jsonb_typeof(state) = 'object'),
    lines       JSONB       NOT NULL DEFAULT '{"v": 1}' CHECK (jsonb_typeof(lines) = 'object'),
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    PRIMARY KEY (match_id, company_id),
    FOREIGN KEY (match_id, company_id) REFERENCES match_side_companies (match_id, company_id) ON DELETE CASCADE
);
CREATE INDEX match_aftermaths_company_idx ON match_aftermaths (company_id);

CREATE TABLE company_perks (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id BIGINT      NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    match_id   BIGINT      REFERENCES matches (id) ON DELETE SET NULL,
    unit_id    BIGINT,     -- the relic bearer; NULL for the company (no foreign key: the unit may fall)
    effect     TEXT        NOT NULL CHECK (effect <> ''),
    name       TEXT        NOT NULL DEFAULT '',
    text       TEXT        NOT NULL DEFAULT '',
    applies    TEXT        NOT NULL DEFAULT '',
    used_on    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX company_perks_company_idx ON company_perks (company_id, (used_on IS NOT NULL), id);

-- Matches that ended before the aftermath existed have nothing left to apply.
INSERT INTO match_aftermaths (match_id, company_id, status)
SELECT sc.match_id, sc.company_id, 'done' FROM match_side_companies sc JOIN matches m ON m.id = sc.match_id WHERE m.ended;

-- +goose Down
DROP TABLE company_perks;
DROP TABLE match_aftermaths;
