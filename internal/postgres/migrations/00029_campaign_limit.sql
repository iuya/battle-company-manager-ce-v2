-- +goose Up
-- How many unfinished (active or paused) campaigns an account may organise. NULL: the server's default (CAMPAIGN_LIMIT, 10).
-- Only a superuser changes it (audit action campaign_limit.changed).
ALTER TABLE users ADD COLUMN max_campaigns INTEGER CHECK (max_campaigns IS NULL OR max_campaigns >= 0);

ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK (action IN (
    'role.changed', 'campaign_limit.changed',
    'invitation.created', 'invitation.revoked', 'invitation.used',
    'recovery.created', 'recovery.revoked', 'recovery.used', 'recovery.invalidated'));

-- +goose Down
DELETE FROM audit_events WHERE action = 'campaign_limit.changed';
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK (action IN (
    'role.changed',
    'invitation.created', 'invitation.revoked', 'invitation.used',
    'recovery.created', 'recovery.revoked', 'recovery.used', 'recovery.invalidated'));
ALTER TABLE users DROP COLUMN max_campaigns;
