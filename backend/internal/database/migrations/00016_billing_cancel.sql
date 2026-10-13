-- +goose Up
-- True when the customer cancelled a subscription whose paid cycle is still
-- running: the plan stays until current_period_end and is not charged again.
ALTER TABLE go_billing_subscriptions
    ADD COLUMN cancel_at_period_end boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE go_billing_subscriptions DROP COLUMN cancel_at_period_end;
