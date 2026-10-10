-- +goose Up
-- Credit balance and transaction ledger for DataForSEO & usage metering.
CREATE TABLE go_billing_credits (
    organization_id text PRIMARY KEY REFERENCES organization (id) ON DELETE CASCADE,
    balance_credits bigint NOT NULL DEFAULT 0 CHECK (balance_credits >= 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE go_billing_credit_reservations (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organization (id) ON DELETE CASCADE,
    amount_credits bigint NOT NULL CHECK (amount_credits > 0),
    status text NOT NULL CHECK (status IN ('reserved', 'settled', 'refunded')),
    idempotency_key text UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE go_billing_credit_ledger (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organization (id) ON DELETE CASCADE,
    reservation_id text REFERENCES go_billing_credit_reservations (id) ON DELETE SET NULL,
    amount_credits bigint NOT NULL,
    type text NOT NULL CHECK (type IN ('reserve', 'settle', 'refund', 'grant')),
    reason text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_go_billing_credit_reservations_org ON go_billing_credit_reservations (organization_id);
CREATE INDEX idx_go_billing_credit_ledger_org ON go_billing_credit_ledger (organization_id);

-- +goose Down
DROP TABLE go_billing_credit_ledger;
DROP TABLE go_billing_credit_reservations;
DROP TABLE go_billing_credits;
