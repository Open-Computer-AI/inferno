-- 235: add the enforcing model allowlist beside the legacy display-only list.
-- Existing groups using models_list_config must not become request-blocked merely
-- because this migration is applied; model_allowlist is opt-in and disabled.
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

COMMENT ON COLUMN groups.model_allowlist IS
    'Group model allowlist: constrains both model listing responses and request admission';

COMMENT ON COLUMN groups.models_list_config IS
    'Legacy display-only /v1/models configuration; does not gate request admission';
