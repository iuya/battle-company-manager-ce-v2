-- +goose Up
-- Invitations to create an account: named (one address, one use) or anonymous (anyone with the link, up to max_uses accounts).
-- Only the SHA-256 of the token is stored. The state (pending, used, expired, revoked) is derived, never stored.
CREATE TABLE account_invitations (
    id                   BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash           BYTEA       NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    kind                 TEXT        NOT NULL CHECK (kind IN ('named', 'anonymous')),
    email                TEXT,
    can_create_campaigns BOOLEAN     NOT NULL DEFAULT false,
    max_uses             INTEGER     NOT NULL CHECK (max_uses BETWEEN 1 AND 50),
    uses                 INTEGER     NOT NULL DEFAULT 0 CHECK (uses >= 0 AND uses <= max_uses),
    created_by           BIGINT      NOT NULL REFERENCES users (id),
    created_at           TIMESTAMPTZ NOT NULL,
    expires_at           TIMESTAMPTZ NOT NULL,
    revoked_at           TIMESTAMPTZ,
    revoked_by           BIGINT      REFERENCES users (id),
    CHECK (expires_at > created_at AND expires_at <= created_at + interval '30 days'),
    CHECK ((kind = 'named' AND email IS NOT NULL AND max_uses = 1) OR (kind = 'anonymous' AND email IS NULL)),
    CHECK ((revoked_at IS NULL) = (revoked_by IS NULL))
);
CREATE INDEX account_invitations_list_idx ON account_invitations (created_at DESC, id DESC);
CREATE INDEX account_invitations_email_idx ON account_invitations (lower(email)) WHERE email IS NOT NULL;

-- The accounts made through an invitation (kept with the invitation; a user comes from at most one invitation).
CREATE TABLE account_invitation_uses (
    invitation_id BIGINT      NOT NULL REFERENCES account_invitations (id),
    user_id       BIGINT      NOT NULL UNIQUE REFERENCES users (id),
    used_at       TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (invitation_id, user_id)
);

-- +goose Down
DROP TABLE account_invitation_uses;
DROP TABLE account_invitations;
