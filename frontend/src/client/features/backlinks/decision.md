# Backlink research: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Why the feature is structured this way

Shows domain/page backlink summaries, referring domains and top linked pages.

## Decisions to preserve

- **Backlink lookups are paid: validation and plan checks must happen before provider spend** _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- **A subfolder or exact-page scope may not support every aggregate** — preserve backend scope notes and null values. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- **Never silently retry paid POST requests or present missing provider data as zero** _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Verification pointers

- `backlinksFilterTypes.test.ts`
