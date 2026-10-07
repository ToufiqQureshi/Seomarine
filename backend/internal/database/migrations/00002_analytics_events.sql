-- +goose Up
-- Events reference sites by a compact integer instead of the text project id.
ALTER TABLE go_analytics_sites ADD COLUMN id bigint GENERATED ALWAYS AS IDENTITY UNIQUE;

-- One row per pageview. There is no IP and no cookie id: visitor_hash is
-- SHA-256 over a daily salt that exists only in Redis, so it cannot be tied
-- to a person or followed across days. referrer_host is NULL for direct and
-- internal pageviews; ai_source is set exactly for the ai channel.
CREATE TABLE go_analytics_events (
    site_id bigint NOT NULL REFERENCES go_analytics_sites (id) ON DELETE CASCADE,
    occurred_at timestamptz NOT NULL,
    path text NOT NULL,
    referrer_host text,
    channel text NOT NULL CHECK (channel IN ('ai', 'search', 'social', 'direct', 'referral', 'internal')),
    ai_source text CHECK (ai_source IN ('chatgpt', 'perplexity', 'gemini', 'claude', 'copilot', 'other_ai')),
    device text NOT NULL CHECK (device IN ('desktop', 'mobile', 'tablet')),
    visitor_hash bytea NOT NULL CHECK (length(visitor_hash) = 32),
    CHECK ((channel = 'ai') = (ai_source IS NOT NULL))
);

-- Every summary query filters one site over a time range.
CREATE INDEX go_analytics_events_site_time ON go_analytics_events (site_id, occurred_at);

-- +goose Down
DROP TABLE go_analytics_events;
ALTER TABLE go_analytics_sites DROP COLUMN id;
