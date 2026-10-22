-- +goose Up
-- The scaffold table used to validate the first pages; companies will belong to campaigns.
DROP TABLE companies;

-- +goose Down
CREATE TABLE companies (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
