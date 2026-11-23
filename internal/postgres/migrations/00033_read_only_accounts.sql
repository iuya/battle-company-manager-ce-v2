-- +goose Up
-- A read-only account (the smoke-test account of a deployment): it signs in and reads, and every other request is refused by the web
-- layer. Only a superuser sets it (audit action read_only.changed).
ALTER TABLE users ADD COLUMN read_only BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK (action IN (
    'role.changed', 'campaign_limit.changed', 'read_only.changed',
    'invitation.created', 'invitation.revoked', 'invitation.used',
    'recovery.created', 'recovery.revoked', 'recovery.used', 'recovery.invalidated'));

-- +goose Down
DELETE FROM audit_events WHERE action = 'read_only.changed';
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK (action IN (
    'role.changed', 'campaign_limit.changed',
    'invitation.created', 'invitation.revoked', 'invitation.used',
    'recovery.created', 'recovery.revoked', 'recovery.used', 'recovery.invalidated'));
ALTER TABLE users DROP COLUMN read_only;
