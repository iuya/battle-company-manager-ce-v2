-- +goose Up
-- Processes that wait for the player between two requests: the company's recruit in progress, and a unit's level-up roll or
-- treatment. Opaque to SQL and versioned with the rules code, so jsonb with a top-level "v" (like progress and injuries).
ALTER TABLE companies
    ADD COLUMN pending JSONB NOT NULL DEFAULT '{"v": 1}' CHECK (jsonb_typeof(pending) = 'object');
ALTER TABLE company_units
    ADD COLUMN pending JSONB NOT NULL DEFAULT '{"v": 1}' CHECK (jsonb_typeof(pending) = 'object');

-- +goose Down
ALTER TABLE company_units DROP COLUMN pending;
ALTER TABLE companies DROP COLUMN pending;
