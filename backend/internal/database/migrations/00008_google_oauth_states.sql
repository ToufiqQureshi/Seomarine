-- +goose Up
CREATE TABLE go_google_oauth_states (
    state_hash bytea PRIMARY KEY,
    provider text NOT NULL CHECK (provider IN ('gsc', 'ga4')),
    user_id text NOT NULL,
    callback_path text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX go_google_oauth_states_expires_at_idx ON go_google_oauth_states (expires_at);

-- +goose Down
DROP TABLE go_google_oauth_states;
