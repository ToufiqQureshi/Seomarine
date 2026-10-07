-- +goose Up
-- An organization's current Razorpay subscription. A new checkout replaces a
-- subscription that has ended or was never paid, so there is one row per
-- organization. event_at is the Razorpay time of the state stored here;
-- webhooks older than it are stale and change nothing.
CREATE TABLE go_billing_subscriptions (
    organization_id text PRIMARY KEY REFERENCES organization (id) ON DELETE CASCADE,
    razorpay_subscription_id text NOT NULL UNIQUE,
    plan text NOT NULL CHECK (plan IN ('pro')),
    status text NOT NULL CHECK (status IN (
        'created', 'authenticated', 'active', 'pending', 'halted',
        'paused', 'cancelled', 'completed', 'expired'
    )),
    current_period_end timestamptz,
    event_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Razorpay delivers a webhook at least once; an event id recorded here has
-- been applied and its redeliveries are ignored.
CREATE TABLE go_billing_webhook_events (
    event_id text PRIMARY KEY,
    received_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE go_billing_webhook_events;
DROP TABLE go_billing_subscriptions;
