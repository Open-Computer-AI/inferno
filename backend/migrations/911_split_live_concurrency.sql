-- 911: persist opt-in independent Live/request concurrency budgets.
--
-- Existing rows remain in shared mode (both nullable columns are NULL). The
-- explicit IS NULL/IS NOT NULL predicates are intentional: PostgreSQL CHECK
-- constraints accept UNKNOWN, so a shorter boolean expression could admit a
-- half-configured policy.

ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS live_concurrency INTEGER,
    ADD COLUMN IF NOT EXISTS request_concurrency INTEGER,
    ADD COLUMN IF NOT EXISTS concurrency_policy_generation BIGINT NOT NULL DEFAULT 0;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS live_concurrency INTEGER,
    ADD COLUMN IF NOT EXISTS request_concurrency INTEGER,
    ADD COLUMN IF NOT EXISTS concurrency_policy_generation BIGINT NOT NULL DEFAULT 0;

ALTER TABLE accounts
    DROP CONSTRAINT IF EXISTS accounts_split_concurrency_pair_check,
    ADD CONSTRAINT accounts_split_concurrency_pair_check CHECK (
        (live_concurrency IS NULL AND request_concurrency IS NULL)
        OR (
            live_concurrency IS NOT NULL
            AND request_concurrency IS NOT NULL
            AND live_concurrency > 0
            AND request_concurrency > 0
        )
    );

-- The generation belongs to the durable row, not to any one application
-- writer.  Ent updates, raw SQL batches, redeem adjustments, and future
-- writers all pass through these triggers.  The AFTER trigger records the
-- trigger-adjusted policy in the same transaction as the row update.
CREATE TABLE IF NOT EXISTS concurrency_policy_outbox (
    id BIGSERIAL PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK (entity_type IN ('account', 'user')),
    entity_id BIGINT NOT NULL,
    generation BIGINT NOT NULL CHECK (generation >= 0),
    shared_concurrency INTEGER NOT NULL,
    live_concurrency INTEGER,
    request_concurrency INTEGER,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_until TIMESTAMPTZ,
    claimed_by TEXT,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT concurrency_policy_outbox_pair_check CHECK (
        (live_concurrency IS NULL AND request_concurrency IS NULL)
        OR (
            live_concurrency IS NOT NULL
            AND request_concurrency IS NOT NULL
            AND live_concurrency > 0
            AND request_concurrency > 0
        )
    ),
    CONSTRAINT concurrency_policy_outbox_generation_key UNIQUE (entity_type, entity_id, generation)
);

CREATE INDEX IF NOT EXISTS idx_concurrency_policy_outbox_pending
    ON concurrency_policy_outbox (available_at, id)
    WHERE published_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_concurrency_policy_outbox_lease
    ON concurrency_policy_outbox (claimed_until)
    WHERE published_at IS NULL AND claimed_until IS NOT NULL;

CREATE OR REPLACE FUNCTION bump_concurrency_policy_generation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
	IF TG_OP = 'INSERT' THEN
		-- The generation is database-owned, including on callers that happen
		-- to set the generated Ent field during a create.
		NEW.concurrency_policy_generation := 0;
		RETURN NEW;
	END IF;

    IF NEW.concurrency IS DISTINCT FROM OLD.concurrency
       OR NEW.live_concurrency IS DISTINCT FROM OLD.live_concurrency
       OR NEW.request_concurrency IS DISTINCT FROM OLD.request_concurrency THEN
        NEW.concurrency_policy_generation := OLD.concurrency_policy_generation + 1;
	ELSE
		-- Ignore generation-only writes.  A caller may never advance the
		-- durable fence without changing the effective policy in this row.
		NEW.concurrency_policy_generation := OLD.concurrency_policy_generation;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enqueue_concurrency_policy_outbox()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_type TEXT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.concurrency IS NOT DISTINCT FROM OLD.concurrency
       AND NEW.live_concurrency IS NOT DISTINCT FROM OLD.live_concurrency
       AND NEW.request_concurrency IS NOT DISTINCT FROM OLD.request_concurrency THEN
        RETURN NEW;
    END IF;

    target_type := CASE WHEN TG_TABLE_NAME = 'accounts' THEN 'account' ELSE 'user' END;
    INSERT INTO concurrency_policy_outbox (
        entity_type,
        entity_id,
        generation,
        shared_concurrency,
        live_concurrency,
        request_concurrency
    ) VALUES (
        target_type,
        NEW.id,
        NEW.concurrency_policy_generation,
        NEW.concurrency,
        NEW.live_concurrency,
        NEW.request_concurrency
    )
    ON CONFLICT (entity_type, entity_id, generation) DO NOTHING;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_accounts_concurrency_policy_generation ON accounts;
