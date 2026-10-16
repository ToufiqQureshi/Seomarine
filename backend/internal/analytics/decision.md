# Uplumo Analytics: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Collects first-party pageviews/custom events and returns project traffic summaries.

## Decisions to preserve

- Do not store cookies or raw IP addresses. _Why:_ Collecting or retaining less identifying data reduces privacy exposure if logs, storage or exports are accessed improperly.
- Public ingestion must keep same-origin/domain validation, bot filtering, payload bounds and per-IP/site limits. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.
- Custom events must not inflate pageview totals or bypass ingestion checks. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.

## Dependencies

Postgres analytics tables; Redis daily salt/rate-limit counters; no paid SEO provider.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
