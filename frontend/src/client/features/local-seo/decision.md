# Local SEO research: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Looks up local businesses/profiles and runs a bounded local search rank grid.

## Decisions to preserve

- Local SERP/business requests are paid; validate location, grid dimensions and result limits before spend. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Per-point failures must remain visible; do not collapse a partial grid into a false complete result. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- No automatic recurring grid checks are created by opening the page. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.

## Verification pointers

- `localSeoApi.test.ts`
