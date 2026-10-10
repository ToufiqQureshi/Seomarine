-- Go-owned tables share the legacy app's database, so every one is prefixed
-- go_ to keep it out of the legacy schema's namespace.

-- +goose Up
-- One tracked site per project. site_key is the public id the tracker script
-- sends with every event.
CREATE TABLE go_analytics_sites (
    project_id text PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    site_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE go_analytics_sites;
