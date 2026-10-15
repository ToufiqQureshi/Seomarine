-- +goose Up
-- SERP briefs are intentionally shared across organizations. The key contains
-- only normalized query and market data; tenant-specific filtering happens
-- after a cached brief is read.
CREATE TABLE go_content_brief_cache (
    cache_key text PRIMARY KEY CHECK (cache_key ~ '^[0-9a-f]{64}$'),
    brief jsonb NOT NULL,
    fetched_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    CHECK (expires_at > fetched_at)
);

CREATE INDEX go_content_brief_cache_expiry_idx
    ON go_content_brief_cache (expires_at);

-- +goose Down
DROP TABLE go_content_brief_cache;
