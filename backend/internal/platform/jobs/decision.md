# Postgres background job queue: decisions

> Internal guide for developers and coding agents. Update with code changes.

## Responsibility

Enqueues, leases, retries and completes asynchronous jobs with optional idempotency keys.

## Decisions to preserve

- Use stable idempotency keys for work that must not duplicate provider spend. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- At-least-once means handlers must be idempotent; a lease is not exactly-once execution. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Keep attempts, timeout, payload and batch bounds; stale workers cannot complete a newer claim. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.

## Cost and risk boundary

Postgres `go_jobs`; worker lifecycle in server startup.
