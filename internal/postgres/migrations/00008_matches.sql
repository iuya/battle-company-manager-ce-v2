-- +goose Up
-- Matches (docs/matches.md): the match, its sides and companies, the scenario (a rulebook name or a one-off, stored with the
-- match only), the reported result, the history of ending and reopening, and the grants of gold, influence and experience.
ALTER TABLE companies ADD CONSTRAINT companies_id_campaign_key UNIQUE (id, campaign_id);

CREATE TABLE matches (
    id                   BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    campaign_id          BIGINT      NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    number               INTEGER     NOT NULL CHECK (number >= 1), -- 1, 2, 3... within the campaign
    played_on            TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by           BIGINT      NOT NULL REFERENCES users (id),
    -- scenario: NULL kind means none chosen yet; description and victory are for a one-off only
    scenario_kind        TEXT        CHECK (scenario_kind IN ('rulebook', 'custom')),
    scenario_name        TEXT        NOT NULL DEFAULT '',
    scenario_description TEXT        NOT NULL DEFAULT '',
    scenario_victory     TEXT        NOT NULL DEFAULT '',
    -- result: decided says the players reported an outcome; winner is the 1-based side, 0 a draw
    decided              BOOLEAN     NOT NULL DEFAULT false,
    winner               INTEGER     NOT NULL DEFAULT 0 CHECK (winner BETWEEN 0 AND 8),
    note                 TEXT        NOT NULL DEFAULT '' CHECK (char_length(note) <= 500),
    ended                BOOLEAN     NOT NULL DEFAULT false,
    ended_on             TIMESTAMPTZ,
    revision             INTEGER     NOT NULL DEFAULT 1 CHECK (revision >= 1), -- bumped by every write
    UNIQUE (campaign_id, number),
    UNIQUE (id, campaign_id),
    CONSTRAINT matches_scenario_chk CHECK (
        (scenario_kind IS NULL AND scenario_name = '' AND scenario_description = '' AND scenario_victory = '') OR
        (scenario_kind = 'rulebook' AND char_length(scenario_name) BETWEEN 1 AND 60 AND scenario_description = '' AND scenario_victory = '') OR
        (scenario_kind = 'custom' AND char_length(scenario_name) BETWEEN 1 AND 60
            AND char_length(scenario_description) <= 1000 AND char_length(scenario_victory) <= 1000)),
    CONSTRAINT matches_decided_chk CHECK (decided OR winner = 0),
    CONSTRAINT matches_ended_chk CHECK (ended = (ended_on IS NOT NULL))
);
CREATE INDEX matches_campaign_idx ON matches (campaign_id, number DESC);

CREATE TABLE match_sides (
    match_id       BIGINT   NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    position       INTEGER  NOT NULL CHECK (position >= 0), -- 0-based, side number - 1
    name           TEXT     NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40),
    victory_points SMALLINT CHECK (victory_points BETWEEN 0 AND 99), -- NULL: not reported
    PRIMARY KEY (match_id, position)
);

-- A company plays a match once; the composite key to companies makes it a company of the match's campaign.
CREATE TABLE match_side_companies (
    match_id    BIGINT  NOT NULL,
    campaign_id BIGINT  NOT NULL,
    position    INTEGER NOT NULL,
    slot        INTEGER NOT NULL CHECK (slot >= 0), -- order within the side
    company_id  BIGINT  NOT NULL,
    fled        BOOLEAN NOT NULL DEFAULT false, -- All is Lost
    PRIMARY KEY (match_id, company_id),
    UNIQUE (match_id, position, slot),
    FOREIGN KEY (match_id, position)    REFERENCES match_sides (match_id, position) ON DELETE CASCADE,
    FOREIGN KEY (match_id, campaign_id) REFERENCES matches (id, campaign_id) ON DELETE CASCADE,
    FOREIGN KEY (company_id, campaign_id) REFERENCES companies (id, campaign_id)
);
CREATE INDEX match_side_companies_company_idx ON match_side_companies (company_id);

CREATE TABLE match_objectives (
    match_id   BIGINT  NOT NULL,
    company_id BIGINT  NOT NULL,
    key        TEXT    NOT NULL CHECK (key <> ''),
    times      INTEGER NOT NULL CHECK (times > 0),
    PRIMARY KEY (match_id, company_id, key),
    FOREIGN KEY (match_id, company_id) REFERENCES match_side_companies (match_id, company_id) ON DELETE CASCADE
);

CREATE TABLE match_events (
    id       BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    match_id BIGINT      NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    kind     TEXT        NOT NULL CHECK (kind IN ('ended', 'reopened')),
    user_id  BIGINT      NOT NULL REFERENCES users (id),
    at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX match_events_match_idx ON match_events (match_id, id);

-- The history of awards to a company. unit_id has no foreign key (the unit may fall later and leave the roster); grants die
-- with their company, and keep existing (match_id cleared) if their match is deleted.
CREATE TABLE grants (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id BIGINT      NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    kind       TEXT        NOT NULL CHECK (kind IN ('organiser', 'scenario')),
    gold       INTEGER     NOT NULL DEFAULT 0,
    influence  INTEGER     NOT NULL DEFAULT 0,
    experience INTEGER     NOT NULL DEFAULT 0,
    unit_id    BIGINT,
    match_id   BIGINT      REFERENCES matches (id) ON DELETE SET NULL,
    note       TEXT        NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    granted_by BIGINT      REFERENCES users (id),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT grants_note_chk CHECK (kind <> 'organiser' OR char_length(note) >= 1),
    CONSTRAINT grants_experience_chk CHECK (experience = 0 OR unit_id IS NOT NULL)
);
CREATE INDEX grants_company_idx ON grants (company_id, id DESC);

-- +goose Down
DROP TABLE grants;
DROP TABLE match_events;
DROP TABLE match_objectives;
DROP TABLE match_side_companies;
DROP TABLE match_sides;
DROP TABLE matches;
ALTER TABLE companies DROP CONSTRAINT companies_id_campaign_key;
