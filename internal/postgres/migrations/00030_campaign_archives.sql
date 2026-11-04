-- +goose Up
-- Ending a campaign archives it (docs/archive.md): one gzipped JSON document (campaign_archives.document) holds what the read-only pages need - the
-- settings, the stored state of every company, the matches, the grants - and the three rules files pruned to what the campaign used. The rows
-- the document replaces are deleted in the same transaction, and the campaign stops pinning its rules files (the hash columns become NULL;
-- the archive carries its own copy). The campaign row, its members and the identity of its companies stay: the home list, the Users page
-- and the member list read them.
ALTER TABLE campaigns ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE campaigns ADD COLUMN archived_matches INTEGER;  -- the counts the lists show, kept since the matches are gone
ALTER TABLE campaigns ADD COLUMN archived_practice INTEGER;
ALTER TABLE campaigns ALTER COLUMN vanilla_hash DROP NOT NULL;
ALTER TABLE campaigns ALTER COLUMN bc_hash DROP NOT NULL;
ALTER TABLE campaigns ALTER COLUMN glossary_hash DROP NOT NULL;
ALTER TABLE campaigns ADD CONSTRAINT campaigns_pins_unless_archived
    CHECK (archived_at IS NOT NULL OR (vanilla_hash IS NOT NULL AND bc_hash IS NOT NULL AND glossary_hash IS NOT NULL));
ALTER TABLE campaigns ADD CONSTRAINT campaigns_archived_is_finished CHECK (archived_at IS NULL OR status = 'finished');

-- Set on every company of the campaign when it is archived: a company read tells at once that its state is in the archive (no second query), and
-- the figures the lists show (units, wins, draws, losses) are kept here, since the rows they were counted from are gone.
ALTER TABLE companies ADD COLUMN archived_at TIMESTAMPTZ;
ALTER TABLE companies ADD COLUMN archived_units INTEGER;
ALTER TABLE companies ADD COLUMN archived_wins INTEGER;
ALTER TABLE companies ADD COLUMN archived_draws INTEGER;
ALTER TABLE companies ADD COLUMN archived_losses INTEGER;

CREATE TABLE campaign_archives (
    campaign_id BIGINT      PRIMARY KEY REFERENCES campaigns (id) ON DELETE CASCADE,
    format      INTEGER     NOT NULL CHECK (format >= 1),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    raw_size    INTEGER     NOT NULL CHECK (raw_size >= 0),   -- bytes of the JSON document before it was gzipped
    packed_size INTEGER     NOT NULL CHECK (packed_size = octet_length(document)),
    companies   INTEGER     NOT NULL CHECK (companies >= 0),
    matches     INTEGER     NOT NULL CHECK (matches >= 0),
    files       JSONB       NOT NULL CHECK (jsonb_typeof(files) = 'array'), -- the three files the campaign was played with (kind, name, version, hash)
    document    BYTEA       NOT NULL
);

-- An archived campaign cannot change: the services refuse first, these triggers are the last word. SQLSTATE P0480 is "campaign archived".
-- +goose StatementBegin
CREATE FUNCTION refuse_archived_by_campaign() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM campaigns WHERE id = NEW.campaign_id AND archived_at IS NOT NULL) THEN
        RAISE EXCEPTION 'campaign % is archived', NEW.campaign_id USING ERRCODE = 'P0480';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION refuse_archived_by_company() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM companies WHERE id = NEW.company_id AND archived_at IS NOT NULL) THEN
        RAISE EXCEPTION 'the campaign of company % is archived', NEW.company_id USING ERRCODE = 'P0480';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION refuse_archived_company_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.archived_at IS NOT NULL THEN
        RAISE EXCEPTION 'the campaign of company % is archived', OLD.id USING ERRCODE = 'P0480';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

CREATE TRIGGER companies_not_archived_ins BEFORE INSERT ON companies FOR EACH ROW EXECUTE FUNCTION refuse_archived_by_campaign();
CREATE TRIGGER companies_not_archived_upd BEFORE UPDATE ON companies FOR EACH ROW EXECUTE FUNCTION refuse_archived_company_change();
CREATE TRIGGER matches_not_archived BEFORE INSERT ON matches FOR EACH ROW EXECUTE FUNCTION refuse_archived_by_campaign();
CREATE TRIGGER invites_not_archived BEFORE INSERT ON campaign_invites FOR EACH ROW EXECUTE FUNCTION refuse_archived_by_campaign();
CREATE TRIGGER members_not_archived BEFORE INSERT ON campaign_members FOR EACH ROW EXECUTE FUNCTION refuse_archived_by_campaign();
CREATE TRIGGER company_units_not_archived BEFORE INSERT OR UPDATE ON company_units FOR EACH ROW EXECUTE FUNCTION refuse_archived_by_company();
CREATE TRIGGER fallen_units_not_archived BEFORE INSERT OR UPDATE ON fallen_units FOR EACH ROW EXECUTE FUNCTION refuse_archived_by_company();
CREATE TRIGGER company_perks_not_archived BEFORE INSERT OR UPDATE ON company_perks FOR EACH ROW EXECUTE FUNCTION refuse_archived_by_company();
CREATE TRIGGER grants_not_archived BEFORE INSERT ON grants FOR EACH ROW EXECUTE FUNCTION refuse_archived_by_company();

-- +goose Down
-- Going back would need the rules files the archives dropped, so it refuses while any archive exists.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM campaigns WHERE archived_at IS NOT NULL) THEN
        RAISE EXCEPTION 'archived campaigns exist; their rules files are gone, so this migration cannot be undone';
    END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER grants_not_archived ON grants;
DROP TRIGGER company_perks_not_archived ON company_perks;
DROP TRIGGER fallen_units_not_archived ON fallen_units;
DROP TRIGGER company_units_not_archived ON company_units;
DROP TRIGGER members_not_archived ON campaign_members;
DROP TRIGGER invites_not_archived ON campaign_invites;
DROP TRIGGER matches_not_archived ON matches;
DROP TRIGGER companies_not_archived_upd ON companies;
DROP TRIGGER companies_not_archived_ins ON companies;
DROP FUNCTION refuse_archived_company_change();
DROP FUNCTION refuse_archived_by_company();
DROP FUNCTION refuse_archived_by_campaign();
DROP TABLE campaign_archives;
ALTER TABLE companies DROP COLUMN archived_losses, DROP COLUMN archived_draws, DROP COLUMN archived_wins, DROP COLUMN archived_units, DROP COLUMN archived_at;
ALTER TABLE campaigns DROP CONSTRAINT campaigns_archived_is_finished;
ALTER TABLE campaigns DROP CONSTRAINT campaigns_pins_unless_archived;
ALTER TABLE campaigns ALTER COLUMN glossary_hash SET NOT NULL;
ALTER TABLE campaigns ALTER COLUMN bc_hash SET NOT NULL;
ALTER TABLE campaigns ALTER COLUMN vanilla_hash SET NOT NULL;
ALTER TABLE campaigns DROP COLUMN archived_practice, DROP COLUMN archived_matches, DROP COLUMN archived_at;
