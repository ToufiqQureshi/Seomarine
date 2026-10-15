-- +goose Up
-- Author profiles for content-editor drafts. They live on the project and
-- carry no auth: they only feed the WordPress byline and the Article schema.
CREATE TABLE go_content_authors (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name text NOT NULL,
    role text NOT NULL DEFAULT '',
    bio text NOT NULL DEFAULT '',
    photo_url text NOT NULL DEFAULT '',
    profile_url text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX go_content_authors_project_idx ON go_content_authors (project_id);

-- Which author wrote a draft and when it was last reviewed. author_id is a
-- real reference so deleting an author cannot leave a draft pointing at a
-- missing row. Nullable: an author and a review date are optional.
ALTER TABLE go_content_drafts
    ADD COLUMN author_id text REFERENCES go_content_authors (id) ON DELETE SET NULL,
    ADD COLUMN last_reviewed_at date;

-- +goose Down
ALTER TABLE go_content_drafts
    DROP COLUMN author_id,
    DROP COLUMN last_reviewed_at;
DROP TABLE go_content_authors;
