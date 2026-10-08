-- +goose Up
-- One row per billed DataForSEO task, so an organization's provider spend can
-- be shown and capped. cost_usd is the provider-reported price of the task.
CREATE TABLE go_dataforseo_usage (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organization (id) ON DELETE CASCADE,
    path text NOT NULL CHECK (length(path) BETWEEN 1 AND 512),
    cost_usd numeric(12, 6) NOT NULL CHECK (cost_usd >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX go_dataforseo_usage_org_created
    ON go_dataforseo_usage (organization_id, created_at);

-- +goose Down
DROP TABLE go_dataforseo_usage;
