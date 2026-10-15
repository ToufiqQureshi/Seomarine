# Local SEO endpoint layer: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Validates and orchestrates local business lookup/profile and bounded Google Maps rank grids.

## Decisions to preserve

- Run plan, bounds and credit checks before paid provider calls. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Bound grid size to control worst-case provider spend and request time. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Preserve individual errors; do not convert provider failures into a rank of zero. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Dependencies

`backend/internal/keywords` provider methods; DataForSEO billing/usage; plan service.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
