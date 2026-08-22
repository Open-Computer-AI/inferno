CREATE TABLE IF NOT EXISTS agents (
    id                  BIGSERIAL PRIMARY KEY,
    public_id           VARCHAR(64)  NOT NULL UNIQUE,
    user_id             BIGINT       NOT NULL,
    org_id              BIGINT       NOT NULL,
    name                VARCHAR(200) NOT NULL,
    dashboard_url       VARCHAR(500) NOT NULL DEFAULT '',
    oc_platform_user_id VARCHAR(64),
    last_seen_at        TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agents_user_id_idx ON agents (user_id);
CREATE INDEX IF NOT EXISTS agents_org_id_idx  ON agents (org_id);
