-- +goose Up
-- The IANA time zone a user wants times shown in; NULL means automatic (the browser's zone). Stored timestamps stay UTC.
ALTER TABLE users ADD COLUMN timezone TEXT CHECK (timezone IS NULL OR char_length(timezone) BETWEEN 1 AND 64);

-- +goose Down
ALTER TABLE users DROP COLUMN timezone;
