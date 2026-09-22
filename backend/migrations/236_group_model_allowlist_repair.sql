-- 236: repair partially migrated groups without renaming away the legacy list.
--
-- This is idempotent and handles either column, both columns, or neither.
-- No legacy configuration is promoted to an enforcing allowlist automatically.
DO $$
BEGIN
    IF to_regclass('groups') IS NOT NULL THEN
        ALTER TABLE groups
            ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;
        ALTER TABLE groups
            ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;
    END IF;
END
$$;

UPDATE groups SET model_allowlist = '{}'::jsonb WHERE model_allowlist IS NULL;
UPDATE groups SET models_list_config = '{}'::jsonb WHERE models_list_config IS NULL;

ALTER TABLE groups ALTER COLUMN model_allowlist SET DEFAULT '{}'::jsonb;
ALTER TABLE groups ALTER COLUMN model_allowlist SET NOT NULL;
ALTER TABLE groups ALTER COLUMN models_list_config SET DEFAULT '{}'::jsonb;
ALTER TABLE groups ALTER COLUMN models_list_config SET NOT NULL;

COMMENT ON COLUMN groups.model_allowlist IS
    'Group model allowlist: constrains both model listing responses and request admission';

COMMENT ON COLUMN groups.models_list_config IS
    'Legacy display-only /v1/models configuration; does not gate request admission';
