-- +goose Up
-- Provider task ids of queued (scheduled) rank checks. A task is charged when it
-- is posted, so its id must be stored at once: after a crash the worker collects
-- the paid tasks instead of posting, and paying for, them again.
CREATE TABLE go_rank_check_tasks (
    run_id text NOT NULL REFERENCES go_rank_check_runs (id) ON DELETE CASCADE,
    tracking_keyword_id text NOT NULL,
    device text NOT NULL CHECK (device IN ('desktop', 'mobile')),
    task_id text NOT NULL,
    state text NOT NULL DEFAULT 'posted' CHECK (state IN ('posted', 'collected', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, tracking_keyword_id, device)
);

-- +goose Down
DROP TABLE go_rank_check_tasks;
