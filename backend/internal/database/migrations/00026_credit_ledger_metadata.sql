-- +goose Up
ALTER TABLE go_billing_credit_reservations
    ADD COLUMN endpoint text NOT NULL DEFAULT '',
    ADD COLUMN user_id text;
ALTER TABLE go_billing_credit_ledger
    ADD COLUMN user_id text,
    ADD COLUMN feature text NOT NULL DEFAULT '',
    ADD COLUMN endpoint text NOT NULL DEFAULT '',
    ADD COLUMN provider_cost_usd numeric(12, 6) NOT NULL DEFAULT 0 CHECK (provider_cost_usd >= 0),
    ADD COLUMN cached boolean NOT NULL DEFAULT false,
    ADD COLUMN status text NOT NULL DEFAULT 'recorded';

UPDATE go_billing_credit_ledger l
SET user_id = r.user_id,
    feature = COALESCE(NULLIF(r.feature, ''), l.reason, ''),
    endpoint = COALESCE(r.endpoint, ''),
    status = CASE l.type WHEN 'reserve' THEN 'reserved' WHEN 'settle' THEN 'settled' WHEN 'refund' THEN 'refunded' WHEN 'grant' THEN 'granted' ELSE 'expired' END
FROM go_billing_credit_reservations r
WHERE l.reservation_id = r.id;
UPDATE go_billing_credit_ledger
SET status = CASE type WHEN 'reserve' THEN 'reserved' WHEN 'settle' THEN 'settled' WHEN 'refund' THEN 'refunded' WHEN 'grant' THEN 'granted' ELSE 'expired' END
WHERE status = 'recorded';

-- +goose Down
ALTER TABLE go_billing_credit_ledger
    DROP COLUMN status,
    DROP COLUMN cached,
    DROP COLUMN provider_cost_usd,
    DROP COLUMN endpoint,
    DROP COLUMN feature,
    DROP COLUMN user_id;
ALTER TABLE go_billing_credit_reservations DROP COLUMN user_id, DROP COLUMN endpoint;
