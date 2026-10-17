-- +goose Up
CREATE TABLE go_billing_credit_grants (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organization (id) ON DELETE CASCADE,
    granted_credits bigint NOT NULL CHECK (granted_credits > 0),
    remaining_credits bigint NOT NULL CHECK (remaining_credits >= 0 AND remaining_credits <= granted_credits),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_go_billing_credit_grants_fifo
    ON go_billing_credit_grants (organization_id, expires_at, created_at)
    WHERE remaining_credits > 0;

CREATE TABLE go_billing_credit_reservation_grants (
    reservation_id text NOT NULL REFERENCES go_billing_credit_reservations (id) ON DELETE CASCADE,
    grant_id text NOT NULL REFERENCES go_billing_credit_grants (id) ON DELETE CASCADE,
    amount_credits bigint NOT NULL CHECK (amount_credits > 0),
    PRIMARY KEY (reservation_id, grant_id)
);

INSERT INTO go_billing_credit_grants (id, organization_id, granted_credits, remaining_credits, expires_at)
SELECT 'legacy-' || organization_id, organization_id, balance_credits, balance_credits, now() + interval '2 months'
FROM go_billing_credits WHERE balance_credits > 0;

ALTER TABLE go_billing_credit_ledger DROP CONSTRAINT go_billing_credit_ledger_type_check;
ALTER TABLE go_billing_credit_ledger ADD CONSTRAINT go_billing_credit_ledger_type_check
    CHECK (type IN ('reserve', 'settle', 'refund', 'grant', 'expire'));

-- +goose Down
ALTER TABLE go_billing_credit_ledger DROP CONSTRAINT go_billing_credit_ledger_type_check;
ALTER TABLE go_billing_credit_ledger ADD CONSTRAINT go_billing_credit_ledger_type_check
    CHECK (type IN ('reserve', 'settle', 'refund', 'grant'));
DROP TABLE go_billing_credit_reservation_grants;
DROP TABLE go_billing_credit_grants;
