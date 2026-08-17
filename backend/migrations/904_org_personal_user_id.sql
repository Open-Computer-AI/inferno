ALTER TABLE orgs
    ADD COLUMN IF NOT EXISTS personal_user_id BIGINT;

-- Nulls are excluded (and Postgres unique indexes already treat NULL as
-- distinct from other NULLs), so any number of non-personal orgs can coexist
-- while at most one personal org per user is enforced at the database level,
-- even under concurrent EnsurePersonalOrg callers across multiple processes.
CREATE UNIQUE INDEX IF NOT EXISTS orgs_personal_user_id_key
    ON orgs (personal_user_id)
    WHERE personal_user_id IS NOT NULL;
