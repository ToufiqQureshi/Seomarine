-- +goose Up
ALTER TABLE go_analytics_events
    ADD COLUMN country_code text CHECK (country_code ~ '^[A-Z]{2}$');

CREATE INDEX go_analytics_events_site_country_time
    ON go_analytics_events (site_id, country_code, occurred_at)
    WHERE country_code IS NOT NULL;

-- +goose Down
DROP INDEX go_analytics_events_site_country_time;
ALTER TABLE go_analytics_events DROP COLUMN country_code;
