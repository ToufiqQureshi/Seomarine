-- +goose Up
-- One row per custom event a site sends, such as a button click. Kept apart
-- from pageviews so page counts stay exact. Same privacy as pageviews: no IP,
-- no cookie id; visitor_hash rotates daily.
CREATE TABLE go_analytics_custom_events (
    site_id bigint NOT NULL REFERENCES go_analytics_sites (id) ON DELETE CASCADE,
    occurred_at timestamptz NOT NULL,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
    path text NOT NULL,
    visitor_hash bytea NOT NULL CHECK (length(visitor_hash) = 32)
);

CREATE INDEX go_analytics_custom_events_site_time ON go_analytics_custom_events (site_id, occurred_at);

-- +goose Down
DROP TABLE go_analytics_custom_events;
