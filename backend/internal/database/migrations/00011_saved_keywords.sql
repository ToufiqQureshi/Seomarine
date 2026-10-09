-- +goose Up
CREATE TABLE go_saved_keywords (
    id text PRIMARY KEY,
    project_id text NOT NULL,
    keyword text NOT NULL CHECK (keyword <> ''),
    location_code integer NOT NULL DEFAULT 2840,
    language_code text NOT NULL DEFAULT 'en',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX go_saved_keywords_unique_idx
    ON go_saved_keywords (project_id, keyword, location_code, language_code);
CREATE INDEX go_saved_keywords_project_created_idx ON go_saved_keywords (project_id, created_at);

CREATE TABLE go_saved_keyword_tags (
    id text PRIMARY KEY,
    project_id text NOT NULL,
    name text NOT NULL CHECK (name <> ''),
    normalized_name text NOT NULL CHECK (normalized_name <> ''),
    -- Palette key; NULL means the page derives a stable color from the id.
    color text CHECK (color IN ('slate', 'rose', 'amber', 'lime', 'emerald', 'sky', 'violet', 'fuchsia')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX go_saved_keyword_tags_name_idx ON go_saved_keyword_tags (project_id, normalized_name);

CREATE TABLE go_saved_keyword_tag_assignments (
    saved_keyword_id text NOT NULL REFERENCES go_saved_keywords (id) ON DELETE CASCADE,
    tag_id text NOT NULL REFERENCES go_saved_keyword_tags (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (saved_keyword_id, tag_id)
);

CREATE INDEX go_saved_keyword_tag_assignments_tag_idx ON go_saved_keyword_tag_assignments (tag_id);

-- Latest metrics per keyword and market within a project, joined onto the saved list.
CREATE TABLE go_keyword_metrics (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id text NOT NULL,
    keyword text NOT NULL,
    location_code integer NOT NULL,
    language_code text NOT NULL DEFAULT 'en',
    search_volume integer CHECK (search_volume IS NULL OR search_volume >= 0),
    cpc double precision CHECK (cpc IS NULL OR cpc >= 0),
    competition double precision CHECK (competition IS NULL OR competition BETWEEN 0 AND 1),
    keyword_difficulty integer CHECK (keyword_difficulty IS NULL OR keyword_difficulty BETWEEN 0 AND 100),
    intent text,
    monthly_searches jsonb NOT NULL DEFAULT '[]',
    fetched_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX go_keyword_metrics_unique_idx
    ON go_keyword_metrics (project_id, keyword, location_code, language_code);

-- +goose Down
DROP TABLE go_keyword_metrics;
DROP TABLE go_saved_keyword_tag_assignments;
DROP TABLE go_saved_keyword_tags;
DROP TABLE go_saved_keywords;
