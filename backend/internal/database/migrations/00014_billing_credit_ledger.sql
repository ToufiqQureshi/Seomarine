-- +goose Up
-- Idempotency keys identify one operation within an organization, not globally.
ALTER TABLE go_billing_credit_reservations
    DROP CONSTRAINT IF EXISTS go_billing_credit_reservations_idempotency_key_key;
ALTER TABLE go_billing_credit_reservations
    ADD CONSTRAINT go_billing_credit_reservations_org_idempotency_key_key
    UNIQUE (organization_id, idempotency_key);

-- An uncollectable settlement shortfall is distinct from available balance.
ALTER TABLE go_billing_credit_ledger
    DROP CONSTRAINT IF EXISTS go_billing_credit_ledger_type_check;
ALTER TABLE go_billing_credit_ledger
    ADD CONSTRAINT go_billing_credit_ledger_type_check
    CHECK (type IN ('reserve', 'settle', 'refund', 'grant', 'debt'));

-- +goose Down
ALTER TABLE go_billing_credit_ledger
    DROP CONSTRAINT IF EXISTS go_billing_credit_ledger_type_check;
ALTER TABLE go_billing_credit_ledger
    ADD CONSTRAINT go_billing_credit_ledger_type_check
    CHECK (type IN ('reserve', 'settle', 'refund', 'grant'));

ALTER TABLE go_billing_credit_reservations
    DROP CONSTRAINT IF EXISTS go_billing_credit_reservations_org_idempotency_key_key;
ALTER TABLE go_billing_credit_reservations
    ADD CONSTRAINT go_billing_credit_reservations_idempotency_key_key
    UNIQUE (idempotency_key);
