-- +goose Up
CREATE TABLE go_content_drafts (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    title text NOT NULL DEFAULT '',
    target_keyword text NOT NULL DEFAULT '',
    slug text NOT NULL DEFAULT '',
    meta_description text NOT NULL DEFAULT '',
    content text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX go_content_drafts_project_updated_idx
    ON go_content_drafts (project_id, updated_at DESC);

-- +goose Down
DROP TABLE go_content_drafts;
