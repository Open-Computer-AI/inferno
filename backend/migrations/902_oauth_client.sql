CREATE TABLE IF NOT EXISTS oauth_clients (
    id                  BIGSERIAL PRIMARY KEY,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    client_id           VARCHAR(128) NOT NULL UNIQUE,
    kind                VARCHAR(16)  NOT NULL DEFAULT 'SELF_HOSTED',
    name                VARCHAR(64)  NOT NULL,
    owner_user_id       BIGINT       NOT NULL,
    org_id              BIGINT       NOT NULL,
    instance_id         VARCHAR(128),
    status              VARCHAR(16)  NOT NULL DEFAULT 'pending',
    redirect_uri_origin VARCHAR(255) NOT NULL,
    revoked_at          TIMESTAMPTZ
);

-- Nulls are excluded (and Postgres unique indexes already treat NULL as
-- distinct from other NULLs), so any number of clients with no instance_id
-- yet (i.e. all of them, as of this task) can coexist while at most one
-- client per instance_id is enforced once oc-platform starts setting it.
-- Mirrors migrations/904_org_personal_user_id.sql.
CREATE UNIQUE INDEX IF NOT EXISTS oauth_clients_instance_id_key
    ON oauth_clients (instance_id)
    WHERE instance_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS oauth_clients_org_id_idx        ON oauth_clients (org_id);
CREATE INDEX IF NOT EXISTS oauth_clients_owner_user_id_idx ON oauth_clients (owner_user_id);
CREATE INDEX IF NOT EXISTS oauth_clients_status_idx        ON oauth_clients (status);
