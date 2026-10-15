# Backlink research: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Serves project-scoped backlink summaries, history, referring-domain and top-page data.

## Decisions to preserve

- Validate target/filter limits and verify plan before any billed provider task. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Preserve null/unsupported metrics by target scope; never present them as zero. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Do not retry a paid task automatically; partial provider limits must be visible. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Dependencies

DataForSEO paid endpoints and usage metering; project plan/credit checks.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
