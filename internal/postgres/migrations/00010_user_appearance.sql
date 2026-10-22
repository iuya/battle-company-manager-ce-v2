-- +goose Up
-- NULL means the user never chose; the keys are the ones core.Palettes and core.Modes list.
ALTER TABLE users
    ADD COLUMN palette TEXT CHECK (palette IN ('forest', 'ember', 'mithril')),
    ADD COLUMN mode    TEXT CHECK (mode IN ('system', 'light', 'dark')),
    ADD CONSTRAINT users_appearance_both CHECK ((palette IS NULL) = (mode IS NULL));

-- +goose Down
ALTER TABLE users DROP CONSTRAINT users_appearance_both, DROP COLUMN mode, DROP COLUMN palette;
