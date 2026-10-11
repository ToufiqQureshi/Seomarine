# Credit usage and settlement design

## Scope

Go-owned organization balances and an append-only ledger for provider usage.
Razorpay subscriptions remain the plan source. Credits use the legacy pricing
rules: 1.28 markup over provider cost and 1,000 credits per USD.

## Invariants

- A reservation is scoped by `(organization_id, idempotency_key)`; retries
  in one organization return the same reservation, while another organization
  may use the same key.
- The organization balance row is locked before a new reservation is created.
  The balance cannot become negative.
- A `reserve` ledger entry is negative for the amount held.
- `settle` records only the difference from the hold: positive for a refund,
  negative for additional balance consumed. It must not repeat the full actual
  usage charge.
- If actual usage exceeds the reservation and available balance, settlement
  clamps the available balance to zero and writes a negative `debt` entry for
  the uncollected shortfall.
- A `refund` ledger entry is positive for the released reservation.
- Reservation state changes and balance/ledger updates commit in one
  PostgreSQL transaction. Closed reservations cannot be settled or refunded
  again.
- Database failures stay wrapped with their original error; only a missing or
  too-small balance maps to `ErrInsufficientBalance`.

## Owner decisions

- Recommendation: keep the legacy 1.28 markup and 1,000 credits/USD conversion
  until pricing is reviewed separately.
- Recommendation: keep debt visible in the ledger while preventing negative
  spendable balance. The debt ledger is a liability; it is separate from the
  available-balance sum.

## Implementation status

The ledger correctness change and schema update are on the credits work branch.
The authenticated balance endpoint is wired. DataForSEO/MCP
reserve-before-call and settle/refund-after-call, Razorpay-plan integration in
place of Autumn usage, and end-to-end validation remain pending. Do not consider credits migration
complete until those pieces and real PostgreSQL tests pass.
