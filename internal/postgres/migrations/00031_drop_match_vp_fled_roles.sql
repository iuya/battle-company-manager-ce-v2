-- +goose Up
-- Matches report the winner, or a draw, and nothing else (docs/matches.md, "Concepts"): the Victory Points and Attacker / Defender tag of a
-- side and the All is Lost flag of a company are gone, with what older matches stored in them.
DROP INDEX match_sides_role_key;
ALTER TABLE match_sides DROP COLUMN role, DROP COLUMN victory_points;
ALTER TABLE match_side_companies DROP COLUMN fled;

-- +goose Down
-- The columns come back empty: what they held is not restored.
ALTER TABLE match_side_companies ADD COLUMN fled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE match_sides
    ADD COLUMN victory_points SMALLINT CHECK (victory_points BETWEEN 0 AND 99),
    ADD COLUMN role TEXT CHECK (role IN ('attacker', 'defender'));
CREATE UNIQUE INDEX match_sides_role_key ON match_sides (match_id, role) WHERE role IS NOT NULL;
