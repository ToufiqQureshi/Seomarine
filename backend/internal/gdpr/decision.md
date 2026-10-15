# Account erasure: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Finds, previews and erases only the data owned by one user within one Postgres transaction.

## Decisions to preserve

- Dry-run remains default; destructive execution needs explicit operator intent. _Why:_ These actions affect a customer account, site or external service and must follow an explicit user action with a clear result.
- Never erase shared organization data when another member owns it. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Report completion only after the whole transaction commits; preserve auditability without retaining erased personal data. _Why:_ Collecting or retaining less identifying data reduces privacy exposure if logs, storage or exports are accessed improperly.

## Dependencies

Postgres tables; CLI and database connection.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
