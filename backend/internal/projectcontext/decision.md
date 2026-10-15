# Project context memory: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Stores bounded structured/custom project facts shared by settings, SAM and MCP.

## Decisions to preserve

- Keep project-scoped auth and atomic updates; concurrent writes must preserve caps/uniqueness. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Bound prompt memory (counts, text length and history window) to control model cost and avoid unbounded storage. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Omitted fields retain values only where documented; do not accidentally clear user data. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.

## Dependencies

Postgres project context rows; `projects` authorization; SAM/MCP consumers.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
