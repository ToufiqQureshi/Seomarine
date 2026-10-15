# Keyword and local business research: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Owns keyword/serp research, location resolution, saved keywords and local business provider operations.

## Decisions to preserve

- Validate every billed task before provider call; no blind retries of paid task-post endpoints. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Cache keys include project/organization, geography and all result-changing options. _Why:_ A cache can be stale or absent, so it must not replace the authoritative source or change the result silently.
- Keep saved keyword rows and tags project-scoped; guard batches/caps and null metrics. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.

## Dependencies

DataForSEO; Google Ads for selected markets; Postgres saved rows; Redis caches; platform market registry.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
