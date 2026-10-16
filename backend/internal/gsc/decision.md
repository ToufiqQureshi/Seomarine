# Google Search Console: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Manages selected Search Console property, reads search performance and inspects URLs.

## Decisions to preserve

- Property and grant must belong to the authorized organization/project. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Google quotas require bounded read-only retries; do not retry writes blindly. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Treat provider row limits and inspection errors accurately; null/empty is not proof of no search demand. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Dependencies

Google Search Console API; encrypted Google grants; selected property in Postgres.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
