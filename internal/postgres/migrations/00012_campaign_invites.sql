-- +goose Up
-- An invite link to a campaign. Only the SHA-256 (hex) of the token is stored, like sessions. max_uses 0 is unlimited; uses
-- counts the players the link made members.
CREATE TABLE campaign_invites (
    token_hash  TEXT        PRIMARY KEY CHECK (token_hash ~ '^[0-9a-f]{64}$'),
    campaign_id BIGINT      NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    created_by  BIGINT      NOT NULL REFERENCES users (id),
    expires_at  TIMESTAMPTZ NOT NULL,
    max_uses    INTEGER     NOT NULL DEFAULT 0 CHECK (max_uses >= 0),
    uses        INTEGER     NOT NULL DEFAULT 0 CHECK (uses >= 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX campaign_invites_campaign_idx ON campaign_invites (campaign_id);

-- +goose Down
DROP TABLE campaign_invites;
