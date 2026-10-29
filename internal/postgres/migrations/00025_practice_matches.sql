-- +goose Up
-- A practice match awards nothing and has no aftermath (docs/practice-match-spec.md). The flag is set when the match is created and never
-- updated. The aftermath of a practice match is derived as done and empty by the queries, nothing is written to match_aftermaths.
ALTER TABLE matches ADD COLUMN practice BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE matches DROP COLUMN practice;
