# Model Context Protocol server: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Authenticates API-key/OAuth callers and exposes registered Go tools with per-tool project and plan checks.

## Decisions to preserve

- Never forward an unregistered call to legacy/other backend code. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Tool project IDs are untrusted; authorize every call and hide IDs from schemas when server-bound. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Bound JSON bodies, tool output, call ceilings and OAuth tokens; classify read-only vs spend/mutation tools explicitly. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Dependencies

API keys, OAuth/Postgres token tables, project auth, feature services and provider cost guards.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
