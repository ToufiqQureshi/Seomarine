# Dashboard overview: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Reads audit and backlink snapshots for the project dashboard; supports an explicit backlink refresh route.

## Decisions to preserve

- Opening a dashboard must remain read-only and must not trigger paid provider requests. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.
- Refresh is an explicit action with server-side plan/credit checks and a visible stale/failure state. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.
- All database reads and refresh writes are project-scoped. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.

## Dependencies

Postgres snapshots; optional DataForSEO backlinks provider; plans and billing.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
