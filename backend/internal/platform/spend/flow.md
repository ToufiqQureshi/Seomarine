# Provider spend guards and queues: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** No public route; used by rank tracking, AI, audits and provider-backed features.
- **Source:** `spend.go`, `spend_test.go`

## Request or job flow

Feature asks the spend guard for a hold before billable work, scoped to the responsible organization; after the provider returns, the caller settles actual cost or releases the reservation. Durable job queuing is a separate package under `../jobs`.

## Dependencies and cost boundary

Postgres provider spend/credit holds; Redis/Postgres queue; billing.

## Failure, security and data invariants

- A customer’s normal usage limit must be per organization/plan so one workspace cannot consume another’s allowance.
- Retries must be classified by idempotency/provider billing behavior; never retry billed task creation blindly.
- Guard and queue failures must have explicit fail-open/closed behavior and preserve reservation cleanup.
