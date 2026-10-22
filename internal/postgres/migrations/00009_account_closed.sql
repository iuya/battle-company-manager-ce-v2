-- +goose Up
-- A closed account is anonymised, not deleted: its companies and matches stay for the other players.
ALTER TABLE users ADD COLUMN closed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN closed_at;
