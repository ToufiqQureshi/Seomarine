-- +goose Up
-- The spend guard adds today's open holds to today's settled spend when it
-- checks the daily limits; this keeps that sum to the few open rows.
CREATE INDEX go_billing_credit_reservations_open
    ON go_billing_credit_reservations (organization_id, created_at)
    WHERE status = 'reserved';

-- +goose Down
DROP INDEX go_billing_credit_reservations_open;
