-- +goose Up
ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin', 'superuser')),
    ADD COLUMN can_create_campaigns BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users DROP COLUMN can_create_campaigns, DROP COLUMN role;
