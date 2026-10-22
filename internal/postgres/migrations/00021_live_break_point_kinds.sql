-- +goose Up
-- New kinds of live log entries: the turn boundary, the manual correction of the Break Point counts, the courage tests passed without a
-- roll, the All is Lost choice of a Broken company and the use of a loaned or owned consumable (epic 36y). Their fields are part of the
-- jsonb payload; match-level entries (turn) have company_id and unit_id 0.
ALTER TABLE match_live_log DROP CONSTRAINT match_live_log_kind_check;
ALTER TABLE match_live_log ADD CONSTRAINT match_live_log_kind_check
    CHECK (kind IN ('stat', 'wound', 'fate', 'broken', 'retreat', 'died', 'mount', 'return', 'objective', 'mvpVote', 'secretNominate', 'secretDone',
                    'turn', 'breakAdjust', 'courageSkip', 'allIsLost', 'itemUsed'));

-- +goose Down
DELETE FROM match_live_log WHERE kind IN ('turn', 'breakAdjust', 'courageSkip', 'allIsLost', 'itemUsed');
ALTER TABLE match_live_log DROP CONSTRAINT match_live_log_kind_check;
ALTER TABLE match_live_log ADD CONSTRAINT match_live_log_kind_check
    CHECK (kind IN ('stat', 'wound', 'fate', 'broken', 'retreat', 'died', 'mount', 'return', 'objective', 'mvpVote', 'secretNominate', 'secretDone'));
