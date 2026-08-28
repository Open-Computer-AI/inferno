CREATE TABLE IF NOT EXISTS oauth_device_authorizations (
    id               BIGSERIAL PRIMARY KEY,
    device_code      VARCHAR(128) NOT NULL UNIQUE,
    user_code        VARCHAR(16)  NOT NULL UNIQUE,
    client_id        VARCHAR(128) NOT NULL,
    scope            VARCHAR(255) NOT NULL DEFAULT '',
    status           VARCHAR(16)  NOT NULL DEFAULT 'pending',
    approved_user_id BIGINT,
    expires_at       TIMESTAMPTZ  NOT NULL,
    last_polled_at   TIMESTAMPTZ,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS oauth_device_authorizations_status_idx
    ON oauth_device_authorizations (status);
CREATE INDEX IF NOT EXISTS oauth_device_authorizations_expires_at_idx
    ON oauth_device_authorizations (expires_at);
