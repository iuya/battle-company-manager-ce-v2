-- +goose Up
-- The company aggregate. Everything queried or constrained is a column; what is opaque to SQL and versioned with the rules
-- code is jsonb with a top-level "v". Ids are never reused: a unit keeps its id when it falls.
CREATE TABLE companies (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    campaign_id BIGINT      NOT NULL,
    owner_id    BIGINT      NOT NULL,
    name        TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    faction     TEXT        NOT NULL CHECK (faction <> ''),
    influence   INTEGER     NOT NULL DEFAULT 0,
    gold        INTEGER     NOT NULL DEFAULT 0,
    rerolls     INTEGER     NOT NULL DEFAULT 0 CHECK (rerolls >= 0),
    pack        TEXT[]      NOT NULL DEFAULT '{}',
    unit_order  BIGINT[]    NOT NULL DEFAULT '{}', -- roster order the player saved (unit ids; stale ids are ignored)
    grave_order BIGINT[]    NOT NULL DEFAULT '{}', -- cemetery order the player saved
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT companies_campaign_owner_key UNIQUE (campaign_id, owner_id),
    CONSTRAINT companies_member_fk FOREIGN KEY (campaign_id, owner_id) REFERENCES campaign_members (campaign_id, user_id)
);

CREATE TABLE company_units (
    id              BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id      BIGINT  NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    position        INTEGER NOT NULL CHECK (position >= 0),
    base            TEXT    NOT NULL,
    custom_name     TEXT    NOT NULL DEFAULT '',
    role            TEXT    NOT NULL CHECK (role IN ('leader', 'sergeant', 'hero', 'warrior')),
    kind            TEXT    NOT NULL DEFAULT '',
    path            TEXT    NOT NULL DEFAULT '',
    path_rules      INTEGER NOT NULL DEFAULT 0 CHECK (path_rules >= 0),
    level           INTEGER NOT NULL DEFAULT 0 CHECK (level >= 0),
    xp              INTEGER NOT NULL DEFAULT 0 CHECK (xp >= 0),
    points          INTEGER NOT NULL DEFAULT 0 CHECK (points >= 0),
    wanderer        TEXT    NOT NULL DEFAULT '',
    creature        TEXT    NOT NULL DEFAULT '',
    injured         BOOLEAN NOT NULL DEFAULT false,
    sent_to_healers BOOLEAN NOT NULL DEFAULT false,
    progress        JSONB   CHECK (jsonb_typeof(progress) = 'object'),
    injuries        JSONB   NOT NULL DEFAULT '{"v": 1}' CHECK (jsonb_typeof(injuries) = 'object'),
    data            JSONB   NOT NULL DEFAULT '{"v": 1}' CHECK (jsonb_typeof(data) = 'object'), -- bonus, actions, spells
    UNIQUE (company_id, position) DEFERRABLE INITIALLY DEFERRED
);

-- Only deviations from the vanilla wargear are stored: what the unit came with, what it bought, what it dropped.
CREATE TABLE unit_equipment (
    unit_id  BIGINT  NOT NULL REFERENCES company_units (id) ON DELETE CASCADE,
    origin   TEXT    NOT NULL CHECK (origin IN ('gear', 'bought', 'dropped')),
    position INTEGER NOT NULL CHECK (position >= 0),
    name     TEXT    NOT NULL,
    PRIMARY KEY (unit_id, origin, position)
);

-- unit_id is the id the unit had on the roster (no foreign key: the roster row is gone).
CREATE TABLE fallen_units (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id BIGINT      NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    unit_id    BIGINT      NOT NULL UNIQUE,
    position   INTEGER     NOT NULL CHECK (position >= 0),
    fate       TEXT        NOT NULL DEFAULT '',
    fell_in    TEXT        NOT NULL DEFAULT '',
    fell_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    snapshot   JSONB       NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'), -- frozen core.CompanyUnit
    UNIQUE (company_id, position) DEFERRABLE INITIALLY DEFERRED
);

-- +goose Down
DROP TABLE fallen_units;
DROP TABLE unit_equipment;
DROP TABLE company_units;
DROP TABLE companies;