CREATE TRIGGER trg_accounts_concurrency_policy_generation
BEFORE INSERT OR UPDATE OF concurrency, live_concurrency, request_concurrency, concurrency_policy_generation ON accounts
FOR EACH ROW EXECUTE FUNCTION bump_concurrency_policy_generation();

DROP TRIGGER IF EXISTS trg_users_concurrency_policy_generation ON users;
CREATE TRIGGER trg_users_concurrency_policy_generation
BEFORE INSERT OR UPDATE OF concurrency, live_concurrency, request_concurrency, concurrency_policy_generation ON users
FOR EACH ROW EXECUTE FUNCTION bump_concurrency_policy_generation();

DROP TRIGGER IF EXISTS trg_accounts_concurrency_policy_outbox ON accounts;
CREATE TRIGGER trg_accounts_concurrency_policy_outbox
AFTER INSERT OR UPDATE OF concurrency, live_concurrency, request_concurrency, concurrency_policy_generation ON accounts
FOR EACH ROW EXECUTE FUNCTION enqueue_concurrency_policy_outbox();

DROP TRIGGER IF EXISTS trg_users_concurrency_policy_outbox ON users;
CREATE TRIGGER trg_users_concurrency_policy_outbox
AFTER INSERT OR UPDATE OF concurrency, live_concurrency, request_concurrency, concurrency_policy_generation ON users
FOR EACH ROW EXECUTE FUNCTION enqueue_concurrency_policy_outbox();

-- Existing rows need a durable starting point as well.  This makes startup
-- recovery independent of whether the row was changed after migration 911.
INSERT INTO concurrency_policy_outbox (
    entity_type,
    entity_id,
    generation,
    shared_concurrency,
    live_concurrency,
    request_concurrency
)
SELECT 'account', id, concurrency_policy_generation, concurrency,
       live_concurrency, request_concurrency
FROM accounts
ON CONFLICT (entity_type, entity_id, generation) DO NOTHING;

INSERT INTO concurrency_policy_outbox (
    entity_type,
    entity_id,
    generation,
    shared_concurrency,
    live_concurrency,
    request_concurrency
)
SELECT 'user', id, concurrency_policy_generation, concurrency,
       live_concurrency, request_concurrency
FROM users
ON CONFLICT (entity_type, entity_id, generation) DO NOTHING;

-- Policy changes affect API-key-authenticated user snapshots as well as the
-- explicit policy cache.  Keep this trigger's existing status/role/deletion
-- coverage and add every effective policy field plus its durable generation.
CREATE OR REPLACE FUNCTION enqueue_user_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_user_id BIGINT;
BEGIN
    target_user_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.role IS NOT DISTINCT FROM NEW.role
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at
       AND OLD.concurrency IS NOT DISTINCT FROM NEW.concurrency
       AND OLD.live_concurrency IS NOT DISTINCT FROM NEW.live_concurrency
       AND OLD.request_concurrency IS NOT DISTINCT FROM NEW.request_concurrency
       AND OLD.concurrency_policy_generation IS NOT DISTINCT FROM NEW.concurrency_policy_generation THEN
        RETURN NEW;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.user_id = target_user_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

-- Keep the function replacement above effective on databases that already had
-- the older trigger definition.
DROP TRIGGER IF EXISTS trg_users_auth_cache_invalidation ON users;
CREATE TRIGGER trg_users_auth_cache_invalidation
AFTER UPDATE OR DELETE ON users
FOR EACH ROW EXECUTE FUNCTION enqueue_user_auth_cache_invalidation();

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_split_concurrency_pair_check,
    ADD CONSTRAINT users_split_concurrency_pair_check CHECK (
        (live_concurrency IS NULL AND request_concurrency IS NULL)
        OR (
            live_concurrency IS NOT NULL
            AND request_concurrency IS NOT NULL
            AND live_concurrency > 0
            AND request_concurrency > 0
        )
    );
