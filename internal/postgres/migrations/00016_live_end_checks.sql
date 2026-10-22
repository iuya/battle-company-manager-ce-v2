-- +goose Up
-- New kinds of live log entries: the objective flag and the MVP vote of the end checks, and the secret objectives of Fog of War
-- (nomination and completion, visible to their author only until the match ends).
ALTER TABLE match_live_log DROP CONSTRAINT match_live_log_kind_check;
ALTER TABLE match_live_log ADD CONSTRAINT match_live_log_kind_check
    CHECK (kind IN ('stat', 'wound', 'fate', 'broken', 'retreat', 'died', 'mount', 'return', 'objective', 'mvpVote', 'secretNominate', 'secretDone'));

-- +goose Down
DELETE FROM match_live_log WHERE kind IN ('objective', 'mvpVote', 'secretNominate', 'secretDone');
ALTER TABLE match_live_log DROP CONSTRAINT match_live_log_kind_check;
ALTER TABLE match_live_log ADD CONSTRAINT match_live_log_kind_check
    CHECK (kind IN ('stat', 'wound', 'fate', 'broken', 'retreat', 'died', 'mount', 'return'));
