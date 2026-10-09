-- +goose Up
CREATE TABLE go_rank_tracking_configs (
    id text PRIMARY KEY,
    project_id text NOT NULL,
    domain text NOT NULL,
    location_code integer NOT NULL DEFAULT 2840,
    language_code text NOT NULL DEFAULT 'en',
    location_name text,
    devices text NOT NULL DEFAULT 'both' CHECK (devices IN ('both', 'desktop', 'mobile')),
    serp_depth integer NOT NULL CHECK (serp_depth BETWEEN 10 AND 100 AND serp_depth % 10 = 0),
    schedule_interval text NOT NULL DEFAULT 'weekly' CHECK (schedule_interval IN ('daily', 'weekly', 'monthly', 'manual')),
    is_active boolean NOT NULL DEFAULT true,
    last_checked_at timestamptz,
    next_check_at timestamptz,
    last_skip_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((schedule_interval = 'manual') = (next_check_at IS NULL))
);

CREATE INDEX go_rank_tracking_configs_project_idx
    ON go_rank_tracking_configs (project_id, is_active, created_at);
CREATE UNIQUE INDEX go_rank_tracking_configs_national_idx
    ON go_rank_tracking_configs (project_id, domain, location_code) WHERE location_name IS NULL;
CREATE UNIQUE INDEX go_rank_tracking_configs_local_idx
    ON go_rank_tracking_configs (project_id, domain, location_code, location_name) WHERE location_name IS NOT NULL;
CREATE INDEX go_rank_tracking_configs_due_idx
    ON go_rank_tracking_configs (next_check_at) WHERE is_active AND next_check_at IS NOT NULL;

CREATE TABLE go_rank_tracking_keywords (
    id text PRIMARY KEY,
    config_id text NOT NULL REFERENCES go_rank_tracking_configs (id) ON DELETE CASCADE,
    keyword text NOT NULL CHECK (keyword <> ''),
    match_case boolean NOT NULL DEFAULT false,
    search_volume integer,
    keyword_difficulty integer,
    cpc double precision,
    metrics_fetched_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX go_rank_tracking_keywords_config_keyword_idx
    ON go_rank_tracking_keywords (config_id, keyword);

CREATE TABLE go_rank_check_runs (
    id text PRIMARY KEY,
    config_id text NOT NULL REFERENCES go_rank_tracking_configs (id) ON DELETE CASCADE,
    project_id text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    keywords_total integer NOT NULL DEFAULT 0 CHECK (keywords_total >= 0),
    keywords_checked integer NOT NULL DEFAULT 0 CHECK (keywords_checked >= 0),
    is_subset_run boolean NOT NULL DEFAULT false,
    error_message text,
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX go_rank_check_runs_config_idx ON go_rank_check_runs (config_id, started_at DESC);
CREATE INDEX go_rank_check_runs_project_idx ON go_rank_check_runs (project_id, started_at DESC);
-- At most one in-flight run per config: a second trigger fails on this index.
CREATE UNIQUE INDEX go_rank_check_runs_one_active_idx
    ON go_rank_check_runs (config_id) WHERE status IN ('pending', 'running');

CREATE TABLE go_rank_snapshots (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id text NOT NULL REFERENCES go_rank_check_runs (id) ON DELETE CASCADE,
    -- No FK on purpose: history of a removed keyword stays readable.
    tracking_keyword_id text NOT NULL,
    keyword text NOT NULL,
    device text NOT NULL CHECK (device IN ('desktop', 'mobile')),
    position integer CHECK (position IS NULL OR position >= 1),
    url text,
    serp_features jsonb NOT NULL DEFAULT '[]',
    checked_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX go_rank_snapshots_keyword_device_idx
    ON go_rank_snapshots (tracking_keyword_id, device, checked_at);
CREATE UNIQUE INDEX go_rank_snapshots_run_keyword_device_idx
    ON go_rank_snapshots (run_id, tracking_keyword_id, device);

-- +goose Down
DROP TABLE go_rank_snapshots;
DROP TABLE go_rank_check_runs;
DROP TABLE go_rank_tracking_keywords;
DROP TABLE go_rank_tracking_configs;
