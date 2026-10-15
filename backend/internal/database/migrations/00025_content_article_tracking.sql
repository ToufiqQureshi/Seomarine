-- +goose Up
-- One opt-in row per draft. The publication join in the repository only
-- creates a row for a successfully published draft. Draft deletion cascades.
CREATE TABLE go_content_article_tracking (
    draft_id text PRIMARY KEY REFERENCES go_content_drafts (id) ON DELETE CASCADE,
    project_id text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    organization_id text NOT NULL REFERENCES organization (id) ON DELETE CASCADE,
    published_url text NOT NULL,
    target_keyword text NOT NULL,
    rank_config_id text REFERENCES go_rank_tracking_configs (id) ON DELETE SET NULL,
    ai_prompt text NOT NULL,
    ai_enabled boolean NOT NULL DEFAULT true,
    ai_next_check_at timestamptz NOT NULL DEFAULT now(),
    ai_last_checked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, draft_id)
);

CREATE INDEX go_content_article_tracking_ai_due_idx
    ON go_content_article_tracking (ai_next_check_at)
    WHERE ai_enabled;
CREATE INDEX go_content_article_tracking_org_project_idx
    ON go_content_article_tracking (organization_id, project_id);

CREATE TABLE go_content_article_ai_results (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    draft_id text NOT NULL REFERENCES go_content_article_tracking (draft_id) ON DELETE CASCADE,
    model text NOT NULL,
    status text NOT NULL CHECK (status IN ('success', 'failed')),
    result jsonb NOT NULL,
    fetched_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX go_content_article_ai_results_latest_idx
    ON go_content_article_ai_results (draft_id, fetched_at DESC, id DESC);

-- +goose Down
DROP TABLE go_content_article_ai_results;
DROP TABLE go_content_article_tracking;
