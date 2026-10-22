-- +goose Up
-- A campaign is played with three rules files (rows of rulesets, content addressed). The generated *_kind columns with the
-- composite foreign keys make the database check that each hash names a file of the right kind.
ALTER TABLE rulesets ADD CONSTRAINT rulesets_hash_kind_key UNIQUE (hash, kind);

CREATE TABLE campaigns (
    id            BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    description   TEXT        NOT NULL DEFAULT '' CHECK (char_length(description) <= 280),
    status        TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'finished')),
    round         INTEGER     NOT NULL DEFAULT 0 CHECK (round >= 0),
    rounds        INTEGER     NOT NULL DEFAULT 0 CHECK (rounds >= 0),
    vanilla_hash  TEXT        NOT NULL,
    bc_hash       TEXT        NOT NULL,
    glossary_hash TEXT        NOT NULL,
    vanilla_kind  TEXT        GENERATED ALWAYS AS ('vanilla') STORED,
    bc_kind       TEXT        GENERATED ALWAYS AS ('battle_companies') STORED,
    glossary_kind TEXT        GENERATED ALWAYS AS ('glossary') STORED,
    -- opaque to SQL, versioned by the "v" key
    settings      JSONB       NOT NULL DEFAULT '{"v": 1}' CHECK (jsonb_typeof(settings) = 'object'),
    created_by    BIGINT      NOT NULL REFERENCES users (id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (vanilla_hash, vanilla_kind)   REFERENCES rulesets (hash, kind),
    FOREIGN KEY (bc_hash, bc_kind)             REFERENCES rulesets (hash, kind),
    FOREIGN KEY (glossary_hash, glossary_kind) REFERENCES rulesets (hash, kind)
);

CREATE TABLE campaign_members (
    campaign_id BIGINT      NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    user_id     BIGINT      NOT NULL REFERENCES users (id),
    role        TEXT        NOT NULL CHECK (role IN ('organiser', 'player')),
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, user_id)
);
CREATE INDEX campaign_members_user_idx ON campaign_members (user_id);
CREATE UNIQUE INDEX campaign_members_organiser_key ON campaign_members (campaign_id) WHERE role = 'organiser';

-- +goose Down
DROP TABLE campaign_members;
DROP TABLE campaigns;
ALTER TABLE rulesets DROP CONSTRAINT rulesets_hash_kind_key;
