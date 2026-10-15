# Plans, credits and provider spend: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** GET /api/v1/billing/{plans|status|credits|usage}; POST checkout/cancel/usage-events
- **Source:** `handler.go`, `service.go`, `credits.go`, `spendguard.go`

## Request or job flow

Handlers read the single plan catalog; user checkout creates a Razorpay subscription; webhooks verify signatures and deduplicate event IDs; provider calls reserve credits, settle actual usage or refund safe failures.

## Dependencies and cost boundary

Postgres ledger/reservations/subscriptions; Razorpay adapter; plan catalog.

## Failure, security and data invariants

- The plan catalog is the only source for price/limits; never trust client-supplied amounts or entitlements.
- Credit holds, settlement/refund and webhook processing must be idempotent under retries and concurrency.
- Scope spend to the responsible workspace; emergency global limits must not punish unrelated customers.
