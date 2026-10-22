-- +goose Up
-- The tag of a team in a scenario with an Attacker and a Defender; at most one side of a match has each.
ALTER TABLE match_sides ADD COLUMN role TEXT CHECK (role IN ('attacker', 'defender'));
CREATE UNIQUE INDEX match_sides_role_key ON match_sides (match_id, role) WHERE role IS NOT NULL;

-- +goose Down
DROP INDEX match_sides_role_key;
ALTER TABLE match_sides DROP COLUMN role;
