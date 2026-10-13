-- +goose Up
ALTER TABLE go_billing_subscriptions
    DROP CONSTRAINT go_billing_subscriptions_plan_check;
ALTER TABLE go_billing_subscriptions
    ADD CONSTRAINT go_billing_subscriptions_plan_check
    CHECK (plan IN ('pro', 'basic', 'agency'));

-- +goose Down
-- Subscription rows on newer paid tiers must be migrated before rolling back.
ALTER TABLE go_billing_subscriptions
    DROP CONSTRAINT go_billing_subscriptions_plan_check;
ALTER TABLE go_billing_subscriptions
    ADD CONSTRAINT go_billing_subscriptions_plan_check CHECK (plan IN ('pro'));
