-- +goose Up
-- Against the Odds is decided on the match page before the game (docs/matches.md, "Against the Odds before the game"): the lower rated player's
-- confirmation is kept with the match. Matches that ended, have live entries or a pending end are past that point; open ones that have not
-- started stay pending.
ALTER TABLE matches ADD COLUMN ato_decided BOOLEAN NOT NULL DEFAULT false;
UPDATE matches m SET ato_decided = true
 WHERE m.ended
    OR EXISTS (SELECT 1 FROM match_live_log l WHERE l.match_id = m.id AND l.undone_at IS NULL)
    OR EXISTS (SELECT 1 FROM match_end_proposal p WHERE p.match_id = m.id);

-- +goose Down
ALTER TABLE matches DROP COLUMN ato_decided;
