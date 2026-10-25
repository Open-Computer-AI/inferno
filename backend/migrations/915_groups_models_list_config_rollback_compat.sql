-- 915: keep groups.models_list_config next to model_allowlist, so an image from before 235 can run on this database.
--
-- 235/236 rename models_list_config to model_allowlist. Images built before them (the stage line up to a71629670,
-- production c595d020e) still select models_list_config in every groups query, so rolling back to one of them after
-- this release has booted once (by hand, or automatically when /health is late) would fail API-key auth and every
-- group load with "column groups.models_list_config does not exist".
--
-- Additive and replayable: the current code never reads this column (it reads model_allowlist only), and 236 already
-- treats "both columns present" as a valid state. Before 235 the column only shaped the /v1/models listing, so after
-- a rollback the old image reads the copy taken here and behaves as it did before the upgrade.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE groups SET models_list_config = model_allowlist
WHERE models_list_config = '{}'::jsonb AND model_allowlist <> '{}'::jsonb;
