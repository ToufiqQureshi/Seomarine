# Rank tracking engine: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Schedules and executes bounded rank checks, stores runs/results and settles provider cost.

## Decisions to preserve

- Check estimated ceiling again before execution; a browser estimate is never authority. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Use idempotency and task state to prevent duplicate paid work across retries/restarts. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Scope caps to the workspace causing spend; expose partial failures and enforce schedule/pause rules. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Dependencies

DataForSEO SERP; Postgres configs/runs/tasks/snapshots; Redis jobs/locks; billing/spend guards.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
