-- +goose Up
-- SAM chat transcript. The registry row stays in sam_sessions; messages live
-- here because the Go runtime streams over HTTP instead of a Durable Object.
-- parts holds AI SDK UIMessage parts (text, reasoning, tool-<name>, step-start).
CREATE TABLE go_sam_messages (
    seq bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id text NOT NULL,
    session_id text NOT NULL REFERENCES sam_sessions (id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('user', 'assistant')),
    parts jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (session_id, id)
);

CREATE INDEX go_sam_messages_session_seq_idx ON go_sam_messages (session_id, seq);

-- Rolling summary of the oldest messages, so long chats stay under the model's
-- context budget. Messages up to through_seq are represented by summary.
CREATE TABLE go_sam_session_summaries (
    session_id text PRIMARY KEY REFERENCES sam_sessions (id) ON DELETE CASCADE,
    summary text NOT NULL,
    through_seq bigint NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE go_sam_session_summaries;
DROP TABLE go_sam_messages;
