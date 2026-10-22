-- +goose Up
-- The Against the Odds selection of a match (docs/matches.md): one row per match, for the lower rated company that took the bonuses.
-- selection is {"v":1,...} (bonuses, kits, where each kit item went, the temporary Wanderer, the Ambush! scenario); previous_scenario is
-- the scenario the match had before Ambush! replaced it (had_previous says whether it had one). Read by the live match and the aftermath.
CREATE TABLE match_ato (
    match_id          BIGINT      PRIMARY KEY,
    company_id        BIGINT      NOT NULL,
    selection         JSONB       NOT NULL,
    previous_scenario JSONB,
    had_previous      BOOLEAN     NOT NULL DEFAULT false,
    revision          INTEGER     NOT NULL DEFAULT 1 CHECK (revision >= 1), -- saves of this row
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (match_id, company_id) REFERENCES match_side_companies (match_id, company_id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE match_ato;
