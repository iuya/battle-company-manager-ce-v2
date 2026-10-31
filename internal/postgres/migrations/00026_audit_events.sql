-- +goose Up
-- The audit trail of account administration (roles, account invitations, password recovery links). Rows are written by the
-- repositories in the same transaction as the change they record, with the caller's clock. They hold ids only: never a token,
-- a hash, a password or an email address. Never purged.
CREATE TABLE audit_events (
    id              BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at              TIMESTAMPTZ NOT NULL,
    source          TEXT        NOT NULL CHECK (source IN ('web', 'console', 'public', 'system')),
    actor_id        BIGINT      REFERENCES users (id),
    action          TEXT        NOT NULL CHECK (action IN (
                        'role.changed',
                        'invitation.created', 'invitation.revoked', 'invitation.used',
                        'recovery.created', 'recovery.revoked', 'recovery.used', 'recovery.invalidated')),
    subject_user_id BIGINT      REFERENCES users (id),
    target_kind     TEXT        CHECK (target_kind IN ('account_invitation', 'recovery_link')),
    target_id       BIGINT,
    detail          JSONB       NOT NULL DEFAULT '{}',
    CHECK (source <> 'web' OR actor_id IS NOT NULL),
    CHECK ((target_kind IS NULL) = (target_id IS NULL))
);
CREATE INDEX audit_events_at_idx ON audit_events (at DESC);
CREATE INDEX audit_events_subject_idx ON audit_events (subject_user_id, at DESC) WHERE subject_user_id IS NOT NULL;
CREATE INDEX audit_events_target_idx ON audit_events (target_kind, target_id);

-- +goose Down
DROP TABLE audit_events;
