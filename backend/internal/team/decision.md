# Team invites and seats: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Creates and manages organization invitations with role, seat and email-rate controls.

## Decisions to preserve

- Check role, membership and seat cap before sending mail or creating membership. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Rate-limit by recipient and organization; fail closed if anti-abuse budget is unavailable. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Never allow invitation flows to remove/replace owner without explicit ownership transfer. _Why:_ These actions affect a customer account, site or external service and must follow an explicit user action with a clear result.

## Dependencies

Postgres organization/invitations; Redis invite rate limits; Loops transactional email; plan catalog.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
