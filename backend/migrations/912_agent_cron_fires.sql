CREATE TABLE IF NOT EXISTS agent_cron_fires (
    id           BIGSERIAL PRIMARY KEY,
    agent_row_id BIGINT       NOT NULL,
    job_id       VARCHAR(200) NOT NULL,
    fire_at      TIMESTAMPTZ  NOT NULL,
    callback_url VARCHAR(500) NOT NULL,
    dedup_key    VARCHAR(300) NOT NULL UNIQUE,
    schedule_id  VARCHAR(64)  NOT NULL,
    state        VARCHAR(16)  NOT NULL DEFAULT 'armed',
    attempts     INTEGER      NOT NULL DEFAULT 0,
    last_error   VARCHAR(500) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_cron_fires_agent_idx ON agent_cron_fires (agent_row_id);
CREATE INDEX IF NOT EXISTS agent_cron_fires_due_idx   ON agent_cron_fires (state, fire_at);
