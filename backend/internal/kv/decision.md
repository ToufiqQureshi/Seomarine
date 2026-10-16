# Key-value and Redis access: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Provides the shared Redis client primitives used for caches, rate limits, queues and short-lived state.

## Decisions to preserve

- Every Redis call must use request context and bounded timeouts. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.
- Do not treat cache data as authoritative customer records. _Why:_ A cache can be stale or absent, so it must not replace the authoritative source or change the result silently.
- Choose fail-open/fail-closed per security and cost risk; document it at the consumer. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.

## Dependencies

Redis.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
