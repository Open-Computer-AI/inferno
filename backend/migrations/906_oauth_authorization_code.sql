CREATE TABLE IF NOT EXISTS oauth_authorization_codes (
    id                    BIGSERIAL PRIMARY KEY,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    code                  VARCHAR(128) NOT NULL UNIQUE,
    client_id             VARCHAR(128) NOT NULL,
    user_id               BIGINT       NOT NULL,
    redirect_uri          VARCHAR(512) NOT NULL,
    scope                 VARCHAR(255) NOT NULL DEFAULT '',
    code_challenge        VARCHAR(128) NOT NULL,
    code_challenge_method VARCHAR(8)   NOT NULL,
    status                VARCHAR(16)  NOT NULL DEFAULT 'pending',
    issued_token_family   VARCHAR(64),
    expires_at            TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS oauth_authorization_codes_status_idx
    ON oauth_authorization_codes (status);
CREATE INDEX IF NOT EXISTS oauth_authorization_codes_expires_at_idx
    ON oauth_authorization_codes (expires_at);
