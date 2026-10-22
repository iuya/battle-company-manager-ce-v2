-- +goose Up
-- Live match mode (docs/architecture.md, "Live match"): the starting values of the units, the per-match counter and the log of
-- actions. State is a fold of the units and the not-undone log rows; nothing else is stored.

-- Existence of a row = the match is initialised; seq is the counter, incremented under the matches row lock.
CREATE TABLE match_live (
    match_id   BIGINT      PRIMARY KEY REFERENCES matches (id) ON DELETE CASCADE,
    seq        BIGINT      NOT NULL DEFAULT 0 CHECK (seq >= 0),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- unit_id has no foreign key: a unit that dies in the aftermath leaves company_units.
CREATE TABLE match_live_unit (
    match_id       BIGINT   NOT NULL REFERENCES match_live (match_id) ON DELETE CASCADE,
    unit_id        BIGINT   NOT NULL,
    company_id     BIGINT   NOT NULL,
    start_wounds   SMALLINT NOT NULL CHECK (start_wounds >= 0),
    start_might    SMALLINT NOT NULL CHECK (start_might >= 0),
    start_will     SMALLINT NOT NULL CHECK (start_will >= 0),
    start_fate     SMALLINT NOT NULL CHECK (start_fate >= 0),
    has_mount      BOOLEAN  NOT NULL DEFAULT false,
    mount_wounds   SMALLINT NOT NULL DEFAULT 0 CHECK (mount_wounds >= 0),
    counts_as_kill BOOLEAN  NOT NULL DEFAULT false,
    absent         BOOLEAN  NOT NULL DEFAULT false,
    PRIMARY KEY (match_id, unit_id),
    FOREIGN KEY (match_id, company_id) REFERENCES match_side_companies (match_id, company_id) ON DELETE CASCADE
);
CREATE INDEX match_live_unit_company_idx ON match_live_unit (match_id, company_id);

-- seq is the entry's own sequence; an undo takes the next seq of the match too (undone_seq) but leaves the entry in place.
CREATE TABLE match_live_log (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    match_id   BIGINT      NOT NULL REFERENCES match_live (match_id) ON DELETE CASCADE,
    seq        BIGINT      NOT NULL CHECK (seq >= 1),
    company_id BIGINT      NOT NULL,
    unit_id    BIGINT      NOT NULL,
    kind       TEXT        NOT NULL CHECK (kind IN ('stat', 'wound', 'fate', 'broken', 'retreat', 'died', 'mount', 'return')),
    payload    JSONB       NOT NULL, -- {"v":1,...}: the LiveAction
    text       TEXT        NOT NULL DEFAULT '',
    touches    BIGINT[]    NOT NULL DEFAULT '{}',
    by_user_id BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    undone_at  TIMESTAMPTZ,
    undone_by  BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    undone_seq BIGINT,
    UNIQUE (match_id, seq),
    CONSTRAINT match_live_log_undone_chk CHECK ((undone_at IS NULL) = (undone_seq IS NULL))
);
CREATE INDEX match_live_log_live_idx ON match_live_log (match_id, seq DESC) WHERE undone_at IS NULL;

-- +goose Down
DROP TABLE match_live_log;
DROP TABLE match_live_unit;
DROP TABLE match_live;
