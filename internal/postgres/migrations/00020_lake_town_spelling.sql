-- +goose Up
-- The company is called Lake-town (lowercase t) in the rules since the files were regenerated. Companies of campaigns pinned to a
-- rules file that spells it so are renamed; campaigns pinned to an older file keep "Lake-Town" (the app also resolves a company name
-- ignoring case, so either spelling works).
UPDATE companies c SET faction = 'Lake-town'
  FROM campaigns g JOIN rulesets r ON r.hash = g.bc_hash
 WHERE c.campaign_id = g.id AND c.faction = 'Lake-Town'
   AND position('"name":"Lake-town"'::bytea IN r.content) > 0;

-- +goose Down
-- Not reversible without knowing which rows were changed; the spellings are equivalent for the app.
SELECT 1;
