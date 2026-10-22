-- +goose Up
-- An organiser can invalidate the links of a campaign (and ending the campaign does): the row stays, the link stops working.
ALTER TABLE campaign_invites ADD COLUMN revoked_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE campaign_invites DROP COLUMN revoked_at;
