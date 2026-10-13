-- +goose Up
-- What a credit reservation was for, so the Billing screen can show where a
-- workspace's credits went.
ALTER TABLE go_billing_credit_reservations
    ADD COLUMN feature text NOT NULL DEFAULT '' CHECK (length(feature) <= 64);

CREATE INDEX idx_go_billing_credit_ledger_org_created
    ON go_billing_credit_ledger (organization_id, created_at);

-- Every settled provider cost in US dollars, across DataForSEO and AI models,
-- staff workspaces included. The spend guard sums today's rows to stop all
-- paid provider work once the daily safety limit is reached.
CREATE TABLE go_provider_spend (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organization (id) ON DELETE CASCADE,
    feature text NOT NULL CHECK (length(feature) BETWEEN 1 AND 64),
    cost_usd numeric(12, 6) NOT NULL CHECK (cost_usd >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX go_provider_spend_created ON go_provider_spend (created_at);

-- +goose Down
DROP TABLE go_provider_spend;
DROP INDEX idx_go_billing_credit_ledger_org_created;
ALTER TABLE go_billing_credit_reservations DROP COLUMN feature;
