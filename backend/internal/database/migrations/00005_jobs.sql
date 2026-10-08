-- +goose Up
CREATE TABLE go_jobs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    queue text NOT NULL CHECK (length(queue) BETWEEN 1 AND 128),
    idempotency_key text,
    payload jsonb NOT NULL,
    state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'succeeded', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 100),
    timeout_seconds integer NOT NULL DEFAULT 300 CHECK (timeout_seconds BETWEEN 1 AND 86400),
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    worker_id text,
    claim_version bigint NOT NULL DEFAULT 0,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    CHECK ((state = 'running') = (lease_until IS NOT NULL AND worker_id IS NOT NULL)),
    CHECK ((state IN ('succeeded', 'failed')) = (finished_at IS NOT NULL))
);

CREATE UNIQUE INDEX go_jobs_idempotency
    ON go_jobs (queue, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX go_jobs_ready
    ON go_jobs (queue, available_at, id) WHERE state IN ('queued', 'running');

-- +goose Down
DROP TABLE go_jobs;
