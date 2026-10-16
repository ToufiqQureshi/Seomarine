# Sessions and organization access: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Verifies Better Auth sessions and authorizes users, projects, organizations and membership actions.

## Decisions to preserve

- Cookie signatures, expiry and revocation are authoritative; never trust frontend session cache for API access. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Project and active-organization IDs must be re-authorized from Postgres on every request. _Why:_ Callers can tamper with IDs and UI state, so server-verified membership must prevent cross-customer access.
- Organization removal/transfer must preserve owner invariants and clear stale active membership. _Why:_ Ownership changes must not leave an organization without an owner or keep a removed user active.

## Dependencies

Better Auth tables and session cookie; project/team services.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
