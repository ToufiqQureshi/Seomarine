-- +goose Up
-- The website a project publishes articles to. One per project. The secret
-- (a WordPress application password) is encrypted with AES-GCM before it is
-- stored; it is never sent back to the browser.
CREATE TABLE go_site_connections (
    project_id text PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    platform text NOT NULL CHECK (platform IN ('wordpress')),
    site_url text NOT NULL,
    api_base text NOT NULL,
    username text NOT NULL,
    secret_ciphertext text NOT NULL,
    site_name text NOT NULL DEFAULT '',
    connected_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Where a draft was published, so publishing it again updates that post
-- instead of creating a copy.
CREATE TABLE go_site_publications (
    draft_id text PRIMARY KEY REFERENCES go_content_drafts (id) ON DELETE CASCADE,
    project_id text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    site_url text NOT NULL,
    remote_id text NOT NULL,
    remote_url text NOT NULL,
    remote_status text NOT NULL,
    -- Images already copied to the site: {"<draft image url>": {"id": 12,
    -- "url": "<site url>"}}, so publishing again does not upload them twice.
    media jsonb NOT NULL DEFAULT '{}',
    published_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX go_site_publications_project_idx ON go_site_publications (project_id);

-- +goose Down
DROP TABLE go_site_publications;
DROP TABLE go_site_connections;
