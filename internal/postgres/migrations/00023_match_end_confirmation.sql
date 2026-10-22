-- +goose Up
-- Ending a match needs the confirmation of every participant (docs/matches.md, "Ending a match: confirmation by all participants").
-- One proposal per open match; one confirmation row per company that confirmed (a waiting company has no row). The proposal and its
-- confirmations are dropped when the match ends or the proposal is cancelled.
CREATE TABLE match_end_proposal (
    match_id    BIGINT      PRIMARY KEY REFERENCES matches (id) ON DELETE CASCADE,
    proposed_by BIGINT      NOT NULL REFERENCES users (id),
    proposed_on TIMESTAMPTZ NOT NULL DEFAULT now(),
    notice      TEXT        NOT NULL DEFAULT '', -- why the confirmations were reset ("Hador changed the result; confirm again")
    revision    INTEGER     NOT NULL DEFAULT 1 CHECK (revision >= 1) -- saves of this proposal
);

CREATE TABLE match_end_confirmation (
    match_id     BIGINT      NOT NULL REFERENCES match_end_proposal (match_id) ON DELETE CASCADE,
    company_id   BIGINT      NOT NULL,
    confirmed_by BIGINT      NOT NULL REFERENCES users (id),
    confirmed_on TIMESTAMPTZ NOT NULL DEFAULT now(),
    override     BOOLEAN     NOT NULL DEFAULT false, -- confirmed by the organiser or an administrator, not by the owner
    note         TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (match_id, company_id),
    FOREIGN KEY (match_id, company_id) REFERENCES match_side_companies (match_id, company_id) ON DELETE CASCADE
);

-- Who confirmed the end, or the override and its note.
ALTER TABLE match_events ADD COLUMN detail TEXT NOT NULL DEFAULT '';

-- Informational log entries of a pending end (not undoable, not part of the fold).
ALTER TABLE match_live_log DROP CONSTRAINT match_live_log_kind_check;
ALTER TABLE match_live_log ADD CONSTRAINT match_live_log_kind_check
    CHECK (kind IN ('stat', 'wound', 'fate', 'broken', 'retreat', 'died', 'mount', 'return', 'objective', 'mvpVote', 'secretNominate', 'secretDone',
                    'turn', 'breakAdjust', 'courageSkip', 'allIsLost', 'itemUsed',
                    'endProposed', 'endConfirmed', 'endWithdrawn', 'endCancelled', 'endOverride'));

-- +goose Down
DELETE FROM match_live_log WHERE kind IN ('endProposed', 'endConfirmed', 'endWithdrawn', 'endCancelled', 'endOverride');
ALTER TABLE match_live_log DROP CONSTRAINT match_live_log_kind_check;
ALTER TABLE match_live_log ADD CONSTRAINT match_live_log_kind_check
    CHECK (kind IN ('stat', 'wound', 'fate', 'broken', 'retreat', 'died', 'mount', 'return', 'objective', 'mvpVote', 'secretNominate', 'secretDone',
                    'turn', 'breakAdjust', 'courageSkip', 'allIsLost', 'itemUsed'));
ALTER TABLE match_events DROP COLUMN detail;
DROP TABLE match_end_confirmation;
DROP TABLE match_end_proposal;
