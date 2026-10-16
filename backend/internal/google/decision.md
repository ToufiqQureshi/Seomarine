# Google OAuth and account grants: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Starts OAuth, encrypts tokens, refreshes Google access and manages account/property grant removal.

## Decisions to preserve

- Validate state, PKCE, exact redirect/callback origins and requested scopes. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Keep refresh tokens encrypted; never retry a non-idempotent Google request after 401. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Account removal impact and owner check must happen before destructive deletes. _Why:_ These actions affect a customer account, site or external service and must follow an explicit user action with a clear result.

## Dependencies

Google OAuth/token endpoints; encrypted token storage; GA4/GSC consumers.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
