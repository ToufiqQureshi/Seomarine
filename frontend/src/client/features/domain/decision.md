# Domain overview and research: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Shows organic estimates, keyword/page tables and filtered domain research.

## Decisions to preserve

- Domain research can incur provider charges; validate target, country and filter budget before calling the provider. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Keep project market defaults explicit and reject unsupported country/language pairs. _Why:_ A silent fallback can target the wrong market or metric and produce misleading or wasted research.
- Never imply provider estimates are exact Search Console measurements. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Verification pointers

- `domainRouteState.test.ts`
- `domainResultLinks.test.ts`
