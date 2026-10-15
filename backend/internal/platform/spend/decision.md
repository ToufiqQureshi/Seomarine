# Provider spend guards and queues: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Provides shared reservations/limits and queue mechanics that prevent unbounded provider spend or duplicate jobs.

## Decisions to preserve

- A customer’s normal usage limit must be per organization/plan so one workspace cannot consume another’s allowance. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.
- Retries must be classified by idempotency/provider billing behavior; never retry billed task creation blindly. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Guard and queue failures must have explicit fail-open/closed behavior and preserve reservation cleanup. _Why:_ Explicit failure behavior helps callers recover safely instead of treating an error as a valid result.

## Dependencies

Postgres provider spend/credit holds; Redis/Postgres queue; billing.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
