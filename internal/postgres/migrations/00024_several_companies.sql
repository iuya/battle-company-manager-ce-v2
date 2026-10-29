-- +goose Up
-- A player may own several companies in one campaign (campaign setting "companies per player", stored in campaigns.settings JSON, so no
-- column). The cap is enforced by companyStore.Create under a lock on the owner's campaign_members row, not by a constraint: it is per
-- campaign and variable. companies_member_fk stays: it keeps a company's owner a member.
ALTER TABLE companies DROP CONSTRAINT companies_campaign_owner_key;
CREATE INDEX companies_owner_idx ON companies (campaign_id, owner_id, id);

-- +goose Down
-- Refuses (the unique constraint cannot be added) while a player owns two companies in one campaign: that is the right answer.
DROP INDEX companies_owner_idx;
ALTER TABLE companies ADD CONSTRAINT companies_campaign_owner_key UNIQUE (campaign_id, owner_id);
